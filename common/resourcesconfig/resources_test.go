/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package resourcesconfig

import (
	"testing"

	cb "github.com/hyperledger/fabric/protos/common"

	logging "github.com/op/go-logging"
	"github.com/stretchr/testify/assert"
)

func init() {
	logging.SetLevel(logging.DEBUG, "")
}

var sampleResourceGroup = &cb.ConfigGroup{
	Groups: map[string]*cb.ConfigGroup{
		APIsGroupKey:         sampleAPIsGroup,
		PeerPoliciesGroupKey: samplePeerPoliciesGroup,
		ChaincodesGroupKey:   sampleChaincodesGroup,
	},
}

func TestGreenPathResourceGroup(t *testing.T) {
	rg, err := newResourceGroup(sampleResourceGroup)
	assert.NoError(t, err)
	assert.NotNil(t, rg)
}

func TestBadPathResourceGroup(t *testing.T) {
	t.Run("BadValues", func(t *testing.T) {
		rg, err := newResourceGroup(&cb.ConfigGroup{
			Values: map[string]*cb.ConfigValue{
				"foo": {},
			},
		})
		assert.Nil(t, rg)
		assert.Error(t, err)
		assert.Regexp(t, "/Resources group does not support any values", err.Error())
	})

	t.Run("BadSubGroup", func(t *testing.T) {
		rg, err := newResourceGroup(&cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{
				"bar": {},
			},
		})
		assert.Nil(t, rg)
		assert.Error(t, err)
		assert.Regexp(t, "unknown sub-group", err.Error())
	})
}
