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

package statetransfer

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"reflect"
	"sync"
	"testing"

	"github.com/hyperledger/fabric/core/ledger/statemgmt"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/protos"
)

type mockRequest int

const (
	SyncDeltas mockRequest = iota
	SyncBlocks
	SyncSnapshot
)

type mockResponse int

const (
	Normal mockResponse = iota
	Corrupt
	Timeout
	OutOfOrder
)

func (r mockResponse) String() string {
	switch r {
	case Normal:
		return "Normal"
	case Corrupt:
		return "Corrupt"
	case Timeout:
		return "Timeout"
	}

	return "ERROR"
}

type LedgerDirectory interface {
	GetLedgerByPeerID(peerID *protos.PeerID) (peer.BlockChainAccessor, bool)
}

type HashLedgerDirectory struct {
	remoteLedgers map[protos.PeerID]peer.BlockChainAccessor
}

func (hd *HashLedgerDirectory) GetLedgerByPeerID(peerID *protos.PeerID) (peer.BlockChainAccessor, bool) {
	ledger, ok := hd.remoteLedgers[*peerID]
	return ledger, ok
}

func (hd *HashLedgerDirectory) GetPeers() (*protos.PeersMessage, error) {
	_, network, err := hd.GetNetworkInfo()
	return &protos.PeersMessage{Peers: network}, err
}

func (hd *HashLedgerDirectory) GetPeerEndpoint() (*protos.PeerEndpoint, error) {
	self, _, err := hd.GetNetworkInfo()
	return self, err
}

func (hd *HashLedgerDirectory) GetNetworkInfo() (self *protos.PeerEndpoint, network []*protos.PeerEndpoint, err error) {
	network = make([]*protos.PeerEndpoint, len(hd.remoteLedgers)+1)
	i := 0
	for peerID := range hd.remoteLedgers {
		peerID := peerID // Get a memory address which will not be overwritten
		network[i] = &protos.PeerEndpoint{
			ID:   &peerID,
			Type: protos.PeerEndpoint_VALIDATOR,
		}
		i++
	}
	network[i] = &protos.PeerEndpoint{
		ID: &protos.PeerID{
			Name: "SelfID",
		},
		Type: protos.PeerEndpoint_VALIDATOR,
	}

	self = network[i]
	return
}

func (hd *HashLedgerDirectory) GetNetworkHandles() (self *protos.PeerID, network []*protos.PeerID, err error) {
	oSelf, oNetwork, err := hd.GetNetworkInfo()
	if nil != err {
		return
	}

	self = oSelf.ID
	network = make([]*protos.PeerID, len(oNetwork))
	for i, endpoint := range oNetwork {
		network[i] = endpoint.ID
	}
	return
}

const MagicDeltaKey string = "The only key/string we ever use for deltas"

type MockLedger struct {
	cleanML       *MockLedger
	blocks        map[uint64]*protos.Block
	blockHeight   uint64
	state         uint64
	remoteLedgers LedgerDirectory
	filter        func(request mockRequest, peerID *protos.PeerID) mockResponse

	mutex *sync.Mutex

	txID          interface{}
	curBatch      []*protos.Transaction
	curResults    []byte
	preBatchState uint64

	deltaID       interface{}
	preDeltaValue uint64

	t *testing.T
}

func NewMockLedger(remoteLedgers LedgerDirectory, filter func(request mockRequest, peerID *protos.PeerID) mockResponse, t *testing.T) *MockLedger {
	mock := &MockLedger{}
	mock.mutex = &sync.Mutex{}
	mock.blocks = make(map[uint64]*protos.Block)
	mock.state = 0
	mock.blockHeight = 0
	mock.t = t

	if nil == filter {
		mock.filter = func(request mockRequest, peerID *protos.PeerID) mockResponse {
			return Normal
		}
	} else {
		mock.filter = filter
	}

	mock.remoteLedgers = remoteLedgers

	return mock
}

func (mock *MockLedger) GetBlockchainSize() uint64 {
	mock.mutex.Lock()
	defer func() {
		mock.mutex.Unlock()
	}()
	return mock.blockHeight
}

func (mock *MockLedger) GetBlock(id uint64) (*protos.Block, error) {
	mock.mutex.Lock()
	defer func() {
		mock.mutex.Unlock()
	}()
	block, ok := mock.blocks[id]
	if !ok {
		return nil, fmt.Errorf("Block not found")
	}
	return block, nil
}

func (mock *MockLedger) GetBlockByNumber(blockNumber uint64) (block *protos.Block, err error) {
	return mock.GetBlock(blockNumber)
}

func (mock *MockLedger) HashBlock(block *protos.Block) ([]byte, error) {
	return SimpleHashBlock(block), nil
}

type remoteLedger struct {
	mockLedger *MockLedger
	peerID     *protos.PeerID
}

func (rl *remoteLedger) RequestBlocks(rng *protos.SyncBlockRange) (<-chan *protos.SyncBlocks, error) {
	return rl.mockLedger.GetRemoteBlocks(rl.peerID, rng.Start, rng.End)
}
func (rl *remoteLedger) RequestStateSnapshot() (<-chan *protos.SyncStateSnapshot, error) {
	return rl.mockLedger.GetRemoteStateSnapshot(rl.peerID)
}
func (rl *remoteLedger) RequestStateDeltas(rng *protos.SyncBlockRange) (<-chan *protos.SyncStateDeltas, error) {
	return rl.mockLedger.GetRemoteStateDeltas(rl.peerID, rng.Start, rng.End)
}

func (mock *MockLedger) GetRemoteLedger(peerID *protos.PeerID) (peer.RemoteLedger, error) {
	return &remoteLedger{
		mockLedger: mock,
		peerID:     peerID,
	}, nil
}

func (mock *MockLedger) GetRemoteBlocks(peerID *protos.PeerID, start, finish uint64) (<-chan *protos.SyncBlocks, error) {
	rl, ok := mock.remoteLedgers.GetLedgerByPeerID(peerID)
	if !ok {
		return nil, fmt.Errorf("Bad peer ID %v", peerID)
	}

	var size int
	if start > finish {
		size = int(start - finish)
	} else {
		size = int(finish - start)
	}

	res := make(chan *protos.SyncBlocks, size+1) // Allows the thread to exit even if the consumer doesn't finish
	ft := mock.filter(SyncBlocks, peerID)
	if ft == Timeout {
		return res, nil
	}

	go func() {

		current := start
		corruptBlock := start + (finish - start/2) // Try to pick a block in the middle, if possible

		for {
			switch {
			case ft == Normal || (ft == Corrupt && current != corruptBlock):
				if block, err := rl.GetBlockByNumber(current); nil == err {
					res <- &protos.SyncBlocks{
						Range: &protos.SyncBlockRange{
							Start: current,
							End:   current,
						},
						Blocks: []*protos.Block{block},
					}

				} else {
					fmt.Printf("TEST LEDGER: %v could not retrieve block %d : %s\n", peerID, current, err)
					break
				}
			case ft == Corrupt:
				res <- &protos.SyncBlocks{
					Range: &protos.SyncBlockRange{
						Start: current,
						End:   current,
					},
					Blocks: []*protos.Block{{
						PreviousBlockHash: []byte("GARBAGE_BLOCK_HASH"),
						StateHash:         []byte("GARBAGE_STATE_HASH"),
						Transactions: []*protos.Transaction{
							{
								Payload: []byte("GARBAGE_PAYLOAD"),
							},
						},
					}},
				}
			case ft == OutOfOrder:
				// Get an adjacent block, if available
				outOfOrder := current + 1
				block, err := rl.GetBlockByNumber(outOfOrder)
				if err != nil {
					outOfOrder = current - 1
					block, err = rl.GetBlockByNumber(outOfOrder)
					if err != nil {
						block = &protos.Block{}
					}
				}

				fmt.Printf("ASDF: Request block %d but sending block %d", current, outOfOrder)

				res <- &protos.SyncBlocks{
					Range: &protos.SyncBlockRange{
						Start: outOfOrder,
						End:   outOfOrder,
					},
					Blocks: []*protos.Block{block},
				}
			default:
				mock.t.Fatalf("Unsupported filter result %d", ft)
			}

			if current == finish {
				break
			}

			if start < finish {
				current++
			} else {
				current--
			}
		}
	}()

	return res, nil
}

func (mock *MockLedger) GetRemoteStateSnapshot(peerID *protos.PeerID) (<-chan *protos.SyncStateSnapshot, error) {

	rl, ok := mock.remoteLedgers.GetLedgerByPeerID(peerID)
	if !ok {
		return nil, fmt.Errorf("Bad peer ID %v", peerID)
	}

	remoteBlockHeight := rl.GetBlockchainSize()
	res := make(chan *protos.SyncStateSnapshot, remoteBlockHeight) // Allows the thread to exit even if the consumer doesn't finish
	ft := mock.filter(SyncSnapshot, peerID)

	if ft == Timeout {
		return res, nil
	}

	if remoteBlockHeight < 1 {
		close(res)
		return res, nil
	}
	rds, err := mock.getRemoteStateDeltas(peerID, 0, remoteBlockHeight-1, SyncSnapshot)
	if nil != err {
		return nil, err
	}
	go func() {
		switch ft {
		case OutOfOrder:
			fallthrough // This is an equivalent case to corruption, as we cannot detect out of order
		case Corrupt:
			res <- &protos.SyncStateSnapshot{
				Delta:       []byte("GARBAGE_DELTA"),
				Sequence:    0,
				BlockNumber: ^uint64(0),
				Request:     nil,
			}
			fallthrough
		case Normal:
			i := uint64(0)
			for deltas := range rds {
				for _, delta := range deltas.Deltas {
					res <- &protos.SyncStateSnapshot{
						Delta:       delta,
						Sequence:    i,
						BlockNumber: remoteBlockHeight - 1,
						Request:     nil,
					}
					i++
				}
				if i == remoteBlockHeight {
					break
				}
			}
			res <- &protos.SyncStateSnapshot{
				Delta:       []byte{},
				Sequence:    i,
				BlockNumber: ^uint64(0),
				Request:     nil,
			}
		default:
			mock.t.Fatalf("Unsupported filter result %d", ft)
		}
	}()
	return res, nil
}

func (mock *MockLedger) GetRemoteStateDeltas(peerID *protos.PeerID, start, finish uint64) (<-chan *protos.SyncStateDeltas, error) {
	return mock.getRemoteStateDeltas(peerID, start, finish, SyncDeltas)
}

func (mock *MockLedger) getRemoteStateDeltas(peerID *protos.PeerID, start, finish uint64, requestType mockRequest) (<-chan *protos.SyncStateDeltas, error) {
	rl, ok := mock.remoteLedgers.GetLedgerByPeerID(peerID)

	if !ok {
		return nil, fmt.Errorf("Bad peer ID %v", peerID)
	}

	var size int
	if start > finish {
		size = int(start - finish)
	} else {
		size = int(finish - start)
	}

	res := make(chan *protos.SyncStateDeltas, size+1) // Allows the thread to exit even if the consumer doesn't finish
	ft := mock.filter(requestType, peerID)
	if ft == Timeout {
		return res, nil
	}
	go func() {
		current := start
		corruptBlock := start + (finish - start/2) // Try to pick a block in the middle, if possible
		for {
			switch {
			case ft == Normal || (ft == Corrupt && current != corruptBlock):
				if remoteBlock, err := rl.GetBlockByNumber(current); nil == err {
					deltas := make([][]byte, len(remoteBlock.Transactions))
					for i, transaction := range remoteBlock.Transactions {
						deltas[i] = SimpleBytesToStateDelta(transaction.Payload).Marshal()
					}
					res <- &protos.SyncStateDeltas{
						Range: &protos.SyncBlockRange{
							Start: current,
							End:   current,
						},
						Deltas: deltas,
					}
				} else {
					break
				}
			case ft == OutOfOrder:
				// Get an adjacent block, if available
				outOfOrder := current + 1
				remoteBlock, err := rl.GetBlockByNumber(outOfOrder)
				if err != nil {
					outOfOrder = current - 1
					remoteBlock, err = rl.GetBlockByNumber(outOfOrder)
					if err != nil {
						remoteBlock = &protos.Block{}
					}
				}

				fmt.Printf("ASDF: Request block %d but sending block %d", current, outOfOrder)

				deltas := make([][]byte, len(remoteBlock.Transactions))
				for i, transaction := range remoteBlock.Transactions {
					deltas[i] = SimpleBytesToStateDelta(transaction.Payload).Marshal()
				}
				res <- &protos.SyncStateDeltas{
					Range: &protos.SyncBlockRange{
						Start: outOfOrder,
						End:   outOfOrder,
					},
					Deltas: deltas,
				}

			case ft == Corrupt:
				deltas := [][]byte{
					[]byte("GARBAGE_DELTA"),
				}
				res <- &protos.SyncStateDeltas{
					Range: &protos.SyncBlockRange{
						Start: current,
						End:   current,
					},
					Deltas: deltas,
				}
			default:
				mock.t.Fatalf("Unsupported filter result %d", ft)

			}

			if current == finish {
				break
			}

			if start < finish {
				current++
			} else {
				current--
			}
		}
	}()
	return res, nil
}

func (mock *MockLedger) PutBlock(blockNumber uint64, block *protos.Block) error {
	mock.mutex.Lock()
	defer func() {
		mock.mutex.Unlock()
	}()
	mock.blocks[blockNumber] = block
	if blockNumber >= mock.blockHeight {
		mock.blockHeight = blockNumber + 1
	}
	return nil
}

func (mock *MockLedger) ApplyStateDelta(id interface{}, delta *statemgmt.StateDelta) error {
	mock.mutex.Lock()
	defer func() {
		mock.mutex.Unlock()
	}()

	if nil != mock.deltaID {
		if !reflect.DeepEqual(id, mock.deltaID) {
			return fmt.Errorf("A different state delta is already being applied")
		}
	} else {
		mock.deltaID = id
		mock.preDeltaValue = mock.state
	}

	d, r := binary.Uvarint(SimpleStateDeltaToBytes(delta))
	if r <= 0 {
		return fmt.Errorf("State delta could not be applied, was not a uint64, %x", d)
	}
	if !delta.RollBackwards {
		mock.state += d
	} else {
		mock.state -= d
	}
	return nil
}

func (mock *MockLedger) CommitStateDelta(id interface{}) error {
	mock.mutex.Lock()
	defer func() {
		mock.mutex.Unlock()
	}()

	mock.deltaID = nil
	return nil
}

func (mock *MockLedger) RollbackStateDelta(id interface{}) error {
	mock.mutex.Lock()
	defer func() {
		mock.mutex.Unlock()
	}()
	mock.deltaID = nil

	mock.state = mock.preDeltaValue
	return nil
}

func (mock *MockLedger) EmptyState() error {
	mock.mutex.Lock()
	defer func() {
		mock.mutex.Unlock()
	}()
	mock.state = 0
	return nil
}

func (mock *MockLedger) GetCurrentStateHash() ([]byte, error) {
	mock.mutex.Lock()
	defer func() {
		mock.mutex.Unlock()
	}()
	return []byte(fmt.Sprintf("%d", mock.state)), nil
}

func (mock *MockLedger) VerifyBlockchain(start, finish uint64) (uint64, error) {
	current := start

	cb, err := mock.GetBlock(current)
	if nil != err {
		return current, err
	}

	for {
		if current == finish {
			return finish, nil
		}

		next := current

		if start < finish {
			next++
		} else {
			next--
		}

		nb, err := mock.GetBlock(next)

		if nil != err {
			return current, nil
		}

		nbh, err := mock.HashBlock(nb)

		if nil != err {
			return current, nil
		}

		if !bytes.Equal(nbh, cb.PreviousBlockHash) {
			return current, nil
		}

		cb = nb
		current = next
	}
}

// Used when the actual transaction content is irrelevant, useful for testing
// state transfer, and other situations without requiring a simulated network
type MockRemoteLedger struct {
	blockHeight uint64
}

func (mock *MockRemoteLedger) setBlockHeight(blockHeight uint64) {
	mock.blockHeight = blockHeight
}

func (mock *MockRemoteLedger) GetBlockByNumber(blockNumber uint64) (block *protos.Block, err error) {
	if blockNumber >= mock.blockHeight {
		return nil, fmt.Errorf("Request block above block height")
	}
	return SimpleGetBlock(blockNumber), nil
}

func (mock *MockRemoteLedger) GetBlockchainSize() uint64 {
	return mock.blockHeight
}

func (mock *MockRemoteLedger) GetCurrentStateHash() (stateHash []byte, err error) {
	return SimpleEncodeUint64(SimpleGetState(mock.blockHeight - 1)), nil
}

func SimpleEncodeUint64(num uint64) []byte {
	result := make([]byte, binary.MaxVarintLen64)
	binary.PutUvarint(result, num)
	return result
}

func SimpleHashBlock(block *protos.Block) []byte {
	buffer := make([]byte, binary.MaxVarintLen64)
	for _, transaction := range block.Transactions {
		for i, b := range transaction.Payload {
			buffer[i%binary.MaxVarintLen64] += b
		}
	}
	return []byte(fmt.Sprintf("BlockHash:%s-%s-%s", buffer, block.StateHash, block.ConsensusMetadata))
}

func SimpleGetState(blockNumber uint64) uint64 {
	// The simple state is (blockNumber) * (blockNumber + 1) / 2
	var computedState uint64
	if 0 == blockNumber%2 {
		computedState = blockNumber / 2 * (blockNumber + 1)
	} else {
		computedState = (blockNumber + 1) / 2 * blockNumber
	}
	return computedState
}

func SimpleGetStateDelta(blockNumber uint64) []byte {
	return SimpleEncodeUint64(blockNumber)
}

func SimpleGetStateHash(blockNumber uint64) []byte {
	return []byte(fmt.Sprintf("%d", SimpleGetState(blockNumber)))
}

func SimpleGetTransactions(blockNumber uint64) []*protos.Transaction {
	return []*protos.Transaction{{
		Payload: SimpleGetStateDelta(blockNumber),
	}}
}

func SimpleBytesToStateDelta(bDelta []byte) *statemgmt.StateDelta {
	mDelta := &statemgmt.StateDelta{
		RollBackwards: false,
	}
	mDelta.ChaincodeStateDeltas = make(map[string]*statemgmt.ChaincodeStateDelta)
	mDelta.ChaincodeStateDeltas[MagicDeltaKey] = &statemgmt.ChaincodeStateDelta{}
	mDelta.ChaincodeStateDeltas[MagicDeltaKey].UpdatedKVs = make(map[string]*statemgmt.UpdatedValue)
	mDelta.ChaincodeStateDeltas[MagicDeltaKey].UpdatedKVs[MagicDeltaKey] = &statemgmt.UpdatedValue{Value: bDelta}
	return mDelta
}

func SimpleStateDeltaToBytes(sDelta *statemgmt.StateDelta) []byte {
	return sDelta.ChaincodeStateDeltas[MagicDeltaKey].UpdatedKVs[MagicDeltaKey].Value
}

func SimpleGetConsensusMetadata(blockNumber uint64) []byte {
	return []byte(fmt.Sprintf("ConsensusMetaData:%d", blockNumber))
}

func SimpleGetBlockHash(blockNumber uint64) []byte {
	if blockNumber == ^uint64(0) {
		// This occurs only when we are the genesis block
		return []byte("GenesisHash")
	}
	return SimpleHashBlock(&protos.Block{
		Transactions:      SimpleGetTransactions(blockNumber),
		ConsensusMetadata: SimpleGetConsensusMetadata(blockNumber),
		StateHash:         SimpleGetStateHash(blockNumber),
	})
}

func SimpleGetBlock(blockNumber uint64) *protos.Block {
	return &protos.Block{
		Transactions:      SimpleGetTransactions(blockNumber),
		ConsensusMetadata: SimpleGetConsensusMetadata(blockNumber),
		StateHash:         SimpleGetStateHash(blockNumber),
		PreviousBlockHash: SimpleGetBlockHash(blockNumber - 1),
	}
}

func TestMockLedger(t *testing.T) {
	remoteLedgers := make(map[protos.PeerID]peer.BlockChainAccessor)
	rl := &MockRemoteLedger{11}
	rlPeerID := &protos.PeerID{
		Name: "TestMockLedger",
	}
	remoteLedgers[*rlPeerID] = rl

	ml := NewMockLedger(&HashLedgerDirectory{remoteLedgers}, nil, t)
	ml.GetCurrentStateHash()

	blockMessages, err := ml.GetRemoteBlocks(rlPeerID, 10, 0)

	success := false

	for blockMessage := range blockMessages {
		current := blockMessage.Range.Start
		i := 0
		for {
			_ = ml.PutBlock(current, blockMessage.Blocks[i]) // Never fails
			i++

			if current == blockMessage.Range.End {
				break
			}

			if blockMessage.Range.Start < blockMessage.Range.End {
				current++
			} else {
				current--
			}
		}
		if current == 0 {
			success = true
			break
		}
	}

	if !success {
		t.Fatalf("Expected more blocks before channel close")
	}

	blockNumber, err := ml.VerifyBlockchain(10, 0)

	if nil != err {
		t.Fatalf("Retrieved blockchain did not validate at block %d with error '%s', error in mock ledger implementation.", blockNumber, err)
	}

	if blockNumber != 0 {
		t.Fatalf("Retrieved blockchain did not validate at block %d, error in mock ledger implementation.", blockNumber)
	}

	_ = ml.PutBlock(3, &protos.Block{ // Never fails
		PreviousBlockHash: []byte("WRONG"),
		StateHash:         []byte("WRONG"),
	})

	blockNumber, err = ml.VerifyBlockchain(10, 0)

	if blockNumber != 4 {
		t.Fatalf("Mangled blockchain did not detect the correct block with the wrong hash, error in mock ledger implementation.")
	}

	syncStateMessages, err := ml.GetRemoteStateSnapshot(rlPeerID)

	if nil != err {
		t.Fatalf("Remote state snapshot call failed, error in mock ledger implementation: %s", err)
	}

	success = false
	_ = ml.EmptyState() // Never fails
	for syncStateMessage := range syncStateMessages {
		if 0 == len(syncStateMessage.Delta) {
			success = true
			break
		}

		delta := &statemgmt.StateDelta{}
		if err := delta.Unmarshal(syncStateMessage.Delta); nil != err {
			t.Fatalf("Error unmarshaling state delta : %s", err)
		}

		if err := ml.ApplyStateDelta(blockNumber, delta); err != nil {
			t.Fatalf("Error applying state delta : %s", err)
		}

		if err := ml.CommitStateDelta(blockNumber); err != nil {
			t.Fatalf("Error committing state delta : %s", err)
		}
	}

	if !success {
		t.Fatalf("Expected nil slice to finish snapshot transfer")
	}

	block10, err := ml.GetBlock(10)

	if nil != err {
		t.Fatalf("Error retrieving block 10, which we should have, error in mock ledger implementation")
	}
	stateHash, _ := ml.GetCurrentStateHash()
	if !bytes.Equal(block10.StateHash, stateHash) {
		t.Fatalf("Ledger state hash %s and block state hash %s do not match, error in mock ledger implementation", stateHash, block10.StateHash)
	}
}
