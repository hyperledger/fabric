/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gateway

// GatewayOptions is used to configure the gateway settings
type Options struct {
	// GatewayEnabled is used to enable the gateway service
	Enabled bool
}

// DefaultOptions gets the default Gateway configuration Options
func DefaultOptions() Options {
	return Options{
		Enabled: false,
	}
}
