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
	"fmt"
	"math/rand"
	"sort"
	"time"

	_ "github.com/hyperledger/fabric/core" // Logging format init

	"github.com/hyperledger/fabric/core/ledger/statemgmt"
	"github.com/hyperledger/fabric/core/peer"
	pb "github.com/hyperledger/fabric/protos"
	"github.com/op/go-logging"
	"github.com/spf13/viper"
)

// =============================================================================
// init
// =============================================================================

var logger *logging.Logger // package-level logger

func init() {
	logger = logging.MustGetLogger("consensus/statetransfer")
}

// =============================================================================
// public methods and structure definitions
// =============================================================================

// PartialStack is a subset of peer.MessageHandlerCoordinator functionality which is necessary to perform state transfer
type PartialStack interface {
	peer.BlockChainAccessor
	peer.BlockChainModifier
	peer.BlockChainUtil
	GetPeers() (*pb.PeersMessage, error)
	GetPeerEndpoint() (*pb.PeerEndpoint, error)
	GetRemoteLedger(receiver *pb.PeerID) (peer.RemoteLedger, error)
}

// Coordinator is used to initiate state transfer.  Start must be called before use, and Stop should be called to free allocated resources
type Coordinator interface {
	Start() // Start the block transfer go routine
	Stop()  // Stop up the block transfer go routine

	// SyncToTarget attempts to move the state to the given target, returning an error, and whether this target might succeed if attempted at a later time
	SyncToTarget(blockNumber uint64, blockHash []byte, peerIDs []*pb.PeerID) (error, bool)
}

// coordinatorImpl is the structure used to manage the state of state transfer
type coordinatorImpl struct {
	stack PartialStack

	DiscoveryThrottleTime time.Duration // The amount of time to wait after discovering there are no connected peers

	stateValid bool // Are we currently operating under the assumption that the state is valid?
	inProgress bool // Set when state transfer is in progress so that the state may not be consistent

	blockVerifyChunkSize uint64        // The max block length to attempt to sync at once, this prevents state transfer from being delayed while the blockchain is validated
	validBlockRanges     []*blockRange // Used by the block thread to track which pieces of the blockchain have already been hashed
	RecoverDamage        bool          // Whether state transfer should ever modify or delete existing blocks if they are determined to be corrupted

	blockSyncReq chan *blockSyncReq // Used to request a block sync, new requests cause the existing request to abort, write only from the state thread

	threadExit chan struct{} // Used to inform the threads that we are shutting down

	BlockRequestTimeout         time.Duration // How long to wait for a peer to respond to a block request
	StateDeltaRequestTimeout    time.Duration // How long to wait for a peer to respond to a state delta request
	StateSnapshotRequestTimeout time.Duration // How long to wait for a peer to respond to a state snapshot request

	maxStateDeltas     int    // The maximum number of state deltas to attempt to retrieve before giving up and performing a full state snapshot retrieval
	maxBlockRange      uint64 // The maximum number blocks to attempt to retrieve at once, to prevent from overflowing the peer's buffer
	maxStateDeltaRange uint64 // The maximum number of state deltas to attempt to retrieve at once, to prevent from overflowing the peer's buffer

	currentStateBlockNumber uint64 // When state transfer does not complete successfully, the current state does not always correspond to the block height
}

// SyncToTarget consumes the calling thread and attempts to perform state transfer until success or an error occurs
// If the peerIDs are nil, then all peers are assumed to have the given block.
// If the call returns an error, a boolean is included which indicates if the error may be transient and the caller should retry
func (sts *coordinatorImpl) SyncToTarget(blockNumber uint64, blockHash []byte, peerIDs []*pb.PeerID) (error, bool) {
	logger.Infof("Syncing to target %x for block number %d with peers %v", blockHash, blockNumber, peerIDs)

	if !sts.inProgress {
		sts.currentStateBlockNumber = sts.stack.GetBlockchainSize() - 1 // The block height is one more than the latest block number
		sts.inProgress = true
	}

	err, recoverable := sts.attemptStateTransfer(blockNumber, peerIDs, blockHash)
	if err == nil {
		sts.inProgress = false
	}

	logger.Debugf("Sync to target %x for block number %d returned, now at block height %d with err=%v recoverable=%v", blockHash, blockNumber, sts.stack.GetBlockchainSize(), err, recoverable)
	return err, recoverable
}

// Start starts the block thread go routine
func (sts *coordinatorImpl) Start() {
	go sts.blockThread()
}

// Stop stops the blockthread go routine
func (sts *coordinatorImpl) Stop() {
	select {
	case <-sts.threadExit:
	default:
		close(sts.threadExit)
	}
}

// =============================================================================
// constructors
// =============================================================================

// NewCoordinatorImpl constructs a coordinatorImpl
func NewCoordinatorImpl(stack PartialStack) Coordinator {
	var err error
	sts := &coordinatorImpl{}

	sts.stack = stack

	sts.RecoverDamage = viper.GetBool("statetransfer.recoverdamage")

	sts.stateValid = true // Assume our starting state is correct unless told otherwise

	sts.validBlockRanges = make([]*blockRange, 0)
	sts.blockVerifyChunkSize = uint64(viper.GetInt("statetransfer.blocksperrequest"))
	if sts.blockVerifyChunkSize == 0 {
		panic(fmt.Errorf("Must set statetransfer.blocksperrequest to be nonzero"))
	}

	sts.blockSyncReq = make(chan *blockSyncReq)

	sts.threadExit = make(chan struct{})

	sts.DiscoveryThrottleTime = 1 * time.Second // TODO make this configurable

	sts.BlockRequestTimeout, err = time.ParseDuration(viper.GetString("statetransfer.timeout.singleblock"))
	if err != nil {
		panic(fmt.Errorf("Cannot parse statetransfer.timeout.singleblock timeout: %s", err))
	}
	sts.StateDeltaRequestTimeout, err = time.ParseDuration(viper.GetString("statetransfer.timeout.singlestatedelta"))
	if err != nil {
		panic(fmt.Errorf("Cannot parse statetransfer.timeout.singlestatedelta timeout: %s", err))
	}
	sts.StateSnapshotRequestTimeout, err = time.ParseDuration(viper.GetString("statetransfer.timeout.fullstate"))
	if err != nil {
		panic(fmt.Errorf("Cannot parse statetransfer.timeout.fullstate timeout: %s", err))
	}

	sts.maxStateDeltas = viper.GetInt("statetransfer.maxdeltas")
	if sts.maxStateDeltas <= 0 {
		panic(fmt.Errorf("sts.maxdeltas must be greater than 0"))
	}

	tmp := viper.GetInt("peer.sync.blocks.channelSize")
	if tmp <= 0 {
		panic(fmt.Errorf("peer.sync.blocks.channelSize must be greater than 0"))
	}
	sts.maxBlockRange = uint64(tmp)

	tmp = viper.GetInt("peer.sync.state.deltas.channelSize")
	if tmp <= 0 {
		panic(fmt.Errorf("peer.sync.state.deltas.channelSize must be greater than 0"))
	}
	sts.maxStateDeltaRange = uint64(tmp)

	return sts
}

// =============================================================================
// custom interfaces and structure definitions
// =============================================================================

type blockSyncReq struct {
	blockNumber    uint64
	peerIDs        []*pb.PeerID
	reportOnBlock  uint64
	replyChan      chan error
	firstBlockHash []byte
}

type blockRange struct {
	highBlock   uint64
	lowBlock    uint64
	lowNextHash []byte
}

type blockRangeSlice []*blockRange

func (a blockRangeSlice) Len() int {
	return len(a)
}
func (a blockRangeSlice) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}
func (a blockRangeSlice) Less(i, j int) bool {
	if a[i].highBlock == a[j].highBlock {
		// If the highs match, the bigger range comes first
		return a[i].lowBlock < a[j].lowBlock
	}
	return a[i].highBlock > a[j].highBlock
}

// =============================================================================
// helper functions for state transfer
// =============================================================================

// Executes a func trying each peer included in peerIDs until successful
// Attempts to execute over all peers if peerIDs is nil
func (sts *coordinatorImpl) tryOverPeers(passedPeerIDs []*pb.PeerID, do func(peerID *pb.PeerID) error) (err error) {

	peerIDs := passedPeerIDs

	ep, err := sts.stack.GetPeerEndpoint()

	if err != nil {
		// Unless we throttle here, this condition will likely cause a tight loop which will adversely affect the rest of the system
		time.Sleep(sts.DiscoveryThrottleTime)
		return fmt.Errorf("Error resolving our own PeerID, this shouldn't happen")
	}

	if nil == passedPeerIDs {
		logger.Debugf("tryOverPeers: no peerIDs given, discovering")

		peersMsg, err := sts.stack.GetPeers()
		if err != nil {
			return fmt.Errorf("Couldn't retrieve list of peers: %v", err)
		}
		peers := peersMsg.GetPeers()
		for _, endpoint := range peers {
			if endpoint.Type == pb.PeerEndpoint_VALIDATOR {
				if endpoint.ID.Name == ep.ID.Name {
					continue
				}
				peerIDs = append(peerIDs, endpoint.ID)
			}
		}

		logger.Debugf("Discovered %d peerIDs", len(peerIDs))
	}

	logger.Debugf("tryOverPeers: using peerIDs: %v", peerIDs)

	if 0 == len(peerIDs) {
		logger.Errorf("Invoked tryOverPeers with no peers specified, throttling thread")
		// Unless we throttle here, this condition will likely cause a tight loop which will adversely affect the rest of the system
		time.Sleep(sts.DiscoveryThrottleTime)
		return fmt.Errorf("No peers available to try over")
	}

	numReplicas := len(peerIDs)
	startIndex := rand.Int() % numReplicas

	for i := 0; i < numReplicas; i++ {
		index := (i + startIndex) % numReplicas
		err = do(peerIDs[index])
		if err == nil {
			break
		} else {
			logger.Warningf("tryOverPeers: loop error from %v : %s", peerIDs[index], err)
		}
	}

	return err

}

// Attempts to complete a blockSyncReq using the supplied peers
// Will return the last block number attempted to sync, and the last block successfully synced (or nil) and error on failure
// This means on failure, the returned block corresponds to 1 higher than the returned block number
func (sts *coordinatorImpl) syncBlocks(highBlock, lowBlock uint64, highHash []byte, peerIDs []*pb.PeerID) (uint64, *pb.Block, error) {
	logger.Debugf("Syncing blocks from %d to %d with head hash of %x", highBlock, lowBlock, highHash)
	validBlockHash := highHash
	blockCursor := highBlock
	var block *pb.Block
	var goodRange *blockRange

	err := sts.tryOverPeers(peerIDs, func(peerID *pb.PeerID) error {
		for {
			intermediateBlock := blockCursor + 1
			var blockChan <-chan *pb.SyncBlocks
			var err error
			for {

				if intermediateBlock == blockCursor+1 {
					if sts.maxBlockRange > blockCursor {
						// Don't underflow
						intermediateBlock = 0
					} else {
						intermediateBlock = blockCursor - sts.maxBlockRange
					}
					if intermediateBlock < lowBlock {
						intermediateBlock = lowBlock
					}
					logger.Debugf("Requesting block range from %d to %d", blockCursor, intermediateBlock)
					blockChan, err = sts.GetRemoteBlocks(peerID, blockCursor, intermediateBlock)
				}

				if nil != err {
					logger.Warningf("Failed to get blocks from %d to %d from %v: %s", blockCursor, lowBlock, peerID, err)
					return err
				}

				select {
				case syncBlockMessage, ok := <-blockChan:

					if !ok {
						return fmt.Errorf("Channel closed before we could finish reading")
					}

					if syncBlockMessage.Range.Start < syncBlockMessage.Range.End {
						// If the message is not replying with blocks backwards, we did not ask for it
						return fmt.Errorf("Received a block with wrong (increasing) order from %v, aborting", peerID)
					}

					var i int
					for i, block = range syncBlockMessage.Blocks {
						// It no longer correct to get duplication or out of range blocks, so we treat this as an error
						if syncBlockMessage.Range.Start-uint64(i) != blockCursor {
							return fmt.Errorf("Received a block out of order, indicating a buffer overflow or other corruption: start=%d, end=%d, wanted %d", syncBlockMessage.Range.Start, syncBlockMessage.Range.End, blockCursor)
						}

						testHash, err := sts.stack.HashBlock(block)
						if nil != err {
							return fmt.Errorf("Got a block %d which could not hash from %v: %s", blockCursor, peerID, err)
						}

						if !bytes.Equal(testHash, validBlockHash) {
							return fmt.Errorf("Got block %d from %v with hash %x, was expecting hash %x", blockCursor, peerID, testHash, validBlockHash)
						}

						logger.Debugf("Putting block %d to with PreviousBlockHash %x and StateHash %x", blockCursor, block.PreviousBlockHash, block.StateHash)
						if !sts.RecoverDamage {

							// If we are not supposed to be destructive in our recovery, check to make sure this block doesn't already exist
							if oldBlock, err := sts.stack.GetBlockByNumber(blockCursor); err == nil && oldBlock != nil {
								oldBlockHash, err := sts.stack.HashBlock(oldBlock)
								if nil == err {
									if !bytes.Equal(oldBlockHash, validBlockHash) {
										panic("The blockchain is corrupt and the configuration has specified that bad blocks should not be deleted/overridden")
									}
								} else {
									logger.Errorf("Could not compute the hash of block %d", blockCursor)
									panic("The blockchain is corrupt and the configuration has specified that bad blocks should not be deleted/overridden")
								}
								logger.Debugf("Not actually putting block %d to with PreviousBlockHash %x and StateHash %x, as it already exists", blockCursor, block.PreviousBlockHash, block.StateHash)
							} else {
								sts.stack.PutBlock(blockCursor, block)
							}
						} else {
							sts.stack.PutBlock(blockCursor, block)
						}

						goodRange = &blockRange{
							highBlock:   highBlock,
							lowBlock:    blockCursor,
							lowNextHash: block.PreviousBlockHash,
						}

						validBlockHash = block.PreviousBlockHash

						if blockCursor == lowBlock {
							logger.Debugf("Successfully synced from block %d to block %d", highBlock, lowBlock)
							return nil
						}
						blockCursor--

					}
				case <-time.After(sts.BlockRequestTimeout):
					return fmt.Errorf("Had block sync request to %v time out", peerID)
				}
			}
		}
	})

	if nil != block {
		logger.Debugf("Returned from sync with block %d and state hash %x", blockCursor, block.StateHash)
	} else {
		logger.Debugf("Returned from sync with no new blocks")
	}

	if goodRange != nil {
		goodRange.lowNextHash = block.PreviousBlockHash
		sts.validBlockRanges = append(sts.validBlockRanges, goodRange)
	}

	return blockCursor, block, err

}

func (sts *coordinatorImpl) syncBlockchainToTarget(blockSyncReq *blockSyncReq) {

	logger.Debugf("Processing a blockSyncReq to block %d through %d", blockSyncReq.blockNumber, blockSyncReq.reportOnBlock)

	blockchainSize := sts.stack.GetBlockchainSize()

	if blockSyncReq.blockNumber+1 < blockchainSize {
		if !sts.RecoverDamage {
			panic("The blockchain height is higher than advertised by consensus, the configuration has specified that bad blocks should not be deleted/overridden, so we cannot proceed")
		} else {
			// TODO For now, unimplemented because we have no way to delete blocks
			panic("Our blockchain is already higher than a sync target, this is unlikely, but unimplemented")
		}
	} else {
		blockCursor := blockSyncReq.blockNumber
		validHash := blockSyncReq.firstBlockHash

		// Don't bother fetching blocks which are already here and valid
		// This is especially useful at startup
		for {
			block, err := sts.stack.GetBlockByNumber(blockCursor)
			if err != nil || block == nil {
				// Need to fetch this block
				break
			}
			bh, err := sts.stack.HashBlock(block)
			if err != nil {
				// Something wrong with this block
				break
			}
			if !bytes.Equal(bh, validHash) {
				// Block is corrupt
				break
			}
			blockCursor--
			validHash = block.PreviousBlockHash
			if blockCursor+1 == blockSyncReq.reportOnBlock {
				break
			}
		}

		if blockCursor+1 <= blockSyncReq.blockNumber {
			logger.Debugf("Skipped remote syncing of block %d through %d because they were already present and valid", blockSyncReq.blockNumber, blockCursor+1)
		}

		var err error
		// Note, this must accomodate blockCursor underflowing
		if blockCursor+1 > blockSyncReq.reportOnBlock {
			_, _, err = sts.syncBlocks(blockCursor, blockSyncReq.reportOnBlock, validHash, blockSyncReq.peerIDs)
		}

		if nil != blockSyncReq.replyChan {
			logger.Debugf("Replying to blockSyncReq on reply channel with : %s", err)
			blockSyncReq.replyChan <- err
		}
	}
}

func (sts *coordinatorImpl) verifyAndRecoverBlockchain() bool {

	if 0 == len(sts.validBlockRanges) {
		size := sts.stack.GetBlockchainSize()
		if 0 == size {
			logger.Warningf("No blocks in the blockchain, including the genesis block")
			return false
		}

		block, err := sts.stack.GetBlockByNumber(size - 1)
		if nil != err {
			logger.Warningf("Could not retrieve head block %d: %s", size, err)
			return false
		}

		sts.validBlockRanges = append(sts.validBlockRanges, &blockRange{
			highBlock:   size - 1,
			lowBlock:    size - 1,
			lowNextHash: block.PreviousBlockHash,
		})

	}

	sort.Sort(blockRangeSlice(sts.validBlockRanges))

	lowBlock := sts.validBlockRanges[0].lowBlock

	logger.Debugf("Validating existing blockchain, highest validated block is %d, valid through %d", sts.validBlockRanges[0].highBlock, lowBlock)

	if 1 == len(sts.validBlockRanges) {
		if 0 == lowBlock {
			// We have exactly one valid block range, and it is from 0 to at least the block height at startup, consider the chain valid
			return true
		}
	}

	lowNextHash := sts.validBlockRanges[0].lowNextHash
	targetBlock := uint64(0)

	if 1 < len(sts.validBlockRanges) {
		if sts.validBlockRanges[1].highBlock+1 >= lowBlock {
			// Ranges are not distinct (or are adjacent), we will collapse them or discard the lower if it does not chain
			if sts.validBlockRanges[1].lowBlock < lowBlock {
				// Range overlaps or is adjacent
				block, err := sts.stack.GetBlockByNumber(lowBlock - 1) // Subtraction is safe here, lowBlock > 0
				if nil != err {
					logger.Warningf("Could not retrieve block %d, believed to be valid: %s", lowBlock-1, err)
				} else {
					if blockHash, err := sts.stack.HashBlock(block); err == nil {
						if bytes.Equal(blockHash, lowNextHash) {
							// The chains connect, no need to validate all the way down
							sts.validBlockRanges[0].lowBlock = sts.validBlockRanges[1].lowBlock
							sts.validBlockRanges[0].lowNextHash = sts.validBlockRanges[1].lowNextHash
						} else {
							logger.Warningf("Detected a block range starting at %d previously believed to be valid did not hash correctly", lowBlock-1)
						}
					} else {
						logger.Warningf("Could not hash block %d, believed to be valid: %s", lowBlock-1, err)
					}
				}
			} else {
				// Range is a subset, we will simply delete
			}

			// If there was an error validating or retrieving, delete, if it was successful, delete
			for j := 1; j < len(sts.validBlockRanges)-1; j++ {
				sts.validBlockRanges[j] = (sts.validBlockRanges)[j+1]
			}
			sts.validBlockRanges = sts.validBlockRanges[:len(sts.validBlockRanges)-1]
			logger.Debugf("Deleted from validBlockRanges, new length %d", len(sts.validBlockRanges))
			return false
		}

		// Ranges are distinct and not adjacent
		targetBlock = sts.validBlockRanges[1].highBlock
	}

	if targetBlock+sts.blockVerifyChunkSize > lowBlock {
		// The sync range is small enough
	} else {
		// targetBlock >=0, targetBlock+blockVerifyChunkSize <= lowBlock --> lowBlock - blockVerifyChunkSize >= 0
		targetBlock = lowBlock - sts.blockVerifyChunkSize
	}

	lastGoodBlockNumber, err := sts.stack.VerifyBlockchain(lowBlock, targetBlock)

	logger.Debugf("Verified chain from %d to %d, with target of %d", lowBlock, lastGoodBlockNumber, targetBlock)

	if err != nil {
		logger.Criticalf("Something went wrong validating the blockchain, recover may be impossible: %s", err)
		return false
	}

	lastGoodBlock, err := sts.stack.GetBlockByNumber(lastGoodBlockNumber)
	if nil != err {
		logger.Errorf("Could not retrieve block %d, believed to be valid: %s", lowBlock-1, err)
		return false
	}

	sts.validBlockRanges[0].lowBlock = lastGoodBlockNumber
	sts.validBlockRanges[0].lowNextHash = lastGoodBlock.PreviousBlockHash

	if targetBlock < lastGoodBlockNumber {
		sts.syncBlocks(lastGoodBlockNumber-1, targetBlock, lastGoodBlock.PreviousBlockHash, nil)
	}

	return false
}

func (sts *coordinatorImpl) blockThread() {

	toggleOn := make(chan struct{})
	close(toggleOn)
	var toggleOff chan struct{}

	// toggleOn causes the toggle case to always be able to be selected
	// toggleOff causes the toggle case to never be selected
	toggle := toggleOn

	for {
		select {
		case blockSyncReq := <-sts.blockSyncReq:
			sts.syncBlockchainToTarget(blockSyncReq)
			toggle = toggleOn
		case <-toggle:
			// If there is no target to sync to, make sure the rest of the chain is valid
			if !sts.verifyAndRecoverBlockchain() {
				// There is more verification to be done, so loop
				continue
			}
			logger.Infof("Validated blockchain to the genesis block")
			toggle = toggleOff
		case <-sts.threadExit:
			logger.Debug("Received request for block transfer thread to exit (1)")
			return
		}
	}
}

func (sts *coordinatorImpl) attemptStateTransfer(blockNumber uint64, peerIDs []*pb.PeerID, blockHash []byte) (error, bool) {
	var err error

	if sts.currentStateBlockNumber+uint64(sts.maxStateDeltas) < blockNumber {
		sts.stateValid = false
	}

	if !sts.stateValid {
		// Our state is currently bad, so get a new one
		sts.currentStateBlockNumber, err = sts.syncStateSnapshot(blockNumber, peerIDs)

		if nil != err {
			return fmt.Errorf("Could not retrieve state as recent as %d from any of specified peers", blockNumber), true
		}

		logger.Debugf("Completed state transfer to block %d", sts.currentStateBlockNumber)
	}

	// TODO, eventually we should allow lower block numbers and rewind transactions as needed
	if blockNumber < sts.currentStateBlockNumber {
		return fmt.Errorf("Cannot validate its state, because its current state corresponds to a higher block number %d than was supplied %d", sts.currentStateBlockNumber, blockNumber), false
	}

	blockReplyChannel := make(chan error)

	req := &blockSyncReq{
		blockNumber:    blockNumber,
		peerIDs:        peerIDs,
		reportOnBlock:  sts.currentStateBlockNumber,
		replyChan:      blockReplyChannel,
		firstBlockHash: blockHash,
	}

	select {
	case sts.blockSyncReq <- req:
	case <-sts.threadExit:
		logger.Debug("Received request to exit while calling thread waiting for block sync reply")
		return fmt.Errorf("Interrupted with request to exit while waiting for block sync reply."), false
	}

	logger.Debugf("State transfer thread waiting for block sync to complete")
	select {
	case err = <-blockReplyChannel:
	case <-sts.threadExit:
		return fmt.Errorf("Interrupted while waiting for block sync reply"), false
	}
	logger.Debugf("State transfer thread continuing")

	if err != nil {
		return fmt.Errorf("Could not retrieve all blocks as recent as %d as requested: %s", blockNumber, err), true
	}

	stateHash, err := sts.stack.GetCurrentStateHash()
	if nil != err {
		sts.stateValid = false
		return fmt.Errorf("Could not compute its current state hash: %s", err), true

	}

	block, err := sts.stack.GetBlockByNumber(sts.currentStateBlockNumber)
	if err != nil {
		return fmt.Errorf("Could not get block %d though we just retrieved it: %s", sts.currentStateBlockNumber, err), true
	}

	if !bytes.Equal(stateHash, block.StateHash) {
		if sts.stateValid {
			sts.stateValid = false
			return fmt.Errorf("Believed its state for block %d to be valid, but its hash (%x) did not match the recovered blockchain's (%x)", sts.currentStateBlockNumber, stateHash, block.StateHash), true
		}
		return fmt.Errorf("Recovered to an incorrect state at block number %d, (%x, %x)", sts.currentStateBlockNumber, stateHash, block.StateHash), true
	}

	logger.Debugf("State is now valid at block %d and hash %x", sts.currentStateBlockNumber, stateHash)

	sts.stateValid = true

	if sts.currentStateBlockNumber < blockNumber {
		err = sts.playStateUpToBlockNumber(blockNumber, peerIDs)
		if nil != err {
			// This is unlikely, in the future, we may wish to play transactions forward rather than retry
			sts.stateValid = false
			return fmt.Errorf("Was unable to play the state from block number %d forward to block %d: %s", sts.currentStateBlockNumber, blockNumber, err), true
		}
	}

	return nil, true
}

func (sts *coordinatorImpl) playStateUpToBlockNumber(toBlockNumber uint64, peerIDs []*pb.PeerID) error {
	logger.Debugf("Attempting to play state forward from %v to block %d", peerIDs, toBlockNumber)
	var stateHash []byte
	err := sts.tryOverPeers(peerIDs, func(peerID *pb.PeerID) error {

		var deltaMessages <-chan *pb.SyncStateDeltas
		for {

			intermediateBlock := sts.currentStateBlockNumber + 1 + sts.maxStateDeltaRange
			if intermediateBlock > toBlockNumber {
				intermediateBlock = toBlockNumber
			}
			logger.Debugf("Requesting state delta range from %d to %d", sts.currentStateBlockNumber+1, intermediateBlock)
			var err error
			deltaMessages, err = sts.GetRemoteStateDeltas(peerID, sts.currentStateBlockNumber+1, intermediateBlock)

			if err != nil {
				return fmt.Errorf("Received an error while trying to get the state deltas for blocks %d through %d from %v", sts.currentStateBlockNumber+1, intermediateBlock, peerID)
			}

			for sts.currentStateBlockNumber < intermediateBlock {
				select {
				case deltaMessage, ok := <-deltaMessages:
					if !ok {
						return fmt.Errorf("Was only able to recover to block number %d when desired to recover to %d", sts.currentStateBlockNumber, toBlockNumber)
					}

					if deltaMessage.Range.Start != sts.currentStateBlockNumber+1 || deltaMessage.Range.End < deltaMessage.Range.Start || deltaMessage.Range.End > toBlockNumber {
						return fmt.Errorf("Received a state delta from %v either in the wrong order (backwards) or not next in sequence, aborting, start=%d, end=%d", peerID, deltaMessage.Range.Start, deltaMessage.Range.End)
					}

					for _, delta := range deltaMessage.Deltas {
						umDelta := &statemgmt.StateDelta{}
						if err := umDelta.Unmarshal(delta); nil != err {
							return fmt.Errorf("Received a corrupt state delta from %v : %s", peerID, err)
						}
						sts.stack.ApplyStateDelta(deltaMessage, umDelta)

						success := false

						testBlock, err := sts.stack.GetBlockByNumber(sts.currentStateBlockNumber + 1)

						if err != nil {
							logger.Warningf("Could not retrieve block %d, though it should be present", deltaMessage.Range.End)
						} else {

							stateHash, err = sts.stack.GetCurrentStateHash()
							if err != nil {
								logger.Warningf("Could not compute state hash for some reason: %s", err)
							}
							logger.Debugf("Played state forward from %v to block %d with StateHash (%x), block has StateHash (%x)", peerID, deltaMessage.Range.End, stateHash, testBlock.StateHash)
							if bytes.Equal(testBlock.StateHash, stateHash) {
								success = true
							}
						}

						if !success {
							if sts.stack.RollbackStateDelta(deltaMessage) != nil {
								sts.stateValid = false
								return fmt.Errorf("played state forward according to %v, but the state hash did not match, failed to roll back, invalidated state", peerID)
							}
							return fmt.Errorf("Played state forward according to %v, but the state hash did not match, rolled back", peerID)

						}

						if sts.stack.CommitStateDelta(deltaMessage) != nil {
							sts.stateValid = false
							return fmt.Errorf("Played state forward according to %v, hashes matched, but failed to commit, invalidated state", peerID)
						}

						logger.Debugf("Moved state from %d to %d", sts.currentStateBlockNumber, sts.currentStateBlockNumber+1)
						sts.currentStateBlockNumber++

						if sts.currentStateBlockNumber == toBlockNumber {
							logger.Debugf("Caught up to block %d", sts.currentStateBlockNumber)
							return nil
						}
					}

				case <-time.After(sts.StateDeltaRequestTimeout):
					logger.Warningf("Timed out during state delta recovery from %v", peerID)
					return fmt.Errorf("timed out during state delta recovery from %v", peerID)
				}
			}
		}

	})
	logger.Debugf("State is now valid at block %d and hash %x", sts.currentStateBlockNumber, stateHash)
	return err
}

// This function will retrieve the current state from a peer.
// Note that no state verification can occur yet, we must wait for the next target, so it is important
// not to consider this state as valid
func (sts *coordinatorImpl) syncStateSnapshot(minBlockNumber uint64, peerIDs []*pb.PeerID) (uint64, error) {

	logger.Debugf("Attempting to retrieve state snapshot from %v", peerIDs)

	currentStateBlock := uint64(0)

	ok := sts.tryOverPeers(peerIDs, func(peerID *pb.PeerID) error {
		logger.Debugf("Initiating state recovery from %v", peerID)

		if err := sts.stack.EmptyState(); nil != err {
			logger.Errorf("Could not empty the current state: %s", err)
		}

		stateChan, err := sts.GetRemoteStateSnapshot(peerID)

		if err != nil {
			return err
		}

		timer := time.NewTimer(sts.StateSnapshotRequestTimeout)
		counter := 0

		for {
			select {
			case piece, ok := <-stateChan:
				if !ok {
					return fmt.Errorf("had state snapshot channel close prematurely after %d deltas: %s", counter, err)
				}
				if 0 == len(piece.Delta) {
					stateHash, err := sts.stack.GetCurrentStateHash()
					if nil != err {
						sts.stateValid = false
						return fmt.Errorf("could not compute its current state hash: %x", err)

					}

					logger.Debugf("Received final piece of state snapshot from %v after %d deltas, now has hash %x", peerID, counter, stateHash)
					return nil
				}
				umDelta := &statemgmt.StateDelta{}
				if err := umDelta.Unmarshal(piece.Delta); nil != err {
					return fmt.Errorf("received a corrupt delta from %v after %d deltas : %s", peerID, counter, err)
				}
				sts.stack.ApplyStateDelta(piece, umDelta)
				currentStateBlock = piece.BlockNumber
				if err := sts.stack.CommitStateDelta(piece); nil != err {
					return fmt.Errorf("could not commit state delta from %v after %d deltas: %s", peerID, counter, err)
				}
				counter++
			case <-timer.C:
				return fmt.Errorf("Timed out during state recovery from %v", peerID)
			}
		}

	})

	return currentStateBlock, ok
}

// The below were stolen from helper.go, they should eventually be removed there, and probably made private here

// GetRemoteBlocks will return a channel to stream blocks from the desired replicaID
func (sts *coordinatorImpl) GetRemoteBlocks(replicaID *pb.PeerID, start, finish uint64) (<-chan *pb.SyncBlocks, error) {
	remoteLedger, err := sts.stack.GetRemoteLedger(replicaID)
	if nil != err {
		return nil, err
	}
	return remoteLedger.RequestBlocks(&pb.SyncBlockRange{
		Start: start,
		End:   finish,
	})
}

// GetRemoteStateSnapshot will return a channel to stream a state snapshot from the desired replicaID
func (sts *coordinatorImpl) GetRemoteStateSnapshot(replicaID *pb.PeerID) (<-chan *pb.SyncStateSnapshot, error) {
	remoteLedger, err := sts.stack.GetRemoteLedger(replicaID)
	if nil != err {
		return nil, err
	}
	return remoteLedger.RequestStateSnapshot()
}

// GetRemoteStateDeltas will return a channel to stream a state snapshot deltas from the desired replicaID
func (sts *coordinatorImpl) GetRemoteStateDeltas(replicaID *pb.PeerID, start, finish uint64) (<-chan *pb.SyncStateDeltas, error) {
	remoteLedger, err := sts.stack.GetRemoteLedger(replicaID)
	if nil != err {
		return nil, err
	}
	return remoteLedger.RequestStateDeltas(&pb.SyncBlockRange{
		Start: start,
		End:   finish,
	})
}
