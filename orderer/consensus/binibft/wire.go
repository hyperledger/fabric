/*
Copyright BiniWorld. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package binibft

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/hyperledger/fabric/orderer/consensus/binibft/engine"
)

// Everything the plugin sends between orderers travels inside an
// ab.ConsensusRequest payload, which Fabric's cluster service authenticates and
// attributes to a consenter ID. The payload is a JSON wireEnvelope that is
// either a message of the BiniBFT engine, or a block signature produced by the
// plugin itself.
const (
	kindEngine   = "engine"
	kindBlockSig = "blocksig"
)

type wireEnvelope struct {
	Kind     string      `json:"kind"`
	Engine   *engineWire `json:"engine,omitempty"`
	BlockSig *blockSig   `json:"blocksig,omitempty"`
}

// engineWire carries an engine.Message. From/To are deliberately absent: the
// receiver derives From from the authenticated cluster sender, so a consenter
// can never impersonate another one inside the engine.
type engineWire struct {
	Type      engine.MessageType `json:"type"`
	ShardID   engine.ShardID     `json:"shard"`
	Timestamp time.Time          `json:"ts"`
	Payload   json.RawMessage    `json:"payload"`
}

// blockSig is one consenter's signature over a committed block.
type blockSig struct {
	Number               uint64 `json:"number"`
	BlockHeader          []byte `json:"block_header"`
	IdentifierHeader     []byte `json:"identifier_header"`
	OrdererBlockMetadata []byte `json:"orderer_block_metadata"`
	Signature            []byte `json:"signature"`
}

func nodeID(id uint64) engine.NodeID {
	return engine.NodeID(strconv.FormatUint(id, 10))
}

func consenterID(id engine.NodeID) (uint64, error) {
	v, err := strconv.ParseUint(string(id), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("node ID %q is not a consenter ID: %w", id, err)
	}
	return v, nil
}

func encodeEngineMessage(msg engine.Message) ([]byte, error) {
	payload, err := json.Marshal(msg.Payload)
	if err != nil {
		return nil, fmt.Errorf("marshalling payload of message type %d: %w", msg.Type, err)
	}
	return json.Marshal(&wireEnvelope{
		Kind: kindEngine,
		Engine: &engineWire{
			Type:      msg.Type,
			ShardID:   msg.ShardID,
			Timestamp: msg.Timestamp,
			Payload:   payload,
		},
	})
}

func encodeBlockSig(sig *blockSig) ([]byte, error) {
	return json.Marshal(&wireEnvelope{Kind: kindBlockSig, BlockSig: sig})
}

func decodeWire(raw []byte) (*wireEnvelope, error) {
	w := &wireEnvelope{}
	if err := json.Unmarshal(raw, w); err != nil {
		return nil, fmt.Errorf("malformed wire message: %w", err)
	}
	switch w.Kind {
	case kindEngine:
		if w.Engine == nil {
			return nil, fmt.Errorf("engine wire message without engine body")
		}
	case kindBlockSig:
		if w.BlockSig == nil {
			return nil, fmt.Errorf("block signature wire message without body")
		}
	default:
		return nil, fmt.Errorf("unknown wire message kind %q", w.Kind)
	}
	return w, nil
}

// toEngineMessage rebuilds the engine message with a concretely typed payload.
// The engine type-asserts the payload of leader-election messages, so a generic
// map[string]interface{} would be silently ignored.
func (w *engineWire) toEngineMessage(sender, self uint64) (engine.Message, error) {
	payload, err := payloadFor(w.Type)
	if err != nil {
		return engine.Message{}, err
	}
	if len(w.Payload) > 0 && string(w.Payload) != "null" {
		if err := json.Unmarshal(w.Payload, payload); err != nil {
			return engine.Message{}, fmt.Errorf("unmarshalling payload of message type %d: %w", w.Type, err)
		}
	}
	return engine.Message{
		Type:      w.Type,
		From:      nodeID(sender),
		To:        nodeID(self),
		ShardID:   w.ShardID,
		Timestamp: w.Timestamp,
		Payload:   payload,
	}, nil
}

func payloadFor(t engine.MessageType) (interface{}, error) {
	switch t {
	case engine.MsgProposal:
		return &engine.ProposalMessage{}, nil
	case engine.MsgVote:
		return &engine.VoteMessage{}, nil
	case engine.MsgShardDecision:
		return &engine.ShardDecisionMessage{}, nil
	case engine.MsgHeartbeat:
		return &engine.HeartbeatMessage{}, nil
	case engine.MsgViewChange:
		return &engine.ViewChangeRequest{}, nil
	case engine.MsgPrepare:
		return &engine.PrepareMessage{}, nil
	case engine.MsgCommitPhase:
		return &engine.CommitMessage{}, nil
	case engine.MsgRequest:
		return &engine.RequestMessage{}, nil
	case engine.MsgPrePrep:
		return &engine.PrePrepMessage{}, nil
	case engine.MsgPreparePhase:
		return &engine.PreparePhaseMessage{}, nil
	case engine.MsgCommitRequest:
		return &engine.CommitRequestMessage{}, nil
	case engine.MsgCrossShardRequest:
		return &engine.CrossShardRequestMessage{}, nil
	case engine.MsgShardAck:
		return &engine.ShardAckMessage{}, nil
	case engine.MsgFinalizedBlock:
		return &engine.FinalizedBlockMessage{}, nil
	case engine.MsgIntraShardVote:
		return &engine.IntraShardVoteMessage{}, nil
	case engine.MsgIntraShardVoteResponse:
		return &engine.IntraShardVoteResponse{}, nil
	case engine.MsgLeaderElection:
		return &engine.LeaderElectionMessage{}, nil
	case engine.MsgElectionAck:
		return &engine.ElectionAckMessage{}, nil
	case engine.MsgShardAssignment:
		return &engine.ShardAssignmentMessage{}, nil
	case engine.MsgLeaderAnnouncement:
		return &engine.LeaderAnnouncementMessage{}, nil
	case engine.MsgSyncRequest:
		return &engine.SyncRequestMessage{}, nil
	case engine.MsgSyncResponse:
		return &engine.SyncResponseMessage{}, nil
	default:
		return nil, fmt.Errorf("unsupported engine message type %d", t)
	}
}
