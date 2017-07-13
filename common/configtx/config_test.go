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

package configtx

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/assert"
)

func TestJSON(t *testing.T) {
	cr := &configResult{
		groupKey: "rootGroup",
		group: &cb.ConfigGroup{
			Values: map[string]*cb.ConfigValue{
				"outer": &cb.ConfigValue{Version: 1, ModPolicy: "mod1"},
			},
		},
		subResults: []*configResult{
			&configResult{
				groupKey: "innerGroup1",
				group: &cb.ConfigGroup{
					Values: map[string]*cb.ConfigValue{
						"inner1": &cb.ConfigValue{ModPolicy: "mod3"},
					},
					Policies: map[string]*cb.ConfigPolicy{
						"policy1": &cb.ConfigPolicy{Policy: &cb.Policy{}, ModPolicy: "mod1"},
					},
				},
				deserializedValues: map[string]proto.Message{
					"inner1": &ab.ConsensusType{Type: "inner1"},
				},
				deserializedPolicies: map[string]proto.Message{
					"policy1": &ab.ConsensusType{Type: "policy1"},
				},
			},
			&configResult{
				groupKey: "innerGroup2",
				group: &cb.ConfigGroup{
					Values: map[string]*cb.ConfigValue{
						"inner2": &cb.ConfigValue{ModPolicy: "mod3"},
					},
					Policies: map[string]*cb.ConfigPolicy{
						"policy2": &cb.ConfigPolicy{Policy: &cb.Policy{Type: 1}, ModPolicy: "mod2"},
					},
				},
				deserializedValues: map[string]proto.Message{
					"inner2": &ab.ConsensusType{Type: "inner2"},
				},
				deserializedPolicies: map[string]proto.Message{
					"policy2": &ab.ConsensusType{Type: "policy2"},
				},
			},
		},
		deserializedValues: map[string]proto.Message{
			"outer": &ab.ConsensusType{Type: "outer"},
		},
	}

	crWrapper := &configResult{subResults: []*configResult{cr}}

	buffer := &bytes.Buffer{}
	assert.NoError(t, json.Indent(buffer, []byte(crWrapper.JSON()), "", ""), "JSON should parse nicely")

	expected := "{\"rootGroup\":{\"Values\":{\"outer\":{\"Version\":\"1\",\"ModPolicy\":\"mod1\",\"Value\":{\"type\":\"outer\"}}},\"Policies\":{},\"Groups\":{\"innerGroup1\":{\"Values\":{\"inner1\":{\"Version\":\"0\",\"ModPolicy\":\"mod3\",\"Value\":{\"type\":\"inner1\"}}},\"Policies\":{\"policy1\":{\"Version\":\"0\",\"ModPolicy\":\"mod1\",\"Policy\":{\"PolicyType\":\"0\",\"Policy\":{\"type\":\"policy1\"}}}},\"Groups\":{}},\"innerGroup2\":{\"Values\":{\"inner2\":{\"Version\":\"0\",\"ModPolicy\":\"mod3\",\"Value\":{\"type\":\"inner2\"}}},\"Policies\":{\"policy2\":{\"Version\":\"0\",\"ModPolicy\":\"mod2\",\"Policy\":{\"PolicyType\":\"1\",\"Policy\":{\"type\":\"policy2\"}}}},\"Groups\":{}}}}}"

	// Remove all newlines and spaces from the JSON
	compactedJSON := strings.Replace(strings.Replace(buffer.String(), "\n", "", -1), " ", "", -1)

	assert.Equal(t, expected, compactedJSON)
}
