/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package status

import (
	"bytes"
	"fmt"
	"math"
	"strings"
)

type EpochTargetState int

const (
	// EpochPrepending indicates we have sent an epoch-change, but waiting for a quorum
	EpochPrepending = iota

	// EpochPending indicates that we have a quorum of epoch-change messages, waits on new-epoch
	EpochPending

	// EpochVerifying indicates we have received a new view message but it references epoch changes we cannot yet verify
	EpochVerifying

	// EpochFetching indicates we have received and verified a new epoch messages, and are waiting to get state
	EpochFetching

	// EpochEchoing indicates we have received and validated a new-epoch, waiting for a quorum of echos
	EpochEchoing

	// EpochReadying indicates we have received a quorum of echos, waiting a on qourum of readies
	EpochReadying

	// EpochReady indicates the new epoch is ready to begin
	EpochReady

	// EpochInProgress indicates the epoch is currently active
	EpochInProgress

	// EpochDone indicates this epoch has ended, either gracefully or because we sent an epoch change
	EpochDone
)

type SequenceState int

const (
	// SequenceUnitialized indicates no batch has been assigned to this sequence.
	SequenceUninitialized SequenceState = iota

	// SequenceAllocated indicates that a potentially valid batch has been assigned to this sequence.
	SequenceAllocated

	// SequencePendingRequests indicates that we are waiting for missing requests to arrive or be validated.
	SequencePendingRequests

	// SequenceReady indicates that we have all requests and are ready to proceed with the 3-phase commit.
	SequenceReady

	// SequencePreprepared indicates that we have sent a Prepare/Preprepare as follow/leader respectively.
	SequencePreprepared

	// SequencePreprepared indicates that we have sent a Commit message.
	SequencePrepared

	// SequenceCommitted indicates that we have a quorum of commit messages and the sequence is
	// eligible to commit.  Note though, that all prior sequences must commit prior to the consumer
	// seeing this commit event.
	SequenceCommitted
)

type StateMachine struct {
	NodeID        uint64           `json:"node_id"`
	LowWatermark  uint64           `json:"low_watermark"`
	HighWatermark uint64           `json:"high_watermark"`
	EpochTracker  *EpochTracker    `json:"epoch_tracker"`
	NodeBuffers   []*NodeBuffer    `json:"node_buffers"`
	Buckets       []*Bucket        `json:"buckets"`
	Checkpoints   []*Checkpoint    `json:"checkpoints"`
	ClientWindows []*ClientTracker `json:"client_tracker"`
}

type Bucket struct {
	ID        uint64          `json:"id"`
	Leader    bool            `json:"leader"`
	Sequences []SequenceState `json:"sequences"`
}

type Checkpoint struct {
	SeqNo         uint64 `json:"seq_no"`
	MaxAgreements int    `json:"max_agreements"`
	NetQuorum     bool   `json:"net_quorum"`
	LocalDecision bool   `json:"local_decision"`
}

type EpochTracker struct {
	ActiveEpoch *EpochTarget `json:"last_active_epoch"`
}

type EpochTarget struct {
	Number       uint64           `json:"number"`
	State        EpochTargetState `json:"state"`
	EpochChanges []*EpochChange   `json:"epoch_changes"`
	Echos        []uint64         `json:"echos"`
	Readies      []uint64         `json:"readies"`
	Suspicions   []uint64         `json:"suspicions"`
	Leaders      []uint64         `json:"leaders"`
}

type EpochChange struct {
	Source uint64            `json:"source"`
	Msgs   []*EpochChangeMsg `json:"messages"`
}

type EpochChangeMsg struct {
	Digest []byte   `json:"digest"`
	Acks   []uint64 `json:"acks"`
}

type NodeBuffer struct {
	ID         uint64       `json:"id"`
	Size       int          `json:"size"`
	Msgs       int          `json:"msgs"`
	MsgBuffers []*MsgBuffer `json:"msg_buffers"`
}

type MsgBuffer struct {
	Component string `json:"component"`
	Size      int    `json:"size"`
	Msgs      int    `json:"msgs"`
}

// Used for sorting MsgBuffers
// (e.g. to produce deterministic output after iteration over a map).
// Returns a value > 0 if mb is "greater than" other,
//                 < 0 if mb is "smaller than" other,
//                 0 if mb and other are equal.
// Definition of greater / smaller is arbitrary.
func (mb *MsgBuffer) Compare(other *MsgBuffer) int {
	if mb.Size != other.Size {
		return mb.Size - other.Size
	} else if mb.Msgs != other.Msgs {
		return mb.Msgs - other.Msgs
	} else {
		return strings.Compare(mb.Component, other.Component)
	}
}

type NodeBucket struct {
	BucketID    int    `json:"bucket_id"`
	IsLeader    bool   `json:"is_leader"`
	LastPrepare uint64 `json:"last_prepare"`
	LastCommit  uint64 `json:"last_commit"`
}

type ClientTracker struct {
	ClientID      uint64   `json:"client_id"`
	LowWatermark  uint64   `json:"low_watermark"`
	HighWatermark uint64   `json:"high_watermark"`
	Allocated     []uint64 `json:"allocated"`
}

func (s *StateMachine) Pretty() string {
	var buffer bytes.Buffer
	fmt.Fprintf(&buffer, "===========================================\n")
	fmt.Fprintf(&buffer, "NodeID=%d, LowWatermark=%d, HighWatermark=%d, Epoch=%d\n", s.NodeID, s.LowWatermark, s.HighWatermark, s.EpochTracker.ActiveEpoch.Number)
	fmt.Fprintf(&buffer, "===========================================\n\n")

	fmt.Fprintf(&buffer, "=== Epoch Number %d ===\n", s.EpochTracker.ActiveEpoch.Number)
	fmt.Fprintf(&buffer, "Epoch is in state: %d\n", s.EpochTracker.ActiveEpoch.State)

	et := s.EpochTracker.ActiveEpoch
	fmt.Fprintf(&buffer, "  EpochChanges:\n")
	for _, ec := range et.EpochChanges {
		for _, ecm := range ec.Msgs {
			fmt.Fprintf(&buffer, "    Source=%d Digest=%.4x Acks=%v\n", ec.Source, ecm.Digest, ecm.Acks)
		}
	}
	fmt.Fprintf(&buffer, "  Echos: %v\n", et.Echos)
	fmt.Fprintf(&buffer, "  Readies: %v\n", et.Readies)
	fmt.Fprintf(&buffer, "  Suspicions: %v\n", et.Suspicions)
	fmt.Fprintf(&buffer, "  Leaders: %v\n", et.Leaders)
	fmt.Fprintf(&buffer, "\n")
	fmt.Fprintf(&buffer, "=====================\n")
	fmt.Fprintf(&buffer, "\n")

	hRule := func() {
		for seqNo := s.LowWatermark; seqNo <= s.HighWatermark; seqNo += uint64(len(s.Buckets)) {
			fmt.Fprintf(&buffer, "--")
		}
	}

	if s.LowWatermark == s.HighWatermark {
		fmt.Fprintf(&buffer, "=== Empty Watermarks ===\n")
	} else {
		if s.HighWatermark-s.LowWatermark > 10000 {
			fmt.Fprintf(&buffer, "=== Suspiciously wide watermarks [%d, %d] ===\n", s.LowWatermark, s.HighWatermark)
			return buffer.String()
		}

		for i := len(fmt.Sprintf("%d", s.HighWatermark)); i > 0; i-- {
			magnitude := math.Pow10(i - 1)
			for seqNo := s.LowWatermark; seqNo <= s.HighWatermark; seqNo += uint64(len(s.Buckets)) {
				fmt.Fprintf(&buffer, " %d", seqNo/uint64(magnitude)%10)
			}
			fmt.Fprintf(&buffer, "\n")
		}

		hRule()
		fmt.Fprintf(&buffer, "- === Buckets ===\n")

		for _, bucketBuffer := range s.Buckets {
			for _, state := range bucketBuffer.Sequences {
				switch state {
				case SequenceUninitialized:
					fmt.Fprintf(&buffer, "| ")
				case SequenceAllocated:
					fmt.Fprintf(&buffer, "|A")
				case SequencePendingRequests:
					fmt.Fprintf(&buffer, "|F")
				case SequenceReady:
					fmt.Fprintf(&buffer, "|R")
				case SequencePreprepared:
					fmt.Fprintf(&buffer, "|Q")
				case SequencePrepared:
					fmt.Fprintf(&buffer, "|P")
				case SequenceCommitted:
					fmt.Fprintf(&buffer, "|C")
				}
			}
			if bucketBuffer.Leader {
				fmt.Fprintf(&buffer, "| Bucket=%d (LocalLeader)\n", bucketBuffer.ID)
			} else {
				fmt.Fprintf(&buffer, "| Bucket=%d\n", bucketBuffer.ID)
			}
		}

		hRule()
		fmt.Fprintf(&buffer, "- === Checkpoints ===\n")
		i := 0
		for seqNo := s.LowWatermark; seqNo <= s.HighWatermark; seqNo += uint64(len(s.Buckets)) {
			if len(s.Checkpoints) > i {
				checkpoint := s.Checkpoints[i]
				if seqNo == checkpoint.SeqNo {
					fmt.Fprintf(&buffer, "|%d", checkpoint.MaxAgreements)
					i++
					continue
				}
			}
			fmt.Fprintf(&buffer, "| ")
		}
		fmt.Fprintf(&buffer, "| Max Agreements\n")
		i = 0
		for seqNo := s.LowWatermark; seqNo <= s.HighWatermark; seqNo += uint64(len(s.Buckets)) {
			if len(s.Checkpoints) > i {
				checkpoint := s.Checkpoints[i]
				if seqNo == s.Checkpoints[i].SeqNo/uint64(len(s.Buckets)) {
					switch {
					case checkpoint.NetQuorum && !checkpoint.LocalDecision:
						fmt.Fprintf(&buffer, "|N")
					case checkpoint.NetQuorum && checkpoint.LocalDecision:
						fmt.Fprintf(&buffer, "|G")
					case !checkpoint.NetQuorum && checkpoint.LocalDecision:
						fmt.Fprintf(&buffer, "|M")
					default:
						fmt.Fprintf(&buffer, "|P")
					}
					i++
					continue
				}
			}
			fmt.Fprintf(&buffer, "| ")
		}
		fmt.Fprintf(&buffer, "| Status\n")
	}

	hRule()
	fmt.Fprintf(&buffer, "-\n")

	fmt.Fprintf(&buffer, "\n\n Request Windows\n")
	hRule()
	for _, rws := range s.ClientWindows {
		fmt.Fprintf(&buffer, "\nClient %x L/H %d/%d : %v\n", rws.ClientID, rws.LowWatermark, rws.HighWatermark, rws.Allocated)
		hRule()
	}

	fmt.Fprintf(&buffer, "\n\n Message Buffers\n")
	hRule()

	for _, nodeBuffer := range s.NodeBuffers {
		fmt.Fprintf(&buffer, "- === Node %3d buffers === \n", nodeBuffer.ID)
		fmt.Fprintf(&buffer, "  Bytes=%-8d, Messages=%-5d\n", nodeBuffer.Size, nodeBuffer.Msgs)
		for _, msgBuf := range nodeBuffer.MsgBuffers {
			fmt.Fprintf(&buffer, "  -  Bytes=%-8d Messages=%-5d Component=%s", msgBuf.Size, msgBuf.Msgs, msgBuf.Component)
		}
	}

	fmt.Fprintf(&buffer, "\n\nDone\n")

	return buffer.String()
}
