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

package peer

import (
	"fmt"

	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/msp"

	"github.com/golang/protobuf/proto"
)

func init() {
	common.ChannelGroupMap["Application"] = DynamicApplicationGroupFactory{}
}

type DynamicApplicationGroupFactory struct{}

func (dagf DynamicApplicationGroupFactory) DynamicConfigGroup(cg *common.ConfigGroup) proto.Message {
	return &DynamicApplicationGroup{
		ConfigGroup: cg,
	}
}

type DynamicApplicationGroup struct {
	*common.ConfigGroup
}

func (dag *DynamicApplicationGroup) DynamicMapFieldProto(name string, key string, base proto.Message) (proto.Message, error) {
	switch name {
	case "groups":
		cg, ok := base.(*common.ConfigGroup)
		if !ok {
			return nil, fmt.Errorf("ConfigGroup groups can only contain ConfigGroup messages")
		}

		return &DynamicApplicationOrgGroup{
			ConfigGroup: cg,
		}, nil
	case "values":
		cv, ok := base.(*common.ConfigValue)
		if !ok {
			return nil, fmt.Errorf("ConfigGroup values can only contain ConfigValue messages")
		}
		return &DynamicApplicationConfigValue{
			ConfigValue: cv,
			name:        key,
		}, nil
	default:
		return nil, fmt.Errorf("ConfigGroup does not have a dynamic field: %s", name)
	}
}

type DynamicApplicationOrgGroup struct {
	*common.ConfigGroup
}

func (dag *DynamicApplicationOrgGroup) DynamicMapFieldProto(name string, key string, base proto.Message) (proto.Message, error) {
	switch name {
	case "groups":
		return nil, fmt.Errorf("The application orgs do not support sub-groups")
	case "values":
		cv, ok := base.(*common.ConfigValue)
		if !ok {
			return nil, fmt.Errorf("ConfigGroup values can only contain ConfigValue messages")
		}

		return &DynamicApplicationOrgConfigValue{
			ConfigValue: cv,
			name:        key,
		}, nil
	default:
		return nil, fmt.Errorf("Not a dynamic application map field: %s", name)
	}
}

type DynamicApplicationConfigValue struct {
	*common.ConfigValue
	name string
}

func (ccv *DynamicApplicationConfigValue) VariablyOpaqueFieldProto(name string) (proto.Message, error) {
	if name != ccv.VariablyOpaqueFields()[0] {
		return nil, fmt.Errorf("Not a marshaled field: %s", name)
	}
	switch ccv.name {
	case "Capabilities":
		return &common.Capabilities{}, nil
	case "ACLs":
		return &ACLs{}, nil
	default:
		return nil, fmt.Errorf("Unknown Application ConfigValue name: %s", ccv.name)
	}
}

type DynamicApplicationOrgConfigValue struct {
	*common.ConfigValue
	name string
}

func (daocv *DynamicApplicationOrgConfigValue) VariablyOpaqueFieldProto(name string) (proto.Message, error) {
	if name != daocv.VariablyOpaqueFields()[0] {
		return nil, fmt.Errorf("Not a marshaled field: %s", name)
	}
	switch daocv.name {
	case "MSP":
		return &msp.MSPConfig{}, nil
	case "AnchorPeers":
		return &AnchorPeers{}, nil
	default:
		return nil, fmt.Errorf("Unknown Application Org ConfigValue name: %s", daocv.name)
	}
}
