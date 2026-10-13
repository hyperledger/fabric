package engine

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
)

// Domain separation constants to prevent cross-phase and cross-domain replay attacks
const (
	DomainProposal        = "BiniBFT/proposal/v1"
	DomainPrePrep         = "BiniBFT/preprep/v1"
	DomainPrePrepAck      = "BiniBFT/preprep-ack/v1"
	DomainPrepareVote     = "BiniBFT/prepare-vote/v1"
	DomainShardPrepareQC  = "BiniBFT/shard-prepare-qc/v1"
	DomainCommitVote      = "BiniBFT/commit-vote/v1"
	DomainShardCommitQC   = "BiniBFT/shard-commit-qc/v1"
	DomainViewChange      = "BiniBFT/view-change/v1"
	DomainLeaderElection  = "BiniBFT/leader-election/v1"
	DomainElectionAck     = "BiniBFT/election-ack/v1"
	DomainShardAssignment = "BiniBFT/shard-assignment/v1"
	DomainHeartbeat       = "BiniBFT/heartbeat/v1"
	DomainRequest         = "BiniBFT/request/v1"
)

type sha256DigestWriter struct {
	buf []byte
}

func newDigestWriter() *sha256DigestWriter {
	return &sha256DigestWriter{buf: make([]byte, 0, 256)}
}

func (dw *sha256DigestWriter) writeLengthPrefixed(data []byte) {
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(data)))
	dw.buf = append(dw.buf, lenBuf...)
	dw.buf = append(dw.buf, data...)
}

func (dw *sha256DigestWriter) writeUint64(val uint64) {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, val)
	dw.buf = append(dw.buf, buf...)
}

func (dw *sha256DigestWriter) writeUint32(val uint32) {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, val)
	dw.buf = append(dw.buf, buf...)
}

func (dw *sha256DigestWriter) sum() []byte {
	h := sha256.Sum256(dw.buf)
	return h[:]
}

func (dw *sha256DigestWriter) sumHex() string {
	return hex.EncodeToString(dw.sum())
}

// ComputeCanonicalProposalDigest calculates the canonical digest for a proposal
func ComputeCanonicalProposalDigest(channelID string, view uint64, sequence uint64, header []byte, payload []byte, metadata []byte) string {
	dw := newDigestWriter()
	dw.writeLengthPrefixed([]byte(DomainProposal))
	dw.writeLengthPrefixed([]byte(channelID))
	dw.writeUint64(view)
	dw.writeUint64(sequence)
	dw.writeLengthPrefixed(header)
	dw.writeLengthPrefixed(payload)
	dw.writeLengthPrefixed(metadata)
	return dw.sumHex()
}

// ComputePrePrepDigest calculates the digest for a Pre-Prepare message
func ComputePrePrepDigest(channelID string, view uint64, sequence uint64, proposalDigest string) []byte {
	dw := newDigestWriter()
	dw.writeLengthPrefixed([]byte(DomainPrePrep))
	dw.writeLengthPrefixed([]byte(channelID))
	dw.writeUint64(view)
	dw.writeUint64(sequence)
	dw.writeLengthPrefixed([]byte(proposalDigest))
	return dw.sum()
}

// ComputePrePrepAckDigest calculates the digest for a Pre-Prepare ACK from a follower
func ComputePrePrepAckDigest(channelID string, view uint64, sequence uint64, shardID ShardID, nodeID NodeID, proposalDigest string) []byte {
	dw := newDigestWriter()
	dw.writeLengthPrefixed([]byte(DomainPrePrepAck))
	dw.writeLengthPrefixed([]byte(channelID))
	dw.writeUint64(view)
	dw.writeUint64(sequence)
	dw.writeUint32(uint32(shardID))
	dw.writeLengthPrefixed([]byte(nodeID))
	dw.writeLengthPrefixed([]byte(proposalDigest))
	return dw.sum()
}

// ComputePrepareVoteDigest calculates the digest for a Prepare vote
func ComputePrepareVoteDigest(channelID string, view uint64, sequence uint64, shardID ShardID, nodeID NodeID, proposalDigest string) []byte {
	dw := newDigestWriter()
	dw.writeLengthPrefixed([]byte(DomainPrepareVote))
	dw.writeLengthPrefixed([]byte(channelID))
	dw.writeUint64(view)
	dw.writeUint64(sequence)
	dw.writeUint32(uint32(shardID))
	dw.writeLengthPrefixed([]byte(nodeID))
	dw.writeLengthPrefixed([]byte(proposalDigest))
	return dw.sum()
}

// ComputeCommitVoteDigest calculates the digest for a Commit vote
func ComputeCommitVoteDigest(channelID string, view uint64, sequence uint64, shardID ShardID, nodeID NodeID, proposalDigest string) []byte {
	dw := newDigestWriter()
	dw.writeLengthPrefixed([]byte(DomainCommitVote))
	dw.writeLengthPrefixed([]byte(channelID))
	dw.writeUint64(view)
	dw.writeUint64(sequence)
	dw.writeUint32(uint32(shardID))
	dw.writeLengthPrefixed([]byte(nodeID))
	dw.writeLengthPrefixed([]byte(proposalDigest))
	return dw.sum()
}

// ComputeViewChangeDigest calculates the digest for a View Change message
func ComputeViewChangeDigest(channelID string, newView uint64, shardID ShardID, nodeID NodeID, reason string) []byte {
	dw := newDigestWriter()
	dw.writeLengthPrefixed([]byte(DomainViewChange))
	dw.writeLengthPrefixed([]byte(channelID))
	dw.writeUint64(newView)
	dw.writeUint32(uint32(shardID))
	dw.writeLengthPrefixed([]byte(nodeID))
	dw.writeLengthPrefixed([]byte(reason))
	return dw.sum()
}

// ComputeElectionDigest calculates the digest for a Leader Election message
func ComputeElectionDigest(channelID string, term uint64, electionID string, candidateID NodeID) []byte {
	dw := newDigestWriter()
	dw.writeLengthPrefixed([]byte(DomainLeaderElection))
	dw.writeLengthPrefixed([]byte(channelID))
	dw.writeUint64(term)
	dw.writeLengthPrefixed([]byte(electionID))
	dw.writeLengthPrefixed([]byte(candidateID))
	return dw.sum()
}

// ComputeTransactionDigest calculates a deterministic transaction digest
func ComputeTransactionDigest(channelID string, clientID string, ts int, data string) string {
	dw := newDigestWriter()
	dw.writeLengthPrefixed([]byte(DomainRequest))
	dw.writeLengthPrefixed([]byte(channelID))
	dw.writeLengthPrefixed([]byte(clientID))
	dw.writeUint64(uint64(ts))
	dw.writeLengthPrefixed([]byte(data))
	return dw.sumHex()
}
