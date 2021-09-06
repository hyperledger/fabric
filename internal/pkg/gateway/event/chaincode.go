/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package event

import (
	"github.com/hyperledger/fabric-protos-go/peer"
)

type ChaincodeEvent struct {
	parent  *Transaction
	message *peer.ChaincodeEvent
}

func (event *ChaincodeEvent) Transaction() *Transaction {
	return event.parent
}

func (event *ChaincodeEvent) ChaincodeID() string {
	return event.message.ChaincodeId
}

func (event *ChaincodeEvent) EventName() string {
	return event.message.EventName
}

func (event *ChaincodeEvent) Payload() []byte {
	return event.message.Payload
}

func (event *ChaincodeEvent) ProtoMessage() *peer.ChaincodeEvent {
	return event.message
}
