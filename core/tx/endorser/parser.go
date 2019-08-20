/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorsertx

import (
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/pkg/tx"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

// EndorserTx represents a parsed common.Envelope protobuf
type EndorserTx struct {
	ComputedTxID string
	ChID         string
	CcID         string
	Creator      []byte
	Response     *peer.Response
	Events       []byte
	Results      []byte
	Endorsements []*peer.Endorsement
}

// NewEndorserTx receives a tx.Envelope containing a partially
// unmarshalled endorser transaction and returns an EndorserTx
// instance (or an error)
func NewEndorserTx(txenv *tx.Envelope) (*EndorserTx, error) {

	// TODO FAB-16170: validate headers

	if len(txenv.ChannelHeader.Extension) == 0 {
		return nil, errors.New("empty header extension")
	}

	hdrExt, err := protoutil.UnmarshalChaincodeHeaderExtension(
		txenv.ChannelHeader.Extension,
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

	// TODO FAB-16170: check that header in the tx action and channel header match bitwise

	if len(txAction.Payload) == 0 {
		return nil, errors.New("empty ChaincodeActionPayload")
	}

	ccActionPayload, err := protoutil.UnmarshalChaincodeActionPayload(txAction.Payload)
	if err != nil {
		return nil, err
	}

	if ccActionPayload.Action == nil {
		return nil, errors.New("nil ChaincodeEndorsedAction")
	}

	if len(ccActionPayload.Action.ProposalResponsePayload) == 0 {
		return nil, errors.New("empty ProposalResponsePayload")
	}

	proposalResponsePayload, err := protoutil.UnmarshalProposalResponsePayload(ccActionPayload.Action.ProposalResponsePayload)
	if err != nil {
		return nil, err
	}

	if len(proposalResponsePayload.Extension) == 0 {
		return nil, errors.New("nil Extension")
	}

	ccAction, err := protoutil.UnmarshalChaincodeAction(proposalResponsePayload.Extension)
	if err != nil {
		return nil, err
	}

	// TODO FAB-16170: check proposal hash

	computedTxID := protoutil.ComputeTxID(
		txenv.SignatureHeader.Nonce,
		txenv.SignatureHeader.Creator,
	)

	// TODO FAB-16170: verify that txid matches the one in the header

	return &EndorserTx{
		ComputedTxID: computedTxID,
		ChID:         txenv.ChannelHeader.ChannelId,
		Creator:      txenv.SignatureHeader.Creator,
		Response:     ccAction.Response,
		Events:       ccAction.Events,
		Results:      ccAction.Results,
		Endorsements: ccActionPayload.Action.Endorsements,
		CcID:         hdrExt.ChaincodeId.Name, // FIXME: we might have to get the ccid from the CIS
	}, nil
}
