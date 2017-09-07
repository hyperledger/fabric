/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package capabilities

import (
	"testing"

	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/op/go-logging"
	"github.com/stretchr/testify/assert"
)

func init() {
	logging.SetLevel(logging.DEBUG, pkgLogID)
}

func TestSatisfied(t *testing.T) {
	t.Run("NilCapabilities", func(t *testing.T) {
		var capsMap map[string]*cb.Capability
		for _, provider := range []*registry{
			NewChannelProvider(capsMap).registry,
			NewOrdererProvider(capsMap).registry,
			NewApplicationProvider(capsMap).registry,
		} {
			assert.Nil(t, provider.Supported())
		}
	})

	t.Run("NotRequiredCapabilities", func(t *testing.T) {
		capsMap := map[string]*cb.Capability{
			"FakeCapability1": &cb.Capability{
				Required: false,
			},
			"FakeCapability2": &cb.Capability{
				Required: false,
			},
		}
		for _, provider := range []*registry{
			NewChannelProvider(capsMap).registry,
			NewOrdererProvider(capsMap).registry,
			NewApplicationProvider(capsMap).registry,
		} {
			assert.Nil(t, provider.Supported())
		}
	})
}

func TestNotSatisfied(t *testing.T) {
	capsMap := map[string]*cb.Capability{
		"FakeCapability": &cb.Capability{
			Required: true,
		},
	}
	for _, provider := range []*registry{
		NewChannelProvider(capsMap).registry,
		NewOrdererProvider(capsMap).registry,
		NewApplicationProvider(capsMap).registry,
	} {
		assert.Error(t, provider.Supported())
	}
}
