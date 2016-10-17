/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package executor

import (
	"github.com/hyperledger/fabric/consensus"
	"github.com/hyperledger/fabric/consensus/util/events"
	"github.com/hyperledger/fabric/core/peer/statetransfer"
	pb "github.com/hyperledger/fabric/protos"

	"github.com/op/go-logging"
)

var logger *logging.Logger // package-level logger

func init() {
	logger = logging.MustGetLogger("consensus/executor")
}

// PartialStack contains the ledger features required by the executor.Coordinator
type PartialStack interface {
	consensus.LegacyExecutor
	GetBlockchainInfo() *pb.BlockchainInfo
}

type coordinatorImpl struct {
	manager         events.Manager              // Maintains event thread and sends events to the coordinator
	rawExecutor     PartialStack                // Does the real interaction with the ledger
	consumer        consensus.ExecutionConsumer // The consumer of this coordinator which receives the callbacks
	stc             statetransfer.Coordinator   // State transfer instance
	batchInProgress bool                        // Are we mid execution batch
	skipInProgress  bool                        // Are we mid state transfer
}

// NewCoordinatorImpl creates a new executor.Coordinator
func NewImpl(consumer consensus.ExecutionConsumer, rawExecutor PartialStack, stps statetransfer.PartialStack) consensus.Executor {
	co := &coordinatorImpl{
		rawExecutor: rawExecutor,
		consumer:    consumer,
		stc:         statetransfer.NewCoordinatorImpl(stps),
		manager:     events.NewManagerImpl(),
	}
	co.manager.SetReceiver(co)
	return co
}

// ProcessEvent is the main event loop for the executor.Coordinator
func (co *coordinatorImpl) ProcessEvent(event events.Event) events.Event {
	switch et := event.(type) {
	case executeEvent:
		logger.Debug("Executor is processing an executeEvent")
		if co.skipInProgress {
			logger.Error("FATAL programming error, attempted to execute a transaction during state transfer")
			return nil
		}

		if !co.batchInProgress {
			logger.Debug("Starting new transaction batch")
			co.batchInProgress = true
			err := co.rawExecutor.BeginTxBatch(co)
			_ = err // TODO This should probably panic, see issue 752
		}

		co.rawExecutor.ExecTxs(co, et.txs)

		co.consumer.Executed(et.tag)
	case commitEvent:
		logger.Debug("Executor is processing an commitEvent")
		if co.skipInProgress {
			logger.Error("Likely FATAL programming error, attempted to commit a transaction batch during state transfer")
			return nil
		}

		if !co.batchInProgress {
			logger.Error("Likely FATAL programming error, attemted to commit a transaction batch when one does not exist")
			return nil
		}

		_, err := co.rawExecutor.CommitTxBatch(co, et.metadata)
		_ = err // TODO This should probably panic, see issue 752

		co.batchInProgress = false

		info := co.rawExecutor.GetBlockchainInfo()

		logger.Debugf("Committed block %d with hash %x to chain", info.Height-1, info.CurrentBlockHash)

		co.consumer.Committed(et.tag, info)
	case rollbackEvent:
		logger.Debug("Executor is processing an rollbackEvent")
		if co.skipInProgress {
			logger.Error("Programming error, attempted to rollback a transaction batch during state transfer")
			return nil
		}

		if !co.batchInProgress {
			logger.Error("Programming error, attempted to rollback a transaction batch which had not started")
			return nil
		}

		err := co.rawExecutor.RollbackTxBatch(co)
		_ = err // TODO This should probably panic, see issue 752

		co.batchInProgress = false

		co.consumer.RolledBack(et.tag)
	case stateUpdateEvent:
		logger.Debug("Executor is processing a stateUpdateEvent")
		if co.batchInProgress {
			err := co.rawExecutor.RollbackTxBatch(co)
			_ = err // TODO This should probably panic, see issue 752
		}

		co.skipInProgress = true

		info := et.blockchainInfo
		for {
			err, recoverable := co.stc.SyncToTarget(info.Height-1, info.CurrentBlockHash, et.peers)
			if err == nil {
				logger.Debug("State transfer sync completed, returning")
				co.skipInProgress = false
				co.consumer.StateUpdated(et.tag, info)
				return nil
			}
			if !recoverable {
				logger.Warningf("State transfer failed irrecoverably, calling back to consumer: %s", err)
				co.consumer.StateUpdated(et.tag, nil)
				return nil
			}
			logger.Warningf("State transfer did not complete successfully but is recoverable, trying again: %s", err)
			et.peers = nil // Broaden the peers included in recover to all connected
		}
	default:
		logger.Errorf("Unknown event type %s", et)
	}

	return nil
}

// Commit commits whatever outstanding requests have been executed, it is an error to call this without pending executions
func (co *coordinatorImpl) Commit(tag interface{}, metadata []byte) {
	co.manager.Queue() <- commitEvent{tag, metadata}
}

// Execute adds additional executions to the current batch
func (co *coordinatorImpl) Execute(tag interface{}, txs []*pb.Transaction) {
	co.manager.Queue() <- executeEvent{tag, txs}
}

// Rollback rolls back the executions from the current batch
func (co *coordinatorImpl) Rollback(tag interface{}) {
	co.manager.Queue() <- rollbackEvent{tag}
}

// UpdateState uses the state transfer subsystem to attempt to progress to a target
func (co *coordinatorImpl) UpdateState(tag interface{}, info *pb.BlockchainInfo, peers []*pb.PeerID) {
	co.manager.Queue() <- stateUpdateEvent{tag, info, peers}
}

// Start must be called before utilizing the Coordinator
func (co *coordinatorImpl) Start() {
	co.stc.Start()
	co.manager.Start()
}

// Halt should be called to clean up resources allocated by the Coordinator
func (co *coordinatorImpl) Halt() {
	co.stc.Stop()
	co.manager.Halt()
}

// Event types

type executeEvent struct {
	tag interface{}
	txs []*pb.Transaction
}

// Note, this cannot be a simple type alias, in case tag is nil
type rollbackEvent struct {
	tag interface{}
}

type commitEvent struct {
	tag      interface{}
	metadata []byte
}

type stateUpdateEvent struct {
	tag            interface{}
	blockchainInfo *pb.BlockchainInfo
	peers          []*pb.PeerID
}
