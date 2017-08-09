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
	"fmt"
	"testing"

	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/stretchr/testify/assert"
)

func verifyItemsResult(t *testing.T, template Template, count int) {
	newChainID := "foo"

	configEnv, err := template.Envelope(newChainID)
	if err != nil {
		t.Fatalf("Should not have errored: %s", err)
	}

	configNext, err := UnmarshalConfigUpdate(configEnv.ConfigUpdate)
	if err != nil {
		t.Fatalf("Should not have errored: %s", err)
	}

	assert.Equal(t, len(configNext.WriteSet.Values), count, "Not the right number of config values")

	for i := 0; i < len(configNext.WriteSet.Values); i++ {
		_, ok := configNext.WriteSet.Values[fmt.Sprintf("%d", i)]
		assert.True(t, ok, "Expected %d but did not find it", i)
	}
}

func simpleGroup(index int) *cb.ConfigGroup {
	group := cb.NewConfigGroup()
	group.Values[fmt.Sprintf("%d", index)] = &cb.ConfigValue{}
	return group
}

func TestSimpleTemplate(t *testing.T) {
	simple := NewSimpleTemplate(
		simpleGroup(0),
		simpleGroup(1),
	)
	verifyItemsResult(t, simple, 2)
}

func TestCompositeTemplate(t *testing.T) {
	composite := NewCompositeTemplate(
		NewSimpleTemplate(
			simpleGroup(0),
			simpleGroup(1),
		),
		NewSimpleTemplate(
			simpleGroup(2),
		),
	)

	verifyItemsResult(t, composite, 3)
}

func TestModPolicySettingTemplate(t *testing.T) {
	existingModPolicy := "bar"

	subGroup := "group"
	subGroupExistingModPolicy := "otherGroup"
	input := cb.NewConfigGroup()
	input.Groups[subGroup] = cb.NewConfigGroup()
	input.Groups[subGroupExistingModPolicy] = &cb.ConfigGroup{ModPolicy: existingModPolicy}

	policyName := "policy"
	valueName := "value"
	policyExistingModPolicy := "otherPolicy"
	valueExistingModPolicy := "otherValue"
	for _, group := range []*cb.ConfigGroup{input, input.Groups[subGroup]} {
		group.Values[valueName] = &cb.ConfigValue{}
		group.Policies[policyName] = &cb.ConfigPolicy{}
		group.Values[valueExistingModPolicy] = &cb.ConfigValue{ModPolicy: existingModPolicy}
		group.Policies[policyExistingModPolicy] = &cb.ConfigPolicy{ModPolicy: existingModPolicy}
	}

	modPolicyName := "foo"
	mpst := NewModPolicySettingTemplate(modPolicyName, NewSimpleTemplate(input))
	output, err := mpst.Envelope("bar")
	assert.NoError(t, err, "Creating envelope")

	configUpdate := UnmarshalConfigUpdateOrPanic(output.ConfigUpdate)

	assert.Equal(t, modPolicyName, configUpdate.WriteSet.ModPolicy)
	assert.Equal(t, modPolicyName, configUpdate.WriteSet.Values[valueName].ModPolicy)
	assert.Equal(t, modPolicyName, configUpdate.WriteSet.Policies[policyName].ModPolicy)
	assert.Equal(t, existingModPolicy, configUpdate.WriteSet.Values[valueExistingModPolicy].ModPolicy)
	assert.Equal(t, existingModPolicy, configUpdate.WriteSet.Policies[policyExistingModPolicy].ModPolicy)
	assert.Equal(t, modPolicyName, configUpdate.WriteSet.Groups[subGroup].ModPolicy)
	assert.Equal(t, modPolicyName, configUpdate.WriteSet.Groups[subGroup].Values[valueName].ModPolicy)
	assert.Equal(t, modPolicyName, configUpdate.WriteSet.Groups[subGroup].Policies[policyName].ModPolicy)
	assert.Equal(t, existingModPolicy, configUpdate.WriteSet.Groups[subGroup].Values[valueExistingModPolicy].ModPolicy)
	assert.Equal(t, existingModPolicy, configUpdate.WriteSet.Groups[subGroup].Policies[policyExistingModPolicy].ModPolicy)
	assert.Equal(t, existingModPolicy, configUpdate.WriteSet.Groups[subGroupExistingModPolicy].ModPolicy)
}
