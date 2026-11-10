/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"time"

	"github.com/spf13/viper"
)

// Options configures the gateway service and its request timeouts.
type Options struct {
	// Enabled controls whether the gateway service is enabled.
	Enabled bool
	// EndorsementTimeout is used to specify the maximum time to wait for endorsement responses from external peers.
	EndorsementTimeout time.Duration
	// BroadcastTimeout is used to specify the maximum time to wait for responses from ordering nodes.
	BroadcastTimeout time.Duration
	// DialTimeout is used to specify the maximum time to wait for connecting to external peers and orderer nodes.
	DialTimeout time.Duration
}

var defaultOptions = Options{
	Enabled:            true,
	EndorsementTimeout: 10 * time.Second,
	BroadcastTimeout:   10 * time.Second,
	DialTimeout:        30 * time.Second,
}

// GetOptions returns the gateway defaults with any settings present in v applied.
func GetOptions(v *viper.Viper) Options {
	options := defaultOptions
	if v.IsSet("peer.gateway.enabled") {
		options.Enabled = v.GetBool("peer.gateway.enabled")
	}
	if v.IsSet("peer.gateway.endorsementTimeout") {
		options.EndorsementTimeout = v.GetDuration("peer.gateway.endorsementTimeout")
	}
	if v.IsSet("peer.gateway.broadcastTimeout") {
		options.BroadcastTimeout = v.GetDuration("peer.gateway.broadcastTimeout")
	}
	if v.IsSet("peer.gateway.dialTimeout") {
		options.DialTimeout = v.GetDuration("peer.gateway.dialTimeout")
	}

	return options
}
