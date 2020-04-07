/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package validation

import (
	"bytes"

	"github.com/hyperledger/fabric-protos-go/common"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/common/flogging"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

var putilsLogger = flogging.MustGetLogger("protoutils")

// given a creator, a message and a signature,
// this function returns nil if the creator
// is a valid cert and the signature is valid
func checkSignatureFromCreator(creatorBytes, sig, msg []byte, ChannelID string, cryptoProvider bccsp.BCCSP) error {
	putilsLogger.Debugf("begin")

	// check for nil argument
	if creatorBytes == nil || sig == nil || msg == nil {
		return errors.New("nil arguments")
	}

	mspObj := mspmgmt.GetIdentityDeserializer(ChannelID, cryptoProvider)
	if mspObj == nil {
		return errors.Errorf("could not get msp for channel [%s]", ChannelID)
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

	// TODO: validate channelID in cHdr.ChannelID

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
	tx, err := protoutil.UnmarshalTransaction(data)
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
		sHdr, err := protoutil.UnmarshalSignatureHeader(act.Header)
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
		ccActionPayload, err := protoutil.UnmarshalChaincodeActionPayload(act.Payload)
		if err != nil {
			return err
		}

		// extract the proposal response payload
		prp, err := protoutil.UnmarshalProposalResponsePayload(ccActionPayload.Action.ProposalResponsePayload)
		if err != nil {
			return err
		}

		// build the original header by stitching together
		// the common ChannelHeader and the per-action SignatureHeader
		hdrOrig := &common.Header{ChannelHeader: hdr.ChannelHeader, SignatureHeader: act.Header}

		// compute proposalHash
		pHash, err := protoutil.GetProposalHash2(hdrOrig, ccActionPayload.ChaincodeProposalPayload)
		if err != nil {
			return err
		}

		// ensure that the proposal hash matches
		if !bytes.Equal(pHash, prp.ProposalHash) {
			return errors.New("proposal hash does not match")
		}
	}

	return nil
}

// ValidateTransaction checks that the transaction envelope is properly formed
func ValidateTransaction(e *common.Envelope, cryptoProvider bccsp.BCCSP) (*common.Payload, pb.TxValidationCode) {
	putilsLogger.Debugf("ValidateTransactionEnvelope starts for envelope %p", e)

	// check for nil argument
	if e == nil {
		putilsLogger.Errorf("Error: nil envelope")
		return nil, pb.TxValidationCode_NIL_ENVELOPE
	}

	// get the payload from the envelope
	payload, err := protoutil.UnmarshalPayload(e.Payload)
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
	err = checkSignatureFromCreator(shdr.Creator, e.Signature, e.Payload, chdr.ChannelId, cryptoProvider)
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
		err = protoutil.CheckTxID(
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
		}
		return payload, pb.TxValidationCode_VALID
	case common.HeaderType_CONFIG:
		// Config transactions have signatures inside which will be validated, especially at genesis there may be no creator or
		// signature on the outermost envelope

		err = validateConfigTransaction(payload.Data, payload.Header)

		if err != nil {
			putilsLogger.Errorf("validateConfigTransaction returns err %s", err)
			return payload, pb.TxValidationCode_INVALID_CONFIG_TRANSACTION
		}
		return payload, pb.TxValidationCode_VALID
	default:
		return nil, pb.TxValidationCode_UNSUPPORTED_TX_PAYLOAD
	}
}
