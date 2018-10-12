/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package policies

import (
	"fmt"
	"reflect"
	"strconv"
	"testing"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/msp"
	"github.com/stretchr/testify/assert"
)

type mockProvider struct{}

func (mpp mockProvider) NewPolicy(data []byte) (Policy, proto.Message, error) {
	return nil, nil, nil
}

const mockType = int32(0)

func defaultProviders() map[int32]Provider {
	providers := make(map[int32]Provider)
	providers[mockType] = &mockProvider{}
	return providers
}

func TestUnnestedManager(t *testing.T) {
	config := &cb.ConfigGroup{
		Policies: map[string]*cb.ConfigPolicy{
			"1": {Policy: &cb.Policy{Type: mockType}},
			"2": {Policy: &cb.Policy{Type: mockType}},
			"3": {Policy: &cb.Policy{Type: mockType}},
		},
	}

	m, err := NewManagerImpl("test", defaultProviders(), config)
	assert.NoError(t, err)
	assert.NotNil(t, m)

	_, ok := m.Manager([]string{"subGroup"})
	assert.False(t, ok, "Should not have found a subgroup manager")

	r, ok := m.Manager([]string{})
	assert.True(t, ok, "Should have found the root manager")
	assert.Equal(t, m, r)

	assert.Len(t, m.policies, len(config.Policies))

	for policyName := range config.Policies {
		_, ok := m.GetPolicy(policyName)
		assert.True(t, ok, "Should have found policy %s", policyName)
	}
}

func TestNestedManager(t *testing.T) {
	config := &cb.ConfigGroup{
		Policies: map[string]*cb.ConfigPolicy{
			"n0a": {Policy: &cb.Policy{Type: mockType}},
			"n0b": {Policy: &cb.Policy{Type: mockType}},
			"n0c": {Policy: &cb.Policy{Type: mockType}},
		},
		Groups: map[string]*cb.ConfigGroup{
			"nest1": {
				Policies: map[string]*cb.ConfigPolicy{
					"n1a": {Policy: &cb.Policy{Type: mockType}},
					"n1b": {Policy: &cb.Policy{Type: mockType}},
					"n1c": {Policy: &cb.Policy{Type: mockType}},
				},
				Groups: map[string]*cb.ConfigGroup{
					"nest2a": {
						Policies: map[string]*cb.ConfigPolicy{
							"n2a_1": {Policy: &cb.Policy{Type: mockType}},
							"n2a_2": {Policy: &cb.Policy{Type: mockType}},
							"n2a_3": {Policy: &cb.Policy{Type: mockType}},
						},
					},
					"nest2b": {
						Policies: map[string]*cb.ConfigPolicy{
							"n2b_1": {Policy: &cb.Policy{Type: mockType}},
							"n2b_2": {Policy: &cb.Policy{Type: mockType}},
							"n2b_3": {Policy: &cb.Policy{Type: mockType}},
						},
					},
				},
			},
		},
	}

	m, err := NewManagerImpl("nest0", defaultProviders(), config)
	assert.NoError(t, err)
	assert.NotNil(t, m)

	r, ok := m.Manager([]string{})
	assert.True(t, ok, "Should have found the root manager")
	assert.Equal(t, m, r)

	n1, ok := m.Manager([]string{"nest1"})
	assert.True(t, ok)
	n2a, ok := m.Manager([]string{"nest1", "nest2a"})
	assert.True(t, ok)
	n2b, ok := m.Manager([]string{"nest1", "nest2b"})
	assert.True(t, ok)

	n2as, ok := n1.Manager([]string{"nest2a"})
	assert.True(t, ok)
	assert.Equal(t, n2a, n2as)
	n2bs, ok := n1.Manager([]string{"nest2b"})
	assert.True(t, ok)
	assert.Equal(t, n2b, n2bs)

	absPrefix := PathSeparator + "nest0" + PathSeparator
	for policyName := range config.Policies {
		_, ok := m.GetPolicy(policyName)
		assert.True(t, ok, "Should have found policy %s", policyName)

		absName := absPrefix + policyName
		_, ok = m.GetPolicy(absName)
		assert.True(t, ok, "Should have found absolute policy %s", absName)
	}

	for policyName := range config.Groups["nest1"].Policies {
		_, ok := n1.GetPolicy(policyName)
		assert.True(t, ok, "Should have found policy %s", policyName)

		relPathFromBase := "nest1" + PathSeparator + policyName
		_, ok = m.GetPolicy(relPathFromBase)
		assert.True(t, ok, "Should have found policy %s", policyName)

		for i, abs := range []Manager{n1, m} {
			absName := absPrefix + relPathFromBase
			_, ok = abs.GetPolicy(absName)
			assert.True(t, ok, "Should have found absolutely policy for manager %d", i)
		}
	}

	for policyName := range config.Groups["nest1"].Groups["nest2a"].Policies {
		_, ok := n2a.GetPolicy(policyName)
		assert.True(t, ok, "Should have found policy %s", policyName)

		relPathFromN1 := "nest2a" + PathSeparator + policyName
		_, ok = n1.GetPolicy(relPathFromN1)
		assert.True(t, ok, "Should have found policy %s", policyName)

		relPathFromBase := "nest1" + PathSeparator + relPathFromN1
		_, ok = m.GetPolicy(relPathFromBase)
		assert.True(t, ok, "Should have found policy %s", policyName)

		for i, abs := range []Manager{n2a, n1, m} {
			absName := absPrefix + relPathFromBase
			_, ok = abs.GetPolicy(absName)
			assert.True(t, ok, "Should have found absolutely policy for manager %d", i)
		}
	}

	for policyName := range config.Groups["nest1"].Groups["nest2b"].Policies {
		_, ok := n2b.GetPolicy(policyName)
		assert.True(t, ok, "Should have found policy %s", policyName)

		relPathFromN1 := "nest2b" + PathSeparator + policyName
		_, ok = n1.GetPolicy(relPathFromN1)
		assert.True(t, ok, "Should have found policy %s", policyName)

		relPathFromBase := "nest1" + PathSeparator + relPathFromN1
		_, ok = m.GetPolicy(relPathFromBase)
		assert.True(t, ok, "Should have found policy %s", policyName)

		for i, abs := range []Manager{n2b, n1, m} {
			absName := absPrefix + relPathFromBase
			_, ok = abs.GetPolicy(absName)
			assert.True(t, ok, "Should have found absolutely policy for manager %d", i)
		}
	}
}

func TestPrincipalUniqueSet(t *testing.T) {
	var principalSet PrincipalSet
	addPrincipal := func(i int) {
		principalSet = append(principalSet, &msp.MSPPrincipal{
			PrincipalClassification: msp.MSPPrincipal_Classification(i),
			Principal:               []byte(fmt.Sprintf("%d", i)),
		})
	}

	addPrincipal(1)
	addPrincipal(2)
	addPrincipal(2)
	addPrincipal(3)
	addPrincipal(3)
	addPrincipal(3)

	for principal, plurality := range principalSet.UniqueSet() {
		assert.Equal(t, int(principal.PrincipalClassification), plurality)
		assert.Equal(t, fmt.Sprintf("%d", plurality), string(principal.Principal))
	}

	v := reflect.Indirect(reflect.ValueOf(msp.MSPPrincipal{}))
	// Ensure msp.MSPPrincipal has only 2 fields.
	// This is essential for 'UniqueSet' to work properly
	// XXX This is a rather brittle check and brittle way to fix the test
	// There seems to be an assumption that the number of fields in the proto
	// struct matches the number of fields in the proto message
	assert.Equal(t, 5, v.NumField())
}

func TestPrincipalSetContainingOnly(t *testing.T) {
	var principalSets PrincipalSets
	var principalSet PrincipalSet
	for j := 0; j < 3; j++ {
		for i := 0; i < 10; i++ {
			principalSet = append(principalSet, &msp.MSPPrincipal{
				PrincipalClassification: msp.MSPPrincipal_IDENTITY,
				Principal:               []byte(fmt.Sprintf("%d", j*10+i)),
			})
		}
		principalSets = append(principalSets, principalSet)
		principalSet = nil
	}

	between20And30 := func(principal *msp.MSPPrincipal) bool {
		n, _ := strconv.ParseInt(string(principal.Principal), 10, 32)
		return n >= 20 && n <= 29
	}

	principalSets = principalSets.ContainingOnly(between20And30)

	assert.Len(t, principalSets, 1)
	assert.True(t, principalSets[0].ContainingOnly(between20And30))
}
