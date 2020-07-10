/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package configtx

import (
	"fmt"
	"reflect"
	"testing"

	cb "github.com/hyperledger/fabric-protos-go/common"
	mockpolicies "github.com/hyperledger/fabric/common/configtx/mock"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/stretchr/testify/require"
)

func TestReadSetNotPresent(t *testing.T) {
	vi := &ValidatorImpl{
		configMap: make(map[string]comparable),
	}

	vi.configMap["1"] = comparable{}
	vi.configMap["2"] = comparable{}

	readSet := make(map[string]comparable)
	readSet["1"] = comparable{}
	readSet["3"] = comparable{}

	require.Error(t, vi.verifyReadSet(readSet), "ReadSet contained '3', not in config")
}

func TestReadSetBackVersioned(t *testing.T) {
	vi := &ValidatorImpl{
		configMap: make(map[string]comparable),
	}

	vi.configMap["1"] = comparable{ConfigValue: &cb.ConfigValue{Version: 1}}
	vi.configMap["2"] = comparable{}

	readSet := make(map[string]comparable)
	readSet["1"] = comparable{}

	require.Error(t, vi.verifyReadSet(readSet), "ReadSet contained '1', at old version")
}

func TestComputeDeltaSet(t *testing.T) {
	readSet := make(map[string]comparable)
	readSet["1"] = comparable{}
	readSet["2"] = comparable{}

	writeSet := make(map[string]comparable)
	writeSet["1"] = comparable{}
	writeSet["2"] = comparable{ConfigValue: &cb.ConfigValue{Version: 1}}
	writeSet["3"] = comparable{}

	result := computeDeltaSet(readSet, writeSet)
	require.Len(t, result, 2, "Should have two values in the delta set")
	require.NotNil(t, result["2"], "Element had version increased")
	require.NotNil(t, result["3"], "Element was new")
}

func TestVerifyDeltaSet(t *testing.T) {
	pm := &mockpolicies.PolicyManager{}
	pm.GetPolicyReturns(&mockpolicies.Policy{}, true)
	vi := &ValidatorImpl{
		pm:        pm,
		configMap: make(map[string]comparable),
	}

	vi.configMap["foo"] = comparable{path: []string{"foo"}}

	t.Run("Green path", func(t *testing.T) {
		deltaSet := make(map[string]comparable)

		deltaSet["foo"] = comparable{ConfigValue: &cb.ConfigValue{Version: 1, ModPolicy: "foo"}}

		require.NoError(t, vi.verifyDeltaSet(deltaSet, nil), "Good update")
	})

	t.Run("Bad mod policy", func(t *testing.T) {
		deltaSet := make(map[string]comparable)

		deltaSet["foo"] = comparable{ConfigValue: &cb.ConfigValue{Version: 1}}

		require.Regexp(t, "invalid mod_policy for element", vi.verifyDeltaSet(deltaSet, nil))
	})

	t.Run("Big Skip", func(t *testing.T) {
		deltaSet := make(map[string]comparable)

		deltaSet["foo"] = comparable{ConfigValue: &cb.ConfigValue{Version: 2, ModPolicy: "foo"}}

		require.Error(t, vi.verifyDeltaSet(deltaSet, nil), "Version skip from 0 to 2")
	})

	t.Run("New item high version", func(t *testing.T) {
		deltaSet := make(map[string]comparable)

		deltaSet["bar"] = comparable{ConfigValue: &cb.ConfigValue{Version: 1, ModPolicy: "foo"}}

		require.Error(t, vi.verifyDeltaSet(deltaSet, nil), "New key not at version 0")
	})

	t.Run("Policy evalaution to false", func(t *testing.T) {
		deltaSet := make(map[string]comparable)

		deltaSet["foo"] = comparable{ConfigValue: &cb.ConfigValue{Version: 1, ModPolicy: "foo"}}
		fakePolicy := &mockpolicies.Policy{}
		fakePolicy.EvaluateSignedDataReturns(fmt.Errorf("MockErr-fakePolicy.Evaluate-1557327297"))
		vi.pm.(*mockpolicies.PolicyManager).GetPolicyReturns(fakePolicy, true)

		require.Error(t, vi.verifyDeltaSet(deltaSet, nil), "Policy evaluation should have failed")
	})

	t.Run("Empty delta set", func(t *testing.T) {
		err := (&ValidatorImpl{}).verifyDeltaSet(map[string]comparable{}, nil)
		require.Error(t, err, "Empty delta set should be rejected")
		require.Contains(t, err.Error(), "delta set was empty -- update would have no effect")
	})
}

func TestPolicyForItem(t *testing.T) {
	// Policies are set to different error values to differentiate them in equal assertion

	fakeFooPolicy := &mockpolicies.Policy{}
	fakeFooPolicy.EvaluateSignedDataReturns(fmt.Errorf("MockErr-fooPolicy-1557327481"))
	fakeFooPolicyManager := &mockpolicies.PolicyManager{}
	fakeFooPolicyManager.GetPolicyStub = func(s string) (i policies.Policy, b bool) {
		if s == "foo" {
			return fakeFooPolicy, true
		}
		return nil, false
	}
	fakeRootPolicy := &mockpolicies.Policy{}
	fakeRootPolicy.EvaluateSignedDataReturns(fmt.Errorf("MockErr-rootPolicy-1557327456"))
	fakeRootPolicyManager := &mockpolicies.PolicyManager{}
	fakeRootPolicyManager.GetPolicyStub = func(s string) (i policies.Policy, b bool) {
		if s == "rootPolicy" {
			return fakeRootPolicy, true
		}
		return nil, false
	}
	fakeRootPolicyManager.ManagerStub = func(path []string) (manager policies.Manager, b bool) {
		switch {
		case len(path) == 0:
			return fakeRootPolicyManager, true
		case reflect.DeepEqual(path, []string{"foo"}):
			return fakeFooPolicyManager, true
		default:
			return nil, false
		}
	}

	vi := &ValidatorImpl{
		pm: fakeRootPolicyManager,
	}

	t.Run("Root manager", func(t *testing.T) {
		policy, ok := vi.policyForItem(comparable{
			path: []string{"root"},
			ConfigValue: &cb.ConfigValue{
				ModPolicy: "rootPolicy",
			},
		})
		require.True(t, ok)
		require.Equal(t, policy, fakeRootPolicy, "Should have found relative policy off the root manager")
	})

	t.Run("Nonexistent manager", func(t *testing.T) {
		_, ok := vi.policyForItem(comparable{
			path: []string{"root", "wrong"},
			ConfigValue: &cb.ConfigValue{
				ModPolicy: "rootPolicy",
			},
		})
		require.False(t, ok, "Should not have found rootPolicy off a nonexistent manager")
	})

	t.Run("Foo manager", func(t *testing.T) {
		policy, ok := vi.policyForItem(comparable{
			path: []string{"root", "foo"},
			ConfigValue: &cb.ConfigValue{
				ModPolicy: "foo",
			},
		})
		require.True(t, ok)
		require.Equal(t, policy, fakeFooPolicy, "Should have found relative foo policy off the foo manager")
	})

	t.Run("Foo group", func(t *testing.T) {
		policy, ok := vi.policyForItem(comparable{
			key:  "foo",
			path: []string{"root"},
			ConfigGroup: &cb.ConfigGroup{
				ModPolicy: "foo",
			},
		})
		require.True(t, ok)
		require.Equal(t, policy, fakeFooPolicy, "Should have found relative foo policy for foo group")
	})

	t.Run("Root group manager", func(t *testing.T) {
		policy, ok := vi.policyForItem(comparable{
			path: []string{},
			key:  "root",
			ConfigGroup: &cb.ConfigGroup{
				ModPolicy: "rootPolicy",
			},
		})
		require.True(t, ok)
		require.Equal(t, policy, fakeRootPolicy, "Should have found relative policy off the root manager")
	})
}

func TestValidateModPolicy(t *testing.T) {
	t.Run("Valid", func(t *testing.T) {
		require.Nil(t, validateModPolicy("/foo/bar"))
	})
	t.Run("Empty", func(t *testing.T) {
		require.Regexp(t, "mod_policy not set", validateModPolicy(""))
	})
	t.Run("InvalidFirstChar", func(t *testing.T) {
		require.Regexp(t, "path element at 0 is invalid", validateModPolicy("^foo"))
	})
	t.Run("InvalidRootPath", func(t *testing.T) {
		require.Regexp(t, "path element at 0 is invalid", validateModPolicy("/"))
	})
	t.Run("InvalidSubPath", func(t *testing.T) {
		require.Regexp(t, "path element at 1 is invalid", validateModPolicy("foo//bar"))
	})
}
