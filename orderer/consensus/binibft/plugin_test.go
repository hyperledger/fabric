/*
Copyright BiniWorld. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package binibft

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"testing"

	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric/orderer/consensus/binibft/engine"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func envelopeOfType(t *testing.T, typ cb.HeaderType, tag string) []byte {
	t.Helper()
	chdr := protoutil.MarshalOrPanic(&cb.ChannelHeader{Type: int32(typ), ChannelId: "ch", TxId: tag})
	payload := protoutil.MarshalOrPanic(&cb.Payload{Header: &cb.Header{ChannelHeader: chdr}})
	return protoutil.MarshalOrPanic(&cb.Envelope{Payload: payload})
}

// The quorum must match the block-validation policy peers evaluate.
func TestBftQuorum(t *testing.T) {
	for n, want := range map[int]int{1: 1, 3: 2, 4: 3, 5: 4, 7: 5, 10: 7} {
		require.Equal(t, want, bftQuorum(n), "n=%d", n)
	}
}

func TestSplitConfigTx(t *testing.T) {
	normal1 := envelopeOfType(t, cb.HeaderType_ENDORSER_TRANSACTION, "n1")
	normal2 := envelopeOfType(t, cb.HeaderType_ENDORSER_TRANSACTION, "n2")
	config := envelopeOfType(t, cb.HeaderType_CONFIG, "c")

	t.Run("only normal transactions share a block", func(t *testing.T) {
		batch, rest := splitConfigTx([][]byte{normal1, normal2})
		require.Len(t, batch, 2)
		require.Empty(t, rest)
	})
	t.Run("a leading config transaction gets its own block", func(t *testing.T) {
		batch, rest := splitConfigTx([][]byte{config, normal1, normal2})
		require.Equal(t, [][]byte{config}, batch)
		require.Equal(t, [][]byte{normal1, normal2}, rest)
	})
	t.Run("transactions before a config transaction go first and it waits", func(t *testing.T) {
		batch, rest := splitConfigTx([][]byte{normal1, config, normal2})
		require.Equal(t, [][]byte{normal1}, batch)
		require.Equal(t, [][]byte{config, normal2}, rest)
	})
	t.Run("nothing is lost across repeated splits", func(t *testing.T) {
		pending := [][]byte{normal1, config, normal2, config, normal1}
		var total int
		for len(pending) > 0 {
			batch, rest := splitConfigTx(pending)
			require.NotEmpty(t, batch, "must always make progress")
			total += len(batch)
			pending = rest
		}
		require.Equal(t, 5, total)
	})
}

// The engine type-asserts the payload of some messages, so the wire codec must
// restore the concrete payload type, and must never trust a claimed sender.
func TestWireRoundTrip(t *testing.T) {
	sent := engine.Message{
		Type:      engine.MsgLeaderElection,
		From:      "9", // a spoofed sender inside the message must be ignored
		ShardID:   3,
		Timestamp: engine.Message{}.Timestamp,
		Payload:   &engine.LeaderElectionMessage{ElectionID: "e1", CandidateID: "2", Term: 7},
	}
	raw, err := encodeEngineMessage(sent)
	require.NoError(t, err)

	w, err := decodeWire(raw)
	require.NoError(t, err)
	require.Equal(t, kindEngine, w.Kind)

	got, err := w.Engine.toEngineMessage(2, 1)
	require.NoError(t, err)
	require.Equal(t, engine.NodeID("2"), got.From, "From comes from the authenticated sender")
	require.Equal(t, engine.NodeID("1"), got.To)

	payload, ok := got.Payload.(*engine.LeaderElectionMessage)
	require.True(t, ok, "payload must be the concrete pointer type, got %T", got.Payload)
	require.Equal(t, "e1", payload.ElectionID)
	require.Equal(t, uint64(7), payload.Term)
}

func TestDecodeWireRejectsGarbage(t *testing.T) {
	for _, raw := range [][]byte{nil, []byte("not json"), []byte(`{"kind":"engine"}`), []byte(`{"kind":"nope"}`), []byte(`{"kind":"blocksig"}`)} {
		_, err := decodeWire(raw)
		require.Error(t, err, "input %q", raw)
	}
}

// Fabric's signer hashes with SHA-256 before ECDSA; the plugin's verifier must
// accept exactly that and reject tampering.
func TestVerifyECDSA(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	msg := []byte("OrdererBlockMetadata||IdentifierHeader||BlockHeader")
	digest := sha256.Sum256(msg)
	sig, err := ecdsa.SignASN1(rand.Reader, key, digest[:])
	require.NoError(t, err)

	require.True(t, verifyECDSA(&key.PublicKey, msg, sig))
	require.False(t, verifyECDSA(&key.PublicKey, append(msg, 'x'), sig))
	require.False(t, verifyECDSA(&key.PublicKey, msg, nil))
}

func TestProposalToBlockRejectsInconsistentHeader(t *testing.T) {
	block := protoutil.NewBlock(1, []byte("prev"))
	block.Data = &cb.BlockData{Data: [][]byte{[]byte("tx")}}
	block.Header.DataHash = protoutil.ComputeBlockDataHash(block.Data)
	payload, err := proto.Marshal(block)
	require.NoError(t, err)

	good := engine.Proposal{Header: protoutil.BlockHeaderBytes(block.Header), Payload: payload}
	_, err = blockFromProposal(good)
	require.NoError(t, err)

	bad := engine.Proposal{Header: []byte("some other header"), Payload: payload}
	_, err = blockFromProposal(bad)
	require.Error(t, err, "a proposal whose header does not match its block must be rejected")

	_, err = blockFromProposal(engine.Proposal{})
	require.Error(t, err)
}
