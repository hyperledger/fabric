/*
Copyright IBM Corp. 2017 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

                 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package common

import (
	"fmt"

	"github.com/hyperledger/fabric/protos/msp"

	"github.com/golang/protobuf/proto"
)

type DynamicConfigGroupFactory interface {
	DynamicConfigGroup(cg *ConfigGroup) proto.Message
}

// ChannelGroupMap is a slightly hacky way to break the dependency cycle which would
// be created if the protos/common package imported the protos/orderer or protos/peer
// packages.  These packages instead register a DynamicConfigGroupFactory with the
// ChannelGroupMap when they are loaded, creating a runtime linkage between the two
var ChannelGroupMap = map[string]DynamicConfigGroupFactory{
	"Consortiums": DynamicConsortiumsGroupFactory{},
}

type DynamicChannelGroup struct {
	*ConfigGroup
}

func (dcg *DynamicChannelGroup) DynamicMapFieldProto(name string, key string, base proto.Message) (proto.Message, error) {
	switch name {
	case "groups":
		cg, ok := base.(*ConfigGroup)
		if !ok {
			return nil, fmt.Errorf("ConfigGroup groups can only contain ConfigGroup messages")
		}

		dcgf, ok := ChannelGroupMap[key]
		if !ok {
			return nil, fmt.Errorf("unknown channel ConfigGroup sub-group: %s", key)
		}
		return dcgf.DynamicConfigGroup(cg), nil
	case "values":
		cv, ok := base.(*ConfigValue)
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
	*ConfigValue
	name string
}

func (dccv *DynamicChannelConfigValue) VariablyOpaqueFieldProto(name string) (proto.Message, error) {
	if name != dccv.VariablyOpaqueFields()[0] {
		return nil, fmt.Errorf("not a marshaled field: %s", name)
	}
	switch dccv.name {
	case "HashingAlgorithm":
		return &HashingAlgorithm{}, nil
	case "BlockDataHashingStructure":
		return &BlockDataHashingStructure{}, nil
	case "OrdererAddresses":
		return &OrdererAddresses{}, nil
	case "Consortium":
		return &Consortium{}, nil
	default:
		return nil, fmt.Errorf("unknown Channel ConfigValue name: %s", dccv.name)
	}
}

func (dccv *DynamicChannelConfigValue) Underlying() proto.Message {
	return dccv.ConfigValue
}

type DynamicConsortiumsGroupFactory struct{}

func (dogf DynamicConsortiumsGroupFactory) DynamicConfigGroup(cg *ConfigGroup) proto.Message {
	return &DynamicConsortiumsGroup{
		ConfigGroup: cg,
	}
}

type DynamicConsortiumsGroup struct {
	*ConfigGroup
}

func (dcg *DynamicConsortiumsGroup) DynamicMapFieldProto(name string, key string, base proto.Message) (proto.Message, error) {
	switch name {
	case "groups":
		cg, ok := base.(*ConfigGroup)
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

func (dcg *DynamicConsortiumsGroup) Underlying() proto.Message {
	return dcg.ConfigGroup
}

type DynamicConsortiumGroup struct {
	*ConfigGroup
}

func (dcg *DynamicConsortiumGroup) DynamicMapFieldProto(name string, key string, base proto.Message) (proto.Message, error) {
	switch name {
	case "groups":
		cg, ok := base.(*ConfigGroup)
		if !ok {
			return nil, fmt.Errorf("ConfigGroup groups can only contain ConfigGroup messages")
		}
		return &DynamicConsortiumOrgGroup{
			ConfigGroup: cg,
		}, nil
	case "values":
		cv, ok := base.(*ConfigValue)
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

func (dcg *DynamicConsortiumGroup) Underlying() proto.Message {
	return dcg.ConfigGroup
}

type DynamicConsortiumConfigValue struct {
	*ConfigValue
	name string
}

func (dccv *DynamicConsortiumConfigValue) VariablyOpaqueFieldProto(name string) (proto.Message, error) {
	if name != dccv.VariablyOpaqueFields()[0] {
		return nil, fmt.Errorf("not a marshaled field: %s", name)
	}
	switch dccv.name {
	case "ChannelCreationPolicy":
		return &Policy{}, nil
	default:
		return nil, fmt.Errorf("unknown Consortium ConfigValue name: %s", dccv.name)
	}
}

type DynamicConsortiumOrgGroup struct {
	*ConfigGroup
}

func (dcg *DynamicConsortiumOrgGroup) DynamicMapFieldProto(name string, key string, base proto.Message) (proto.Message, error) {
	switch name {
	case "groups":
		return nil, fmt.Errorf("ConsortiumOrg groups do not support sub groups")
	case "values":
		cv, ok := base.(*ConfigValue)
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
	*ConfigValue
	name string
}

func (dcocv *DynamicConsortiumOrgConfigValue) VariablyOpaqueFieldProto(name string) (proto.Message, error) {
	if name != dcocv.VariablyOpaqueFields()[0] {
		return nil, fmt.Errorf("not a marshaled field: %s", name)
	}
	switch dcocv.name {
	case "MSP":
		return &msp.MSPConfig{}, nil
	default:
		return nil, fmt.Errorf("unknown Consortium Org ConfigValue name: %s", dcocv.name)
	}
}
