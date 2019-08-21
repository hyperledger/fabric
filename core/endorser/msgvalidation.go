/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser

import (
	"crypto/sha256"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protos/common"
	cb "github.com/hyperledger/fabric/protos/common"
	mspproto "github.com/hyperledger/fabric/protos/msp"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

// UnpackedProposal contains the interesting artifacts from inside the proposal.
type UnpackedProposal struct {
	ChaincodeName   string
	ChannelHeader   *cb.ChannelHeader
	Input           *pb.ChaincodeInput
	Proposal        *pb.Proposal
	SignatureHeader *cb.SignatureHeader
	SignedProposal  *pb.SignedProposal
	ProposalHash    []byte
}

func (up *UnpackedProposal) ChannelID() string {
	return up.ChannelHeader.ChannelId
}

func (up *UnpackedProposal) TxID() string {
	return up.ChannelHeader.TxId
}

// UnpackProposal creates an an *UnpackedProposal which is guaranteed to have
// no zero-ed fields or it returns an error.
func UnpackProposal(signedProp *pb.SignedProposal) (*UnpackedProposal, error) {
	prop, err := protoutil.UnmarshalProposal(signedProp.ProposalBytes)
	if err != nil {
		return nil, err
	}

	hdr, err := protoutil.UnmarshalHeader(prop.Header)
	if err != nil {
		return nil, err
	}

	chdr, err := protoutil.UnmarshalChannelHeader(hdr.ChannelHeader)
	if err != nil {
		return nil, err
	}

	shdr, err := protoutil.UnmarshalSignatureHeader(hdr.SignatureHeader)
	if err != nil {
		return nil, err
	}

	chaincodeHdrExt, err := protoutil.UnmarshalChaincodeHeaderExtension(chdr.Extension)
	if err != nil {
		return nil, err
	}

	if chaincodeHdrExt.ChaincodeId == nil {
		return nil, errors.Errorf("ChaincodeHeaderExtension.ChaincodeId is nil")
	}

	if chaincodeHdrExt.ChaincodeId.Name == "" {
		return nil, errors.Errorf("ChaincodeHeaderExtension.ChaincodeId.Name is empty")
	}

	cpp, err := protoutil.UnmarshalChaincodeProposalPayload(prop.Payload)
	if err != nil {
		return nil, err
	}

	cis, err := protoutil.UnmarshalChaincodeInvocationSpec(cpp.Input)
	if err != nil {
		return nil, err
	}

	if cis.ChaincodeSpec == nil {
		return nil, errors.Errorf("chaincode invocation spec did not contain chaincode spec")
	}

	if cis.ChaincodeSpec.Input == nil {
		return nil, errors.Errorf("chaincode input did not contain any input")
	}

	cppNoTransient := &pb.ChaincodeProposalPayload{Input: cpp.Input, TransientMap: nil}
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
	propHash.Write(hdr.ChannelHeader)
	propHash.Write(hdr.SignatureHeader)
	propHash.Write(ppBytes)

	return &UnpackedProposal{
		SignedProposal:  signedProp,
		Proposal:        prop,
		ChannelHeader:   chdr,
		SignatureHeader: shdr,
		ChaincodeName:   chaincodeHdrExt.ChaincodeId.Name,
		Input:           cis.ChaincodeSpec.Input,
		ProposalHash:    propHash.Sum(nil)[:],
	}, nil
}

// ValidateUnpackedProposal checks the validity of an unpacked proposal,
// considering its signatures, its header values, including creator, nonce,
// and txid.
func ValidateUnpackedProposal(unpackedProp *UnpackedProposal, idDeserializer msp.IdentityDeserializer) error {
	if err := validateChannelHeader(unpackedProp.ChannelHeader); err != nil {
		return err
	}

	if err := validateSignatureHeader(unpackedProp.SignatureHeader); err != nil {
		return err
	}

	// validate the signature
	err := checkSignatureFromCreator(
		unpackedProp.SignatureHeader.Creator,
		unpackedProp.SignedProposal.Signature,
		unpackedProp.SignedProposal.ProposalBytes,
		unpackedProp.ChannelHeader.ChannelId,
		idDeserializer,
	)
	if err != nil {
		// log the exact message on the peer but return a generic error message to
		// avoid malicious users scanning for channels
		endorserLogger.Warningf("channel [%s]: %s", unpackedProp.ChannelHeader.ChannelId, err)
		sId := &mspproto.SerializedIdentity{}
		err := proto.Unmarshal(unpackedProp.SignatureHeader.Creator, sId)
		if err != nil {
			// log the error here as well but still only return the generic error
			err = errors.Wrap(err, "could not deserialize a SerializedIdentity")
			endorserLogger.Warningf("channel [%s]: %s", unpackedProp.ChannelHeader.ChannelId, err)
		}
		return errors.Errorf("access denied: channel [%s] creator org [%s]", unpackedProp.ChannelHeader.ChannelId, sId.Mspid)
	}

	// Verify that the transaction ID has been computed properly.
	// This check is needed to ensure that the lookup into the ledger
	// for the same TxID catches duplicates.
	err = protoutil.CheckTxID(
		unpackedProp.ChannelHeader.TxId,
		unpackedProp.SignatureHeader.Nonce,
		unpackedProp.SignatureHeader.Creator,
	)
	if err != nil {
		return err
	}

	return nil
}

// given a creator, a message and a signature,
// this function returns nil if the creator
// is a valid cert and the signature is valid
func checkSignatureFromCreator(creatorBytes []byte, sig []byte, msg []byte, ChainID string, idDeserializer msp.IdentityDeserializer) error {
	// check for nil argument
	if creatorBytes == nil || sig == nil || msg == nil {
		return errors.New("nil arguments")
	}

	// get the identity of the creator
	creator, err := idDeserializer.DeserializeIdentity(creatorBytes)
	if err != nil {
		return errors.WithMessage(err, "MSP error")
	}

	endorserLogger.Debugf("creator is %s", creator.GetIdentifier())

	// ensure that creator is a valid certificate
	err = creator.Validate()
	if err != nil {
		return errors.WithMessage(err, "creator certificate is not valid")
	}

	endorserLogger.Debugf("creator is valid")

	// validate the signature
	err = creator.Verify(msg, sig)
	if err != nil {
		return errors.WithMessage(err, "creator's signature over the proposal is not valid")
	}

	endorserLogger.Debugf("exits successfully")

	return nil
}

// checks for a valid SignatureHeader
func validateSignatureHeader(sHdr *common.SignatureHeader) error {
	// ensure that there is a nonce
	if sHdr.Nonce == nil || len(sHdr.Nonce) == 0 {
		return errors.New("invalid nonce specified in the header")
	}

	// ensure that there is a creator
	if sHdr.Creator == nil || len(sHdr.Creator) == 0 {
		return errors.New("invalid creator specified in the header")
	}

	return nil
}

// checks for a valid ChannelHeader
func validateChannelHeader(cHdr *common.ChannelHeader) error {
	// validate the header type
	switch common.HeaderType(cHdr.Type) {
	case common.HeaderType_ENDORSER_TRANSACTION:
	case common.HeaderType_CONFIG:
		// The CONFIG transaction type has _no_ business coming to the propose API.
		// In fact, anything coming to the Propose API is by definition an endorser
		// transaction, so any other header type seems like it ought to be an error... oh well.

	default:
		return errors.Errorf("invalid header type %s", common.HeaderType(cHdr.Type))
	}

	return nil
}
