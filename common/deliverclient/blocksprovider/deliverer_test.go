/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blocksprovider_test

import (
	"fmt"
	"os"
	"path"
	"sync"
	"time"

	"github.com/hyperledger/fabric-lib-go/bccsp"
	"github.com/hyperledger/fabric-lib-go/bccsp/sw"
	"github.com/hyperledger/fabric-lib-go/common/flogging"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/common/deliverclient/blocksprovider"
	"github.com/hyperledger/fabric/common/deliverclient/blocksprovider/fake"
	"github.com/hyperledger/fabric/common/deliverclient/orderers"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/internal/configtxgen/encoder"
	"github.com/hyperledger/fabric/internal/configtxgen/genesisconfig"
	. "github.com/hyperledger/fabric/internal/test"
	"github.com/hyperledger/fabric/protoutil"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
)

const eventuallyTO = 20 * time.Second

var _ = Describe("CFT-Deliverer", func() {
	var (
		d                                  *blocksprovider.Deliverer
		ccs                                []*grpc.ClientConn
		fakeDialer                         *fake.Dialer
		fakeBlockHandler                   *fake.BlockHandler
		fakeOrdererConnectionSource        *fake.OrdererConnectionSource
		fakeOrdererConnectionSourceFactory *fake.OrdererConnectionSourceFactory
		fakeLedgerInfo                     *fake.LedgerInfo
		fakeUpdatableBlockVerifier         *fake.UpdatableBlockVerifier
		fakeSigner                         *fake.Signer
		fakeDeliverStreamer                *fake.DeliverStreamer
		fakeDeliverClient                  *fake.DeliverClient
		fakeSleeper                        *fake.Sleeper
		fakeDurationExceededHandler        *fake.DurationExceededHandler
		fakeCryptoProvider                 bccsp.BCCSP
		doneC                              chan struct{}
		recvStep                           chan struct{}
		endC                               chan struct{}
		mutex                              sync.Mutex
		tempDir                            string
		channelConfig                      *common.Config
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "deliverer")
		Expect(err).NotTo(HaveOccurred())

		doneC = make(chan struct{})
		recvStep = make(chan struct{})

		// appease the race detector
		recvStep := recvStep
		doneC := doneC

		fakeDialer = &fake.Dialer{}
		ccs = nil
		fakeDialer.DialStub = func(string, [][]byte) (*grpc.ClientConn, error) {
			mutex.Lock()
			defer mutex.Unlock()
			cc, err := grpc.Dial("localhost:6006", grpc.WithTransportCredentials(insecure.NewCredentials()))
			ccs = append(ccs, cc)
			Expect(err).NotTo(HaveOccurred())
			Expect(cc.GetState()).NotTo(Equal(connectivity.Shutdown))
			return cc, nil
		}

		fakeBlockHandler = &fake.BlockHandler{}
		fakeUpdatableBlockVerifier = &fake.UpdatableBlockVerifier{}
		fakeSigner = &fake.Signer{}

		fakeLedgerInfo = &fake.LedgerInfo{}
		fakeLedgerInfo.LedgerHeightReturns(7, nil)

		fakeOrdererConnectionSource = &fake.OrdererConnectionSource{}
		fakeOrdererConnectionSource.RandomEndpointReturns(&orderers.Endpoint{
			Address: "orderer-address",
		}, nil)

		fakeOrdererConnectionSourceFactory = &fake.OrdererConnectionSourceFactory{}
		fakeOrdererConnectionSourceFactory.CreateConnectionSourceReturns(fakeOrdererConnectionSource)

		fakeDeliverClient = &fake.DeliverClient{}
		fakeDeliverClient.RecvStub = func() (*orderer.DeliverResponse, error) {
			select {
			case <-recvStep:
				return nil, fmt.Errorf("fake-recv-step-error")
			case <-doneC:
				return nil, nil
			}
		}

		fakeDeliverClient.CloseSendStub = func() error {
			select {
			case recvStep <- struct{}{}:
			case <-doneC:
			}
			return nil
		}

		fakeDeliverStreamer = &fake.DeliverStreamer{}
		fakeDeliverStreamer.DeliverReturns(fakeDeliverClient, nil)

		fakeDurationExceededHandler = &fake.DurationExceededHandler{}
		fakeDurationExceededHandler.DurationExceededHandlerReturns(false)

		channelConfig, fakeCryptoProvider, err = testSetup(tempDir, "CFT")
		Expect(err).NotTo(HaveOccurred())

		d = &blocksprovider.Deliverer{
			ChannelID:                       "channel-id",
			BlockHandler:                    fakeBlockHandler,
			Ledger:                          fakeLedgerInfo,
			UpdatableBlockVerifier:          fakeUpdatableBlockVerifier,
			Dialer:                          fakeDialer,
			OrderersSourceFactory:           fakeOrdererConnectionSourceFactory,
			CryptoProvider:                  fakeCryptoProvider,
			DoneC:                           make(chan struct{}),
			Signer:                          fakeSigner,
			DeliverStreamer:                 fakeDeliverStreamer,
			Logger:                          flogging.MustGetLogger("blocksprovider"),
			TLSCertHash:                     []byte("tls-cert-hash"),
			MaxRetryDuration:                time.Hour,
			MaxRetryDurationExceededHandler: fakeDurationExceededHandler.DurationExceededHandler,
			MaxRetryInterval:                10 * time.Second,
			InitialRetryInterval:            100 * time.Millisecond,
		}
		d.Initialize(channelConfig)
		fakeSleeper = &fake.Sleeper{}
		blocksprovider.SetSleeper(d, fakeSleeper)
	})

	JustBeforeEach(func() {
		endC = make(chan struct{})
		go func() {
			d.DeliverBlocks()
			close(endC)
		}()
	})

	AfterEach(func() {
		d.Stop()
		close(doneC)
		<-endC

		_ = os.RemoveAll(tempDir)
	})

	It("waits patiently for new blocks from the orderer", func() {
		Consistently(endC).ShouldNot(BeClosed())
		mutex.Lock()
		defer mutex.Unlock()
		Expect(ccs[0].GetState()).NotTo(Equal(connectivity.Shutdown))
	})

	It("checks the ledger height", func() {
		Eventually(fakeLedgerInfo.LedgerHeightCallCount, eventuallyTO).Should(Equal(1))
	})

	When("the ledger returns an error", func() {
		BeforeEach(func() {
			fakeLedgerInfo.LedgerHeightReturns(0, fmt.Errorf("fake-ledger-error"))
		})

		It("exits the loop", func() {
			Eventually(endC, eventuallyTO).Should(BeClosed())
		})
	})

	It("signs the seek info request", func() {
		Eventually(fakeSigner.SignCallCount, eventuallyTO).Should(Equal(1))
		// Note, the signer is used inside a util method
		// which has its own set of tests, so checking the args
		// in this test is unnecessary
	})

	When("the signer returns an error", func() {
		BeforeEach(func() {
			fakeSigner.SignReturns(nil, fmt.Errorf("fake-signer-error"))
		})

		It("exits the loop", func() {
			Eventually(endC, eventuallyTO).Should(BeClosed())
		})
	})

	It("gets a random endpoint to connect to from the orderer connection source", func() {
		Eventually(fakeOrdererConnectionSource.RandomEndpointCallCount,
			eventuallyTO).Should(Equal(1))
	})

	When("the orderer connection source returns an error", func() {
		BeforeEach(func() {
			fakeOrdererConnectionSource.RandomEndpointReturnsOnCall(0, nil, fmt.Errorf("fake-endpoint-error"))
			fakeOrdererConnectionSource.RandomEndpointReturnsOnCall(1, &orderers.Endpoint{
				Address: "orderer-address",
			}, nil)
		})

		It("sleeps and retries until a valid endpoint is selected", func() {
			Eventually(fakeOrdererConnectionSource.RandomEndpointCallCount, eventuallyTO).Should(Equal(2))
			Expect(fakeSleeper.SleepCallCount()).To(Equal(1))
			Expect(fakeSleeper.SleepArgsForCall(0)).To(Equal(100 * time.Millisecond))
		})
	})

	When("the orderer connect is refreshed", func() {
		BeforeEach(func() {
			refreshedC := make(chan struct{})
			close(refreshedC)
			fakeOrdererConnectionSource.RandomEndpointReturnsOnCall(0, &orderers.Endpoint{
				Address:   "orderer-address",
				Refreshed: refreshedC,
			}, nil)
			fakeOrdererConnectionSource.RandomEndpointReturnsOnCall(1, &orderers.Endpoint{
				Address: "orderer-address",
			}, nil)
		})

		It("does not sleep, but disconnects and immediately tries to reconnect", func() {
			Eventually(fakeOrdererConnectionSource.RandomEndpointCallCount, eventuallyTO).Should(Equal(2))
			Expect(fakeSleeper.SleepCallCount()).To(Equal(0))
		})
	})

	It("dials the random endpoint", func() {
		Eventually(fakeDialer.DialCallCount, eventuallyTO).Should(Equal(1))
		addr, tlsCerts := fakeDialer.DialArgsForCall(0)
		Expect(addr).To(Equal("orderer-address"))
		Expect(tlsCerts).To(BeNil()) // TODO
	})

	When("the dialer returns an error", func() {
		BeforeEach(func() {
			fakeDialer.DialReturnsOnCall(0, nil, fmt.Errorf("fake-dial-error"))
			cc, err := grpc.Dial("localhost:6006", grpc.WithTransportCredentials(insecure.NewCredentials()))
			Expect(err).NotTo(HaveOccurred())
			fakeDialer.DialReturnsOnCall(1, cc, nil)
		})

		It("sleeps and retries until dial is successful", func() {
			Eventually(fakeDialer.DialCallCount, eventuallyTO).Should(Equal(2))
			Expect(fakeSleeper.SleepCallCount()).To(Equal(1))
			Expect(fakeSleeper.SleepArgsForCall(0)).To(Equal(100 * time.Millisecond))
		})
	})

	It("constructs a deliver client", func() {
		Eventually(fakeDeliverStreamer.DeliverCallCount, eventuallyTO).Should(Equal(1))
	})

	When("the deliver client cannot be created", func() {
		BeforeEach(func() {
			fakeDeliverStreamer.DeliverReturnsOnCall(0, nil, fmt.Errorf("deliver-error"))
			fakeDeliverStreamer.DeliverReturnsOnCall(1, fakeDeliverClient, nil)
		})

		It("closes the grpc connection, sleeps, and tries again", func() {
			Eventually(fakeDeliverStreamer.DeliverCallCount, eventuallyTO).Should(Equal(2))
			Expect(fakeSleeper.SleepCallCount()).To(Equal(1))
			Expect(fakeSleeper.SleepArgsForCall(0)).To(Equal(100 * time.Millisecond))
		})
	})

	When("there are consecutive errors", func() {
		BeforeEach(func() {
			fakeDeliverStreamer.DeliverReturnsOnCall(0, nil, fmt.Errorf("deliver-error"))
			fakeDeliverStreamer.DeliverReturnsOnCall(1, nil, fmt.Errorf("deliver-error"))
			fakeDeliverStreamer.DeliverReturnsOnCall(2, nil, fmt.Errorf("deliver-error"))
			fakeDeliverStreamer.DeliverReturnsOnCall(3, fakeDeliverClient, nil)
		})

		It("sleeps in an exponential fashion and retries until dial is successful", func() {
			Eventually(fakeDeliverStreamer.DeliverCallCount, eventuallyTO).Should(Equal(4))
			Expect(fakeSleeper.SleepCallCount()).To(Equal(3))
			Expect(fakeSleeper.SleepArgsForCall(0)).To(Equal(100 * time.Millisecond))
			Expect(fakeSleeper.SleepArgsForCall(1)).To(Equal(120 * time.Millisecond))
			Expect(fakeSleeper.SleepArgsForCall(2)).To(Equal(144 * time.Millisecond))
		})
	})

	When("the consecutive errors are unbounded and the peer is not a static leader", func() {
		BeforeEach(func() {
			fakeDurationExceededHandler.DurationExceededHandlerReturns(true)
			fakeDeliverStreamer.DeliverReturns(nil, fmt.Errorf("deliver-error"))
			fakeDeliverStreamer.DeliverReturnsOnCall(500, fakeDeliverClient, nil)
		})

		It("hits the maximum sleep time value in an exponential fashion and retries until exceeding the max retry duration", func() {
			Eventually(fakeDurationExceededHandler.DurationExceededHandlerCallCount, eventuallyTO).Should(BeNumerically(">", 0))
			Eventually(endC, eventuallyTO).Should(BeClosed())
			Eventually(fakeSleeper.SleepCallCount, eventuallyTO).Should(Equal(380))
			Expect(fakeSleeper.SleepArgsForCall(25)).To(Equal(9539 * time.Millisecond))
			Expect(fakeSleeper.SleepArgsForCall(26)).To(Equal(10 * time.Second))
			Expect(fakeSleeper.SleepArgsForCall(27)).To(Equal(10 * time.Second))
			Expect(fakeSleeper.SleepArgsForCall(379)).To(Equal(10 * time.Second))
			Expect(fakeDurationExceededHandler.DurationExceededHandlerCallCount()).Should(Equal(1))
		})
	})

	When("the consecutive errors are coming in short bursts and the peer is not a static leader", func() {
		BeforeEach(func() {
			// appease the race detector
			doneC := doneC
			recvStep := recvStep
			fakeDeliverClient := fakeDeliverClient

			fakeDeliverClient.CloseSendStub = func() error {
				if fakeDeliverClient.CloseSendCallCount() >= 1000 {
					select {
					case <-doneC:
					case recvStep <- struct{}{}:
					}
				}
				return nil
			}
			fakeDeliverClient.RecvStub = func() (*orderer.DeliverResponse, error) {
				c := fakeDeliverClient.RecvCallCount()
				switch c {
				case 300:
					return &orderer.DeliverResponse{
						Type: &orderer.DeliverResponse_Block{
							Block: &common.Block{
								Header: &common.BlockHeader{
									Number: 8,
								},
							},
						},
					}, nil
				case 600:
					return &orderer.DeliverResponse{
						Type: &orderer.DeliverResponse_Block{
							Block: &common.Block{
								Header: &common.BlockHeader{
									Number: 9,
								},
							},
						},
					}, nil
				case 900:
					return &orderer.DeliverResponse{
						Type: &orderer.DeliverResponse_Block{
							Block: &common.Block{
								Header: &common.BlockHeader{
									Number: 9,
								},
							},
						},
					}, nil
				default:
					if c < 900 {
						return nil, fmt.Errorf("fake-recv-error-XXX")
					}

					select {
					case <-recvStep:
						return nil, fmt.Errorf("fake-recv-step-error-XXX")
					case <-doneC:
						return nil, nil
					}
				}
			}
			fakeDurationExceededHandler.DurationExceededHandlerReturns(true)
		})

		It("hits the maximum sleep time value in an exponential fashion and retries but does not exceed the max retry duration", func() {
			Eventually(fakeSleeper.SleepCallCount, eventuallyTO).Should(Equal(897))
			Expect(fakeSleeper.SleepArgsForCall(0)).To(Equal(100 * time.Millisecond))
			Expect(fakeSleeper.SleepArgsForCall(25)).To(Equal(9539 * time.Millisecond))
			Expect(fakeSleeper.SleepArgsForCall(26)).To(Equal(10 * time.Second))
			Expect(fakeSleeper.SleepArgsForCall(27)).To(Equal(10 * time.Second))
			Expect(fakeSleeper.SleepArgsForCall(298)).To(Equal(10 * time.Second))
			Expect(fakeSleeper.SleepArgsForCall(299)).To(Equal(100 * time.Millisecond))
			Expect(fakeSleeper.SleepArgsForCall(2*299 - 1)).To(Equal(10 * time.Second))
			Expect(fakeSleeper.SleepArgsForCall(2 * 299)).To(Equal(100 * time.Millisecond))
			Expect(fakeSleeper.SleepArgsForCall(3*299 - 1)).To(Equal(10 * time.Second))

			Expect(fakeDurationExceededHandler.DurationExceededHandlerCallCount()).Should(Equal(0))
		})
	})

	When("the consecutive errors are unbounded and the peer is static leader", func() {
		BeforeEach(func() {
			fakeDeliverStreamer.DeliverReturns(nil, fmt.Errorf("deliver-error"))
			fakeDeliverStreamer.DeliverReturnsOnCall(500, fakeDeliverClient, nil)
		})

		It("hits the maximum sleep time value in an exponential fashion and retries indefinitely", func() {
			Eventually(fakeSleeper.SleepCallCount, eventuallyTO).Should(Equal(500))
			Expect(fakeSleeper.SleepArgsForCall(25)).To(Equal(9539 * time.Millisecond))
			Expect(fakeSleeper.SleepArgsForCall(26)).To(Equal(10 * time.Second))
			Expect(fakeSleeper.SleepArgsForCall(27)).To(Equal(10 * time.Second))
			Expect(fakeSleeper.SleepArgsForCall(499)).To(Equal(10 * time.Second))
			Eventually(fakeDurationExceededHandler.DurationExceededHandlerCallCount, eventuallyTO).Should(Equal(120))
		})
	})

	When("an error occurs, then a block is successfully delivered", func() {
		BeforeEach(func() {
			fakeDeliverStreamer.DeliverReturnsOnCall(0, nil, fmt.Errorf("deliver-error"))
			fakeDeliverStreamer.DeliverReturnsOnCall(1, nil, fmt.Errorf("deliver-error"))
			fakeDeliverStreamer.DeliverReturnsOnCall(2, nil, fmt.Errorf("deliver-error"))
			fakeDeliverStreamer.DeliverReturnsOnCall(3, fakeDeliverClient, nil)
		})

		It("sleeps in an exponential fashion and retries until dial is successful", func() {
			Eventually(fakeDeliverStreamer.DeliverCallCount, eventuallyTO).Should(Equal(4))
			Expect(fakeSleeper.SleepCallCount()).To(Equal(3))
			Expect(fakeSleeper.SleepArgsForCall(0)).To(Equal(100 * time.Millisecond))
			Expect(fakeSleeper.SleepArgsForCall(1)).To(Equal(120 * time.Millisecond))
			Expect(fakeSleeper.SleepArgsForCall(2)).To(Equal(144 * time.Millisecond))
		})
	})

	It("sends a request to the deliver client for new blocks", func() {
		Eventually(fakeDeliverClient.SendCallCount, eventuallyTO).Should(Equal(1))
		mutex.Lock()
		defer mutex.Unlock()
		Expect(len(ccs)).To(Equal(1))
	})

	When("the send fails", func() {
		BeforeEach(func() {
			fakeDeliverClient.SendReturnsOnCall(0, fmt.Errorf("fake-send-error"))
			fakeDeliverClient.SendReturnsOnCall(1, nil)
			fakeDeliverClient.CloseSendStub = nil
		})

		It("disconnects, sleeps and retries until the send is successful", func() {
			Eventually(fakeDeliverClient.SendCallCount, eventuallyTO).Should(Equal(2))
			Expect(fakeDeliverClient.CloseSendCallCount()).To(Equal(1))
			Expect(fakeSleeper.SleepCallCount()).To(Equal(1))
			Expect(fakeSleeper.SleepArgsForCall(0)).To(Equal(100 * time.Millisecond))
			mutex.Lock()
			defer mutex.Unlock()
			Expect(len(ccs)).To(Equal(2))
			Eventually(ccs[0].GetState, eventuallyTO).Should(Equal(connectivity.Shutdown))
		})
	})

	It("attempts to read blocks from the deliver stream", func() {
		Eventually(fakeDeliverClient.RecvCallCount, eventuallyTO).Should(Equal(1))
	})

	When("reading blocks from the deliver stream fails", func() {
		BeforeEach(func() {
			// appease the race detector
			doneC := doneC
			recvStep := recvStep
			fakeDeliverClient := fakeDeliverClient

			fakeDeliverClient.CloseSendStub = nil
			fakeDeliverClient.RecvStub = func() (*orderer.DeliverResponse, error) {
				if fakeDeliverClient.RecvCallCount() == 1 {
					return nil, fmt.Errorf("fake-recv-error")
				}
				select {
				case <-recvStep:
					return nil, fmt.Errorf("fake-recv-step-error")
				case <-doneC:
					return nil, nil
				}
			}
		})

		It("disconnects, sleeps, and retries until the recv is successful", func() {
			Eventually(fakeDeliverClient.RecvCallCount, eventuallyTO).Should(Equal(2))
			Expect(fakeSleeper.SleepCallCount()).To(Equal(1))
			Expect(fakeSleeper.SleepArgsForCall(0)).To(Equal(100 * time.Millisecond))
		})
	})

	When("reading blocks from the deliver stream fails and then recovers", func() {
		BeforeEach(func() {
			// appease the race detector
			doneC := doneC
			recvStep := recvStep
			fakeDeliverClient := fakeDeliverClient

			fakeDeliverClient.CloseSendStub = func() error {
				if fakeDeliverClient.CloseSendCallCount() >= 5 {
					select {
					case <-doneC:
					case recvStep <- struct{}{}:
					}
				}
				return nil
			}
			fakeDeliverClient.RecvStub = func() (*orderer.DeliverResponse, error) {
				switch fakeDeliverClient.RecvCallCount() {
				case 1, 2, 4:
					return nil, fmt.Errorf("fake-recv-error")
				case 3:
					return &orderer.DeliverResponse{
						Type: &orderer.DeliverResponse_Block{
							Block: &common.Block{
								Header: &common.BlockHeader{
									Number: 8,
								},
							},
						},
					}, nil
				default:
					select {
					case <-recvStep:
						return nil, fmt.Errorf("fake-recv-step-error")
					case <-doneC:
						return nil, nil
					}
				}
			}
		})

		It("disconnects, sleeps, and retries until the recv is successful and resets the failure count", func() {
			Eventually(fakeDeliverClient.RecvCallCount, eventuallyTO).Should(Equal(5))
			Expect(fakeSleeper.SleepCallCount()).To(Equal(3))
			Expect(fakeSleeper.SleepArgsForCall(0)).To(Equal(100 * time.Millisecond))
			Expect(fakeSleeper.SleepArgsForCall(1)).To(Equal(120 * time.Millisecond))
			Expect(fakeSleeper.SleepArgsForCall(2)).To(Equal(100 * time.Millisecond))
		})
	})

	When("the deliver client returns a block", func() {
		BeforeEach(func() {
			// appease the race detector
			doneC := doneC
			recvStep := recvStep
			fakeDeliverClient := fakeDeliverClient

			fakeDeliverClient.RecvStub = func() (*orderer.DeliverResponse, error) {
				if fakeDeliverClient.RecvCallCount() == 1 {
					return &orderer.DeliverResponse{
						Type: &orderer.DeliverResponse_Block{
							Block: &common.Block{
								Header: &common.BlockHeader{
									Number: 8,
								},
							},
						},
					}, nil
				}
				select {
				case <-recvStep:
					return nil, fmt.Errorf("fake-recv-step-error")
				case <-doneC:
					return nil, nil
				}
			}
		})

		It("receives the block and loops, not sleeping", func() {
			Eventually(fakeDeliverClient.RecvCallCount, eventuallyTO).Should(Equal(2))
			Expect(fakeSleeper.SleepCallCount()).To(Equal(0))
		})

		It("checks the validity of the block", func() {
			Eventually(fakeUpdatableBlockVerifier.VerifyBlockCallCount, eventuallyTO).Should(Equal(1))
			block := fakeUpdatableBlockVerifier.VerifyBlockArgsForCall(0)
			Expect(block).To(ProtoEqual(&common.Block{
				Header: &common.BlockHeader{
					Number: 8,
				},
			}))
		})

		When("the block is invalid", func() {
			BeforeEach(func() {
				fakeUpdatableBlockVerifier.VerifyBlockReturns(fmt.Errorf("fake-verify-error"))
			})

			It("disconnects, sleeps, and tries again", func() {
				Eventually(fakeSleeper.SleepCallCount, eventuallyTO).Should(Equal(1))
				Expect(fakeDeliverClient.CloseSendCallCount()).To(Equal(1))
				mutex.Lock()
				defer mutex.Unlock()
				Expect(len(ccs)).To(Equal(2))
			})
		})

		When("the block is valid", func() {
			It("handle the block", func() {
				Eventually(fakeBlockHandler.HandleBlockCallCount, eventuallyTO).Should(Equal(1))
				channelID, block := fakeBlockHandler.HandleBlockArgsForCall(0)
				Expect(channelID).To(Equal("channel-id"))
				Expect(block).To(Equal(&common.Block{
					Header: &common.BlockHeader{
						Number: 8,
					},
				},
				))
			})
		})

		When("handling the block fails", func() {
			BeforeEach(func() {
				fakeBlockHandler.HandleBlockReturns(fmt.Errorf("payload-error"))
			})

			It("disconnects, sleeps, and tries again", func() {
				Eventually(fakeSleeper.SleepCallCount, eventuallyTO).Should(Equal(1))
				Expect(fakeDeliverClient.CloseSendCallCount()).To(Equal(1))
				mutex.Lock()
				defer mutex.Unlock()
				Expect(len(ccs)).To(Equal(2))
			})
		})
	})

	When("the deliver client returns a config block", func() {
		var env *common.Envelope

		BeforeEach(func() {
			// appease the race detector
			doneC := doneC
			recvStep := recvStep
			fakeDeliverClient := fakeDeliverClient
			env = &common.Envelope{
				Payload: protoutil.MarshalOrPanic(&common.Payload{
					Header: &common.Header{
						ChannelHeader: protoutil.MarshalOrPanic(&common.ChannelHeader{
							Type:      int32(common.HeaderType_CONFIG),
							ChannelId: "test-chain",
						}),
					},
					Data: protoutil.MarshalOrPanic(&common.ConfigEnvelope{
						Config: channelConfig, // it must be a legal config that can produce a new bundle
					}),
				}),
			}

			configBlock := &common.Block{
				Header: &common.BlockHeader{Number: 8},
				Data: &common.BlockData{
					Data: [][]byte{protoutil.MarshalOrPanic(env)},
				},
			}

			fakeDeliverClient.RecvStub = func() (*orderer.DeliverResponse, error) {
				if fakeDeliverClient.RecvCallCount() == 1 {
					return &orderer.DeliverResponse{
						Type: &orderer.DeliverResponse_Block{
							Block: configBlock,
						},
					}, nil
				}
				select {
				case <-recvStep:
					return nil, fmt.Errorf("fake-recv-step-error")
				case <-doneC:
					return nil, nil
				}
			}
		})

		It("receives the block and loops, not sleeping", func() {
			Eventually(fakeDeliverClient.RecvCallCount, eventuallyTO).Should(Equal(2))
			Expect(fakeSleeper.SleepCallCount()).To(Equal(0))
		})

		It("checks the validity of the block", func() {
			Eventually(fakeUpdatableBlockVerifier.VerifyBlockCallCount, eventuallyTO).Should(Equal(1))
			block := fakeUpdatableBlockVerifier.VerifyBlockArgsForCall(0)
			Expect(block).To(ProtoEqual(&common.Block{
				Header: &common.BlockHeader{Number: 8},
				Data: &common.BlockData{
					Data: [][]byte{protoutil.MarshalOrPanic(env)},
				},
			}))
		})

		It("handle the block and updates the verifier config", func() {
			Eventually(fakeBlockHandler.HandleBlockCallCount, eventuallyTO).Should(Equal(1))
			channelID, block := fakeBlockHandler.HandleBlockArgsForCall(0)
			Expect(channelID).To(Equal("channel-id"))
			Expect(block).To(ProtoEqual(&common.Block{
				Header: &common.BlockHeader{Number: 8},
				Data: &common.BlockData{
					Data: [][]byte{protoutil.MarshalOrPanic(env)},
				},
			},
			))
			Eventually(fakeUpdatableBlockVerifier.VerifyBlockCallCount, eventuallyTO).Should(Equal(1))
		})

		It("updates the orderer connection source", func() {
			Eventually(fakeOrdererConnectionSource.UpdateCallCount, eventuallyTO).Should(Equal(1))
			globalAddresses, orgsAddresses := fakeOrdererConnectionSource.UpdateArgsForCall(0)
			Expect(globalAddresses).To(BeNil())
			Expect(orgsAddresses).ToNot(BeNil())
			Expect(len(orgsAddresses)).To(Equal(1))
			orgAddr, ok := orgsAddresses["SampleOrg"]
			Expect(ok).To(BeTrue())
			Expect(orgAddr.Addresses).To(Equal([]string{"127.0.0.1:7050", "127.0.0.1:7051", "127.0.0.1:7052"}))
			Expect(len(orgAddr.RootCerts)).To(Equal(2))
		})
	})

	When("the deliver client returns a status", func() {
		var status common.Status

		BeforeEach(func() {
			// appease the race detector
			doneC := doneC
			recvStep := recvStep
			fakeDeliverClient := fakeDeliverClient

			status = common.Status_SUCCESS
			fakeDeliverClient.RecvStub = func() (*orderer.DeliverResponse, error) {
				if fakeDeliverClient.RecvCallCount() == 1 {
					return &orderer.DeliverResponse{
						Type: &orderer.DeliverResponse_Status{
							Status: status,
						},
					}, nil
				}
				select {
				case <-recvStep:
					return nil, fmt.Errorf("fake-recv-step-error")
				case <-doneC:
					return nil, nil
				}
			}
		})

		It("disconnects with an error, and sleeps because the block request is infinite and should never complete", func() {
			Eventually(fakeSleeper.SleepCallCount, eventuallyTO).Should(Equal(1))
		})

		When("the status is not successful", func() {
			BeforeEach(func() {
				status = common.Status_FORBIDDEN
			})

			It("still disconnects with an error", func() {
				Eventually(fakeSleeper.SleepCallCount, eventuallyTO).Should(Equal(1))
			})
		})
	})
})

func testSetup(certDir string, consensusClass string) (*common.Config, bccsp.BCCSP, error) {
	var configProfile *genesisconfig.Profile
	tlsCA, err := tlsgen.NewCA()
	if err != nil {
		return nil, nil, err
	}

	switch consensusClass {
	case "CFT":
		configProfile = genesisconfig.Load(genesisconfig.SampleAppChannelEtcdRaftProfile, configtest.GetDevConfigDir())
		err = generateCertificates(configProfile, tlsCA, certDir)
	case "BFT":
		configProfile = genesisconfig.Load(genesisconfig.SampleAppChannelSmartBftProfile, configtest.GetDevConfigDir())
		err = generateCertificatesSmartBFT(configProfile, tlsCA, certDir)
	default:
		err = fmt.Errorf("expected CFT or BFT")
	}

	if err != nil {
		return nil, nil, err
	}

	bootstrapper, err := encoder.NewBootstrapper(configProfile)
	if err != nil {
		return nil, nil, err
	}
	channelConfigProto := &common.Config{ChannelGroup: bootstrapper.GenesisChannelGroup()}
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	if err != nil {
		return nil, nil, err
	}

	return channelConfigProto, cryptoProvider, nil
}

// TODO this pattern repeats itself in several places. Make it common in the 'genesisconfig' package to easily create
// Raft genesis blocks
func generateCertificates(confAppRaft *genesisconfig.Profile, tlsCA tlsgen.CA, certDir string) error {
	for i, c := range confAppRaft.Orderer.EtcdRaft.Consenters {
		srvC, err := tlsCA.NewServerCertKeyPair(c.Host)
		if err != nil {
			return err
		}
		srvP := path.Join(certDir, fmt.Sprintf("server%d.crt", i))
		err = os.WriteFile(srvP, srvC.Cert, 0o644)
		if err != nil {
			return err
		}

		clnC, err := tlsCA.NewClientCertKeyPair()
		if err != nil {
			return err
		}
		clnP := path.Join(certDir, fmt.Sprintf("client%d.crt", i))
		err = os.WriteFile(clnP, clnC.Cert, 0o644)
		if err != nil {
			return err
		}

		c.ServerTlsCert = []byte(srvP)
		c.ClientTlsCert = []byte(clnP)
	}

	return nil
}

func generateCertificatesSmartBFT(confAppSmartBFT *genesisconfig.Profile, tlsCA tlsgen.CA, certDir string) error {
	for i, c := range confAppSmartBFT.Orderer.ConsenterMapping {
		srvC, err := tlsCA.NewServerCertKeyPair(c.Host)
		if err != nil {
			return err
		}

		srvP := path.Join(certDir, fmt.Sprintf("server%d.crt", i))
		err = os.WriteFile(srvP, srvC.Cert, 0o644)
		if err != nil {
			return err
		}

		clnC, err := tlsCA.NewClientCertKeyPair()
		if err != nil {
			return err
		}

		clnP := path.Join(certDir, fmt.Sprintf("client%d.crt", i))
		err = os.WriteFile(clnP, clnC.Cert, 0o644)
		if err != nil {
			return err
		}

		c.Identity = srvP
		c.ServerTLSCert = srvP
		c.ClientTLSCert = clnP
	}

	return nil
}
