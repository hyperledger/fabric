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
	assert.False(t, ok, "Should not have found rootPolicy off a non-existant manager")

	policy, ok = cm.policyForItem(comparable{
		path: []string{"root", "foo"},
		ConfigValue: &cb.ConfigValue{
			ModPolicy: "foo",
		},
	})
	assert.True(t, ok)
	assert.Equal(t, policy, fooPolicy, "Should not have found relative foo policy the foo manager")

	policy, ok = cm.policyForItem(comparable{
		path: []string{"root", "foo"},
		ConfigValue: &cb.ConfigValue{
			ModPolicy: "/rootPolicy",
		},
	})
	assert.True(t, ok)
	assert.Equal(t, policy, rootPolicy, "Should not have found absolute root policy from the foo path position")
}
