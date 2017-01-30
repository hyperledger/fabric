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

package configtx_test

import (
	"fmt"
	"testing"

	. "github.com/hyperledger/fabric/common/configtx"
	mockconfigtx "github.com/hyperledger/fabric/common/mocks/configtx"
	"github.com/hyperledger/fabric/common/policies"
	cb "github.com/hyperledger/fabric/protos/common"

	"errors"

	"github.com/golang/protobuf/proto"
)

var defaultChain = "DefaultChainID"

func defaultHandlers() map[cb.ConfigurationItem_ConfigurationType]Handler {
	handlers := make(map[cb.ConfigurationItem_ConfigurationType]Handler)
	for ctype := range cb.ConfigurationItem_ConfigurationType_name {
		handlers[cb.ConfigurationItem_ConfigurationType(ctype)] = NewBytesHandler()
	}
	return handlers
}

// mockPolicy always returns the error set as policyResult
type mockPolicy struct {
	policyResult error
	// validReplies is the number of times to successfully validate before returning policyResult
	// it is decremented at each invocation
	validReplies int
}

func (mp *mockPolicy) Evaluate(signedData []*cb.SignedData) error {
	if mp == nil {
		return errors.New("Invoked nil policy")
	}
	if mp.validReplies > 0 {
		return nil
	}
	mp.validReplies--
	return mp.policyResult
}

// mockPolicyManager always returns the policy set as policy, note that if unset, the default policy always returns error when evaluated
type mockPolicyManager struct {
	policy *mockPolicy
}

func (mpm *mockPolicyManager) GetPolicy(id string) (policies.Policy, bool) {
	return mpm.policy, (mpm.policy != nil)
}

func makeConfigurationItem(id, modificationPolicy string, lastModified uint64, data []byte) *cb.ConfigurationItem {
	return &cb.ConfigurationItem{
		ModificationPolicy: modificationPolicy,
		LastModified:       lastModified,
		Key:                id,
		Value:              data,
	}
}

func makeSignedConfigurationItem(id, modificationPolicy string, lastModified uint64, data []byte) *cb.SignedConfigurationItem {
	config := makeConfigurationItem(id, modificationPolicy, lastModified, data)
	marshaledConfig, err := proto.Marshal(config)
	if err != nil {
		panic(err)
	}
	return &cb.SignedConfigurationItem{
		ConfigurationItem: marshaledConfig,
	}
}

// TestOmittedHandler tests that startup fails if not all configuration types have an associated handler
func TestOmittedHandler(t *testing.T) {
	_, err := NewManagerImpl(&cb.ConfigurationEnvelope{
		Items:  []*cb.SignedConfigurationItem{makeSignedConfigurationItem("foo", "foo", 0, []byte("foo"))},
		Header: &cb.ChainHeader{ChainID: defaultChain},
	}, &mockconfigtx.Initializer{PolicyManagerVal: &mockPolicyManager{&mockPolicy{}}, HandlersVal: map[cb.ConfigurationItem_ConfigurationType]Handler{}}, nil)

	if err == nil {
		t.Fatal("Should have failed to construct manager because handlers were missing")
	}
}

func TestCallback(t *testing.T) {
	var calledBack Manager
	callback := func(m Manager) {
		calledBack = m
	}

	cm, err := NewManagerImpl(&cb.ConfigurationEnvelope{
		Items:  []*cb.SignedConfigurationItem{makeSignedConfigurationItem("foo", "foo", 0, []byte("foo"))},
		Header: &cb.ChainHeader{ChainID: defaultChain},
	}, &mockconfigtx.Initializer{PolicyManagerVal: &mockPolicyManager{&mockPolicy{}}, HandlersVal: defaultHandlers()}, []func(Manager){callback})

	if err != nil {
		t.Fatalf("Error constructing configuration manager: %s", err)
	}

	if calledBack != cm {
		t.Fatalf("Should have called back with the correct manager")
	}
}

// TestDifferentChainID tests that a configuration update for a different chain ID fails
func TestDifferentChainID(t *testing.T) {
	cm, err := NewManagerImpl(&cb.ConfigurationEnvelope{
		Items:  []*cb.SignedConfigurationItem{makeSignedConfigurationItem("foo", "foo", 0, []byte("foo"))},
		Header: &cb.ChainHeader{ChainID: defaultChain},
	}, &mockconfigtx.Initializer{PolicyManagerVal: &mockPolicyManager{&mockPolicy{}}, HandlersVal: defaultHandlers()}, nil)

	if err != nil {
		t.Fatalf("Error constructing configuration manager: %s", err)
	}

	newConfig := &cb.ConfigurationEnvelope{
		Items:  []*cb.SignedConfigurationItem{makeSignedConfigurationItem("foo", "foo", 1, []byte("foo"))},
		Header: &cb.ChainHeader{ChainID: "wrongChain"},
	}

	err = cm.Validate(newConfig)
	if err == nil {
		t.Error("Should have errored when validating a new configuration set the wrong chain ID")
	}

	err = cm.Apply(newConfig)
	if err == nil {
		t.Error("Should have errored when applying a new configuration with the wrong chain ID")
	}
}

// TestOldConfigReplay tests that resubmitting a config for a sequence number which is not newer is ignored
func TestOldConfigReplay(t *testing.T) {
	cm, err := NewManagerImpl(&cb.ConfigurationEnvelope{
		Items:  []*cb.SignedConfigurationItem{makeSignedConfigurationItem("foo", "foo", 0, []byte("foo"))},
		Header: &cb.ChainHeader{ChainID: defaultChain},
	}, &mockconfigtx.Initializer{PolicyManagerVal: &mockPolicyManager{&mockPolicy{}}, HandlersVal: defaultHandlers()}, nil)

	if err != nil {
		t.Fatalf("Error constructing configuration manager: %s", err)
	}

	newConfig := &cb.ConfigurationEnvelope{
		Items: []*cb.SignedConfigurationItem{makeSignedConfigurationItem("foo", "foo", 0, []byte("foo"))},
	}

	err = cm.Validate(newConfig)
	if err == nil {
		t.Error("Should have errored when validating a configuration that is not a newer sequence number")
	}

	err = cm.Apply(newConfig)
	if err == nil {
		t.Error("Should have errored when applying a configuration that is not a newer sequence number")
	}
}

// TestInvalidInitialConfigByStructure tests to make sure that if the config contains corrupted configuration that construction results in error
func TestInvalidInitialConfigByStructure(t *testing.T) {
	entries := []*cb.SignedConfigurationItem{makeSignedConfigurationItem("foo", "foo", 0, []byte("foo"))}
	entries[0].ConfigurationItem = []byte("Corrupted")
	_, err := NewManagerImpl(&cb.ConfigurationEnvelope{
		Items:  entries,
		Header: &cb.ChainHeader{ChainID: defaultChain},
	}, &mockconfigtx.Initializer{PolicyManagerVal: &mockPolicyManager{&mockPolicy{}}, HandlersVal: defaultHandlers()}, nil)

	if err == nil {
		t.Fatal("Should have failed to construct configuration by policy")
	}
}

// TestValidConfigChange tests the happy path of updating a configuration value with no defaultModificationPolicy
func TestValidConfigChange(t *testing.T) {
	cm, err := NewManagerImpl(&cb.ConfigurationEnvelope{
		Items:  []*cb.SignedConfigurationItem{makeSignedConfigurationItem("foo", "foo", 0, []byte("foo"))},
		Header: &cb.ChainHeader{ChainID: defaultChain},
	}, &mockconfigtx.Initializer{PolicyManagerVal: &mockPolicyManager{&mockPolicy{}}, HandlersVal: defaultHandlers()}, nil)

	if err != nil {
		t.Fatalf("Error constructing configuration manager: %s", err)
	}

	newConfig := &cb.ConfigurationEnvelope{
		Items:  []*cb.SignedConfigurationItem{makeSignedConfigurationItem("foo", "foo", 1, []byte("foo"))},
		Header: &cb.ChainHeader{ChainID: defaultChain},
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

// TestConfigChangeRegressedSequence tests to make sure that a new config cannot roll back one of the
// config values while advancing another
func TestConfigChangeRegressedSequence(t *testing.T) {
	cm, err := NewManagerImpl(&cb.ConfigurationEnvelope{
		Items:  []*cb.SignedConfigurationItem{makeSignedConfigurationItem("foo", "foo", 1, []byte("foo"))},
		Header: &cb.ChainHeader{ChainID: defaultChain},
	}, &mockconfigtx.Initializer{PolicyManagerVal: &mockPolicyManager{&mockPolicy{}}, HandlersVal: defaultHandlers()}, nil)

	if err != nil {
		t.Fatalf("Error constructing configuration manager: %s", err)
	}

	newConfig := &cb.ConfigurationEnvelope{
		Items: []*cb.SignedConfigurationItem{
			makeSignedConfigurationItem("foo", "foo", 0, []byte("foo")),
			makeSignedConfigurationItem("bar", "bar", 2, []byte("bar")),
		},
		Header: &cb.ChainHeader{ChainID: defaultChain},
	}

	err = cm.Validate(newConfig)
	if err == nil {
		t.Error("Should have errored validating config because foo's sequence number regressed")
	}

	err = cm.Apply(newConfig)
	if err == nil {
		t.Error("Should have errored applying config because foo's sequence number regressed")
	}
}

// TestConfigChangeOldSequence tests to make sure that a new config cannot roll back one of the
// config values while advancing another
func TestConfigChangeOldSequence(t *testing.T) {
	cm, err := NewManagerImpl(&cb.ConfigurationEnvelope{
		Items:  []*cb.SignedConfigurationItem{makeSignedConfigurationItem("foo", "foo", 1, []byte("foo"))},
		Header: &cb.ChainHeader{ChainID: defaultChain},
	}, &mockconfigtx.Initializer{PolicyManagerVal: &mockPolicyManager{&mockPolicy{}}, HandlersVal: defaultHandlers()}, nil)

	if err != nil {
		t.Fatalf("Error constructing configuration manager: %s", err)
	}

	newConfig := &cb.ConfigurationEnvelope{
		Items: []*cb.SignedConfigurationItem{
			makeSignedConfigurationItem("foo", "foo", 2, []byte("foo")),
			makeSignedConfigurationItem("bar", "bar", 1, []byte("bar")),
		},
		Header: &cb.ChainHeader{ChainID: defaultChain},
	}

	err = cm.Validate(newConfig)
	if err == nil {
		t.Error("Should have errored validating config because bar was new but its sequence number was old")
	}

	err = cm.Apply(newConfig)
	if err == nil {
		t.Error("Should have errored applying config because bar was new but its sequence number was old")
	}
}

// TestConfigImplicitDelete tests to make sure that a new config does not implicitly delete config items
// by omitting them in the new config
func TestConfigImplicitDelete(t *testing.T) {
	cm, err := NewManagerImpl(&cb.ConfigurationEnvelope{
		Items: []*cb.SignedConfigurationItem{
			makeSignedConfigurationItem("foo", "foo", 0, []byte("foo")),
			makeSignedConfigurationItem("bar", "bar", 0, []byte("bar")),
		},
		Header: &cb.ChainHeader{ChainID: defaultChain},
	}, &mockconfigtx.Initializer{PolicyManagerVal: &mockPolicyManager{&mockPolicy{}}, HandlersVal: defaultHandlers()}, nil)

	if err != nil {
		t.Fatalf("Error constructing configuration manager: %s", err)
	}

	newConfig := &cb.ConfigurationEnvelope{
		Items: []*cb.SignedConfigurationItem{
			makeSignedConfigurationItem("bar", "bar", 1, []byte("bar")),
		},
		Header: &cb.ChainHeader{ChainID: defaultChain},
	}

	err = cm.Validate(newConfig)
	if err == nil {
		t.Error("Should have errored validating config because foo was implicitly deleted")
	}

	err = cm.Apply(newConfig)
	if err == nil {
		t.Error("Should have errored applying config because foo was implicitly deleted")
	}
}

// TestEmptyConfigUpdate tests to make sure that an empty config is rejected as an update
func TestEmptyConfigUpdate(t *testing.T) {
	cm, err := NewManagerImpl(&cb.ConfigurationEnvelope{
		Items:  []*cb.SignedConfigurationItem{makeSignedConfigurationItem("foo", "foo", 0, []byte("foo"))},
		Header: &cb.ChainHeader{ChainID: defaultChain},
	}, &mockconfigtx.Initializer{PolicyManagerVal: &mockPolicyManager{&mockPolicy{}}, HandlersVal: defaultHandlers()}, nil)

	if err != nil {
		t.Fatalf("Error constructing configuration manager: %s", err)
	}

	newConfig := &cb.ConfigurationEnvelope{}

	err = cm.Validate(newConfig)
	if err == nil {
		t.Error("Should not errored validating config because new config is empty")
	}

	err = cm.Apply(newConfig)
	if err == nil {
		t.Error("Should not errored applying config because new config is empty")
	}
}

// TestSilentConfigModification tests to make sure that even if a validly signed new configuration for an existing sequence number
// is substituted into an otherwise valid new config, that the new config is rejected for attempting a modification without
// increasing the config item's LastModified
func TestSilentConfigModification(t *testing.T) {
	cm, err := NewManagerImpl(&cb.ConfigurationEnvelope{
		Items: []*cb.SignedConfigurationItem{
			makeSignedConfigurationItem("foo", "foo", 0, []byte("foo")),
			makeSignedConfigurationItem("bar", "bar", 0, []byte("bar")),
		},
		Header: &cb.ChainHeader{ChainID: defaultChain},
	}, &mockconfigtx.Initializer{PolicyManagerVal: &mockPolicyManager{&mockPolicy{}}, HandlersVal: defaultHandlers()}, nil)

	if err != nil {
		t.Fatalf("Error constructing configuration manager: %s", err)
	}

	newConfig := &cb.ConfigurationEnvelope{
		Items: []*cb.SignedConfigurationItem{
			makeSignedConfigurationItem("foo", "foo", 0, []byte("different")),
			makeSignedConfigurationItem("bar", "bar", 1, []byte("bar")),
		},
		Header: &cb.ChainHeader{ChainID: defaultChain},
	}

	err = cm.Validate(newConfig)
	if err == nil {
		t.Error("Should not errored validating config because foo was silently modified (despite modification allowed by policy)")
	}

	err = cm.Apply(newConfig)
	if err == nil {
		t.Error("Should not errored applying config because foo was silently modified (despite modification allowed by policy)")
	}
}

// TestInvalidInitialConfigByPolicy tests to make sure that if an existing policies does not validate the config that
// even construction fails
func TestInvalidInitialConfigByPolicy(t *testing.T) {
	_, err := NewManagerImpl(&cb.ConfigurationEnvelope{
		Items:  []*cb.SignedConfigurationItem{makeSignedConfigurationItem("foo", "foo", 0, []byte("foo"))},
		Header: &cb.ChainHeader{ChainID: defaultChain},
	}, &mockconfigtx.Initializer{PolicyManagerVal: &mockPolicyManager{&mockPolicy{policyResult: fmt.Errorf("err")}}, HandlersVal: defaultHandlers()}, nil)

	if err == nil {
		t.Fatal("Should have failed to construct configuration by policy")
	}
}

// TestConfigChangeViolatesPolicy checks to make sure that if policy rejects the validation of a config item that
// it is rejected in a config update
func TestConfigChangeViolatesPolicy(t *testing.T) {
	mpm := &mockPolicyManager{}
	cm, err := NewManagerImpl(&cb.ConfigurationEnvelope{
		Items:  []*cb.SignedConfigurationItem{makeSignedConfigurationItem("foo", "foo", 0, []byte("foo"))},
		Header: &cb.ChainHeader{ChainID: defaultChain},
	}, &mockconfigtx.Initializer{PolicyManagerVal: mpm, HandlersVal: defaultHandlers()}, nil)

	if err != nil {
		t.Fatalf("Error constructing configuration manager: %s", err)
	}
	// Set the mock policy to error
	mpm.policy = &mockPolicy{policyResult: fmt.Errorf("err")}

	newConfig := &cb.ConfigurationEnvelope{
		Items:  []*cb.SignedConfigurationItem{makeSignedConfigurationItem("foo", "foo", 1, []byte("foo"))},
		Header: &cb.ChainHeader{ChainID: defaultChain},
	}

	err = cm.Validate(newConfig)
	if err == nil {
		t.Error("Should have errored validating config because policy rejected modification")
	}

	err = cm.Apply(newConfig)
	if err == nil {
		t.Error("Should have errored applying config because policy rejected modification")
	}
}

// TestUnchangedConfigViolatesPolicy checks to make sure that existing config items are not revalidated against their modification policies
// as the policy may have changed, certs revoked, etc. since the config was adopted.
func TestUnchangedConfigViolatesPolicy(t *testing.T) {
	mpm := &mockPolicyManager{}
	cm, err := NewManagerImpl(&cb.ConfigurationEnvelope{
		Items:  []*cb.SignedConfigurationItem{makeSignedConfigurationItem("foo", "foo", 0, []byte("foo"))},
		Header: &cb.ChainHeader{ChainID: defaultChain},
	}, &mockconfigtx.Initializer{PolicyManagerVal: mpm, HandlersVal: defaultHandlers()}, nil)

	if err != nil {
		t.Fatalf("Error constructing configuration manager: %s", err)
	}
	// Set the mock policy to error
	mpm.policy = &mockPolicy{
		policyResult: fmt.Errorf("err"),
		validReplies: 1,
	}

	newConfig := &cb.ConfigurationEnvelope{
		Items: []*cb.SignedConfigurationItem{
			makeSignedConfigurationItem("foo", "foo", 0, []byte("foo")),
			makeSignedConfigurationItem("bar", "bar", 1, []byte("foo")),
		},
		Header: &cb.ChainHeader{ChainID: defaultChain},
	}

	err = cm.Validate(newConfig)
	if err != nil {
		t.Errorf("Should not have errored validating config, but got %s", err)
	}

	err = cm.Apply(newConfig)
	if err != nil {
		t.Errorf("Should not have errored applying config, but got %s", err)
	}
}

type failHandler struct{}

func (fh failHandler) BeginConfig()    {}
func (fh failHandler) RollbackConfig() {}
func (fh failHandler) CommitConfig()   {}
func (fh failHandler) ProposeConfig(item *cb.ConfigurationItem) error {
	return errors.New("Fail")
}

// TestInvalidProposal checks that even if the policy allows the transaction and the sequence etc. is well formed,
// that if the handler does not accept the config, it is rejected
func TestInvalidProposal(t *testing.T) {
	handlers := defaultHandlers()
	initializer := &mockconfigtx.Initializer{PolicyManagerVal: &mockPolicyManager{&mockPolicy{}}, HandlersVal: handlers}
	cm, err := NewManagerImpl(&cb.ConfigurationEnvelope{
		Items:  []*cb.SignedConfigurationItem{makeSignedConfigurationItem("foo", "foo", 0, []byte("foo"))},
		Header: &cb.ChainHeader{ChainID: defaultChain},
	}, initializer, nil)

	if err != nil {
		t.Fatalf("Error constructing configuration manager: %s", err)
	}

	initializer.Handlers()[cb.ConfigurationItem_ConfigurationType(0)] = failHandler{}

	newConfig := &cb.ConfigurationEnvelope{
		Items: []*cb.SignedConfigurationItem{makeSignedConfigurationItem("foo", "foo", 1, []byte("foo"))},
	}

	err = cm.Validate(newConfig)
	if err == nil {
		t.Error("Should have errored validating config because the handler rejected it")
	}

	err = cm.Apply(newConfig)
	if err == nil {
		t.Error("Should have errored applying config because the handler rejected it")
	}
}

// TestMissingHeader checks that a configuration item with a missing header causes the config to be rejected
func TestMissingHeader(t *testing.T) {
	handlers := defaultHandlers()
	configItem := makeConfigurationItem("foo", "foo", 0, []byte("foo"))
	data, _ := proto.Marshal(configItem)
	_, err := NewManagerImpl(&cb.ConfigurationEnvelope{
		Items: []*cb.SignedConfigurationItem{&cb.SignedConfigurationItem{ConfigurationItem: data}},
	}, &mockconfigtx.Initializer{PolicyManagerVal: &mockPolicyManager{&mockPolicy{}}, HandlersVal: handlers}, nil)

	if err == nil {
		t.Error("Should have errored creating the configuration manager because of the missing header")
	}
}

// TestMissingChainID checks that a configuration item with a missing chainID causes the config to be rejected
func TestMissingChainID(t *testing.T) {
	handlers := defaultHandlers()
	_, err := NewManagerImpl(&cb.ConfigurationEnvelope{
		Items:  []*cb.SignedConfigurationItem{makeSignedConfigurationItem("foo", "foo", 0, []byte("foo"))},
		Header: &cb.ChainHeader{},
	}, &mockconfigtx.Initializer{PolicyManagerVal: &mockPolicyManager{&mockPolicy{}}, HandlersVal: handlers}, nil)

	if err == nil {
		t.Error("Should have errored creating the configuration manager because of the missing header")
	}
}
