/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package etcdraft

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/cluster/mocks"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/onsi/gomega"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"go.etcd.io/etcd/raft/v3/raftpb"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestPeriodicCheck(t *testing.T) {
	t.Parallel()

	g := gomega.NewGomegaWithT(t)

	var cond uint32
	var checkNum uint32

	fiveChecks := func() bool {
		return atomic.LoadUint32(&checkNum) > uint32(5)
	}

	condition := func() bool {
		atomic.AddUint32(&checkNum, 1)
		return atomic.LoadUint32(&cond) == uint32(1)
	}

	reports := make(chan time.Duration, 1000)

	report := func(duration time.Duration) {
		reports <- duration
	}

	clears := make(chan struct{}, 1000)

	reportCleared := func() {
		clears <- struct{}{}
	}

	check := &PeriodicCheck{
		Logger:        flogging.MustGetLogger("test"),
		Condition:     condition,
		CheckInterval: time.Millisecond,
		Report:        report,
		ReportCleared: reportCleared,
	}

	go check.Run()

	g.Eventually(fiveChecks, time.Minute, time.Millisecond).Should(gomega.BeTrue())
	// trigger condition to be true
	atomic.StoreUint32(&cond, 1)
	g.Eventually(reports, time.Minute, time.Millisecond).Should(gomega.Not(gomega.BeEmpty()))
	// read first report
	firstReport := <-reports
	g.Eventually(reports, time.Minute, time.Millisecond).Should(gomega.Not(gomega.BeEmpty()))
	// read second report
	secondReport := <-reports
	// time increases between reports
	g.Expect(secondReport).To(gomega.BeNumerically(">", firstReport))
	// wait for the reports channel to be full
	g.Eventually(func() int { return len(reports) }, time.Minute, time.Millisecond).Should(gomega.BeNumerically("==", 1000))

	// trigger condition to be false
	atomic.StoreUint32(&cond, 0)

	var lastReport time.Duration
	// drain the reports channel
	for len(reports) > 0 {
		select {
		case report := <-reports:
			lastReport = report
		default:
		}
	}

	g.Eventually(clears).Should(gomega.Receive())
	g.Consistently(clears).ShouldNot(gomega.Receive())

	// ensure the checks have been made
	checksDoneSoFar := atomic.LoadUint32(&checkNum)
	g.Consistently(reports, time.Second*2, time.Millisecond).Should(gomega.BeEmpty())
	checksDoneAfter := atomic.LoadUint32(&checkNum)
	g.Expect(checksDoneAfter).To(gomega.BeNumerically(">", checksDoneSoFar))
	// but nothing has been reported
	g.Expect(reports).To(gomega.BeEmpty())

	// trigger the condition again
	atomic.StoreUint32(&cond, 1)
	g.Eventually(reports, time.Minute, time.Millisecond).Should(gomega.Not(gomega.BeEmpty()))
	// The first report is smaller than the last report,
	// so the countdown has been reset when the condition was reset
	firstReport = <-reports
	g.Expect(lastReport).To(gomega.BeNumerically(">", firstReport))
	// Stop the periodic check.
	check.Stop()
	checkCountAfterStop := atomic.LoadUint32(&checkNum)
	// Wait 50 times the check interval.
	time.Sleep(check.CheckInterval * 50)
	// Ensure that we cease checking the condition, hence the PeriodicCheck is stopped.
	g.Expect(atomic.LoadUint32(&checkNum)).To(gomega.BeNumerically("<", checkCountAfterStop+2))
	g.Consistently(clears).ShouldNot(gomega.Receive())
}

func TestEvictionSuspector(t *testing.T) {
	configBlock := &common.Block{
		Header: &common.BlockHeader{Number: 9},
		Metadata: &common.BlockMetadata{
			Metadata: [][]byte{{}, {}, {}, {}},
		},
	}
	configBlock.Metadata.Metadata[common.BlockMetadataIndex_SIGNATURES] = protoutil.MarshalOrPanic(&common.Metadata{
		Value: protoutil.MarshalOrPanic(&common.OrdererBlockMetadata{
			LastConfig: &common.LastConfig{Index: 9},
		}),
	})

	puller := &mocks.ChainPuller{}
	puller.On("Close")
	puller.On("HeightsByEndpoints").Return(map[string]uint64{"foo": 10}, nil)
	puller.On("PullBlock", uint64(9)).Return(configBlock)

	for _, testCase := range []struct {
		description                 string
		expectedPanic               string
		expectedLog                 string
		expectedCommittedBlockCount int
		amIInChannelReturns         error
		evictionSuspicionThreshold  time.Duration
		blockPuller                 BlockPuller
		blockPullerErr              error
		height                      uint64
		timesTriggered              int
		halt                        func()
	}{
		{
			description:                "suspected time is lower than threshold",
			evictionSuspicionThreshold: 11 * time.Minute,
			halt:                       t.Fail,
		},
		{
			description:                "timesTriggered multiplier prevents threshold",
			evictionSuspicionThreshold: 6 * time.Minute,
			timesTriggered:             1,
			halt:                       t.Fail,
		},
		{
			description:                "puller creation fails",
			evictionSuspicionThreshold: 10*time.Minute - time.Second,
			blockPullerErr:             errors.New("oops"),
			expectedPanic:              "Failed creating a block puller: oops",
			halt:                       t.Fail,
		},
		{
			description:                "failed pulling the block",
			expectedLog:                "Cannot confirm our own eviction from the channel: bad block",
			evictionSuspicionThreshold: 10*time.Minute - time.Second,
			amIInChannelReturns:        errors.New("bad block"),
			blockPuller:                puller,
			height:                     9,
			halt:                       t.Fail,
		},
		{
			description:                "we are still in the channel",
			expectedLog:                "Cannot confirm our own eviction from the channel, our certificate was found in config block with sequence 9",
			evictionSuspicionThreshold: 10*time.Minute - time.Second,
			amIInChannelReturns:        nil,
			blockPuller:                puller,
			height:                     9,
			halt:                       t.Fail,
		},
		{
			description:                "our height is the highest",
			expectedLog:                "Our height is higher or equal than the height of the orderer we pulled the last block from, aborting",
			evictionSuspicionThreshold: 10*time.Minute - time.Second,
			amIInChannelReturns:        cluster.ErrNotInChannel,
			blockPuller:                puller,
			height:                     10,
			halt:                       func() {},
		},
		{
			description:                 "we are not in the channel",
			expectedLog:                 "Detected our own eviction from the channel in block [9]",
			evictionSuspicionThreshold:  10*time.Minute - time.Second,
			amIInChannelReturns:         cluster.ErrNotInChannel,
			blockPuller:                 puller,
			height:                      8,
			expectedCommittedBlockCount: 2,
			halt: func() {
				puller.On("PullBlock", uint64(8)).Return(&common.Block{
					Header: &common.BlockHeader{Number: 8},
					Metadata: &common.BlockMetadata{
						Metadata: [][]byte{{}, {}, {}, {}},
					},
				})
			},
		},
	} {
		testCase := testCase
		t.Run(testCase.description, func(t *testing.T) {
			committedBlocks := make(chan *common.Block, 2)

			commitBlock := func(block *common.Block) error {
				committedBlocks <- block
				return nil
			}

			es := &evictionSuspector{
				halt: testCase.halt,
				amIInChannel: func(_ *common.Block) error {
					return testCase.amIInChannelReturns
				},
				evictionSuspicionThreshold: testCase.evictionSuspicionThreshold,
				createPuller: func() (BlockPuller, error) {
					return testCase.blockPuller, testCase.blockPullerErr
				},
				writeBlock: commitBlock,
				height: func() uint64 {
					return testCase.height
				},
				logger:         flogging.MustGetLogger("test"),
				triggerCatchUp: func(sn *raftpb.Snapshot) {},
				timesTriggered: testCase.timesTriggered,
			}

			foundExpectedLog := testCase.expectedLog == ""
			es.logger = es.logger.WithOptions(zap.Hooks(func(entry zapcore.Entry) error {
				if strings.Contains(entry.Message, testCase.expectedLog) {
					foundExpectedLog = true
				}
				return nil
			}))

			runTestCase := func() {
				es.confirmSuspicion(time.Minute * 10)
			}

			if testCase.expectedPanic != "" {
				require.PanicsWithValue(t, testCase.expectedPanic, runTestCase)
			} else {
				runTestCase()
				// Run the test case again.
				// Conditions that do not lead to a conclusion of a chain eviction
				// should be idempotent.
				// Conditions that do lead to conclusion of a chain eviction
				// in the second time - should result in a no-op.
				runTestCase()
			}

			require.True(t, foundExpectedLog, "expected to find %s but didn't", testCase.expectedLog)
			require.Equal(t, testCase.expectedCommittedBlockCount, len(committedBlocks))
		})
	}
}
