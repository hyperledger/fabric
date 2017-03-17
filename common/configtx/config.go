/*
Copyright IBM Corp. 2016-2017 All Rights Reserved.

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

package configtx

import (
	"bytes"
	"fmt"

	"github.com/hyperledger/fabric/common/config"
	"github.com/hyperledger/fabric/common/configtx/api"
	"github.com/hyperledger/fabric/common/policies"
	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
)

type ConfigResult interface {
	JSON() string
}

func NewConfigResult(config *cb.ConfigGroup, proposer api.Proposer) (ConfigResult, error) {
	return processConfig(config, proposer)
}

type configResult struct {
	tx                   interface{}
	groupName            string
	groupKey             string
	group                *cb.ConfigGroup
	valueHandler         config.ValueProposer
	policyHandler        policies.Proposer
	subResults           []*configResult
	deserializedValues   map[string]proto.Message
	deserializedPolicies map[string]proto.Message
}

func (cr *configResult) JSON() string {
	var buffer bytes.Buffer
	buffer.WriteString("{")
	cr.subResults[0].bufferJSON(&buffer)
	buffer.WriteString("}")
	return buffer.String()

}

// bufferJSON takes a buffer and writes a JSON representation of the configResult into the buffer
// Note that we use some mildly ad-hoc JSON encoding because the proto documentation explicitly
// mentions that the encoding/json package does not correctly marshal proto objects, and we
// do not have a proto object (nor can one be defined) which presents the mixed-map style of
// keys mapping to different types of the config
func (cr *configResult) bufferJSON(buffer *bytes.Buffer) {
	jpb := &jsonpb.Marshaler{
		EmitDefaults: true,
		Indent:       "    ",
	}

	// "GroupName": {
	buffer.WriteString("\"")
	buffer.WriteString(cr.groupKey)
	buffer.WriteString("\": {")

	//    "Values": {
	buffer.WriteString("\"Values\": {")
	count := 0
	for key, value := range cr.group.Values {
		// "Key": {
		buffer.WriteString("\"")
		buffer.WriteString(key)
		buffer.WriteString("\": {")
		// 	"Version": "X",
		buffer.WriteString("\"Version\":\"")
		buffer.WriteString(fmt.Sprintf("%d", value.Version))
		buffer.WriteString("\",")
		//      "ModPolicy": "foo",
		buffer.WriteString("\"ModPolicy\":\"")
		buffer.WriteString(value.ModPolicy)
		buffer.WriteString("\",")
		//      "Value": protoAsJSON
		buffer.WriteString("\"Value\":")
		jpb.Marshal(buffer, cr.deserializedValues[key])
		// },
		buffer.WriteString("}")
		count++
		if count < len(cr.group.Values) {
			buffer.WriteString(",")
		}
	}
	//     },
	buffer.WriteString("},")

	count = 0
	//    "Policies": {
	buffer.WriteString("\"Policies\": {")
	for key, policy := range cr.group.Policies {
		// "Key": {
		buffer.WriteString("\"")
		buffer.WriteString(key)
		buffer.WriteString("\": {")
		// 	"Version": "X",
		buffer.WriteString("\"Version\":\"")
		buffer.WriteString(fmt.Sprintf("%d", policy.Version))
		buffer.WriteString("\",")
		//      "ModPolicy": "foo",
		buffer.WriteString("\"ModPolicy\":\"")
		buffer.WriteString(policy.ModPolicy)
		buffer.WriteString("\",")
		//      "Policy": {
		buffer.WriteString("\"Policy\":{")
		//          "PolicyType" :
		buffer.WriteString(fmt.Sprintf("\"PolicyType\":\"%d\",", policy.Policy.Type))
		//          "Policy" : policyAsJSON
		buffer.WriteString("\"Policy\":")
		jpb.Marshal(buffer, cr.deserializedPolicies[key])
		//      }
		// },
		buffer.WriteString("}}")
		count++
		if count < len(cr.group.Policies) {
			buffer.WriteString(",")
		}
	}
	//     },
	buffer.WriteString("},")

	//     "Groups": {
	count = 0
	buffer.WriteString("\"Groups\": {")
	for _, subResult := range cr.subResults {
		subResult.bufferJSON(buffer)
		count++
		if count < len(cr.subResults) {
			buffer.WriteString(",")
		}
	}
	//     }
	buffer.WriteString("}")

	//     }
	buffer.WriteString("}")
}

func (cr *configResult) preCommit() error {
	for _, subResult := range cr.subResults {
		err := subResult.preCommit()
		if err != nil {
			return err
		}
	}
	return cr.valueHandler.PreCommit(cr.tx)
}

func (cr *configResult) commit() {
	for _, subResult := range cr.subResults {
		subResult.commit()
	}
	cr.valueHandler.CommitProposals(cr.tx)
	cr.policyHandler.CommitProposals(cr.tx)
}

func (cr *configResult) rollback() {
	for _, subResult := range cr.subResults {
		subResult.rollback()
	}
	cr.valueHandler.RollbackProposals(cr.tx)
	cr.policyHandler.RollbackProposals(cr.tx)
}

// proposeGroup proposes a group configuration with a given handler
// it will in turn recursively call itself until all groups have been exhausted
// at each call, it updates the configResult to contain references to the handlers
// which have been invoked so that calling result.commit() or result.rollback() will
// appropriately cleanup
func proposeGroup(result *configResult) error {
	subGroups := make([]string, len(result.group.Groups))
	i := 0
	for subGroup := range result.group.Groups {
		subGroups[i] = subGroup
		i++
	}

	valueDeserializer, subValueHandlers, err := result.valueHandler.BeginValueProposals(result.tx, subGroups)
	if err != nil {
		return err
	}

	subPolicyHandlers, err := result.policyHandler.BeginPolicyProposals(result.tx, subGroups)
	if err != nil {
		return err
	}

	if len(subValueHandlers) != len(subGroups) || len(subPolicyHandlers) != len(subGroups) {
		return fmt.Errorf("Programming error, did not return as many handlers as groups %d vs %d vs %d", len(subValueHandlers), len(subGroups), len(subPolicyHandlers))
	}

	for key, value := range result.group.Values {
		msg, err := valueDeserializer.Deserialize(key, value.Value)
		if err != nil {
			result.rollback()
			return fmt.Errorf("Error deserializing key %s for group %s: %s", key, result.groupName, err)
		}
		result.deserializedValues[key] = msg
	}

	for key, policy := range result.group.Policies {
		policy, err := result.policyHandler.ProposePolicy(result.tx, key, policy)
		if err != nil {
			result.rollback()
			return err
		}
		result.deserializedPolicies[key] = policy
	}

	result.subResults = make([]*configResult, 0, len(subGroups))

	for i, subGroup := range subGroups {
		result.subResults = append(result.subResults, &configResult{
			tx:                   result.tx,
			groupKey:             subGroup,
			groupName:            result.groupName + "/" + subGroup,
			group:                result.group.Groups[subGroup],
			valueHandler:         subValueHandlers[i],
			policyHandler:        subPolicyHandlers[i],
			deserializedValues:   make(map[string]proto.Message),
			deserializedPolicies: make(map[string]proto.Message),
		})

		if err := proposeGroup(result.subResults[i]); err != nil {
			result.rollback()
			return err
		}
	}

	return nil
}

func processConfig(channelGroup *cb.ConfigGroup, proposer api.Proposer) (*configResult, error) {
	helperGroup := cb.NewConfigGroup()
	helperGroup.Groups[RootGroupKey] = channelGroup

	configResult := &configResult{
		group:         helperGroup,
		valueHandler:  proposer.ValueProposer(),
		policyHandler: proposer.PolicyProposer(),
	}
	err := proposeGroup(configResult)
	if err != nil {
		return nil, err
	}

	return configResult, nil
}

func (cm *configManager) processConfig(channelGroup *cb.ConfigGroup) (*configResult, error) {
	logger.Debugf("Beginning new config for channel %s", cm.current.channelID)
	configResult, err := processConfig(channelGroup, cm.initializer)
	if err != nil {
		return nil, err
	}

	err = configResult.preCommit()
	if err != nil {
		configResult.rollback()
		return nil, err
	}

	return configResult, nil
}
