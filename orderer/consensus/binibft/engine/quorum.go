package engine

import (
	"fmt"
	"slices"
)

// CalculateMaxByzantineNodes computes the maximum number of Byzantine faulty nodes f = floor((n-1)/3)
func CalculateMaxByzantineNodes(n int) int {
	if n < 4 {
		return 0
	}
	return (n - 1) / 3
}

// CalculateIntraShardQuorum computes the Byzantine fault tolerant quorum size for an intra-shard committee
// Formula: Q_intra = 2*f + 1 = 2*floor((n-1)/3) + 1 (with graceful handling for small clusters)
func CalculateIntraShardQuorum(shardNodesCount int) int {
	if shardNodesCount <= 0 {
		return 0
	}
	if shardNodesCount == 1 {
		return 1
	}
	if shardNodesCount <= 3 {
		return 2
	}
	f := (shardNodesCount - 1) / 3
	return 2*f + 1
}

// CalculateCrossShardQuorum computes the Byzantine fault tolerant quorum size across shard leaders
// Formula: Q_cross = 2*f_shards + 1 = 2*floor((S-1)/3) + 1
func CalculateCrossShardQuorum(totalShardsCount int) int {
	if totalShardsCount <= 0 {
		return 0
	}
	if totalShardsCount == 1 {
		return 1
	}
	if totalShardsCount <= 3 {
		return 2
	}
	f := (totalShardsCount - 1) / 3
	return 2*f + 1
}

// CreateShardQC constructs an intra-shard Quorum Certificate
func CreateShardQC(
	channelID string,
	view uint64,
	sequence uint64,
	phase string,
	shardID ShardID,
	digest string,
	signatures map[NodeID][]byte,
) *ShardQC {
	sigsCopy := make(map[NodeID][]byte, len(signatures))
	for k, v := range signatures {
		sigCopy := make([]byte, len(v))
		copy(sigCopy, v)
		sigsCopy[k] = sigCopy
	}
	return &ShardQC{
		ChannelID:  channelID,
		View:       view,
		Sequence:   sequence,
		Phase:      phase,
		ShardID:    shardID,
		Digest:     digest,
		Signatures: sigsCopy,
	}
}

// VerifyShardQC verifies that a ShardQC has at least minQuorum valid signatures from verified shard members
func VerifyShardQC(
	verifier Verifier,
	shardQC *ShardQC,
	shardNodes []NodeID,
	minQuorum int,
) error {
	if shardQC == nil {
		return fmt.Errorf("nil ShardQC")
	}

	if len(shardQC.Signatures) < minQuorum {
		return fmt.Errorf("insufficient signatures in ShardQC: got %d, required %d", len(shardQC.Signatures), minQuorum)
	}

	validCount := 0
	for nodeID, sig := range shardQC.Signatures {
		if !slices.Contains(shardNodes, nodeID) {
			return fmt.Errorf("signer %s is not a recognized member of shard %d", nodeID, shardQC.ShardID)
		}

		if len(sig) == 0 {
			return fmt.Errorf("empty signature from node %s in ShardQC", nodeID)
		}

		if verifier != nil {
			var expectedDigest []byte
			switch shardQC.Phase {
			case "preprep":
				expectedDigest = ComputePrePrepAckDigest(shardQC.ChannelID, shardQC.View, shardQC.Sequence, shardQC.ShardID, nodeID, shardQC.Digest)
			case "prepare":
				expectedDigest = ComputePrepareVoteDigest(shardQC.ChannelID, shardQC.View, shardQC.Sequence, shardQC.ShardID, nodeID, shardQC.Digest)
			case "commit":
				expectedDigest = ComputeCommitVoteDigest(shardQC.ChannelID, shardQC.View, shardQC.Sequence, shardQC.ShardID, nodeID, shardQC.Digest)
			default:
				expectedDigest = ComputePrepareVoteDigest(shardQC.ChannelID, shardQC.View, shardQC.Sequence, shardQC.ShardID, nodeID, shardQC.Digest)
			}

			if err := verifier.VerifyDigestSignature(nodeID, expectedDigest, sig); err != nil {
				return fmt.Errorf("signature verification failed for node %s in ShardQC: %w", nodeID, err)
			}
		}

		validCount++
	}

	if validCount < minQuorum {
		return fmt.Errorf("valid signature count %d is below required quorum %d", validCount, minQuorum)
	}

	return nil
}

// CreateCommitQC constructs a cross-shard Commit Quorum Certificate
func CreateCommitQC(
	channelID string,
	view uint64,
	sequence uint64,
	digest string,
	shardQCs map[ShardID]*ShardQC,
) *CommitQC {
	qcMap := make(map[ShardID]*ShardQC, len(shardQCs))
	for k, v := range shardQCs {
		qcMap[k] = v
	}
	return &CommitQC{
		ChannelID: channelID,
		View:      view,
		Sequence:  sequence,
		Digest:    digest,
		ShardQCs:  qcMap,
	}
}

// VerifyCommitQC verifies that a CommitQC contains valid ShardQCs from at least minCrossShardQuorum distinct shards
func VerifyCommitQC(
	verifier Verifier,
	commitQC *CommitQC,
	shardNodesMap map[ShardID][]NodeID,
	minCrossShardQuorum int,
) error {
	if commitQC == nil {
		return fmt.Errorf("nil CommitQC")
	}

	if len(commitQC.ShardQCs) < minCrossShardQuorum {
		return fmt.Errorf("insufficient ShardQCs in CommitQC: got %d, required %d", len(commitQC.ShardQCs), minCrossShardQuorum)
	}

	for shardID, shardQC := range commitQC.ShardQCs {
		if shardQC == nil {
			return fmt.Errorf("nil ShardQC for shard %d in CommitQC", shardID)
		}

		if shardQC.Sequence != commitQC.Sequence {
			return fmt.Errorf("sequence mismatch in ShardQC for shard %d: expected %d, got %d", shardID, commitQC.Sequence, shardQC.Sequence)
		}

		if shardQC.View != commitQC.View {
			return fmt.Errorf("view mismatch in ShardQC for shard %d: expected %d, got %d", shardID, commitQC.View, shardQC.View)
		}

		shardNodes, exists := shardNodesMap[shardID]
		if !exists || len(shardNodes) == 0 {
			return fmt.Errorf("unknown shard ID %d in CommitQC", shardID)
		}

		minIntraQuorum := CalculateIntraShardQuorum(len(shardNodes))
		if err := VerifyShardQC(verifier, shardQC, shardNodes, minIntraQuorum); err != nil {
			return fmt.Errorf("invalid ShardQC for shard %d: %w", shardID, err)
		}
	}

	return nil
}
