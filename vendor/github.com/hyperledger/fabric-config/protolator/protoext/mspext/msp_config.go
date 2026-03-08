/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mspext

import (
	"fmt"

	"github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"google.golang.org/protobuf/proto"
)

type MSPConfig struct{ *msp.MSPConfig }

func (mc *MSPConfig) Underlying() proto.Message {
	return mc.MSPConfig
}

func (mc *MSPConfig) VariablyOpaqueFields() []string {
	return []string{"config"}
}

func (mc *MSPConfig) VariablyOpaqueFieldProto(name string) (proto.Message, error) {
	if name != mc.VariablyOpaqueFields()[0] {
		return nil, fmt.Errorf("not a marshaled field: %s", name)
	}
	switch mc.Type {
	case 0:
		return &msp.FabricMSPConfig{}, nil
	case 1:
		return &msp.IdemixMSPConfig{}, nil
	default:
		return nil, fmt.Errorf("unable to decode MSP type: %v", mc.Type)
	}
}
