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

package orderer

import (
	"fmt"

	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/msp"

	"github.com/golang/protobuf/proto"
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
	default:
		return nil, fmt.Errorf("unknown Orderer Org ConfigValue name: %s", doocv.name)
	}
}
