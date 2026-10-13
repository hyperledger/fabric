package engine

import (
	"github.com/hyperledger/fabric/orderer/consensus/binibft/engine/protos"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
)

// SequenceVoteTracker tracks votes for a specific sequence within a view
type SequenceVoteTracker struct {
	Sequence      uint64
	Votes         map[NodeID]*Vote
	QuorumReached map[string]bool // phase -> quorum reached
	Timestamp     time.Time
}

// View represents a consensus view with its state and message tracking
type View struct {
	Number          uint64
	Primary         NodeID
	ShardID         ShardID
	Sequence        uint64
	DecisionsInView uint64

	// Phase tracking
	mu    sync.RWMutex
	phase ViewPhase

	// Message storage for current view - sequence-based
	prePrepMessages map[uint64]map[NodeID]*PrePrepMessage
	prepareMessages map[uint64]map[NodeID]*PreparePhaseMessage
	commitMessages  map[uint64]map[NodeID]*CommitRequestMessage

	// SmartBFT-style sequence-based vote tracking
	sequenceVotes map[uint64]*SequenceVoteTracker

	// Shard acknowledgments for cross-shard coordination
	shardAcks map[uint64]map[NodeID]*ShardAckMessage

	// In-flight requests tracking - stores request info for each sequence
	inFlightRequests map[uint64]*RequestInfo

	// Finalization tracking
	finalizedSequences map[uint64]bool

	// Intra-shard vote tracking - sequence -> phase -> nodeID -> vote
	intraShardVotes map[uint64]map[string]map[NodeID]bool

	// Timers
	viewTimer *time.Timer

	// Configuration
	config *Config
	logger Logger
}

// ViewPhase represents the current phase of consensus in a view
type ViewPhase int

const (
	ViewPhaseIdle ViewPhase = iota
	ViewPhasePrePrepare
	ViewPhasePrepare
	ViewPhaseCommit
	ViewPhaseDecided
)

func (p ViewPhase) String() string {
	switch p {
	case ViewPhaseIdle:
		return "Idle"
	case ViewPhasePrePrepare:
		return "PrePrepare"
	case ViewPhasePrepare:
		return "Prepare"
	case ViewPhaseCommit:
		return "Commit"
	case ViewPhaseDecided:
		return "Decided"
	default:
		return "Unknown"
	}
}

// NewView creates a new consensus view
func NewView(primary NodeID, shardID ShardID, config *Config) *View {
	var viewId uint64
	var seq uint64 = 1
	if config.Metadata != nil {
		viewId = config.Metadata.ViewId
		seq = config.Metadata.LatestSequence + 1
	}
	return &View{
		Number:             viewId,
		Primary:            primary,
		ShardID:            shardID,
		Sequence:           seq,
		phase:              ViewPhaseIdle,
		prePrepMessages:    make(map[uint64]map[NodeID]*PrePrepMessage),
		prepareMessages:    make(map[uint64]map[NodeID]*PreparePhaseMessage),
		commitMessages:     make(map[uint64]map[NodeID]*CommitRequestMessage),
		sequenceVotes:      make(map[uint64]*SequenceVoteTracker),
		shardAcks:          make(map[uint64]map[NodeID]*ShardAckMessage),
		inFlightRequests:   make(map[uint64]*RequestInfo),
		finalizedSequences: make(map[uint64]bool),
		intraShardVotes:    make(map[uint64]map[string]map[NodeID]bool),
		config:             config,
		logger:             config.Logger,
	}
}

func (v *View) GetMetadata() []byte {
	metadata := &protos.ViewMetadata{
		ViewId:          v.Number,
		LatestSequence:  v.Sequence,
		DecisionsInView: v.DecisionsInView,
	}
	jsonmetadata, _ := proto.Marshal(metadata)
	return jsonmetadata
}

// Propose initiates consensus for a proposal from batch (Primary Leader only)
func (v *View) Propose(proposal Proposal) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	// Check if sequence is already finalized
	if v.finalizedSequences[v.Sequence] {
		v.logger.Debug("Sequence already finalized, skipping", "sequence", v.Sequence)
		return nil
	}

	// Check if there's already a proposal in progress for this sequence
	if _, exists := v.prePrepMessages[v.Sequence]; exists {
		v.logger.Debug("Proposal already in progress for sequence, skipping",
			"sequence", v.Sequence,
			"prePrepCount", len(v.prePrepMessages),
			"finalizedCount", len(v.finalizedSequences))
		return nil
	}

	// Clean up old sequences before proposing
	v.cleanupOldSequences()

	// Only primary leader can start pre-prepare
	if v.config.Role != RolePrimaryLeader {
		return fmt.Errorf("only primary leader can start pre-prepare")
	}

	v.logger.Info("Primary leader starting consensus for proposal",
		"view", v.Number,
		"sequence", v.Sequence,
		"nodeID", v.config.NodeID,
		"totalShardLeaders", len(v.config.ShardLeaders))

	// Store in-flight request info
	v.inFlightRequests[v.Sequence] = &RequestInfo{
		ClientID: "batch",
		ID:       fmt.Sprintf("seq-%d", v.Sequence),
	}

	proposalDigest := proposal.Digest()
	prePrepDigest := ComputePrePrepDigest(v.config.ChannelID, v.Number, v.Sequence, proposalDigest)

	prePrepMsg := &PrePrepMessage{
		Proposal:  proposal,
		View:      v.Number,
		Sequence:  v.Sequence,
		Digest:    proposalDigest,
		NodeID:    v.config.NodeID,
		ShardID:   v.ShardID,
		Signature: v.signDigest(prePrepDigest),
	}

	// Store pre-prepare
	if _, exists := v.prePrepMessages[v.Sequence]; !exists {
		v.prePrepMessages[v.Sequence] = make(map[NodeID]*PrePrepMessage)
	}
	v.prePrepMessages[v.Sequence][v.config.NodeID] = prePrepMsg
	v.phase = ViewPhasePrePrepare

	// Send pre-prepare to all shard leaders
	otherShardLeaderCount := v.sendPrePrepToShardLeaders(prePrepMsg)

	// Only proceed immediately if there are no other shard leaders
	if otherShardLeaderCount == 0 {
		// No other shard leaders - proceed directly
		v.logger.Info("No other shard leaders - proceeding to prepare phase", "sequence", v.Sequence)
		v.startPreparePhaseWithShardLeaders(v.Sequence)
	} else {
		// Wait for other shard leaders to respond before proceeding
		v.logger.Info("Waiting for shard leader responses before proceeding to prepare phase",
			"sequence", v.Sequence,
			"otherShardLeaders", otherShardLeaderCount)
		// The prepare phase will be started when enough shard ACKs are received
	}

	return nil
}

// sendPrePrepToShardLeaders sends pre-prepare to all shard leaders and returns count of messages sent
func (v *View) sendPrePrepToShardLeaders(prePrepMsg *PrePrepMessage) int {
	shardLeaders := make([]NodeID, 0, len(v.config.ShardLeaders))
	for _, leaderID := range v.config.ShardLeaders {
		if leaderID != v.config.NodeID { // Don't send to self
			shardLeaders = append(shardLeaders, leaderID)
		}
	}
	sentCount := len(shardLeaders)

	msg := Message{
		Type:      MsgPrePrep,
		From:      v.config.NodeID,
		ShardID:   v.ShardID,
		Timestamp: time.Now(),
		Payload:   prePrepMsg,
	}
	v.config.Network.Broadcast(shardLeaders, msg)

	v.logger.Info("Pre-prep distribution complete",
		"sequence", prePrepMsg.Sequence,
		"sentToShardLeaders", sentCount,
		"totalShardLeaders", len(v.config.ShardLeaders))

	return sentCount
}

// HandlePrePrepare processes a pre-prepare message
func (v *View) HandlePrePrepare(msg *PrePrepMessage) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	// Check if sequence is already finalized
	if v.finalizedSequences[msg.Sequence] {
		v.logger.Debug("Sequence already finalized, skipping PrePrep phase handling", "sequence", msg.Sequence)
		return nil
	}

	// Verify Primary Leader's signature
	if len(msg.Signature) > 0 {
		expectedDigest := ComputePrePrepDigest(v.config.ChannelID, msg.View, msg.Sequence, msg.Digest)
		if err := v.verifyDigestSignature(msg.NodeID, expectedDigest, msg.Signature); err != nil {
			v.logger.Error("Invalid signature on PrePrep message", "from", msg.NodeID, "error", err)
			return fmt.Errorf("invalid signature on PrePrep message: %w", err)
		}
	}

	// Verify proposal structure and batch payload if verifier is configured
	if v.config.Verifier != nil && (len(msg.Proposal.Header) > 0 || len(msg.Proposal.Payload) > 0) {
		if _, err := v.config.Verifier.VerifyProposal(msg.Proposal); err != nil {
			v.logger.Error("Invalid proposal in PrePrep message", "from", msg.NodeID, "error", err)
			return fmt.Errorf("invalid proposal in PrePrep message: %w", err)
		}
	}

	// Handle sequence synchronization
	if !v.isValidSequenceRange(msg.Sequence) {
		v.logger.Debug("Received pre-prep for sequence outside valid range, ignoring",
			"receivedSequence", msg.Sequence,
			"currentSequence", v.Sequence)
		return nil
	}

	// If we receive a message for a future sequence, advance our sequence to catch up
	if msg.Sequence > v.Sequence {
		v.logger.Info("Received pre-prep for future sequence, advancing to catch up",
			"receivedSequence", msg.Sequence,
			"currentSequence", v.Sequence,
			"nodeID", v.config.NodeID)
		v.Sequence = msg.Sequence
		// Update metadata if available
		if v.config.Metadata != nil {
			v.config.Metadata.LatestSequence = v.Sequence
		}
	}

	v.logger.Info("Handling pre-prep message",
		"sequence", msg.Sequence,
		"view", msg.View,
		"from", msg.NodeID,
		"role", v.config.Role.String(),
		"nodeID", v.config.NodeID)

	// Store in-flight request info from proposal
	v.inFlightRequests[msg.Sequence] = &RequestInfo{
		ClientID: "batch",
		ID:       fmt.Sprintf("seq-%d", msg.Sequence),
	}

	// Store pre-prep message and check for equivocation
	if _, exists := v.prePrepMessages[msg.Sequence]; !exists {
		v.prePrepMessages[msg.Sequence] = make(map[NodeID]*PrePrepMessage)
	} else if existing, exists := v.prePrepMessages[msg.Sequence][msg.NodeID]; exists {
		if existing.Digest != msg.Digest {
			v.logger.Error("Equivocation detected: conflicting proposal for sequence", "sequence", msg.Sequence, "from", msg.NodeID)
			return fmt.Errorf("equivocation detected: conflicting proposal for sequence %d from %s", msg.Sequence, msg.NodeID)
		}
		return nil // Duplicate identical pre-prep: ignore safely
	}
	v.prePrepMessages[msg.Sequence][msg.NodeID] = msg

	switch v.config.Role {
	case RoleShardLeader:
		// Shard leaders forward Pre-Prep to their followers and wait for ACKs
		v.forwardPrePrepToFollowers(msg)
		// Also check if we have majority (including self) and send ACK to primary
		v.checkFollowerMajority(msg.Sequence)
	case RoleShardFollower:
		// Followers send ACK back to their shard leader
		v.sendAckToShardLeader(msg)
	}

	return nil
}

// forwardPrePrepToFollowers forwards pre-prep to shard followers
func (v *View) forwardPrePrepToFollowers(prePrepMsg *PrePrepMessage) {
	shardNodes := v.config.ShardNodes[v.config.ShardID]
	for _, nodeID := range shardNodes {
		if nodeID != v.config.NodeID { // Don't send to self
			msg := Message{
				Type:      MsgPrePrep,
				From:      v.config.NodeID,
				To:        nodeID,
				ShardID:   v.config.ShardID,
				Timestamp: time.Now(),
				Payload:   prePrepMsg,
			}
			v.config.Network.Send(nodeID, msg)
			v.logger.Info("Forwarded pre-prep to follower",
				"sequence", prePrepMsg.Sequence,
				"follower", nodeID)
		}
	}
}

// checkFollowerMajority checks if majority of followers have sent pre-prep ACKs
func (v *View) checkFollowerMajority(sequence uint64) {
	shardNodes := v.config.ShardNodes[v.config.ShardID]
	requiredCount := CalculateIntraShardQuorum(len(shardNodes))

	// Count pre-prep ACKs from followers in this shard (including self)
	ackCount := 1 // Count self as ACK
	if acks, exists := v.shardAcks[sequence]; exists {
		for _, ack := range acks {
			if ack.ShardID == v.config.ShardID && ack.Acknowledged && ack.Phase == "preprep" {
				ackCount++
			}
		}
	}

	v.logger.Info("Checking follower majority for pre-prep",
		"sequence", sequence,
		"ackCount", ackCount,
		"required", requiredCount)

	if ackCount >= requiredCount {
		// Majority reached - send ACK to primary leader
		v.logger.Info("Follower majority reached - sending pre-prep ACK to primary", "sequence", sequence)
		v.sendPrePrepAckToPrimary(sequence)
	}
}

// sendPrePrepAckToPrimary sends pre-prep acknowledgment from shard leader to primary leader
func (v *View) sendPrePrepAckToPrimary(sequence uint64) {
	ackDigest := ComputePrePrepAckDigest(v.config.ChannelID, v.Number, sequence, v.config.ShardID, v.config.NodeID, "shard-ack")
	ack := &ShardAckMessage{
		Sequence:     sequence,
		ShardID:      v.config.ShardID,
		NodeID:       v.config.NodeID,
		Acknowledged: true,
		Phase:        "preprep",
		Digest:       "shard-ack",
		Signature:    v.signDigest(ackDigest),
		Timestamp:    time.Now(),
	}

	msg := Message{
		Type:      MsgShardAck,
		From:      v.config.NodeID,
		To:        v.config.PrimaryLeader,
		ShardID:   v.config.ShardID,
		Timestamp: time.Now(),
		Payload:   ack,
	}

	v.config.Network.Send(v.config.PrimaryLeader, msg)
	v.logger.Info("📤 Sent pre-prep ACK to primary leader",
		"sequence", sequence,
		"primaryLeader", v.config.PrimaryLeader,
		"fromShardLeader", v.config.NodeID)
}

// sendAckToShardLeader sends ACK from follower to shard leader after getting intra-shard consensus
func (v *View) sendAckToShardLeader(prePrepMsg *PrePrepMessage) {
	// First, broadcast to other nodes in the same shard to get consensus
	v.broadcastToShardNodes(prePrepMsg, "preprep")
}

// HandleShardAck processes shard acknowledgment messages
func (v *View) HandleShardAck(ack *ShardAckMessage) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	// Check if sequence is already finalized
	if v.finalizedSequences[ack.Sequence] {
		v.logger.Debug("Sequence already finalized, ignoring shard ACK",
			"sequence", ack.Sequence,
			"phase", ack.Phase,
			"from", ack.NodeID)
		return nil
	}

	// Verify shard leader's signature on the ACK
	if len(ack.Signature) > 0 {
		var expectedDigest []byte
		switch ack.Phase {
		case "preprep":
			expectedDigest = ComputePrePrepAckDigest(v.config.ChannelID, v.Number, ack.Sequence, ack.ShardID, ack.NodeID, ack.Digest)
		case "prepare":
			expectedDigest = ComputePrepareVoteDigest(v.config.ChannelID, v.Number, ack.Sequence, ack.ShardID, ack.NodeID, ack.Digest)
		case "commit":
			expectedDigest = ComputeCommitVoteDigest(v.config.ChannelID, v.Number, ack.Sequence, ack.ShardID, ack.NodeID, ack.Digest)
		}
		if len(expectedDigest) > 0 {
			if err := v.verifyDigestSignature(ack.NodeID, expectedDigest, ack.Signature); err != nil {
				v.logger.Error("Invalid signature on ShardAck message", "from", ack.NodeID, "phase", ack.Phase, "error", err)
				return fmt.Errorf("invalid signature on ShardAck message: %w", err)
			}
		}
	}

	// Handle sequence synchronization
	if !v.isValidSequenceRange(ack.Sequence) {
		v.logger.Debug("Received shard ACK for sequence outside valid range, ignoring",
			"receivedSequence", ack.Sequence,
			"currentSequence", v.Sequence,
			"phase", ack.Phase,
			"from", ack.NodeID)
		return nil
	}

	// If we receive a message for a future sequence, advance our sequence to catch up
	if ack.Sequence > v.Sequence {
		v.logger.Info("Received shard ACK for future sequence, advancing to catch up",
			"receivedSequence", ack.Sequence,
			"currentSequence", v.Sequence,
			"nodeID", v.config.NodeID,
			"phase", ack.Phase)
		v.Sequence = ack.Sequence
		// Update metadata if available
		if v.config.Metadata != nil {
			v.config.Metadata.LatestSequence = v.Sequence
		}
	}

	v.logger.Info("Handling shard ACK",
		"sequence", ack.Sequence,
		"from", ack.NodeID,
		"phase", ack.Phase,
		"shardID", ack.ShardID)

	// Store shard ACK
	if _, exists := v.shardAcks[ack.Sequence]; !exists {
		v.shardAcks[ack.Sequence] = make(map[NodeID]*ShardAckMessage)
	}
	v.shardAcks[ack.Sequence][ack.NodeID] = ack

	vote := &Vote{
		NodeID:    ack.NodeID,
		ShardID:   ack.ShardID,
		Approve:   ack.Acknowledged,
		Signature: ack.Signature,
	}

	if v.captureVoteForSequence(ack.Sequence, vote, ack.Phase) {
		v.logger.Info("Shard ACK triggered SmartBFT quorum",
			"sequence", ack.Sequence,
			"phase", ack.Phase)
		v.handlePhaseQuorumReached(ack.Sequence, ack.Phase)
	}

	// Special handling for primary leader receiving commit ACKs from shard leaders
	if v.config.Role == RolePrimaryLeader && ack.Phase == "commit" {
		v.checkPrimaryCommitQuorum(ack.Sequence)
	}

	return nil
}

// HandlePreparePhase processes prepare phase messages
func (v *View) HandlePreparePhase(msg *PreparePhaseMessage) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	// Check if sequence is already finalized
	if v.finalizedSequences[msg.Sequence] {
		v.logger.Debug("Sequence already finalized, skipping Prepare phase handling", "sequence", msg.Sequence)
		return nil
	}

	// Verify Primary Leader's signature
	if len(msg.Signature) > 0 {
		expectedDigest := ComputePrepareVoteDigest(v.config.ChannelID, msg.View, msg.Sequence, msg.ShardID, msg.NodeID, msg.Digest)
		if err := v.verifyDigestSignature(msg.NodeID, expectedDigest, msg.Signature); err != nil {
			v.logger.Error("Invalid signature on PreparePhase message", "from", msg.NodeID, "error", err)
			return fmt.Errorf("invalid signature on PreparePhase message: %w", err)
		}
	}

	// Handle sequence synchronization
	if !v.isValidSequenceRange(msg.Sequence) {
		v.logger.Debug("Received prepare phase for sequence outside valid range, ignoring",
			"receivedSequence", msg.Sequence,
			"currentSequence", v.Sequence)
		return nil
	}

	// If we receive a message for a future sequence, advance our sequence to catch up
	if msg.Sequence > v.Sequence {
		v.logger.Info("Received prepare phase for future sequence, advancing to catch up",
			"receivedSequence", msg.Sequence,
			"currentSequence", v.Sequence,
			"nodeID", v.config.NodeID)
		v.Sequence = msg.Sequence
		// Update metadata if available
		if v.config.Metadata != nil {
			v.config.Metadata.LatestSequence = v.Sequence
		}
	}

	v.logger.Info("Handling prepare phase message",
		"sequence", msg.Sequence,
		"from", msg.NodeID,
		"role", v.config.Role.String(),
		"nodeID", v.config.NodeID,
		"view", msg.View)

	// Store prepare message
	if _, exists := v.prepareMessages[msg.Sequence]; !exists {
		v.prepareMessages[msg.Sequence] = make(map[NodeID]*PreparePhaseMessage)
	}
	v.prepareMessages[msg.Sequence][msg.NodeID] = msg

	// Store in-flight request info from proposal
	v.inFlightRequests[msg.Sequence] = &RequestInfo{
		ClientID: "batch",
		ID:       fmt.Sprintf("seq-%d", msg.Sequence),
	}

	// SmartBFT-style vote capture
	vote := &Vote{
		NodeID:    msg.NodeID,
		ShardID:   msg.ShardID,
		Approve:   true,
		Signature: msg.Signature,
	}

	// Capture vote for this sequence and check quorum
	if v.captureVoteForSequence(msg.Sequence, vote, "prepare") {
		// Quorum reached - proceed to commit phase
		v.logger.Info("Prepare phase quorum reached via SmartBFT", "sequence", msg.Sequence)
		v.startCommitPhaseWithShardLeaders(msg.Sequence)
	}

	// For shard leaders, also add their own vote if they haven't already
	if v.config.Role == RoleShardLeader && msg.NodeID != v.config.NodeID {
		prepDigest := ComputePrepareVoteDigest(v.config.ChannelID, v.Number, msg.Sequence, v.config.ShardID, v.config.NodeID, msg.Proposal.Digest())
		ownVote := &Vote{
			NodeID:    v.config.NodeID,
			ShardID:   v.config.ShardID,
			Approve:   true,
			Signature: v.signDigest(prepDigest),
		}
		if v.captureVoteForSequence(msg.Sequence, ownVote, "prepare") {
			v.logger.Info("Prepare phase quorum reached via SmartBFT (with own vote)", "sequence", msg.Sequence)
			v.startCommitPhaseWithShardLeaders(msg.Sequence)
		}
	}

	// Handle role-specific forwarding
	switch v.config.Role {
	case RoleShardLeader:
		// Shard leaders forward Prepare to their followers
		v.forwardPrepareToFollowers(msg)
	case RoleShardFollower:
		// Followers send prepare ACK back to their shard leader
		v.sendPrepareAckToShardLeader(msg.Sequence)
	}

	return nil
}

// HandleCommitRequest processes commit request messages
func (v *View) HandleCommitRequest(msg *CommitRequestMessage) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	// Check if sequence is already finalized
	if v.finalizedSequences[msg.Sequence] {
		v.logger.Debug("Sequence already finalized, skipping Commit phase handling", "sequence", msg.Sequence)
		return nil
	}

	// Verify Primary Leader's signature
	if len(msg.Signature) > 0 {
		expectedDigest := ComputeCommitVoteDigest(v.config.ChannelID, msg.View, msg.Sequence, msg.ShardID, msg.NodeID, msg.Digest)
		if err := v.verifyDigestSignature(msg.NodeID, expectedDigest, msg.Signature); err != nil {
			v.logger.Error("Invalid signature on CommitRequest message", "from", msg.NodeID, "error", err)
			return fmt.Errorf("invalid signature on CommitRequest message: %w", err)
		}
	}

	// Handle sequence synchronization
	if !v.isValidSequenceRange(msg.Sequence) {
		v.logger.Debug("Received commit request for sequence outside valid range, ignoring",
			"receivedSequence", msg.Sequence,
			"currentSequence", v.Sequence)
		return nil
	}

	// If we receive a message for a future sequence, advance our sequence to catch up
	if msg.Sequence > v.Sequence {
		v.logger.Info("Received commit request for future sequence, advancing to catch up",
			"receivedSequence", msg.Sequence,
			"currentSequence", v.Sequence,
			"nodeID", v.config.NodeID)
		v.Sequence = msg.Sequence
		// Update metadata if available
		if v.config.Metadata != nil {
			v.config.Metadata.LatestSequence = v.Sequence
		}
	}

	v.logger.Info("Handling commit request message",
		"sequence", msg.Sequence,
		"from", msg.NodeID,
		"role", v.config.Role.String(),
		"nodeID", v.config.NodeID,
		"view", msg.View)

	// Store commit message
	if _, exists := v.commitMessages[msg.Sequence]; !exists {
		v.commitMessages[msg.Sequence] = make(map[NodeID]*CommitRequestMessage)
	}
	v.commitMessages[msg.Sequence][msg.NodeID] = msg

	// Store in-flight request info from proposal
	v.inFlightRequests[msg.Sequence] = &RequestInfo{
		ClientID: "batch",
		ID:       fmt.Sprintf("seq-%d", msg.Sequence),
	}

	// SmartBFT-style vote capture
	vote := &Vote{
		NodeID:    msg.NodeID,
		ShardID:   msg.ShardID,
		Approve:   true,
		Signature: msg.Signature,
	}

	// Capture vote for this sequence and check quorum
	if v.captureVoteForSequence(msg.Sequence, vote, "commit") {
		// Commit quorum reached - finalize
		v.logger.Info("Commit phase quorum reached via SmartBFT", "sequence", msg.Sequence)
		v.finalizeProposalAndCreateBlock(msg.Sequence, msg.Proposal)

		// If this is a shard leader, send commit ACK to primary leader
		if v.config.Role == RoleShardLeader && v.config.NodeID != v.config.PrimaryLeader {
			v.sendCommitAckToPrimary(msg.Sequence)
		}
	}

	// For shard leaders, also add their own vote if they haven't already
	if v.config.Role == RoleShardLeader && msg.NodeID != v.config.NodeID {
		commitDigest := ComputeCommitVoteDigest(v.config.ChannelID, v.Number, msg.Sequence, v.config.ShardID, v.config.NodeID, msg.Proposal.Digest())
		ownVote := &Vote{
			NodeID:    v.config.NodeID,
			ShardID:   v.config.ShardID,
			Approve:   true,
			Signature: v.signDigest(commitDigest),
		}
		if v.captureVoteForSequence(msg.Sequence, ownVote, "commit") {
			v.logger.Info("Commit phase quorum reached via SmartBFT (with own vote)", "sequence", msg.Sequence)
			v.finalizeProposalAndCreateBlock(msg.Sequence, msg.Proposal)

			// If this is a shard leader, send commit ACK to primary leader
			if v.config.Role == RoleShardLeader && v.config.NodeID != v.config.PrimaryLeader {
				v.sendCommitAckToPrimary(msg.Sequence)
			}
		}
	}

	// Handle role-specific forwarding
	switch v.config.Role {
	case RoleShardLeader:
		// Shard leaders forward Commit to their followers
		v.forwardCommitToFollowers(msg)
	case RoleShardFollower:
		// Followers send commit ACK back to their shard leader
		v.sendCommitAckToShardLeader(msg.Sequence)
	}

	return nil
}

// startPreparePhaseWithShardLeaders starts prepare phase by sending to shard leaders
func (v *View) startPreparePhaseWithShardLeaders(sequence uint64) {
	// Get the proposal from pre-prep messages
	var proposal Proposal
	if prePrepMsgs, exists := v.prePrepMessages[sequence]; exists {
		for _, msg := range prePrepMsgs {
			proposal = msg.Proposal
			break
		}
	}

	proposalDigest := proposal.Digest()
	prepDigest := ComputePrepareVoteDigest(v.config.ChannelID, v.Number, sequence, v.config.ShardID, v.config.NodeID, proposalDigest)

	prepareMsg := &PreparePhaseMessage{
		Proposal:  proposal,
		View:      v.Number,
		Sequence:  sequence,
		Digest:    proposalDigest,
		NodeID:    v.config.NodeID,
		ShardID:   v.config.ShardID,
		Signature: v.signDigest(prepDigest),
	}

	// Send to all other shard leaders (excluding primary itself)
	sentCount := 0
	for shardID, shardLeader := range v.config.ShardLeaders {
		if shardLeader != v.config.NodeID { // Don't send to self
			msg := Message{
				Type:      MsgPreparePhase,
				From:      v.config.NodeID,
				To:        shardLeader,
				ShardID:   shardID,
				Timestamp: time.Now(),
				Payload:   prepareMsg,
			}
			v.config.Network.Send(shardLeader, msg)
			v.logger.Info("Sent prepare phase to shard leader",
				"sequence", sequence,
				"shardLeader", shardLeader,
				"shardID", shardID)
			sentCount++
		}
	}

	v.logger.Info("Prepare phase distribution complete",
		"sequence", sequence,
		"sentToShardLeaders", sentCount)

	// If there are no other shard leaders, proceed directly to commit phase
	if sentCount == 0 {
		v.logger.Info("No other shard leaders - proceeding directly to commit phase",
			"sequence", sequence)
		v.startCommitPhaseWithShardLeaders(sequence)
	}
}

func (v *View) signDigest(digest []byte) []byte {
	if v.config.Signer != nil {
		sig, err := v.config.Signer.SignDigest(digest)
		if err == nil && len(sig) > 0 {
			return sig
		}
		return v.config.Signer.Sign(digest)
	}
	return nil
}

func (v *View) verifyDigestSignature(nodeID NodeID, digest []byte, sig []byte) error {
	if v.config.Verifier != nil && len(sig) > 0 {
		return v.config.Verifier.VerifyDigestSignature(nodeID, digest, sig)
	}
	return nil
}

func (v *View) signMessage(data []byte) []byte {
	if v.config.Signer != nil {
		return v.config.Signer.Sign(data)
	}
	return nil
}

// signProposalForCommit uses the proper signer for commit phase
func (v *View) signProposalForCommit(proposal Proposal) []byte {
	proposalDigest := proposal.Digest()
	commitDigest := ComputeCommitVoteDigest(v.config.ChannelID, v.Number, v.Sequence, v.config.ShardID, v.config.NodeID, proposalDigest)
	if v.config.Signer != nil {
		sig := v.signDigest(commitDigest)
		if len(sig) > 0 {
			return sig
		}
		signature := v.config.Signer.SignProposal(proposal, proposal.Payload)
		if signature != nil && len(signature.Value) > 0 {
			return signature.Value
		}
	}
	return v.signDigest(commitDigest)
}

// broadcastToShardNodes broadcasts a message to other nodes in the same shard for intra-shard consensus
func (v *View) broadcastToShardNodes(payload interface{}, phase string) {
	shardNodes := v.config.ShardNodes[v.config.ShardID]

	var sequence uint64
	var proposal Proposal
	var digest string
	var signature []byte

	// Extract common fields based on message type
	switch msg := payload.(type) {
	case *PrePrepMessage:
		sequence = msg.Sequence
		proposal = msg.Proposal
		digest = msg.Digest
	case *PreparePhaseMessage:
		sequence = msg.Sequence
		proposal = msg.Proposal
		digest = msg.Digest
	case *CommitRequestMessage:
		sequence = msg.Sequence
		proposal = msg.Proposal
		digest = msg.Digest
	default:
		v.logger.Error("Unknown message type for intra-shard broadcast")
		return
	}

	// Sign intra-shard vote with this node's own private key
	var ownDigest []byte
	switch phase {
	case "preprep":
		ownDigest = ComputePrePrepAckDigest(v.config.ChannelID, v.Number, sequence, v.config.ShardID, v.config.NodeID, "shard-ack")
	case "prepare":
		ownDigest = ComputePrepareVoteDigest(v.config.ChannelID, v.Number, sequence, v.config.ShardID, v.config.NodeID, digest)
	case "commit":
		ownDigest = ComputeCommitVoteDigest(v.config.ChannelID, v.Number, sequence, v.config.ShardID, v.config.NodeID, digest)
	default:
		ownDigest = ComputePrepareVoteDigest(v.config.ChannelID, v.Number, sequence, v.config.ShardID, v.config.NodeID, digest)
	}
	signature = v.signDigest(ownDigest)

	voteMsg := &IntraShardVoteMessage{
		Sequence:  sequence,
		Proposal:  proposal,
		Phase:     phase,
		ShardID:   v.config.ShardID,
		NodeID:    v.config.NodeID,
		Digest:    digest,
		Signature: signature,
		Timestamp: time.Now(),
	}

	// Send to all other nodes in the same shard
	for _, nodeID := range shardNodes {
		if nodeID != v.config.NodeID { // Don't send to self
			msg := Message{
				Type:      MsgIntraShardVote,
				From:      v.config.NodeID,
				To:        nodeID,
				ShardID:   v.config.ShardID,
				Timestamp: time.Now(),
				Payload:   voteMsg,
			}
			v.config.Network.Send(nodeID, msg)
		}
	}

	v.logger.Info("Broadcasted intra-shard vote request",
		"sequence", sequence,
		"phase", phase,
		"shardNodes", len(shardNodes)-1) // -1 because we don't send to self

	// Add our own vote
	v.recordIntraShardVote(sequence, phase, v.config.NodeID, true)

	// Check if we already have majority (in case there are only 2 nodes in shard)
	v.checkIntraShardMajority(sequence, phase)
}

// HandleIntraShardVote processes intra-shard vote requests
func (v *View) HandleIntraShardVote(voteMsg *IntraShardVoteMessage) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.logger.Info("Handling intra-shard vote request",
		"sequence", voteMsg.Sequence,
		"phase", voteMsg.Phase,
		"from", voteMsg.NodeID)

	// Validate the vote request
	if len(voteMsg.Signature) > 0 {
		var expectedDigest []byte
		switch voteMsg.Phase {
		case "preprep":
			expectedDigest = ComputePrePrepAckDigest(v.config.ChannelID, v.Number, voteMsg.Sequence, voteMsg.ShardID, voteMsg.NodeID, "shard-ack")
		case "prepare":
			expectedDigest = ComputePrepareVoteDigest(v.config.ChannelID, v.Number, voteMsg.Sequence, voteMsg.ShardID, voteMsg.NodeID, voteMsg.Digest)
		case "commit":
			expectedDigest = ComputeCommitVoteDigest(v.config.ChannelID, v.Number, voteMsg.Sequence, voteMsg.ShardID, voteMsg.NodeID, voteMsg.Digest)
		default:
			expectedDigest = ComputePrepareVoteDigest(v.config.ChannelID, v.Number, voteMsg.Sequence, voteMsg.ShardID, voteMsg.NodeID, voteMsg.Digest)
		}
		if err := v.verifyDigestSignature(voteMsg.NodeID, expectedDigest, voteMsg.Signature); err != nil {
			v.logger.Error("Invalid signature on IntraShardVote message", "from", voteMsg.NodeID, "error", err)
			return fmt.Errorf("invalid signature on IntraShardVote message: %w", err)
		}
	}

	vote := true // For now, always vote yes
	var voteDigest []byte
	switch voteMsg.Phase {
	case "preprep":
		voteDigest = ComputePrePrepAckDigest(v.config.ChannelID, v.Number, voteMsg.Sequence, v.config.ShardID, v.config.NodeID, "shard-ack")
	case "prepare":
		voteDigest = ComputePrepareVoteDigest(v.config.ChannelID, v.Number, voteMsg.Sequence, v.config.ShardID, v.config.NodeID, voteMsg.Digest)
	case "commit":
		voteDigest = ComputeCommitVoteDigest(v.config.ChannelID, v.Number, voteMsg.Sequence, v.config.ShardID, v.config.NodeID, voteMsg.Digest)
	}

	// Send vote response back
	response := &IntraShardVoteResponse{
		Sequence:  voteMsg.Sequence,
		Phase:     voteMsg.Phase,
		ShardID:   v.config.ShardID,
		NodeID:    v.config.NodeID,
		Digest:    voteMsg.Digest,
		Vote:      vote,
		Signature: v.signDigest(voteDigest),
		Timestamp: time.Now(),
	}

	msg := Message{
		Type:      MsgIntraShardVoteResponse,
		From:      v.config.NodeID,
		To:        voteMsg.NodeID,
		ShardID:   v.config.ShardID,
		Timestamp: time.Now(),
		Payload:   response,
	}

	v.config.Network.Send(voteMsg.NodeID, msg)
	v.logger.Info("Sent intra-shard vote response",
		"sequence", voteMsg.Sequence,
		"phase", voteMsg.Phase,
		"vote", vote,
		"to", voteMsg.NodeID)

	return nil
}

// HandleIntraShardVoteResponse processes intra-shard vote responses
func (v *View) HandleIntraShardVoteResponse(response *IntraShardVoteResponse) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.logger.Info("Handling intra-shard vote response",
		"sequence", response.Sequence,
		"phase", response.Phase,
		"from", response.NodeID,
		"vote", response.Vote)

	// Verify vote signature
	if len(response.Signature) > 0 {
		var voteDigest []byte
		switch response.Phase {
		case "preprep":
			voteDigest = ComputePrePrepAckDigest(v.config.ChannelID, v.Number, response.Sequence, response.ShardID, response.NodeID, "shard-ack")
		case "prepare":
			voteDigest = ComputePrepareVoteDigest(v.config.ChannelID, v.Number, response.Sequence, response.ShardID, response.NodeID, response.Digest)
		case "commit":
			voteDigest = ComputeCommitVoteDigest(v.config.ChannelID, v.Number, response.Sequence, response.ShardID, response.NodeID, response.Digest)
		}
		if len(voteDigest) > 0 {
			if err := v.verifyDigestSignature(response.NodeID, voteDigest, response.Signature); err != nil {
				v.logger.Error("Invalid signature on IntraShardVoteResponse message", "from", response.NodeID, "error", err)
				return fmt.Errorf("invalid signature on IntraShardVoteResponse message: %w", err)
			}
		}
	}

	// Record the vote
	v.recordIntraShardVote(response.Sequence, response.Phase, response.NodeID, response.Vote)

	// Check if we have majority
	v.checkIntraShardMajority(response.Sequence, response.Phase)

	return nil
}

// recordIntraShardVote records a vote from a shard node
func (v *View) recordIntraShardVote(sequence uint64, phase string, nodeID NodeID, vote bool) {
	if _, exists := v.intraShardVotes[sequence]; !exists {
		v.intraShardVotes[sequence] = make(map[string]map[NodeID]bool)
	}
	if _, exists := v.intraShardVotes[sequence][phase]; !exists {
		v.intraShardVotes[sequence][phase] = make(map[NodeID]bool)
	}
	v.intraShardVotes[sequence][phase][nodeID] = vote
}

// checkIntraShardMajority checks if we have majority votes and sends ACK to shard leader
func (v *View) checkIntraShardMajority(sequence uint64, phase string) {
	shardNodes := v.config.ShardNodes[v.config.ShardID]
	requiredCount := CalculateIntraShardQuorum(len(shardNodes))

	votes, exists := v.intraShardVotes[sequence][phase]
	if !exists {
		return
	}

	approveCount := 0
	totalVotes := 0
	for _, vote := range votes {
		totalVotes++
		if vote {
			approveCount++
		}
	}

	v.logger.Info("Checking intra-shard majority",
		"sequence", sequence,
		"phase", phase,
		"approveCount", approveCount,
		"totalVotes", totalVotes,
		"required", requiredCount)

	if approveCount >= requiredCount {
		// Majority reached within shard - send ACK to shard leader
		v.logger.Info("Intra-shard majority reached - sending ACK to shard leader",
			"sequence", sequence,
			"phase", phase)
		v.sendAckToShardLeaderDirect(sequence, phase)
	}
}

// sendAckToShardLeaderDirect sends ACK directly to shard leader after intra-shard consensus
func (v *View) sendAckToShardLeaderDirect(sequence uint64, phase string) {
	shardLeader := v.config.ShardLeaders[v.config.ShardID]

	// Don't send to self if this node is the shard leader
	if shardLeader == v.config.NodeID {
		v.logger.Info("This node is shard leader, processing ACK directly",
			"sequence", sequence,
			"phase", phase)
		// Process ACK directly as shard leader
		ack := &ShardAckMessage{
			Sequence:     sequence,
			ShardID:      v.config.ShardID,
			NodeID:       v.config.NodeID,
			Acknowledged: true,
			Phase:        phase,
			Timestamp:    time.Now(),
		}
		v.HandleShardAck(ack)
		return
	}

	ackDigest := ComputePrePrepAckDigest(v.config.ChannelID, v.Number, sequence, v.config.ShardID, v.config.NodeID, "follower-ack")
	ack := &ShardAckMessage{
		Sequence:     sequence,
		ShardID:      v.config.ShardID,
		NodeID:       v.config.NodeID,
		Acknowledged: true,
		Phase:        phase,
		Digest:       "follower-ack",
		Signature:    v.signDigest(ackDigest),
		Timestamp:    time.Now(),
	}

	msg := Message{
		Type:      MsgShardAck,
		From:      v.config.NodeID,
		To:        shardLeader,
		ShardID:   v.config.ShardID,
		Timestamp: time.Now(),
		Payload:   ack,
	}

	v.config.Network.Send(shardLeader, msg)
	v.logger.Info("Sent ACK to shard leader after intra-shard consensus",
		"sequence", sequence,
		"phase", phase,
		"shardLeader", shardLeader,
		"fromFollower", v.config.NodeID)
}

// GetPhase returns the current consensus phase
func (v *View) GetPhase() ViewPhase {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.phase
}

// GetViewNumber returns the view number
func (v *View) GetViewNumber() uint64 {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.Number
}

// isValidSequenceRange checks if a sequence is within the valid range (current or previous)
func (v *View) isValidSequenceRange(sequence uint64) bool {
	// Allow current sequence and previous sequence only
	if sequence == v.Sequence {
		return true
	}
	if v.Sequence > 0 && sequence == v.Sequence-1 {
		return true
	}
	// Also allow next sequence to handle synchronization issues where
	// primary leader advances faster than other nodes
	if sequence == v.Sequence+1 {
		return true
	}
	return false
}

// IsProposalInProgress checks if there's already a proposal in progress for the current sequence
func (v *View) IsProposalInProgress() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()

	// Check if sequence is already finalized
	if v.finalizedSequences[v.Sequence] {
		return false
	}

	// Check if there's already a proposal in progress for this sequence
	// Only consider it in progress if we have pre-prep messages for current sequence
	if prePrepMsgs, exists := v.prePrepMessages[v.Sequence]; exists && len(prePrepMsgs) > 0 {
		return true
	}

	return false
}

// Reset resets the view state for a new consensus round
func (v *View) Reset() {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.phase = ViewPhaseIdle
	v.prePrepMessages = make(map[uint64]map[NodeID]*PrePrepMessage)
	v.prepareMessages = make(map[uint64]map[NodeID]*PreparePhaseMessage)
	v.commitMessages = make(map[uint64]map[NodeID]*CommitRequestMessage)
	v.sequenceVotes = make(map[uint64]*SequenceVoteTracker)
	v.shardAcks = make(map[uint64]map[NodeID]*ShardAckMessage)
	v.inFlightRequests = make(map[uint64]*RequestInfo)
	v.finalizedSequences = make(map[uint64]bool)
	v.intraShardVotes = make(map[uint64]map[string]map[NodeID]bool)

	if v.viewTimer != nil {
		v.viewTimer.Stop()
	}
}

// forwardPrepareToFollowers forwards prepare phase to shard followers
func (v *View) forwardPrepareToFollowers(prepareMsg *PreparePhaseMessage) {
	shardNodes := v.config.ShardNodes[v.config.ShardID]
	for _, nodeID := range shardNodes {
		if nodeID != v.config.NodeID { // Don't send to self
			msg := Message{
				Type:      MsgPreparePhase,
				From:      v.config.NodeID,
				To:        nodeID,
				ShardID:   v.config.ShardID,
				Timestamp: time.Now(),
				Payload:   prepareMsg,
			}
			v.config.Network.Send(nodeID, msg)
			v.logger.Info("Forwarded prepare phase to follower",
				"sequence", prepareMsg.Sequence,
				"follower", nodeID)
		}
	}
}

// sendPrepareAckToShardLeader sends prepare ACK from follower to shard leader after getting intra-shard consensus
func (v *View) sendPrepareAckToShardLeader(sequence uint64) {
	// Get the proposal from prepare messages for broadcasting
	var proposal Proposal
	if prepareMsgs, exists := v.prepareMessages[sequence]; exists {
		for _, msg := range prepareMsgs {
			proposal = msg.Proposal
			break
		}
	}

	// Create a prepare message for intra-shard consensus
	prepareMsg := &PreparePhaseMessage{
		Proposal:  proposal,
		View:      v.Number,
		Sequence:  sequence,
		Digest:    proposal.Digest(),
		NodeID:    v.config.NodeID,
		ShardID:   v.config.ShardID,
		Signature: v.signMessage(proposal.Payload),
	}

	// Broadcast to other nodes in the same shard to get consensus
	v.broadcastToShardNodes(prepareMsg, "prepare")
}

// startCommitPhaseWithShardLeaders starts commit phase by sending to shard leaders
func (v *View) startCommitPhaseWithShardLeaders(sequence uint64) {
	// Get the proposal from prepare messages
	var proposal Proposal
	if prepareMsgs, exists := v.prepareMessages[sequence]; exists {
		for _, msg := range prepareMsgs {
			proposal = msg.Proposal
			break
		}
	} else if prePrepMsgs, exists := v.prePrepMessages[sequence]; exists {
		for _, msg := range prePrepMsgs {
			proposal = msg.Proposal
			break
		}
	}

	commitMsg := &CommitRequestMessage{
		Proposal:  proposal,
		View:      v.Number,
		Sequence:  sequence,
		Digest:    proposal.Digest(),
		NodeID:    v.config.NodeID,
		ShardID:   v.config.ShardID,
		Signature: v.signProposalForCommit(proposal),
	}

	// Send to all other shard leaders
	sentCount := 0
	for shardID, shardLeader := range v.config.ShardLeaders {
		if shardLeader != v.config.NodeID { // Don't send to self
			msg := Message{
				Type:      MsgCommitRequest,
				From:      v.config.NodeID,
				To:        shardLeader,
				ShardID:   shardID,
				Timestamp: time.Now(),
				Payload:   commitMsg,
			}
			v.config.Network.Send(shardLeader, msg)
			v.logger.Info("Sent commit request to shard leader",
				"sequence", sequence,
				"shardLeader", shardLeader,
				"shardID", shardID)
			sentCount++
		}
	}

	v.logger.Info("Commit phase distribution complete",
		"sequence", sequence,
		"sentToShardLeaders", sentCount)

	// If there are no other shard leaders, proceed directly to finalization
	if sentCount == 0 {
		v.logger.Info("No other shard leaders - proceeding directly to finalization",
			"sequence", sequence)
		v.finalizeProposalAndCreateBlock(sequence, proposal)
	}
}

// forwardCommitToFollowers forwards commit request to shard followers
func (v *View) forwardCommitToFollowers(commitMsg *CommitRequestMessage) {
	shardNodes := v.config.ShardNodes[v.config.ShardID]
	for _, nodeID := range shardNodes {
		if nodeID != v.config.NodeID { // Don't send to self
			msg := Message{
				Type:      MsgCommitRequest,
				From:      v.config.NodeID,
				To:        nodeID,
				ShardID:   v.config.ShardID,
				Timestamp: time.Now(),
				Payload:   commitMsg,
			}
			v.config.Network.Send(nodeID, msg)
			v.logger.Info("Forwarded commit request to follower",
				"sequence", commitMsg.Sequence,
				"follower", nodeID)
		}
	}
}

// sendCommitAckToShardLeader sends commit ACK from follower to shard leader after getting intra-shard consensus
func (v *View) sendCommitAckToShardLeader(sequence uint64) {
	// Get the proposal from commit messages for broadcasting
	var proposal Proposal
	if commitMsgs, exists := v.commitMessages[sequence]; exists {
		for _, msg := range commitMsgs {
			proposal = msg.Proposal
			break
		}
	}

	// Create a commit message for intra-shard consensus
	commitMsg := &CommitRequestMessage{
		Proposal:  proposal,
		View:      v.Number,
		Sequence:  sequence,
		Digest:    proposal.Digest(),
		NodeID:    v.config.NodeID,
		ShardID:   v.config.ShardID,
		Signature: v.signProposalForCommit(proposal),
	}

	// Broadcast to other nodes in the same shard to get consensus
	v.broadcastToShardNodes(commitMsg, "commit")
}

// sendCommitAckToPrimary sends commit acknowledgment from shard leader to primary leader
func (v *View) sendCommitAckToPrimary(sequence uint64) {
	ack := &ShardAckMessage{
		Sequence:     sequence,
		ShardID:      v.config.ShardID,
		NodeID:       v.config.NodeID,
		Acknowledged: true,
		Phase:        "commit",
		Timestamp:    time.Now(),
	}

	msg := Message{
		Type:      MsgShardAck,
		From:      v.config.NodeID,
		To:        v.config.PrimaryLeader,
		ShardID:   v.config.ShardID,
		Timestamp: time.Now(),
		Payload:   ack,
	}

	v.config.Network.Send(v.config.PrimaryLeader, msg)
	v.logger.Info("Sent commit ACK to primary leader",
		"sequence", sequence,
		"primaryLeader", v.config.PrimaryLeader,
		"fromShardLeader", v.config.NodeID)
}

// checkPrimaryCommitQuorum checks if BFT cross-shard quorum of shard leaders have committed
func (v *View) checkPrimaryCommitQuorum(sequence uint64) {
	// Count commit ACKs from shard leaders
	commitAckCount := 0
	shardQCs := make(map[ShardID]*ShardQC)

	if acks, exists := v.shardAcks[sequence]; exists {
		for _, ack := range acks {
			if ack.Phase == "commit" && ack.Acknowledged {
				commitAckCount++
				if len(ack.Signatures) > 0 {
					shardQCs[ack.ShardID] = CreateShardQC(v.config.ChannelID, v.Number, sequence, "commit", ack.ShardID, ack.Digest, ack.Signatures)
				}
			}
		}
	}

	totalShards := len(v.config.ShardLeaders)
	requiredCount := CalculateCrossShardQuorum(totalShards)

	otherShardLeaders := 0
	for _, shardLeader := range v.config.ShardLeaders {
		if shardLeader != v.config.NodeID {
			otherShardLeaders++
		}
	}

	v.logger.Info("Checking primary commit quorum",
		"sequence", sequence,
		"commitAckCount", commitAckCount,
		"requiredCount", requiredCount,
		"totalShards", totalShards,
		"otherShardLeaders", otherShardLeaders)

	// If there are no other shard leaders, or we have reached cross-shard quorum, finalize
	if otherShardLeaders == 0 || commitAckCount >= requiredCount {
		v.logger.Info("Primary commit quorum reached - consensus complete", "sequence", sequence)
		// Get the proposal from pre-prepare messages (primary leader has these)
		var proposal Proposal
		if prePrepMsgs, exists := v.prePrepMessages[sequence]; exists {
			for _, msg := range prePrepMsgs {
				proposal = msg.Proposal
				break
			}
		} else if commitMsgs, exists := v.commitMessages[sequence]; exists {
			// Fallback to commit messages if pre-prep not available
			for _, msg := range commitMsgs {
				proposal = msg.Proposal
				break
			}
		}
		v.finalizeProposalAndCreateBlock(sequence, proposal)
	}
}

// storeProposalAsBlock stores a finalized proposal as a block
func (v *View) storeProposalAsBlock(proposal Proposal, sequence uint64) *Block {
	v.logger.Info("Storing proposal as block",
		"sequence", sequence,
		"payloadSize", len(proposal.Payload))

	// Use the consensus storage method to store the block
	if v.config.Storage != nil {
		// Get the latest block to determine the sequence and previous hash
		var blockSequence int64 = 1
		var prevHash string

		if latestBlock, err := v.config.Storage.GetLatestBlock(); err == nil {
			blockSequence = latestBlock.Sequence + 1
			prevHash = fmt.Sprintf("%x", sha256.Sum256(latestBlock.ToBytes()))
		}

		// Create transaction from proposal payload
		transaction := Transaction{
			ClientID: "consensus",
			TS:       int(time.Now().UnixNano() / 1000000),
			ID:       fmt.Sprintf("seq-%d", sequence),
			Data:     base64.StdEncoding.EncodeToString(proposal.Payload),
		}

		// Create block using the Block type
		block := &Block{
			Sequence:     blockSequence,
			PrevHash:     prevHash,
			Metadata:     []byte(fmt.Sprintf(`{"view":%d,"sequence":%d,"digest":"%s"}`, v.Number, sequence, proposal.Digest())),
			Transactions: []Transaction{transaction},
		}

		// Store block using the storage interface
		if err := v.config.Storage.StoreBlock(block); err != nil {
			v.logger.Error("Failed to store block", "error", err, "sequence", blockSequence)
			return block
		}

		v.logger.Info("✅ Block stored successfully",
			"blockSequence", block.Sequence,
			"prevHash", prevHash,
			"viewSequence", sequence,
			"nodeID", v.config.NodeID)

		return block
	} else {
		v.logger.Info("No storage configured - block not persisted", "sequence", sequence)
		return nil
	}
}

// cleanupSequenceTrackingData cleans up tracking data for a completed sequence
func (v *View) cleanupSequenceTrackingData(sequence uint64) {
	delete(v.prePrepMessages, sequence)
	delete(v.prepareMessages, sequence)
	delete(v.commitMessages, sequence)
	delete(v.shardAcks, sequence)
	delete(v.inFlightRequests, sequence)

	// Also clean up any sequences that are now too old (more than 1 behind current)
	v.cleanupOldSequences()
}

// cleanupOldSequences removes data for sequences that are outside the valid range
func (v *View) cleanupOldSequences() {
	minValidSequence := uint64(0)
	if v.Sequence > 1 {
		minValidSequence = v.Sequence - 1
	}

	// Clean up old pre-prep messages
	for seq := range v.prePrepMessages {
		if seq < minValidSequence {
			delete(v.prePrepMessages, seq)
		}
	}

	// Clean up old prepare messages
	for seq := range v.prepareMessages {
		if seq < minValidSequence {
			delete(v.prepareMessages, seq)
		}
	}

	// Clean up old commit messages
	for seq := range v.commitMessages {
		if seq < minValidSequence {
			delete(v.commitMessages, seq)
		}
	}

	// Clean up old shard acks
	for seq := range v.shardAcks {
		if seq < minValidSequence {
			delete(v.shardAcks, seq)
		}
	}

	// Clean up old in-flight requests
	for seq := range v.inFlightRequests {
		if seq < minValidSequence {
			delete(v.inFlightRequests, seq)
		}
	}

	// Clean up old sequence votes
	for seq := range v.sequenceVotes {
		if seq < minValidSequence {
			delete(v.sequenceVotes, seq)
		}
	}

	// Clean up old finalized sequences (keep only recent ones)
	for seq := range v.finalizedSequences {
		if seq < minValidSequence {
			delete(v.finalizedSequences, seq)
		}
	}
}

// captureVoteForSequence captures votes for SmartBFT-style sequence-based consensus
func (v *View) captureVoteForSequence(sequence uint64, vote *Vote, phase string) bool {
	// Check if sequence is already finalized
	if v.finalizedSequences[sequence] {
		v.logger.Debug("Sequence already finalized, ignoring vote",
			"sequence", sequence,
			"phase", phase,
			"voter", vote.NodeID)
		return false
	}

	// Handle sequence synchronization
	if !v.isValidSequenceRange(sequence) {
		v.logger.Debug("Received vote for sequence outside valid range, ignoring",
			"receivedSequence", sequence,
			"currentSequence", v.Sequence,
			"phase", phase,
			"voter", vote.NodeID)
		return false
	}

	// If we receive a vote for a future sequence, advance our sequence to catch up
	if sequence > v.Sequence {
		v.logger.Info("Received vote for future sequence, advancing to catch up",
			"receivedSequence", sequence,
			"currentSequence", v.Sequence,
			"nodeID", v.config.NodeID,
			"phase", phase,
			"voter", vote.NodeID)
		v.Sequence = sequence
		// Update metadata if available
		if v.config.Metadata != nil {
			v.config.Metadata.LatestSequence = v.Sequence
		}
	}

	// Initialize tracker if not exists
	if v.sequenceVotes[sequence] == nil {
		v.sequenceVotes[sequence] = &SequenceVoteTracker{
			Sequence:      sequence,
			Votes:         make(map[NodeID]*Vote),
			QuorumReached: make(map[string]bool),
			Timestamp:     time.Now(),
		}
	}

	tracker := v.sequenceVotes[sequence]

	// Add/update vote for this node
	tracker.Votes[vote.NodeID] = vote

	// Check quorum for this phase
	if !tracker.QuorumReached[phase] {
		tracker.QuorumReached[phase] = v.checkSequenceQuorum(tracker, phase)

		if tracker.QuorumReached[phase] {
			v.logger.Info("SmartBFT sequence quorum reached",
				"view", v.Number,
				"sequence", tracker.Sequence,
				"phase", phase,
				"voteCount", len(tracker.Votes))
			return true
		}
	}

	return false
}

// checkSequenceQuorum checks if quorum is reached for a sequence
func (v *View) checkSequenceQuorum(tracker *SequenceVoteTracker, phase string) bool {
	switch v.config.Role {
	case RolePrimaryLeader:
		return v.checkCrossShardSequenceQuorum(tracker, phase)
	case RoleShardLeader:
		return v.checkIntraShardSequenceQuorum(tracker, phase)
	}
	return false
}

// checkCrossShardSequenceQuorum checks cross-shard quorum for primary leader
func (v *View) checkCrossShardSequenceQuorum(tracker *SequenceVoteTracker, phase string) bool {
	totalShards := len(v.config.ShardLeaders)
	f := (totalShards - 1) / 3
	required := 2*f + 1

	// For small number of shards, require majority participation
	// This ensures proper cross-shard coordination
	if totalShards <= 3 {
		required = totalShards // Require all shards for small clusters
	} else {
		// For larger clusters, require at least 2/3 of shards
		required = (totalShards * 2) / 3
		if required < 2*f+1 {
			required = 2*f + 1
		}
	}

	// Count shard approvals for this sequence, including own shard
	shardApprovals := make(map[ShardID]bool)
	for _, vote := range tracker.Votes {
		if vote.Approve {
			shardApprovals[vote.ShardID] = true
		}
	}

	// Always count our own shard as approved for primary leader
	if v.config.Role == RolePrimaryLeader {
		shardApprovals[v.config.ShardID] = true
	}

	approved := len(shardApprovals)

	v.logger.Debug("SmartBFT cross-shard sequence quorum check",
		"view", v.Number,
		"sequence", tracker.Sequence,
		"phase", phase,
		"approved", approved,
		"required", required,
		"totalShards", totalShards,
		"f", f,
		"shardApprovals", shardApprovals)

	return approved >= required
}

// checkIntraShardSequenceQuorum checks intra-shard quorum for shard leaders
func (v *View) checkIntraShardSequenceQuorum(tracker *SequenceVoteTracker, phase string) bool {
	shardNodes := v.config.ShardNodes[v.config.ShardID]
	n := len(shardNodes)
	f := (n - 1) / 3
	required := 2*f + 1

	// For single node shards, always approve
	if n == 1 {
		required = 1
	}

	// Count approvals in this shard, including own vote
	approved := 0
	hasOwnVote := false
	for _, vote := range tracker.Votes {
		if vote.Approve && vote.ShardID == v.config.ShardID {
			approved++
			if vote.NodeID == v.config.NodeID {
				hasOwnVote = true
			}
		}
	}

	// If we haven't counted our own vote yet, add it
	if !hasOwnVote && v.config.Role == RoleShardLeader {
		approved++
	}

	v.logger.Debug("SmartBFT intra-shard sequence quorum check",
		"view", v.Number,
		"sequence", tracker.Sequence,
		"phase", phase,
		"approved", approved,
		"required", required,
		"n", n,
		"f", f,
		"hasOwnVote", hasOwnVote)

	return approved >= required
}

// forwardFinalizedBlockToFollowers forwards finalized blocks to shard followers
func (v *View) forwardFinalizedBlockToFollowers(sequence uint64, proposal Proposal) {
	shardNodes := v.config.ShardNodes[v.config.ShardID]
	for _, nodeID := range shardNodes {
		if nodeID != v.config.NodeID { // Don't send to self
			// Create a finalized block message for followers
			finalizedMsg := &FinalizedBlockMessage{
				Sequence:  sequence,
				Proposal:  proposal,
				ShardID:   v.config.ShardID,
				LeaderID:  v.config.NodeID,
				Timestamp: time.Now(),
			}

			msg := Message{
				Type:      MsgFinalizedBlock,
				From:      v.config.NodeID,
				To:        nodeID,
				ShardID:   v.config.ShardID,
				Timestamp: time.Now(),
				Payload:   finalizedMsg,
			}

			v.config.Network.Send(nodeID, msg)
			v.logger.Info("Forwarded finalized block to follower",
				"sequence", sequence,
				"follower", nodeID,
				"shardID", v.config.ShardID)
		}
	}
}

// cleanupSequenceTracking cleans up vote tracking for a completed sequence
func (v *View) cleanupSequenceTracking(sequence uint64) {
	delete(v.sequenceVotes, sequence)
	v.logger.Debug("Cleaned up SmartBFT sequence vote tracker",
		"view", v.Number,
		"sequence", sequence)
}

// finalizeProposalAndCreateBlock finalizes the proposal and creates a block
func (v *View) finalizeProposalAndCreateBlock(sequence uint64, proposal Proposal) {
	// Check if already finalized to prevent double processing
	if v.finalizedSequences[sequence] {
		v.logger.Debug("Sequence already finalized, skipping duplicate finalization", "sequence", sequence)
		return
	}

	// Mark as finalized to prevent further processing
	v.finalizedSequences[sequence] = true

	v.logger.Info("Finalizing proposal and creating block",
		"sequence", sequence,
		"view", v.Number,
		"nodeID", v.config.NodeID,
		"currentSequence", v.Sequence)

	// Deliver proposal to application for block creation and storage
	if v.config.Application != nil {
		if err := v.config.Application.Deliver(proposal); err != nil {
			v.logger.Error("Failed to deliver proposal to application",
				"error", err,
				"sequence", sequence)
		}
	} else if v.config.Storage != nil {
		// Fallback to direct storage if no application delivery is configured
		v.storeProposalAsBlock(proposal, sequence)
	}

	// If this is a shard leader, forward the finalized block to followers
	if v.config.Role == RoleShardLeader {
		v.forwardFinalizedBlockToFollowers(sequence, proposal)
	}

	// Clean up SmartBFT sequence tracking
	v.cleanupSequenceTracking(sequence)

	// Clean up sequence tracking data
	v.cleanupSequenceTrackingData(sequence)

	// Increment sequence for next consensus round (SmartBFT style)
	// Only increment if this is the current sequence being processed
	if sequence == v.Sequence {
		v.Sequence++
		v.DecisionsInView++

		// Update metadata with new sequence
		if v.config.Metadata != nil {
			v.config.Metadata.LatestSequence = v.Sequence
			v.config.Metadata.DecisionsInView = v.DecisionsInView
		}

		// Clean up old sequences now that we've advanced
		v.cleanupOldSequences()

		v.logger.Info("Sequence advanced after successful consensus",
			"view", v.Number,
			"newSequence", v.Sequence,
			"finalizedSequence", sequence,
			"decisionsInView", v.DecisionsInView)
	} else {
		v.logger.Info("Finalized out-of-order sequence, not advancing current sequence",
			"finalizedSequence", sequence,
			"currentSequence", v.Sequence)
	}
}

// handlePhaseQuorumReached handles phase transitions when SmartBFT quorum is reached
func (v *View) handlePhaseQuorumReached(sequence uint64, phase string) {
	v.logger.Info("phase quorum reached",
		"view", v.Number,
		"sequence", sequence,
		"phase", phase)

	switch phase {
	case "preprep":
		v.startPreparePhaseWithShardLeaders(sequence)
	case "prepare":
		v.startCommitPhaseWithShardLeaders(sequence)
	case "commit":
		// Get the proposal from the most reliable source
		var proposal Proposal

		// First try to get from commit messages
		if commitMsgs, exists := v.commitMessages[sequence]; exists {
			for _, msg := range commitMsgs {
				proposal = msg.Proposal
				break
			}
		}

		// If no proposal found in commit messages, try prepare messages
		if proposal.Payload == nil {
			if prepareMsgs, exists := v.prepareMessages[sequence]; exists {
				for _, msg := range prepareMsgs {
					proposal = msg.Proposal
					break
				}
			}
		}

		// If still no proposal found, try pre-prep messages (for primary leader)
		if proposal.Payload == nil {
			if prePrepMsgs, exists := v.prePrepMessages[sequence]; exists {
				for _, msg := range prePrepMsgs {
					proposal = msg.Proposal
					break
				}
			}
		}

		v.finalizeProposalAndCreateBlock(sequence, proposal)
	}
}
