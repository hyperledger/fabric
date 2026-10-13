package engine

import (
	"fmt"
	"sync"
	"time"
)

// ConsensusMetrics tracks performance metrics for the consensus protocol
type ConsensusMetrics struct {
	mu                sync.RWMutex
	proposalsReceived int
	votesReceived     int
	decisionsReached  int

	// Latency metrics
	commitLatencies       []time.Duration
	shardConsensusLatency map[ShardID][]time.Duration
	crossShardLatency     []time.Duration
}

// NewConsensusMetrics creates a new metrics instance
func NewConsensusMetrics() *ConsensusMetrics {
	return &ConsensusMetrics{
		shardConsensusLatency: make(map[ShardID][]time.Duration),
	}
}

// RecordProposalReceived increments the proposals received counter
func (cm *ConsensusMetrics) RecordProposalReceived() {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.proposalsReceived++
}

// RecordVoteReceived increments the votes received counter
func (cm *ConsensusMetrics) RecordVoteReceived() {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.votesReceived++
}

// RecordDecisionReached increments the decisions reached counter
func (cm *ConsensusMetrics) RecordDecisionReached() {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.decisionsReached++
}

// RecordCommitLatency records the latency for a commit
func (cm *ConsensusMetrics) RecordCommitLatency(latency time.Duration) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.commitLatencies = append(cm.commitLatencies, latency)
}

// RecordShardConsensusTime records the time taken for shard consensus
func (cm *ConsensusMetrics) RecordShardConsensusTime(shardID ShardID, latency time.Duration) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if _, exists := cm.shardConsensusLatency[shardID]; !exists {
		cm.shardConsensusLatency[shardID] = make([]time.Duration, 0)
	}
	cm.shardConsensusLatency[shardID] = append(cm.shardConsensusLatency[shardID], latency)
}

// RecordCrossShardTime records the time taken for cross-shard consensus
func (cm *ConsensusMetrics) RecordCrossShardTime(latency time.Duration) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.crossShardLatency = append(cm.crossShardLatency, latency)
}

// GetMetrics returns the current metrics
func (cm *ConsensusMetrics) GetMetrics() map[string]interface{} {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	metrics := map[string]interface{}{
		"proposals_received": cm.proposalsReceived,
		"votes_received":     cm.votesReceived,
		"decisions_reached":  cm.decisionsReached,
	}

	// Calculate average commit latency
	if len(cm.commitLatencies) > 0 {
		var total time.Duration
		for _, latency := range cm.commitLatencies {
			total += latency
		}
		metrics["avg_commit_latency_ms"] = float64(total.Milliseconds()) / float64(len(cm.commitLatencies))
	}

	// Calculate average shard consensus latency
	shardLatencies := make(map[string]float64)
	for shardID, latencies := range cm.shardConsensusLatency {
		if len(latencies) > 0 {
			var total time.Duration
			for _, latency := range latencies {
				total += latency
			}
			shardLatencies[fmt.Sprintf("shard_%d", shardID)] = float64(total.Milliseconds()) / float64(len(latencies))
		}
	}
	metrics["shard_consensus_latency_ms"] = shardLatencies

	// Calculate average cross-shard consensus latency
	if len(cm.crossShardLatency) > 0 {
		var total time.Duration
		for _, latency := range cm.crossShardLatency {
			total += latency
		}
		metrics["avg_cross_shard_latency_ms"] = float64(total.Milliseconds()) / float64(len(cm.crossShardLatency))
	}

	return metrics
}
