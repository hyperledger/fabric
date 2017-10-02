/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package resourcesconfig

import (
	"fmt"
	"testing"

	"github.com/hyperledger/fabric/common/channelconfig"
	mockpolicies "github.com/hyperledger/fabric/common/mocks/policies"
	"github.com/hyperledger/fabric/common/policies"

	"github.com/stretchr/testify/assert"
)

func TestInterface(t *testing.T) {
	_ = policies.Manager(&policyRouter{})
}

func TestGetPolicy(t *testing.T) {
	pr := &policyRouter{
		channelPolicyManager: &mockpolicies.Manager{
			Policy: &mockpolicies.Policy{
				Err: fmt.Errorf("channel policy err"),
			},
		},
		resourcesPolicyManager: &mockpolicies.Manager{
			Policy: &mockpolicies.Policy{
				Err: fmt.Errorf("resource policy err"),
			},
		},
	}

	t.Run("Channel policy", func(t *testing.T) {
		policy, ok := pr.GetPolicy(fmt.Sprintf("/%s/foo", channelconfig.RootGroupKey))
		assert.True(t, ok)
		assert.Equal(t, pr.channelPolicyManager.(*mockpolicies.Manager).Policy, policy)
	})

	t.Run("Resource policy", func(t *testing.T) {
		policy, ok := pr.GetPolicy(fmt.Sprintf("/%s/foo", RootGroupKey))
		assert.True(t, ok)
		assert.Equal(t, pr.resourcesPolicyManager.(*mockpolicies.Manager).Policy, policy)
	})

	t.Run("Unknown policy", func(t *testing.T) {
		_, ok := pr.GetPolicy(fmt.Sprintf("/bar/foo"))
		assert.False(t, ok)
	})
}

func TestManagerFailures(t *testing.T) {
	pr := &policyRouter{}

	t.Run("Empty path", func(t *testing.T) {
		m, ok := pr.Manager(nil)
		assert.True(t, ok)
		assert.Equal(t, pr, m)
	})

	t.Run("Bad path", func(t *testing.T) {
		_, ok := pr.Manager([]string{"foo"})
		assert.False(t, ok)
	})

	t.Run("Missing channel path", func(t *testing.T) {
		_, ok := pr.Manager([]string{channelconfig.RootGroupKey})
		assert.False(t, ok)
	})

	t.Run("Missing resources path", func(t *testing.T) {
		_, ok := pr.Manager([]string{RootGroupKey})
		assert.False(t, ok)
	})
}

func TestManagerSuccess(t *testing.T) {
	pr := &policyRouter{
		channelPolicyManager: &mockpolicies.Manager{
			Policy: &mockpolicies.Policy{
				Err: fmt.Errorf("channel policy err"),
			},
		},
		resourcesPolicyManager: &mockpolicies.Manager{
			Policy: &mockpolicies.Policy{
				Err: fmt.Errorf("resource policy err"),
			},
		},
	}

	t.Run("Channel path", func(t *testing.T) {
		m, ok := pr.Manager([]string{channelconfig.RootGroupKey})
		assert.True(t, ok)
		assert.Equal(t, m, pr.channelPolicyManager)
	})

	t.Run("Resources path", func(t *testing.T) {
		m, ok := pr.Manager([]string{RootGroupKey})
		assert.True(t, ok)
		assert.Equal(t, m, pr.resourcesPolicyManager)
	})
}
