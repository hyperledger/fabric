/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"github.com/spf13/viper"
)

var (
	// Is the configuration cached?
	configurationCached = false
	// Is TLS enabled
	tlsEnabled bool
	// Max send and receive bytes for grpc clients and servers
	maxRecvMsgSize = 100 * 1024 * 1024
	maxSendMsgSize = 100 * 1024 * 1024
)

// cacheConfiguration caches common package scoped variables
func cacheConfiguration() {
	if !configurationCached {
		tlsEnabled = viper.GetBool("peer.tls.enabled")
		configurationCached = true
	}
}

// TLSEnabled return cached value for "peer.tls.enabled" configuration value
func TLSEnabled() bool {
	if !configurationCached {
		cacheConfiguration()
	}
	return tlsEnabled
}

// MaxRecvMsgSize returns the maximum message size in bytes that gRPC clients
// and servers can receive
func MaxRecvMsgSize() int {
	return maxRecvMsgSize
}

// SetMaxRecvMsgSize sets the maximum message size in bytes that gRPC clients
// and servers can receive
func SetMaxRecvMsgSize(size int) {
	maxRecvMsgSize = size
}

// MaxSendMsgSize returns the maximum message size in bytes that gRPC clients
// and servers can send
func MaxSendMsgSize() int {
	return maxSendMsgSize
}

// SetMaxSendMsgSize sets the maximum message size in bytes that gRPC clients
// and servers can send
func SetMaxSendMsgSize(size int) {
	maxSendMsgSize = size
}
