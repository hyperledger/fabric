/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package peer

import (
	"fmt"

	"github.com/golang/protobuf/proto"
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
