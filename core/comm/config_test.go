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
	// check the defaults
	assert.EqualValues(t, maxRecvMsgSize, MaxRecvMsgSize())
	assert.EqualValues(t, maxSendMsgSize, MaxSendMsgSize())
	assert.EqualValues(t, false, TLSEnabled())
	assert.EqualValues(t, true, configurationCached)

	// set send/recv msg sizes
	size := 10 * 1024 * 1024
	SetMaxRecvMsgSize(size)
	SetMaxSendMsgSize(size)
	assert.EqualValues(t, size, MaxRecvMsgSize())
	assert.EqualValues(t, size, MaxSendMsgSize())

	// set keepalive options
	timeout := 1000
	ka := KeepaliveOptions{
		ClientKeepaliveTime:    timeout,
		ClientKeepaliveTimeout: timeout + 1,
		ServerKeepaliveTime:    timeout + 2,
		ServerKeepaliveTimeout: timeout + 3,
	}
	SetKeepaliveOptions(ka)
	assert.EqualValues(t, timeout, keepaliveOptions.ClientKeepaliveTime)
	assert.EqualValues(t, timeout+1, keepaliveOptions.ClientKeepaliveTimeout)
	assert.EqualValues(t, timeout+2, keepaliveOptions.ServerKeepaliveTime)
	assert.EqualValues(t, timeout+3, keepaliveOptions.ServerKeepaliveTimeout)
	assert.EqualValues(t, 2, len(ServerKeepaliveOptions()))
	assert.Equal(t, 1, len(ClientKeepaliveOptions()))

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
