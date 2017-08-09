/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package peer

import (
	"fmt"

	"github.com/hyperledger/fabric/protos/common"

	"github.com/golang/protobuf/proto"
)

const RSCCSeedDataKey = "rscc_seed_data"

func init() {
	common.DynamicConfigTypes[common.ConfigType_RESOURCE] = func(cg *common.ConfigGroup) proto.Message {
		return &DynamicResourceConfigGroup{
			ConfigGroup: cg,
		}
	}

	common.ConfigUpdateIsolatedDataTypes[RSCCSeedDataKey] = func(key string) proto.Message {
		return &common.Config{}
	}
}

type DynamicResourceConfigGroup struct {
	*common.ConfigGroup
}

func (drcg *DynamicResourceConfigGroup) DynamicMapFields() []string {
	return []string{"values"}
}

func (drcg *DynamicResourceConfigGroup) DynamicMapFieldProto(name string, key string, base proto.Message) (proto.Message, error) {
	switch name {
	case "values":
		cv, ok := base.(*common.ConfigValue)
		if !ok {
			return nil, fmt.Errorf("ConfigGroup values can only contain ConfigValue messages")
		}
		return &DynamicResourceConfigValue{
			ConfigValue: cv,
		}, nil
	default:
		return nil, fmt.Errorf("ConfigGroup does not have a dynamic field: %s", name)
	}
}

type DynamicResourceConfigValue struct {
	*common.ConfigValue
}

func (drcv *DynamicResourceConfigValue) VariablyOpaqueFieldProto(name string) (proto.Message, error) {
	if name != drcv.VariablyOpaqueFields()[0] {
		return nil, fmt.Errorf("Not a marshaled field: %s", name)
	}

	return &Resource{}, nil
}
