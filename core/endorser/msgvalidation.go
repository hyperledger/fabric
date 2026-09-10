/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser

import (
	"crypto/sha256"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

// UnpackedProposal contains the interesting artifacts from inside the proposal.
type UnpackedProposal struct {
	ChaincodeName   string
	ChannelHeader   *common.ChannelHeader
	Input           *peer.ChaincodeInput
	Proposal        *peer.Proposal
	SignatureHeader *common.SignatureHeader
	SignedProposal  *peer.SignedProposal
	ProposalHash    []byte
}

func (up *UnpackedProposal) ChannelID() string {
	return up.ChannelHeader.GetChannelId()
}

func (up *UnpackedProposal) TxID() string {
	return up.ChannelHeader.GetTxId()
}

// UnpackProposal creates an *UnpackedProposal which is guaranteed to have
// no zero-ed fields or it returns an error.
func UnpackProposal(signedProp *peer.SignedProposal) (*UnpackedProposal, error) {
	prop, err := protoutil.UnmarshalProposal(signedProp.GetProposalBytes())
	if err != nil {
		return nil, err
	}

	hdr, err := protoutil.UnmarshalHeader(prop.GetHeader())
	if err != nil {
		return nil, err
	}

	chdr, err := protoutil.UnmarshalChannelHeader(hdr.GetChannelHeader())
	if err != nil {
		return nil, err
	}

	shdr, err := protoutil.UnmarshalSignatureHeader(hdr.GetSignatureHeader())
	if err != nil {
		return nil, err
	}

	chaincodeHdrExt, err := protoutil.UnmarshalChaincodeHeaderExtension(chdr.GetExtension())
	if err != nil {
		return nil, err
	}

	if chaincodeHdrExt.GetChaincodeId() == nil {
		return nil, errors.Errorf("ChaincodeHeaderExtension.ChaincodeId is nil")
	}

	if chaincodeHdrExt.GetChaincodeId().GetName() == "" {
		return nil, errors.Errorf("ChaincodeHeaderExtension.ChaincodeId.Name is empty")
	}

	cpp, err := protoutil.UnmarshalChaincodeProposalPayload(prop.GetPayload())
	if err != nil {
		return nil, err
	}

	cis, err := protoutil.UnmarshalChaincodeInvocationSpec(cpp.GetInput())
	if err != nil {
		return nil, err
	}

	if cis.GetChaincodeSpec() == nil {
		return nil, errors.Errorf("chaincode invocation spec did not contain chaincode spec")
	}

	if cis.GetChaincodeSpec().GetInput() == nil {
		return nil, errors.Errorf("chaincode input did not contain any input")
	}

	cppNoTransient := &peer.ChaincodeProposalPayload{Input: cpp.GetInput(), TransientMap: nil}
	ppBytes, err := proto.Marshal(cppNoTransient)
	if err != nil {
		return nil, errors.WithMessage(err, "could not marshal non-transient portion of payload")
	}

	// TODO, this was preserved from the proputils stuff, but should this be BCCSP?

	// The proposal hash is the hash of the concatenation of:
	// 1) The serialized Channel Header object
	// 2) The serialized Signature Header object
	// 3) The hash of the part of the chaincode proposal payload that will go to the tx
	// (ie, the parts without the transient data)
	propHash := sha256.New()
	propHash.Write(hdr.GetChannelHeader())
	propHash.Write(hdr.GetSignatureHeader())
	propHash.Write(ppBytes)

	return &UnpackedProposal{
		SignedProposal:  signedProp,
		Proposal:        prop,
		ChannelHeader:   chdr,
		SignatureHeader: shdr,
		ChaincodeName:   chaincodeHdrExt.GetChaincodeId().GetName(),
		Input:           cis.GetChaincodeSpec().GetInput(),
		ProposalHash:    propHash.Sum(nil)[:],
	}, nil
}

func (up *UnpackedProposal) Validate(idDeserializer msp.IdentityDeserializer) error {
	logger := decorateLogger(endorserLogger, &ccprovider.TransactionParams{
		ChannelID: up.ChannelHeader.GetChannelId(),
		TxID:      up.TxID(),
	})

	// validate the header type
	switch common.HeaderType(up.ChannelHeader.GetType()) {
	case common.HeaderType_ENDORSER_TRANSACTION:
	case common.HeaderType_CONFIG:
		// The CONFIG transaction type has _no_ business coming to the propose API.
		// In fact, anything coming to the Propose API is by definition an endorser
		// transaction, so any other header type seems like it ought to be an error... oh well.

	default:
		return errors.Errorf("invalid header type %s", common.HeaderType(up.ChannelHeader.GetType()))
	}

	// ensure the epoch is 0
	if up.ChannelHeader.GetEpoch() != 0 {
		return errors.Errorf("epoch is non-zero")
	}

	// ensure that there is a nonce
	if len(up.SignatureHeader.GetNonce()) == 0 {
		return errors.Errorf("nonce is empty")
	}

	// ensure that there is a creator
	if len(up.SignatureHeader.GetCreator()) == 0 {
		return errors.New("creator is empty")
	}

	expectedTxID := protoutil.ComputeTxID(up.SignatureHeader.GetNonce(), up.SignatureHeader.GetCreator())
	if up.TxID() != expectedTxID {
		return errors.Errorf("incorrectly computed txid '%s' -- expected '%s'", up.TxID(), expectedTxID)
	}

	if up.SignedProposal.ProposalBytes == nil {
		return errors.Errorf("empty proposal bytes")
	}

	if up.SignedProposal.Signature == nil {
		return errors.Errorf("empty signature bytes")
	}

	// get the identity of the creator
	creator, err := idDeserializer.DeserializeIdentity(up.SignatureHeader.GetCreator())
	if err != nil {
		logger.Warnw("access denied", "error", err, "identity", protoutil.LogMessageForSerializedIdentity(up.SignatureHeader.GetCreator()))
		return errors.Errorf("access denied: channel [%s] creator org unknown, creator is malformed", up.ChannelID())
	}

	genericAuthError := errors.Errorf("access denied: channel [%s] creator org [%s]", up.ChannelID(), creator.GetMSPIdentifier())
	// ensure that creator is a valid certificate
	err = creator.Validate()
	if err != nil {
		logger.Warnw("access denied: identity is not valid", "error", err, "identity", protoutil.LogMessageForSerializedIdentity(up.SignatureHeader.GetCreator()))
		return genericAuthError
	}

	logger = logger.With("mspID", creator.GetMSPIdentifier())

	logger.Debug("creator is valid")

	// validate the signature
	err = creator.Verify(up.SignedProposal.GetProposalBytes(), up.SignedProposal.GetSignature())
	if err != nil {
		logger.Warnw("access denied: creator's signature over the proposal is not valid", "error", err, "identity", protoutil.LogMessageForSerializedIdentity(up.SignatureHeader.GetCreator()))
		return genericAuthError
	}

	logger.Debug("signature is valid")

	return nil
}
