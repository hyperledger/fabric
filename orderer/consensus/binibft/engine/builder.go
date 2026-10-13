package engine

import (
	"github.com/hyperledger/fabric/orderer/consensus/binibft/engine/protos"
	"fmt"
	"log/slog"
	"os"
	"time"
)

// ConsensusBuilder provides a fluent interface for building consensus instances
type ConsensusBuilder struct {
	config *Config
	err    error
}

// NewConsensusBuilder creates a new consensus builder
func NewConsensusBuilder() *ConsensusBuilder {
	return &ConsensusBuilder{
		config: &Config{
			ShardNodes:             make(map[ShardID][]NodeID),
			ShardLeaders:           make(map[ShardID]NodeID),
			ShardMajorityThreshold: 0.67,
			CrossShardThreshold:    0.51,
			Timeout:                5 * time.Second,
			ViewChangeTimeout:      30 * time.Second,
			BatchSize:              10,
			MaxBatchDelay:          500 * time.Millisecond,
		},
	}
}

// WithNode sets the node configuration
func (b *ConsensusBuilder) WithNode(nodeID NodeID, shardID ShardID, role NodeRole) *ConsensusBuilder {
	if b.err != nil {
		return b
	}

	b.config.NodeID = nodeID
	b.config.ShardID = shardID
	b.config.Role = role

	return b
}

// WithPrimaryLeader sets the primary leader
func (b *ConsensusBuilder) WithPrimaryLeader(nodeID NodeID) *ConsensusBuilder {
	if b.err != nil {
		return b
	}

	b.config.PrimaryLeader = nodeID

	return b
}

// WithShard adds a shard configuration
func (b *ConsensusBuilder) WithShard(shardID ShardID, leaderID NodeID, followerIDs []NodeID) *ConsensusBuilder {
	if b.err != nil {
		return b
	}

	// Add shard leader
	b.config.ShardLeaders[shardID] = leaderID

	// Add shard nodes (leader + followers)
	nodes := make([]NodeID, 0, len(followerIDs)+1)
	nodes = append(nodes, leaderID)
	nodes = append(nodes, followerIDs...)
	b.config.ShardNodes[shardID] = nodes

	return b
}

// WithNetwork sets the network interface
func (b *ConsensusBuilder) WithNetwork(network NetworkInterface) *ConsensusBuilder {
	if b.err != nil {
		return b
	}

	b.config.Network = network

	return b
}

// WithStorage sets the block storage
func (b *ConsensusBuilder) WithStorage(walStorage, storage BlockStorage) *ConsensusBuilder {
	if b.err != nil {
		return b
	}

	b.config.WalStorage = storage
	b.config.Storage = storage

	return b
}

// WithLogger sets the logger
func (b *ConsensusBuilder) WithLogger(logger Logger) *ConsensusBuilder {
	if b.err != nil {
		return b
	}

	b.config.Logger = logger

	return b
}

// WithTimeout sets the consensus timeout
func (b *ConsensusBuilder) WithTimeout(timeout time.Duration) *ConsensusBuilder {
	if b.err != nil {
		return b
	}

	b.config.Timeout = timeout

	return b
}

// WithViewChangeTimeout sets the view change timeout
func (b *ConsensusBuilder) WithViewChangeTimeout(timeout time.Duration) *ConsensusBuilder {
	if b.err != nil {
		return b
	}

	b.config.ViewChangeTimeout = timeout

	return b
}

// WithShardMajorityThreshold sets the shard majority threshold
func (b *ConsensusBuilder) WithShardMajorityThreshold(threshold float64) *ConsensusBuilder {
	if b.err != nil {
		return b
	}

	if threshold < 0.5 || threshold > 1.0 {
		b.err = fmt.Errorf("shard majority threshold must be between 0.5 and 1.0")
		return b
	}

	b.config.ShardMajorityThreshold = threshold

	return b
}

// WithCrossShardThreshold sets the cross-shard threshold
func (b *ConsensusBuilder) WithCrossShardThreshold(threshold float64) *ConsensusBuilder {
	if b.err != nil {
		return b
	}

	if threshold < 0.5 || threshold > 1.0 {
		b.err = fmt.Errorf("cross-shard threshold must be between 0.5 and 1.0")
		return b
	}

	b.config.CrossShardThreshold = threshold

	return b
}

// WithBatchingConfig sets the batching configuration
func (b *ConsensusBuilder) WithBatchingConfig(batchSize int, maxDelay time.Duration) *ConsensusBuilder {
	if b.err != nil {
		return b
	}
	b.config.RequestBatchMaxBytes = 10 * 1024 * 1024
	b.config.RequestBatchMaxCount = uint64(batchSize)
	b.config.RequestBatchMaxInterval = maxDelay

	return b
}

// WithBFTConfig sets the Byzantine fault tolerance configuration
func (b *ConsensusBuilder) WithBFTConfig(maxFaultyNodes int, signingKey []byte, verificationKeys map[NodeID][]byte) *ConsensusBuilder {
	if b.err != nil {
		return b
	}

	b.config.MaxFaultyNodes = maxFaultyNodes
	b.config.SigningKey = signingKey
	b.config.VerificationKeys = verificationKeys

	return b
}

func (b *ConsensusBuilder) WithViewMetaData(metadata *protos.ViewMetadata) *ConsensusBuilder {
	if b.err != nil {
		return b
	}

	b.config.Metadata = metadata

	return b
}

func (b *ConsensusBuilder) WitAssembler(assembler Assembler) *ConsensusBuilder {
	if b.err != nil {
		return b
	}

	b.config.Assembler = assembler

	return b
}

func (b *ConsensusBuilder) WithRequestInspector(reqInspector RequestInspector) *ConsensusBuilder {
	if b.err != nil {
		return b
	}

	b.config.RequestInspector = reqInspector

	return b
}

func (b *ConsensusBuilder) WithApplication(app ApplicationDelivery) *ConsensusBuilder {
	if b.err != nil {
		return b
	}

	b.config.Application = app

	return b
}

func (b *ConsensusBuilder) WithSigner(signer Signer) *ConsensusBuilder {
	if b.err != nil {
		return b
	}

	b.config.Signer = signer

	return b
}

func (b *ConsensusBuilder) WithVerifier(verifier Verifier) *ConsensusBuilder {
	if b.err != nil {
		return b
	}

	b.config.Verifier = verifier

	return b
}

func (b *ConsensusBuilder) WithChannelID(channelID string) *ConsensusBuilder {
	if b.err != nil {
		return b
	}

	b.config.ChannelID = channelID

	return b
}

// Build creates a new consensus instance
func (b *ConsensusBuilder) Build() (*Consensus, error) {
	if b.err != nil {
		return nil, b.err
	}

	// Validate configuration
	if b.config.NodeID == "" {
		return nil, fmt.Errorf("node ID is required")
	}

	if b.config.Network == nil {
		return nil, fmt.Errorf("network interface is required")
	}

	if b.config.Logger == nil {
		// Use default logger if none provided
		b.config.Logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))
	}

	// Create consensus instance
	consensus := NewConsensus(b.config)

	return consensus, nil
}
