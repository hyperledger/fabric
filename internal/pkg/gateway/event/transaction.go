/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package event

import (
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/peer"
)

type Transaction struct {
	parent          *Block
	payload         *common.Payload
	id              string
	timestamp       *timestamp.Timestamp
	status          peer.TxValidationCode
	chaincodeEvents []*ChaincodeEvent
}

func (tx *Transaction) Block() *Block {
	return tx.parent
}

func (tx *Transaction) ID() string {
	return tx.id
}

func (tx *Transaction) Timestamp() *timestamp.Timestamp {
	return tx.timestamp
}

func (tx *Transaction) Status() peer.TxValidationCode {
	return tx.status
}

func (tx *Transaction) Valid() bool {
	return tx.status == peer.TxValidationCode_VALID
}

func (tx *Transaction) ChaincodeEvents() ([]*ChaincodeEvent, error) {
	var err error

	if tx.chaincodeEvents == nil {
		tx.chaincodeEvents, err = tx.readChaincodeEvents()
	}

	return tx.chaincodeEvents, err
}

func (tx *Transaction) readChaincodeEvents() ([]*ChaincodeEvent, error) {
	transaction := &peer.Transaction{}
	if err := proto.Unmarshal(tx.payload.GetData(), transaction); err != nil {
		return nil, err
	}

	chaincodeEvents := make([]*ChaincodeEvent, 0)

	for _, action := range transaction.GetActions() {
		actionPayload := &peer.ChaincodeActionPayload{}
		if err := proto.Unmarshal(action.GetPayload(), actionPayload); err != nil {
			continue
		}

		responsePayload := &peer.ProposalResponsePayload{}
		if err := proto.Unmarshal(actionPayload.GetAction().GetProposalResponsePayload(), responsePayload); err != nil {
			continue
		}

		action := &peer.ChaincodeAction{}
		if err := proto.Unmarshal(responsePayload.GetExtension(), action); err != nil {
			continue
		}

		event := &peer.ChaincodeEvent{}
		if err := proto.Unmarshal(action.GetEvents(), event); err != nil {
			continue
		}

		if !validChaincodeEvent(event) {
			continue
		}

		chaincodeEvent := &ChaincodeEvent{
			parent:  tx,
			message: event,
		}
		chaincodeEvents = append(chaincodeEvents, chaincodeEvent)
	}

	return chaincodeEvents, nil
}

func validChaincodeEvent(event *peer.ChaincodeEvent) bool {
	return len(event.GetChaincodeId()) > 0 && len(event.GetEventName()) > 0 && len(event.GetTxId()) > 0
}
