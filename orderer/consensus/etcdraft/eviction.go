/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package etcdraft

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/protoutil"
	"go.etcd.io/etcd/raft/v3/raftpb"
)

// PeriodicCheck checks periodically a condition, and reports
// the cumulative consecutive period the condition was fulfilled.
type PeriodicCheck struct {
	Logger              *flogging.FabricLogger
	CheckInterval       time.Duration
	Condition           func() bool
	Report              func(cumulativePeriod time.Duration)
	ReportCleared       func()
	conditionHoldsSince time.Time
	once                sync.Once // Used to prevent double initialization
	stopped             uint32
}

// Run runs the PeriodicCheck
func (pc *PeriodicCheck) Run() {
	pc.once.Do(pc.check)
}

// Stop stops the periodic checks
func (pc *PeriodicCheck) Stop() {
	pc.Logger.Info("Periodic check is stopping.")
	atomic.AddUint32(&pc.stopped, 1)
}

func (pc *PeriodicCheck) shouldRun() bool {
	return atomic.LoadUint32(&pc.stopped) == 0
}

func (pc *PeriodicCheck) check() {
	if pc.Condition() {
		pc.conditionFulfilled()
	} else {
		pc.conditionNotFulfilled()
	}

	if !pc.shouldRun() {
		return
	}
	time.AfterFunc(pc.CheckInterval, pc.check)
}

func (pc *PeriodicCheck) conditionNotFulfilled() {
	if pc.ReportCleared != nil && !pc.conditionHoldsSince.IsZero() {
		pc.ReportCleared()
	}
	pc.conditionHoldsSince = time.Time{}
}

func (pc *PeriodicCheck) conditionFulfilled() {
	if pc.conditionHoldsSince.IsZero() {
		pc.conditionHoldsSince = time.Now()
	}

	pc.Report(time.Since(pc.conditionHoldsSince))
}

// SelfMembershipPredicate determines whether the caller is found in the given config block
type SelfMembershipPredicate func(configBlock *common.Block) error

type evictionSuspector struct {
	evictionSuspicionThreshold time.Duration
	logger                     *flogging.FabricLogger
	createPuller               CreateBlockPuller
	height                     func() uint64
	amIInChannel               SelfMembershipPredicate
	halt                       func()
	writeBlock                 func(block *common.Block) error
	triggerCatchUp             func(sn *raftpb.Snapshot)
	halted                     bool
	timesTriggered             int
}

func (es *evictionSuspector) clearSuspicion() {
	es.timesTriggered = 0
}

func (es *evictionSuspector) confirmSuspicion(cumulativeSuspicion time.Duration) {
	// The goal here is to only execute the body of the function once every es.evictionSuspicionThreshold
	if es.evictionSuspicionThreshold*time.Duration(es.timesTriggered+1) > cumulativeSuspicion || es.halted {
		return
	}
	es.timesTriggered++

	es.logger.Infof("Suspecting our own eviction from the channel for %v", cumulativeSuspicion)
	puller, err := es.createPuller()
	if err != nil {
		es.logger.Panicf("Failed creating a block puller: %v", err)
	}

	lastConfigBlock, err := cluster.PullLastConfigBlock(puller)
	if err != nil {
		es.logger.Errorf("Failed pulling the last config block: %v", err)
		return
	}

	es.logger.Infof("Last config block was found to be block [%d]", lastConfigBlock.Header.Number)

	err = es.amIInChannel(lastConfigBlock)
	if err != cluster.ErrNotInChannel && err != cluster.ErrForbidden {
		details := fmt.Sprintf(", our certificate was found in config block with sequence %d", lastConfigBlock.Header.Number)
		if err != nil {
			details = fmt.Sprintf(": %s", err.Error())
		}
		es.logger.Infof("Cannot confirm our own eviction from the channel%s", details)

		es.triggerCatchUp(&raftpb.Snapshot{Data: protoutil.MarshalOrPanic(lastConfigBlock)})
		return
	}
	es.logger.Warningf("Detected our own eviction from the channel in block [%d]", lastConfigBlock.Header.Number)

	es.logger.Infof("Waiting for chain to halt")
	es.halt()
	es.halted = true

	height := es.height()
	if lastConfigBlock.Header.Number+1 <= height {
		es.logger.Infof("Our height is higher or equal than the height of the orderer we pulled the last block from, aborting.")
		return
	}

	es.logger.Infof("Chain has been halted, pulling remaining blocks up to (and including) eviction block.")

	nextBlock := height
	es.logger.Infof("Will now pull blocks %d to %d", nextBlock, lastConfigBlock.Header.Number)
	for seq := nextBlock; seq <= lastConfigBlock.Header.Number; seq++ {
		es.logger.Infof("Pulling block [%d]", seq)
		block := puller.PullBlock(seq)
		err := es.writeBlock(block)
		if err != nil {
			es.logger.Panicf("Failed writing block [%d] to the ledger: %v", block.Header.Number, err)
		}
	}

	es.logger.Infof("Pulled all blocks up to eviction block.")
}
