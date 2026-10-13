package engine

import (
	"github.com/hyperledger/fabric/orderer/consensus/binibft/engine/protos"
	"context"
	"crypto/sha256"
	"encoding/asn1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// NodeRole defines the role of a node in the hierarchical consensus
type NodeRole int

const (
	RolePrimaryLeader NodeRole = iota
	RoleShardLeader
	RoleShardFollower
)

func (r NodeRole) String() string {
	switch r {
	case RolePrimaryLeader:
		return "PrimaryLeader"
	case RoleShardLeader:
		return "ShardLeader"
	case RoleShardFollower:
		return "ShardFollower"
	default:
		return "Unknown"
	}
}

// NodeID represents a unique identifier for a node
type NodeID string

// ShardID represents a unique identifier for a shard
type ShardID uint32

// Proposal represents a consensus proposal
type Proposal struct {
	Payload              []byte
	Header               []byte
	Metadata             []byte
	VerificationSequence int64 // int64 for asn1 marshaling
}

func (p Proposal) Digest() string {
	rawBytes, err := asn1.Marshal(Proposal{
		VerificationSequence: p.VerificationSequence,
		Metadata:             p.Metadata,
		Payload:              p.Payload,
		Header:               p.Header,
	})
	if err != nil {
		panic(fmt.Sprintf("failed marshaling proposal: %v", err))
	}

	return computeDigest(rawBytes)
}

func computeDigest(rawBytes []byte) string {
	h := sha256.New()
	h.Write(rawBytes)
	digest := h.Sum(nil)
	return hex.EncodeToString(digest)
}

// Vote represents a vote on a proposal
type Vote struct {
	ProposalID string
	NodeID     NodeID
	ShardID    ShardID
	Approve    bool
	Signature  []byte
}

// Decision represents a consensus decision
type Decision struct {
	ProposalID string
	Committed  bool
	ShardVotes map[ShardID][]Vote
	Timestamp  time.Time
	BatchID    string // For batch decisions
}

// Transaction represents a transaction within a block
type Transaction struct {
	ClientID string `json:"clientID"`
	TS       int    `json:"ts"`
	ID       string `json:"id"`
	Data     string `json:"data"`
}

// Block represents a block containing transactions
type Block struct {
	Sequence     int64         `json:"sequence"`
	PrevHash     string        `json:"prevHash"`
	Metadata     []byte        `json:"metadata"`
	Transactions []Transaction `json:"transactions"`
}

func (block Block) ToBytes() []byte {
	raw, err := json.Marshal(block)
	if err != nil {
		return nil
	}
	return raw
}

func (block Block) ToBytesChecked() ([]byte, error) {
	return json.Marshal(block)
}

func BlockFromBytes(raw []byte) (*Block, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty block bytes")
	}
	var block Block
	if err := json.Unmarshal(raw, &block); err != nil {
		return nil, fmt.Errorf("failed to unmarshal block: %w", err)
	}
	return &block, nil
}

func (txn Transaction) ToBytes() []byte {
	raw, err := json.Marshal(txn)
	if err != nil {
		return nil
	}
	return raw
}

func (txn Transaction) ToBytesChecked() ([]byte, error) {
	return json.Marshal(txn)
}

func TransactionFromBytes(raw []byte) (*Transaction, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty transaction bytes")
	}
	var txn Transaction
	if err := json.Unmarshal(raw, &txn); err != nil {
		return nil, fmt.Errorf("failed to unmarshal transaction: %w", err)
	}
	return &txn, nil
}

type BlockHeader struct {
	Sequence int64
	PrevHash string
	DataHash string
}

func (header BlockHeader) ToBytes() []byte {
	raw, err := asn1.Marshal(header)
	if err != nil {
		return nil
	}
	return raw
}

func (header BlockHeader) ToBytesChecked() ([]byte, error) {
	return asn1.Marshal(header)
}

func BlockHeaderFromBytes(raw []byte) (*BlockHeader, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty block header bytes")
	}
	var header BlockHeader
	if _, err := asn1.Unmarshal(raw, &header); err != nil {
		return nil, fmt.Errorf("failed to unmarshal block header: %w", err)
	}
	return &header, nil
}

type BlockData struct {
	Transactions [][]byte
}

func (b BlockData) ToBytes() []byte {
	raw, err := asn1.Marshal(b)
	if err != nil {
		return nil
	}
	return raw
}

func (b BlockData) ToBytesChecked() ([]byte, error) {
	return asn1.Marshal(b)
}

func BlockDataFromBytes(raw []byte) (*BlockData, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty block data bytes")
	}
	var block BlockData
	if _, err := asn1.Unmarshal(raw, &block); err != nil {
		return nil, fmt.Errorf("failed to unmarshal block data: %w", err)
	}
	return &block, nil
}

// Config holds the configuration for the consensus instance
type Config struct {
	NodeID NodeID

	RequestBatchMaxCount uint64
	// RequestBatchMaxBytes is the maximal total size of requests in a batch, in bytes.
	// This is also the maximal size of a request. A request batch that reaches this size is proposed immediately.
	RequestBatchMaxBytes uint64
	// RequestBatchMaxInterval is the maximal time interval a request batch is waiting before it is proposed.
	// A request batch is accumulating requests until RequestBatchMaxInterval had elapsed from the time the batch was
	// first created (i.e. the time the first request was added to it), or until it is of count RequestBatchMaxCount,
	// or total size RequestBatchMaxBytes, which ever happens first.
	RequestBatchMaxInterval time.Duration

	ShardID           ShardID
	Role              NodeRole
	ShardNodes        map[ShardID][]NodeID
	PrimaryLeader     NodeID
	ShardLeaders      map[ShardID]NodeID
	MajorityRequired  int
	Timeout           time.Duration
	ViewChangeTimeout time.Duration
	Network           NetworkInterface
	Logger            Logger
	// Byzantine fault tolerance settings
	MaxFaultyNodes   int
	SigningKey       []byte
	VerificationKeys map[NodeID][]byte
	// Hierarchical sharding settings
	ShardMajorityThreshold float64       // Percentage of nodes required for shard consensus (0.5-1.0)
	CrossShardThreshold    float64       // Percentage of shards required for global consensus (0.5-1.0)
	BatchSize              int           // Number of proposals to batch together
	MaxBatchDelay          time.Duration // Maximum time to wait before processing a batch
	// Storage
	Storage          BlockStorage // Storage for blocks and transactions
	WalStorage       BlockStorage // Storage for blocks and transactions
	Metadata         *protos.ViewMetadata
	Assembler        Assembler
	RequestInspector RequestInspector
	// Application delivery interface
	Application ApplicationDelivery
	// Signer for signing proposals and messages
	Signer Signer
	// Verifier for verifying node signatures and quorum certificates
	Verifier Verifier
	// ChannelID for multi-channel domain separation
	ChannelID string
	// Node reference for role updates
	Node NodeUpdater // Interface for node role updates
}

// ConsensusInterface defines the main consensus operations
type ConsensusInterface interface {
	Start(ctx context.Context) error
	Stop() error
	Propose(proposal *Proposal) error
	GetStatus() Status
}

// NetworkInterface defines network operations
type NetworkInterface interface {
	Send(nodeID NodeID, message Message) error
	Broadcast(nodeIDs []NodeID, message Message) error
	RegisterHandler(handler MessageHandler)
	SendTransaction(targetID NodeID, request []byte) error
}

// MessageHandler handles incoming messages
type MessageHandler interface {
	HandleMessage(from NodeID, message Message) error
}

// Logger interface for logging
type Logger interface {
	Info(msg string, fields ...interface{})
	Error(msg string, fields ...interface{})
	Debug(msg string, fields ...interface{})
}

// ApplicationDelivery interface for delivering finalized proposals to the application
type ApplicationDelivery interface {
	Deliver(proposal Proposal) error
	ReceiveHeartbeat(from NodeID, timestamp time.Time)
}

// Signer interface for signing proposals and messages
type Signer interface {
	SignProposal(proposal Proposal, data []byte) *Signature
	Sign(msg []byte) []byte
	SignDigest(digest []byte) ([]byte, error)
}

// Verifier interface for verifying node signatures, proposals, and requests
type Verifier interface {
	VerifySignature(nodeID NodeID, data []byte, signature []byte) error
	VerifyDigestSignature(nodeID NodeID, digest []byte, signature []byte) error
	VerifyProposalSignature(nodeID NodeID, proposal Proposal, sig Signature) error
	VerifyRequest(request []byte) (RequestInfo, error)
	VerifyProposal(proposal Proposal) ([]RequestInfo, error)
	RequestsFromProposal(proposal Proposal) []RequestInfo
}

// ShardQC represents an intra-shard quorum certificate
type ShardQC struct {
	ChannelID  string
	View       uint64
	Sequence   uint64
	Phase      string // "preprep", "prepare", "commit"
	ShardID    ShardID
	Digest     string
	Signatures map[NodeID][]byte
}

// CommitQC represents a cross-shard commit quorum certificate
type CommitQC struct {
	ChannelID string
	View      uint64
	Sequence  uint64
	Digest    string
	ShardQCs  map[ShardID]*ShardQC
}

// NodeUpdater interface for updating node roles and configuration
type NodeUpdater interface {
	UpdateNodeRole(newRole NodeRole)
	UpdateNodeConfig(primaryId NodeID, shardLeaders map[ShardID]NodeID, shardNodes map[ShardID][]NodeID)
}

// Status represents the current status of the consensus
type Status struct {
	Role         NodeRole
	ShardID      ShardID
	IsActive     bool
	LastDecision time.Time
	CurrentView  uint64
	IsPrimary    bool
}

// ViewChange represents a view change request
type ViewChange struct {
	NewView   uint64
	NodeID    NodeID
	ShardID   ShardID
	Reason    string
	Timestamp time.Time
	Signature []byte
}

// ViewChangeRequest represents a request to change views
type ViewChangeRequest struct {
	NewView   uint64
	NodeID    NodeID
	ShardID   ShardID
	Reason    string
	Timestamp time.Time
	Signature []byte
}

// RequestPhase represents the phase of request processing
type RequestPhase int

const (
	PhasePrePrep RequestPhase = iota
	PhasePrepare
	PhaseCommit
)

func (p RequestPhase) String() string {
	switch p {
	case PhasePrePrep:
		return "PrePrep"
	case PhasePrepare:
		return "Prepare"
	case PhaseCommit:
		return "Commit"
	default:
		return "Unknown"
	}
}

// Request represents a client request in the request pool
type Request struct {
	ID        string
	Data      []byte
	ClientID  string
	Timestamp time.Time
	Phase     RequestPhase
	ShardID   ShardID
}

// PrepareMessage represents a prepare phase message
type PrepareMessage struct {
	Proposal  Proposal
	View      uint64
	Sequence  uint64
	Digest    []byte
	NodeID    NodeID
	Signature []byte
}

// CommitPhaseMessage represents a commit phase message
type CommitPhaseMessage struct {
	View       uint64
	Sequence   uint64
	ProposalID string
	Digest     []byte
	NodeID     NodeID
	Signature  []byte
}

type RequestInfo struct {
	ClientID string
	ID       string
}

type Signature struct {
	ID    uint64
	Value []byte
	Msg   []byte
}
