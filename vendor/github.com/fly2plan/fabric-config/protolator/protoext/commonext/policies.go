/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package commonext

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
)

type Policy struct{ *common.Policy }

func (p *Policy) Underlying() proto.Message {
	return p.Policy
}

func (p *Policy) VariablyOpaqueFields() []string {
	return []string{"value"}
}

func (p *Policy) VariablyOpaqueFieldProto(name string) (proto.Message, error) {
	if name != p.VariablyOpaqueFields()[0] {
		return nil, fmt.Errorf("not a marshaled field: %s", name)
	}
	switch p.Type {
	case int32(common.Policy_SIGNATURE):
		return &common.SignaturePolicyEnvelope{}, nil
	case int32(common.Policy_IMPLICIT_META):
		return &common.ImplicitMetaPolicy{}, nil
	default:
		return nil, fmt.Errorf("unable to decode policy type: %v", p.Type)
	}
}
