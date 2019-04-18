/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orderer

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/msp"
)

func init() {
	common.ChannelGroupMap["Orderer"] = DynamicOrdererGroupFactory{}
}

type DynamicOrdererGroupFactory struct{}

func (dogf DynamicOrdererGroupFactory) DynamicConfigGroup(cg *common.ConfigGroup) proto.Message {
	return &DynamicOrdererGroup{
		ConfigGroup: cg,
	}
}

type DynamicOrdererGroup struct {
	*common.ConfigGroup
}

func (dcg *DynamicOrdererGroup) DynamicMapFieldProto(name string, key string, base proto.Message) (proto.Message, error) {
	switch name {
	case "groups":
		cg, ok := base.(*common.ConfigGroup)
		if !ok {
			return nil, fmt.Errorf("ConfigGroup groups can only contain ConfigGroup messages")
		}

		return &DynamicOrdererOrgGroup{
			ConfigGroup: cg,
		}, nil
	case "values":
		cv, ok := base.(*common.ConfigValue)
		if !ok {
			return nil, fmt.Errorf("ConfigGroup values can only contain ConfigValue messages")
		}
		return &DynamicOrdererConfigValue{
			ConfigValue: cv,
			name:        key,
		}, nil
	default:
		return nil, fmt.Errorf("ConfigGroup does not have a dynamic field: %s", name)
	}
}

type ConsensusTypeMetadataFactory interface {
	NewMessage() proto.Message
}

// ConsensuTypeMetadataMap should have consensus implementations register their metadata message factories
var ConsensusTypeMetadataMap = map[string]ConsensusTypeMetadataFactory{}

func (ct *ConsensusType) VariablyOpaqueFields() []string {
	return []string{"metadata"}
}

func (ct *ConsensusType) VariablyOpaqueFieldProto(name string) (proto.Message, error) {
	if name != "metadata" {
		return nil, fmt.Errorf("not a valid opaque field: %s", name)
	}
	ctmf, ok := ConsensusTypeMetadataMap[ct.Type]
	if ok {
		return ctmf.NewMessage(), nil
	}
	return &empty.Empty{}, nil

}

type DynamicOrdererOrgGroup struct {
	*common.ConfigGroup
}

func (dcg *DynamicOrdererOrgGroup) DynamicMapFieldProto(name string, key string, base proto.Message) (proto.Message, error) {
	switch name {
	case "groups":
		return nil, fmt.Errorf("the orderer orgs do not support sub-groups")
	case "values":
		cv, ok := base.(*common.ConfigValue)
		if !ok {
			return nil, fmt.Errorf("ConfigGroup values can only contain ConfigValue messages")
		}

		return &DynamicOrdererOrgConfigValue{
			ConfigValue: cv,
			name:        key,
		}, nil
	default:
		return nil, fmt.Errorf("not a dynamic orderer map field: %s", name)
	}
}

type DynamicOrdererConfigValue struct {
	*common.ConfigValue
	name string
}

func (docv *DynamicOrdererConfigValue) VariablyOpaqueFieldProto(name string) (proto.Message, error) {
	if name != docv.VariablyOpaqueFields()[0] {
		return nil, fmt.Errorf("not a marshaled field: %s", name)
	}
	switch docv.name {
	case "ConsensusType":
		return &ConsensusType{}, nil
	case "BatchSize":
		return &BatchSize{}, nil
	case "BatchTimeout":
		return &BatchTimeout{}, nil
	case "KafkaBrokers":
		return &KafkaBrokers{}, nil
	case "ChannelRestrictions":
		return &ChannelRestrictions{}, nil
	case "Capabilities":
		return &common.Capabilities{}, nil
	default:
		return nil, fmt.Errorf("unknown Orderer ConfigValue name: %s", docv.name)
	}
}

type DynamicOrdererOrgConfigValue struct {
	*common.ConfigValue
	name string
}

func (doocv *DynamicOrdererOrgConfigValue) VariablyOpaqueFieldProto(name string) (proto.Message, error) {
	if name != doocv.VariablyOpaqueFields()[0] {
		return nil, fmt.Errorf("not a marshaled field: %s", name)
	}
	switch doocv.name {
	case "MSP":
		return &msp.MSPConfig{}, nil
	case "Endpoints":
		return &common.OrdererAddresses{}, nil
	default:
		return nil, fmt.Errorf("unknown Orderer Org ConfigValue name: %s", doocv.name)
	}
}
