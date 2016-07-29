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

package noops

import (
	"fmt"
	"strconv"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/op/go-logging"

	"github.com/hyperledger/fabric/consensus"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/statemgmt"
	"github.com/hyperledger/fabric/core/util"
	pb "github.com/hyperledger/fabric/protos"
)

var logger *logging.Logger // package-level logger

func init() {
	logger = logging.MustGetLogger("consensus/noops")
}

// Noops is a plugin object implementing the consensus.Consenter interface.
type Noops struct {
	stack    consensus.Stack
	txQ      *txq
	timer    *time.Timer
	duration time.Duration
	channel  chan *pb.Transaction
}

// Setting up a singleton NOOPS consenter
var iNoops consensus.Consenter

// GetNoops returns a singleton of NOOPS
func GetNoops(c consensus.Stack) consensus.Consenter {
	if iNoops == nil {
		iNoops = newNoops(c)
	}
	return iNoops
}

// newNoops is a constructor returning a consensus.Consenter object.
func newNoops(c consensus.Stack) consensus.Consenter {
	var err error
	if logger.IsEnabledFor(logging.DEBUG) {
		logger.Debug("Creating a NOOPS object")
	}
	i := &Noops{}
	i.stack = c
	config := loadConfig()
	blockSize := config.GetInt("block.size")
	blockWait := config.GetString("block.wait")
	if _, err = strconv.Atoi(blockWait); err == nil {
		blockWait = blockWait + "s" //if string does not have unit of measure, default to seconds
	}
	i.duration, err = time.ParseDuration(blockWait)
	if err != nil || i.duration == 0 {
		panic(fmt.Errorf("Cannot parse block wait: %s", err))
	}

	logger.Infof("NOOPS consensus type = %T", i)
	logger.Infof("NOOPS block size = %v", blockSize)
	logger.Infof("NOOPS block wait = %v", i.duration)

	i.txQ = newTXQ(blockSize)

	i.channel = make(chan *pb.Transaction, 100)
	i.timer = time.NewTimer(i.duration) // start timer now so we can just reset it
	i.timer.Stop()
	go i.handleChannels()
	return i
}

// RecvMsg is called for Message_CHAIN_TRANSACTION and Message_CONSENSUS messages.
func (i *Noops) RecvMsg(msg *pb.Message, senderHandle *pb.PeerID) error {
	if logger.IsEnabledFor(logging.DEBUG) {
		logger.Debugf("Handling Message of type: %s ", msg.Type)
	}
	if msg.Type == pb.Message_CHAIN_TRANSACTION {
		if err := i.broadcastConsensusMsg(msg); nil != err {
			return err
		}
	}
	if msg.Type == pb.Message_CONSENSUS {
		tx, err := i.getTxFromMsg(msg)
		if nil != err {
			return err
		}
		if logger.IsEnabledFor(logging.DEBUG) {
			logger.Debugf("Sending to channel tx uuid: %s", tx.Txid)
		}
		i.channel <- tx
	}
	return nil
}

func (i *Noops) broadcastConsensusMsg(msg *pb.Message) error {
	t := &pb.Transaction{}
	if err := proto.Unmarshal(msg.Payload, t); err != nil {
		return fmt.Errorf("Error unmarshalling payload of received Message:%s.", msg.Type)
	}

	// Change the msg type to consensus and broadcast to the network so that
	// other validators may execute the transaction
	msg.Type = pb.Message_CONSENSUS
	if logger.IsEnabledFor(logging.DEBUG) {
		logger.Debugf("Broadcasting %s", msg.Type)
	}
	txs := &pb.TransactionBlock{Transactions: []*pb.Transaction{t}}
	payload, err := proto.Marshal(txs)
	if err != nil {
		return err
	}
	msg.Payload = payload
	if errs := i.stack.Broadcast(msg, pb.PeerEndpoint_VALIDATOR); nil != errs {
		return fmt.Errorf("Failed to broadcast with errors: %v", errs)
	}
	return nil
}

func (i *Noops) canProcessBlock(tx *pb.Transaction) bool {
	// For NOOPS, if we have completed the sync since we last connected,
	// we can assume that we are at the current state; otherwise, we need to
	// wait for the sync process to complete before we can exec the transactions

	// TODO: Ask coordinator if we need to start sync

	i.txQ.append(tx)

	// start timer if we get a tx
	if i.txQ.size() == 1 {
		i.timer.Reset(i.duration)
	}
	return i.txQ.isFull()
}

func (i *Noops) handleChannels() {
	// Noops is a singleton object and only exits when peer exits, so we
	// don't need a condition to exit this loop
	for {
		select {
		case tx := <-i.channel:
			if i.canProcessBlock(tx) {
				if logger.IsEnabledFor(logging.DEBUG) {
					logger.Debug("Process block due to size")
				}
				if err := i.processBlock(); nil != err {
					logger.Error(err.Error())
				}
			}
		case <-i.timer.C:
			if logger.IsEnabledFor(logging.DEBUG) {
				logger.Debug("Process block due to time")
			}
			if err := i.processBlock(); nil != err {
				logger.Error(err.Error())
			}
		}
	}
}

func (i *Noops) processBlock() error {
	i.timer.Stop()

	if i.txQ.size() < 1 {
		if logger.IsEnabledFor(logging.DEBUG) {
			logger.Debug("processBlock() called but transaction Q is empty")
		}
		return nil
	}
	var data *pb.Block
	var delta *statemgmt.StateDelta
	var err error

	if err = i.processTransactions(); nil != err {
		return err
	}
	if data, delta, err = i.getBlockData(); nil != err {
		return err
	}
	go i.notifyBlockAdded(data, delta)
	return nil
}

func (i *Noops) processTransactions() error {
	timestamp := util.CreateUtcTimestamp()
	if logger.IsEnabledFor(logging.DEBUG) {
		logger.Debugf("Starting TX batch with timestamp: %v", timestamp)
	}
	if err := i.stack.BeginTxBatch(timestamp); err != nil {
		return err
	}

	// Grab all transactions from the FIFO queue and run them in order
	txarr := i.txQ.getTXs()
	if logger.IsEnabledFor(logging.DEBUG) {
		logger.Debugf("Executing batch of %d transactions with timestamp %v", len(txarr), timestamp)
	}
	_, err := i.stack.ExecTxs(timestamp, txarr)

	//consensus does not need to understand transaction errors, errors here are
	//actual ledger errors, and often irrecoverable
	if err != nil {
		logger.Debugf("Rolling back TX batch with timestamp: %v", timestamp)
		i.stack.RollbackTxBatch(timestamp)
		return fmt.Errorf("Fail to execute transactions: %v", err)
	}
	if logger.IsEnabledFor(logging.DEBUG) {
		logger.Debugf("Committing TX batch with timestamp: %v", timestamp)
	}
	if _, err := i.stack.CommitTxBatch(timestamp, nil); err != nil {
		logger.Debugf("Rolling back TX batch with timestamp: %v", timestamp)
		i.stack.RollbackTxBatch(timestamp)
		return err
	}
	return nil
}

func (i *Noops) getTxFromMsg(msg *pb.Message) (*pb.Transaction, error) {
	txs := &pb.TransactionBlock{}
	if err := proto.Unmarshal(msg.Payload, txs); err != nil {
		return nil, err
	}
	return txs.GetTransactions()[0], nil
}

func (i *Noops) getBlockData() (*pb.Block, *statemgmt.StateDelta, error) {
	ledger, err := ledger.GetLedger()
	if err != nil {
		return nil, nil, fmt.Errorf("Fail to get the ledger: %v", err)
	}

	blockHeight := ledger.GetBlockchainSize()
	if logger.IsEnabledFor(logging.DEBUG) {
		logger.Debugf("Preparing to broadcast with block number %v", blockHeight)
	}
	block, err := ledger.GetBlockByNumber(blockHeight - 1)
	if nil != err {
		return nil, nil, err
	}
	//delta, err := ledger.GetStateDeltaBytes(blockHeight)
	delta, err := ledger.GetStateDelta(blockHeight - 1)
	if nil != err {
		return nil, nil, err
	}
	if logger.IsEnabledFor(logging.DEBUG) {
		logger.Debugf("Got the delta state of block number %v", blockHeight)
	}

	return block, delta, nil
}

func (i *Noops) notifyBlockAdded(block *pb.Block, delta *statemgmt.StateDelta) error {
	//make Payload nil to reduce block size..
	//anything else to remove .. do we need StateDelta ?
	for _, tx := range block.Transactions {
		tx.Payload = nil
	}
	data, err := proto.Marshal(&pb.BlockState{Block: block, StateDelta: delta.Marshal()})
	if err != nil {
		return fmt.Errorf("Fail to marshall BlockState structure: %v", err)
	}
	if logger.IsEnabledFor(logging.DEBUG) {
		logger.Debug("Broadcasting Message_SYNC_BLOCK_ADDED to non-validators")
	}

	// Broadcast SYNC_BLOCK_ADDED to connected NVPs
	// VPs already know about this newly added block since they participate
	// in the execution. That is, they can compare their current block with
	// the network block
	msg := &pb.Message{Type: pb.Message_SYNC_BLOCK_ADDED,
		Payload: data, Timestamp: util.CreateUtcTimestamp()}
	if errs := i.stack.Broadcast(msg, pb.PeerEndpoint_NON_VALIDATOR); nil != errs {
		return fmt.Errorf("Failed to broadcast with errors: %v", errs)
	}
	return nil
}

// Executed is called whenever Execute completes, no-op for noops as it uses the legacy synchronous api
func (i *Noops) Executed(tag interface{}) {
	// Never called
}

// Committed is called whenever Commit completes, no-op for noops as it uses the legacy synchronous api
func (i *Noops) Committed(tag interface{}, target *pb.BlockchainInfo) {
	// Never called
}

// RolledBack is called whenever a Rollback completes, no-op for noops as it uses the legacy synchronous api
func (i *Noops) RolledBack(tag interface{}) {
	// Never called
}

// StatedUpdates is called when state transfer completes, if target is nil, this indicates a failure and a new target should be supplied, no-op for noops as it uses the legacy synchronous api
func (i *Noops) StateUpdated(tag interface{}, target *pb.BlockchainInfo) {
	// Never called
}
