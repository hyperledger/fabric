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
	"testing"

	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/golang/protobuf/proto"
	logging "github.com/op/go-logging"
	"github.com/stretchr/testify/assert"
)

func init() {
	logging.SetLevel(logging.DEBUG, "")
}

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
	m := NewManagerImpl("test", defaultProviders())
	handlers, err := m.BeginPolicyProposals(t, []string{})
	assert.NoError(t, err)
	assert.Empty(t, handlers, "Should not have returned additional handlers")

	policyNames := []string{"1", "2", "3"}

	for _, policyName := range policyNames {
		_, err := m.ProposePolicy(t, policyName, &cb.ConfigPolicy{Policy: &cb.Policy{Type: mockType}})
		assert.NoError(t, err)
	}

	m.CommitProposals(t)

	_, ok := m.Manager([]string{"subGroup"})
	assert.False(t, ok, "Should not have found a subgroup manager")

	r, ok := m.Manager([]string{})
	assert.True(t, ok, "Should have found the root manager")
	assert.Equal(t, m, r)

	for _, policyName := range policyNames {
		_, ok := m.GetPolicy(policyName)
		assert.True(t, ok, "Should have found policy %s", policyName)
	}
}

func TestNestedManager(t *testing.T) {
	m := NewManagerImpl("test", defaultProviders())
	absPrefix := "/test/"
	nesting1, err := m.BeginPolicyProposals(t, []string{"nest1"})
	assert.NoError(t, err)
	assert.Len(t, nesting1, 1, "Should not have returned exactly one additional manager")

	nesting2, err := nesting1[0].BeginPolicyProposals(t, []string{"nest2a", "nest2b"})
	assert.NoError(t, err)
	assert.Len(t, nesting2, 2, "Should not have returned two one additional managers")

	_, err = nesting2[0].BeginPolicyProposals(t, []string{})
	assert.NoError(t, err)
	_, err = nesting2[1].BeginPolicyProposals(t, []string{})
	assert.NoError(t, err)

	policyNames := []string{"n0a", "n0b", "n0c"}
	for _, policyName := range policyNames {
		_, err := m.ProposePolicy(t, policyName, &cb.ConfigPolicy{Policy: &cb.Policy{Type: mockType}})
		assert.NoError(t, err)
	}

	n1PolicyNames := []string{"n1a", "n1b", "n1c"}
	for _, policyName := range n1PolicyNames {
		_, err := nesting1[0].ProposePolicy(t, policyName, &cb.ConfigPolicy{Policy: &cb.Policy{Type: mockType}})
		assert.NoError(t, err)
	}

	n2aPolicyNames := []string{"n2a_1", "n2a_2", "n2a_3"}
	for _, policyName := range n2aPolicyNames {
		_, err := nesting2[0].ProposePolicy(t, policyName, &cb.ConfigPolicy{Policy: &cb.Policy{Type: mockType}})
		assert.NoError(t, err)
	}

	n2bPolicyNames := []string{"n2b_1", "n2b_2", "n2b_3"}
	for _, policyName := range n2bPolicyNames {
		_, err := nesting2[1].ProposePolicy(t, policyName, &cb.ConfigPolicy{Policy: &cb.Policy{Type: mockType}})
		assert.NoError(t, err)
	}

	nesting2[0].CommitProposals(t)
	nesting2[1].CommitProposals(t)
	nesting1[0].CommitProposals(t)
	m.CommitProposals(t)

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

	for _, policyName := range policyNames {
		_, ok := m.GetPolicy(policyName)
		assert.True(t, ok, "Should have found policy %s", policyName)

		absName := absPrefix + policyName
		_, ok = m.GetPolicy(absName)
		assert.True(t, ok, "Should have found absolute policy %s", absName)
	}

	for _, policyName := range n1PolicyNames {
		_, ok := n1.GetPolicy(policyName)
		assert.True(t, ok, "Should have found policy %s", policyName)

		_, ok = m.GetPolicy(n1.BasePath() + "/" + policyName)
		assert.True(t, ok, "Should have found policy %s", policyName)

		for i, abs := range []Manager{n1, m} {
			absName := absPrefix + n1.BasePath() + "/" + policyName
			_, ok = abs.GetPolicy(absName)
			assert.True(t, ok, "Should have found absolutely policy for manager %d", i)
		}
	}

	for _, policyName := range n2aPolicyNames {
		_, ok := n2a.GetPolicy(policyName)
		assert.True(t, ok, "Should have found policy %s", policyName)

		_, ok = n1.GetPolicy(n2a.BasePath() + "/" + policyName)
		assert.True(t, ok, "Should have found policy %s", policyName)

		_, ok = m.GetPolicy(n1.BasePath() + "/" + n2a.BasePath() + "/" + policyName)
		assert.True(t, ok, "Should have found policy %s", policyName)

		for i, abs := range []Manager{n2a, n1, m} {
			absName := absPrefix + n1.BasePath() + "/" + n2a.BasePath() + "/" + policyName
			_, ok = abs.GetPolicy(absName)
			assert.True(t, ok, "Should have found absolutely policy for manager %d", i)
		}
	}

	for _, policyName := range n2bPolicyNames {
		_, ok := n2b.GetPolicy(policyName)
		assert.True(t, ok, "Should have found policy %s", policyName)

		_, ok = n1.GetPolicy(n2b.BasePath() + "/" + policyName)
		assert.True(t, ok, "Should have found policy %s", policyName)

		_, ok = m.GetPolicy(n1.BasePath() + "/" + n2b.BasePath() + "/" + policyName)
		assert.True(t, ok, "Should have found policy %s", policyName)

		for i, abs := range []Manager{n2b, n1, m} {
			absName := absPrefix + n1.BasePath() + "/" + n2b.BasePath() + "/" + policyName
			_, ok = abs.GetPolicy(absName)
			assert.True(t, ok, "Should have found absolutely policy for manager %d", i)
		}
	}
}
