/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestConfig(t *testing.T) {
	t.Parallel()
	// check the defaults
	assert.EqualValues(t, keepaliveOptions, DefaultKeepaliveOptions())
	assert.EqualValues(t, false, TLSEnabled())
	assert.EqualValues(t, true, configurationCached)

	// reset cache
	configurationCached = false
	viper.Set("peer.tls.enabled", true)
	assert.EqualValues(t, true, TLSEnabled())
	// check that value is cached
	viper.Set("peer.tls.enabled", false)
	assert.NotEqual(t, false, TLSEnabled())
	// reset tls
	configurationCached = false
	viper.Set("peer.tls.enabled", false)
}
