/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gateway

import (
	"time"

	"github.com/spf13/viper"
)

// GatewayOptions is used to configure the gateway settings
type Options struct {
	// GatewayEnabled is used to enable the gateway service
	Enabled            bool
	EndorsementTimeout time.Duration
}

var defaultOptions = Options{
	Enabled:            false,
	EndorsementTimeout: 10 * time.Second,
}

// DefaultOptions gets the default Gateway configuration Options
func GetOptions(v *viper.Viper) Options {
	options := defaultOptions
	if v.IsSet("peer.gateway.enabled") {
		options.Enabled = v.GetBool("peer.gateway.enabled")
	}
	if v.IsSet("peer.gateway.endorsementTimeout") {
		options.EndorsementTimeout = v.GetDuration("peer.gateway.endorsementTimeout")
	}

	return options
}
