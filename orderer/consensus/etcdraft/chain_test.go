/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package etcdraft_test

import (
	"encoding/pem"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"code.cloudfoundry.org/clock/fakeclock"
	"github.com/coreos/etcd/raft"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/common/flogging"
	mockconfig "github.com/hyperledger/fabric/common/mocks/config"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	clustermocks "github.com/hyperledger/fabric/orderer/common/cluster/mocks"
	"github.com/hyperledger/fabric/orderer/consensus/etcdraft"
	consensusmocks "github.com/hyperledger/fabric/orderer/consensus/mocks"
	mockblockcutter "github.com/hyperledger/fabric/orderer/mocks/common/blockcutter"
	"github.com/hyperledger/fabric/protos/common"
	raftprotos "github.com/hyperledger/fabric/protos/orderer/etcdraft"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

var _ = Describe("Chain", func() {
	var (
		m           *common.Envelope
		normalBlock *common.Block
		interval    time.Duration
		channelID   string
		tlsCA       tlsgen.CA
	)

	BeforeEach(func() {
		tlsCA, _ = tlsgen.NewCA()
		channelID = "test-chain"
		m = &common.Envelope{
			Payload: utils.MarshalOrPanic(&common.Payload{
				Header: &common.Header{ChannelHeader: utils.MarshalOrPanic(&common.ChannelHeader{Type: int32(common.HeaderType_MESSAGE), ChannelId: channelID})},
				Data:   []byte("TEST_MESSAGE"),
			}),
		}
		normalBlock = &common.Block{Data: &common.BlockData{Data: [][]byte{[]byte("foo")}}}
		interval = time.Second
	})

	Describe("Single raft node", func() {
		var (
			comm              *clustermocks.Communicator
			consenterMetadata *raftprotos.Metadata
			clock             *fakeclock.FakeClock
			opts              etcdraft.Options
			support           *consensusmocks.FakeConsenterSupport
			cutter            *mockblockcutter.Receiver
			storage           *raft.MemoryStorage
			observeC          chan uint64
			chain             *etcdraft.Chain
			logger            *flogging.FabricLogger
		)

		BeforeEach(func() {
			comm = &clustermocks.Communicator{}
			comm.On("Configure", mock.Anything, mock.Anything)
			clock = fakeclock.NewFakeClock(time.Now())
			storage = raft.NewMemoryStorage()
			logger = flogging.NewFabricLogger(zap.NewNop())
			observeC = make(chan uint64, 1)
			opts = etcdraft.Options{
				RaftID:          1,
				Clock:           clock,
				TickInterval:    interval,
				ElectionTick:    2,
				HeartbeatTick:   1,
				MaxSizePerMsg:   1024 * 1024,
				MaxInflightMsgs: 256,
				Peers:           []raft.Peer{{ID: 1}},
				Logger:          logger,
				Storage:         storage,
			}
			support = &consensusmocks.FakeConsenterSupport{}
			support.ChainIDReturns(channelID)
			consenterMetadata = createMetadata(3, tlsCA)
			support.SharedConfigReturns(&mockconfig.Orderer{
				BatchTimeoutVal:      time.Hour,
				ConsensusMetadataVal: utils.MarshalOrPanic(consenterMetadata),
			})
			cutter = mockblockcutter.NewReceiver()
			support.BlockCutterReturns(cutter)

			var err error
			chain, err = etcdraft.NewChain(support, opts, observeC, comm)
			Expect(err).NotTo(HaveOccurred())
		})

		JustBeforeEach(func() {
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
			}).ShouldNot(HaveOccurred())
		})

		AfterEach(func() {
			chain.Halt()
		})

		Context("when a node starts up", func() {
			It("properly configures the communication layer", func() {
				expectedNodeConfig := nodeConfigFromMetadata(consenterMetadata)
				comm.AssertCalled(testingInstance, "Configure", channelID, expectedNodeConfig)
			})
		})

		Context("when no raft leader is elected", func() {
			It("fails to order envelope", func() {
				err := chain.Order(m, 0)
				Expect(err).To(MatchError("no raft leader"))
			})
		})

		Context("when raft leader is elected", func() {
			JustBeforeEach(func() {
				Eventually(func() bool {
					clock.Increment(interval)
					select {
					case <-observeC:
						return true
					default:
						return false
					}
				}).Should(BeTrue())
			})

			It("fails to order envelope if chain is halted", func() {
				chain.Halt()
				err := chain.Order(m, 0)
				Expect(err).To(MatchError("chain is stopped"))
			})

			It("produces blocks following batch rules", func() {
				close(cutter.Block)
				support.CreateNextBlockReturns(normalBlock)

				By("cutting next batch directly")
				cutter.CutNext = true
				err := chain.Order(m, 0)
				Expect(err).NotTo(HaveOccurred())
				Eventually(support.WriteBlockCallCount).Should(Equal(1))

				By("respecting batch timeout")
				cutter.CutNext = false
				timeout := time.Second
				support.SharedConfigReturns(&mockconfig.Orderer{BatchTimeoutVal: timeout})
				err = chain.Order(m, 0)
				Expect(err).NotTo(HaveOccurred())

				clock.WaitForNWatchersAndIncrement(timeout, 2)
				Eventually(support.WriteBlockCallCount).Should(Equal(2))
			})

			It("does not reset timer for every envelope", func() {
				close(cutter.Block)
				support.CreateNextBlockReturns(normalBlock)

				timeout := time.Second
				support.SharedConfigReturns(&mockconfig.Orderer{BatchTimeoutVal: timeout})

				err := chain.Order(m, 0)
				Expect(err).NotTo(HaveOccurred())
				Eventually(cutter.CurBatch).Should(HaveLen(1))

				clock.WaitForNWatchersAndIncrement(timeout/2, 2)

				err = chain.Order(m, 0)
				Expect(err).NotTo(HaveOccurred())
				Eventually(cutter.CurBatch).Should(HaveLen(2))

				// the second envelope should not reset the timer; it should
				// therefore expire if we increment it by just timeout/2
				clock.Increment(timeout / 2)
				Eventually(support.WriteBlockCallCount).Should(Equal(1))
			})

			It("does not write a block if halted before timeout", func() {
				close(cutter.Block)
				timeout := time.Second
				support.SharedConfigReturns(&mockconfig.Orderer{BatchTimeoutVal: timeout})

				err := chain.Order(m, 0)
				Expect(err).NotTo(HaveOccurred())
				Eventually(cutter.CurBatch).Should(HaveLen(1))

				// wait for timer to start
				Eventually(clock.WatcherCount).Should(Equal(2))

				chain.Halt()
				Consistently(support.WriteBlockCallCount).Should(Equal(0))
			})

			It("stops the timer if a batch is cut", func() {
				close(cutter.Block)
				support.CreateNextBlockReturns(normalBlock)

				timeout := time.Second
				support.SharedConfigReturns(&mockconfig.Orderer{BatchTimeoutVal: timeout})

				err := chain.Order(m, 0)
				Expect(err).NotTo(HaveOccurred())
				Eventually(cutter.CurBatch).Should(HaveLen(1))

				clock.WaitForNWatchersAndIncrement(timeout/2, 2)

				By("force a batch to be cut before timer expires")
				cutter.CutNext = true
				err = chain.Order(m, 0)
				Expect(err).NotTo(HaveOccurred())
				Eventually(support.WriteBlockCallCount).Should(Equal(1))
				Expect(support.CreateNextBlockArgsForCall(0)).To(HaveLen(2))
				Expect(cutter.CurBatch()).To(HaveLen(0))

				// this should start a fresh timer
				cutter.CutNext = false
				err = chain.Order(m, 0)
				Expect(err).NotTo(HaveOccurred())
				Eventually(cutter.CurBatch).Should(HaveLen(1))

				clock.WaitForNWatchersAndIncrement(timeout/2, 2)
				Consistently(support.WriteBlockCallCount).Should(Equal(1))

				clock.Increment(timeout / 2)
				Eventually(support.WriteBlockCallCount).Should(Equal(2))
				Expect(support.CreateNextBlockArgsForCall(1)).To(HaveLen(1))
			})

			It("cut two batches if incoming envelope does not fit into first batch", func() {
				close(cutter.Block)
				support.CreateNextBlockReturns(normalBlock)

				timeout := time.Second
				support.SharedConfigReturns(&mockconfig.Orderer{BatchTimeoutVal: timeout})

				err := chain.Order(m, 0)
				Expect(err).NotTo(HaveOccurred())
				Eventually(cutter.CurBatch).Should(HaveLen(1))

				cutter.IsolatedTx = true
				err = chain.Order(m, 0)
				Expect(err).NotTo(HaveOccurred())

				Eventually(support.CreateNextBlockCallCount).Should(Equal(2))
				Eventually(support.WriteBlockCallCount).Should(Equal(2))
			})

			Context("revalidation", func() {
				BeforeEach(func() {
					close(cutter.Block)
					support.CreateNextBlockReturns(normalBlock)

					timeout := time.Hour
					support.SharedConfigReturns(&mockconfig.Orderer{BatchTimeoutVal: timeout})
					support.SequenceReturns(1)
				})

				It("enqueue if an envelope is still valid", func() {
					support.ProcessNormalMsgReturns(1, nil)

					err := chain.Order(m, 0)
					Expect(err).NotTo(HaveOccurred())
					Eventually(cutter.CurBatch).Should(HaveLen(1))
				})

				It("does not enqueue if an envelope is not valid", func() {
					support.ProcessNormalMsgReturns(1, errors.Errorf("Envelope is invalid"))

					err := chain.Order(m, 0)
					Expect(err).NotTo(HaveOccurred())
					Consistently(cutter.CurBatch).Should(HaveLen(0))
				})
			})

			It("unblocks Errored if chain is halted", func() {
				Expect(chain.Errored()).NotTo(Receive())
				chain.Halt()
				Expect(chain.Errored()).Should(BeClosed())
			})

			It("config message is not yet supported", func() {
				c := &common.Envelope{
					Payload: utils.MarshalOrPanic(&common.Payload{
						Header: &common.Header{ChannelHeader: utils.MarshalOrPanic(&common.ChannelHeader{Type: int32(common.HeaderType_CONFIG), ChannelId: channelID})},
						Data:   []byte("TEST_MESSAGE"),
					}),
				}

				Expect(func() {
					chain.Configure(c, 0)
				}).To(Panic())
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
