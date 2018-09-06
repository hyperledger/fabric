/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package etcdraft_test

import (
	"time"

	"github.com/hyperledger/fabric/common/flogging"
	mockconfig "github.com/hyperledger/fabric/common/mocks/config"
	"github.com/hyperledger/fabric/orderer/consensus/etcdraft"
	consensusmocks "github.com/hyperledger/fabric/orderer/consensus/mocks"
	mockblockcutter "github.com/hyperledger/fabric/orderer/mocks/common/blockcutter"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"

	"code.cloudfoundry.org/clock/fakeclock"
	"github.com/coreos/etcd/raft"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
)

var _ = Describe("Chain", func() {
	var (
		m           *common.Envelope
		normalBlock *common.Block
		interval    time.Duration
		channelID   string
	)

	BeforeEach(func() {
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
			clock   *fakeclock.FakeClock
			opt     etcdraft.Options
			support *consensusmocks.FakeConsenterSupport
			cutter  *mockblockcutter.Receiver
			storage *raft.MemoryStorage
			observe chan uint64
			chain   *etcdraft.Chain
			logger  *flogging.FabricLogger
		)

		campaign := func() {
			clock.Increment(interval)
			Consistently(observe).ShouldNot(Receive())

			clock.Increment(interval)
			// raft election timeout is randomized in
			// [electiontimeout, 2 * electiontimeout - 1]
			// So we may need one extra tick to trigger
			// leader election.
			clock.Increment(interval)
			Eventually(observe).Should(Receive())
		}

		BeforeEach(func() {
			clock = fakeclock.NewFakeClock(time.Now())
			storage = raft.NewMemoryStorage()
			logger = flogging.NewFabricLogger(zap.NewNop())
			observe = make(chan uint64, 1)
			opt = etcdraft.Options{
				RaftID:          uint64(1),
				Clock:           clock,
				TickInterval:    interval,
				ElectionTick:    2,
				HeartbeatTick:   1,
				MaxSizePerMsg:   1024 * 1024,
				MaxInflightMsgs: 256,
				Peers:           []raft.Peer{{ID: uint64(1)}},
				Logger:          logger,
				Storage:         storage,
			}
			support = &consensusmocks.FakeConsenterSupport{}
			support.ChainIDReturns(channelID)
			support.SharedConfigReturns(&mockconfig.Orderer{BatchTimeoutVal: time.Hour})
			cutter = mockblockcutter.NewReceiver()
			support.BlockCutterReturns(cutter)

			var err error
			chain, err = etcdraft.NewChain(support, opt, observe)
			Expect(err).NotTo(HaveOccurred())
			chain.Start()

			// When raft node bootstraps, it produces a ConfChange
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
				_, err := storage.Entries(1, 1, 1)
				return err
			}).ShouldNot(HaveOccurred())
		})

		AfterEach(func() {
			chain.Halt()
		})

		Context("when no raft leader is elected", func() {
			It("fails to order envelope", func() {
				err := chain.Order(m, uint64(0))
				Expect(err).To(MatchError("no raft leader"))
			})
		})

		Context("when raft leader is elected", func() {
			BeforeEach(func() {
				campaign()
			})

			It("fails to order envelope if chain is halted", func() {
				chain.Halt()
				err := chain.Order(m, uint64(0))
				Expect(err).To(MatchError("chain is stopped"))
			})

			It("produces blocks following batch rules", func() {
				close(cutter.Block)
				support.CreateNextBlockReturns(normalBlock)

				By("cutting next batch directly")
				cutter.CutNext = true
				err := chain.Order(m, uint64(0))
				Expect(err).NotTo(HaveOccurred())
				Eventually(support.WriteBlockCallCount).Should(Equal(1))

				By("respecting batch timeout")
				cutter.CutNext = false
				timeout := time.Second
				support.SharedConfigReturns(&mockconfig.Orderer{BatchTimeoutVal: timeout})
				err = chain.Order(m, uint64(0))
				Expect(err).NotTo(HaveOccurred())

				clock.WaitForNWatchersAndIncrement(timeout, 2)
				Eventually(support.WriteBlockCallCount).Should(Equal(2))
			})
		})
	})
})
