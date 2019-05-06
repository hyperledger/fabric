/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scc

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsWhitelisted(t *testing.T) {
	p := Provider{
		Whitelist: map[string]bool{"InWhiteList": true},
	}
	positiveOutput := p.isWhitelisted(&SysCCWrapper{
		SCC: &SystemChaincode{
			Name:              "InWhiteList",
			InvokableExternal: true,
			InvokableCC2CC:    false,
			Enabled:           true,
		},
	})
	assert.True(t, positiveOutput)

	negativeOutput := p.isWhitelisted(&SysCCWrapper{
		SCC: &SystemChaincode{
			Name:              "NotWhiteList",
			InvokableExternal: true,
			InvokableCC2CC:    false,
			Enabled:           true,
		},
	})
	assert.False(t, negativeOutput)

}
