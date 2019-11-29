/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package peerext

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/peer"
)

type TransactionAction struct { // nothing was testing this
	*peer.TransactionAction
}

func (ta *TransactionAction) Underlying() proto.Message {
	return ta.TransactionAction
}

func (ta *TransactionAction) StaticallyOpaqueFields() []string {
	return []string{"header", "payload"}
}

func (ta *TransactionAction) StaticallyOpaqueFieldProto(name string) (proto.Message, error) {
	switch name {
	case ta.StaticallyOpaqueFields()[0]:
		return &common.SignatureHeader{}, nil
	case ta.StaticallyOpaqueFields()[1]:
		return &peer.ChaincodeActionPayload{}, nil
	default:
		return nil, fmt.Errorf("not a marshaled field: %s", name)
	}
}

type ChaincodeActionPayload struct {
	*peer.ChaincodeActionPayload
}

func (cap *ChaincodeActionPayload) Underlying() proto.Message {
	return cap.ChaincodeActionPayload
}

func (cap *ChaincodeActionPayload) StaticallyOpaqueFields() []string {
	return []string{"chaincode_proposal_payload"}
}

func (cap *ChaincodeActionPayload) StaticallyOpaqueFieldProto(name string) (proto.Message, error) {
	if name != cap.StaticallyOpaqueFields()[0] {
		return nil, fmt.Errorf("not a marshaled field: %s", name)
	}
	return &peer.ChaincodeProposalPayload{}, nil
}

type ChaincodeEndorsedAction struct {
	*peer.ChaincodeEndorsedAction
}

func (cae *ChaincodeEndorsedAction) Underlying() proto.Message {
	return cae.ChaincodeEndorsedAction
}

func (cae *ChaincodeEndorsedAction) StaticallyOpaqueFields() []string {
	return []string{"proposal_response_payload"}
}

func (cae *ChaincodeEndorsedAction) StaticallyOpaqueFieldProto(name string) (proto.Message, error) {
	if name != cae.StaticallyOpaqueFields()[0] {
		return nil, fmt.Errorf("not a marshaled field: %s", name)
	}
	return &peer.ProposalResponsePayload{}, nil
}
