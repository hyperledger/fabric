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

package helper

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/spf13/viper"
	"golang.org/x/net/context"

	"github.com/hyperledger/fabric/consensus"
	"github.com/hyperledger/fabric/consensus/executor"
	"github.com/hyperledger/fabric/consensus/helper/persist"
	"github.com/hyperledger/fabric/core/chaincode"
	crypto "github.com/hyperledger/fabric/core/crypto"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/peer"
	pb "github.com/hyperledger/fabric/protos"
)

// Helper contains the reference to the peer's MessageHandlerCoordinator
type Helper struct {
	consenter    consensus.Consenter
	coordinator  peer.MessageHandlerCoordinator
	secOn        bool
	valid        bool // Whether we believe the state is up to date
	secHelper    crypto.Peer
	curBatch     []*pb.Transaction       // TODO, remove after issue 579
	curBatchErrs []*pb.TransactionResult // TODO, remove after issue 579
	persist.Helper

	executor consensus.Executor
}

// NewHelper constructs the consensus helper object
func NewHelper(mhc peer.MessageHandlerCoordinator) *Helper {
	h := &Helper{
		coordinator: mhc,
		secOn:       viper.GetBool("security.enabled"),
		secHelper:   mhc.GetSecHelper(),
		valid:       true, // Assume our state is consistent until we are told otherwise, actual consensus (pbft) will invalidate this immediately, but noops will not
	}

	h.executor = executor.NewImpl(h, h, mhc)
	return h
}

func (h *Helper) setConsenter(c consensus.Consenter) {
	h.consenter = c
	h.executor.Start() // The consenter may be expecting a callback from the executor because of state transfer completing, it will miss this if we start the executor too early
}

// GetNetworkInfo returns the PeerEndpoints of the current validator and the entire validating network
func (h *Helper) GetNetworkInfo() (self *pb.PeerEndpoint, network []*pb.PeerEndpoint, err error) {
	ep, err := h.coordinator.GetPeerEndpoint()
	if err != nil {
		return self, network, fmt.Errorf("Couldn't retrieve own endpoint: %v", err)
	}
	self = ep

	peersMsg, err := h.coordinator.GetPeers()
	if err != nil {
		return self, network, fmt.Errorf("Couldn't retrieve list of peers: %v", err)
	}
	peers := peersMsg.GetPeers()
	for _, endpoint := range peers {
		if endpoint.Type == pb.PeerEndpoint_VALIDATOR {
			network = append(network, endpoint)
		}
	}
	network = append(network, self)

	return
}

// GetNetworkHandles returns the PeerIDs of the current validator and the entire validating network
func (h *Helper) GetNetworkHandles() (self *pb.PeerID, network []*pb.PeerID, err error) {
	selfEP, networkEP, err := h.GetNetworkInfo()
	if err != nil {
		return self, network, fmt.Errorf("Couldn't retrieve validating network's endpoints: %v", err)
	}

	self = selfEP.ID

	for _, endpoint := range networkEP {
		network = append(network, endpoint.ID)
	}
	network = append(network, self)

	return
}

// Broadcast sends a message to all validating peers
func (h *Helper) Broadcast(msg *pb.Message, peerType pb.PeerEndpoint_Type) error {
	errors := h.coordinator.Broadcast(msg, peerType)
	if len(errors) > 0 {
		return fmt.Errorf("Couldn't broadcast successfully")
	}
	return nil
}

// Unicast sends a message to a specified receiver
func (h *Helper) Unicast(msg *pb.Message, receiverHandle *pb.PeerID) error {
	return h.coordinator.Unicast(msg, receiverHandle)
}

// Sign a message with this validator's signing key
func (h *Helper) Sign(msg []byte) ([]byte, error) {
	if h.secOn {
		return h.secHelper.Sign(msg)
	}
	logger.Debug("Security is disabled")
	return msg, nil
}

// Verify that the given signature is valid under the given replicaID's verification key
// If replicaID is nil, use this validator's verification key
// If the signature is valid, the function should return nil
func (h *Helper) Verify(replicaID *pb.PeerID, signature []byte, message []byte) error {
	if !h.secOn {
		logger.Debug("Security is disabled")
		return nil
	}

	logger.Debugf("Verify message from: %v", replicaID.Name)
	_, network, err := h.GetNetworkInfo()
	if err != nil {
		return fmt.Errorf("Couldn't retrieve validating network's endpoints: %v", err)
	}

	// check that the sender is a valid replica
	// if so, call crypto verify() with that endpoint's pkiID
	for _, endpoint := range network {
		logger.Debugf("Endpoint name: %v", endpoint.ID.Name)
		if *replicaID == *endpoint.ID {
			cryptoID := endpoint.PkiID
			return h.secHelper.Verify(cryptoID, signature, message)
		}
	}
	return fmt.Errorf("Could not verify message from %s (unknown peer)", replicaID.Name)
}

// BeginTxBatch gets invoked when the next round
// of transaction-batch execution begins
func (h *Helper) BeginTxBatch(id interface{}) error {
	ledger, err := ledger.GetLedger()
	if err != nil {
		return fmt.Errorf("Failed to get the ledger: %v", err)
	}
	if err := ledger.BeginTxBatch(id); err != nil {
		return fmt.Errorf("Failed to begin transaction with the ledger: %v", err)
	}
	h.curBatch = nil     // TODO, remove after issue 579
	h.curBatchErrs = nil // TODO, remove after issue 579
	return nil
}

// ExecTxs executes all the transactions listed in the txs array
// one-by-one. If all the executions are successful, it returns
// the candidate global state hash, and nil error array.
func (h *Helper) ExecTxs(id interface{}, txs []*pb.Transaction) ([]byte, error) {
	// TODO id is currently ignored, fix once the underlying implementation accepts id

	// The secHelper is set during creat ChaincodeSupport, so we don't need this step
	// cxt := context.WithValue(context.Background(), "security", h.coordinator.GetSecHelper())
	// TODO return directly once underlying implementation no longer returns []error

	succeededTxs, res, ccevents, txerrs, err := chaincode.ExecuteTransactions(context.Background(), chaincode.DefaultChain, txs)

	h.curBatch = append(h.curBatch, succeededTxs...) // TODO, remove after issue 579

	//copy errs to result
	txresults := make([]*pb.TransactionResult, len(txerrs))

	//process errors for each transaction
	for i, e := range txerrs {
		//NOTE- it'll be nice if we can have error values. For now success == 0, error == 1
		if txerrs[i] != nil {
			txresults[i] = &pb.TransactionResult{Txid: txs[i].Txid, Error: e.Error(), ErrorCode: 1, ChaincodeEvent: ccevents[i]}
		} else {
			txresults[i] = &pb.TransactionResult{Txid: txs[i].Txid, ChaincodeEvent: ccevents[i]}
		}
	}
	h.curBatchErrs = append(h.curBatchErrs, txresults...) // TODO, remove after issue 579

	return res, err
}

// CommitTxBatch gets invoked when the current transaction-batch needs
// to be committed. This function returns successfully iff the
// transactions details and state changes (that may have happened
// during execution of this transaction-batch) have been committed to
// permanent storage.
func (h *Helper) CommitTxBatch(id interface{}, metadata []byte) (*pb.Block, error) {
	ledger, err := ledger.GetLedger()
	if err != nil {
		return nil, fmt.Errorf("Failed to get the ledger: %v", err)
	}
	// TODO fix this one the ledger has been fixed to implement
	if err := ledger.CommitTxBatch(id, h.curBatch, h.curBatchErrs, metadata); err != nil {
		return nil, fmt.Errorf("Failed to commit transaction to the ledger: %v", err)
	}

	size := ledger.GetBlockchainSize()
	defer func() {
		h.curBatch = nil     // TODO, remove after issue 579
		h.curBatchErrs = nil // TODO, remove after issue 579
	}()

	block, err := ledger.GetBlockByNumber(size - 1)
	if err != nil {
		return nil, fmt.Errorf("Failed to get the block at the head of the chain: %v", err)
	}

	logger.Debugf("Committed block with %d transactions, intended to include %d", len(block.Transactions), len(h.curBatch))

	return block, nil
}

// RollbackTxBatch discards all the state changes that may have taken
// place during the execution of current transaction-batch
func (h *Helper) RollbackTxBatch(id interface{}) error {
	ledger, err := ledger.GetLedger()
	if err != nil {
		return fmt.Errorf("Failed to get the ledger: %v", err)
	}
	if err := ledger.RollbackTxBatch(id); err != nil {
		return fmt.Errorf("Failed to rollback transaction with the ledger: %v", err)
	}
	h.curBatch = nil     // TODO, remove after issue 579
	h.curBatchErrs = nil // TODO, remove after issue 579
	return nil
}

// PreviewCommitTxBatch retrieves a preview of the block info blob (as
// returned by GetBlockchainInfoBlob) that would describe the
// blockchain if CommitTxBatch were invoked.  The blockinfo will
// change if additional ExecTXs calls are invoked.
func (h *Helper) PreviewCommitTxBatch(id interface{}, metadata []byte) ([]byte, error) {
	ledger, err := ledger.GetLedger()
	if err != nil {
		return nil, fmt.Errorf("Failed to get the ledger: %v", err)
	}
	// TODO fix this once the underlying API is fixed
	blockInfo, err := ledger.GetTXBatchPreviewBlockInfo(id, h.curBatch, metadata)
	if err != nil {
		return nil, fmt.Errorf("Failed to preview commit: %v", err)
	}
	rawInfo, _ := proto.Marshal(blockInfo)
	return rawInfo, nil
}

// GetBlock returns a block from the chain
func (h *Helper) GetBlock(blockNumber uint64) (block *pb.Block, err error) {
	ledger, err := ledger.GetLedger()
	if err != nil {
		return nil, fmt.Errorf("Failed to get the ledger :%v", err)
	}
	return ledger.GetBlockByNumber(blockNumber)
}

// GetCurrentStateHash returns the current/temporary state hash
func (h *Helper) GetCurrentStateHash() (stateHash []byte, err error) {
	ledger, err := ledger.GetLedger()
	if err != nil {
		return nil, fmt.Errorf("Failed to get the ledger :%v", err)
	}
	return ledger.GetTempStateHash()
}

// GetBlockchainSize returns the current size of the blockchain
func (h *Helper) GetBlockchainSize() uint64 {
	return h.coordinator.GetBlockchainSize()
}

// GetBlockchainInfo gets the ledger's BlockchainInfo
func (h *Helper) GetBlockchainInfo() *pb.BlockchainInfo {
	ledger, _ := ledger.GetLedger()
	info, _ := ledger.GetBlockchainInfo()
	return info
}

// GetBlockchainInfoBlob marshals a ledger's BlockchainInfo into a protobuf
func (h *Helper) GetBlockchainInfoBlob() []byte {
	ledger, _ := ledger.GetLedger()
	info, _ := ledger.GetBlockchainInfo()
	rawInfo, _ := proto.Marshal(info)
	return rawInfo
}

// GetBlockHeadMetadata returns metadata from block at the head of the blockchain
func (h *Helper) GetBlockHeadMetadata() ([]byte, error) {
	ledger, err := ledger.GetLedger()
	if err != nil {
		return nil, err
	}
	head := ledger.GetBlockchainSize()
	block, err := ledger.GetBlockByNumber(head - 1)
	if err != nil {
		return nil, err
	}
	return block.ConsensusMetadata, nil
}

// InvalidateState is invoked to tell us that consensus realizes the ledger is out of sync
func (h *Helper) InvalidateState() {
	logger.Debug("Invalidating the current state")
	h.valid = false
}

// ValidateState is invoked to tell us that consensus has the ledger back in sync
func (h *Helper) ValidateState() {
	logger.Debug("Validating the current state")
	h.valid = true
}

// Execute will execute a set of transactions, this may be called in succession
func (h *Helper) Execute(tag interface{}, txs []*pb.Transaction) {
	h.executor.Execute(tag, txs)
}

// Commit will commit whatever transactions have been executed
func (h *Helper) Commit(tag interface{}, metadata []byte) {
	h.executor.Commit(tag, metadata)
}

// Rollback will roll back whatever transactions have been executed
func (h *Helper) Rollback(tag interface{}) {
	h.executor.Rollback(tag)
}

// UpdateState attempts to synchronize state to a particular target, implicitly calls rollback if needed
func (h *Helper) UpdateState(tag interface{}, target *pb.BlockchainInfo, peers []*pb.PeerID) {
	if h.valid {
		logger.Warning("State transfer is being called for, but the state has not been invalidated")
	}

	h.executor.UpdateState(tag, target, peers)
}

// Executed is called whenever Execute completes
func (h *Helper) Executed(tag interface{}) {
	if h.consenter != nil {
		h.consenter.Executed(tag)
	}
}

// Committed is called whenever Commit completes
func (h *Helper) Committed(tag interface{}, target *pb.BlockchainInfo) {
	if h.consenter != nil {
		h.consenter.Committed(tag, target)
	}
}

// RolledBack is called whenever a Rollback completes
func (h *Helper) RolledBack(tag interface{}) {
	if h.consenter != nil {
		h.consenter.RolledBack(tag)
	}
}

// StateUpdated is called when state transfer completes, if target is nil, this indicates a failure and a new target should be supplied
func (h *Helper) StateUpdated(tag interface{}, target *pb.BlockchainInfo) {
	if h.consenter != nil {
		h.consenter.StateUpdated(tag, target)
	}
}

// Start his is a byproduct of the consensus API needing some cleaning, for now it's a no-op
func (h *Helper) Start() {}

// Halt is a byproduct of the consensus API needing some cleaning, for now it's a no-op
func (h *Helper) Halt() {}
