/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorsertx

import (
	"regexp"

	"github.com/hyperledger/fabric-protos-go-apiv2/peer"

	"github.com/hyperledger/fabric/pkg/tx"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

var (
	// ChannelAllowedChars tracks the permitted characters for a channel name
	ChannelAllowedChars = "[a-z][a-z0-9.-]*"
	// MaxLength tracks the maximum length of a channel name
	MaxLength = 249
)

// EndorserTx represents a parsed common.Envelope protobuf
type EndorserTx struct {
	ComputedTxID string
	ChannelID    string
	ChaincodeID  *peer.ChaincodeID
	Creator      []byte
	Response     *peer.Response
	Events       []byte
	Results      []byte
	Endorsements []*peer.Endorsement
	Type         int32
	Version      int32
	Epoch        uint64
	Nonce        []byte
}

func unmarshalEndorserTx(txenv *tx.Envelope) (*EndorserTx, error) {
	if len(txenv.ChannelHeader.GetExtension()) == 0 {
		return nil, errors.New("empty header extension")
	}

	hdrExt, err := protoutil.UnmarshalChaincodeHeaderExtension(
		txenv.ChannelHeader.GetExtension(),
	)
	if err != nil {
		return nil, err
	}

	if len(txenv.Data) == 0 {
		return nil, errors.New("nil payload data")
	}

	tx, err := protoutil.UnmarshalTransaction(txenv.Data)
	if err != nil {
		return nil, err
	}

	if len(tx.GetActions()) != 1 {
		return nil, errors.Errorf("only one transaction action is supported, %d were present", len(tx.GetActions()))
	}

	txAction := tx.GetActions()[0]

	if txAction == nil {
		return nil, errors.New("nil action")
	}

	if len(txAction.GetPayload()) == 0 {
		return nil, errors.New("empty ChaincodeActionPayload")
	}

	ccActionPayload, err := protoutil.UnmarshalChaincodeActionPayload(txAction.GetPayload())
	if err != nil {
		return nil, err
	}

	if ccActionPayload.GetAction() == nil {
		return nil, errors.New("nil ChaincodeEndorsedAction")
	}

	if len(ccActionPayload.GetAction().GetProposalResponsePayload()) == 0 {
		return nil, errors.New("empty ProposalResponsePayload")
	}

	proposalResponsePayload, err := protoutil.UnmarshalProposalResponsePayload(
		ccActionPayload.GetAction().GetProposalResponsePayload(),
	)
	if err != nil {
		return nil, err
	}

	if len(proposalResponsePayload.GetExtension()) == 0 {
		return nil, errors.New("nil Extension")
	}

	ccAction, err := protoutil.UnmarshalChaincodeAction(proposalResponsePayload.GetExtension())
	if err != nil {
		return nil, err
	}

	computedTxID := protoutil.ComputeTxID(
		txenv.SignatureHeader.GetNonce(),
		txenv.SignatureHeader.GetCreator(),
	)

	return &EndorserTx{
		ComputedTxID: computedTxID,
		ChannelID:    txenv.ChannelHeader.GetChannelId(),
		Creator:      txenv.SignatureHeader.GetCreator(),
		Response:     ccAction.GetResponse(),
		Events:       ccAction.GetEvents(),
		Results:      ccAction.GetResults(),
		Endorsements: ccActionPayload.GetAction().GetEndorsements(),
		ChaincodeID:  hdrExt.GetChaincodeId(),
		Type:         txenv.ChannelHeader.GetType(),
		Version:      txenv.ChannelHeader.GetVersion(),
		Epoch:        txenv.ChannelHeader.GetEpoch(),
		Nonce:        txenv.SignatureHeader.GetNonce(),
	}, nil
}

func (e *EndorserTx) validate() error {
	if e.Epoch != 0 {
		return errors.Errorf("invalid epoch in ChannelHeader. Expected 0, got [%d]", e.Epoch)
	}

	if e.Version != 0 {
		return errors.Errorf("invalid version in ChannelHeader. Expected 0, got [%d]", e.Version)
	}

	if err := ValidateChannelID(e.ChannelID); err != nil {
		return err
	}

	if len(e.Nonce) == 0 {
		return errors.New("empty nonce")
	}

	if len(e.Creator) == 0 {
		return errors.New("empty creator")
	}

	if e.ChaincodeID == nil {
		return errors.New("nil ChaincodeId")
	}

	if e.ChaincodeID.GetName() == "" {
		return errors.New("empty chaincode name in chaincode id")
	}

	// TODO FAB-16170: check proposal hash

	// TODO FAB-16170: verify that txid matches the one in the header

	// TODO FAB-16170: check that header in the tx action and channel header match bitwise

	return nil
}

// UnmarshalEndorserTxAndValidate receives a tx.Envelope containing
// a partially unmarshalled endorser transaction and returns an EndorserTx
// instance (or an error)
func UnmarshalEndorserTxAndValidate(txenv *tx.Envelope) (*EndorserTx, error) {
	etx, err := unmarshalEndorserTx(txenv)
	if err != nil {
		return nil, err
	}

	err = etx.validate()
	if err != nil {
		return nil, err
	}

	return etx, nil
}

// ValidateChannelID makes sure that proposed channel IDs comply with the
// following restrictions:
//  1. Contain only lower case ASCII alphanumerics, dots '.', and dashes '-'
//  2. Are shorter than 250 characters.
//  3. Start with a letter
//
// This is the intersection of the Kafka restrictions and CouchDB restrictions
// with the following exception: '.' is converted to '_' in the CouchDB naming
// This is to accommodate existing channel names with '.', especially in the
// behave tests which rely on the dot notation for their sluggification.
//
// note: this function is a copy of the same in common/configtx/validator.go
func ValidateChannelID(channelID string) error {
	re, _ := regexp.Compile(ChannelAllowedChars)
	// Length
	if len(channelID) <= 0 {
		return errors.Errorf("channel ID illegal, cannot be empty")
	}
	if len(channelID) > MaxLength {
		return errors.Errorf("channel ID illegal, cannot be longer than %d", MaxLength)
	}

	// Illegal characters
	matched := re.FindString(channelID)
	if len(matched) != len(channelID) {
		return errors.Errorf("'%s' contains illegal characters", channelID)
	}

	return nil
}
