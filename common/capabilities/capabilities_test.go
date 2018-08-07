/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package capabilities

import (
	"testing"

	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/stretchr/testify/assert"
)

func TestSatisfied(t *testing.T) {
	var capsMap map[string]*cb.Capability
	for _, provider := range []*registry{
		NewChannelProvider(capsMap).registry,
		NewOrdererProvider(capsMap).registry,
		NewApplicationProvider(capsMap).registry,
	} {
		assert.Nil(t, provider.Supported())
	}
}

func TestNotSatisfied(t *testing.T) {
	capsMap := map[string]*cb.Capability{
		"FakeCapability": {},
	}
	for _, provider := range []*registry{
		NewChannelProvider(capsMap).registry,
		NewOrdererProvider(capsMap).registry,
		NewApplicationProvider(capsMap).registry,
	} {
		assert.Error(t, provider.Supported())
	}
}
