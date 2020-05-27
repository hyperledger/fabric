/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package commonext

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-config/protolator/protoext/ordererext"
	"github.com/hyperledger/fabric-config/protolator/protoext/peerext"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/msp"
)

type DynamicChannelGroup struct {
	*common.ConfigGroup
}

func (dcg *DynamicChannelGroup) DynamicMapFields() []string {
	return []string{"values", "groups"}
}

func (dcg *DynamicChannelGroup) Underlying() proto.Message {
	return dcg.ConfigGroup
}

func (dcg *DynamicChannelGroup) DynamicMapFieldProto(name string, key string, base proto.Message) (proto.Message, error) {
	switch name {
	case "groups":
		cg, ok := base.(*common.ConfigGroup)
		if !ok {
			return nil, fmt.Errorf("ConfigGroup groups can only contain ConfigGroup messages")
		}

		switch key {
		case "Consortiums":
			return &DynamicConsortiumsGroup{ConfigGroup: cg}, nil
		case "Orderer":
			return &ordererext.DynamicOrdererGroup{ConfigGroup: cg}, nil
		case "Application":
			return &peerext.DynamicApplicationGroup{ConfigGroup: cg}, nil
		default:
			return nil, fmt.Errorf("unknown channel group sub-group '%s'", key)
		}
	case "values":
		cv, ok := base.(*common.ConfigValue)
		if !ok {
			return nil, fmt.Errorf("ConfigGroup values can only contain ConfigValue messages")
		}
		return &DynamicChannelConfigValue{
			ConfigValue: cv,
			name:        key,
		}, nil
	default:
		return nil, fmt.Errorf("ConfigGroup does not have a dynamic field: %s", name)
	}
}

type DynamicChannelConfigValue struct {
	*common.ConfigValue
	name string
}

func (dccv *DynamicChannelConfigValue) StaticallyOpaqueFields() []string {
	return []string{"value"}
}

func (dccv *DynamicChannelConfigValue) Underlying() proto.Message {
	return dccv.ConfigValue
}

func (dccv *DynamicChannelConfigValue) StaticallyOpaqueFieldProto(name string) (proto.Message, error) {
	if name != "value" {
		return nil, fmt.Errorf("not a marshaled field: %s", name)
	}
	switch dccv.name {
	case "HashingAlgorithm":
		return &common.HashingAlgorithm{}, nil
	case "BlockDataHashingStructure":
		return &common.BlockDataHashingStructure{}, nil
	case "OrdererAddresses":
		return &common.OrdererAddresses{}, nil
	case "Consortium":
		return &common.Consortium{}, nil
	case "Capabilities":
		return &common.Capabilities{}, nil
	default:
		return nil, fmt.Errorf("unknown Channel ConfigValue name: %s", dccv.name)
	}
}

type DynamicConsortiumsGroup struct {
	*common.ConfigGroup
}

func (dcg *DynamicConsortiumsGroup) Underlying() proto.Message {
	return dcg.ConfigGroup
}

func (dcg *DynamicConsortiumsGroup) DynamicMapFields() []string {
	return []string{"values", "groups"}
}

func (dcg *DynamicConsortiumsGroup) DynamicMapFieldProto(name string, key string, base proto.Message) (proto.Message, error) {
	switch name {
	case "groups":
		cg, ok := base.(*common.ConfigGroup)
		if !ok {
			return nil, fmt.Errorf("ConfigGroup groups can only contain ConfigGroup messages")
		}

		return &DynamicConsortiumGroup{
			ConfigGroup: cg,
		}, nil
	case "values":
		return nil, fmt.Errorf("Consortiums currently support no config values")
	default:
		return nil, fmt.Errorf("ConfigGroup does not have a dynamic field: %s", name)
	}
}

type DynamicConsortiumGroup struct {
	*common.ConfigGroup
}

func (dcg *DynamicConsortiumGroup) Underlying() proto.Message {
	return dcg.ConfigGroup
}

func (dcg *DynamicConsortiumGroup) DynamicMapFields() []string {
	return []string{"values", "groups"}
}

func (dcg *DynamicConsortiumGroup) DynamicMapFieldProto(name string, key string, base proto.Message) (proto.Message, error) {
	switch name {
	case "groups":
		cg, ok := base.(*common.ConfigGroup)
		if !ok {
			return nil, fmt.Errorf("ConfigGroup groups can only contain ConfigGroup messages")
		}
		return &DynamicConsortiumOrgGroup{
			ConfigGroup: cg,
		}, nil
	case "values":
		cv, ok := base.(*common.ConfigValue)
		if !ok {
			return nil, fmt.Errorf("ConfigGroup values can only contain ConfigValue messages")
		}

		return &DynamicConsortiumConfigValue{
			ConfigValue: cv,
			name:        key,
		}, nil
	default:
		return nil, fmt.Errorf("not a dynamic orderer map field: %s", name)
	}
}

type DynamicConsortiumConfigValue struct {
	*common.ConfigValue
	name string
}

func (dccv *DynamicConsortiumConfigValue) Underlying() proto.Message {
	return dccv.ConfigValue
}

func (dccv *DynamicConsortiumConfigValue) VariablyOpaqueFields() []string {
	return []string{"value"}
}

func (dccv *DynamicConsortiumConfigValue) VariablyOpaqueFieldProto(name string) (proto.Message, error) {
	if name != "value" {
		return nil, fmt.Errorf("not a marshaled field: %s", name)
	}
	switch dccv.name {
	case "ChannelCreationPolicy":
		return &common.Policy{}, nil
	default:
		return nil, fmt.Errorf("unknown Consortium ConfigValue name: %s", dccv.name)
	}
}

type DynamicConsortiumOrgGroup struct {
	*common.ConfigGroup
}

func (dcg *DynamicConsortiumOrgGroup) Underlying() proto.Message {
	return dcg.ConfigGroup
}

func (dcg *DynamicConsortiumOrgGroup) DynamicMapFields() []string {
	return []string{"groups", "values"}
}

func (dcg *DynamicConsortiumOrgGroup) DynamicMapFieldProto(name string, key string, base proto.Message) (proto.Message, error) {
	switch name {
	case "groups":
		return nil, fmt.Errorf("ConsortiumOrg groups do not support sub groups")
	case "values":
		cv, ok := base.(*common.ConfigValue)
		if !ok {
			return nil, fmt.Errorf("ConfigGroup values can only contain ConfigValue messages")
		}

		return &DynamicConsortiumOrgConfigValue{
			ConfigValue: cv,
			name:        key,
		}, nil
	default:
		return nil, fmt.Errorf("not a dynamic orderer map field: %s", name)
	}
}

type DynamicConsortiumOrgConfigValue struct {
	*common.ConfigValue
	name string
}

func (dcocv *DynamicConsortiumOrgConfigValue) Underlying() proto.Message {
	return dcocv.ConfigValue
}

func (dcocv *DynamicConsortiumOrgConfigValue) StaticallyOpaqueFields() []string {
	return []string{"value"}
}

func (dcocv *DynamicConsortiumOrgConfigValue) StaticallyOpaqueFieldProto(name string) (proto.Message, error) {
	if name != "value" {
		return nil, fmt.Errorf("not a marshaled field: %s", name)
	}
	switch dcocv.name {
	case "MSP":
		return &msp.MSPConfig{}, nil
	default:
		return nil, fmt.Errorf("unknown Consortium Org ConfigValue name: %s", dcocv.name)
	}
}
