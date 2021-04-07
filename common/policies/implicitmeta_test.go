/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package policies

import (
	"fmt"
	"testing"

	"github.com/hyperledger/fabric/msp"

	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
)

const TestPolicyName = "TestPolicyName"

type acceptPolicy struct{}

func (ap acceptPolicy) EvaluateSignedData(signedData []*protoutil.SignedData) error {
	return nil
}

func (ap acceptPolicy) EvaluateIdentities(identity []msp.Identity) error {
	return nil
}

func TestImplicitMarshalError(t *testing.T) {
	_, err := NewImplicitMetaPolicy([]byte("GARBAGE"), nil)
	require.Error(t, err, "Should have errored unmarshalling garbage")
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
			Policies: policyMap,
		}
	}
	return result
}

// makePolicyTest creates an implicitMetaPolicy with a set of
func runPolicyTest(t *testing.T, rule cb.ImplicitMetaPolicy_Rule, managerCount int, passingCount int) error {
	imp, err := NewImplicitMetaPolicy(protoutil.MarshalOrPanic(&cb.ImplicitMetaPolicy{
		Rule:      rule,
		SubPolicy: TestPolicyName,
	}), makeManagers(managerCount, passingCount))
	if err != nil {
		panic(err)
	}

	errSD := imp.EvaluateSignedData(nil)

	imp, err = NewImplicitMetaPolicy(protoutil.MarshalOrPanic(&cb.ImplicitMetaPolicy{
		Rule:      rule,
		SubPolicy: TestPolicyName,
	}), makeManagers(managerCount, passingCount))
	if err != nil {
		panic(err)
	}

	errI := imp.EvaluateIdentities(nil)

	require.False(t, ((errI == nil && errSD != nil) || (errSD == nil && errI != nil)))
	if errI != nil && errSD != nil {
		require.Equal(t, errI.Error(), errSD.Error())
	}

	return errI
}

func TestImplicitMetaAny(t *testing.T) {
	require.NoError(t, runPolicyTest(t, cb.ImplicitMetaPolicy_ANY, 1, 1))
	require.NoError(t, runPolicyTest(t, cb.ImplicitMetaPolicy_ANY, 10, 1))
	require.NoError(t, runPolicyTest(t, cb.ImplicitMetaPolicy_ANY, 10, 8))
	require.NoError(t, runPolicyTest(t, cb.ImplicitMetaPolicy_ANY, 0, 0))

	err := runPolicyTest(t, cb.ImplicitMetaPolicy_ANY, 10, 0)
	require.EqualError(t, err, "implicit policy evaluation failed - 0 sub-policies were satisfied, but this policy requires 1 of the 'TestPolicyName' sub-policies to be satisfied")
}

func TestImplicitMetaAll(t *testing.T) {
	require.NoError(t, runPolicyTest(t, cb.ImplicitMetaPolicy_ALL, 1, 1))
	require.NoError(t, runPolicyTest(t, cb.ImplicitMetaPolicy_ALL, 10, 10))
	require.NoError(t, runPolicyTest(t, cb.ImplicitMetaPolicy_ALL, 0, 0))

	err := runPolicyTest(t, cb.ImplicitMetaPolicy_ALL, 10, 1)
	require.EqualError(t, err, "implicit policy evaluation failed - 1 sub-policies were satisfied, but this policy requires 10 of the 'TestPolicyName' sub-policies to be satisfied")

	err = runPolicyTest(t, cb.ImplicitMetaPolicy_ALL, 10, 0)
	require.EqualError(t, err, "implicit policy evaluation failed - 0 sub-policies were satisfied, but this policy requires 10 of the 'TestPolicyName' sub-policies to be satisfied")
}

func TestImplicitMetaMajority(t *testing.T) {
	require.NoError(t, runPolicyTest(t, cb.ImplicitMetaPolicy_MAJORITY, 1, 1))
	require.NoError(t, runPolicyTest(t, cb.ImplicitMetaPolicy_MAJORITY, 10, 6))
	require.NoError(t, runPolicyTest(t, cb.ImplicitMetaPolicy_MAJORITY, 3, 2))
	require.NoError(t, runPolicyTest(t, cb.ImplicitMetaPolicy_MAJORITY, 0, 0))

	err := runPolicyTest(t, cb.ImplicitMetaPolicy_MAJORITY, 10, 5)
	require.EqualError(t, err, "implicit policy evaluation failed - 5 sub-policies were satisfied, but this policy requires 6 of the 'TestPolicyName' sub-policies to be satisfied")

	err = runPolicyTest(t, cb.ImplicitMetaPolicy_MAJORITY, 10, 0)
	require.EqualError(t, err, "implicit policy evaluation failed - 0 sub-policies were satisfied, but this policy requires 6 of the 'TestPolicyName' sub-policies to be satisfied")
}
