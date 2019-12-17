/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package peerext

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset"
	"github.com/hyperledger/fabric-protos-go/peer"
)

type ChaincodeProposalPayload struct {
	*peer.ChaincodeProposalPayload
}

func (cpp *ChaincodeProposalPayload) Underlying() proto.Message {
	return cpp.ChaincodeProposalPayload
}

func (cpp *ChaincodeProposalPayload) StaticallyOpaqueFields() []string {
	return []string{"input"}
}

func (cpp *ChaincodeProposalPayload) StaticallyOpaqueFieldProto(name string) (proto.Message, error) {
	if name != cpp.StaticallyOpaqueFields()[0] {
		return nil, fmt.Errorf("not a marshaled field: %s", name)
	}
	return &peer.ChaincodeInvocationSpec{}, nil
}

type ChaincodeAction struct {
	*peer.ChaincodeAction
}

func (ca *ChaincodeAction) Underlying() proto.Message {
	return ca.ChaincodeAction
}

func (ca *ChaincodeAction) StaticallyOpaqueFields() []string {
	return []string{"results", "events"}
}

func (ca *ChaincodeAction) StaticallyOpaqueFieldProto(name string) (proto.Message, error) {
	switch name {
	case "results":
		return &rwset.TxReadWriteSet{}, nil
	case "events":
		return &peer.ChaincodeEvent{}, nil
	default:
		return nil, fmt.Errorf("not a marshaled field: %s", name)
	}
}
