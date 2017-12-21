/*
Copyright IBM Corp. 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package utils

import (
	"errors"
	"fmt"

	"encoding/binary"

	"encoding/hex"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/chaincode/platforms"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
)

// GetChaincodeInvocationSpec get the ChaincodeInvocationSpec from the proposal
func GetChaincodeInvocationSpec(prop *peer.Proposal) (*peer.ChaincodeInvocationSpec, error) {
	if prop == nil {
		return nil, fmt.Errorf("Proposal is nil")
	}
	_, err := GetHeader(prop.Header)
	if err != nil {
		return nil, err
	}
	ccPropPayload := &peer.ChaincodeProposalPayload{}
	err = proto.Unmarshal(prop.Payload, ccPropPayload)
	if err != nil {
		return nil, err
	}
	cis := &peer.ChaincodeInvocationSpec{}
	err = proto.Unmarshal(ccPropPayload.Input, cis)
	return cis, err
}

// GetChaincodeProposalContext returns creator and transient
func GetChaincodeProposalContext(prop *peer.Proposal) ([]byte, map[string][]byte, error) {
	if prop == nil {
		return nil, nil, fmt.Errorf("Proposal is nil")
	}
	if len(prop.Header) == 0 {
		return nil, nil, fmt.Errorf("Proposal's header is nil")
	}
	if len(prop.Payload) == 0 {
		return nil, nil, fmt.Errorf("Proposal's payload is nil")
	}

	//// get back the header
	hdr, err := GetHeader(prop.Header)
	if err != nil {
		return nil, nil, fmt.Errorf("Could not extract the header from the proposal: %s", err)
	}
	if hdr == nil {
		return nil, nil, fmt.Errorf("Unmarshalled header is nil")
	}

	chdr, err := UnmarshalChannelHeader(hdr.ChannelHeader)
	if err != nil {
		return nil, nil, fmt.Errorf("Could not extract the channel header from the proposal: %s", err)
	}

	if common.HeaderType(chdr.Type) != common.HeaderType_ENDORSER_TRANSACTION &&
		common.HeaderType(chdr.Type) != common.HeaderType_CONFIG {
		return nil, nil, fmt.Errorf("Invalid proposal type expected ENDORSER_TRANSACTION or CONFIG. Was: %d", chdr.Type)
	}

	shdr, err := GetSignatureHeader(hdr.SignatureHeader)
	if err != nil {
		return nil, nil, fmt.Errorf("Could not extract the signature header from the proposal: %s", err)
	}

	ccPropPayload := &peer.ChaincodeProposalPayload{}
	err = proto.Unmarshal(prop.Payload, ccPropPayload)
	if err != nil {
		return nil, nil, err
	}

	return shdr.Creator, ccPropPayload.TransientMap, nil
}

// GetHeader Get Header from bytes
func GetHeader(bytes []byte) (*common.Header, error) {
	hdr := &common.Header{}
	err := proto.Unmarshal(bytes, hdr)
	return hdr, err
}

// GetNonce returns the nonce used in Proposal
func GetNonce(prop *peer.Proposal) ([]byte, error) {
	if prop == nil {
		return nil, fmt.Errorf("Proposal is nil")
	}
	// get back the header
	hdr, err := GetHeader(prop.Header)
	if err != nil {
		return nil, fmt.Errorf("Could not extract the header from the proposal: %s", err)
	}

	chdr, err := UnmarshalChannelHeader(hdr.ChannelHeader)
	if err != nil {
		return nil, fmt.Errorf("Could not extract the channel header from the proposal: %s", err)
	}

	if common.HeaderType(chdr.Type) != common.HeaderType_ENDORSER_TRANSACTION &&
		common.HeaderType(chdr.Type) != common.HeaderType_CONFIG {
		return nil, fmt.Errorf("Invalid proposal type expected ENDORSER_TRANSACTION or CONFIG. Was: %d", chdr.Type)
	}

	shdr, err := GetSignatureHeader(hdr.SignatureHeader)
	if err != nil {
		return nil, fmt.Errorf("Could not extract the signature header from the proposal: %s", err)
	}

	if hdr.SignatureHeader == nil {
		return nil, errors.New("Invalid signature header. It must be different from nil.")
	}

	return shdr.Nonce, nil
}

// GetChaincodeHeaderExtension get chaincode header extension given header
func GetChaincodeHeaderExtension(hdr *common.Header) (*peer.ChaincodeHeaderExtension, error) {
	chdr, err := UnmarshalChannelHeader(hdr.ChannelHeader)
	if err != nil {
		return nil, err
	}

	chaincodeHdrExt := &peer.ChaincodeHeaderExtension{}
	err = proto.Unmarshal(chdr.Extension, chaincodeHdrExt)
	return chaincodeHdrExt, err
}

// GetProposalResponse given proposal in bytes
func GetProposalResponse(prBytes []byte) (*peer.ProposalResponse, error) {
	proposalResponse := &peer.ProposalResponse{}
	err := proto.Unmarshal(prBytes, proposalResponse)
	return proposalResponse, err
}

// GetChaincodeDeploymentSpec returns a ChaincodeDeploymentSpec given args
func GetChaincodeDeploymentSpec(code []byte) (*peer.ChaincodeDeploymentSpec, error) {
	cds := &peer.ChaincodeDeploymentSpec{}
	err := proto.Unmarshal(code, cds)
	if err != nil {
		return nil, err
	}

	// FAB-2122: Validate the CDS according to platform specific requirements
	platform, err := platforms.Find(cds.ChaincodeSpec.Type)
	if err != nil {
		return nil, err
	}

	err = platform.ValidateDeploymentSpec(cds)
	return cds, err
}

// GetChaincodeAction gets the ChaincodeAction given chaicnode action bytes
func GetChaincodeAction(caBytes []byte) (*peer.ChaincodeAction, error) {
	chaincodeAction := &peer.ChaincodeAction{}
	err := proto.Unmarshal(caBytes, chaincodeAction)
	return chaincodeAction, err
}

// GetResponse gets the Response given response bytes
func GetResponse(resBytes []byte) (*peer.Response, error) {
	response := &peer.Response{}
	err := proto.Unmarshal(resBytes, response)
	return response, err
}

// GetChaincodeEvents gets the ChaincodeEvents given chaincode event bytes
func GetChaincodeEvents(eBytes []byte) (*peer.ChaincodeEvent, error) {
	chaincodeEvent := &peer.ChaincodeEvent{}
	err := proto.Unmarshal(eBytes, chaincodeEvent)
	return chaincodeEvent, err
}

// GetProposalResponsePayload gets the proposal response payload
func GetProposalResponsePayload(prpBytes []byte) (*peer.ProposalResponsePayload, error) {
	prp := &peer.ProposalResponsePayload{}
	err := proto.Unmarshal(prpBytes, prp)
	return prp, err
}

// GetProposal returns a Proposal message from its bytes
func GetProposal(propBytes []byte) (*peer.Proposal, error) {
	prop := &peer.Proposal{}
	err := proto.Unmarshal(propBytes, prop)
	return prop, err
}

// GetPayload Get Payload from Envelope message
func GetPayload(e *common.Envelope) (*common.Payload, error) {
	payload := &common.Payload{}
	err := proto.Unmarshal(e.Payload, payload)
	return payload, err
}

// GetTransaction Get Transaction from bytes
func GetTransaction(txBytes []byte) (*peer.Transaction, error) {
	tx := &peer.Transaction{}
	err := proto.Unmarshal(txBytes, tx)
	return tx, err
}

// GetChaincodeActionPayload Get ChaincodeActionPayload from bytes
func GetChaincodeActionPayload(capBytes []byte) (*peer.ChaincodeActionPayload, error) {
	cap := &peer.ChaincodeActionPayload{}
	err := proto.Unmarshal(capBytes, cap)
	return cap, err
}

// GetChaincodeProposalPayload Get ChaincodeProposalPayload from bytes
func GetChaincodeProposalPayload(bytes []byte) (*peer.ChaincodeProposalPayload, error) {
	cpp := &peer.ChaincodeProposalPayload{}
	err := proto.Unmarshal(bytes, cpp)
	return cpp, err
}

// GetSignatureHeader Get SignatureHeader from bytes
func GetSignatureHeader(bytes []byte) (*common.SignatureHeader, error) {
	sh := &common.SignatureHeader{}
	err := proto.Unmarshal(bytes, sh)
	return sh, err
}

// CreateChaincodeProposal creates a proposal from given input.
// It returns the proposal and the transaction id associated to the proposal
func CreateChaincodeProposal(typ common.HeaderType, chainID string, cis *peer.ChaincodeInvocationSpec, creator []byte) (*peer.Proposal, string, error) {
	return CreateChaincodeProposalWithTransient(typ, chainID, cis, creator, nil)
}

// CreateChaincodeProposalWithTransient creates a proposal from given input
// It returns the proposal and the transaction id associated to the proposal
func CreateChaincodeProposalWithTransient(typ common.HeaderType, chainID string, cis *peer.ChaincodeInvocationSpec, creator []byte, transientMap map[string][]byte) (*peer.Proposal, string, error) {
	// generate a random nonce
	nonce, err := crypto.GetRandomNonce()
	if err != nil {
		return nil, "", err
	}

	// compute txid
	txid, err := ComputeProposalTxID(nonce, creator)
	if err != nil {
		return nil, "", err
	}

	return CreateChaincodeProposalWithTxIDNonceAndTransient(txid, typ, chainID, cis, nonce, creator, transientMap)
}

// CreateChaincodeProposalWithTxIDNonceAndTransient creates a proposal from given input
func CreateChaincodeProposalWithTxIDNonceAndTransient(txid string, typ common.HeaderType, chainID string, cis *peer.ChaincodeInvocationSpec, nonce, creator []byte, transientMap map[string][]byte) (*peer.Proposal, string, error) {
	ccHdrExt := &peer.ChaincodeHeaderExtension{ChaincodeId: cis.ChaincodeSpec.ChaincodeId}
	ccHdrExtBytes, err := proto.Marshal(ccHdrExt)
	if err != nil {
		return nil, "", err
	}

	cisBytes, err := proto.Marshal(cis)
	if err != nil {
		return nil, "", err
	}

	ccPropPayload := &peer.ChaincodeProposalPayload{Input: cisBytes, TransientMap: transientMap}
	ccPropPayloadBytes, err := proto.Marshal(ccPropPayload)
	if err != nil {
		return nil, "", err
	}

	// TODO: epoch is now set to zero. This must be changed once we
	// get a more appropriate mechanism to handle it in.
	var epoch uint64 = 0

	timestamp := util.CreateUtcTimestamp()

	hdr := &common.Header{ChannelHeader: MarshalOrPanic(&common.ChannelHeader{
		Type:      int32(typ),
		TxId:      txid,
		Timestamp: timestamp,
		ChannelId: chainID,
		Extension: ccHdrExtBytes,
		Epoch:     epoch}),
		SignatureHeader: MarshalOrPanic(&common.SignatureHeader{Nonce: nonce, Creator: creator})}

	hdrBytes, err := proto.Marshal(hdr)
	if err != nil {
		return nil, "", err
	}

	return &peer.Proposal{Header: hdrBytes, Payload: ccPropPayloadBytes}, txid, nil
}

// GetBytesProposalResponsePayload gets proposal response payload
func GetBytesProposalResponsePayload(hash []byte, response *peer.Response, result []byte, event []byte, ccid *peer.ChaincodeID) ([]byte, error) {
	cAct := &peer.ChaincodeAction{Events: event, Results: result, Response: response, ChaincodeId: ccid}
	cActBytes, err := proto.Marshal(cAct)
	if err != nil {
		return nil, err
	}

	prp := &peer.ProposalResponsePayload{Extension: cActBytes, ProposalHash: hash}
	prpBytes, err := proto.Marshal(prp)
	return prpBytes, err
}

// GetBytesChaincodeProposalPayload gets the chaincode proposal payload
func GetBytesChaincodeProposalPayload(cpp *peer.ChaincodeProposalPayload) ([]byte, error) {
	cppBytes, err := proto.Marshal(cpp)
	return cppBytes, err
}

// GetBytesResponse gets the bytes of Response
func GetBytesResponse(res *peer.Response) ([]byte, error) {
	resBytes, err := proto.Marshal(res)
	return resBytes, err
}

// GetBytesChaincodeEvent gets the bytes of ChaincodeEvent
func GetBytesChaincodeEvent(event *peer.ChaincodeEvent) ([]byte, error) {
	eventBytes, err := proto.Marshal(event)
	return eventBytes, err
}

// GetBytesChaincodeActionPayload get the bytes of ChaincodeActionPayload from the message
func GetBytesChaincodeActionPayload(cap *peer.ChaincodeActionPayload) ([]byte, error) {
	capBytes, err := proto.Marshal(cap)
	return capBytes, err
}

// GetBytesProposalResponse gets proposal bytes response
func GetBytesProposalResponse(pr *peer.ProposalResponse) ([]byte, error) {
	respBytes, err := proto.Marshal(pr)
	return respBytes, err
}

// GetBytesProposal returns the bytes of a proposal message
func GetBytesProposal(prop *peer.Proposal) ([]byte, error) {
	propBytes, err := proto.Marshal(prop)
	return propBytes, err
}

// GetBytesHeader get the bytes of Header from the message
func GetBytesHeader(hdr *common.Header) ([]byte, error) {
	bytes, err := proto.Marshal(hdr)
	return bytes, err
}

// GetBytesSignatureHeader get the bytes of SignatureHeader from the message
func GetBytesSignatureHeader(hdr *common.SignatureHeader) ([]byte, error) {
	bytes, err := proto.Marshal(hdr)
	return bytes, err
}

// GetBytesTransaction get the bytes of Transaction from the message
func GetBytesTransaction(tx *peer.Transaction) ([]byte, error) {
	bytes, err := proto.Marshal(tx)
	return bytes, err
}

// GetBytesPayload get the bytes of Payload from the message
func GetBytesPayload(payl *common.Payload) ([]byte, error) {
	bytes, err := proto.Marshal(payl)
	return bytes, err
}

// GetBytesEnvelope get the bytes of Envelope from the message
func GetBytesEnvelope(env *common.Envelope) ([]byte, error) {
	bytes, err := proto.Marshal(env)
	return bytes, err
}

// GetActionFromEnvelope extracts a ChaincodeAction message from a serialized Envelope
func GetActionFromEnvelope(envBytes []byte) (*peer.ChaincodeAction, error) {
	env, err := GetEnvelopeFromBlock(envBytes)
	if err != nil {
		return nil, err
	}

	payl, err := GetPayload(env)
	if err != nil {
		return nil, err
	}

	tx, err := GetTransaction(payl.Data)
	if err != nil {
		return nil, err
	}

	if len(tx.Actions) == 0 {
		return nil, fmt.Errorf("At least one TransactionAction is required")
	}

	_, respPayload, err := GetPayloads(tx.Actions[0])
	return respPayload, err
}

// CreateProposalFromCIS returns a proposal given a serialized identity and a ChaincodeInvocationSpec
func CreateProposalFromCISAndTxid(txid string, typ common.HeaderType, chainID string, cis *peer.ChaincodeInvocationSpec, creator []byte) (*peer.Proposal, string, error) {
	nonce, err := crypto.GetRandomNonce()
	if err != nil {
		return nil, "", err
	}
	return CreateChaincodeProposalWithTxIDNonceAndTransient(txid, typ, chainID, cis, nonce, creator, nil)
}

// CreateProposalFromCIS returns a proposal given a serialized identity and a ChaincodeInvocationSpec
func CreateProposalFromCIS(typ common.HeaderType, chainID string, cis *peer.ChaincodeInvocationSpec, creator []byte) (*peer.Proposal, string, error) {
	return CreateChaincodeProposal(typ, chainID, cis, creator)
}

// CreateGetChaincodesProposal returns a GETCHAINCODES proposal given a serialized identity
func CreateGetChaincodesProposal(chainID string, creator []byte) (*peer.Proposal, string, error) {
	ccinp := &peer.ChaincodeInput{Args: [][]byte{[]byte("getchaincodes")}}
	lsccSpec := &peer.ChaincodeInvocationSpec{
		ChaincodeSpec: &peer.ChaincodeSpec{
			Type:        peer.ChaincodeSpec_GOLANG,
			ChaincodeId: &peer.ChaincodeID{Name: "lscc"},
			Input:       ccinp},
	}
	return CreateProposalFromCIS(common.HeaderType_ENDORSER_TRANSACTION, chainID, lsccSpec, creator)
}

// CreateGetChaincodesProposal returns a GETINSTALLEDCHAINCODES proposal given a serialized identity
func CreateGetInstalledChaincodesProposal(creator []byte) (*peer.Proposal, string, error) {
	ccinp := &peer.ChaincodeInput{Args: [][]byte{[]byte("getinstalledchaincodes")}}
	lsccSpec := &peer.ChaincodeInvocationSpec{
		ChaincodeSpec: &peer.ChaincodeSpec{
			Type:        peer.ChaincodeSpec_GOLANG,
			ChaincodeId: &peer.ChaincodeID{Name: "lscc"},
			Input:       ccinp},
	}
	return CreateProposalFromCIS(common.HeaderType_ENDORSER_TRANSACTION, "", lsccSpec, creator)
}

// CreateInstallProposalFromCDS returns a install proposal given a serialized identity and a ChaincodeDeploymentSpec
func CreateInstallProposalFromCDS(ccpack proto.Message, creator []byte) (*peer.Proposal, string, error) {
	return createProposalFromCDS("", ccpack, creator, "install")
}

// CreateDeployProposalFromCDS returns a deploy proposal given a serialized identity and a ChaincodeDeploymentSpec
func CreateDeployProposalFromCDS(
	chainID string,
	cds *peer.ChaincodeDeploymentSpec,
	creator []byte,
	policy []byte,
	escc []byte,
	vscc []byte,
	collectionConfig []byte) (*peer.Proposal, string, error) {
	if collectionConfig == nil {
		return createProposalFromCDS(chainID, cds, creator, "deploy", policy, escc, vscc)
	} else {
		return createProposalFromCDS(chainID, cds, creator, "deploy", policy, escc, vscc, collectionConfig)
	}
}

// CreateUpgradeProposalFromCDS returns a upgrade proposal given a serialized identity and a ChaincodeDeploymentSpec
func CreateUpgradeProposalFromCDS(chainID string, cds *peer.ChaincodeDeploymentSpec, creator []byte, policy []byte, escc []byte, vscc []byte) (*peer.Proposal, string, error) {
	return createProposalFromCDS(chainID, cds, creator, "upgrade", policy, escc, vscc)
}

// createProposalFromCDS returns a deploy or upgrade proposal given a serialized identity and a ChaincodeDeploymentSpec
func createProposalFromCDS(chainID string, msg proto.Message, creator []byte, propType string, args ...[]byte) (*peer.Proposal, string, error) {
	//in the new mode, cds will be nil, "deploy" and "upgrade" are instantiates.
	var ccinp *peer.ChaincodeInput
	var b []byte
	var err error
	if msg != nil {
		b, err = proto.Marshal(msg)
		if err != nil {
			return nil, "", err
		}
	}
	switch propType {
	case "deploy":
		fallthrough
	case "upgrade":
		cds, ok := msg.(*peer.ChaincodeDeploymentSpec)
		if !ok || cds == nil {
			return nil, "", fmt.Errorf("invalid message for creating lifecycle chaincode proposal from")
		}
		Args := [][]byte{[]byte(propType), []byte(chainID), b}
		Args = append(Args, args...)

		ccinp = &peer.ChaincodeInput{Args: Args}
	case "install":
		ccinp = &peer.ChaincodeInput{Args: [][]byte{[]byte(propType), b}}
	}

	//wrap the deployment in an invocation spec to lscc...
	lsccSpec := &peer.ChaincodeInvocationSpec{
		ChaincodeSpec: &peer.ChaincodeSpec{
			Type:        peer.ChaincodeSpec_GOLANG,
			ChaincodeId: &peer.ChaincodeID{Name: "lscc"},
			Input:       ccinp}}

	//...and get the proposal for it
	return CreateProposalFromCIS(common.HeaderType_ENDORSER_TRANSACTION, chainID, lsccSpec, creator)
}

// ComputeProposalTxID computes TxID as the Hash computed
// over the concatenation of nonce and creator.
func ComputeProposalTxID(nonce, creator []byte) (string, error) {
	// TODO: Get the Hash function to be used from
	// channel configuration
	digest, err := factory.GetDefault().Hash(
		append(nonce, creator...),
		&bccsp.SHA256Opts{})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(digest), nil
}

// CheckProposalTxID checks that txid is equal to the Hash computed
// over the concatenation of nonce and creator.
func CheckProposalTxID(txid string, nonce, creator []byte) error {
	computedTxID, err := ComputeProposalTxID(nonce, creator)
	if err != nil {
		return fmt.Errorf("Failed computing target TXID for comparison [%s]", err)
	}

	if txid != computedTxID {
		return fmt.Errorf("Transaction is not valid. Got [%s], expected [%s]", txid, computedTxID)
	}

	return nil
}

// ComputeProposalBinding computes the binding of a proposal
func ComputeProposalBinding(proposal *peer.Proposal) ([]byte, error) {
	if proposal == nil {
		return nil, fmt.Errorf("Porposal is nil")
	}
	if len(proposal.Header) == 0 {
		return nil, fmt.Errorf("Proposal's Header is nil")
	}

	h, err := GetHeader(proposal.Header)
	if err != nil {
		return nil, err
	}

	chdr, err := UnmarshalChannelHeader(h.ChannelHeader)
	if err != nil {
		return nil, err
	}
	shdr, err := GetSignatureHeader(h.SignatureHeader)
	if err != nil {
		return nil, err
	}

	return computeProposalBindingInternal(shdr.Nonce, shdr.Creator, chdr.Epoch)
}

func computeProposalBindingInternal(nonce, creator []byte, epoch uint64) ([]byte, error) {
	epochBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(epochBytes, epoch)

	// TODO: add to genesis block the hash function used for the binding computation.
	return factory.GetDefault().Hash(
		append(append(nonce, creator...), epochBytes...),
		&bccsp.SHA256Opts{})
}
