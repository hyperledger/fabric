/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package token_test

import (
	"testing"

	"github.com/hyperledger/fabric/token"
	"github.com/stretchr/testify/assert"
)

func TestFabTokenProcessor_GenerateSimulationResults(t *testing.T) {
	p := &token.TxProcessor{}
	assert.NotNil(t, p)

	// GenerateSimulationResults returns always an error, no matter what
	err := p.GenerateSimulationResults(nil, nil, false)
	assert.Error(t, err)
	assert.Equal(t, "implement me", err.Error())
}
