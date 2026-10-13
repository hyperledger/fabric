/*
Copyright BiniWorld. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package binibft

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric/orderer/consensus"
	"github.com/hyperledger/fabric/orderer/consensus/binibft/engine"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

// engineSigner adapts the orderer's local signing identity (MSP/BCCSP, via
// ConsenterSupport) to the engine's Signer interface.
//
// Fabric's signer hashes the message with SHA-256 before the ECDSA operation,
// so the matching verifier below always verifies over sha256(message).
type engineSigner struct {
	id      uint64
	support consensus.ConsenterSupport
}

func (s *engineSigner) Sign(msg []byte) []byte {
	sig, err := s.support.Sign(msg)
	if err != nil {
		return nil
	}
	return sig
}

func (s *engineSigner) SignDigest(digest []byte) ([]byte, error) {
	return s.support.Sign(digest)
}

func (s *engineSigner) SignProposal(p engine.Proposal, _ []byte) *engine.Signature {
	digest := []byte(p.Digest())
	sig, err := s.support.Sign(digest)
	if err != nil {
		return nil
	}
	return &engine.Signature{ID: s.id, Value: sig, Msg: digest}
}

// engineVerifier verifies signatures of consenters against the public keys in
// the channel config, and validates requests and proposals as Fabric would.
type engineVerifier struct {
	channel string
	support consensus.ConsenterSupport
	set     *consenterSet
	chain   *Chain
}

func verifyECDSA(pub *ecdsa.PublicKey, msg, sig []byte) bool {
	if len(sig) == 0 {
		return false
	}
	h := sha256.Sum256(msg)
	return ecdsa.VerifyASN1(pub, h[:], sig)
}

func (v *engineVerifier) pubKey(node engine.NodeID) (*ecdsa.PublicKey, uint64, error) {
	id, err := consenterID(node)
	if err != nil {
		return nil, 0, err
	}
	pub, ok := v.set.pubKeyOf(id)
	if !ok {
		return nil, 0, errors.Errorf("node %s is not a consenter of channel %s", node, v.channel)
	}
	return pub, id, nil
}

func (v *engineVerifier) VerifySignature(node engine.NodeID, data, sig []byte) error {
	pub, _, err := v.pubKey(node)
	if err != nil {
		return err
	}
	if !verifyECDSA(pub, data, sig) {
		return errors.Errorf("invalid signature from node %s", node)
	}
	return nil
}

func (v *engineVerifier) VerifyDigestSignature(node engine.NodeID, digest, sig []byte) error {
	return v.VerifySignature(node, digest, sig)
}

func (v *engineVerifier) VerifyProposalSignature(node engine.NodeID, p engine.Proposal, sig engine.Signature) error {
	pub, id, err := v.pubKey(node)
	if err != nil {
		return err
	}
	if sig.ID != id {
		return errors.Errorf("signature ID %d does not match node %s", sig.ID, node)
	}
	if !verifyECDSA(pub, []byte(p.Digest()), sig.Value) {
		return errors.Errorf("invalid proposal signature from node %s", node)
	}
	return nil
}

// VerifyRequest validates one client request the way every Fabric consenter
// must: the envelope has to pass the channel's message processor (signature,
// ACL, size, expiration), and config updates are re-derived and compared.
func (v *engineVerifier) VerifyRequest(raw []byte) (engine.RequestInfo, error) {
	return v.verifyRequest(raw, false)
}

func (v *engineVerifier) verifyRequest(raw []byte, noConfigAllowed bool) (engine.RequestInfo, error) {
	env, err := protoutil.UnmarshalEnvelope(raw)
	if err != nil {
		return engine.RequestInfo{}, errors.Wrap(err, "malformed envelope")
	}
	chdr, err := channelHeaderOf(env)
	if err != nil {
		return engine.RequestInfo{}, err
	}
	if chdr.ChannelId != v.channel {
		return engine.RequestInfo{}, errors.Errorf("request is for channel %s but this is %s", chdr.ChannelId, v.channel)
	}

	switch chdr.Type {
	case int32(cb.HeaderType_CONFIG):
		if noConfigAllowed {
			return engine.RequestInfo{}, errors.New("a config transaction cannot share a block with other transactions")
		}
		if err := v.validateConfig(env); err != nil {
			return engine.RequestInfo{}, errors.Wrap(err, "invalid config update")
		}
	case int32(cb.HeaderType_ENDORSER_TRANSACTION):
		if _, err := v.support.ProcessNormalMsg(env); err != nil {
			return engine.RequestInfo{}, errors.Wrap(err, "request rejected by the channel message processor")
		}
	default:
		return engine.RequestInfo{}, errors.Errorf("transaction type %s is not allowed in blocks", cb.HeaderType_name[chdr.Type])
	}

	info := requestInfo(env)
	if info.ID == "" {
		return engine.RequestInfo{}, errors.New("cannot derive a request ID")
	}
	return info, nil
}

// validateConfig re-derives the config from the embedded config update and
// requires it to equal the proposed one, so a Byzantine proposer cannot slip an
// arbitrary config into a block.
func (v *engineVerifier) validateConfig(env *cb.Envelope) error {
	payload, err := protoutil.UnmarshalPayload(env.Payload)
	if err != nil {
		return err
	}
	proposed := &cb.ConfigEnvelope{}
	if err := proto.Unmarshal(payload.Data, proposed); err != nil {
		return errors.Wrap(err, "unmarshalling config envelope")
	}
	if proposed.LastUpdate == nil {
		return errors.New("config envelope has no last update")
	}

	rebuilt, _, err := v.support.ProcessConfigMsg(env)
	if err != nil {
		return err
	}
	rebuiltPayload, err := protoutil.UnmarshalPayload(rebuilt.Payload)
	if err != nil {
		return err
	}
	rebuiltEnv := &cb.ConfigEnvelope{}
	if err := proto.Unmarshal(rebuiltPayload.Data, rebuiltEnv); err != nil {
		return err
	}
	if !proto.Equal(proposed.Config, rebuiltEnv.Config) {
		return errors.New("proposed config differs from the one derived from its config update")
	}
	return nil
}

func (v *engineVerifier) VerifyProposal(p engine.Proposal) ([]engine.RequestInfo, error) {
	block, err := blockFromProposal(p)
	if err != nil {
		return nil, err
	}

	tip := v.chain.tipSnapshot()
	if block.Header.Number != tip.number+1 {
		return nil, errors.Errorf("proposal is for block %d but the next block is %d", block.Header.Number, tip.number+1)
	}
	if !bytes.Equal(block.Header.PreviousHash, tip.hash) {
		return nil, errors.Errorf("proposal previous hash %x does not match the chain tip %x", block.Header.PreviousHash, tip.hash)
	}
	dataHash := protoutil.ComputeBlockDataHash(block.Data)
	if !bytes.Equal(block.Header.DataHash, dataHash) {
		return nil, errors.New("proposal data hash does not match its transactions")
	}
	if len(block.Data.Data) == 0 {
		return nil, errors.New("proposal has no transactions")
	}

	obm, err := ordererBlockMetadata(block)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(obm.ConsenterMetadata, p.Metadata) {
		return nil, errors.New("consenter metadata in the block differs from the proposal metadata")
	}
	wantLastConfig := tip.lastConfig
	if protoutil.IsConfigBlock(block) {
		wantLastConfig = block.Header.Number
	}
	if obm.LastConfig == nil || obm.LastConfig.Index != wantLastConfig {
		return nil, errors.Errorf("block last config is not %d", wantLastConfig)
	}

	if seq := v.support.Sequence(); uint64(p.VerificationSequence) != seq {
		return nil, errors.Errorf("proposal verification sequence is %d but the channel config sequence is %d", p.VerificationSequence, seq)
	}

	noConfig := len(block.Data.Data) > 1
	infos := make([]engine.RequestInfo, 0, len(block.Data.Data))
	seen := make(map[string]struct{}, len(block.Data.Data))
	for i, raw := range block.Data.Data {
		info, err := v.verifyRequest(raw, noConfig)
		if err != nil {
			return nil, errors.Wrapf(err, "transaction %d", i)
		}
		if _, dup := seen[info.ID]; dup {
			return nil, errors.Errorf("transaction %d is a duplicate within the block", i)
		}
		seen[info.ID] = struct{}{}
		infos = append(infos, info)
	}
	return infos, nil
}

func (v *engineVerifier) RequestsFromProposal(p engine.Proposal) []engine.RequestInfo {
	block, err := blockFromProposal(p)
	if err != nil {
		return nil
	}
	var infos []engine.RequestInfo
	for _, raw := range block.Data.Data {
		env, err := protoutil.UnmarshalEnvelope(raw)
		if err != nil {
			continue
		}
		infos = append(infos, requestInfo(env))
	}
	return infos
}

// requestInspector derives the pool identity of a request, so the engine can
// de-duplicate transactions.
type requestInspector struct{}

func (requestInspector) RequestID(raw []byte) engine.RequestInfo {
	env, err := protoutil.UnmarshalEnvelope(raw)
	if err != nil {
		return engine.RequestInfo{}
	}
	return requestInfo(env)
}

func requestInfo(env *cb.Envelope) engine.RequestInfo {
	payload, err := protoutil.UnmarshalPayload(env.Payload)
	if err != nil || payload.Header == nil {
		return engine.RequestInfo{}
	}
	sigHdr := &cb.SignatureHeader{}
	if err := proto.Unmarshal(payload.Header.SignatureHeader, sigHdr); err != nil {
		return engine.RequestInfo{}
	}
	txID := sha256.Sum256(append(append([]byte{}, sigHdr.Nonce...), sigHdr.Creator...))
	clientID := sha256.Sum256(sigHdr.Creator)
	return engine.RequestInfo{
		ID:       hex.EncodeToString(txID[:]),
		ClientID: hex.EncodeToString(clientID[:]),
	}
}

func channelHeaderOf(env *cb.Envelope) (*cb.ChannelHeader, error) {
	payload, err := protoutil.UnmarshalPayload(env.Payload)
	if err != nil {
		return nil, errors.Wrap(err, "malformed payload")
	}
	if payload.Header == nil || len(payload.Header.ChannelHeader) == 0 {
		return nil, errors.New("payload has no channel header")
	}
	chdr, err := protoutil.UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		return nil, errors.Wrap(err, "malformed channel header")
	}
	return chdr, nil
}

func blockFromProposal(p engine.Proposal) (*cb.Block, error) {
	if len(p.Header) == 0 || len(p.Payload) == 0 {
		return nil, errors.New("proposal has an empty header or payload")
	}
	block := &cb.Block{}
	if err := proto.Unmarshal(p.Payload, block); err != nil {
		return nil, errors.Wrap(err, "proposal payload is not a block")
	}
	if block.Header == nil || block.Data == nil || block.Metadata == nil {
		return nil, errors.New("proposal block is missing header, data or metadata")
	}
	if len(block.Metadata.Metadata) != len(cb.BlockMetadataIndex_name) {
		return nil, fmt.Errorf("proposal block has %d metadata entries, expected %d",
			len(block.Metadata.Metadata), len(cb.BlockMetadataIndex_name))
	}
	if !bytes.Equal(protoutil.BlockHeaderBytes(block.Header), p.Header) {
		return nil, errors.New("proposal header does not match the block header")
	}
	return block, nil
}

func ordererBlockMetadata(block *cb.Block) (*cb.OrdererBlockMetadata, error) {
	md, err := protoutil.GetMetadataFromBlock(block, cb.BlockMetadataIndex_SIGNATURES)
	if err != nil {
		return nil, err
	}
	obm := &cb.OrdererBlockMetadata{}
	if err := proto.Unmarshal(md.Value, obm); err != nil {
		return nil, errors.Wrap(err, "unmarshalling orderer block metadata")
	}
	return obm, nil
}
