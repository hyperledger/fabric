/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package etcdraft_test

import (
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path"
	"sync"
	"time"

	"code.cloudfoundry.org/clock/fakeclock"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/metrics/metricsfakes"
	mockconfig "github.com/hyperledger/fabric/common/mocks/config"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/consensus/etcdraft"
	"github.com/hyperledger/fabric/orderer/consensus/etcdraft/mocks"
	consensusmocks "github.com/hyperledger/fabric/orderer/consensus/mocks"
	mockblockcutter "github.com/hyperledger/fabric/orderer/mocks/common/blockcutter"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/orderer"
	raftprotos "github.com/hyperledger/fabric/protos/orderer/etcdraft"
	"github.com/hyperledger/fabric/protos/utils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/mock"
	"go.etcd.io/etcd/raft"
	"go.etcd.io/etcd/raft/raftpb"
	"go.uber.org/zap"
)

const (
	interval            = time.Second
	LongEventualTimeout = 10 * time.Second
	ELECTION_TICK       = 2
	HEARTBEAT_TICK      = 1
)

func init() {
	factory.InitFactories(nil)
}

// for some test cases we chmod file/dir to test failures caused by exotic permissions.
// however this does not work if tests are running as root, i.e. in a container.
func skipIfRoot() {
	u, err := user.Current()
	Expect(err).NotTo(HaveOccurred())
	if u.Uid == "0" {
		Skip("you are running test as root, there's no way to make files unreadable")
	}
}

var _ = Describe("Chain", func() {
	var (
		env       *common.Envelope
		channelID string
		tlsCA     tlsgen.CA
		logger    *flogging.FabricLogger
	)

	BeforeEach(func() {
		tlsCA, _ = tlsgen.NewCA()
		channelID = "test-channel"
		logger = flogging.NewFabricLogger(zap.NewExample())
		env = &common.Envelope{
			Payload: marshalOrPanic(&common.Payload{
				Header: &common.Header{ChannelHeader: marshalOrPanic(&common.ChannelHeader{Type: int32(common.HeaderType_MESSAGE), ChannelId: channelID})},
				Data:   []byte("TEST_MESSAGE"),
			}),
		}
	})

	Describe("Single Raft node", func() {
		var (
			configurator      *mocks.Configurator
			consenterMetadata *raftprotos.Metadata
			clock             *fakeclock.FakeClock
			opts              etcdraft.Options
			support           *consensusmocks.FakeConsenterSupport
			cutter            *mockblockcutter.Receiver
			storage           *raft.MemoryStorage
			observeC          chan raft.SoftState
			chain             *etcdraft.Chain
			dataDir           string
			walDir            string
			snapDir           string
			err               error
			fakeFields        *fakeMetricsFields
		)

		BeforeEach(func() {
			configurator = &mocks.Configurator{}
			configurator.On("Configure", mock.Anything, mock.Anything)
			clock = fakeclock.NewFakeClock(time.Now())
			storage = raft.NewMemoryStorage()

			dataDir, err = ioutil.TempDir("", "wal-")
			Expect(err).NotTo(HaveOccurred())
			walDir = path.Join(dataDir, "wal")
			snapDir = path.Join(dataDir, "snapshot")

			observeC = make(chan raft.SoftState, 1)

			support = &consensusmocks.FakeConsenterSupport{}
			support.ChainIDReturns(channelID)
			consenterMetadata = createMetadata(1, tlsCA)
			support.SharedConfigReturns(&mockconfig.Orderer{
				BatchTimeoutVal:      time.Hour,
				ConsensusMetadataVal: marshalOrPanic(consenterMetadata),
			})
			cutter = mockblockcutter.NewReceiver()
			support.BlockCutterReturns(cutter)

			// for block creator initialization
			support.HeightReturns(1)
			support.BlockReturns(getSeedBlock())

			meta := &raftprotos.RaftMetadata{
				Consenters:      map[uint64]*raftprotos.Consenter{},
				NextConsenterId: 1,
			}

			for _, c := range consenterMetadata.Consenters {
				meta.Consenters[meta.NextConsenterId] = c
				meta.NextConsenterId++
			}

			fakeFields = newFakeMetricsFields()

			opts = etcdraft.Options{
				RaftID:          1,
				Clock:           clock,
				TickInterval:    interval,
				ElectionTick:    ELECTION_TICK,
				HeartbeatTick:   HEARTBEAT_TICK,
				MaxSizePerMsg:   1024 * 1024,
				MaxInflightMsgs: 256,
				RaftMetadata:    meta,
				Logger:          logger,
				MemoryStorage:   storage,
				WALDir:          walDir,
				SnapDir:         snapDir,
				Metrics:         newFakeMetrics(fakeFields),
			}
		})

		campaign := func(clock *fakeclock.FakeClock, observeC <-chan raft.SoftState) {
			Eventually(func() bool {
				clock.Increment(interval)
				select {
				case s := <-observeC:
					return s.RaftState == raft.StateLeader
				default:
					return false
				}
			}, LongEventualTimeout).Should(BeTrue())
		}

		JustBeforeEach(func() {
			chain, err = etcdraft.NewChain(support, opts, configurator, nil, nil, observeC)
			Expect(err).NotTo(HaveOccurred())

			chain.Start()

			// When the Raft node bootstraps, it produces a ConfChange
			// to add itself, which needs to be consumed with Ready().
			// If there are pending configuration changes in raft,
			// it refuses to campaign, no matter how many ticks elapse.
			// This is not a problem in the production code because raft.Ready
			// will be consumed eventually, as the wall clock advances.
			//
			// However, this is problematic when using the fake clock and
			// artificial ticks. Instead of ticking raft indefinitely until
			// raft.Ready is consumed, this check is added to indirectly guarantee
			// that the first ConfChange is actually consumed and we can safely
			// proceed to tick the Raft FSM.
			Eventually(func() error {
				_, err := storage.Entries(1, 1, 1)
				return err
			}, LongEventualTimeout).ShouldNot(HaveOccurred())
		})

		AfterEach(func() {
			chain.Halt()
			Eventually(chain.Errored, LongEventualTimeout).Should(BeClosed())
			os.RemoveAll(dataDir)
		})

		Context("when a node starts up", func() {
			It("properly configures the communication layer", func() {
				expectedNodeConfig := nodeConfigFromMetadata(consenterMetadata)
				configurator.AssertCalled(testingInstance, "Configure", channelID, expectedNodeConfig)
			})

			It("correctly sets the metrics labels and publishes requisite metrics", func() {
				type withImplementers interface {
					WithCallCount() int
					WithArgsForCall(int) []string
				}
				metricsList := []withImplementers{
					fakeFields.fakeClusterSize,
					fakeFields.fakeIsLeader,
					fakeFields.fakeCommittedBlockNumber,
					fakeFields.fakeSnapshotBlockNumber,
					fakeFields.fakeLeaderChanges,
					fakeFields.fakeProposalFailures,
				}
				for _, m := range metricsList {
					Expect(m.WithCallCount()).To(Equal(1))
					Expect(func() string {
						return m.WithArgsForCall(0)[1]
					}()).To(Equal(channelID))
				}

				Expect(fakeFields.fakeClusterSize.SetCallCount()).To(Equal(1))
				Expect(fakeFields.fakeClusterSize.SetArgsForCall(0)).To(Equal(float64(1)))
				Expect(fakeFields.fakeIsLeader.SetCallCount()).To(Equal(1))
				Expect(fakeFields.fakeIsLeader.SetArgsForCall(0)).To(Equal(float64(0)))
			})
		})

		Context("when no Raft leader is elected", func() {
			It("fails to order envelope", func() {
				err := chain.Order(env, 0)
				Expect(err).To(MatchError("no Raft leader"))
				Expect(fakeFields.fakeProposalFailures.AddCallCount()).To(Equal(1))
				Expect(fakeFields.fakeProposalFailures.AddArgsForCall(0)).To(Equal(float64(1)))
			})
		})

		Context("when Raft leader is elected", func() {
			JustBeforeEach(func() {
				campaign(clock, observeC)
			})

			It("updates metrics upon leader election)", func() {
				Expect(fakeFields.fakeIsLeader.SetCallCount()).To(Equal(2))
				Expect(fakeFields.fakeIsLeader.SetArgsForCall(1)).To(Equal(float64(1)))
				Expect(fakeFields.fakeLeaderChanges.AddCallCount()).To(Equal(1))
				Expect(fakeFields.fakeLeaderChanges.AddArgsForCall(0)).To(Equal(float64(1)))
			})

			It("fails to order envelope if chain is halted", func() {
				chain.Halt()
				err := chain.Order(env, 0)
				Expect(err).To(MatchError("chain is stopped"))
				Expect(fakeFields.fakeProposalFailures.AddCallCount()).To(Equal(1))
				Expect(fakeFields.fakeProposalFailures.AddArgsForCall(0)).To(Equal(float64(1)))
			})

			It("produces blocks following batch rules", func() {
				close(cutter.Block)

				By("cutting next batch directly")
				cutter.CutNext = true
				err := chain.Order(env, 0)
				Expect(err).NotTo(HaveOccurred())
				Eventually(support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(1))
				Expect(fakeFields.fakeCommittedBlockNumber.SetCallCount()).Should(Equal(1))
				Expect(fakeFields.fakeCommittedBlockNumber.SetArgsForCall(0)).Should(Equal(float64(1)))

				By("respecting batch timeout")
				cutter.CutNext = false
				timeout := time.Second
				support.SharedConfigReturns(&mockconfig.Orderer{BatchTimeoutVal: timeout})
				err = chain.Order(env, 0)
				Expect(err).NotTo(HaveOccurred())

				clock.WaitForNWatchersAndIncrement(timeout, 2)
				Eventually(support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(2))
				Expect(fakeFields.fakeCommittedBlockNumber.SetCallCount()).Should(Equal(2))
				Expect(fakeFields.fakeCommittedBlockNumber.SetArgsForCall(1)).Should(Equal(float64(2)))
			})

			It("does not reset timer for every envelope", func() {
				close(cutter.Block)

				timeout := time.Second
				support.SharedConfigReturns(&mockconfig.Orderer{BatchTimeoutVal: timeout})

				err := chain.Order(env, 0)
				Expect(err).NotTo(HaveOccurred())
				Eventually(cutter.CurBatch, LongEventualTimeout).Should(HaveLen(1))

				clock.WaitForNWatchersAndIncrement(timeout/2, 2)

				err = chain.Order(env, 0)
				Expect(err).NotTo(HaveOccurred())
				Eventually(cutter.CurBatch, LongEventualTimeout).Should(HaveLen(2))

				// the second envelope should not reset the timer; it should
				// therefore expire if we increment it by just timeout/2
				clock.Increment(timeout / 2)
				Eventually(support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(1))
			})

			It("does not write a block if halted before timeout", func() {
				close(cutter.Block)
				timeout := time.Second
				support.SharedConfigReturns(&mockconfig.Orderer{BatchTimeoutVal: timeout})

				err := chain.Order(env, 0)
				Expect(err).NotTo(HaveOccurred())
				Eventually(cutter.CurBatch, LongEventualTimeout).Should(HaveLen(1))

				// wait for timer to start
				Eventually(clock.WatcherCount, LongEventualTimeout).Should(Equal(2))

				chain.Halt()
				Consistently(support.WriteBlockCallCount).Should(Equal(0))
			})

			It("stops the timer if a batch is cut", func() {
				close(cutter.Block)

				timeout := time.Second
				support.SharedConfigReturns(&mockconfig.Orderer{BatchTimeoutVal: timeout})

				err := chain.Order(env, 0)
				Expect(err).NotTo(HaveOccurred())
				Eventually(cutter.CurBatch, LongEventualTimeout).Should(HaveLen(1))

				clock.WaitForNWatchersAndIncrement(timeout/2, 2)

				By("force a batch to be cut before timer expires")
				cutter.CutNext = true
				err = chain.Order(env, 0)
				Expect(err).NotTo(HaveOccurred())

				Eventually(support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(1))
				b, _ := support.WriteBlockArgsForCall(0)
				Expect(b.Data.Data).To(HaveLen(2))
				Expect(cutter.CurBatch()).To(HaveLen(0))

				// this should start a fresh timer
				cutter.CutNext = false
				err = chain.Order(env, 0)
				Expect(err).NotTo(HaveOccurred())
				Eventually(cutter.CurBatch, LongEventualTimeout).Should(HaveLen(1))

				clock.WaitForNWatchersAndIncrement(timeout/2, 2)
				Consistently(support.WriteBlockCallCount).Should(Equal(1))

				clock.Increment(timeout / 2)

				Eventually(support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(2))
				b, _ = support.WriteBlockArgsForCall(1)
				Expect(b.Data.Data).To(HaveLen(1))
			})

			It("cut two batches if incoming envelope does not fit into first batch", func() {
				close(cutter.Block)

				timeout := time.Second
				support.SharedConfigReturns(&mockconfig.Orderer{BatchTimeoutVal: timeout})

				err := chain.Order(env, 0)
				Expect(err).NotTo(HaveOccurred())
				Eventually(cutter.CurBatch, LongEventualTimeout).Should(HaveLen(1))

				cutter.IsolatedTx = true
				err = chain.Order(env, 0)
				Expect(err).NotTo(HaveOccurred())

				Eventually(support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(2))
			})

			Context("revalidation", func() {
				BeforeEach(func() {
					close(cutter.Block)

					timeout := time.Hour
					support.SharedConfigReturns(&mockconfig.Orderer{BatchTimeoutVal: timeout})
					support.SequenceReturns(1)
				})

				It("enqueue if envelope is still valid", func() {
					support.ProcessNormalMsgReturns(1, nil)

					err := chain.Order(env, 0)
					Expect(err).NotTo(HaveOccurred())
					Eventually(cutter.CurBatch, LongEventualTimeout).Should(HaveLen(1))
				})

				It("does not enqueue if envelope is not valid", func() {
					support.ProcessNormalMsgReturns(1, errors.Errorf("Envelope is invalid"))

					err := chain.Order(env, 0)
					Expect(err).NotTo(HaveOccurred())
					Consistently(cutter.CurBatch).Should(HaveLen(0))
				})
			})

			It("unblocks Errored if chain is halted", func() {
				errorC := chain.Errored()
				Expect(errorC).NotTo(BeClosed())
				chain.Halt()
				Eventually(errorC, LongEventualTimeout).Should(BeClosed())
			})

			Describe("Config updates", func() {
				var (
					configEnv *common.Envelope
					configSeq uint64
				)

				Context("when a config update with invalid header comes", func() {

					BeforeEach(func() {
						configEnv = newConfigEnv(channelID,
							common.HeaderType_CONFIG_UPDATE, // invalid header; envelopes with CONFIG_UPDATE header never reach chain
							&common.ConfigUpdateEnvelope{ConfigUpdate: []byte("test invalid envelope")})
						configSeq = 0
					})

					It("should throw an error", func() {
						err := chain.Configure(configEnv, configSeq)
						Expect(err).To(MatchError("config transaction has unknown header type"))
						Expect(fakeFields.fakeProposalFailures.AddCallCount()).To(Equal(1))
						Expect(fakeFields.fakeProposalFailures.AddArgsForCall(0)).To(Equal(float64(1)))
					})
				})

				Context("when a type A config update comes", func() {

					Context("for existing channel", func() {

						// use to prepare the Orderer Values
						BeforeEach(func() {
							values := map[string]*common.ConfigValue{
								"BatchTimeout": {
									Version: 1,
									Value: marshalOrPanic(&orderer.BatchTimeout{
										Timeout: "3ms",
									}),
								},
							}
							configEnv = newConfigEnv(channelID,
								common.HeaderType_CONFIG,
								newConfigUpdateEnv(channelID, values),
							)
							configSeq = 0
						}) // BeforeEach block

						Context("without revalidation (i.e. correct config sequence)", func() {

							Context("without pending normal envelope", func() {
								It("should create a config block and no normal block", func() {
									err := chain.Configure(configEnv, configSeq)
									Expect(err).NotTo(HaveOccurred())
									Eventually(support.WriteConfigBlockCallCount, LongEventualTimeout).Should(Equal(1))
									Consistently(support.WriteBlockCallCount).Should(Equal(0))
									Expect(fakeFields.fakeCommittedBlockNumber.SetCallCount()).Should(Equal(1))
									Expect(fakeFields.fakeCommittedBlockNumber.SetArgsForCall(0)).Should(Equal(float64(1)))
								})
							})

							Context("with pending normal envelope", func() {
								It("should create a normal block and a config block", func() {
									// We do not need to block the cutter from ordering in our test case and therefore close this channel.
									close(cutter.Block)

									By("adding a normal envelope")
									err := chain.Order(env, 0)
									Expect(err).NotTo(HaveOccurred())
									Eventually(cutter.CurBatch, LongEventualTimeout).Should(HaveLen(1))

									// // clock.WaitForNWatchersAndIncrement(timeout, 2)

									By("adding a config envelope")
									err = chain.Configure(configEnv, configSeq)
									Expect(err).NotTo(HaveOccurred())

									Eventually(support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(1))
									Eventually(support.WriteConfigBlockCallCount, LongEventualTimeout).Should(Equal(1))
									Expect(fakeFields.fakeCommittedBlockNumber.SetCallCount()).Should(Equal(2))
									Expect(fakeFields.fakeCommittedBlockNumber.SetArgsForCall(1)).Should(Equal(float64(2)))
								})
							})
						})

						Context("with revalidation (i.e. incorrect config sequence)", func() {

							BeforeEach(func() {
								support.SequenceReturns(1) // this causes the revalidation
							})

							It("should create config block upon correct revalidation", func() {
								support.ProcessConfigMsgReturns(configEnv, 1, nil) // nil implies correct revalidation

								err := chain.Configure(configEnv, configSeq)
								Expect(err).NotTo(HaveOccurred())
								Eventually(support.WriteConfigBlockCallCount, LongEventualTimeout).Should(Equal(1))
							})

							It("should not create config block upon incorrect revalidation", func() {
								support.ProcessConfigMsgReturns(configEnv, 1, errors.Errorf("Invalid config envelope at changed config sequence"))

								err := chain.Configure(configEnv, configSeq)
								Expect(err).NotTo(HaveOccurred())
								Consistently(support.WriteConfigBlockCallCount).Should(Equal(0)) // no call to WriteConfigBlock
							})
						})
					})

					Context("for creating a new channel", func() {

						// use to prepare the Orderer Values
						BeforeEach(func() {
							chainID := "mychannel"
							values := make(map[string]*common.ConfigValue)
							configEnv = newConfigEnv(chainID,
								common.HeaderType_CONFIG,
								newConfigUpdateEnv(chainID, values),
							)
							configSeq = 0
						}) // BeforeEach block

						It("should be able to create a channel", func() {
							err := chain.Configure(configEnv, configSeq)
							Expect(err).NotTo(HaveOccurred())
							Eventually(support.WriteConfigBlockCallCount, LongEventualTimeout).Should(Equal(1))
						})
					})
				}) // Context block for type A config

				Context("when a type B config update comes", func() {
					Context("updating protocol values", func() {
						// use to prepare the Orderer Values
						BeforeEach(func() {
							values := map[string]*common.ConfigValue{
								"ConsensusType": {
									Version: 1,
									Value: marshalOrPanic(&orderer.ConsensusType{
										Metadata: marshalOrPanic(consenterMetadata),
									}),
								},
							}
							configEnv = newConfigEnv(channelID,
								common.HeaderType_CONFIG,
								newConfigUpdateEnv(channelID, values))
							configSeq = 0

						}) // BeforeEach block

						It("should be able to process config update of type B", func() {
							err := chain.Configure(configEnv, configSeq)
							Expect(err).NotTo(HaveOccurred())
						})
					})

					Context("updating consenters set by more than one node", func() {
						// use to prepare the Orderer Values
						BeforeEach(func() {
							values := map[string]*common.ConfigValue{
								"ConsensusType": {
									Version: 1,
									Value: marshalOrPanic(&orderer.ConsensusType{
										Metadata: marshalOrPanic(createMetadata(3, tlsCA)),
									}),
								},
							}
							configEnv = newConfigEnv(channelID,
								common.HeaderType_CONFIG,
								newConfigUpdateEnv(channelID, values))
							configSeq = 0

						}) // BeforeEach block

						It("should fail, since consenters set change is not supported", func() {
							err := chain.Configure(configEnv, configSeq)
							Expect(err).To(MatchError("update of more than one consenters at a time is not supported"))
							Expect(fakeFields.fakeProposalFailures.AddCallCount()).To(Equal(1))
							Expect(fakeFields.fakeProposalFailures.AddArgsForCall(0)).To(Equal(float64(1)))
						})
					})

					Context("updating consenters set by exactly one node", func() {
						It("should be able to process config update adding single node", func() {
							metadata := proto.Clone(consenterMetadata).(*raftprotos.Metadata)
							metadata.Consenters = append(metadata.Consenters, &raftprotos.Consenter{
								Host:          "localhost",
								Port:          7050,
								ServerTlsCert: serverTLSCert(tlsCA),
								ClientTlsCert: clientTLSCert(tlsCA),
							})

							values := map[string]*common.ConfigValue{
								"ConsensusType": {
									Version: 1,
									Value: marshalOrPanic(&orderer.ConsensusType{
										Metadata: marshalOrPanic(metadata),
									}),
								},
							}
							configEnv = newConfigEnv(channelID,
								common.HeaderType_CONFIG,
								newConfigUpdateEnv(channelID, values))
							configSeq = 0

							err := chain.Configure(configEnv, configSeq)
							Expect(err).NotTo(HaveOccurred())
						})

						It("should be able to process config update removing single node", func() {
							metadata := proto.Clone(consenterMetadata).(*raftprotos.Metadata)
							// Remove one of the consenters
							metadata.Consenters = metadata.Consenters[1:]
							values := map[string]*common.ConfigValue{
								"ConsensusType": {
									Version: 1,
									Value: marshalOrPanic(&orderer.ConsensusType{
										Metadata: marshalOrPanic(metadata),
									}),
								},
							}
							configEnv = newConfigEnv(channelID,
								common.HeaderType_CONFIG,
								newConfigUpdateEnv(channelID, values))
							configSeq = 0

							err := chain.Configure(configEnv, configSeq)
							Expect(err).NotTo(HaveOccurred())
						})

						It("fail since not allowed to add and remove node at same change", func() {
							metadata := proto.Clone(consenterMetadata).(*raftprotos.Metadata)
							// Remove one of the consenters
							metadata.Consenters = metadata.Consenters[1:]
							metadata.Consenters = append(metadata.Consenters, &raftprotos.Consenter{
								Host:          "localhost",
								Port:          7050,
								ServerTlsCert: serverTLSCert(tlsCA),
								ClientTlsCert: clientTLSCert(tlsCA),
							})
							values := map[string]*common.ConfigValue{
								"ConsensusType": {
									Version: 1,
									Value: marshalOrPanic(&orderer.ConsensusType{
										Metadata: marshalOrPanic(metadata),
									}),
								},
							}
							configEnv = newConfigEnv(channelID,
								common.HeaderType_CONFIG,
								newConfigUpdateEnv(channelID, values))
							configSeq = 0

							err := chain.Configure(configEnv, configSeq)
							Expect(err).To(MatchError("update of more than one consenters at a time is not supported"))
							Expect(fakeFields.fakeProposalFailures.AddCallCount()).To(Equal(1))
							Expect(fakeFields.fakeProposalFailures.AddArgsForCall(0)).To(Equal(float64(1)))
						})
					})
				})
			})

			Describe("Crash Fault Tolerance", func() {
				var (
					raftMetadata *raftprotos.RaftMetadata
				)

				BeforeEach(func() {
					tlsCA, _ := tlsgen.NewCA()

					raftMetadata = &raftprotos.RaftMetadata{
						Consenters: map[uint64]*raftprotos.Consenter{
							1: {
								Host:          "localhost",
								Port:          7051,
								ClientTlsCert: clientTLSCert(tlsCA),
								ServerTlsCert: serverTLSCert(tlsCA),
							},
						},
						NextConsenterId: 2,
					}
				})

				Describe("when a chain is started with existing WAL", func() {
					var (
						m1 *raftprotos.RaftMetadata
						m2 *raftprotos.RaftMetadata
					)
					JustBeforeEach(func() {
						// to generate WAL data, we start a chain,
						// order several envelopes and then halt the chain.
						close(cutter.Block)
						cutter.CutNext = true

						// enque some data to be persisted on disk by raft
						err := chain.Order(env, uint64(0))
						Expect(err).NotTo(HaveOccurred())
						Eventually(support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(1))

						_, metadata := support.WriteBlockArgsForCall(0)
						m1 = &raftprotos.RaftMetadata{}
						proto.Unmarshal(metadata, m1)

						err = chain.Order(env, uint64(0))
						Expect(err).NotTo(HaveOccurred())
						Eventually(support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(2))

						_, metadata = support.WriteBlockArgsForCall(1)
						m2 = &raftprotos.RaftMetadata{}
						proto.Unmarshal(metadata, m2)

						chain.Halt()
					})

					It("replays blocks from committed entries", func() {
						c := newChain(10*time.Second, channelID, dataDir, 1, raftMetadata)
						c.init()
						c.Start()
						defer c.Halt()

						Eventually(c.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(2))

						_, metadata := c.support.WriteBlockArgsForCall(0)
						m := &raftprotos.RaftMetadata{}
						proto.Unmarshal(metadata, m)
						Expect(m.RaftIndex).To(Equal(m1.RaftIndex))

						_, metadata = c.support.WriteBlockArgsForCall(1)
						m = &raftprotos.RaftMetadata{}
						proto.Unmarshal(metadata, m)
						Expect(m.RaftIndex).To(Equal(m2.RaftIndex))

						// chain should keep functioning
						campaign(c.clock, c.observe)

						c.cutter.CutNext = true

						err := c.Order(env, uint64(0))
						Expect(err).NotTo(HaveOccurred())
						Eventually(c.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(3))

					})

					It("only replays blocks after Applied index", func() {
						raftMetadata.RaftIndex = m1.RaftIndex
						c := newChain(10*time.Second, channelID, dataDir, 1, raftMetadata)
						c.init()
						c.Start()
						defer c.Halt()

						Eventually(c.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(1))

						_, metadata := c.support.WriteBlockArgsForCall(0)
						m := &raftprotos.RaftMetadata{}
						proto.Unmarshal(metadata, m)
						Expect(m.RaftIndex).To(Equal(m2.RaftIndex))

						// chain should keep functioning
						campaign(c.clock, c.observe)

						c.cutter.CutNext = true

						err := c.Order(env, uint64(0))
						Expect(err).NotTo(HaveOccurred())
						Eventually(c.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(2))
					})

					It("does not replay any block if already in sync", func() {
						raftMetadata.RaftIndex = m2.RaftIndex
						c := newChain(10*time.Second, channelID, dataDir, 1, raftMetadata)
						c.init()
						c.Start()
						defer c.Halt()

						Consistently(c.support.WriteBlockCallCount).Should(Equal(0))

						// chain should keep functioning
						campaign(c.clock, c.observe)

						c.cutter.CutNext = true

						err := c.Order(env, uint64(0))
						Expect(err).NotTo(HaveOccurred())
						Eventually(c.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(1))
					})

					Context("WAL file is not readable", func() {
						It("fails to load wal", func() {
							skipIfRoot()

							files, err := ioutil.ReadDir(walDir)
							Expect(err).NotTo(HaveOccurred())
							for _, f := range files {
								os.Chmod(path.Join(walDir, f.Name()), 0300)
							}

							c, err := etcdraft.NewChain(support, opts, configurator, nil, nil, observeC)
							Expect(c).To(BeNil())
							Expect(err).To(MatchError(ContainSubstring("failed to open existing WAL")))
						})
					})
				})

				Describe("when snapshotting is enabled (snapshot interval is not zero)", func() {
					var (
						ledgerLock sync.Mutex
						ledger     map[uint64]*common.Block
					)

					countFiles := func() int {
						files, err := ioutil.ReadDir(snapDir)
						Expect(err).NotTo(HaveOccurred())
						return len(files)
					}

					BeforeEach(func() {
						opts.SnapshotCatchUpEntries = 2

						close(cutter.Block)
						cutter.CutNext = true

						ledgerLock.Lock()
						ledger = map[uint64]*common.Block{
							0: getSeedBlock(), // genesis block
						}
						ledgerLock.Unlock()

						support.WriteBlockStub = func(b *common.Block, meta []byte) {
							bytes, err := proto.Marshal(&common.Metadata{Value: meta})
							Expect(err).NotTo(HaveOccurred())
							b.Metadata.Metadata[common.BlockMetadataIndex_ORDERER] = bytes

							ledgerLock.Lock()
							defer ledgerLock.Unlock()
							ledger[b.Header.Number] = b
						}

						support.HeightStub = func() uint64 {
							ledgerLock.Lock()
							defer ledgerLock.Unlock()
							return uint64(len(ledger))
						}
					})

					Context("Small SnapshotInterval", func() {
						BeforeEach(func() {
							opts.SnapInterval = 1
						})

						It("writes snapshot file to snapDir", func() {
							// Scenario: start a chain with SnapInterval = 1 byte, expect it to take
							// one snapshot for each block

							i, _ := opts.MemoryStorage.FirstIndex()

							Expect(chain.Order(env, uint64(0))).To(Succeed())
							Eventually(support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(1))
							Eventually(countFiles, LongEventualTimeout).Should(Equal(1))
							Eventually(opts.MemoryStorage.FirstIndex, LongEventualTimeout).Should(BeNumerically(">", i))
							Expect(fakeFields.fakeSnapshotBlockNumber.SetCallCount()).To(Equal(1))
							s, _ := opts.MemoryStorage.Snapshot()
							b := utils.UnmarshalBlockOrPanic(s.Data)
							Expect(fakeFields.fakeSnapshotBlockNumber.SetArgsForCall(0)).To(Equal(float64(b.Header.Number)))

							i, _ = opts.MemoryStorage.FirstIndex()

							Expect(chain.Order(env, uint64(0))).To(Succeed())
							Eventually(support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(2))

							Eventually(countFiles, LongEventualTimeout).Should(Equal(2))
							Eventually(opts.MemoryStorage.FirstIndex, LongEventualTimeout).Should(BeNumerically(">", i))
							Expect(fakeFields.fakeSnapshotBlockNumber.SetCallCount()).To(Equal(2))
							s, _ = opts.MemoryStorage.Snapshot()
							b = utils.UnmarshalBlockOrPanic(s.Data)
							Expect(fakeFields.fakeSnapshotBlockNumber.SetArgsForCall(1)).To(Equal(float64(b.Header.Number)))
						})

						It("pauses chain if sync is in progress", func() {
							// Scenario:
							// after a snapshot is taken, reboot chain with raftIndex = 0
							// chain should attempt to sync upon reboot, and blocks on
							// `WaitReady` API

							i, _ := opts.MemoryStorage.FirstIndex()

							Expect(chain.Order(env, uint64(0))).To(Succeed())
							Eventually(support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(1))
							Eventually(countFiles, LongEventualTimeout).Should(Equal(1))
							Eventually(opts.MemoryStorage.FirstIndex, LongEventualTimeout).Should(BeNumerically(">", i))

							i, _ = opts.MemoryStorage.FirstIndex()

							Expect(chain.Order(env, uint64(0))).To(Succeed())
							Eventually(support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(2))
							Eventually(countFiles, LongEventualTimeout).Should(Equal(2))
							Eventually(opts.MemoryStorage.FirstIndex, LongEventualTimeout).Should(BeNumerically(">", i))

							chain.Halt()

							c := newChain(10*time.Second, channelID, dataDir, 1, raftMetadata)
							c.init()

							signal := make(chan struct{})

							c.puller.PullBlockStub = func(i uint64) *common.Block {
								<-signal // blocking for assertions
								ledgerLock.Lock()
								defer ledgerLock.Unlock()
								if i >= uint64(len(ledger)) {
									return nil
								}

								return ledger[i]
							}

							err := c.WaitReady()
							Expect(err).To(MatchError("chain is not started"))

							c.Start()
							defer c.Halt()

							// pull block is called, so chain should be catching up now, WaitReady should block
							signal <- struct{}{}

							done := make(chan error)
							go func() {
								done <- c.WaitReady()
							}()

							Consistently(done).ShouldNot(Receive())
							close(signal)                         // unblock block puller
							Eventually(done).Should(Receive(nil)) // WaitReady should be unblocked
							Eventually(c.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(2))
						})

						It("restores snapshot w/o extra entries", func() {
							// Scenario:
							// after a snapshot is taken, no more entries are appended.
							// then node is restarted, it loads snapshot, finds its term
							// and index. While replaying WAL to memory storage, it should
							// not append any entry because no extra entry was appended
							// after snapshot was taken.

							Expect(chain.Order(env, uint64(0))).To(Succeed())
							Eventually(support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(1))
							_, metadata := support.WriteBlockArgsForCall(0)
							m := &raftprotos.RaftMetadata{}
							proto.Unmarshal(metadata, m)

							Eventually(countFiles, LongEventualTimeout).Should(Equal(1))
							Eventually(opts.MemoryStorage.FirstIndex, LongEventualTimeout).Should(BeNumerically(">", 1))
							snapshot, err := opts.MemoryStorage.Snapshot() // get the snapshot just created
							Expect(err).NotTo(HaveOccurred())
							i, err := opts.MemoryStorage.FirstIndex() // get the first index in memory
							Expect(err).NotTo(HaveOccurred())

							// expect storage to preserve SnapshotCatchUpEntries entries before snapshot
							Expect(i).To(Equal(snapshot.Metadata.Index - opts.SnapshotCatchUpEntries + 1))

							chain.Halt()

							raftMetadata.RaftIndex = m.RaftIndex
							c := newChain(10*time.Second, channelID, dataDir, 1, raftMetadata)

							c.init()
							c.Start()
							defer c.Halt()

							// following arithmetic reflects how etcdraft MemoryStorage is implemented
							// when no entry is appended after snapshot being loaded.
							Eventually(c.opts.MemoryStorage.FirstIndex, LongEventualTimeout).Should(Equal(snapshot.Metadata.Index + 1))
							Eventually(c.opts.MemoryStorage.LastIndex, LongEventualTimeout).Should(Equal(snapshot.Metadata.Index))

							// chain keeps functioning
							Eventually(func() bool {
								c.clock.Increment(interval)
								select {
								case <-c.observe:
									return true
								default:
									return false
								}
							}, LongEventualTimeout).Should(BeTrue())

							c.cutter.CutNext = true
							err = c.Order(env, uint64(0))
							Expect(err).NotTo(HaveOccurred())
							Eventually(c.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(1))
						})
					})

					Context("Large SnapshotInterval", func() {
						BeforeEach(func() {
							opts.SnapInterval = 1024
						})

						It("restores snapshot w/ extra entries", func() {
							// Scenario:
							// after a snapshot is taken, more entries are appended.
							// then node is restarted, it loads snapshot, finds its term
							// and index. While replaying WAL to memory storage, it should
							// append some entries.

							largeEnv := &common.Envelope{
								Payload: marshalOrPanic(&common.Payload{
									Header: &common.Header{ChannelHeader: marshalOrPanic(&common.ChannelHeader{Type: int32(common.HeaderType_MESSAGE), ChannelId: channelID})},
									Data:   make([]byte, 500),
								}),
							}

							By("Ordering two large envelopes to trigger snapshot")
							Expect(chain.Order(largeEnv, uint64(0))).To(Succeed())
							Eventually(support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(1))

							Expect(chain.Order(largeEnv, uint64(0))).To(Succeed())
							Eventually(support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(2))

							_, metadata := support.WriteBlockArgsForCall(1)
							m := &raftprotos.RaftMetadata{}
							proto.Unmarshal(metadata, m)

							// check snapshot does exit
							Eventually(countFiles, LongEventualTimeout).Should(Equal(1))
							Eventually(opts.MemoryStorage.FirstIndex, LongEventualTimeout).Should(BeNumerically(">", 1))
							snapshot, err := opts.MemoryStorage.Snapshot() // get the snapshot just created
							Expect(err).NotTo(HaveOccurred())
							i, err := opts.MemoryStorage.FirstIndex() // get the first index in memory
							Expect(err).NotTo(HaveOccurred())

							// expect storage to preserve SnapshotCatchUpEntries entries before snapshot
							Expect(i).To(Equal(snapshot.Metadata.Index - opts.SnapshotCatchUpEntries + 1))

							By("Ordering another envlope to append new data to memory after snaphost")
							Expect(chain.Order(env, uint64(0))).To(Succeed())
							Eventually(support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(3))

							lasti, _ := opts.MemoryStorage.LastIndex()

							chain.Halt()

							raftMetadata.RaftIndex = m.RaftIndex
							c := newChain(10*time.Second, channelID, dataDir, 1, raftMetadata)
							c.support.HeightReturns(5)
							c.support.BlockReturns(&common.Block{
								Header:   &common.BlockHeader{},
								Data:     &common.BlockData{Data: [][]byte{[]byte("foo")}},
								Metadata: &common.BlockMetadata{Metadata: make([][]byte, 4)},
							})

							By("Restarting the node")
							c.init()
							c.Start()
							defer c.Halt()

							By("Checking latest index is larger than index in snapshot")
							Eventually(c.opts.MemoryStorage.FirstIndex, LongEventualTimeout).Should(Equal(snapshot.Metadata.Index + 1))
							Eventually(c.opts.MemoryStorage.LastIndex, LongEventualTimeout).Should(Equal(lasti))
						})

						When("local ledger is in sync with snapshot", func() {
							It("does not pull blocks and still respects snapshot interval", func() {
								// Scenario:
								// - snapshot is taken at block 2
								// - order one more envelope (block 3)
								// - reboot chain at block 2
								// - block 3 should be replayed from wal
								// - order another envelope to trigger snapshot, containing block 3 & 4
								// Assertions:
								// - block puller should NOT be called
								// - chain should keep functioning after reboot
								// - chain should respect snapshot interval to trigger next snapshot

								largeEnv := &common.Envelope{
									Payload: marshalOrPanic(&common.Payload{
										Header: &common.Header{ChannelHeader: marshalOrPanic(&common.ChannelHeader{Type: int32(common.HeaderType_MESSAGE), ChannelId: channelID})},
										Data:   make([]byte, 500),
									}),
								}

								By("Ordering two large envelopes to trigger snapshot")
								Expect(chain.Order(largeEnv, uint64(0))).To(Succeed())
								Eventually(support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(1))

								Expect(chain.Order(largeEnv, uint64(0))).To(Succeed())
								Eventually(support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(2))

								Eventually(countFiles, LongEventualTimeout).Should(Equal(1))

								_, metadata := support.WriteBlockArgsForCall(1)
								m := &raftprotos.RaftMetadata{}
								proto.Unmarshal(metadata, m)

								By("Cutting block 3")
								// order another envelope. this should not trigger snapshot
								err = chain.Order(largeEnv, uint64(0))
								Expect(err).NotTo(HaveOccurred())
								Eventually(support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(3))

								chain.Halt()

								raftMetadata.RaftIndex = m.RaftIndex
								c := newChain(10*time.Second, channelID, dataDir, 1, raftMetadata)
								// start chain at block 2 (height = 3)

								c.support.HeightReturns(3)
								c.support.BlockReturns(&common.Block{
									Header:   &common.BlockHeader{},
									Data:     &common.BlockData{Data: [][]byte{[]byte("foo")}},
									Metadata: &common.BlockMetadata{Metadata: make([][]byte, 4)},
								})
								c.opts.SnapInterval = 1024

								By("Restarting node at block 2")
								c.init()
								c.Start()
								defer c.Halt()

								// elect leader
								Eventually(func() bool {
									c.clock.Increment(interval)
									select {
									case <-c.observe:
										return true
									default:
										return false
									}
								}, LongEventualTimeout).Should(BeTrue())

								By("Ordering one more block to trigger snapshot")
								c.cutter.CutNext = true
								err = c.Order(largeEnv, uint64(0))
								Expect(err).NotTo(HaveOccurred())

								Eventually(c.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(2))
								Expect(c.puller.PullBlockCallCount()).Should(BeZero())
								// old snapshot file is retained
								Eventually(countFiles, LongEventualTimeout).Should(Equal(2))
							})
						})
					})
				})
			})

			Context("Invalid WAL dir", func() {
				var support = &consensusmocks.FakeConsenterSupport{}
				BeforeEach(func() {
					// for block creator initialization
					support.HeightReturns(1)
					support.BlockReturns(getSeedBlock())
				})

				When("WAL dir is a file", func() {
					It("replaces file with fresh WAL dir", func() {
						f, err := ioutil.TempFile("", "wal-")
						Expect(err).NotTo(HaveOccurred())
						defer os.RemoveAll(f.Name())

						chain, err := etcdraft.NewChain(
							support,
							etcdraft.Options{
								WALDir:        f.Name(),
								SnapDir:       snapDir,
								Logger:        logger,
								MemoryStorage: storage,
								RaftMetadata:  &raftprotos.RaftMetadata{},
								Metrics:       newFakeMetrics(newFakeMetricsFields()),
							},
							configurator,
							nil,
							nil,
							observeC)
						Expect(chain).NotTo(BeNil())
						Expect(err).NotTo(HaveOccurred())

						info, err := os.Stat(f.Name())
						Expect(err).NotTo(HaveOccurred())
						Expect(info.IsDir()).To(BeTrue())
					})
				})

				When("WAL dir is not writeable", func() {
					It("replace it with fresh WAL dir", func() {
						d, err := ioutil.TempDir("", "wal-")
						Expect(err).NotTo(HaveOccurred())
						defer os.RemoveAll(d)

						err = os.Chmod(d, 0500)
						Expect(err).NotTo(HaveOccurred())

						chain, err := etcdraft.NewChain(
							support,
							etcdraft.Options{
								WALDir:        d,
								SnapDir:       snapDir,
								Logger:        logger,
								MemoryStorage: storage,
								RaftMetadata:  &raftprotos.RaftMetadata{},
								Metrics:       newFakeMetrics(newFakeMetricsFields()),
							},
							nil,
							nil,
							nil,
							nil)
						Expect(chain).NotTo(BeNil())
						Expect(err).ToNot(HaveOccurred())
					})
				})

				When("WAL parent dir is not writeable", func() {
					It("fails to bootstrap fresh raft node", func() {
						skipIfRoot()

						d, err := ioutil.TempDir("", "wal-")
						Expect(err).NotTo(HaveOccurred())
						defer os.RemoveAll(d)

						err = os.Chmod(d, 0500)
						Expect(err).NotTo(HaveOccurred())

						chain, err := etcdraft.NewChain(
							support,
							etcdraft.Options{
								WALDir:       path.Join(d, "wal-dir"),
								SnapDir:      snapDir,
								Logger:       logger,
								RaftMetadata: &raftprotos.RaftMetadata{},
							},
							nil,
							nil,
							nil,
							nil)
						Expect(chain).To(BeNil())
						Expect(err).To(MatchError(ContainSubstring("failed to initialize WAL: mkdir")))
					})
				})
			})
		})

	})

	Describe("Multiple Raft nodes", func() {
		var (
			network      *network
			channelID    string
			timeout      time.Duration
			dataDir      string
			c1, c2, c3   *chain
			raftMetadata *raftprotos.RaftMetadata
		)

		BeforeEach(func() {
			var err error

			channelID = "multi-node-channel"
			timeout = 10 * time.Second

			dataDir, err = ioutil.TempDir("", "raft-test-")
			Expect(err).NotTo(HaveOccurred())

			raftMetadata = &raftprotos.RaftMetadata{
				Consenters: map[uint64]*raftprotos.Consenter{
					1: {
						Host:          "localhost",
						Port:          7051,
						ClientTlsCert: clientTLSCert(tlsCA),
						ServerTlsCert: serverTLSCert(tlsCA),
					},
					2: {
						Host:          "localhost",
						Port:          7051,
						ClientTlsCert: clientTLSCert(tlsCA),
						ServerTlsCert: serverTLSCert(tlsCA),
					},
					3: {
						Host:          "localhost",
						Port:          7051,
						ClientTlsCert: clientTLSCert(tlsCA),
						ServerTlsCert: serverTLSCert(tlsCA),
					},
				},
				NextConsenterId: 4,
			}

			network = createNetwork(timeout, channelID, dataDir, raftMetadata)
			c1 = network.chains[1]
			c2 = network.chains[2]
			c3 = network.chains[3]
		})

		AfterEach(func() {
			os.RemoveAll(dataDir)
		})

		When("2/3 nodes are running", func() {
			It("late node can catch up", func() {
				network.init()
				network.start(1, 2)
				network.elect(1)

				c1.cutter.CutNext = true
				err := c1.Order(env, 0)
				Expect(err).ToNot(HaveOccurred())

				Eventually(func() int { return c1.support.WriteBlockCallCount() }, LongEventualTimeout).Should(Equal(1))
				Eventually(func() int { return c2.support.WriteBlockCallCount() }, LongEventualTimeout).Should(Equal(1))
				Eventually(func() int { return c3.support.WriteBlockCallCount() }, LongEventualTimeout).Should(Equal(0))

				network.start(3)

				c1.clock.Increment(interval)
				Eventually(func() int { return c3.support.WriteBlockCallCount() }, LongEventualTimeout).Should(Equal(1))

				network.stop()
			})

			It("late node receives snapshot from leader", func() {
				c1.opts.SnapInterval = 1
				c1.opts.SnapshotCatchUpEntries = 1

				c1.cutter.CutNext = true

				var blocksLock sync.Mutex
				blocks := make(map[uint64]*common.Block) // storing written blocks for block puller

				c1.support.WriteBlockStub = func(b *common.Block, meta []byte) {
					blocksLock.Lock()
					defer blocksLock.Unlock()
					bytes, err := proto.Marshal(&common.Metadata{Value: meta})
					Expect(err).NotTo(HaveOccurred())
					b.Metadata.Metadata[common.BlockMetadataIndex_ORDERER] = bytes
					blocks[b.Header.Number] = b
				}

				c3.puller.PullBlockStub = func(i uint64) *common.Block {
					blocksLock.Lock()
					defer blocksLock.Unlock()
					b, exist := blocks[i]
					if !exist {
						return nil
					}

					return b
				}

				network.init()
				network.start(1, 2)
				network.elect(1)

				err := c1.Order(env, 0)
				Expect(err).ToNot(HaveOccurred())

				Eventually(func() int { return c1.support.WriteBlockCallCount() }, LongEventualTimeout).Should(Equal(1))
				Eventually(func() int { return c2.support.WriteBlockCallCount() }, LongEventualTimeout).Should(Equal(1))
				Eventually(func() int { return c3.support.WriteBlockCallCount() }, LongEventualTimeout).Should(Equal(0))

				err = c1.Order(env, 0)
				Expect(err).ToNot(HaveOccurred())

				Eventually(func() int { return c1.support.WriteBlockCallCount() }, LongEventualTimeout).Should(Equal(2))
				Eventually(func() int { return c2.support.WriteBlockCallCount() }, LongEventualTimeout).Should(Equal(2))
				Eventually(func() int { return c3.support.WriteBlockCallCount() }, LongEventualTimeout).Should(Equal(0))

				network.start(3)

				c1.clock.Increment(interval)
				Eventually(func() int { return c3.support.WriteBlockCallCount() }, LongEventualTimeout).Should(Equal(2))

				network.stop()
			})
		})

		When("reconfiguring raft cluster", func() {
			const (
				defaultTimeout = 5 * time.Second
			)
			var (
				addConsenterConfigValue = func() map[string]*common.ConfigValue {
					metadata := &raftprotos.Metadata{}
					for _, consenter := range raftMetadata.Consenters {
						metadata.Consenters = append(metadata.Consenters, consenter)
					}

					newConsenter := &raftprotos.Consenter{
						Host:          "localhost",
						Port:          7050,
						ServerTlsCert: serverTLSCert(tlsCA),
						ClientTlsCert: clientTLSCert(tlsCA),
					}
					metadata.Consenters = append(metadata.Consenters, newConsenter)

					return map[string]*common.ConfigValue{
						"ConsensusType": {
							Version: 1,
							Value: marshalOrPanic(&orderer.ConsensusType{
								Metadata: marshalOrPanic(metadata),
							}),
						},
					}
				}
				removeConsenterConfigValue = func(id uint64) map[string]*common.ConfigValue {
					newRaftMetadata := proto.Clone(raftMetadata).(*raftprotos.RaftMetadata)
					delete(newRaftMetadata.Consenters, id)

					metadata := &raftprotos.Metadata{}
					for _, consenter := range newRaftMetadata.Consenters {
						metadata.Consenters = append(metadata.Consenters, consenter)
					}

					return map[string]*common.ConfigValue{
						"ConsensusType": {
							Version: 1,
							Value: marshalOrPanic(&orderer.ConsensusType{
								Metadata: marshalOrPanic(metadata),
							}),
						},
					}
				}
			)

			BeforeEach(func() {
				network.init()
				network.start()
				network.elect(1)

				By("Submitting first tx to cut the block")
				c1.cutter.CutNext = true
				err := c1.Order(env, 0)
				Expect(err).ToNot(HaveOccurred())

				c1.clock.Increment(interval)

				network.exec(
					func(c *chain) {
						Eventually(c.support.WriteBlockCallCount, defaultTimeout).Should(Equal(1))
					})
			})

			AfterEach(func() {
				network.stop()
			})

			Context("reconfiguration", func() {
				It("trying to simultaneously add and remove nodes in one config update", func() {

					updatedRaftMetadata := proto.Clone(raftMetadata).(*raftprotos.RaftMetadata)
					// remove second consenter
					delete(updatedRaftMetadata.Consenters, 2)

					metadata := &raftprotos.Metadata{}
					for _, consenter := range updatedRaftMetadata.Consenters {
						metadata.Consenters = append(metadata.Consenters, consenter)
					}

					// add new consenter
					newConsenter := &raftprotos.Consenter{
						Host:          "localhost",
						Port:          7050,
						ServerTlsCert: serverTLSCert(tlsCA),
						ClientTlsCert: clientTLSCert(tlsCA),
					}
					metadata.Consenters = append(metadata.Consenters, newConsenter)

					value := map[string]*common.ConfigValue{
						"ConsensusType": {
							Version: 1,
							Value: marshalOrPanic(&orderer.ConsensusType{
								Metadata: marshalOrPanic(metadata),
							}),
						},
					}

					By("creating new configuration with removed node and new one")
					configEnv := newConfigEnv(channelID, common.HeaderType_CONFIG, newConfigUpdateEnv(channelID, value))
					c1.cutter.CutNext = true

					By("sending config transaction")
					err := c1.Configure(configEnv, 0)
					Expect(err).To(MatchError("update of more than one consenters at a time is not supported"))
				})

				It("adding node to the cluster", func() {
					configEnv := newConfigEnv(channelID, common.HeaderType_CONFIG, newConfigUpdateEnv(channelID, addConsenterConfigValue()))
					c1.cutter.CutNext = true

					By("sending config transaction")
					err := c1.Configure(configEnv, 0)
					Expect(err).ToNot(HaveOccurred())

					network.exec(func(c *chain) {
						Eventually(c.support.WriteConfigBlockCallCount, defaultTimeout).Should(Equal(1))
						Eventually(c.fakeFields.fakeClusterSize.SetCallCount, LongEventualTimeout).Should(Equal(2))
						Expect(c.fakeFields.fakeClusterSize.SetArgsForCall(1)).To(Equal(float64(4)))
					})

					_, raftmetabytes := c1.support.WriteConfigBlockArgsForCall(0)
					meta := &common.Metadata{Value: raftmetabytes}
					raftmeta, err := etcdraft.ReadRaftMetadata(meta, nil)
					Expect(err).NotTo(HaveOccurred())

					c4 := newChain(timeout, channelID, dataDir, 4, raftmeta)
					c4.init()

					// if we join a node to existing network, it MUST already obtained blocks
					// till the config block that adds this node to cluster.
					c4.support.WriteBlock(c1.support.WriteBlockArgsForCall(0))
					c4.support.WriteConfigBlock(c1.support.WriteConfigBlockArgsForCall(0))

					network.addChain(c4)
					c4.Start()

					// ConfChange is applied to etcd/raft asynchronously, meaning node 4 is not added
					// to leader's node list right away. An immediate tick does not trigger a heartbeat
					// being sent to node 4. Therefore, we repeatedly tick the leader until node 4 joins
					// the cluster successfully.
					Eventually(func() <-chan raft.SoftState {
						c1.clock.Increment(interval)
						return c4.observe
					}, defaultTimeout).Should(Receive(Equal(raft.SoftState{Lead: 1, RaftState: raft.StateFollower})))

					Eventually(c4.support.WriteBlockCallCount, defaultTimeout).Should(Equal(1))
					Eventually(c4.support.WriteConfigBlockCallCount, defaultTimeout).Should(Equal(1))

					By("submitting new transaction to follower")
					c1.cutter.CutNext = true
					err = c4.Order(env, 0)
					Expect(err).ToNot(HaveOccurred())

					network.exec(func(c *chain) {
						Eventually(c.support.WriteBlockCallCount, defaultTimeout).Should(Equal(2))
					})
				})

				It("adding node to the cluster of 2/3 available nodes", func() {
					// Scenario: disconnect one of existing nodes from the replica set
					// add new node, reconnect the old one and choose newly added as a
					// leader, check whenever disconnected node will get new configuration
					// and will see newly added as a leader

					// disconnect second node
					network.disconnect(2)

					configEnv := newConfigEnv(channelID, common.HeaderType_CONFIG, newConfigUpdateEnv(channelID, addConsenterConfigValue()))
					c1.cutter.CutNext = true

					By("sending config transaction")
					err := c1.Configure(configEnv, 0)
					Expect(err).ToNot(HaveOccurred())

					Eventually(c1.support.WriteConfigBlockCallCount, defaultTimeout).Should(Equal(1))
					// second node is disconnected hence should not be able to get the config block
					Eventually(c2.support.WriteConfigBlockCallCount, defaultTimeout).Should(Equal(0))
					Eventually(c3.support.WriteConfigBlockCallCount, defaultTimeout).Should(Equal(1))

					_, raftmetabytes := c1.support.WriteConfigBlockArgsForCall(0)
					meta := &common.Metadata{Value: raftmetabytes}
					raftmeta, err := etcdraft.ReadRaftMetadata(meta, nil)
					Expect(err).NotTo(HaveOccurred())

					c4 := newChain(timeout, channelID, dataDir, 4, raftmeta)
					c4.init()

					// if we join a node to existing network, it MUST already obtained blocks
					// till the config block that adds this node to cluster.
					c4.support.WriteBlock(c1.support.WriteBlockArgsForCall(0))
					c4.support.WriteConfigBlock(c1.support.WriteConfigBlockArgsForCall(0))

					network.addChain(c4)
					c4.start()
					Expect(c4.WaitReady()).To(Succeed())
					network.join(4, true)

					Eventually(c4.support.WriteBlockCallCount, defaultTimeout).Should(Equal(1))
					Eventually(c4.support.WriteConfigBlockCallCount, defaultTimeout).Should(Equal(1))

					By("submitting new transaction to follower")
					c1.cutter.CutNext = true
					err = c4.Order(env, 0)
					Expect(err).ToNot(HaveOccurred())
					Eventually(c4.support.WriteBlockCallCount, defaultTimeout).Should(Equal(2))

					// elect newly added node to be the leader
					network.elect(4)

					// connecting node back again, should be able to catch up and get configuration update
					network.connect(2)

					// make sure second node see 4th as a leader
					Eventually(func() <-chan raft.SoftState {
						c4.clock.Increment(interval)
						return c2.observe
					}, defaultTimeout).Should(Receive(Equal(raft.SoftState{Lead: 4, RaftState: raft.StateFollower})))

					By("submitting new transaction to re-connected node")
					c4.cutter.CutNext = true
					err = c2.Order(env, 0)
					Expect(err).ToNot(HaveOccurred())

					network.exec(func(c *chain) {
						Eventually(c.support.WriteBlockCallCount, defaultTimeout).Should(Equal(3))
					})
				})

				It("stop leader and continue reconfiguration failing over to new leader", func() {
					// Scenario: Starting replica set of 3 Raft nodes, electing node c1 to be a leader
					// configure chain support mock to disconnect c1 right after it writes configuration block
					// into the ledger, this to simulate failover.
					// Next boostraping a new node c4 to join a cluster and creating config transaction, submitting
					// it to the leader. Once leader writes configuration block it fails and leadership transferred to
					// c2.
					// Test asserts that new node c4, will join the cluster and c2 will handle failover of
					// re-configuration. Later we connecting c1 back and making sure it capable of catching up with
					// new configuration and successfully rejoins replica set.

					configEnv := newConfigEnv(channelID, common.HeaderType_CONFIG, newConfigUpdateEnv(channelID, addConsenterConfigValue()))
					c1.cutter.CutNext = true

					step1 := c1.getStepFunc()
					count := c1.rpc.SendConsensusCallCount() // record current step call count
					c1.setStepFunc(func(dest uint64, msg *orderer.ConsensusRequest) error {
						// disconnect network after 4 MsgApp are sent by c1:
						// - 2 MsgApp to c2 & c3 that replicate data to raft followers
						// - 2 MsgApp to c2 & c3 that instructs followers to commit data
						if c1.rpc.SendConsensusCallCount() == count+4 {
							defer network.disconnect(1)
						}

						return step1(dest, msg)
					})

					By("sending config transaction")
					err := c1.Configure(configEnv, 0)
					Expect(err).ToNot(HaveOccurred())

					// every node has written config block to the OSN ledger
					network.exec(
						func(c *chain) {
							Eventually(c.support.WriteConfigBlockCallCount, LongEventualTimeout).Should(Equal(1))
						})
					c1.setStepFunc(step1)
					network.elect(2)

					_, raftmetabytes := c1.support.WriteConfigBlockArgsForCall(0)
					meta := &common.Metadata{Value: raftmetabytes}
					raftmeta, err := etcdraft.ReadRaftMetadata(meta, nil)
					Expect(err).NotTo(HaveOccurred())

					c4 := newChain(timeout, channelID, dataDir, 4, raftmeta)
					c4.init()

					// if we join a node to existing network, it MUST already obtained blocks
					// till the config block that adds this node to cluster.
					c4.support.WriteBlock(c1.support.WriteBlockArgsForCall(0))
					c4.support.WriteConfigBlock(c1.support.WriteConfigBlockArgsForCall(0))

					network.addChain(c4)
					c4.start()
					Expect(c4.WaitReady()).To(Succeed())
					network.join(4, true)

					Eventually(c4.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(1))
					Eventually(c4.support.WriteConfigBlockCallCount, LongEventualTimeout).Should(Equal(1))

					By("submitting new transaction to follower")
					c2.cutter.CutNext = true
					err = c4.Order(env, 0)
					Expect(err).ToNot(HaveOccurred())

					// rest nodes are alive include a newly added, hence should write 2 blocks
					Eventually(c2.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(2))
					Eventually(c3.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(2))
					Eventually(c4.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(2))

					// node 1 has been stopped should not write any block
					Consistently(c1.support.WriteBlockCallCount).Should(Equal(1))

					network.connect(1)

					c2.clock.Increment(interval)
					// check that former leader didn't get stuck and actually got resign signal,
					// and once connected capable of communicating with rest of the replicas set
					Eventually(c1.observe, LongEventualTimeout).Should(Receive(Equal(raft.SoftState{Lead: 2, RaftState: raft.StateFollower})))
					Eventually(c1.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(2))
				})

				It("stop cluster quorum and continue reconfiguration after the restart", func() {
					// Scenario: Starting replica set of 3 Raft nodes, electing node c1 to be a leader
					// configure chain support mock to stop cluster after config block is committed.
					// Restart the cluster and ensure it picks up updates and capable to finish reconfiguration.

					configEnv := newConfigEnv(channelID, common.HeaderType_CONFIG, newConfigUpdateEnv(channelID, addConsenterConfigValue()))
					c1.cutter.CutNext = true

					step1 := c1.getStepFunc()
					count := c1.rpc.SendConsensusCallCount() // record current step call count
					c1.setStepFunc(func(dest uint64, msg *orderer.ConsensusRequest) error {
						// disconnect network after 4 MsgApp are sent by c1:
						// - 2 MsgApp to c2 & c3 that replicate data to raft followers
						// - 2 MsgApp to c2 & c3 that instructs followers to commit data
						if c1.rpc.SendConsensusCallCount() == count+4 {
							defer func() {
								network.disconnect(1)
								network.disconnect(2)
								network.disconnect(3)
							}()
						}

						return step1(dest, msg)
					})

					By("sending config transaction")
					err := c1.Configure(configEnv, 0)
					Expect(err).ToNot(HaveOccurred())

					// every node has written config block to the OSN ledger
					network.exec(
						func(c *chain) {
							Eventually(c.support.WriteConfigBlockCallCount, LongEventualTimeout).Should(Equal(1))
						})

					_, raftmetabytes := c1.support.WriteConfigBlockArgsForCall(0)
					meta := &common.Metadata{Value: raftmetabytes}
					raftmeta, err := etcdraft.ReadRaftMetadata(meta, nil)
					Expect(err).NotTo(HaveOccurred())

					c4 := newChain(timeout, channelID, dataDir, 4, raftmeta)
					c4.init()

					// if we join a node to existing network, it MUST already obtained blocks
					// till the config block that adds this node to cluster.
					c4.support.WriteBlock(c1.support.WriteBlockArgsForCall(0))
					c4.support.WriteConfigBlock(c1.support.WriteConfigBlockArgsForCall(0))

					network.addChain(c4)

					By("reconnecting nodes back")
					for i := uint64(1); i < 4; i++ {
						network.connect(i)
					}

					By("re-elect node 2 to be a leader")
					network.elect(2)

					c4.start()
					Expect(c4.WaitReady()).To(Succeed())
					network.join(4, false)

					Eventually(c4.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(1))
					Eventually(c4.support.WriteConfigBlockCallCount, LongEventualTimeout).Should(Equal(1))

					By("submitting new transaction to follower")
					c2.cutter.CutNext = true
					err = c4.Order(env, 0)
					Expect(err).ToNot(HaveOccurred())

					c2.clock.Increment(interval)

					// rest nodes are alive include a newly added, hence should write 2 blocks
					Eventually(c1.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(2))
					Eventually(c2.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(2))
					Eventually(c3.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(2))
					Eventually(c4.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(2))
				})

				It("ensures that despite leader failure cluster continue to process configuration to remove the leader", func() {
					// Scenario: Starting replica set of 3 nodes, electing nodeID = 1 to be the leader.
					// Prepare config update transaction which removes leader (nodeID = 1), then leader
					// fails right after it commits configuration block.

					configEnv := newConfigEnv(channelID,
						common.HeaderType_CONFIG,
						newConfigUpdateEnv(channelID, removeConsenterConfigValue(1))) // remove nodeID == 1

					c1.cutter.CutNext = true

					step1 := c1.getStepFunc()
					count := c1.rpc.SendConsensusCallCount() // record current step call count
					c1.setStepFunc(func(dest uint64, msg *orderer.ConsensusRequest) error {
						// disconnect network after 4 MsgApp are sent by c1:
						// - 2 MsgApp to c2 & c3 that replicate data to raft followers
						// - 2 MsgApp to c2 & c3 that instructs followers to commit data
						if c1.rpc.SendConsensusCallCount() == count+4 {
							defer network.disconnect(1)
						}

						return step1(dest, msg)
					})

					By("sending config transaction")
					err := c1.Configure(configEnv, 0)
					Expect(err).ToNot(HaveOccurred())

					network.exec(func(c *chain) {
						Eventually(c.support.WriteConfigBlockCallCount, LongEventualTimeout).Should(Equal(1))
					})

					// electing new leader
					network.elect(2)

					By("submitting new transaction to follower")
					c2.cutter.CutNext = true
					err = c3.Order(env, 0)
					Expect(err).ToNot(HaveOccurred())

					// rest nodes are alive include a newly added, hence should write 2 blocks
					Eventually(c2.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(2))
					Eventually(c3.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(2))
				})

				It("removes leader from replica set", func() {
					// Scenario: Starting replica set of 3 nodes, electing nodeID = 1 to be the leader.
					// Prepare config update transaction which removes leader (nodeID = 1), this to
					// ensure we handle re-configuration of node removal correctly and remaining two
					// nodes still capable to form functional quorum and Raft capable of making further progress.
					// Moreover test asserts that removed node stops Rafting with rest of the cluster, i.e.
					// should not be able to get updates or forward transactions.

					configEnv := newConfigEnv(channelID,
						common.HeaderType_CONFIG,
						newConfigUpdateEnv(channelID, removeConsenterConfigValue(1))) // remove nodeID == 1

					c1.cutter.CutNext = true

					By("sending config transaction")
					err := c1.Configure(configEnv, 0)
					Expect(err).ToNot(HaveOccurred())

					// every node has written config block to the OSN ledger
					network.exec(
						func(c *chain) {
							Eventually(c.support.WriteConfigBlockCallCount, LongEventualTimeout).Should(Equal(1))
							Eventually(c.fakeFields.fakeClusterSize.SetCallCount, LongEventualTimeout).Should(Equal(2))
							Expect(c.fakeFields.fakeClusterSize.SetArgsForCall(1)).To(Equal(float64(2)))
						})

					// Assert c1 has exited
					Eventually(c1.Errored, LongEventualTimeout).Should(BeClosed())
					close(c1.stopped)

					By("making sure remaining two nodes will elect new leader")

					// deterministically select nodeID == 2 to be a leader
					network.elect(2)

					By("submitting transaction to new leader")
					c2.cutter.CutNext = true
					err = c2.Order(env, 0)
					Expect(err).ToNot(HaveOccurred())

					Eventually(c2.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(2))
					Eventually(c3.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(2))
					// node 1 has been stopped should not write any block
					Consistently(c1.support.WriteBlockCallCount).Should(Equal(1))

					By("trying to submit to new node, expected to fail")
					c1.cutter.CutNext = true
					err = c1.Order(env, 0)
					Expect(err).To(HaveOccurred())

					// number of block writes should remain the same
					Consistently(c2.support.WriteBlockCallCount).Should(Equal(2))
					Consistently(c3.support.WriteBlockCallCount).Should(Equal(2))
					Consistently(c1.support.WriteBlockCallCount).Should(Equal(1))
				})

				It("does not deadlock if leader steps down while config block is in-flight", func() {
					configEnv := newConfigEnv(channelID, common.HeaderType_CONFIG, newConfigUpdateEnv(channelID, addConsenterConfigValue()))
					c1.cutter.CutNext = true

					signal := make(chan struct{})
					stub := c1.support.WriteConfigBlockStub
					c1.support.WriteConfigBlockStub = func(b *common.Block, meta []byte) {
						signal <- struct{}{}
						<-signal
						stub(b, meta)
					}

					By("Sending config transaction")
					Expect(c1.Configure(configEnv, 0)).To(Succeed())

					Eventually(signal, LongEventualTimeout).Should(Receive())
					network.disconnect(1)

					By("Ticking leader till it steps down")
					Eventually(func() raft.SoftState {
						c1.clock.Increment(interval)
						return c1.Node.Status().SoftState
					}, LongEventualTimeout).Should(StateEqual(0, raft.StateFollower))

					close(signal)

					Eventually(c1.observe, LongEventualTimeout).Should(Receive(StateEqual(0, raft.StateFollower)))

					By("Re-electing 1 as leader")
					network.connect(1)
					network.elect(1)

					_, raftmetabytes := c1.support.WriteConfigBlockArgsForCall(0)
					meta := &common.Metadata{Value: raftmetabytes}
					raftmeta, err := etcdraft.ReadRaftMetadata(meta, nil)
					Expect(err).NotTo(HaveOccurred())

					c4 := newChain(timeout, channelID, dataDir, 4, raftmeta)
					c4.init()

					// if we join a node to existing network, it MUST already obtained blocks
					// till the config block that adds this node to cluster.
					c4.support.WriteBlock(c1.support.WriteBlockArgsForCall(0))
					c4.support.WriteConfigBlock(c1.support.WriteConfigBlockArgsForCall(0))

					network.addChain(c4)
					c4.Start()

					Eventually(func() <-chan raft.SoftState {
						c1.clock.Increment(interval)
						return c4.observe
					}, LongEventualTimeout).Should(Receive(StateEqual(1, raft.StateFollower)))

					By("Submitting tx to confirm network is still working")
					Expect(c1.Order(env, 0)).To(Succeed())

					network.exec(func(c *chain) {
						Eventually(c.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(2))
						Eventually(c.support.WriteConfigBlockCallCount, LongEventualTimeout).Should(Equal(1))
					})
				})
			})
		})

		When("3/3 nodes are running", func() {
			JustBeforeEach(func() {
				network.init()
				network.start()
				network.elect(1)
			})

			AfterEach(func() {
				network.stop()
			})

			It("correctly sets the cluster size and leadership metrics", func() {
				// the network should see only one leadership change
				network.exec(func(c *chain) {
					Expect(c.fakeFields.fakeLeaderChanges.AddCallCount()).Should(Equal(1))
					Expect(c.fakeFields.fakeLeaderChanges.AddArgsForCall(0)).Should(Equal(float64(1)))
					Expect(c.fakeFields.fakeClusterSize.SetCallCount()).Should(Equal(1))
					Expect(c.fakeFields.fakeClusterSize.SetArgsForCall(0)).To(Equal(float64(3)))
				})
				// c1 should be the leader
				Expect(c1.fakeFields.fakeIsLeader.SetCallCount()).Should(Equal(2))
				Expect(c1.fakeFields.fakeIsLeader.SetArgsForCall(1)).Should(Equal(float64(1)))
				// c2 and c3 should continue to remain followers
				Expect(c2.fakeFields.fakeIsLeader.SetCallCount()).Should(Equal(1))
				Expect(c2.fakeFields.fakeIsLeader.SetArgsForCall(0)).Should(Equal(float64(0)))
				Expect(c3.fakeFields.fakeIsLeader.SetCallCount()).Should(Equal(1))
				Expect(c3.fakeFields.fakeIsLeader.SetArgsForCall(0)).Should(Equal(float64(0)))
			})

			It("orders envelope on leader", func() {
				By("instructed to cut next block")
				c1.cutter.CutNext = true
				err := c1.Order(env, 0)
				Expect(err).ToNot(HaveOccurred())

				network.exec(
					func(c *chain) {
						Eventually(c.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(1))
					})

				By("respect batch timeout")
				c1.cutter.CutNext = false

				err = c1.Order(env, 0)
				Expect(err).ToNot(HaveOccurred())
				Eventually(c1.cutter.CurBatch, LongEventualTimeout).Should(HaveLen(1))

				c1.clock.WaitForNWatchersAndIncrement(timeout, 2)
				network.exec(
					func(c *chain) {
						Eventually(c.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(2))
					})
			})

			It("orders envelope on follower", func() {
				By("instructed to cut next block")
				c1.cutter.CutNext = true
				err := c2.Order(env, 0)
				Expect(err).ToNot(HaveOccurred())

				network.exec(
					func(c *chain) {
						Eventually(func() int { return c.support.WriteBlockCallCount() }, LongEventualTimeout).Should(Equal(1))
					})

				By("respect batch timeout")
				c1.cutter.CutNext = false

				err = c2.Order(env, 0)
				Expect(err).ToNot(HaveOccurred())
				Eventually(c1.cutter.CurBatch, LongEventualTimeout).Should(HaveLen(1))

				c1.clock.WaitForNWatchersAndIncrement(timeout, 2)
				network.exec(
					func(c *chain) {
						Eventually(c.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(2))
					})
			})

			When("MaxInflightMsgs is reached", func() {
				BeforeEach(func() {
					network.exec(func(c *chain) { c.opts.MaxInflightMsgs = 1 })
				})

				It("waits for in flight blocks to be committed", func() {
					c1.cutter.CutNext = true
					// disconnect c1 to disrupt consensus
					network.disconnect(1)

					Expect(c1.Order(env, 0)).To(Succeed())

					doneProp := make(chan struct{})
					go func() {
						defer GinkgoRecover()
						Expect(c1.Order(env, 0)).To(Succeed())
						close(doneProp)
					}()
					// expect second `Order` to block
					Consistently(doneProp).ShouldNot(BeClosed())
					network.exec(func(c *chain) {
						Consistently(c.support.WriteBlockCallCount).Should(BeZero())
					})

					network.connect(1)
					c1.clock.Increment(interval)

					Eventually(doneProp, LongEventualTimeout).Should(BeClosed())
					network.exec(func(c *chain) {
						Eventually(c.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(2))
					})
				})

				It("resets block in flight when steps down from leader", func() {
					c1.cutter.CutNext = true
					c2.cutter.CutNext = true
					// disconnect c1 to disrupt consensus
					network.disconnect(1)

					Expect(c1.Order(env, 0)).To(Succeed())

					doneProp := make(chan struct{})
					go func() {
						defer GinkgoRecover()

						Expect(c1.Order(env, 0)).To(Succeed())
						close(doneProp)
					}()
					// expect second `Order` to block
					Consistently(doneProp).ShouldNot(BeClosed())
					network.exec(func(c *chain) {
						Consistently(c.support.WriteBlockCallCount).Should(BeZero())
					})

					network.elect(2)
					Expect(c3.Order(env, 0)).To(Succeed())
					Eventually(c1.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(0))
					Eventually(c2.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(1))
					Eventually(c3.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(1))

					network.connect(1)
					c2.clock.Increment(interval)

					Eventually(doneProp, LongEventualTimeout).Should(BeClosed())
					network.exec(func(c *chain) {
						Eventually(c.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(2))
					})
				})
			})

			When("leader is disconnected", func() {
				It("proactively steps down to follower", func() {
					network.disconnect(1)

					By("Ticking leader until it steps down")
					Eventually(func() <-chan raft.SoftState {
						c1.clock.Increment(interval)
						return c1.observe
					}, LongEventualTimeout).Should(Receive(Equal(raft.SoftState{Lead: 0, RaftState: raft.StateFollower})))

					By("Ensuring it does not accept message due to the cluster being leaderless")
					err := c1.Order(env, 0)
					Expect(err).To(MatchError("no Raft leader"))

					network.elect(2)

					// c1 should have lost leadership
					Expect(c1.fakeFields.fakeIsLeader.SetCallCount()).Should(Equal(3))
					Expect(c1.fakeFields.fakeIsLeader.SetArgsForCall(2)).Should(Equal(float64(0)))
					// c2 should become the leader
					Expect(c2.fakeFields.fakeIsLeader.SetCallCount()).Should(Equal(2))
					Expect(c2.fakeFields.fakeIsLeader.SetArgsForCall(1)).Should(Equal(float64(1)))
					// c2 should continue to remain follower
					Expect(c3.fakeFields.fakeIsLeader.SetCallCount()).Should(Equal(1))

					network.join(1, true)
					network.exec(func(c *chain) {
						Expect(c.fakeFields.fakeLeaderChanges.AddCallCount()).Should(Equal(3))
						Expect(c.fakeFields.fakeLeaderChanges.AddArgsForCall(2)).Should(Equal(float64(1)))
					})

					err = c1.Order(env, 0)
					Expect(err).NotTo(HaveOccurred())
				})

				It("does not deadlock if propose is blocked", func() {
					signal := make(chan struct{})
					c1.cutter.CutNext = true
					c1.support.SequenceStub = func() uint64 {
						signal <- struct{}{}
						<-signal
						return 0
					}

					By("Sending a normal transaction")
					Expect(c1.Order(env, 0)).To(Succeed())

					Eventually(signal).Should(Receive())
					network.disconnect(1)

					By("Ticking leader till it steps down")
					Eventually(func() raft.SoftState {
						c1.clock.Increment(interval)
						return c1.Node.Status().SoftState
					}).Should(StateEqual(0, raft.StateFollower))

					close(signal)

					Eventually(c1.observe).Should(Receive(StateEqual(0, raft.StateFollower)))
					c1.support.SequenceStub = nil
					network.exec(func(c *chain) {
						Eventually(c.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(0))
					})

					By("Re-electing 1 as leader")
					network.connect(1)
					network.elect(1)

					By("Sending another normal transaction")
					Expect(c1.Order(env, 0)).To(Succeed())

					network.exec(func(c *chain) {
						Eventually(c.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(1))
					})
				})
			})

			When("follower is disconnected", func() {
				It("should return error when receiving an env", func() {
					network.disconnect(2)

					errorC := c2.Errored()
					Consistently(errorC).ShouldNot(BeClosed()) // assert that errorC is not closed

					By("Ticking node 2 until it becomes pre-candidate")
					Eventually(func() <-chan raft.SoftState {
						c2.clock.Increment(interval)
						return c2.observe
					}, LongEventualTimeout).Should(Receive(Equal(raft.SoftState{Lead: 0, RaftState: raft.StatePreCandidate})))

					Eventually(errorC).Should(BeClosed())
					err := c2.Order(env, 0)
					Expect(err).To(HaveOccurred())

					network.connect(2)
					c1.clock.Increment(interval)
					Expect(errorC).To(BeClosed())

					Eventually(c2.Errored).ShouldNot(BeClosed())
				})
			})

			It("leader retransmits lost messages", func() {
				// This tests that heartbeats will trigger leader to retransmit lost MsgApp

				c1.cutter.CutNext = true

				network.disconnect(1) // drop MsgApp

				err := c1.Order(env, 0)
				Expect(err).ToNot(HaveOccurred())

				network.exec(
					func(c *chain) {
						Consistently(func() int { return c.support.WriteBlockCallCount() }).Should(Equal(0))
					})

				network.connect(1) // reconnect leader

				c1.clock.Increment(interval) // trigger a heartbeat
				network.exec(
					func(c *chain) {
						Eventually(func() int { return c.support.WriteBlockCallCount() }, LongEventualTimeout).Should(Equal(1))
					})
			})

			It("allows the leader to create multiple normal blocks without having to wait for them to be written out", func() {
				// this ensures that the created blocks are not written out
				network.disconnect(1)

				c1.cutter.CutNext = true
				for i := 0; i < 3; i++ {
					Expect(c1.Order(env, 0)).To(Succeed())
				}

				Consistently(c1.support.WriteBlockCallCount).Should(Equal(0))

				network.connect(1)

				// Enque another env to trigger resend of previous data
				Expect(c1.Order(env, 0)).To(Succeed())

				network.exec(func(c *chain) {
					Eventually(c.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(4))
				})
			})

			It("new leader should wait for in-fight blocks to commit before accepting new env", func() {
				// Scenario: when a node is elected as new leader and there are still in-flight blocks,
				// it should not immediately start accepting new envelopes, instead it should wait for
				// those in-flight blocks to be committed, otherwise we may create uncle block which
				// forks and panicks chain.
				//
				// Steps:
				// - start raft cluster with three nodes and genesis block0
				// - order env1 on c1, which creates block1
				// - drop MsgApp from 1 to 3
				// - drop second round of MsgApp sent from 1 to 2, so that block1 is only committed on c1
				// - disconnect c1 and elect c2
				// - order env2 on c2. This env must NOT be immediately accepted, otherwise c2 would create
				//   an uncle block1 based on block0.
				// - c2 commits block1
				// - c2 accepts env2, and creates block2
				// - c2 commits block2
				c1.cutter.CutNext = true
				c2.cutter.CutNext = true

				step1 := c1.getStepFunc()
				c1.setStepFunc(func(dest uint64, msg *orderer.ConsensusRequest) error {
					stepMsg := &raftpb.Message{}
					Expect(proto.Unmarshal(msg.Payload, stepMsg)).NotTo(HaveOccurred())

					if dest == 3 {
						return nil
					}

					if stepMsg.Type == raftpb.MsgApp && len(stepMsg.Entries) == 0 {
						return nil
					}

					return step1(dest, msg)
				})

				Expect(c1.Order(env, 0)).NotTo(HaveOccurred())

				Eventually(c1.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(1))
				Consistently(c2.support.WriteBlockCallCount).Should(Equal(0))
				Consistently(c3.support.WriteBlockCallCount).Should(Equal(0))

				network.disconnect(1)

				step2 := c2.getStepFunc()
				c2.setStepFunc(func(dest uint64, msg *orderer.ConsensusRequest) error {
					stepMsg := &raftpb.Message{}
					Expect(proto.Unmarshal(msg.Payload, stepMsg)).NotTo(HaveOccurred())

					if stepMsg.Type == raftpb.MsgApp && len(stepMsg.Entries) != 0 && dest == 3 {
						for _, ent := range stepMsg.Entries {
							if len(ent.Data) != 0 {
								return nil
							}
						}
					}
					return step2(dest, msg)
				})

				network.elect(2)

				go func() {
					defer GinkgoRecover()
					Expect(c2.Order(env, 0)).NotTo(HaveOccurred())
				}()

				Consistently(c2.support.WriteBlockCallCount).Should(Equal(0))
				Consistently(c3.support.WriteBlockCallCount).Should(Equal(0))

				c2.setStepFunc(step2)
				c2.clock.Increment(interval)

				Eventually(c2.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(2))
				Eventually(c3.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(2))

				b, _ := c2.support.WriteBlockArgsForCall(0)
				Expect(b.Header.Number).To(Equal(uint64(1)))
				b, _ = c2.support.WriteBlockArgsForCall(1)
				Expect(b.Header.Number).To(Equal(uint64(2)))
			})

			Context("handling config blocks", func() {
				var configEnv *common.Envelope
				BeforeEach(func() {
					values := map[string]*common.ConfigValue{
						"BatchTimeout": {
							Version: 1,
							Value: marshalOrPanic(&orderer.BatchTimeout{
								Timeout: "3ms",
							}),
						},
					}
					configEnv = newConfigEnv(channelID,
						common.HeaderType_CONFIG,
						newConfigUpdateEnv(channelID, values),
					)
				})

				It("holds up block creation on leader once a config block has been created and not written out", func() {
					// this ensures that the created blocks are not written out
					network.disconnect(1)

					c1.cutter.CutNext = true
					// config block
					err := c1.Order(configEnv, 0)
					Expect(err).NotTo(HaveOccurred())

					// to avoid data races since we are accessing these within a goroutine
					tempEnv := env
					tempC1 := c1

					done := make(chan struct{})

					// normal block
					go func() {
						defer GinkgoRecover()

						// This should be blocked if config block is not committed
						err := tempC1.Order(tempEnv, 0)
						Expect(err).NotTo(HaveOccurred())

						close(done)
					}()

					Consistently(done).ShouldNot(BeClosed())

					network.connect(1)
					c1.clock.Increment(interval)

					network.exec(
						func(c *chain) {
							Eventually(func() int { return c.support.WriteConfigBlockCallCount() }, LongEventualTimeout).Should(Equal(1))
						})

					network.exec(
						func(c *chain) {
							Eventually(func() int { return c.support.WriteBlockCallCount() }, LongEventualTimeout).Should(Equal(1))
						})
				})

				It("continues creating blocks on leader after a config block has been successfully written out", func() {
					c1.cutter.CutNext = true
					// config block
					err := c1.Configure(configEnv, 0)
					Expect(err).NotTo(HaveOccurred())
					network.exec(
						func(c *chain) {
							Eventually(func() int { return c.support.WriteConfigBlockCallCount() }, LongEventualTimeout).Should(Equal(1))
						})

					// normal block following config block
					err = c1.Order(env, 0)
					Expect(err).ToNot(HaveOccurred())
					network.exec(
						func(c *chain) {
							Eventually(func() int { return c.support.WriteBlockCallCount() }, LongEventualTimeout).Should(Equal(1))
						})
				})
			})

			When("Snapshotting is enabled", func() {
				BeforeEach(func() {
					c1.opts.SnapInterval = 1
					c1.opts.SnapshotCatchUpEntries = 1
				})

				It("keeps running if some entries in memory are purged", func() {
					// Scenario: snapshotting is enabled on node 1 and it purges memory storage
					// per every snapshot. Cluster should be correctly functioning.

					i, err := c1.opts.MemoryStorage.FirstIndex()
					Expect(err).NotTo(HaveOccurred())
					Expect(i).To(Equal(uint64(1)))

					c1.cutter.CutNext = true

					err = c1.Order(env, 0)
					Expect(err).ToNot(HaveOccurred())

					network.exec(
						func(c *chain) {
							Eventually(func() int { return c.support.WriteBlockCallCount() }, LongEventualTimeout).Should(Equal(1))
						})

					Eventually(c1.opts.MemoryStorage.FirstIndex, LongEventualTimeout).Should(BeNumerically(">", i))
					i, err = c1.opts.MemoryStorage.FirstIndex()
					Expect(err).NotTo(HaveOccurred())

					err = c1.Order(env, 0)
					Expect(err).ToNot(HaveOccurred())

					network.exec(
						func(c *chain) {
							Eventually(c.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(2))
						})

					Eventually(c1.opts.MemoryStorage.FirstIndex, LongEventualTimeout).Should(BeNumerically(">", i))
					i, err = c1.opts.MemoryStorage.FirstIndex()
					Expect(err).NotTo(HaveOccurred())

					err = c1.Order(env, 0)
					Expect(err).ToNot(HaveOccurred())

					network.exec(
						func(c *chain) {
							Eventually(c.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(3))
						})

					Eventually(c1.opts.MemoryStorage.FirstIndex, LongEventualTimeout).Should(BeNumerically(">", i))
				})

				It("lagged node can catch up using snapshot", func() {
					network.disconnect(2)

					c1.cutter.CutNext = true

					var lasti uint64
					var indices []uint64
					// Only produce 4 blocks here, so that snapshot pruning does not occur.
					// Otherwise, a slow garbage collection may prevent snapshotting from
					// being triggered on next block, and assertion on number of snapshots
					// would fail nondeterministically.
					for i := 1; i <= 4; i++ {
						err := c1.Order(env, 0)
						Expect(err).NotTo(HaveOccurred())
						Eventually(c1.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(i))
						Eventually(c3.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(i))
						Eventually(func() uint64 {
							indices = etcdraft.ListSnapshots(logger, c1.opts.SnapDir)
							if len(indices) == 0 {
								return 0
							}
							return indices[len(indices)-1]
						}, LongEventualTimeout).Should(BeNumerically(">", lasti))
						lasti = indices[len(indices)-1]
					}

					Eventually(c2.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(0))

					network.join(2, false)

					Eventually(c2.puller.PullBlockCallCount, LongEventualTimeout).Should(Equal(4))
					Eventually(c2.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(4))

					files, err := ioutil.ReadDir(c2.opts.SnapDir)
					Expect(err).NotTo(HaveOccurred())
					Expect(files).To(HaveLen(1)) // expect to store exact 1 snapshot

					// chain should keeps functioning
					err = c1.Order(env, 0)
					Expect(err).NotTo(HaveOccurred())

					network.exec(
						func(c *chain) {
							Eventually(func() int { return c.support.WriteBlockCallCount() }, LongEventualTimeout).Should(Equal(5))
						})
				})
			})

			Context("failover", func() {
				It("follower should step up as leader upon failover", func() {
					network.stop(1)
					network.elect(2)

					By("order envelope on new leader")
					c2.cutter.CutNext = true
					err := c2.Order(env, 0)
					Expect(err).ToNot(HaveOccurred())

					// block should not be produced on chain 1
					Eventually(c1.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(0))

					// block should be produced on chain 2 & 3
					Eventually(c2.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(1))
					Eventually(c3.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(1))

					By("order envelope on follower")
					err = c3.Order(env, 0)
					Expect(err).ToNot(HaveOccurred())

					// block should not be produced on chain 1
					Eventually(c1.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(0))

					// block should be produced on chain 2 & 3
					Eventually(c2.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(2))
					Eventually(c3.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(2))
				})

				It("follower cannot be elected if its log is not up-to-date", func() {
					network.disconnect(2)

					c1.cutter.CutNext = true
					err := c1.Order(env, 0)
					Expect(err).NotTo(HaveOccurred())

					Eventually(c1.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(1))
					Eventually(c2.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(0))
					Eventually(c3.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(1))

					network.disconnect(1)
					network.connect(2)

					// node 2 has not caught up with other nodes
					for tick := 0; tick < 2*ELECTION_TICK-1; tick++ {
						c2.clock.Increment(interval)
						Consistently(c2.observe).ShouldNot(Receive(Equal(2)))
					}

					// When PreVote is enabled, node 2 would fail to collect enough
					// PreVote because its index is not up-to-date. Therefore, it
					// does not cause leader change on other nodes.
					Consistently(c3.observe).ShouldNot(Receive())
					network.elect(3) // node 3 has newest logs among 2&3, so it can be elected
				})

				It("PreVote prevents reconnected node from disturbing network", func() {
					network.disconnect(2)

					c1.cutter.CutNext = true
					err := c1.Order(env, 0)
					Expect(err).NotTo(HaveOccurred())

					Eventually(c1.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(1))
					Eventually(c2.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(0))
					Eventually(c3.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(1))

					network.connect(2)

					for tick := 0; tick < 2*ELECTION_TICK-1; tick++ {
						c2.clock.Increment(interval)
						Consistently(c2.observe).ShouldNot(Receive(Equal(2)))
					}

					Consistently(c1.observe).ShouldNot(Receive())
					Consistently(c3.observe).ShouldNot(Receive())
				})

				It("follower can catch up and then campaign with success", func() {
					network.disconnect(2)

					c1.cutter.CutNext = true
					for i := 0; i < 10; i++ {
						err := c1.Order(env, 0)
						Expect(err).NotTo(HaveOccurred())
					}

					Eventually(c1.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(10))
					Eventually(c2.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(0))
					Eventually(c3.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(10))

					network.join(2, false)
					Eventually(c2.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(10))

					network.disconnect(1)
					network.elect(2)
				})

				It("purges blockcutter, stops timer and discards created blocks if leadership is lost", func() {
					// enqueue one transaction into 1's blockcutter to test for purging of block cutter
					c1.cutter.CutNext = false
					err := c1.Order(env, 0)
					Expect(err).ToNot(HaveOccurred())
					Eventually(c1.cutter.CurBatch, LongEventualTimeout).Should(HaveLen(1))

					// no block should be written because env is not cut into block yet
					c1.clock.WaitForNWatchersAndIncrement(interval, 2)
					Consistently(c1.support.WriteBlockCallCount).Should(Equal(0))

					network.disconnect(1)
					network.elect(2)
					network.join(1, true)

					Eventually(c1.clock.WatcherCount, LongEventualTimeout).Should(Equal(1)) // blockcutter time is stopped
					Eventually(c1.cutter.CurBatch, LongEventualTimeout).Should(HaveLen(0))
					// the created block should be discarded since there is a leadership change
					Consistently(c1.support.WriteBlockCallCount).Should(Equal(0))

					network.disconnect(2)
					network.elect(1)

					err = c1.Order(env, 0)
					Expect(err).ToNot(HaveOccurred())

					// The following group of assertions is redundant - it's here for completeness.
					// If the blockcutter has not been reset, fast-forwarding 1's clock to 'timeout', should result in the blockcutter firing.
					// If the blockcucter has been reset, fast-forwarding won't do anything.
					//
					// Put differently:
					//
					// correct:
					// stop         start                      fire
					// |--------------|---------------------------|
					//    n*intervals              timeout
					// (advanced in election)
					//
					// wrong:
					// unstop                   fire
					// |---------------------------|
					//          timeout
					//
					//              timeout-n*interval   n*interval
					//                 |-----------|----------------|
					//                             ^                ^
					//                at this point of time     it should fire
					//                timer should not fire     at this point

					c1.clock.WaitForNWatchersAndIncrement(timeout-interval, 2)
					Eventually(func() int { return c1.support.WriteBlockCallCount() }, LongEventualTimeout).Should(Equal(0))
					Eventually(func() int { return c3.support.WriteBlockCallCount() }, LongEventualTimeout).Should(Equal(0))

					c1.clock.Increment(interval)
					Eventually(func() int { return c1.support.WriteBlockCallCount() }, LongEventualTimeout).Should(Equal(1))
					Eventually(func() int { return c3.support.WriteBlockCallCount() }, LongEventualTimeout).Should(Equal(1))
				})

				It("stale leader should not be able to propose block because of lagged term", func() {
					network.disconnect(1)
					network.elect(2)
					network.connect(1)

					c1.cutter.CutNext = true
					err := c1.Order(env, 0)
					Expect(err).NotTo(HaveOccurred())

					network.exec(
						func(c *chain) {
							Consistently(c.support.WriteBlockCallCount).Should(Equal(0))
						})
				})

				It("aborts waiting for block to be committed upon leadership lost", func() {
					network.disconnect(1)

					c1.cutter.CutNext = true
					err := c1.Order(env, 0)
					Expect(err).NotTo(HaveOccurred())

					network.exec(
						func(c *chain) {
							Consistently(c.support.WriteBlockCallCount).Should(Equal(0))
						})

					network.elect(2)
					network.connect(1)

					c2.clock.Increment(interval)
					// this check guarantees that signal on resignC is consumed in commitBatches method.
					Eventually(c1.observe, LongEventualTimeout).Should(Receive(Equal(raft.SoftState{Lead: 2, RaftState: raft.StateFollower})))
				})
			})
		})
	})
})

func nodeConfigFromMetadata(consenterMetadata *raftprotos.Metadata) []cluster.RemoteNode {
	var nodes []cluster.RemoteNode
	for i, consenter := range consenterMetadata.Consenters {
		// For now, skip ourselves
		if i == 0 {
			continue
		}
		serverDER, _ := pem.Decode(consenter.ServerTlsCert)
		clientDER, _ := pem.Decode(consenter.ClientTlsCert)
		node := cluster.RemoteNode{
			ID:            uint64(i + 1),
			Endpoint:      "localhost:7050",
			ServerTLSCert: serverDER.Bytes,
			ClientTLSCert: clientDER.Bytes,
		}
		nodes = append(nodes, node)
	}
	return nodes
}

func createMetadata(nodeCount int, tlsCA tlsgen.CA) *raftprotos.Metadata {
	md := &raftprotos.Metadata{}
	for i := 0; i < nodeCount; i++ {
		md.Consenters = append(md.Consenters, &raftprotos.Consenter{
			Host:          "localhost",
			Port:          7050,
			ServerTlsCert: serverTLSCert(tlsCA),
			ClientTlsCert: clientTLSCert(tlsCA),
		})
	}
	return md
}

func serverTLSCert(tlsCA tlsgen.CA) []byte {
	cert, err := tlsCA.NewServerCertKeyPair("localhost")
	if err != nil {
		panic(err)
	}
	return cert.Cert
}

func clientTLSCert(tlsCA tlsgen.CA) []byte {
	cert, err := tlsCA.NewClientCertKeyPair()
	if err != nil {
		panic(err)
	}
	return cert.Cert
}

// marshalOrPanic serializes a protobuf message and panics if this
// operation fails
func marshalOrPanic(pb proto.Message) []byte {
	data, err := proto.Marshal(pb)
	if err != nil {
		panic(err)
	}
	return data
}

// helpers to facilitate tests
type stepFunc func(dest uint64, msg *orderer.ConsensusRequest) error

type chain struct {
	id uint64

	stepLock sync.RWMutex
	step     stepFunc

	support      *consensusmocks.FakeConsenterSupport
	cutter       *mockblockcutter.Receiver
	configurator *mocks.Configurator
	rpc          *mocks.FakeRPC
	storage      *raft.MemoryStorage
	clock        *fakeclock.FakeClock
	opts         etcdraft.Options
	puller       *mocks.FakeBlockPuller

	// store written blocks to be returned by mock block puller
	ledger       map[uint64]*common.Block
	ledgerHeight uint64
	ledgerLock   sync.RWMutex

	observe   chan raft.SoftState
	unstarted chan struct{}
	stopped   chan struct{}

	fakeFields *fakeMetricsFields

	*etcdraft.Chain
}

func newChain(timeout time.Duration, channel string, dataDir string, id uint64, raftMetadata *raftprotos.RaftMetadata) *chain {
	rpc := &mocks.FakeRPC{}
	clock := fakeclock.NewFakeClock(time.Now())
	storage := raft.NewMemoryStorage()

	fakeFields := newFakeMetricsFields()

	opts := etcdraft.Options{
		RaftID:          uint64(id),
		Clock:           clock,
		TickInterval:    interval,
		ElectionTick:    ELECTION_TICK,
		HeartbeatTick:   HEARTBEAT_TICK,
		MaxSizePerMsg:   1024 * 1024,
		MaxInflightMsgs: 256,
		RaftMetadata:    raftMetadata,
		Logger:          flogging.NewFabricLogger(zap.NewExample()),
		MemoryStorage:   storage,
		WALDir:          path.Join(dataDir, "wal"),
		SnapDir:         path.Join(dataDir, "snapshot"),
		Metrics:         newFakeMetrics(fakeFields),
	}

	support := &consensusmocks.FakeConsenterSupport{}
	support.ChainIDReturns(channel)
	support.SharedConfigReturns(&mockconfig.Orderer{
		BatchTimeoutVal: timeout,
		CapabilitiesVal: &mockconfig.OrdererCapabilities{
			Kafka2RaftMigVal: false,
		},
	})

	cutter := mockblockcutter.NewReceiver()
	close(cutter.Block)
	support.BlockCutterReturns(cutter)

	// upon leader change, lead is reset to 0 before set to actual
	// new leader, i.e. 1 -> 0 -> 2. Therefore 2 numbers will be
	// sent on this chan, so we need size to be 2
	observe := make(chan raft.SoftState, 2)

	configurator := &mocks.Configurator{}
	configurator.On("Configure", mock.Anything, mock.Anything)

	puller := &mocks.FakeBlockPuller{}

	ch := make(chan struct{})
	close(ch)

	c := &chain{
		id:           id,
		support:      support,
		cutter:       cutter,
		rpc:          rpc,
		storage:      storage,
		observe:      observe,
		clock:        clock,
		opts:         opts,
		unstarted:    ch,
		stopped:      make(chan struct{}),
		configurator: configurator,
		puller:       puller,
		ledger: map[uint64]*common.Block{
			0: getSeedBlock(), // Very first block
		},
		ledgerHeight: 1,
		fakeFields:   fakeFields,
	}

	// receives blocks and metadata and appends it into
	// the ledger struct to simulate write behaviour
	appendBlockToLedger := func(b *common.Block, meta []byte) {
		b = proto.Clone(b).(*common.Block)
		bytes, err := proto.Marshal(&common.Metadata{Value: meta})
		Expect(err).NotTo(HaveOccurred())
		b.Metadata.Metadata[common.BlockMetadataIndex_ORDERER] = bytes
		c.ledgerLock.Lock()
		defer c.ledgerLock.Unlock()
		c.ledger[b.Header.Number] = b
		if c.ledgerHeight < b.Header.Number+1 {
			c.ledgerHeight = b.Header.Number + 1
		}
	}

	c.support.WriteBlockStub = appendBlockToLedger
	c.support.WriteConfigBlockStub = appendBlockToLedger

	// returns current ledger height
	c.support.HeightStub = func() uint64 {
		c.ledgerLock.RLock()
		defer c.ledgerLock.RUnlock()
		return c.ledgerHeight
	}

	// reads block from the ledger
	c.support.BlockStub = func(number uint64) *common.Block {
		c.ledgerLock.RLock()
		defer c.ledgerLock.RUnlock()
		return c.ledger[number]
	}

	return c
}

func (c *chain) init() {
	ch, err := etcdraft.NewChain(
		c.support,
		c.opts,
		c.configurator,
		c.rpc,
		func() (etcdraft.BlockPuller, error) { return c.puller, nil },
		c.observe,
	)
	Expect(err).NotTo(HaveOccurred())
	c.Chain = ch
}

func (c *chain) start() {
	c.unstarted = nil
	c.Start()
}

func (c *chain) setStepFunc(f stepFunc) {
	c.stepLock.Lock()
	c.step = f
	c.stepLock.Unlock()
}

func (c *chain) getStepFunc() stepFunc {
	c.stepLock.RLock()
	defer c.stepLock.RUnlock()
	return c.step
}

type network struct {
	leader uint64
	chains map[uint64]*chain

	// used to determine connectivity of a chain.
	// the actual value type is `chan struct` because
	// it's used to skip assertion in `elect` if a
	// node is disconnected from network, therefore
	// no leader change should be observed
	connLock     sync.RWMutex
	connectivity map[uint64]chan struct{}
}

func (n *network) appendChain(c *chain) {
	n.connLock.Lock()
	n.chains[c.id] = c
	n.connLock.Unlock()
}

func (n *network) addConnection(id uint64) {
	n.connLock.Lock()
	n.connectivity[id] = make(chan struct{})
	n.connLock.Unlock()
}

func (n *network) addChain(c *chain) {
	n.addConnection(c.id)

	c.step = func(dest uint64, msg *orderer.ConsensusRequest) error {
		n.connLock.RLock()
		defer n.connLock.RUnlock()

		select {
		case <-n.connectivity[dest]:
		case <-n.connectivity[c.id]:
		default:
			// get a reference of chain while
			// n.chains is still RLock'ed
			target := n.chains[dest]
			go func() {
				defer GinkgoRecover()
				target.Consensus(msg, c.id)
			}()
		}

		return nil
	}

	c.rpc.SendConsensusStub = func(dest uint64, msg *orderer.ConsensusRequest) error {
		c.stepLock.RLock()
		defer c.stepLock.RUnlock()
		return c.step(dest, msg)
	}

	c.rpc.SendSubmitStub = func(dest uint64, msg *orderer.SubmitRequest) error {
		n.connLock.RLock()
		defer n.connLock.RUnlock()

		select {
		case <-n.connectivity[dest]:
		case <-n.connectivity[c.id]:
		default:
			// get a reference of chain while
			// n.chains is still RLock'ed
			target := n.chains[dest]
			go func() {
				defer GinkgoRecover()
				target.Submit(msg, c.id)
			}()
		}

		return nil
	}

	c.puller.PullBlockStub = func(i uint64) *common.Block {
		n.connLock.RLock()
		leaderChain := n.chains[n.leader]
		n.connLock.RUnlock()

		leaderChain.ledgerLock.RLock()
		defer leaderChain.ledgerLock.RUnlock()
		block := leaderChain.ledger[i]
		return block
	}

	n.appendChain(c)
}

func createNetwork(timeout time.Duration, channel string, dataDir string, raftMetadata *raftprotos.RaftMetadata) *network {
	n := &network{
		chains:       make(map[uint64]*chain),
		connectivity: make(map[uint64]chan struct{}),
	}

	for nodeID := range raftMetadata.Consenters {
		dir, err := ioutil.TempDir(dataDir, fmt.Sprintf("node-%d-", nodeID))
		Expect(err).NotTo(HaveOccurred())

		m := proto.Clone(raftMetadata).(*raftprotos.RaftMetadata)
		n.addChain(newChain(timeout, channel, dir, nodeID, m))
	}

	return n
}

// tests could alter configuration of a chain before creating it
func (n *network) init() {
	n.exec(func(c *chain) { c.init() })
}

func (n *network) start(ids ...uint64) {
	nodes := ids
	if len(nodes) == 0 {
		for i := range n.chains {
			nodes = append(nodes, i)
		}
	}

	for _, id := range nodes {
		n.chains[id].start()

		// When the Raft node bootstraps, it produces a ConfChange
		// to add itself, which needs to be consumed with Ready().
		// If there are pending configuration changes in raft,
		// it refused to campaign, no matter how many ticks supplied.
		// This is not a problem in production code because eventually
		// raft.Ready will be consumed as real time goes by.
		//
		// However, this is problematic when using fake clock and artificial
		// ticks. Instead of ticking raft indefinitely until raft.Ready is
		// consumed, this check is added to indirectly guarantee
		// that first ConfChange is actually consumed and we can safely
		// proceed to tick raft.
		Eventually(func() error {
			_, err := n.chains[id].storage.Entries(1, 1, 1)
			return err
		}, LongEventualTimeout).ShouldNot(HaveOccurred())
		Eventually(n.chains[id].WaitReady, LongEventualTimeout).ShouldNot(HaveOccurred())
	}
}

func (n *network) stop(ids ...uint64) {
	nodes := ids
	if len(nodes) == 0 {
		for i := range n.chains {
			nodes = append(nodes, i)
		}
	}

	for _, id := range nodes {
		c := n.chains[id]
		c.Halt()
		Eventually(c.Errored).Should(BeClosed())
		select {
		case <-c.stopped:
		default:
			close(c.stopped)
		}
	}
}

func (n *network) exec(f func(c *chain), ids ...uint64) {
	if len(ids) == 0 {
		for _, c := range n.chains {
			f(c)
		}

		return
	}

	for _, i := range ids {
		f(n.chains[i])
	}
}

// connect a node to network and tick leader to trigger
// a heartbeat so newly joined node can detect leader.
//
// expectLeaderChange controls whether leader change should
// be observed on newly joined node.
// - it should be true if newly joined node was leader
// - it should be false if newly joined node was follower, and
//   already knows the leader.
func (n *network) join(id uint64, expectLeaderChange bool) {
	n.connect(id)

	leader, follower := n.chains[n.leader], n.chains[id]
	step := leader.getStepFunc()
	signal := make(chan struct{})
	leader.setStepFunc(func(dest uint64, msg *orderer.ConsensusRequest) error {
		if dest == id {
			// close signal channel when a message targeting newly
			// joined node is observed on wire.
			select {
			case <-signal:
			default:
				close(signal)
			}
		}

		return step(dest, msg)
	})

	leader.clock.Increment(interval)
	Eventually(signal, LongEventualTimeout).Should(BeClosed())

	leader.setStepFunc(step)

	if expectLeaderChange {
		Eventually(follower.observe, LongEventualTimeout).Should(Receive(Equal(raft.SoftState{Lead: n.leader, RaftState: raft.StateFollower})))
	}

	// wait for newly joined node to catch up with leader
	i, err := n.chains[n.leader].opts.MemoryStorage.LastIndex()
	Expect(err).NotTo(HaveOccurred())
	Eventually(n.chains[id].opts.MemoryStorage.LastIndex, LongEventualTimeout).Should(Equal(i))
}

// elect deterministically elects a node as leader
func (n *network) elect(id uint64) {
	n.connLock.RLock()
	c := n.chains[id]
	n.connLock.RUnlock()

	// Send node an artificial MsgTimeoutNow to emulate leadership transfer.
	c.Consensus(&orderer.ConsensusRequest{Payload: utils.MarshalOrPanic(&raftpb.Message{Type: raftpb.MsgTimeoutNow})}, 0)
	Eventually(c.observe, LongEventualTimeout).Should(Receive(StateEqual(id, raft.StateLeader)))

	// now observe leader change on other nodes
	n.connLock.RLock()
	for _, c := range n.chains {
		if c.id == id {
			continue
		}

		select {
		case <-c.stopped: // skip check if node n is stopped
		case <-n.connectivity[c.id]: // skip check if node n is disconnected
		case <-c.unstarted: // skip check if node is not started yet
		default:
			Eventually(c.observe, LongEventualTimeout).Should(Receive(StateEqual(id, raft.StateFollower)))
		}
	}
	n.connLock.RUnlock()

	n.leader = id
}

func (n *network) disconnect(i uint64) {
	n.connLock.Lock()
	defer n.connLock.Unlock()
	close(n.connectivity[i])
}

func (n *network) connect(i uint64) {
	n.connLock.Lock()
	defer n.connLock.Unlock()
	n.connectivity[i] = make(chan struct{})
}

// sets the configEnv var declared above
func newConfigEnv(chainID string, headerType common.HeaderType, configUpdateEnv *common.ConfigUpdateEnvelope) *common.Envelope {
	return &common.Envelope{
		Payload: marshalOrPanic(&common.Payload{
			Header: &common.Header{
				ChannelHeader: marshalOrPanic(&common.ChannelHeader{
					Type:      int32(headerType),
					ChannelId: chainID,
				}),
			},
			Data: marshalOrPanic(&common.ConfigEnvelope{
				LastUpdate: &common.Envelope{
					Payload: marshalOrPanic(&common.Payload{
						Header: &common.Header{
							ChannelHeader: marshalOrPanic(&common.ChannelHeader{
								Type:      int32(common.HeaderType_CONFIG_UPDATE),
								ChannelId: chainID,
							}),
						},
						Data: marshalOrPanic(configUpdateEnv),
					}), // common.Payload
				}, // LastUpdate
			}),
		}),
	}
}

func newConfigUpdateEnv(chainID string, values map[string]*common.ConfigValue) *common.ConfigUpdateEnvelope {
	return &common.ConfigUpdateEnvelope{
		ConfigUpdate: marshalOrPanic(&common.ConfigUpdate{
			ChannelId: chainID,
			ReadSet:   &common.ConfigGroup{},
			WriteSet: &common.ConfigGroup{
				Groups: map[string]*common.ConfigGroup{
					"Orderer": {
						Values: values,
					},
				},
			}, // WriteSet
		}),
	}
}

func getSeedBlock() *common.Block {
	return &common.Block{
		Header:   &common.BlockHeader{},
		Data:     &common.BlockData{Data: [][]byte{[]byte("foo")}},
		Metadata: &common.BlockMetadata{Metadata: make([][]byte, 4)},
	}
}

func StateEqual(lead uint64, state raft.StateType) types.GomegaMatcher {
	return Equal(raft.SoftState{Lead: lead, RaftState: state})
}

func newFakeMetrics(fakeFields *fakeMetricsFields) *etcdraft.Metrics {
	return &etcdraft.Metrics{
		ClusterSize:          fakeFields.fakeClusterSize,
		IsLeader:             fakeFields.fakeIsLeader,
		CommittedBlockNumber: fakeFields.fakeCommittedBlockNumber,
		SnapshotBlockNumber:  fakeFields.fakeSnapshotBlockNumber,
		LeaderChanges:        fakeFields.fakeLeaderChanges,
		ProposalFailures:     fakeFields.fakeProposalFailures,
	}
}

type fakeMetricsFields struct {
	fakeClusterSize          *metricsfakes.Gauge
	fakeIsLeader             *metricsfakes.Gauge
	fakeCommittedBlockNumber *metricsfakes.Gauge
	fakeSnapshotBlockNumber  *metricsfakes.Gauge
	fakeLeaderChanges        *metricsfakes.Counter
	fakeProposalFailures     *metricsfakes.Counter
}

func newFakeMetricsFields() *fakeMetricsFields {
	return &fakeMetricsFields{
		fakeClusterSize:          newFakeGauge(),
		fakeIsLeader:             newFakeGauge(),
		fakeCommittedBlockNumber: newFakeGauge(),
		fakeSnapshotBlockNumber:  newFakeGauge(),
		fakeLeaderChanges:        newFakeCounter(),
		fakeProposalFailures:     newFakeCounter(),
	}
}

func newFakeGauge() *metricsfakes.Gauge {
	fakeGauge := &metricsfakes.Gauge{}
	fakeGauge.WithReturns(fakeGauge)
	return fakeGauge
}

func newFakeCounter() *metricsfakes.Counter {
	fakeCounter := &metricsfakes.Counter{}
	fakeCounter.WithReturns(fakeCounter)
	return fakeCounter
}
