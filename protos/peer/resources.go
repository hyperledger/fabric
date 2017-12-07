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

const ResourceConfigSeedDataKey = "resource_config_seed_data"

func init() {
	common.DynamicConfigTypes[common.ConfigType_RESOURCE] = func(cg *common.ConfigGroup) proto.Message {
		return &DynamicResourceConfigGroup{
			ConfigGroup: cg,
		}
	}

	common.ConfigUpdateIsolatedDataTypes[ResourceConfigSeedDataKey] = func(key string) proto.Message {
		return &common.Config{}
	}
}

type DynamicResourceConfigGroup struct {
	*common.ConfigGroup
}

func (drcg *DynamicResourceConfigGroup) DynamicMapFields() []string {
	return []string{"values", "groups"}
}

func (drcg *DynamicResourceConfigGroup) DynamicMapFieldProto(name string, key string, base proto.Message) (proto.Message, error) {
	switch name {
	case "groups":
		cg, ok := base.(*common.ConfigGroup)
		if !ok {
			return nil, fmt.Errorf("ConfigGroup groups can only contain ConfigGroup messages")
		}
		switch key {
		case "APIs":
			return &DynamicResourceAPIsConfigGroup{ConfigGroup: cg}, nil
		case "Chaincodes":
			return &DynamicResourceChaincodesConfigGroup{ConfigGroup: cg}, nil
		case "PeerPolicies":
			return cg, nil
		}
	}
	return nil, fmt.Errorf("ConfigGroup does not have a dynamic field: %s", name)
}

type DynamicResourceChaincodesConfigGroup struct {
	*common.ConfigGroup
}

func (draccg *DynamicResourceChaincodesConfigGroup) DynamicMapFields() []string {
	return []string{"groups"}
}

func (drccg *DynamicResourceChaincodesConfigGroup) DynamicMapFieldProto(name string, key string, base proto.Message) (proto.Message, error) {
	switch name {
	case "groups":
		cg, ok := base.(*common.ConfigGroup)
		if !ok {
			return nil, fmt.Errorf("ConfigGroup groups can only contain ConfigGroup messages")
		}
		return &DynamicResourceChaincodeConfigGroup{ConfigGroup: cg}, nil
	default:
		return nil, fmt.Errorf("ConfigGroup does not have a dynamic field: %s", name)
	}
}

type DynamicResourceChaincodeConfigGroup struct {
	*common.ConfigGroup
}

func (draccg *DynamicResourceChaincodeConfigGroup) DynamicMapFields() []string {
	return []string{"values"}
}

func (drccg *DynamicResourceChaincodeConfigGroup) DynamicMapFieldProto(name string, key string, base proto.Message) (proto.Message, error) {
	switch name {
	case "values":
		cv, ok := base.(*common.ConfigValue)
		if !ok {
			return nil, fmt.Errorf("ConfigGroup groups can only contain ConfigValue messages")
		}
		return &DynamicResourceChaincodeConfigValue{ConfigValue: cv, key: key}, nil
	default:
		return nil, fmt.Errorf("ConfigGroup does not have a dynamic field: %s", name)
	}
}

type DynamicResourceChaincodeConfigValue struct {
	*common.ConfigValue
	key string
}

func (drccv *DynamicResourceChaincodeConfigValue) VariablyOpaqueFieldProto(name string) (proto.Message, error) {
	if name != drccv.VariablyOpaqueFields()[0] {
		return nil, fmt.Errorf("Not a marshaled field: %s", name)
	}

	switch drccv.key {
	case "ChaincodeIdentifier":
		return &ChaincodeIdentifier{}, nil
	case "ChaincodeValidation":
		return &ChaincodeValidation{}, nil
	case "ChaincodeEndorsement":
		return &ChaincodeEndorsement{}, nil
	default:
		return nil, fmt.Errorf("unknown value key: %s", drccv.key)
	}
}

func (cv *ChaincodeValidation) VariablyOpaqueFields() []string {
	return []string{"argument"}
}

func (cv *ChaincodeValidation) VariablyOpaqueFieldProto(name string) (proto.Message, error) {
	if name != cv.VariablyOpaqueFields()[0] {
		return nil, fmt.Errorf("not a marshaled field: %s", name)
	}
	switch cv.Name {
	case "vscc":
		return &VSCCArgs{}, nil
	default:
		return nil, fmt.Errorf("unknown validation name: %s", cv.Name)
	}
}

type DynamicResourceAPIsConfigGroup struct {
	*common.ConfigGroup
}

func (dracg *DynamicResourceAPIsConfigGroup) DynamicMapFields() []string {
	return []string{"values"}
}

func (dracg *DynamicResourceAPIsConfigGroup) DynamicMapFieldProto(name string, key string, base proto.Message) (proto.Message, error) {
	switch name {
	case "values":
		cv, ok := base.(*common.ConfigValue)
		if !ok {
			return nil, fmt.Errorf("ConfigGroup values can only contain ConfigValue messages")
		}
		return &DynamicResourceAPIsConfigValue{
			ConfigValue: cv,
		}, nil
	default:
		return nil, fmt.Errorf("ConfigGroup does not have a dynamic field: %s", name)
	}
}

type DynamicResourceAPIsConfigValue struct {
	*common.ConfigValue
}

func (drcv *DynamicResourceAPIsConfigValue) VariablyOpaqueFieldProto(name string) (proto.Message, error) {
	if name != drcv.VariablyOpaqueFields()[0] {
		return nil, fmt.Errorf("Not a marshaled field: %s", name)
	}

	return &APIResource{}, nil
}
