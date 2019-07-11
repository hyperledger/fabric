/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package utils

import (
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
	"github.com/pkg/errors"
)

// GetChaincodeInvocationSpec get the ChaincodeInvocationSpec from the proposal
func GetChaincodeInvocationSpec(prop *peer.Proposal) (*peer.ChaincodeInvocationSpec, error) {
	if prop == nil {
		return nil, errors.New("proposal is nil")
	}
	_, err := GetHeader(prop.Header)
	if err != nil {
		return nil, err
	}
	ccPropPayload, err := GetChaincodeProposalPayload(prop.Payload)
	if err != nil {
		return nil, err
	}
	cis := &peer.ChaincodeInvocationSpec{}
	err = proto.Unmarshal(ccPropPayload.Input, cis)
	return cis, errors.Wrap(err, "error unmarshaling ChaincodeInvocationSpec")
}

// GetChaincodeProposalContext returns creator and transient
func GetChaincodeProposalContext(prop *peer.Proposal) ([]byte, map[string][]byte, error) {
	if prop == nil {
		return nil, nil, errors.New("proposal is nil")
	}
	if len(prop.Header) == 0 {
		return nil, nil, errors.New("proposal's header is nil")
	}
	if len(prop.Payload) == 0 {
		return nil, nil, errors.New("proposal's payload is nil")
	}
	// get back the header
	hdr, err := GetHeader(prop.Header)
	if err != nil {
		return nil, nil, errors.WithMessage(err, "error extracting header from proposal")
	}
	if hdr == nil {
		return nil, nil, errors.New("unmarshaled header is nil")
	}

	chdr, err := UnmarshalChannelHeader(hdr.ChannelHeader)
	if err != nil {
		return nil, nil, errors.WithMessage(err, "error extracting channel header from proposal")
	}

	if err = validateChannelHeaderType(chdr, []common.HeaderType{common.HeaderType_ENDORSER_TRANSACTION, common.HeaderType_CONFIG}); err != nil {
		return nil, nil, errors.WithMessage(err, "invalid proposal")
	}

	shdr, err := GetSignatureHeader(hdr.SignatureHeader)
	if err != nil {
		return nil, nil, errors.WithMessage(err, "error extracting signature header from proposal")
	}

	ccPropPayload, err := GetChaincodeProposalPayload(prop.Payload)
	if err != nil {
		return nil, nil, err
	}

	return shdr.Creator, ccPropPayload.TransientMap, nil
}

func validateChannelHeaderType(chdr *common.ChannelHeader, expectedTypes []common.HeaderType) error {
	for _, t := range expectedTypes {
		if common.HeaderType(chdr.Type) == t {
			return nil
		}
	}
	return errors.Errorf("invalid channel header type. expected one of %s, received %s", expectedTypes, common.HeaderType(chdr.Type))
}

// GetHeader Get Header from bytes
func GetHeader(bytes []byte) (*common.Header, error) {
	hdr := &common.Header{}
	err := proto.Unmarshal(bytes, hdr)
	return hdr, errors.Wrap(err, "error unmarshaling Header")
}

// GetNonce returns the nonce used in Proposal
func GetNonce(prop *peer.Proposal) ([]byte, error) {
	if prop == nil {
		return nil, errors.New("proposal is nil")
	}

	// get back the header
	hdr, err := GetHeader(prop.Header)
	if err != nil {
		return nil, err
	}

	chdr, err := UnmarshalChannelHeader(hdr.ChannelHeader)
	if err != nil {
		return nil, err
	}

	if err = validateChannelHeaderType(chdr, []common.HeaderType{common.HeaderType_ENDORSER_TRANSACTION, common.HeaderType_CONFIG}); err != nil {
		return nil, errors.WithMessage(err, "invalid proposal")
	}

	shdr, err := GetSignatureHeader(hdr.SignatureHeader)
	if err != nil {
		return nil, err
	}

	if hdr.SignatureHeader == nil {
		return nil, errors.New("invalid signature header. cannot be nil")
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
	return chaincodeHdrExt, errors.Wrap(err, "error unmarshaling ChaincodeHeaderExtension")
}

// GetProposalResponse given proposal in bytes
func GetProposalResponse(prBytes []byte) (*peer.ProposalResponse, error) {
	proposalResponse := &peer.ProposalResponse{}
	err := proto.Unmarshal(prBytes, proposalResponse)
	return proposalResponse, errors.Wrap(err, "error unmarshaling ProposalResponse")
}

// GetChaincodeDeploymentSpec returns a ChaincodeDeploymentSpec given args
func GetChaincodeDeploymentSpec(code []byte, pr *platforms.Registry) (*peer.ChaincodeDeploymentSpec, error) {
	cds := &peer.ChaincodeDeploymentSpec{}
	err := proto.Unmarshal(code, cds)
	if err != nil {
		return nil, errors.Wrap(err, "error unmarshaling ChaincodeDeploymentSpec")
	}

	// FAB-2122: Validate the CDS according to platform specific requirements
	return cds, pr.ValidateDeploymentSpec(cds.CCType(), cds.Bytes())
}

// GetChaincodeAction gets the ChaincodeAction given chaicnode action bytes
func GetChaincodeAction(caBytes []byte) (*peer.ChaincodeAction, error) {
	chaincodeAction := &peer.ChaincodeAction{}
	err := proto.Unmarshal(caBytes, chaincodeAction)
	return chaincodeAction, errors.Wrap(err, "error unmarshaling ChaincodeAction")
}

// GetResponse gets the Response given response bytes
func GetResponse(resBytes []byte) (*peer.Response, error) {
	response := &peer.Response{}
	err := proto.Unmarshal(resBytes, response)
	return response, errors.Wrap(err, "error unmarshaling Response")
}

// GetChaincodeEvents gets the ChaincodeEvents given chaincode event bytes
func GetChaincodeEvents(eBytes []byte) (*peer.ChaincodeEvent, error) {
	chaincodeEvent := &peer.ChaincodeEvent{}
	err := proto.Unmarshal(eBytes, chaincodeEvent)
	return chaincodeEvent, errors.Wrap(err, "error unmarshaling ChaicnodeEvent")
}

// GetProposalResponsePayload gets the proposal response payload
func GetProposalResponsePayload(prpBytes []byte) (*peer.ProposalResponsePayload, error) {
	prp := &peer.ProposalResponsePayload{}
	err := proto.Unmarshal(prpBytes, prp)
	return prp, errors.Wrap(err, "error unmarshaling ProposalResponsePayload")
}

// GetProposal returns a Proposal message from its bytes
func GetProposal(propBytes []byte) (*peer.Proposal, error) {
	prop := &peer.Proposal{}
	err := proto.Unmarshal(propBytes, prop)
	return prop, errors.Wrap(err, "error unmarshaling Proposal")
}

// GetPayload Get Payload from Envelope message
func GetPayload(e *common.Envelope) (*common.Payload, error) {
	payload := &common.Payload{}
	err := proto.Unmarshal(e.Payload, payload)
	return payload, errors.Wrap(err, "error unmarshaling Payload")
}

// GetTransaction Get Transaction from bytes
func GetTransaction(txBytes []byte) (*peer.Transaction, error) {
	tx := &peer.Transaction{}
	err := proto.Unmarshal(txBytes, tx)
	return tx, errors.Wrap(err, "error unmarshaling Transaction")

}

// GetChaincodeActionPayload Get ChaincodeActionPayload from bytes
func GetChaincodeActionPayload(capBytes []byte) (*peer.ChaincodeActionPayload, error) {
	cap := &peer.ChaincodeActionPayload{}
	err := proto.Unmarshal(capBytes, cap)
	return cap, errors.Wrap(err, "error unmarshaling ChaincodeActionPayload")
}

// GetChaincodeProposalPayload Get ChaincodeProposalPayload from bytes
func GetChaincodeProposalPayload(bytes []byte) (*peer.ChaincodeProposalPayload, error) {
	cpp := &peer.ChaincodeProposalPayload{}
	err := proto.Unmarshal(bytes, cpp)
	return cpp, errors.Wrap(err, "error unmarshaling ChaincodeProposalPayload")
}

// GetSignatureHeader Get SignatureHeader from bytes
func GetSignatureHeader(bytes []byte) (*common.SignatureHeader, error) {
	return UnmarshalSignatureHeader(bytes)
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
	txid, err := ComputeTxID(nonce, creator)
	if err != nil {
		return nil, "", err
	}

	return CreateChaincodeProposalWithTxIDNonceAndTransient(txid, typ, chainID, cis, nonce, creator, transientMap)
}

// CreateChaincodeProposalWithTxIDAndTransient creates a proposal from given
// input. It returns the proposal and the transaction id associated with the
// proposal
func CreateChaincodeProposalWithTxIDAndTransient(typ common.HeaderType, chainID string, cis *peer.ChaincodeInvocationSpec, creator []byte, txid string, transientMap map[string][]byte) (*peer.Proposal, string, error) {
	// generate a random nonce
	nonce, err := crypto.GetRandomNonce()
	if err != nil {
		return nil, "", err
	}

	// compute txid unless provided by tests
	if txid == "" {
		txid, err = ComputeTxID(nonce, creator)
		if err != nil {
			return nil, "", err
		}
	}

	return CreateChaincodeProposalWithTxIDNonceAndTransient(txid, typ, chainID, cis, nonce, creator, transientMap)
}

// CreateChaincodeProposalWithTxIDNonceAndTransient creates a proposal from
// given input
func CreateChaincodeProposalWithTxIDNonceAndTransient(txid string, typ common.HeaderType, chainID string, cis *peer.ChaincodeInvocationSpec, nonce, creator []byte, transientMap map[string][]byte) (*peer.Proposal, string, error) {
	ccHdrExt := &peer.ChaincodeHeaderExtension{ChaincodeId: cis.ChaincodeSpec.ChaincodeId}
	ccHdrExtBytes, err := proto.Marshal(ccHdrExt)
	if err != nil {
		return nil, "", errors.Wrap(err, "error marshaling ChaincodeHeaderExtension")
	}

	cisBytes, err := proto.Marshal(cis)
	if err != nil {
		return nil, "", errors.Wrap(err, "error marshaling ChaincodeInvocationSpec")
	}

	ccPropPayload := &peer.ChaincodeProposalPayload{Input: cisBytes, TransientMap: transientMap}
	ccPropPayloadBytes, err := proto.Marshal(ccPropPayload)
	if err != nil {
		return nil, "", errors.Wrap(err, "error marshaling ChaincodeProposalPayload")
	}

	// TODO: epoch is now set to zero. This must be changed once we
	// get a more appropriate mechanism to handle it in.
	var epoch uint64

	timestamp := util.CreateUtcTimestamp()

	hdr := &common.Header{
		ChannelHeader: MarshalOrPanic(
			&common.ChannelHeader{
				Type:      int32(typ),
				TxId:      txid,
				Timestamp: timestamp,
				ChannelId: chainID,
				Extension: ccHdrExtBytes,
				Epoch:     epoch,
			},
		),
		SignatureHeader: MarshalOrPanic(
			&common.SignatureHeader{
				Nonce:   nonce,
				Creator: creator,
			},
		),
	}

	hdrBytes, err := proto.Marshal(hdr)
	if err != nil {
		return nil, "", err
	}

	prop := &peer.Proposal{
		Header:  hdrBytes,
		Payload: ccPropPayloadBytes,
	}
	return prop, txid, nil
}

// GetBytesProposalResponsePayload gets proposal response payload
func GetBytesProposalResponsePayload(hash []byte, response *peer.Response, result []byte, event []byte, ccid *peer.ChaincodeID) ([]byte, error) {
	cAct := &peer.ChaincodeAction{
		Events: event, Results: result,
		Response:    response,
		ChaincodeId: ccid,
	}
	cActBytes, err := proto.Marshal(cAct)
	if err != nil {
		return nil, errors.Wrap(err, "error marshaling ChaincodeAction")
	}

	prp := &peer.ProposalResponsePayload{
		Extension:    cActBytes,
		ProposalHash: hash,
	}
	prpBytes, err := proto.Marshal(prp)
	return prpBytes, errors.Wrap(err, "error marshaling ProposalResponsePayload")
}

// GetBytesChaincodeProposalPayload gets the chaincode proposal payload
func GetBytesChaincodeProposalPayload(cpp *peer.ChaincodeProposalPayload) ([]byte, error) {
	cppBytes, err := proto.Marshal(cpp)
	return cppBytes, errors.Wrap(err, "error marshaling ChaincodeProposalPayload")
}

// GetBytesResponse gets the bytes of Response
func GetBytesResponse(res *peer.Response) ([]byte, error) {
	resBytes, err := proto.Marshal(res)
	return resBytes, errors.Wrap(err, "error marshaling Response")
}

// GetBytesChaincodeEvent gets the bytes of ChaincodeEvent
func GetBytesChaincodeEvent(event *peer.ChaincodeEvent) ([]byte, error) {
	eventBytes, err := proto.Marshal(event)
	return eventBytes, errors.Wrap(err, "error marshaling ChaincodeEvent")
}

// GetBytesChaincodeActionPayload get the bytes of ChaincodeActionPayload from
// the message
func GetBytesChaincodeActionPayload(cap *peer.ChaincodeActionPayload) ([]byte, error) {
	capBytes, err := proto.Marshal(cap)
	return capBytes, errors.Wrap(err, "error marshaling ChaincodeActionPayload")
}

// GetBytesProposalResponse gets proposal bytes response
func GetBytesProposalResponse(pr *peer.ProposalResponse) ([]byte, error) {
	respBytes, err := proto.Marshal(pr)
	return respBytes, errors.Wrap(err, "error marshaling ProposalResponse")
}

// GetBytesProposal returns the bytes of a proposal message
func GetBytesProposal(prop *peer.Proposal) ([]byte, error) {
	propBytes, err := proto.Marshal(prop)
	return propBytes, errors.Wrap(err, "error marshaling Proposal")
}

// GetBytesHeader get the bytes of Header from the message
func GetBytesHeader(hdr *common.Header) ([]byte, error) {
	bytes, err := proto.Marshal(hdr)
	return bytes, errors.Wrap(err, "error marshaling Header")
}

// GetBytesSignatureHeader get the bytes of SignatureHeader from the message
func GetBytesSignatureHeader(hdr *common.SignatureHeader) ([]byte, error) {
	bytes, err := proto.Marshal(hdr)
	return bytes, errors.Wrap(err, "error marshaling SignatureHeader")
}

// GetBytesTransaction get the bytes of Transaction from the message
func GetBytesTransaction(tx *peer.Transaction) ([]byte, error) {
	bytes, err := proto.Marshal(tx)
	return bytes, errors.Wrap(err, "error unmarshaling Transaction")
}

// GetBytesPayload get the bytes of Payload from the message
func GetBytesPayload(payl *common.Payload) ([]byte, error) {
	bytes, err := proto.Marshal(payl)
	return bytes, errors.Wrap(err, "error marshaling Payload")
}

// GetBytesEnvelope get the bytes of Envelope from the message
func GetBytesEnvelope(env *common.Envelope) ([]byte, error) {
	bytes, err := proto.Marshal(env)
	return bytes, errors.Wrap(err, "error marshaling Envelope")
}

// GetActionFromEnvelope extracts a ChaincodeAction message from a
// serialized Envelope
// TODO: fix function name as per FAB-11831
func GetActionFromEnvelope(envBytes []byte) (*peer.ChaincodeAction, error) {
	env, err := GetEnvelopeFromBlock(envBytes)
	if err != nil {
		return nil, err
	}
	return GetActionFromEnvelopeMsg(env)
}

func GetActionFromEnvelopeMsg(env *common.Envelope) (*peer.ChaincodeAction, error) {
	payl, err := GetPayload(env)
	if err != nil {
		return nil, err
	}

	tx, err := GetTransaction(payl.Data)
	if err != nil {
		return nil, err
	}

	if len(tx.Actions) == 0 {
		return nil, errors.New("at least one TransactionAction required")
	}

	_, respPayload, err := GetPayloads(tx.Actions[0])
	return respPayload, err
}

// CreateProposalFromCISAndTxid returns a proposal given a serialized identity
// and a ChaincodeInvocationSpec
func CreateProposalFromCISAndTxid(txid string, typ common.HeaderType, chainID string, cis *peer.ChaincodeInvocationSpec, creator []byte) (*peer.Proposal, string, error) {
	nonce, err := crypto.GetRandomNonce()
	if err != nil {
		return nil, "", err
	}
	return CreateChaincodeProposalWithTxIDNonceAndTransient(txid, typ, chainID, cis, nonce, creator, nil)
}

// CreateProposalFromCIS returns a proposal given a serialized identity and a
// ChaincodeInvocationSpec
func CreateProposalFromCIS(typ common.HeaderType, chainID string, cis *peer.ChaincodeInvocationSpec, creator []byte) (*peer.Proposal, string, error) {
	return CreateChaincodeProposal(typ, chainID, cis, creator)
}

// CreateGetChaincodesProposal returns a GETCHAINCODES proposal given a
// serialized identity
func CreateGetChaincodesProposal(chainID string, creator []byte) (*peer.Proposal, string, error) {
	ccinp := &peer.ChaincodeInput{Args: [][]byte{[]byte("getchaincodes")}}
	lsccSpec := &peer.ChaincodeInvocationSpec{
		ChaincodeSpec: &peer.ChaincodeSpec{
			Type:        peer.ChaincodeSpec_GOLANG,
			ChaincodeId: &peer.ChaincodeID{Name: "lscc"},
			Input:       ccinp,
		},
	}
	return CreateProposalFromCIS(common.HeaderType_ENDORSER_TRANSACTION, chainID, lsccSpec, creator)
}

// CreateGetInstalledChaincodesProposal returns a GETINSTALLEDCHAINCODES
// proposal given a serialized identity
func CreateGetInstalledChaincodesProposal(creator []byte) (*peer.Proposal, string, error) {
	ccinp := &peer.ChaincodeInput{Args: [][]byte{[]byte("getinstalledchaincodes")}}
	lsccSpec := &peer.ChaincodeInvocationSpec{
		ChaincodeSpec: &peer.ChaincodeSpec{
			Type:        peer.ChaincodeSpec_GOLANG,
			ChaincodeId: &peer.ChaincodeID{Name: "lscc"},
			Input:       ccinp,
		},
	}
	return CreateProposalFromCIS(common.HeaderType_ENDORSER_TRANSACTION, "", lsccSpec, creator)
}

// CreateInstallProposalFromCDS returns a install proposal given a serialized
// identity and a ChaincodeDeploymentSpec
func CreateInstallProposalFromCDS(ccpack proto.Message, creator []byte) (*peer.Proposal, string, error) {
	return createProposalFromCDS("", ccpack, creator, "install")
}

// CreateDeployProposalFromCDS returns a deploy proposal given a serialized
// identity and a ChaincodeDeploymentSpec
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
	}
	return createProposalFromCDS(chainID, cds, creator, "deploy", policy, escc, vscc, collectionConfig)
}

// CreateUpgradeProposalFromCDS returns a upgrade proposal given a serialized
// identity and a ChaincodeDeploymentSpec
func CreateUpgradeProposalFromCDS(
	chainID string,
	cds *peer.ChaincodeDeploymentSpec,
	creator []byte,
	policy []byte,
	escc []byte,
	vscc []byte,
	collectionConfig []byte) (*peer.Proposal, string, error) {
	if collectionConfig == nil {
		return createProposalFromCDS(chainID, cds, creator, "upgrade", policy, escc, vscc)
	}
	return createProposalFromCDS(chainID, cds, creator, "upgrade", policy, escc, vscc, collectionConfig)
}

// createProposalFromCDS returns a deploy or upgrade proposal given a
// serialized identity and a ChaincodeDeploymentSpec
func createProposalFromCDS(chainID string, msg proto.Message, creator []byte, propType string, args ...[]byte) (*peer.Proposal, string, error) {
	// in the new mode, cds will be nil, "deploy" and "upgrade" are instantiates.
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
			return nil, "", errors.New("invalid message for creating lifecycle chaincode proposal")
		}
		Args := [][]byte{[]byte(propType), []byte(chainID), b}
		Args = append(Args, args...)

		ccinp = &peer.ChaincodeInput{Args: Args}
	case "install":
		ccinp = &peer.ChaincodeInput{Args: [][]byte{[]byte(propType), b}}
	}

	// wrap the deployment in an invocation spec to lscc...
	lsccSpec := &peer.ChaincodeInvocationSpec{
		ChaincodeSpec: &peer.ChaincodeSpec{
			Type:        peer.ChaincodeSpec_GOLANG,
			ChaincodeId: &peer.ChaincodeID{Name: "lscc"},
			Input:       ccinp,
		},
	}

	// ...and get the proposal for it
	return CreateProposalFromCIS(common.HeaderType_ENDORSER_TRANSACTION, chainID, lsccSpec, creator)
}

// ComputeTxID computes TxID as the Hash computed
// over the concatenation of nonce and creator.
func ComputeTxID(nonce, creator []byte) (string, error) {
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

// CheckTxID checks that txid is equal to the Hash computed
// over the concatenation of nonce and creator.
func CheckTxID(txid string, nonce, creator []byte) error {
	computedTxID, err := ComputeTxID(nonce, creator)
	if err != nil {
		return errors.WithMessage(err, "error computing target txid")
	}

	if txid != computedTxID {
		return errors.Errorf("invalid txid. got [%s], expected [%s]", txid, computedTxID)
	}

	return nil
}

// ComputeProposalBinding computes the binding of a proposal
func ComputeProposalBinding(proposal *peer.Proposal) ([]byte, error) {
	if proposal == nil {
		return nil, errors.New("proposal is nil")
	}
	if len(proposal.Header) == 0 {
		return nil, errors.New("proposal's header is nil")
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

	// TODO: add to genesis block the hash function used for
	// the binding computation
	return factory.GetDefault().Hash(
		append(append(nonce, creator...), epochBytes...),
		&bccsp.SHA256Opts{})
}
