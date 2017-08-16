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

package policies

import (
	"fmt"
	"testing"

	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"

	"github.com/stretchr/testify/assert"
)

const TestPolicyName = "TestPolicyName"

type acceptPolicy struct{}

func (rp acceptPolicy) Evaluate(signedData []*cb.SignedData) error {
	return nil
}

func TestImplicitMarshalError(t *testing.T) {
	_, err := newImplicitMetaPolicy([]byte("GARBAGE"), nil)
	assert.Error(t, err, "Should have errored unmarshaling garbage")
}

func makeManagers(count, passing int) map[string]*ManagerImpl {
	result := make(map[string]*ManagerImpl)
	remaining := passing
	for i := 0; i < count; i++ {
		policyMap := make(map[string]Policy)
		if remaining > 0 {
			policyMap[TestPolicyName] = acceptPolicy{}
		}
		remaining--

		result[fmt.Sprintf("%d", i)] = &ManagerImpl{
			policies: policyMap,
		}
	}
	return result
}

// makePolicyTest creates an implicitMetaPolicy with a set of
func runPolicyTest(rule cb.ImplicitMetaPolicy_Rule, managerCount int, passingCount int) error {
	imp, err := newImplicitMetaPolicy(utils.MarshalOrPanic(&cb.ImplicitMetaPolicy{
		Rule:      rule,
		SubPolicy: TestPolicyName,
	}), makeManagers(managerCount, passingCount))
	if err != nil {
		panic(err)
	}

	return imp.Evaluate(nil)
}

func TestImplicitMetaAny(t *testing.T) {
	assert.NoError(t, runPolicyTest(cb.ImplicitMetaPolicy_ANY, 1, 1))
	assert.NoError(t, runPolicyTest(cb.ImplicitMetaPolicy_ANY, 10, 1))
	assert.NoError(t, runPolicyTest(cb.ImplicitMetaPolicy_ANY, 10, 8))
	assert.Error(t, runPolicyTest(cb.ImplicitMetaPolicy_ANY, 10, 0))
	assert.NoError(t, runPolicyTest(cb.ImplicitMetaPolicy_ANY, 0, 0))
}

func TestImplicitMetaAll(t *testing.T) {
	assert.NoError(t, runPolicyTest(cb.ImplicitMetaPolicy_ALL, 1, 1))
	assert.Error(t, runPolicyTest(cb.ImplicitMetaPolicy_ALL, 10, 1))
	assert.NoError(t, runPolicyTest(cb.ImplicitMetaPolicy_ALL, 10, 10))
	assert.Error(t, runPolicyTest(cb.ImplicitMetaPolicy_ALL, 10, 0))
	assert.NoError(t, runPolicyTest(cb.ImplicitMetaPolicy_ALL, 0, 0))
}

func TestImplicitMetaMajority(t *testing.T) {
	assert.NoError(t, runPolicyTest(cb.ImplicitMetaPolicy_MAJORITY, 1, 1))
	assert.Error(t, runPolicyTest(cb.ImplicitMetaPolicy_MAJORITY, 10, 5))
	assert.NoError(t, runPolicyTest(cb.ImplicitMetaPolicy_MAJORITY, 10, 6))
	assert.NoError(t, runPolicyTest(cb.ImplicitMetaPolicy_MAJORITY, 3, 2))
	assert.Error(t, runPolicyTest(cb.ImplicitMetaPolicy_MAJORITY, 10, 0))
	assert.NoError(t, runPolicyTest(cb.ImplicitMetaPolicy_MAJORITY, 0, 0))
}
