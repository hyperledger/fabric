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

	"github.com/hyperledger/fabric/common/config"
	configmsp "github.com/hyperledger/fabric/common/config/msp"
	mmsp "github.com/hyperledger/fabric/common/mocks/msp"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"

	"github.com/golang/protobuf/proto"
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

func TestNewChainTemplate(t *testing.T) {
	consortiumName := "Test"
	orgs := []string{"org1", "org2", "org3"}
	nct := NewChainCreationTemplate(consortiumName, orgs)

	newChainID := "foo"
	configEnv, err := nct.Envelope(newChainID)
	if err != nil {
		t.Fatalf("Error creation a chain creation config")
	}

	configUpdate, err := UnmarshalConfigUpdate(configEnv.ConfigUpdate)
	if err != nil {
		t.Fatalf("Should not have errored: %s", err)
	}

	consortiumProto := &cb.Consortium{}
	err = proto.Unmarshal(configUpdate.WriteSet.Values[config.ConsortiumKey].Value, consortiumProto)
	assert.NoError(t, err)
	assert.Equal(t, consortiumName, consortiumProto.Name, "Should have set correct consortium name")

	assert.Equal(t, configUpdate.WriteSet.Groups[config.ApplicationGroupKey].Version, uint64(1))

	assert.Len(t, configUpdate.WriteSet.Groups[config.ApplicationGroupKey].Groups, len(orgs))

	for _, org := range orgs {
		group, ok := configUpdate.WriteSet.Groups[config.ApplicationGroupKey].Groups[org]
		assert.True(t, ok, "Expected to find %s but did not", org)
		for _, policy := range group.Policies {
			assert.Equal(t, configmsp.AdminsPolicyKey, policy.ModPolicy)
		}
	}
}

func TestMakeChainCreationTransactionWithSigner(t *testing.T) {
	channelID := "foo"

	signer, err := mmsp.NewNoopMsp().GetDefaultSigningIdentity()
	assert.NoError(t, err, "Creating noop MSP")

	cct, err := MakeChainCreationTransaction(channelID, "test", signer)
	assert.NoError(t, err, "Making chain creation tx")

	assert.NotEmpty(t, cct.Signature, "Should have signature")

	payload, err := utils.UnmarshalPayload(cct.Payload)
	assert.NoError(t, err, "Unmarshaling payload")

	configUpdateEnv, err := UnmarshalConfigUpdateEnvelope(payload.Data)
	assert.NoError(t, err, "Unmarshaling ConfigUpdateEnvelope")

	assert.NotEmpty(t, configUpdateEnv.Signatures, "Should have config env sigs")

	sigHeader, err := utils.GetSignatureHeader(payload.Header.SignatureHeader)
	assert.NoError(t, err, "Unmarshaling SignatureHeader")
	assert.NotEmpty(t, sigHeader.Creator, "Creator specified")
}

func TestMakeChainCreationTransactionNoSigner(t *testing.T) {
	channelID := "foo"
	cct, err := MakeChainCreationTransaction(channelID, "test", nil)
	assert.NoError(t, err, "Making chain creation tx")

	assert.Empty(t, cct.Signature, "Should have empty signature")

	payload, err := utils.UnmarshalPayload(cct.Payload)
	assert.NoError(t, err, "Unmarshaling payload")

	configUpdateEnv, err := UnmarshalConfigUpdateEnvelope(payload.Data)
	assert.NoError(t, err, "Unmarshaling ConfigUpdateEnvelope")

	assert.Empty(t, configUpdateEnv.Signatures, "Should have no config env sigs")

	sigHeader, err := utils.GetSignatureHeader(payload.Header.SignatureHeader)
	assert.NoError(t, err, "Unmarshaling SignatureHeader")
	assert.Empty(t, sigHeader.Creator, "No creator specified")
}
