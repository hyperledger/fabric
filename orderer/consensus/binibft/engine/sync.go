package engine

import (
	"crypto/sha256"
	"fmt"
	"sync"
	"time"
)

// SyncState represents the status of synchronization
type SyncState int

const (
	SyncStateIdle SyncState = iota
	SyncStateSyncing
	SyncStateComplete
	SyncStateFailed
)

// SyncEngine manages block catch-up and state synchronization for lagging nodes
type SyncEngine struct {
	config        *Config
	storage       BlockStorage
	verifier      Verifier
	logger        Logger
	mu            sync.RWMutex
	state         SyncState
	lastSyncedSeq uint64
	targetSeq     uint64
	syncPeer      NodeID
	maxChunkSize  uint64
}

// NewSyncEngine creates a new state synchronization engine
func NewSyncEngine(config *Config, storage BlockStorage, verifier Verifier, logger Logger) *SyncEngine {
	var currentSeq uint64 = 0
	if storage != nil {
		if latest, err := storage.GetLatestBlock(); err == nil && latest != nil {
			currentSeq = uint64(latest.Sequence)
		}
	}

	return &SyncEngine{
		config:        config,
		storage:       storage,
		verifier:      verifier,
		logger:        logger,
		state:         SyncStateIdle,
		lastSyncedSeq: currentSeq,
		maxChunkSize:  100, // Sync up to 100 blocks per round
	}
}

// GetState returns the current synchronization state
func (s *SyncEngine) GetState() SyncState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

// GetLastSyncedSequence returns the highest sequence synchronized
func (s *SyncEngine) GetLastSyncedSequence() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastSyncedSeq
}

// HandleSyncRequest processes an incoming catch-up request from a peer
func (s *SyncEngine) HandleSyncRequest(req *SyncRequestMessage) (*SyncResponseMessage, error) {
	if req == nil {
		return nil, fmt.Errorf("nil sync request")
	}

	s.mu.RLock()
	storage := s.storage
	s.mu.RUnlock()

	if storage == nil {
		return &SyncResponseMessage{
			FromNodeID: s.config.NodeID,
			ShardID:    s.config.ShardID,
			Error:      "storage not available",
			Timestamp:  time.Now(),
		}, fmt.Errorf("storage not configured on serving node")
	}

	latestBlock, err := storage.GetLatestBlock()
	var latestSeq uint64 = 0
	if err == nil && latestBlock != nil {
		latestSeq = uint64(latestBlock.Sequence)
	}

	endSeq := req.EndSequence
	if endSeq > latestSeq {
		endSeq = latestSeq
	}
	if endSeq < req.StartSequence {
		return &SyncResponseMessage{
			FromNodeID: s.config.NodeID,
			ShardID:    s.config.ShardID,
			LatestSeq:  latestSeq,
			Blocks:     []*Block{},
			Timestamp:  time.Now(),
		}, nil
	}

	// Limit response chunk size
	if endSeq-req.StartSequence+1 > s.maxChunkSize {
		endSeq = req.StartSequence + s.maxChunkSize - 1
	}

	blocks, err := storage.GetBlockRange(req.StartSequence, endSeq)
	if err != nil {
		return &SyncResponseMessage{
			FromNodeID: s.config.NodeID,
			ShardID:    s.config.ShardID,
			LatestSeq:  latestSeq,
			Error:      err.Error(),
			Timestamp:  time.Now(),
		}, err
	}

	return &SyncResponseMessage{
		FromNodeID: s.config.NodeID,
		ShardID:    s.config.ShardID,
		Blocks:     blocks,
		LatestSeq:  latestSeq,
		Timestamp:  time.Now(),
	}, nil
}

// HandleSyncResponse processes incoming blocks from a peer and validates the block chain
func (s *SyncEngine) HandleSyncResponse(resp *SyncResponseMessage) error {
	if resp == nil {
		return fmt.Errorf("nil sync response")
	}

	if resp.Error != "" {
		s.mu.Lock()
		s.state = SyncStateFailed
		s.mu.Unlock()
		return fmt.Errorf("sync peer returned error: %s", resp.Error)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if len(resp.Blocks) == 0 {
		if s.lastSyncedSeq >= s.targetSeq {
			s.state = SyncStateComplete
		}
		return nil
	}

	// Verify and apply blocks sequentially
	var currentPrevHash string
	if s.storage != nil {
		if latest, err := s.storage.GetLatestBlock(); err == nil && latest != nil {
			currentPrevHash = fmt.Sprintf("%x", sha256.Sum256(latest.ToBytes()))
		}
	}

	for idx, block := range resp.Blocks {
		expectedSeq := s.lastSyncedSeq + 1
		if uint64(block.Sequence) != expectedSeq {
			s.state = SyncStateFailed
			return fmt.Errorf("block sequence gap: expected %d, got %d at chunk index %d", expectedSeq, block.Sequence, idx)
		}

		// Verify previous hash chaining (if previous block hash is known)
		if currentPrevHash != "" && block.PrevHash != "" && block.PrevHash != currentPrevHash {
			s.state = SyncStateFailed
			return fmt.Errorf("hash chain mismatch at sequence %d: expected %s, got %s", block.Sequence, currentPrevHash, block.PrevHash)
		}

		// Store verified block in persistent storage
		if s.storage != nil {
			if err := s.storage.StoreBlock(block); err != nil {
				s.state = SyncStateFailed
				return fmt.Errorf("failed to store block at sequence %d: %w", block.Sequence, err)
			}
		}

		currentPrevHash = fmt.Sprintf("%x", sha256.Sum256(block.ToBytes()))
		s.lastSyncedSeq = uint64(block.Sequence)
	}

	if s.lastSyncedSeq >= s.targetSeq || s.lastSyncedSeq >= resp.LatestSeq {
		s.state = SyncStateComplete
		if s.logger != nil {
			s.logger.Info("Catch-up synchronization complete",
				"lastSyncedSeq", s.lastSyncedSeq,
				"targetSeq", s.targetSeq)
		}
	}

	return nil
}

// InitiateSync initiates a catch-up synchronization request to a target peer
func (s *SyncEngine) InitiateSync(targetSeq uint64, peer NodeID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if targetSeq <= s.lastSyncedSeq {
		s.state = SyncStateComplete
		return nil
	}

	s.targetSeq = targetSeq
	s.syncPeer = peer
	s.state = SyncStateSyncing

	startSeq := s.lastSyncedSeq + 1
	endSeq := startSeq + s.maxChunkSize - 1
	if endSeq > targetSeq {
		endSeq = targetSeq
	}

	req := &SyncRequestMessage{
		FromNodeID:    s.config.NodeID,
		ShardID:       s.config.ShardID,
		StartSequence: startSeq,
		EndSequence:   endSeq,
		Timestamp:     time.Now(),
	}

	if s.config.Signer != nil {
		reqDigest := ComputePrePrepAckDigest(s.config.ChannelID, 0, startSeq, s.config.ShardID, s.config.NodeID, "sync-req")
		req.Signature = s.config.Signer.Sign(reqDigest)
	}

	msg := Message{
		Type:      MsgSyncRequest,
		From:      s.config.NodeID,
		To:        peer,
		ShardID:   s.config.ShardID,
		Timestamp: time.Now(),
		Payload:   req,
	}

	if s.config.Network != nil {
		if err := s.config.Network.Send(peer, msg); err != nil {
			s.state = SyncStateFailed
			return fmt.Errorf("failed to send sync request to peer %s: %w", peer, err)
		}
	}

	if s.logger != nil {
		s.logger.Info("Initiated catch-up synchronization",
			"startSeq", startSeq,
			"endSeq", endSeq,
			"targetSeq", targetSeq,
			"peer", peer)
	}

	return nil
}

// VerifyBlockChain validates the cryptographic sequence continuity and hash pointers across a slice of blocks
func VerifyBlockChain(blocks []*Block, expectedStartSeq uint64, initialPrevHash string) error {
	if len(blocks) == 0 {
		return nil
	}

	currentPrevHash := initialPrevHash
	for i, b := range blocks {
		expectedSeq := int64(expectedStartSeq) + int64(i)
		if b.Sequence != expectedSeq {
			return fmt.Errorf("sequence discontinuity at index %d: expected %d, got %d", i, expectedSeq, b.Sequence)
		}

		if currentPrevHash != "" && b.PrevHash != "" && b.PrevHash != currentPrevHash {
			return fmt.Errorf("prevHash mismatch at sequence %d: expected %s, got %s", b.Sequence, currentPrevHash, b.PrevHash)
		}

		currentPrevHash = fmt.Sprintf("%x", sha256.Sum256(b.ToBytes()))
	}

	return nil
}
