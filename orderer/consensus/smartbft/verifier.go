/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package smartbft

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/SmartBFT-Go/consensus/pkg/types"
	"github.com/SmartBFT-Go/consensus/smartbftprotos"
	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
)

//go:generate mockery -dir . -name Sequencer -case underscore -output mocks

// Sequencer returns sequences
type Sequencer interface {
	Sequence() uint64
}

//go:generate mockery -dir . -name ConsenterVerifier -case underscore -output mocks

// ConsenterVerifier is used to determine whether a signature from one of the consenters is valid
type ConsenterVerifier interface {
	// Evaluate takes a set of SignedData and evaluates whether this set of signatures satisfies the policy
	Evaluate(signatureSet []*protoutil.SignedData) error
}

//go:generate mockery -dir . -name AccessController -case underscore -output mocks

// AccessController is used to determine if a signature of a certain client is valid
type AccessController interface {
	// Evaluate takes a set of SignedData and evaluates whether this set of signatures satisfies the policy
	Evaluate(signatureSet []*protoutil.SignedData) error
}

type requestVerifier func(req []byte, isolated bool) (types.RequestInfo, error)

// NodeIdentitiesByID stores Identities by id
type NodeIdentitiesByID map[uint64][]byte

// IdentityToID looks up the Identity in NodeIdentitiesByID and returns id and flag true if found
func (nibd NodeIdentitiesByID) IdentityToID(identity []byte) (uint64, bool) {
	sID := &msp.SerializedIdentity{}
	if err := proto.Unmarshal(identity, sID); err != nil {
		return 0, false
	}
	for id, currIdentity := range nibd {
		currentID := &msp.SerializedIdentity{}
		if err := proto.Unmarshal(currIdentity, currentID); err != nil {
			return 0, false
		}
		if proto.Equal(currentID, sID) {
			return id, true
		}
	}
	return 0, false
}

// Verifier verifies proposals and signatures
type Verifier struct {
	RuntimeConfig         *atomic.Value
	ReqInspector          *RequestInspector
	ConsenterVerifier     ConsenterVerifier
	AccessController      AccessController
	VerificationSequencer Sequencer
	Ledger                Ledger
	Logger                *flogging.FabricLogger
	ConfigValidator       ConfigValidator
}

// AuxiliaryData unmarshals and returns auxiliary data from signature
func (v *Verifier) AuxiliaryData(msg []byte) []byte {
	sig := &Signature{}
	if err := sig.Unmarshal(msg); err != nil {
		v.Logger.Warnf("Failed unmarshalling signature message %s: %v", hex.EncodeToString(msg), err)
	}
	return nil
}

// VerifyProposal verifies proposal and returns []RequestInfo
func (v *Verifier) VerifyProposal(proposal types.Proposal) ([]types.RequestInfo, error) {
	block, err := ProposalToBlock(proposal)
	if err != nil {
		return nil, err
	}

	rtc := v.RuntimeConfig.Load().(RuntimeConfig)
	if err := verifyHashChain(block, rtc.LastCommittedBlockHash); err != nil {
		return nil, err
	}

	requests, err := v.verifyBlockDataAndMetadata(block, proposal.Metadata)
	if err != nil {
		return nil, err
	}

	verificationSeq := v.VerificationSequence()
	if verificationSeq != uint64(proposal.VerificationSequence) {
		return nil, errors.Errorf("expected verification sequence %d, but proposal has %d", verificationSeq, proposal.VerificationSequence)
	}

	return requests, nil
}

// RequestsFromProposal converts proposal to []RequestInfo
func (v *Verifier) RequestsFromProposal(proposal types.Proposal) []types.RequestInfo {
	block, err := ProposalToBlock(proposal)
	if err != nil {
		return []types.RequestInfo{}
	}

	if block.Data == nil {
		return []types.RequestInfo{}
	}

	var res []types.RequestInfo
	for _, txn := range block.Data.Data {
		req := v.ReqInspector.RequestID(txn)
		res = append(res, req)
	}

	return res
}

// VerifySignature verifies signature
func (v *Verifier) VerifySignature(signature types.Signature) error {
	id2Identity := v.RuntimeConfig.Load().(RuntimeConfig).ID2Identities
	identity, exists := id2Identity[signature.ID]
	if !exists {
		return errors.Errorf("node with id of %d doesn't exist", signature.ID)
	}

	return v.ConsenterVerifier.Evaluate([]*protoutil.SignedData{
		{Identity: identity, Data: signature.Msg, Signature: signature.Value},
	})
}

// VerifyRequest verifies raw request
func (v *Verifier) VerifyRequest(rawRequest []byte) (types.RequestInfo, error) {
	return v.verifyRequest(rawRequest, false)
}

func (v *Verifier) verifyRequest(rawRequest []byte, noConfigAllowed bool) (types.RequestInfo, error) {
	req, err := v.ReqInspector.unwrapReq(rawRequest)
	if err != nil {
		return types.RequestInfo{}, err
	}

	err = v.AccessController.Evaluate([]*protoutil.SignedData{
		{Identity: req.sigHdr.Creator, Data: req.envelope.Payload, Signature: req.envelope.Signature},
	})

	if err != nil {
		return types.RequestInfo{}, errors.Wrap(err, "access denied")
	}

	if noConfigAllowed && req.chHdr.Type != int32(cb.HeaderType_ENDORSER_TRANSACTION) {
		return types.RequestInfo{}, errors.Errorf("only endorser transactions can be sent with other transactions")
	}

	switch req.chHdr.Type {
	case int32(cb.HeaderType_CONFIG):
	case int32(cb.HeaderType_ORDERER_TRANSACTION):
		return types.RequestInfo{}, fmt.Errorf("orderer transactions are not supported in v3")
	case int32(cb.HeaderType_ENDORSER_TRANSACTION):
	default:
		return types.RequestInfo{}, errors.Errorf("transaction of type %s is not allowed to be included in blocks", cb.HeaderType_name[req.chHdr.Type])
	}

	if req.chHdr.Type == int32(cb.HeaderType_CONFIG) {
		err := v.ConfigValidator.ValidateConfig(req.envelope)
		if err != nil {
			v.Logger.Errorf("Error verifying config update: %v", err)
			return types.RequestInfo{}, err
		}
	}

	return v.ReqInspector.requestIDFromSigHeader(req.sigHdr)
}

// VerifyConsenterSig verifies consenter signature
func (v *Verifier) VerifyConsenterSig(signature types.Signature, prop types.Proposal) ([]byte, error) {
	id2Identity := v.RuntimeConfig.Load().(RuntimeConfig).ID2Identities

	identity, exists := id2Identity[signature.ID]
	if !exists {
		return nil, errors.Errorf("node with id of %d doesn't exist", signature.ID)
	}

	sig := &Signature{}
	if err := sig.Unmarshal(signature.Msg); err != nil {
		v.Logger.Errorf("Failed unmarshaling signature from %d: %v", signature.ID, err)
		v.Logger.Errorf("Offending signature Msg: %s", base64.StdEncoding.EncodeToString(signature.Msg))
		v.Logger.Errorf("Offending signature Value: %s", base64.StdEncoding.EncodeToString(signature.Value))
		return nil, errors.Wrap(err, "malformed signature format")
	}

	if err := v.verifySignatureIsBoundToProposal(sig, signature.ID, prop); err != nil {
		return nil, err
	}

	expectedMsgToBeSigned := util.ConcatenateBytes(sig.OrdererBlockMetadata, sig.IdentifierHeader, sig.BlockHeader, nil)
	signedData := &protoutil.SignedData{
		Signature: signature.Value,
		Data:      expectedMsgToBeSigned,
		Identity:  identity,
	}

	return nil, v.ConsenterVerifier.Evaluate([]*protoutil.SignedData{signedData})
}

// VerificationSequence returns verification sequence
func (v *Verifier) VerificationSequence() uint64 {
	return v.VerificationSequencer.Sequence()
}

func verifyHashChain(block *cb.Block, prevHeaderHash string) error {
	thisHdrHashOfPrevHdr := hex.EncodeToString(block.Header.PreviousHash)
	if prevHeaderHash != thisHdrHashOfPrevHdr {
		return errors.Errorf("previous header hash is %s but expected %s", thisHdrHashOfPrevHdr, prevHeaderHash)
	}

	dataHash := hex.EncodeToString(block.Header.DataHash)
	actualHashOfData := hex.EncodeToString(protoutil.BlockDataHash(block.Data))
	if dataHash != actualHashOfData {
		return errors.Errorf("data hash is %s but expected %s", dataHash, actualHashOfData)
	}
	return nil
}

func (v *Verifier) verifyBlockDataAndMetadata(block *cb.Block, metadata []byte) ([]types.RequestInfo, error) {
	if block.Data == nil || len(block.Data.Data) == 0 {
		return nil, errors.New("empty block data")
	}

	if block.Metadata == nil || len(block.Metadata.Metadata) < len(cb.BlockMetadataIndex_name) {
		return nil, errors.New("block metadata is either missing or contains too few entries")
	}

	signatureMetadata, err := protoutil.GetMetadataFromBlock(block, cb.BlockMetadataIndex_SIGNATURES)
	if err != nil {
		return nil, err
	}
	ordererMetadataFromSignature := &cb.OrdererBlockMetadata{}
	if err := proto.Unmarshal(signatureMetadata.Value, ordererMetadataFromSignature); err != nil {
		return nil, errors.Wrap(err, "failed unmarshaling OrdererBlockMetadata")
	}

	// Ensure the view metadata in the block signature and in the proposal are the same

	metadataInBlock := &smartbftprotos.ViewMetadata{}
	if err := proto.Unmarshal(ordererMetadataFromSignature.ConsenterMetadata, metadataInBlock); err != nil {
		return nil, errors.Wrap(err, "failed unmarshaling smartbft metadata from block")
	}

	metadataFromProposal := &smartbftprotos.ViewMetadata{}
	if err := proto.Unmarshal(metadata, metadataFromProposal); err != nil {
		return nil, errors.Wrap(err, "failed unmarshaling smartbft metadata from proposal")
	}

	if !proto.Equal(metadataInBlock, metadataFromProposal) {
		return nil, errors.Errorf("expected metadata in block to be [view_id:%d latest_sequence:%d] but got [view_id:%d latest_sequence:%d]",
			metadataFromProposal.GetViewId(), metadataFromProposal.GetLatestSequence(),
			metadataInBlock.GetViewId(), metadataInBlock.GetLatestSequence(),
		)
	}

	rtc := v.RuntimeConfig.Load().(RuntimeConfig)
	lastConfig := rtc.LastConfigBlock.Header.Number

	if protoutil.IsConfigBlock(block) {
		lastConfig = block.Header.Number
	}

	// Verify last config
	if ordererMetadataFromSignature.LastConfig == nil {
		return nil, errors.Errorf("last config is nil")
	}

	if ordererMetadataFromSignature.LastConfig.Index != lastConfig {
		return nil, errors.Errorf("last config in block orderer metadata points to %d but our persisted last config is %d", ordererMetadataFromSignature.LastConfig.Index, lastConfig)
	}

	rawLastConfig, err := protoutil.GetMetadataFromBlock(block, cb.BlockMetadataIndex_LAST_CONFIG)
	if err != nil {
		return nil, err
	}
	lastConf := &cb.LastConfig{}
	if err := proto.Unmarshal(rawLastConfig.Value, lastConf); err != nil {
		return nil, err
	}
	if lastConf.Index != lastConfig {
		return nil, errors.Errorf("last config in block metadata points to %d but our persisted last config is %d", ordererMetadataFromSignature.LastConfig.Index, lastConfig)
	}

	return validateTransactions(block.Data.Data, v.verifyRequest)
}

func validateTransactions(blockData [][]byte, verifyReq requestVerifier) ([]types.RequestInfo, error) {
	var validationFinished sync.WaitGroup
	validationFinished.Add(len(blockData))

	type txnValidation struct {
		indexInBlock  int
		extractedInfo types.RequestInfo
		validationErr error
	}

	noConfigAllowed := len(blockData) > 1

	validations := make(chan txnValidation, len(blockData))
	for i, payload := range blockData {
		go func(indexInBlock int, payload []byte) {
			defer validationFinished.Done()
			reqInfo, err := verifyReq(payload, noConfigAllowed)
			validations <- txnValidation{
				indexInBlock:  indexInBlock,
				extractedInfo: reqInfo,
				validationErr: err,
			}
		}(i, payload)
	}

	validationFinished.Wait()
	close(validations)

	indexToRequestInfo := make(map[int]types.RequestInfo)
	for validationResult := range validations {
		indexToRequestInfo[validationResult.indexInBlock] = validationResult.extractedInfo
		if validationResult.validationErr != nil {
			return nil, validationResult.validationErr
		}
	}

	var res []types.RequestInfo
	for indexInBlock := range blockData {
		res = append(res, indexToRequestInfo[indexInBlock])
	}

	return res, nil
}

func (v *Verifier) verifySignatureIsBoundToProposal(sig *Signature, identityID uint64, prop types.Proposal) error {
	// We verify the following fields:
	// ConsenterMetadata    []byte
	// SignatureHeader      []byte
	// BlockHeader          []byte
	// OrdererBlockMetadata []byte

	// Ensure block header is equal
	if !bytes.Equal(prop.Header, sig.BlockHeader) {
		v.Logger.Errorf("Expected block header %s but got %s", base64.StdEncoding.EncodeToString(prop.Header),
			base64.StdEncoding.EncodeToString(sig.BlockHeader))
		return errors.Errorf("mismatched block header")
	}

	// Ensure signature header matches the identity
	sigHdr := &cb.IdentifierHeader{}
	if err := proto.Unmarshal(sig.IdentifierHeader, sigHdr); err != nil {
		return errors.Wrap(err, "malformed signature header")
	}
	if identityID != uint64(sigHdr.Identifier) {
		v.Logger.Warnf("Expected identity %d but got %d", identityID,
			sigHdr.Identifier)
		return errors.Errorf("identity in signature header does not match expected identity")
	}

	// Ensure orderer block metadata's consenter MD matches the proposal
	ordererMD := &cb.OrdererBlockMetadata{}
	if err := proto.Unmarshal(sig.OrdererBlockMetadata, ordererMD); err != nil {
		return errors.Wrap(err, "malformed orderer metadata in signature")
	}

	if !bytes.Equal(ordererMD.ConsenterMetadata, prop.Metadata) {
		v.Logger.Warnf("Expected consenter metadata %s but got %s in proposal",
			base64.StdEncoding.EncodeToString(ordererMD.ConsenterMetadata), base64.StdEncoding.EncodeToString(prop.Metadata))
		return errors.Errorf("consenter metadata in OrdererBlockMetadata doesn't match proposal")
	}

	block, err := ProposalToBlock(prop)
	if err != nil {
		v.Logger.Warnf("got malformed proposal: %v", err)
		return err
	}

	// Ensure Metadata slice is of the right size
	if len(block.Metadata.Metadata) != len(cb.BlockMetadataIndex_name) {
		return errors.Errorf("block metadata is of size %d but should be of size %d",
			len(block.Metadata.Metadata), len(cb.BlockMetadataIndex_name))
	}

	signatureMetadata := &cb.Metadata{}
	if err := proto.Unmarshal(block.Metadata.Metadata[cb.BlockMetadataIndex_SIGNATURES], signatureMetadata); err != nil {
		return errors.Wrap(err, "malformed signature metadata")
	}

	ordererMDFromBlock := &cb.OrdererBlockMetadata{}
	if err := proto.Unmarshal(signatureMetadata.Value, ordererMDFromBlock); err != nil {
		return errors.Wrap(err, "malformed orderer metadata in block")
	}

	// Ensure the block's OrdererBlockMetadata matches the signature.
	if !proto.Equal(ordererMDFromBlock, ordererMD) {
		return errors.Errorf("signature's OrdererBlockMetadata and OrdererBlockMetadata extracted from block do not match")
	}

	return nil
}

type consenterVerifier struct {
	logger        *flogging.FabricLogger
	channel       string
	policyManager policies.Manager
}

// Evaluate evaluates signed data and returns no error if signature is valid and satisfies the policy
func (cv *consenterVerifier) Evaluate(signatureSet []*protoutil.SignedData) error {
	policy, ok := cv.policyManager.GetPolicy(policies.ChannelOrdererWriters)
	if !ok {
		cv.logger.Errorf("[%s] Error: could not find policy %s in policy manager %v", cv.channel, policies.ChannelOrdererWriters, cv.policyManager)
		return errors.Errorf("could not find policy %s", policies.ChannelOrdererWriters)
	}

	if cv.logger.IsEnabledFor(zapcore.DebugLevel) {
		cv.logger.Debugf("== Evaluating %T Policy %s ==", policy, policies.ChannelOrdererWriters)
		defer cv.logger.Debugf("== Done Evaluating %T Policy %s", policy, policies.ChannelOrdererWriters)
	}

	return policy.EvaluateSignedData(signatureSet)
}
