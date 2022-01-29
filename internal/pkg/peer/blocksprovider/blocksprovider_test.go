/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blocksprovider_test

import (
	"fmt"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/gossip"
	"github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/common/flogging"
	gossipcommon "github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/internal/pkg/peer/blocksprovider"
	"github.com/hyperledger/fabric/internal/pkg/peer/blocksprovider/fake"
	"github.com/hyperledger/fabric/internal/pkg/peer/orderers"
	"github.com/hyperledger/fabric/protoutil"

	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

var _ = Describe("Blocksprovider", func() {
	var (
		d                           *blocksprovider.Deliverer
		ccs                         []*grpc.ClientConn
		fakeDialer                  *fake.Dialer
		fakeGossipServiceAdapter    *fake.GossipServiceAdapter
		fakeOrdererConnectionSource *fake.OrdererConnectionSource
		fakeLedgerInfo              *fake.LedgerInfo
		fakeBlockVerifier           *fake.BlockVerifier
		fakeSigner                  *fake.Signer
		fakeDeliverStreamer         *fake.DeliverStreamer
		fakeDeliverClient           *fake.DeliverClient
		fakeSleeper                 *fake.Sleeper
		doneC                       chan struct{}
		recvStep                    chan struct{}
		endC                        chan struct{}
		mutex                       sync.Mutex
	)

	BeforeEach(func() {
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
			cc, err := grpc.Dial("", grpc.WithInsecure())
			ccs = append(ccs, cc)
			Expect(err).NotTo(HaveOccurred())
			Expect(cc.GetState()).NotTo(Equal(connectivity.Shutdown))
			return cc, nil
		}

		fakeGossipServiceAdapter = &fake.GossipServiceAdapter{}
		fakeBlockVerifier = &fake.BlockVerifier{}
		fakeSigner = &fake.Signer{}

		fakeLedgerInfo = &fake.LedgerInfo{}
		fakeLedgerInfo.LedgerHeightReturns(7, nil)

		fakeOrdererConnectionSource = &fake.OrdererConnectionSource{}
		fakeOrdererConnectionSource.RandomEndpointReturns(&orderers.Endpoint{
			Address: "orderer-address",
		}, nil)

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

		d = &blocksprovider.Deliverer{
			ChannelID:         "channel-id",
			Gossip:            fakeGossipServiceAdapter,
			Ledger:            fakeLedgerInfo,
			BlockVerifier:     fakeBlockVerifier,
			Dialer:            fakeDialer,
			Orderers:          fakeOrdererConnectionSource,
			DoneC:             doneC,
			Signer:            fakeSigner,
			DeliverStreamer:   fakeDeliverStreamer,
			Logger:            flogging.MustGetLogger("blocksprovider"),
			TLSCertHash:       []byte("tls-cert-hash"),
			MaxRetryDuration:  time.Hour,
			MaxRetryDelay:     10 * time.Second,
			InitialRetryDelay: 100 * time.Millisecond,
		}

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
		close(doneC)
		<-endC
	})

	It("waits patiently for new blocks from the orderer", func() {
		Consistently(endC).ShouldNot(BeClosed())
		mutex.Lock()
		defer mutex.Unlock()
		Expect(ccs[0].GetState()).NotTo(Equal(connectivity.Shutdown))
	})

	It("checks the ledger height", func() {
		Eventually(fakeLedgerInfo.LedgerHeightCallCount).Should(Equal(1))
	})

	When("the ledger returns an error", func() {
		BeforeEach(func() {
			fakeLedgerInfo.LedgerHeightReturns(0, fmt.Errorf("fake-ledger-error"))
		})

		It("exits the loop", func() {
			Eventually(endC).Should(BeClosed())
		})
	})

	It("signs the seek info request", func() {
		Eventually(fakeSigner.SignCallCount).Should(Equal(1))
		// Note, the signer is used inside a util method
		// which has its own set of tests, so checking the args
		// in this test is unnecessary
	})

	When("the signer returns an error", func() {
		BeforeEach(func() {
			fakeSigner.SignReturns(nil, fmt.Errorf("fake-signer-error"))
		})

		It("exits the loop", func() {
			Eventually(endC).Should(BeClosed())
		})
	})

	It("gets a random endpoint to connect to from the orderer connection source", func() {
		Eventually(fakeOrdererConnectionSource.RandomEndpointCallCount).Should(Equal(1))
	})

	When("the orderer connection source returns an error", func() {
		BeforeEach(func() {
			fakeOrdererConnectionSource.RandomEndpointReturnsOnCall(0, nil, fmt.Errorf("fake-endpoint-error"))
			fakeOrdererConnectionSource.RandomEndpointReturnsOnCall(1, &orderers.Endpoint{
				Address: "orderer-address",
			}, nil)
		})

		It("sleeps and retries until a valid endpoint is selected", func() {
			Eventually(fakeOrdererConnectionSource.RandomEndpointCallCount).Should(Equal(2))
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
			Eventually(fakeOrdererConnectionSource.RandomEndpointCallCount).Should(Equal(2))
			Expect(fakeSleeper.SleepCallCount()).To(Equal(0))
		})
	})

	It("dials the random endpoint", func() {
		Eventually(fakeDialer.DialCallCount).Should(Equal(1))
		addr, tlsCerts := fakeDialer.DialArgsForCall(0)
		Expect(addr).To(Equal("orderer-address"))
		Expect(tlsCerts).To(BeNil()) // TODO
	})

	When("the dialer returns an error", func() {
		BeforeEach(func() {
			fakeDialer.DialReturnsOnCall(0, nil, fmt.Errorf("fake-dial-error"))
			cc, err := grpc.Dial("", grpc.WithInsecure())
			Expect(err).NotTo(HaveOccurred())
			fakeDialer.DialReturnsOnCall(1, cc, nil)
		})

		It("sleeps and retries until dial is successful", func() {
			Eventually(fakeDialer.DialCallCount).Should(Equal(2))
			Expect(fakeSleeper.SleepCallCount()).To(Equal(1))
			Expect(fakeSleeper.SleepArgsForCall(0)).To(Equal(100 * time.Millisecond))
		})
	})

	It("constructs a deliver client", func() {
		Eventually(fakeDeliverStreamer.DeliverCallCount).Should(Equal(1))
	})

	When("the deliver client cannot be created", func() {
		BeforeEach(func() {
			fakeDeliverStreamer.DeliverReturnsOnCall(0, nil, fmt.Errorf("deliver-error"))
			fakeDeliverStreamer.DeliverReturnsOnCall(1, fakeDeliverClient, nil)
		})

		It("closes the grpc connection, sleeps, and tries again", func() {
			Eventually(fakeDeliverStreamer.DeliverCallCount).Should(Equal(2))
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
			Eventually(fakeDeliverStreamer.DeliverCallCount).Should(Equal(4))
			Expect(fakeSleeper.SleepCallCount()).To(Equal(3))
			Expect(fakeSleeper.SleepArgsForCall(0)).To(Equal(100 * time.Millisecond))
			Expect(fakeSleeper.SleepArgsForCall(1)).To(Equal(120 * time.Millisecond))
			Expect(fakeSleeper.SleepArgsForCall(2)).To(Equal(144 * time.Millisecond))
		})
	})

	When("the consecutive errors are unbounded and the peer is not a static leader", func() {
		BeforeEach(func() {
			fakeDeliverStreamer.DeliverReturns(nil, fmt.Errorf("deliver-error"))
			fakeDeliverStreamer.DeliverReturnsOnCall(500, fakeDeliverClient, nil)
		})

		It("hits the maximum sleep time value in an exponential fashion and retries until exceeding the max retry duration", func() {
			d.YieldLeadership = true
			Eventually(fakeSleeper.SleepCallCount).Should(Equal(380))
			Expect(fakeSleeper.SleepArgsForCall(25)).To(Equal(9539 * time.Millisecond))
			Expect(fakeSleeper.SleepArgsForCall(26)).To(Equal(10 * time.Second))
			Expect(fakeSleeper.SleepArgsForCall(27)).To(Equal(10 * time.Second))
			Expect(fakeSleeper.SleepArgsForCall(379)).To(Equal(10 * time.Second))
		})
	})

	When("the consecutive errors are unbounded and the peer is static leader", func() {
		BeforeEach(func() {
			fakeDeliverStreamer.DeliverReturns(nil, fmt.Errorf("deliver-error"))
			fakeDeliverStreamer.DeliverReturnsOnCall(500, fakeDeliverClient, nil)
		})

		It("hits the maximum sleep time value in an exponential fashion and retries indefinitely", func() {
			d.YieldLeadership = false
			Eventually(fakeSleeper.SleepCallCount).Should(Equal(500))
			Expect(fakeSleeper.SleepArgsForCall(25)).To(Equal(9539 * time.Millisecond))
			Expect(fakeSleeper.SleepArgsForCall(26)).To(Equal(10 * time.Second))
			Expect(fakeSleeper.SleepArgsForCall(27)).To(Equal(10 * time.Second))
			Expect(fakeSleeper.SleepArgsForCall(379)).To(Equal(10 * time.Second))
		})
	})

	When("an error occurs, then a block is successfully delivered", func() {
		BeforeEach(func() {
			fakeDeliverStreamer.DeliverReturnsOnCall(0, nil, fmt.Errorf("deliver-error"))
			fakeDeliverStreamer.DeliverReturnsOnCall(1, fakeDeliverClient, nil)
			fakeDeliverStreamer.DeliverReturnsOnCall(1, nil, fmt.Errorf("deliver-error"))
			fakeDeliverStreamer.DeliverReturnsOnCall(2, nil, fmt.Errorf("deliver-error"))
		})

		It("sleeps in an exponential fashion and retries until dial is successful", func() {
			Eventually(fakeDeliverStreamer.DeliverCallCount).Should(Equal(4))
			Expect(fakeSleeper.SleepCallCount()).To(Equal(3))
			Expect(fakeSleeper.SleepArgsForCall(0)).To(Equal(100 * time.Millisecond))
			Expect(fakeSleeper.SleepArgsForCall(1)).To(Equal(120 * time.Millisecond))
			Expect(fakeSleeper.SleepArgsForCall(2)).To(Equal(144 * time.Millisecond))
		})
	})

	It("sends a request to the deliver client for new blocks", func() {
		Eventually(fakeDeliverClient.SendCallCount).Should(Equal(1))
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
			Eventually(fakeDeliverClient.SendCallCount).Should(Equal(2))
			Expect(fakeDeliverClient.CloseSendCallCount()).To(Equal(1))
			Expect(fakeSleeper.SleepCallCount()).To(Equal(1))
			Expect(fakeSleeper.SleepArgsForCall(0)).To(Equal(100 * time.Millisecond))
			mutex.Lock()
			defer mutex.Unlock()
			Expect(len(ccs)).To(Equal(2))
			Eventually(ccs[0].GetState).Should(Equal(connectivity.Shutdown))
		})
	})

	It("attempts to read blocks from the deliver stream", func() {
		Eventually(fakeDeliverClient.RecvCallCount).Should(Equal(1))
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
			Eventually(fakeDeliverClient.RecvCallCount).Should(Equal(2))
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
			Eventually(fakeDeliverClient.RecvCallCount).Should(Equal(5))
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
			Eventually(fakeDeliverClient.RecvCallCount).Should(Equal(2))
			Expect(fakeSleeper.SleepCallCount()).To(Equal(0))
		})

		It("checks the validity of the block", func() {
			Eventually(fakeBlockVerifier.VerifyBlockCallCount).Should(Equal(1))
			channelID, blockNum, block := fakeBlockVerifier.VerifyBlockArgsForCall(0)
			Expect(channelID).To(Equal(gossipcommon.ChannelID("channel-id")))
			Expect(blockNum).To(Equal(uint64(8)))
			Expect(proto.Equal(block, &common.Block{
				Header: &common.BlockHeader{
					Number: 8,
				},
			})).To(BeTrue())
		})

		When("the block is invalid", func() {
			BeforeEach(func() {
				fakeBlockVerifier.VerifyBlockReturns(fmt.Errorf("fake-verify-error"))
			})

			It("disconnects, sleeps, and tries again", func() {
				Eventually(fakeSleeper.SleepCallCount).Should(Equal(1))
				Expect(fakeDeliverClient.CloseSendCallCount()).To(Equal(1))
				mutex.Lock()
				defer mutex.Unlock()
				Expect(len(ccs)).To(Equal(2))
			})
		})

		It("adds the payload to gossip", func() {
			Eventually(fakeGossipServiceAdapter.AddPayloadCallCount).Should(Equal(1))
			channelID, payload := fakeGossipServiceAdapter.AddPayloadArgsForCall(0)
			Expect(channelID).To(Equal("channel-id"))
			Expect(payload).To(Equal(&gossip.Payload{
				Data: protoutil.MarshalOrPanic(&common.Block{
					Header: &common.BlockHeader{
						Number: 8,
					},
				}),
				SeqNum: 8,
			}))
		})

		When("adding the payload fails", func() {
			BeforeEach(func() {
				fakeGossipServiceAdapter.AddPayloadReturns(fmt.Errorf("payload-error"))
			})

			It("disconnects, sleeps, and tries again", func() {
				Eventually(fakeSleeper.SleepCallCount).Should(Equal(1))
				Expect(fakeDeliverClient.CloseSendCallCount()).To(Equal(1))
				mutex.Lock()
				defer mutex.Unlock()
				Expect(len(ccs)).To(Equal(2))
			})
		})

		It("gossips the block to the other peers", func() {
			Eventually(fakeGossipServiceAdapter.GossipCallCount).Should(Equal(1))
			msg := fakeGossipServiceAdapter.GossipArgsForCall(0)
			Expect(msg).To(Equal(&gossip.GossipMessage{
				Nonce:   0,
				Tag:     gossip.GossipMessage_CHAN_AND_ORG,
				Channel: []byte("channel-id"),
				Content: &gossip.GossipMessage_DataMsg{
					DataMsg: &gossip.DataMessage{
						Payload: &gossip.Payload{
							Data: protoutil.MarshalOrPanic(&common.Block{
								Header: &common.BlockHeader{
									Number: 8,
								},
							}),
							SeqNum: 8,
						},
					},
				},
			}))
		})

		When("gossip dissemination is disabled", func() {
			BeforeEach(func() {
				d.BlockGossipDisabled = true
			})

			It("doesn't gossip, only adds to the payload buffer", func() {
				Eventually(fakeGossipServiceAdapter.AddPayloadCallCount).Should(Equal(1))
				channelID, payload := fakeGossipServiceAdapter.AddPayloadArgsForCall(0)
				Expect(channelID).To(Equal("channel-id"))
				Expect(payload).To(Equal(&gossip.Payload{
					Data: protoutil.MarshalOrPanic(&common.Block{
						Header: &common.BlockHeader{
							Number: 8,
						},
					}),
					SeqNum: 8,
				}))

				Consistently(fakeGossipServiceAdapter.GossipCallCount).Should(Equal(0))
			})
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
			Eventually(fakeSleeper.SleepCallCount).Should(Equal(1))
		})

		When("the status is not successful", func() {
			BeforeEach(func() {
				status = common.Status_FORBIDDEN
			})

			It("still disconnects with an error", func() {
				Eventually(fakeSleeper.SleepCallCount).Should(Equal(1))
			})
		})
	})
})
