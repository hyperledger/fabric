/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package peer

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/protos/ledger/rwset"
)

func (cpp *ChaincodeProposalPayload) StaticallyOpaqueFields() []string {
	return []string{"input"}
}

func (cpp *ChaincodeProposalPayload) StaticallyOpaqueFieldProto(name string) (proto.Message, error) {
	if name != cpp.StaticallyOpaqueFields()[0] {
		return nil, fmt.Errorf("not a marshaled field: %s", name)
	}
	return &ChaincodeInvocationSpec{}, nil
}

func (ca *ChaincodeAction) StaticallyOpaqueFields() []string {
	return []string{"results", "events"}
}

func (ca *ChaincodeAction) StaticallyOpaqueFieldProto(name string) (proto.Message, error) {
	switch name {
	case "results":
		return &rwset.TxReadWriteSet{}, nil
	case "events":
		return &ChaincodeEvent{}, nil
	default:
		return nil, fmt.Errorf("not a marshaled field: %s", name)
	}
}
