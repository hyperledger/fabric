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

	"github.com/hyperledger/fabric/common/configtx/api"
	mockconfigtx "github.com/hyperledger/fabric/common/mocks/configtx"
	mockpolicies "github.com/hyperledger/fabric/common/mocks/policies"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
)

var defaultChain = "DefaultChainID"

func defaultInitializer() *mockconfigtx.Initializer {
	return &mockconfigtx.Initializer{
		Resources: mockconfigtx.Resources{
			PolicyManagerVal: &mockpolicies.Manager{
				Policy: &mockpolicies.Policy{},
			},
		},
		HandlerVal: &mockconfigtx.Handler{},
	}
}

func makeConfigItem(id, modificationPolicy string, lastModified uint64, data []byte) *cb.ConfigItem {
	return &cb.ConfigItem{
		ModificationPolicy: modificationPolicy,
		LastModified:       lastModified,
		Key:                id,
		Value:              data,
	}
}

func makeMarshaledConfig(chainID string, configItems ...*cb.ConfigItem) []byte {
	values := make(map[string]*cb.ConfigValue)
	for _, item := range configItems {
		values[item.Key] = &cb.ConfigValue{
			Version:   item.LastModified,
			ModPolicy: item.ModificationPolicy,
			Value:     item.Value,
		}
	}

	config := &cb.ConfigNext{
		Header: &cb.ChannelHeader{ChannelId: chainID},
		Channel: &cb.ConfigGroup{
			Values: values,
		},
	}
	return utils.MarshalOrPanic(config)
}

func TestCallback(t *testing.T) {
	var calledBack api.Manager
	callback := func(m api.Manager) {
		calledBack = m
	}

	cm, err := NewManagerImpl(&cb.ConfigEnvelope{
		Config: makeMarshaledConfig(defaultChain, makeConfigItem("foo", "foo", 0, []byte("foo"))),
	}, defaultInitializer(), []func(api.Manager){callback})

	if err != nil {
		t.Fatalf("Error constructing config manager: %s", err)
	}

	if calledBack != cm {
		t.Fatalf("Should have called back with the correct manager")
	}
}

// TestDifferentChainID tests that a config update for a different chain ID fails
func TestDifferentChainID(t *testing.T) {
	cm, err := NewManagerImpl(&cb.ConfigEnvelope{
		Config: makeMarshaledConfig(defaultChain, makeConfigItem("foo", "foo", 0, []byte("foo"))),
	}, defaultInitializer(), nil)

	if err != nil {
		t.Fatalf("Error constructing config manager: %s", err)
	}

	newConfig := &cb.ConfigEnvelope{
		Config: makeMarshaledConfig("wrongChain", makeConfigItem("foo", "foo", 1, []byte("foo"))),
	}

	err = cm.Validate(newConfig)
	if err == nil {
		t.Error("Should have errored when validating a new config set the wrong chain ID")
	}

	err = cm.Apply(newConfig)
	if err == nil {
		t.Error("Should have errored when applying a new config with the wrong chain ID")
	}
}

// TestOldConfigReplay tests that resubmitting a config for a sequence number which is not newer is ignored
func TestOldConfigReplay(t *testing.T) {
	cm, err := NewManagerImpl(&cb.ConfigEnvelope{
		Config: makeMarshaledConfig(defaultChain, makeConfigItem("foo", "foo", 0, []byte("foo"))),
	}, defaultInitializer(), nil)

	if err != nil {
		t.Fatalf("Error constructing config manager: %s", err)
	}

	newConfig := &cb.ConfigEnvelope{
		Config: makeMarshaledConfig(defaultChain, makeConfigItem("foo", "foo", 0, []byte("foo"))),
	}

	err = cm.Validate(newConfig)
	if err == nil {
		t.Error("Should have errored when validating a config that is not a newer sequence number")
	}

	err = cm.Apply(newConfig)
	if err == nil {
		t.Error("Should have errored when applying a config that is not a newer sequence number")
	}
}

// TestInvalidInitialConfigByStructure tests to make sure that if the config contains corrupted config that construction results in error
func TestInvalidInitialConfigByStructure(t *testing.T) {
	_, err := NewManagerImpl(&cb.ConfigEnvelope{
		Config: []byte("Corrupted"),
	}, defaultInitializer(), nil)

	if err == nil {
		t.Fatal("Should have failed to construct config by policy")
	}
}

// TestValidConfigChange tests the happy path of updating a config value with no defaultModificationPolicy
func TestValidConfigChange(t *testing.T) {
	cm, err := NewManagerImpl(&cb.ConfigEnvelope{
		Config: makeMarshaledConfig(defaultChain, makeConfigItem("foo", "foo", 0, []byte("foo"))),
	}, defaultInitializer(), nil)

	if err != nil {
		t.Fatalf("Error constructing config manager: %s", err)
	}

	newConfig := &cb.ConfigEnvelope{
		Config: makeMarshaledConfig(defaultChain, makeConfigItem("foo", "foo", 1, []byte("foo"))),
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
	cm, err := NewManagerImpl(&cb.ConfigEnvelope{
		Config: makeMarshaledConfig(defaultChain, makeConfigItem("foo", "foo", 1, []byte("foo"))),
	}, defaultInitializer(), nil)

	if err != nil {
		t.Fatalf("Error constructing config manager: %s", err)
	}

	newConfig := &cb.ConfigEnvelope{
		Config: makeMarshaledConfig(
			defaultChain,
			makeConfigItem("foo", "foo", 0, []byte("foo")),
			makeConfigItem("bar", "bar", 2, []byte("bar")),
		),
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
	cm, err := NewManagerImpl(&cb.ConfigEnvelope{
		Config: makeMarshaledConfig(defaultChain, makeConfigItem("foo", "foo", 1, []byte("foo"))),
	}, defaultInitializer(), nil)

	if err != nil {
		t.Fatalf("Error constructing config manager: %s", err)
	}

	newConfig := &cb.ConfigEnvelope{
		Config: makeMarshaledConfig(
			defaultChain,
			makeConfigItem("foo", "foo", 2, []byte("foo")),
			makeConfigItem("bar", "bar", 1, []byte("bar")),
		),
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
	cm, err := NewManagerImpl(&cb.ConfigEnvelope{
		Config: makeMarshaledConfig(
			defaultChain,
			makeConfigItem("foo", "foo", 0, []byte("foo")),
			makeConfigItem("bar", "bar", 0, []byte("bar")),
		),
	}, defaultInitializer(), nil)

	if err != nil {
		t.Fatalf("Error constructing config manager: %s", err)
	}

	newConfig := &cb.ConfigEnvelope{
		Config: makeMarshaledConfig(
			defaultChain,
			makeConfigItem("bar", "bar", 1, []byte("bar")),
		),
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
	cm, err := NewManagerImpl(&cb.ConfigEnvelope{
		Config: makeMarshaledConfig(defaultChain, makeConfigItem("foo", "foo", 0, []byte("foo"))),
	}, defaultInitializer(), nil)

	if err != nil {
		t.Fatalf("Error constructing config manager: %s", err)
	}

	newConfig := &cb.ConfigEnvelope{}

	err = cm.Validate(newConfig)
	if err == nil {
		t.Error("Should not errored validating config because new config is empty")
	}

	err = cm.Apply(newConfig)
	if err == nil {
		t.Error("Should not errored applying config because new config is empty")
	}
}

// TestSilentConfigModification tests to make sure that even if a validly signed new config for an existing sequence number
// is substituted into an otherwise valid new config, that the new config is rejected for attempting a modification without
// increasing the config item's LastModified
func TestSilentConfigModification(t *testing.T) {
	cm, err := NewManagerImpl(&cb.ConfigEnvelope{
		Config: makeMarshaledConfig(
			defaultChain,
			makeConfigItem("foo", "foo", 0, []byte("foo")),
			makeConfigItem("bar", "bar", 0, []byte("bar")),
		),
	}, defaultInitializer(), nil)

	if err != nil {
		t.Fatalf("Error constructing config manager: %s", err)
	}

	newConfig := &cb.ConfigEnvelope{
		Config: makeMarshaledConfig(
			defaultChain,
			makeConfigItem("foo", "foo", 0, []byte("different")),
			makeConfigItem("bar", "bar", 1, []byte("bar")),
		),
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
	initializer := defaultInitializer()
	initializer.Resources.PolicyManagerVal.Policy.Err = fmt.Errorf("err")
	_, err := NewManagerImpl(&cb.ConfigEnvelope{
		Config: makeMarshaledConfig(defaultChain, makeConfigItem("foo", "foo", 0, []byte("foo"))),
	}, initializer, nil)

	if err == nil {
		t.Fatal("Should have failed to construct config by policy")
	}
}

// TestConfigChangeViolatesPolicy checks to make sure that if policy rejects the validation of a config item that
// it is rejected in a config update
func TestConfigChangeViolatesPolicy(t *testing.T) {
	initializer := defaultInitializer()
	cm, err := NewManagerImpl(&cb.ConfigEnvelope{
		Config: makeMarshaledConfig(defaultChain, makeConfigItem("foo", "foo", 0, []byte("foo"))),
	}, initializer, nil)

	if err != nil {
		t.Fatalf("Error constructing config manager: %s", err)
	}
	// Set the mock policy to error
	initializer.Resources.PolicyManagerVal.Policy.Err = fmt.Errorf("err")

	newConfig := &cb.ConfigEnvelope{
		Config: makeMarshaledConfig(defaultChain, makeConfigItem("foo", "foo", 1, []byte("foo"))),
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
	initializer := defaultInitializer()
	cm, err := NewManagerImpl(&cb.ConfigEnvelope{
		Config: makeMarshaledConfig(defaultChain, makeConfigItem("foo", "foo", 0, []byte("foo"))),
	}, initializer, nil)

	if err != nil {
		t.Fatalf("Error constructing config manager: %s", err)
	}

	// Set the mock policy to error
	initializer.Resources.PolicyManagerVal.PolicyMap = make(map[string]*mockpolicies.Policy)
	initializer.Resources.PolicyManagerVal.PolicyMap["foo"] = &mockpolicies.Policy{Err: fmt.Errorf("err")}

	newConfig := &cb.ConfigEnvelope{
		Config: makeMarshaledConfig(
			defaultChain,
			makeConfigItem("foo", "foo", 0, []byte("foo")),
			makeConfigItem("bar", "bar", 1, []byte("foo")),
		),
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

// TestInvalidProposal checks that even if the policy allows the transaction and the sequence etc. is well formed,
// that if the handler does not accept the config, it is rejected
func TestInvalidProposal(t *testing.T) {
	initializer := defaultInitializer()
	cm, err := NewManagerImpl(&cb.ConfigEnvelope{
		Config: makeMarshaledConfig(defaultChain, makeConfigItem("foo", "foo", 0, []byte("foo"))),
	}, initializer, nil)

	if err != nil {
		t.Fatalf("Error constructing config manager: %s", err)
	}

	initializer.HandlerVal = &mockconfigtx.Handler{ErrorForProposeConfig: fmt.Errorf("err")}

	newConfig := &cb.ConfigEnvelope{
		Config: makeMarshaledConfig(defaultChain, makeConfigItem("foo", "foo", 1, []byte("foo"))),
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

// TestMissingHeader checks that a config item with a missing header causes the config to be rejected
func TestMissingHeader(t *testing.T) {
	configItem := makeConfigItem("foo", "foo", 0, []byte("foo"))
	data := utils.MarshalOrPanic(&cb.Config{Items: []*cb.ConfigItem{configItem}})
	_, err := NewManagerImpl(&cb.ConfigEnvelope{
		Config: data,
	}, defaultInitializer(), nil)

	if err == nil {
		t.Error("Should have errored creating the config manager because of the missing header")
	}
}

// TestMissingChainID checks that a config item with a missing chainID causes the config to be rejected
func TestMissingChainID(t *testing.T) {
	_, err := NewManagerImpl(&cb.ConfigEnvelope{
		Config: makeMarshaledConfig("", makeConfigItem("foo", "foo", 0, []byte("foo"))),
	}, defaultInitializer(), nil)

	if err == nil {
		t.Error("Should have errored creating the config manager because of the missing header")
	}
}
