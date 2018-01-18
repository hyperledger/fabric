/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package resourcesconfig

import (
	"testing"

	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/stretchr/testify/assert"
)

const sampleChaincodeName = "cc1"

var sampleChaincodesGroup = &cb.ConfigGroup{
	Groups: map[string]*cb.ConfigGroup{
		sampleChaincodeName: sampleChaincodeGroup,
	},
}

func TestGreenChaincodesPath(t *testing.T) {
	ccsg, err := newChaincodesGroup(sampleChaincodesGroup)
	assert.NoError(t, err)
	assert.NotNil(t, ccsg)

	t.Run("GoodChaincodename", func(t *testing.T) {
		cd, ok := ccsg.ChaincodeByName(sampleChaincodeName)
		assert.True(t, ok)
		assert.NotNil(t, cd)
	})

	t.Run("BadChaincodeName", func(t *testing.T) {
		cd, ok := ccsg.ChaincodeByName("badChaincodeName")
		assert.False(t, ok)
		assert.Nil(t, cd)
	})
}

func TestChaincodesWithValues(t *testing.T) {
	ccsg, err := newChaincodesGroup(&cb.ConfigGroup{
		Values: map[string]*cb.ConfigValue{
			"foo": {},
		},
	})

	assert.Nil(t, ccsg)
	assert.Error(t, err)
	assert.Regexp(t, "chaincodes group does not support values", err.Error())
}
