/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package policies

import (
	"fmt"
	"testing"

	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/assert"
)

const TestPolicyName = "TestPolicyName"

type acceptPolicy struct{}

func (rp acceptPolicy) Evaluate(signedData []*protoutil.SignedData) error {
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
	imp, err := newImplicitMetaPolicy(protoutil.MarshalOrPanic(&cb.ImplicitMetaPolicy{
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
	assert.NoError(t, runPolicyTest(cb.ImplicitMetaPolicy_ANY, 0, 0))

	err := runPolicyTest(cb.ImplicitMetaPolicy_ANY, 10, 0)
	assert.EqualError(t, err, "implicit policy evaluation failed - 0 sub-policies were satisfied, but this policy requires 1 of the 'TestPolicyName' sub-policies to be satisfied")
}

func TestImplicitMetaAll(t *testing.T) {
	assert.NoError(t, runPolicyTest(cb.ImplicitMetaPolicy_ALL, 1, 1))
	assert.NoError(t, runPolicyTest(cb.ImplicitMetaPolicy_ALL, 10, 10))
	assert.NoError(t, runPolicyTest(cb.ImplicitMetaPolicy_ALL, 0, 0))

	err := runPolicyTest(cb.ImplicitMetaPolicy_ALL, 10, 1)
	assert.EqualError(t, err, "implicit policy evaluation failed - 1 sub-policies were satisfied, but this policy requires 10 of the 'TestPolicyName' sub-policies to be satisfied")

	err = runPolicyTest(cb.ImplicitMetaPolicy_ALL, 10, 0)
	assert.EqualError(t, err, "implicit policy evaluation failed - 0 sub-policies were satisfied, but this policy requires 10 of the 'TestPolicyName' sub-policies to be satisfied")
}

func TestImplicitMetaMajority(t *testing.T) {
	assert.NoError(t, runPolicyTest(cb.ImplicitMetaPolicy_MAJORITY, 1, 1))
	assert.NoError(t, runPolicyTest(cb.ImplicitMetaPolicy_MAJORITY, 10, 6))
	assert.NoError(t, runPolicyTest(cb.ImplicitMetaPolicy_MAJORITY, 3, 2))
	assert.NoError(t, runPolicyTest(cb.ImplicitMetaPolicy_MAJORITY, 0, 0))

	err := runPolicyTest(cb.ImplicitMetaPolicy_MAJORITY, 10, 5)
	assert.EqualError(t, err, "implicit policy evaluation failed - 5 sub-policies were satisfied, but this policy requires 6 of the 'TestPolicyName' sub-policies to be satisfied")

	err = runPolicyTest(cb.ImplicitMetaPolicy_MAJORITY, 10, 0)
	assert.EqualError(t, err, "implicit policy evaluation failed - 0 sub-policies were satisfied, but this policy requires 6 of the 'TestPolicyName' sub-policies to be satisfied")
}
