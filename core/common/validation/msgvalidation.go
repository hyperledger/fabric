/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package validation

import (
	"bytes"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/flogging"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/msp"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
)

var putilsLogger = flogging.MustGetLogger("protoutils")

// validateChaincodeProposalMessage checks the validity of a Proposal message of type CHAINCODE
func validateChaincodeProposalMessage(prop *pb.Proposal, hdr *common.Header) (*pb.ChaincodeHeaderExtension, error) {
	if prop == nil || hdr == nil {
		return nil, errors.New("nil arguments")
	}

	putilsLogger.Debugf("validateChaincodeProposalMessage starts for proposal %p, header %p", prop, hdr)

	// 4) based on the header type (assuming it's CHAINCODE), look at the extensions
	chaincodeHdrExt, err := utils.GetChaincodeHeaderExtension(hdr)
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
	prop, err := utils.GetProposal(signedProp.ProposalBytes)
	if err != nil {
		return nil, nil, nil, err
	}

	// 1) look at the ProposalHeader
	hdr, err := utils.GetHeader(prop.Header)
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
	err = utils.CheckTxID(
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
		//special validation for configuration, we would have to implement
		//special validation
		fallthrough
	case common.HeaderType_ENDORSER_TRANSACTION:
		// validation of the proposal message knowing it's of type CHAINCODE
		chaincodeHdrExt, err := validateChaincodeProposalMessage(prop, hdr)
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
	if common.HeaderType(cHdr.Type) != common.HeaderType_ENDORSER_TRANSACTION &&
		common.HeaderType(cHdr.Type) != common.HeaderType_CONFIG_UPDATE &&
		common.HeaderType(cHdr.Type) != common.HeaderType_CONFIG &&
		common.HeaderType(cHdr.Type) != common.HeaderType_TOKEN_TRANSACTION {
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

	chdr, err := utils.UnmarshalChannelHeader(hdr.ChannelHeader)
	if err != nil {
		return nil, nil, err
	}

	shdr, err := utils.GetSignatureHeader(hdr.SignatureHeader)
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

// validateConfigTransaction validates the payload of a
// transaction assuming its type is CONFIG
func validateConfigTransaction(data []byte, hdr *common.Header) error {
	putilsLogger.Debugf("validateConfigTransaction starts for data %p, header %s", data, hdr)

	// check for nil argument
	if data == nil || hdr == nil {
		return errors.New("nil arguments")
	}

	// There is no need to do this validation here, the configtx.Validator handles this

	return nil
}

// validateEndorserTransaction validates the payload of a
// transaction assuming its type is ENDORSER_TRANSACTION
func validateEndorserTransaction(data []byte, hdr *common.Header) error {
	putilsLogger.Debugf("validateEndorserTransaction starts for data %p, header %s", data, hdr)

	// check for nil argument
	if data == nil || hdr == nil {
		return errors.New("nil arguments")
	}

	// if the type is ENDORSER_TRANSACTION we unmarshal a Transaction message
	tx, err := utils.GetTransaction(data)
	if err != nil {
		return err
	}

	// check for nil argument
	if tx == nil {
		return errors.New("nil transaction")
	}

	// TODO: validate tx.Version

	// TODO: validate ChaincodeHeaderExtension

	// hlf version 1 only supports a single action per transaction
	if len(tx.Actions) != 1 {
		return errors.Errorf("only one action per transaction is supported, tx contains %d", len(tx.Actions))
	}

	putilsLogger.Debugf("validateEndorserTransaction info: there are %d actions", len(tx.Actions))

	for _, act := range tx.Actions {
		// check for nil argument
		if act == nil {
			return errors.New("nil action")
		}

		// if the type is ENDORSER_TRANSACTION we unmarshal a SignatureHeader
		sHdr, err := utils.GetSignatureHeader(act.Header)
		if err != nil {
			return err
		}

		// validate the SignatureHeader - here we actually only
		// care about the nonce since the creator is in the outer header
		err = validateSignatureHeader(sHdr)
		if err != nil {
			return err
		}

		putilsLogger.Debugf("validateEndorserTransaction info: signature header is valid")

		// if the type is ENDORSER_TRANSACTION we unmarshal a ChaincodeActionPayload
		ccActionPayload, err := utils.GetChaincodeActionPayload(act.Payload)
		if err != nil {
			return err
		}

		// extract the proposal response payload
		prp, err := utils.GetProposalResponsePayload(ccActionPayload.Action.ProposalResponsePayload)
		if err != nil {
			return err
		}

		// build the original header by stitching together
		// the common ChannelHeader and the per-action SignatureHeader
		hdrOrig := &common.Header{ChannelHeader: hdr.ChannelHeader, SignatureHeader: act.Header}

		// compute proposalHash
		pHash, err := utils.GetProposalHash2(hdrOrig, ccActionPayload.ChaincodeProposalPayload)
		if err != nil {
			return err
		}

		// ensure that the proposal hash matches
		if bytes.Compare(pHash, prp.ProposalHash) != 0 {
			return errors.New("proposal hash does not match")
		}
	}

	return nil
}

// ValidateTransaction checks that the transaction envelope is properly formed
func ValidateTransaction(e *common.Envelope, c channelconfig.ApplicationCapabilities) (*common.Payload, pb.TxValidationCode) {
	putilsLogger.Debugf("ValidateTransactionEnvelope starts for envelope %p", e)

	// check for nil argument
	if e == nil {
		putilsLogger.Errorf("Error: nil envelope")
		return nil, pb.TxValidationCode_NIL_ENVELOPE
	}

	// get the payload from the envelope
	payload, err := utils.GetPayload(e)
	if err != nil {
		putilsLogger.Errorf("GetPayload returns err %s", err)
		return nil, pb.TxValidationCode_BAD_PAYLOAD
	}

	putilsLogger.Debugf("Header is %s", payload.Header)

	// validate the header
	chdr, shdr, err := validateCommonHeader(payload.Header)
	if err != nil {
		putilsLogger.Errorf("validateCommonHeader returns err %s", err)
		return nil, pb.TxValidationCode_BAD_COMMON_HEADER
	}

	// validate the signature in the envelope
	err = checkSignatureFromCreator(shdr.Creator, e.Signature, e.Payload, chdr.ChannelId)
	if err != nil {
		putilsLogger.Errorf("checkSignatureFromCreator returns err %s", err)
		return nil, pb.TxValidationCode_BAD_CREATOR_SIGNATURE
	}

	// TODO: ensure that creator can transact with us (some ACLs?) which set of APIs is supposed to give us this info?

	// continue the validation in a way that depends on the type specified in the header
	switch common.HeaderType(chdr.Type) {
	case common.HeaderType_ENDORSER_TRANSACTION:
		// Verify that the transaction ID has been computed properly.
		// This check is needed to ensure that the lookup into the ledger
		// for the same TxID catches duplicates.
		err = utils.CheckTxID(
			chdr.TxId,
			shdr.Nonce,
			shdr.Creator)

		if err != nil {
			putilsLogger.Errorf("CheckTxID returns err %s", err)
			return nil, pb.TxValidationCode_BAD_PROPOSAL_TXID
		}

		err = validateEndorserTransaction(payload.Data, payload.Header)
		putilsLogger.Debugf("ValidateTransactionEnvelope returns err %s", err)

		if err != nil {
			putilsLogger.Errorf("validateEndorserTransaction returns err %s", err)
			return payload, pb.TxValidationCode_INVALID_ENDORSER_TRANSACTION
		} else {
			return payload, pb.TxValidationCode_VALID
		}
	case common.HeaderType_CONFIG:
		// Config transactions have signatures inside which will be validated, especially at genesis there may be no creator or
		// signature on the outermost envelope

		err = validateConfigTransaction(payload.Data, payload.Header)

		if err != nil {
			putilsLogger.Errorf("validateConfigTransaction returns err %s", err)
			return payload, pb.TxValidationCode_INVALID_CONFIG_TRANSACTION
		} else {
			return payload, pb.TxValidationCode_VALID
		}
	case common.HeaderType_TOKEN_TRANSACTION:
		// Verify that the transaction ID has been computed properly.
		// This check is needed to ensure that the lookup into the ledger
		// for the same TxID catches duplicates.
		err = utils.CheckTxID(
			chdr.TxId,
			shdr.Nonce,
			shdr.Creator)

		if err != nil {
			putilsLogger.Errorf("CheckTxID returns err %s", err)
			return nil, pb.TxValidationCode_BAD_PROPOSAL_TXID
		}

		return payload, pb.TxValidationCode_VALID
	default:
		return nil, pb.TxValidationCode_UNSUPPORTED_TX_PAYLOAD
	}
}
