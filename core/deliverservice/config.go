/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package deliverservice

import (
	"time"

	"github.com/spf13/viper"
)

const (
	DefaultReConnectBackoffThreshold   = float64(time.Hour)
	DefaultReConnectTotalTimeThreshold = time.Second * 60 * 60
	DefaultConnectionTimeout           = time.Second * 3
)

// DeliverServiceConfig is the struct that defines the deliverservice configuration.
type DeliverServiceConfig struct {
	// PeerTLSEnabled enables/disables Peer TLS.
	PeerTLSEnabled bool
	// ReConnectBackoffThreshold sets the delivery service maximal delay between consencutive retries.
	ReConnectBackoffThreshold float64
	// ReconnectTotalTimeThreshold sets the total time the delivery service may spend in reconnection attempts
	// until its retry logic gives up and returns an error.
	ReconnectTotalTimeThreshold time.Duration
	// ConnectionTimeout sets the delivery service <-> ordering service node connection timeout
	ConnectionTimeout time.Duration
}

// GlobalConfig obtains a set of configuration from viper, build and returns the config struct.
func GlobalConfig() *DeliverServiceConfig {
	c := &DeliverServiceConfig{}
	c.loadDeliverServiceConfig()
	return c
}

func (c *DeliverServiceConfig) loadDeliverServiceConfig() {
	c.PeerTLSEnabled = viper.GetBool("peer.tls.enabled")

	c.ReConnectBackoffThreshold = viper.GetFloat64("peer.deliveryclient.reConnectBackoffThreshold")
	if c.ReConnectBackoffThreshold == 0 {
		c.ReConnectBackoffThreshold = DefaultReConnectBackoffThreshold
	}

	c.ReconnectTotalTimeThreshold = viper.GetDuration("peer.deliveryclient.reconnectTotalTimeThreshold")
	if c.ReconnectTotalTimeThreshold == 0 {
		c.ReconnectTotalTimeThreshold = DefaultReConnectTotalTimeThreshold
	}

	c.ConnectionTimeout = viper.GetDuration("peer.deliveryclient.connTimeout")
	if c.ConnectionTimeout == 0 {
		c.ConnectionTimeout = DefaultConnectionTimeout
	}
}
