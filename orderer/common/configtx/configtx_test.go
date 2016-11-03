/*
Copyright IBM Corp. 2016 All Rights Reserved.

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

	"github.com/hyperledger/fabric/orderer/common/policies"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"

	"github.com/golang/protobuf/proto"
)

var defaultChain = []byte("DefaultChainID")

func defaultHandlers() map[ab.ConfigurationItem_ConfigurationType]Handler {
	handlers := make(map[ab.ConfigurationItem_ConfigurationType]Handler)
	for ctype := range ab.ConfigurationItem_ConfigurationType_name {
		handlers[ab.ConfigurationItem_ConfigurationType(ctype)] = NewBytesHandler()
	}
	return handlers
}

// mockPolicy always returns the error set as policyResult
type mockPolicy struct {
	policyResult error
}

func (mp *mockPolicy) Evaluate(msg []byte, sigs []*cb.Envelope) error {
	if mp == nil {
		return fmt.Errorf("Invoked nil policy")
	}
	return mp.policyResult
}

// mockPolicyManager always returns the policy set as policy, note that if unset, the default policy always returns error when evaluated
type mockPolicyManager struct {
	policy *mockPolicy
}

func (mpm *mockPolicyManager) GetPolicy(id string) (policies.Policy, bool) {
	return mpm.policy, (mpm.policy != nil)
}

func makeConfiguration(id, modificationPolicy string, lastModified uint64, data []byte) *ab.ConfigurationItem {
	return &ab.ConfigurationItem{
		ChainID:            defaultChain,
		ModificationPolicy: modificationPolicy,
		LastModified:       lastModified,
		Key:                id,
		Value:              data,
	}
}

func makeSignedConfigurationItem(id, modificationPolicy string, lastModified uint64, data []byte) *ab.SignedConfigurationItem {
	config := makeConfiguration(id, modificationPolicy, lastModified, data)
	marshaledConfig, err := proto.Marshal(config)
	if err != nil {
		panic(err)
	}
	return &ab.SignedConfigurationItem{
		Configuration: marshaledConfig,
	}
}

// TestOmittedHandler tests that startup fails if not all configuration types have an associated handler
func TestOmittedHandler(t *testing.T) {
	_, err := NewConfigurationManager(&ab.ConfigurationEnvelope{
		Sequence: 0,
		ChainID:  defaultChain,
	}, &mockPolicyManager{&mockPolicy{}}, map[ab.ConfigurationItem_ConfigurationType]Handler{})

	if err == nil {
		t.Fatalf("Should have failed to construct manager because handlers were missing")
	}
}

// TestWrongChainID tests that a configuration update for a different chain ID fails
func TestWrongChainID(t *testing.T) {
	cm, err := NewConfigurationManager(&ab.ConfigurationEnvelope{
		Sequence: 0,
		ChainID:  defaultChain,
	}, &mockPolicyManager{&mockPolicy{}}, defaultHandlers())

	if err != nil {
		t.Fatalf("Error constructing configuration manager: %s", err)
	}

	newConfig := &ab.ConfigurationEnvelope{
		Sequence: 1,
		ChainID:  []byte("wrongChain"),
	}

	err = cm.Validate(newConfig)
	if err == nil {
		t.Errorf("Should have errored when validating a new configuration set the wrong chain ID")
	}

	err = cm.Apply(newConfig)
	if err == nil {
		t.Errorf("Should have errored when applying a new configuration with the wrong chain ID")
	}
}

// TestOldConfigReplay tests that resubmitting a config for a sequence number which is not newer is ignored
func TestOldConfigReplay(t *testing.T) {
	cm, err := NewConfigurationManager(&ab.ConfigurationEnvelope{
		Sequence: 0,
		ChainID:  defaultChain,
	}, &mockPolicyManager{&mockPolicy{}}, defaultHandlers())

	if err != nil {
		t.Fatalf("Error constructing configuration manager: %s", err)
	}

	newConfig := &ab.ConfigurationEnvelope{
		Sequence: 0,
		ChainID:  defaultChain,
	}

	err = cm.Validate(newConfig)
	if err == nil {
		t.Errorf("Should have errored when validating a configuration that is not a newer sequence number")
	}

	err = cm.Apply(newConfig)
	if err == nil {
		t.Errorf("Should have errored when applying a configuration that is not a newer sequence number")
	}
}

// TestInvalidInitialConfigByStructure tests to make sure that if the config contains corrupted configuration that construction results in error
func TestInvalidInitialConfigByStructure(t *testing.T) {
	entries := []*ab.SignedConfigurationItem{makeSignedConfigurationItem("foo", "foo", 0, []byte("foo"))}
	entries[0].Configuration = []byte("Corrupted")
	_, err := NewConfigurationManager(&ab.ConfigurationEnvelope{
		Sequence: 0,
		ChainID:  defaultChain,
		Items:    entries,
	}, &mockPolicyManager{&mockPolicy{}}, defaultHandlers())

	if err == nil {
		t.Fatalf("Should have failed to construct configuration by policy")
	}
}

// TestValidConfigChange tests the happy path of updating a configuration value with no defaultModificationPolicy
func TestValidConfigChange(t *testing.T) {
	cm, err := NewConfigurationManager(&ab.ConfigurationEnvelope{
		Sequence: 0,
		ChainID:  defaultChain,
	}, &mockPolicyManager{}, defaultHandlers())

	if err != nil {
		t.Fatalf("Error constructing configuration manager: %s", err)
	}

	newConfig := &ab.ConfigurationEnvelope{
		Sequence: 1,
		ChainID:  defaultChain,
		Items:    []*ab.SignedConfigurationItem{makeSignedConfigurationItem("foo", "foo", 1, []byte("foo"))},
	}

	err = cm.Validate(newConfig)
	if err != nil {
		t.Errorf("Should not have errored validating config: %s", err)
	}

	err = cm.Apply(newConfig)
	if err != nil {
		t.Errorf("Should not have errored applying config: %s", err)
	}
}

// TestConfigChangeNoUpdatedSequence tests that a new submitted config is rejected if it increments the
// sequence number without a corresponding config item with that sequence number
func TestConfigChangeNoUpdatedSequence(t *testing.T) {
	cm, err := NewConfigurationManager(&ab.ConfigurationEnvelope{
		Sequence: 0,
		ChainID:  defaultChain,
	}, &mockPolicyManager{&mockPolicy{}}, defaultHandlers())

	if err != nil {
		t.Fatalf("Error constructing configuration manager: %s", err)
	}

	newConfig := &ab.ConfigurationEnvelope{
		Sequence: 1,
		ChainID:  defaultChain,
		// Note that the entries do not contain any config items with seqNo=1, so this is invalid
		Items: []*ab.SignedConfigurationItem{makeSignedConfigurationItem("foo", "foo", 0, []byte("foo"))},
	}

	err = cm.Validate(newConfig)
	if err == nil {
		t.Errorf("Should have errored validating config because no new sequence number matches")
	}

	err = cm.Apply(newConfig)
	if err == nil {
		t.Errorf("Should have errored applying config because no new sequence number matches")
	}
}

// TestConfigChangeRegressedSequence tests to make sure that a new config cannot roll back one of the
// config values while advancing another
func TestConfigChangeRegressedSequence(t *testing.T) {
	cm, err := NewConfigurationManager(&ab.ConfigurationEnvelope{
		Sequence: 1,
		ChainID:  defaultChain,
		Items:    []*ab.SignedConfigurationItem{makeSignedConfigurationItem("foo", "foo", 1, []byte("foo"))},
	}, &mockPolicyManager{&mockPolicy{}}, defaultHandlers())

	if err != nil {
		t.Fatalf("Error constructing configuration manager: %s", err)
	}

	newConfig := &ab.ConfigurationEnvelope{
		Sequence: 2,
		ChainID:  defaultChain,
		Items: []*ab.SignedConfigurationItem{
			makeSignedConfigurationItem("foo", "foo", 0, []byte("foo")),
			makeSignedConfigurationItem("bar", "bar", 2, []byte("bar")),
		},
	}

	err = cm.Validate(newConfig)
	if err == nil {
		t.Errorf("Should have errored validating config because foo's sequence number regressed")
	}

	err = cm.Apply(newConfig)
	if err == nil {
		t.Errorf("Should have errored applying config because foo's sequence number regressed")
	}
}

// TestConfigImplicitDelete tests to make sure that a new config does not implicitly delete config items
// by omitting them in the new config
func TestConfigImplicitDelete(t *testing.T) {
	cm, err := NewConfigurationManager(&ab.ConfigurationEnvelope{
		Sequence: 0,
		ChainID:  defaultChain,
		Items: []*ab.SignedConfigurationItem{
			makeSignedConfigurationItem("foo", "foo", 0, []byte("foo")),
			makeSignedConfigurationItem("bar", "bar", 0, []byte("bar")),
		},
	}, &mockPolicyManager{&mockPolicy{}}, defaultHandlers())

	if err != nil {
		t.Fatalf("Error constructing configuration manager: %s", err)
	}

	newConfig := &ab.ConfigurationEnvelope{
		Sequence: 1,
		ChainID:  defaultChain,
		Items: []*ab.SignedConfigurationItem{
			makeSignedConfigurationItem("bar", "bar", 1, []byte("bar")),
		},
	}

	err = cm.Validate(newConfig)
	if err == nil {
		t.Errorf("Should have errored validating config because foo was implicitly deleted")
	}

	err = cm.Apply(newConfig)
	if err == nil {
		t.Errorf("Should have errored applying config because foo was implicitly deleted")
	}
}

// TestConfigModifyWithoutFullIncrease tests to make sure that if an item is modified in a config change
// that it not only increments its LastModified, but also increments it to the current sequence number
func TestConfigModifyWithoutFullIncrease(t *testing.T) {
	cm, err := NewConfigurationManager(&ab.ConfigurationEnvelope{
		Sequence: 0,
		ChainID:  defaultChain,
		Items: []*ab.SignedConfigurationItem{
			makeSignedConfigurationItem("foo", "foo", 0, []byte("foo")),
			makeSignedConfigurationItem("bar", "bar", 0, []byte("bar")),
		},
	}, &mockPolicyManager{&mockPolicy{}}, defaultHandlers())

	if err != nil {
		t.Fatalf("Error constructing configuration manager: %s", err)
	}

	newConfig := &ab.ConfigurationEnvelope{
		Sequence: 1,
		ChainID:  defaultChain,
		Items: []*ab.SignedConfigurationItem{
			makeSignedConfigurationItem("foo", "foo", 0, []byte("foo")),
			makeSignedConfigurationItem("bar", "bar", 1, []byte("bar")),
		},
	}

	err = cm.Validate(newConfig)
	if err != nil {
		t.Errorf("Should not have errored validating config: %s", err)
	}

	err = cm.Apply(newConfig)
	if err != nil {
		t.Errorf("Should not have errored applying config: %s", err)
	}

	newConfig = &ab.ConfigurationEnvelope{
		Sequence: 2,
		ChainID:  defaultChain,
		Items: []*ab.SignedConfigurationItem{
			makeSignedConfigurationItem("foo", "foo", 1, []byte("foo")),
			makeSignedConfigurationItem("bar", "bar", 2, []byte("bar")),
		},
	}

	err = cm.Validate(newConfig)
	if err == nil {
		t.Errorf("Should have errored validating config because foo was modified, but its lastModifiedSeqNo did not increase to the current seqNo")
	}

	err = cm.Apply(newConfig)
	if err == nil {
		t.Errorf("Should have errored applying config because foo was modified, but its lastModifiedSeqNo did not increase to the current seqNo")
	}
}

// TestSilentConfigModification tests to make sure that even if a validly signed new configuration for an existing sequence number
// is substituted into an otherwise valid new config, that the new config is rejected for attempting a modification without
// increasing the config item's LastModified
func TestSilentConfigModification(t *testing.T) {
	cm, err := NewConfigurationManager(&ab.ConfigurationEnvelope{
		Sequence: 0,
		ChainID:  defaultChain,
		Items: []*ab.SignedConfigurationItem{
			makeSignedConfigurationItem("foo", "foo", 0, []byte("foo")),
			makeSignedConfigurationItem("bar", "bar", 0, []byte("bar")),
		},
	}, &mockPolicyManager{&mockPolicy{}}, defaultHandlers())

	if err != nil {
		t.Fatalf("Error constructing configuration manager: %s", err)
	}

	newConfig := &ab.ConfigurationEnvelope{
		Sequence: 1,
		ChainID:  defaultChain,
		Items: []*ab.SignedConfigurationItem{
			makeSignedConfigurationItem("foo", "foo", 0, []byte("different")),
			makeSignedConfigurationItem("bar", "bar", 1, []byte("bar")),
		},
	}

	err = cm.Validate(newConfig)
	if err == nil {
		t.Errorf("Should not errored validating config because foo was silently modified (despite modification allowed by policy)")
	}

	err = cm.Apply(newConfig)
	if err == nil {
		t.Errorf("Should not errored applying config because foo was silently modified (despite modification allowed by policy)")
	}
}

// TestInvalidInitialConfigByPolicy tests to make sure that if an existing policies does not validate the config that
// even construction fails
func TestInvalidInitialConfigByPolicy(t *testing.T) {
	_, err := NewConfigurationManager(&ab.ConfigurationEnvelope{
		Sequence: 0,
		ChainID:  defaultChain,
		Items:    []*ab.SignedConfigurationItem{makeSignedConfigurationItem("foo", "foo", 0, []byte("foo"))},
	}, &mockPolicyManager{&mockPolicy{fmt.Errorf("err")}}, defaultHandlers())
	// mockPolicyManager will return non-validating defualt policy

	if err == nil {
		t.Fatalf("Should have failed to construct configuration by policy")
	}
}

// TestConfigChangeViolatesPolicy checks to make sure that if policy rejects the validation of a config item that
// it is rejected in a config update
func TestConfigChangeViolatesPolicy(t *testing.T) {
	mpm := &mockPolicyManager{}
	cm, err := NewConfigurationManager(&ab.ConfigurationEnvelope{
		Sequence: 0,
		ChainID:  defaultChain,
	}, mpm, defaultHandlers())

	if err != nil {
		t.Fatalf("Error constructing configuration manager: %s", err)
	}
	// Set the mock policy to error
	mpm.policy = &mockPolicy{fmt.Errorf("err")}

	newConfig := &ab.ConfigurationEnvelope{
		Sequence: 1,
		ChainID:  defaultChain,
		Items:    []*ab.SignedConfigurationItem{makeSignedConfigurationItem("foo", "foo", 1, []byte("foo"))},
	}

	err = cm.Validate(newConfig)
	if err == nil {
		t.Errorf("Should have errored validating config because policy rejected modification")
	}

	err = cm.Apply(newConfig)
	if err == nil {
		t.Errorf("Should have errored applying config because policy rejected modification")
	}
}

type failHandler struct{}

func (fh failHandler) BeginConfig()    {}
func (fh failHandler) RollbackConfig() {}
func (fh failHandler) CommitConfig()   {}
func (fh failHandler) ProposeConfig(item *ab.ConfigurationItem) error {
	return fmt.Errorf("Fail")
}

// TestInvalidProposal checks that even if the policy allows the transaction and the sequence etc. is well formed,
// that if the handler does not accept the config, it is rejected
func TestInvalidProposal(t *testing.T) {
	handlers := defaultHandlers()
	handlers[ab.ConfigurationItem_ConfigurationType(0)] = failHandler{}
	cm, err := NewConfigurationManager(&ab.ConfigurationEnvelope{
		Sequence: 0,
		ChainID:  defaultChain,
	}, &mockPolicyManager{&mockPolicy{}}, handlers)

	if err != nil {
		t.Fatalf("Error constructing configuration manager: %s", err)
	}

	newConfig := &ab.ConfigurationEnvelope{
		Sequence: 1,
		ChainID:  defaultChain,
		Items:    []*ab.SignedConfigurationItem{makeSignedConfigurationItem("foo", "foo", 1, []byte("foo"))},
	}

	err = cm.Validate(newConfig)
	if err == nil {
		t.Errorf("Should have errored validating config because the handler rejected it")
	}

	err = cm.Apply(newConfig)
	if err == nil {
		t.Errorf("Should have errored applying config because the handler rejected it")
	}
}

// TestConfigItemOnWrongChain tests to make sure that a config is rejected if it contains an item for the wrong chain
func TestConfigItemOnWrongChain(t *testing.T) {
	cm, err := NewConfigurationManager(&ab.ConfigurationEnvelope{
		Sequence: 0,
		ChainID:  defaultChain,
	}, &mockPolicyManager{&mockPolicy{}}, defaultHandlers())

	if err != nil {
		t.Fatalf("Error constructing configuration manager: %s", err)
	}

	config := makeConfiguration("foo", "foo", 1, []byte("foo"))
	config.ChainID = []byte("Wrong")
	marshaledConfig, err := proto.Marshal(config)
	if err != nil {
		t.Fatalf("Should have been able marshal config: %s", err)
	}
	newConfig := &ab.ConfigurationEnvelope{
		Sequence: 1,
		ChainID:  defaultChain,
		Items:    []*ab.SignedConfigurationItem{&ab.SignedConfigurationItem{Configuration: marshaledConfig}},
	}

	err = cm.Validate(newConfig)
	if err == nil {
		t.Errorf("Should have errored validating config because new config item is for a different chain")
	}

	err = cm.Apply(newConfig)
	if err == nil {
		t.Errorf("Should have errored applying config because new config item is for a different chain")
	}
}
