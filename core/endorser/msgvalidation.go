/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser

import (
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/flogging"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/msp"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

var putilsLogger = flogging.MustGetLogger("protoutils")

// validateChaincodeProposalMessage checks the validity of a Proposal message of type CHAINCODE
func validateChaincodeProposalMessage(prop *pb.Proposal, hdr *common.Header, chdr *common.ChannelHeader) (*pb.ChaincodeHeaderExtension, error) {
	if prop == nil || hdr == nil {
		return nil, errors.New("nil arguments")
	}

	putilsLogger.Debugf("validateChaincodeProposalMessage starts for proposal %p, header %p", prop, hdr)

	// 4) based on the header type (assuming it's CHAINCODE), look at the extensions
	chaincodeHdrExt, err := protoutil.UnmarshalChaincodeHeaderExtension(chdr.Extension)
	if err != nil {
		return nil, errors.New("invalid header extension for type CHAINCODE")
	}

	if chaincodeHdrExt.ChaincodeId == nil {
		return nil, errors.New("ChaincodeHeaderExtension.ChaincodeId is nil")
	}

	putilsLogger.Debugf("validateChaincodeProposalMessage info: header extension references chaincode %s", chaincodeHdrExt.ChaincodeId)

	//    - ensure that the chaincodeID is correct (?)
	// TODO: should we even do this? If so, using which interface?

	//    - ensure that the visibility field has some value we understand
	// currently the fabric only supports full visibility: this means that
	// there are no restrictions on which parts of the proposal payload will
	// be visible in the final transaction; this default approach requires
	// no additional instructions in the PayloadVisibility field which is
	// therefore expected to be nil; however the fabric may be extended to
	// encode more elaborate visibility mechanisms that shall be encoded in
	// this field (and handled appropriately by the peer)
	if chaincodeHdrExt.PayloadVisibility != nil {
		return nil, errors.New("invalid payload visibility field")
	}

	return chaincodeHdrExt, nil
}

// ValidateProposalMessage checks the validity of a SignedProposal message
// this function returns Header and ChaincodeHeaderExtension messages since they
// have been unmarshalled and validated
func ValidateProposalMessage(signedProp *pb.SignedProposal) (*pb.Proposal, *common.Header, *pb.ChaincodeHeaderExtension, error) {
	if signedProp == nil {
		return nil, nil, nil, errors.New("nil arguments")
	}

	putilsLogger.Debugf("ValidateProposalMessage starts for signed proposal %p", signedProp)

	// extract the Proposal message from signedProp
	prop, err := protoutil.UnmarshalProposal(signedProp.ProposalBytes)
	if err != nil {
		return nil, nil, nil, err
	}

	// 1) look at the ProposalHeader
	hdr, err := protoutil.UnmarshalHeader(prop.Header)
	if err != nil {
		return nil, nil, nil, err
	}

	// validate the header
	chdr, shdr, err := validateCommonHeader(hdr)
	if err != nil {
		return nil, nil, nil, err
	}

	// validate the signature
	err = checkSignatureFromCreator(shdr.Creator, signedProp.Signature, signedProp.ProposalBytes, chdr.ChannelId)
	if err != nil {
		// log the exact message on the peer but return a generic error message to
		// avoid malicious users scanning for channels
		putilsLogger.Warningf("channel [%s]: %s", chdr.ChannelId, err)
		sId := &msp.SerializedIdentity{}
		err := proto.Unmarshal(shdr.Creator, sId)
		if err != nil {
			// log the error here as well but still only return the generic error
			err = errors.Wrap(err, "could not deserialize a SerializedIdentity")
			putilsLogger.Warningf("channel [%s]: %s", chdr.ChannelId, err)
		}
		return nil, nil, nil, errors.Errorf("access denied: channel [%s] creator org [%s]", chdr.ChannelId, sId.Mspid)
	}

	// Verify that the transaction ID has been computed properly.
	// This check is needed to ensure that the lookup into the ledger
	// for the same TxID catches duplicates.
	err = protoutil.CheckTxID(
		chdr.TxId,
		shdr.Nonce,
		shdr.Creator)
	if err != nil {
		return nil, nil, nil, err
	}

	// continue the validation in a way that depends on the type specified in the header
	switch common.HeaderType(chdr.Type) {
	case common.HeaderType_CONFIG:
		//which the types are different the validation is the same
		//viz, validate a proposal to a chaincode. If we need other
		//special validation for confguration, we would have to implement
		//special validation
		fallthrough
	case common.HeaderType_ENDORSER_TRANSACTION:
		// validation of the proposal message knowing it's of type CHAINCODE
		chaincodeHdrExt, err := validateChaincodeProposalMessage(prop, hdr, chdr)
		if err != nil {
			return nil, nil, nil, err
		}

		return prop, hdr, chaincodeHdrExt, err
	default:
		//NOTE : we proably need a case
		return nil, nil, nil, errors.Errorf("unsupported proposal type %d", common.HeaderType(chdr.Type))
	}
}

// given a creator, a message and a signature,
// this function returns nil if the creator
// is a valid cert and the signature is valid
func checkSignatureFromCreator(creatorBytes []byte, sig []byte, msg []byte, ChainID string) error {
	putilsLogger.Debugf("begin")

	// check for nil argument
	if creatorBytes == nil || sig == nil || msg == nil {
		return errors.New("nil arguments")
	}

	mspObj := mspmgmt.GetIdentityDeserializer(ChainID)
	if mspObj == nil {
		return errors.Errorf("could not get msp for channel [%s]", ChainID)
	}

	// get the identity of the creator
	creator, err := mspObj.DeserializeIdentity(creatorBytes)
	if err != nil {
		return errors.WithMessage(err, "MSP error")
	}

	putilsLogger.Debugf("creator is %s", creator.GetIdentifier())

	// ensure that creator is a valid certificate
	err = creator.Validate()
	if err != nil {
		return errors.WithMessage(err, "creator certificate is not valid")
	}

	putilsLogger.Debugf("creator is valid")

	// validate the signature
	err = creator.Verify(msg, sig)
	if err != nil {
		return errors.WithMessage(err, "creator's signature over the proposal is not valid")
	}

	putilsLogger.Debugf("exits successfully")

	return nil
}

// checks for a valid SignatureHeader
func validateSignatureHeader(sHdr *common.SignatureHeader) error {
	// check for nil argument
	if sHdr == nil {
		return errors.New("nil SignatureHeader provided")
	}

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
	// check for nil argument
	if cHdr == nil {
		return errors.New("nil ChannelHeader provided")
	}

	// validate the header type
	switch common.HeaderType(cHdr.Type) {
	case common.HeaderType_ENDORSER_TRANSACTION:
	case common.HeaderType_CONFIG_UPDATE:
	case common.HeaderType_CONFIG:
	default:
		return errors.Errorf("invalid header type %s", common.HeaderType(cHdr.Type))
	}

	putilsLogger.Debugf("validateChannelHeader info: header type %d", common.HeaderType(cHdr.Type))

	// TODO: validate chainID in cHdr.ChainID

	// Validate epoch in cHdr.Epoch
	// Currently we enforce that Epoch is 0.
	// TODO: This check will be modified once the Epoch management
	// will be in place.
	if cHdr.Epoch != 0 {
		return errors.Errorf("invalid Epoch in ChannelHeader. Expected 0, got [%d]", cHdr.Epoch)
	}

	// TODO: Validate version in cHdr.Version

	return nil
}

// checks for a valid Header
func validateCommonHeader(hdr *common.Header) (*common.ChannelHeader, *common.SignatureHeader, error) {
	if hdr == nil {
		return nil, nil, errors.New("nil header")
	}

	chdr, err := protoutil.UnmarshalChannelHeader(hdr.ChannelHeader)
	if err != nil {
		return nil, nil, err
	}

	shdr, err := protoutil.UnmarshalSignatureHeader(hdr.SignatureHeader)
	if err != nil {
		return nil, nil, err
	}

	err = validateChannelHeader(chdr)
	if err != nil {
		return nil, nil, err
	}

	err = validateSignatureHeader(shdr)
	if err != nil {
		return nil, nil, err
	}

	return chdr, shdr, nil
}
