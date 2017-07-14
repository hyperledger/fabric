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

	mockconfigtx "github.com/hyperledger/fabric/common/mocks/configtx"
	mockpolicies "github.com/hyperledger/fabric/common/mocks/policies"
	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/stretchr/testify/assert"
)

func TestReadSetNotPresent(t *testing.T) {
	cm := &configSet{
		configMap: make(map[string]comparable),
	}

	cm.configMap["1"] = comparable{}
	cm.configMap["2"] = comparable{}

	readSet := make(map[string]comparable)
	readSet["1"] = comparable{}
	readSet["3"] = comparable{}

	assert.Error(t, cm.verifyReadSet(readSet), "ReadSet contained '3', not in config")
}

func TestReadSetBackVersioned(t *testing.T) {
	cm := &configSet{
		configMap: make(map[string]comparable),
	}

	cm.configMap["1"] = comparable{ConfigValue: &cb.ConfigValue{Version: 1}}
	cm.configMap["2"] = comparable{}

	readSet := make(map[string]comparable)
	readSet["1"] = comparable{}

	assert.Error(t, cm.verifyReadSet(readSet), "ReadSet contained '1', at old version")
}

func TestComputeDeltaSet(t *testing.T) {
	readSet := make(map[string]comparable)
	readSet["1"] = comparable{}
	readSet["2"] = comparable{}

	writeSet := make(map[string]comparable)
	writeSet["1"] = comparable{}
	writeSet["2"] = comparable{ConfigValue: &cb.ConfigValue{Version: 1}}
	writeSet["3"] = comparable{}

	result := ComputeDeltaSet(readSet, writeSet)
	assert.Len(t, result, 2, "Should have two values in the delta set")
	assert.NotNil(t, result["2"], "Element had version increased")
	assert.NotNil(t, result["3"], "Element was new")
}

func TestVerifyDeltaSet(t *testing.T) {
	cm := &configManager{
		Resources: &mockconfigtx.Resources{
			PolicyManagerVal: &mockpolicies.Manager{
				Policy: &mockpolicies.Policy{},
			},
		},
		current: &configSet{
			configMap: make(map[string]comparable),
		},
	}

	cm.current.configMap["foo"] = comparable{path: []string{"foo"}}

	t.Run("Green path", func(t *testing.T) {
		deltaSet := make(map[string]comparable)

		deltaSet["foo"] = comparable{ConfigValue: &cb.ConfigValue{Version: 1, ModPolicy: "foo"}}

		assert.NoError(t, cm.verifyDeltaSet(deltaSet, nil), "Good update")
	})

	t.Run("Bad mod policy", func(t *testing.T) {
		deltaSet := make(map[string]comparable)

		deltaSet["foo"] = comparable{ConfigValue: &cb.ConfigValue{Version: 1}}

		assert.Regexp(t, "invalid mod_policy for element", cm.verifyDeltaSet(deltaSet, nil))
	})

	t.Run("Big Skip", func(t *testing.T) {
		deltaSet := make(map[string]comparable)

		deltaSet["foo"] = comparable{ConfigValue: &cb.ConfigValue{Version: 2, ModPolicy: "foo"}}

		assert.Error(t, cm.verifyDeltaSet(deltaSet, nil), "Version skip from 0 to 2")
	})

	t.Run("New item high version", func(t *testing.T) {
		deltaSet := make(map[string]comparable)

		deltaSet["bar"] = comparable{ConfigValue: &cb.ConfigValue{Version: 1, ModPolicy: "foo"}}

		assert.Error(t, cm.verifyDeltaSet(deltaSet, nil), "New key not at version 0")
	})

	t.Run("Policy evalaution to false", func(t *testing.T) {
		deltaSet := make(map[string]comparable)

		deltaSet["foo"] = comparable{ConfigValue: &cb.ConfigValue{Version: 1, ModPolicy: "foo"}}
		cm.Resources.(*mockconfigtx.Resources).PolicyManagerVal.Policy = &mockpolicies.Policy{Err: fmt.Errorf("Err")}

		assert.Error(t, cm.verifyDeltaSet(deltaSet, nil), "Policy evaluation should have failed")
	})

	t.Run("Empty delta set", func(t *testing.T) {
		err := (&configManager{}).verifyDeltaSet(map[string]comparable{}, nil)
		assert.Error(t, err, "Empty delta set should be rejected")
		assert.Contains(t, err.Error(), "Delta set was empty.  Update would have no effect.")
	})
}

func TestPolicyForItem(t *testing.T) {
	// Policies are set to different error values to differentiate them in equal assertion
	rootPolicy := &mockpolicies.Policy{Err: fmt.Errorf("rootPolicy")}
	fooPolicy := &mockpolicies.Policy{Err: fmt.Errorf("fooPolicy")}

	cm := &configManager{
		Resources: &mockconfigtx.Resources{
			PolicyManagerVal: &mockpolicies.Manager{
				BasePathVal: "root",
				Policy:      rootPolicy,
				SubManagersMap: map[string]*mockpolicies.Manager{
					"foo": &mockpolicies.Manager{
						Policy:      fooPolicy,
						BasePathVal: "foo",
					},
				},
			},
		},
	}

	policy, ok := cm.policyForItem(comparable{
		path: []string{"root"},
		ConfigValue: &cb.ConfigValue{
			ModPolicy: "rootPolicy",
		},
	})
	assert.True(t, ok)
	assert.Equal(t, policy, rootPolicy, "Should have found relative policy off the root manager")

	policy, ok = cm.policyForItem(comparable{
		path: []string{"root", "wrong"},
		ConfigValue: &cb.ConfigValue{
			ModPolicy: "rootPolicy",
		},
	})
	assert.False(t, ok, "Should not have found rootPolicy off a non-existent manager")

	policy, ok = cm.policyForItem(comparable{
		path: []string{"root", "foo"},
		ConfigValue: &cb.ConfigValue{
			ModPolicy: "foo",
		},
	})
	assert.True(t, ok)
	assert.Equal(t, policy, fooPolicy, "Should not have found relative foo policy the foo manager")

	policy, ok = cm.policyForItem(comparable{
		key:  "foo",
		path: []string{"root"},
		ConfigGroup: &cb.ConfigGroup{
			ModPolicy: "foo",
		},
	})
	assert.True(t, ok)
	assert.Equal(t, policy, fooPolicy, "Should have found relative foo policy for foo group")
}

func TestValidateModPolicy(t *testing.T) {
	t.Run("Valid", func(t *testing.T) {
		assert.Nil(t, validateModPolicy("/foo/bar"))
	})
	t.Run("Empty", func(t *testing.T) {
		assert.Regexp(t, "mod_policy not set", validateModPolicy(""))
	})
	t.Run("InvalidFirstChar", func(t *testing.T) {
		assert.Regexp(t, "path element at 0 is invalid", validateModPolicy("^foo"))
	})
	t.Run("InvalidRootPath", func(t *testing.T) {
		assert.Regexp(t, "path element at 0 is invalid", validateModPolicy("/"))
	})
	t.Run("InvalidSubPath", func(t *testing.T) {
		assert.Regexp(t, "path element at 1 is invalid", validateModPolicy("foo//bar"))
	})
}
