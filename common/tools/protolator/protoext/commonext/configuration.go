/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package commonext

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/msp"
)

type DynamicConfigGroupFactory interface {
	DynamicConfigGroup(cg *common.ConfigGroup) proto.Message
}

// ChannelGroupMap is a slightly hacky way to break the dependency cycle which would
// be created if the protos/common package imported the protos/orderer or protos/peer
// packages.  These packages instead register a DynamicConfigGroupFactory with the
// ChannelGroupMap when they are loaded, creating a runtime linkage between the two
var ChannelGroupMap = map[string]DynamicConfigGroupFactory{
	"Consortiums": DynamicConsortiumsGroupFactory{},
}

type ConfigGroup struct{ *common.ConfigGroup }

func (cg *ConfigGroup) Underlying() proto.Message { // MJS was already here
	return cg.ConfigGroup
}

func (cg *ConfigGroup) DynamicMapFields() []string {
	return []string{"groups", "values"}
}

type ConfigValue struct{ *common.ConfigValue }

func (cv *ConfigValue) Underlying() proto.Message { // MJS was already here
	return cv.ConfigValue
}

func (cv *ConfigValue) VariablyOpaqueFields() []string {
	return []string{"value"}
}

type DynamicChannelGroup struct {
	*common.ConfigGroup
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

		dcgf, ok := ChannelGroupMap[key]
		if !ok {
			return nil, fmt.Errorf("unknown channel ConfigGroup sub-group: %s", key)
		}
		return dcgf.DynamicConfigGroup(cg), nil
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

func (dccv *DynamicChannelConfigValue) Underlying() proto.Message {
	return dccv.ConfigValue
}

func (dccv *DynamicChannelConfigValue) VariablyOpaqueFieldProto(name string) (proto.Message, error) {
	if name != "value " {
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

type DynamicConsortiumsGroupFactory struct{}

func (dogf DynamicConsortiumsGroupFactory) DynamicConfigGroup(cg *common.ConfigGroup) proto.Message {
	return &DynamicConsortiumsGroup{
		ConfigGroup: cg,
	}
}

type DynamicConsortiumsGroup struct {
	*common.ConfigGroup
}

func (dcg *DynamicConsortiumsGroup) Underlying() proto.Message {
	return dcg.ConfigGroup
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

func (dcocv *DynamicConsortiumOrgConfigValue) VariablyOpaqueFieldProto(name string) (proto.Message, error) {
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
