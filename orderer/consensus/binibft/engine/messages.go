package engine

import "time"

// MessageType defines the type of consensus message
type MessageType int

const (
	MsgProposal MessageType = iota
	MsgVote
	MsgShardDecision
	MsgHeartbeat
	MsgViewChange
	MsgPrepare
	MsgCommitPhase
	MsgNewView
	MsgBatchProposal
	MsgRequest
	MsgPrePrep
	MsgPreparePhase
	MsgCommitRequest
	MsgCrossShardRequest
	MsgShardAck
	MsgFinalizedBlock
	MsgIntraShardVote
	MsgIntraShardVoteResponse
	MsgLeaderElection
	MsgElectionAck
	MsgShardAssignment
	MsgLeaderAnnouncement
	MsgSyncRequest
	MsgSyncResponse
)

// Message represents a consensus message
type Message struct {
	Type      MessageType
	From      NodeID
	To        NodeID
	ShardID   ShardID
	Timestamp time.Time
	Payload   interface{}
}

// ProposalMessage contains proposal data
type ProposalMessage struct {
	Proposal *Proposal
}

// VoteMessage contains vote data
type VoteMessage struct {
	Vote *Vote
}

// ShardDecisionMessage contains shard-level decision
type ShardDecisionMessage struct {
	ShardID   ShardID
	Decision  bool
	Votes     []Vote
	Signature []byte
	BatchID   string // For batch decisions
}

// CommitMessage signals final commitment
type CommitMessage struct {
	ProposalID     string
	ShardDecisions map[ShardID]*ShardDecisionMessage
	FinalDecision  bool
	RequestData    *CommitRequestData // Include request data for finalization
}

// CommitRequestData contains request data for finalization
type CommitRequestData struct {
	ID        string
	Data      []byte
	ClientID  string
	Timestamp time.Time
	ShardID   ShardID
}

// HeartbeatMessage for liveness detection
type HeartbeatMessage struct {
	NodeID    NodeID
	ShardID   ShardID
	Timestamp time.Time
}

// RequestMessage contains client request data
type RequestMessage struct {
	Request *Request
}

// PrePrepMessage for pre-preparation phase
type PrePrepMessage struct {
	Proposal  Proposal
	View      uint64
	Sequence  uint64
	Digest    string
	NodeID    NodeID
	ShardID   ShardID
	Signature []byte
}

// PrepMessage for preparation phase
type PrepMessage struct {
	Proposal  Proposal
	View      uint64
	Sequence  uint64
	Digest    string
	NodeID    NodeID
	ShardID   ShardID
	Signature []byte
}

// PreparePhaseMessage for prepare phase
type PreparePhaseMessage struct {
	Proposal  Proposal
	View      uint64
	Sequence  uint64
	Digest    string
	NodeID    NodeID
	ShardID   ShardID
	Signature []byte
}

// CommitRequestMessage for commit phase
type CommitRequestMessage struct {
	Proposal  Proposal
	View      uint64
	Sequence  uint64
	Digest    string
	NodeID    NodeID
	ShardID   ShardID
	Signature []byte
}

// CrossShardRequestMessage for cross-shard request coordination
type CrossShardRequestMessage struct {
	Request     *Request
	OriginShard ShardID
	TargetShard ShardID
	Timestamp   time.Time
}

// ShardAckMessage for shard acknowledgment
type ShardAckMessage struct {
	Sequence     uint64
	ShardID      ShardID
	NodeID       NodeID
	Acknowledged bool
	Phase        string // "preprep", "prepare", "commit"
	Digest       string
	Signature    []byte
	Signatures   map[NodeID][]byte // Collected follower signatures for the shard quorum
	Timestamp    time.Time
}

// FinalizedBlockMessage for forwarding finalized blocks to followers
type FinalizedBlockMessage struct {
	Sequence  uint64
	Proposal  Proposal
	ShardID   ShardID
	LeaderID  NodeID
	Timestamp time.Time
}

// IntraShardVoteMessage for requesting votes within a shard
type IntraShardVoteMessage struct {
	Sequence  uint64
	Proposal  Proposal
	Phase     string // "preprep", "prepare", "commit"
	ShardID   ShardID
	NodeID    NodeID
	Digest    string
	Signature []byte
	Timestamp time.Time
}

// IntraShardVoteResponse for responding to intra-shard vote requests
type IntraShardVoteResponse struct {
	Sequence  uint64
	Phase     string
	ShardID   ShardID
	NodeID    NodeID
	Digest    string
	Vote      bool // true for approve, false for reject
	Signature []byte
	Timestamp time.Time
}

// LeaderElectionMessage for initiating leader election
type LeaderElectionMessage struct {
	ElectionID   string
	CandidateID  NodeID
	ActiveNodes  []NodeID
	Term         uint64
	VRFOutput    []byte
	VRFProof     []byte
	Timestamp    time.Time
	Signature    []byte
}

// ElectionAckMessage for acknowledging election participation
type ElectionAckMessage struct {
	ElectionID   string
	NodeID       NodeID
	Term         uint64
	VRFOutput    []byte
	VRFProof     []byte
	Acknowledged bool
	Timestamp    time.Time
	Signature    []byte
}

// ShardAssignmentMessage for announcing shard assignments and leaders
type ShardAssignmentMessage struct {
	ElectionID     string
	PrimaryLeader  NodeID
	ShardLeaders   map[ShardID]NodeID
	ShardNodes     map[ShardID][]NodeID
	Timestamp      time.Time
	Signature      []byte
}

// LeaderAnnouncementMessage for announcing new leaders
type LeaderAnnouncementMessage struct {
	ElectionID    string
	PrimaryLeader NodeID
	ShardLeaders  map[ShardID]NodeID
	ShardNodes    map[ShardID][]NodeID
	Timestamp     time.Time
	Signature     []byte
}

// SyncRequestMessage requests a range of blocks for catch-up synchronization
type SyncRequestMessage struct {
	FromNodeID    NodeID
	ShardID       ShardID
	StartSequence uint64
	EndSequence   uint64
	Timestamp     time.Time
	Signature     []byte
}

// SyncResponseMessage delivers requested blocks and QC proofs for catch-up synchronization
type SyncResponseMessage struct {
	FromNodeID  NodeID
	ShardID     ShardID
	Blocks      []*Block
	CommitQCs   map[uint64]*CommitQC
	LatestSeq   uint64
	Error       string
	Timestamp   time.Time
	Signature   []byte
}
