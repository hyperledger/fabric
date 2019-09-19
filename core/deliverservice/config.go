/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package deliverservice

import (
	"io/ioutil"
	"time"

	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/core/config"

	"github.com/pkg/errors"
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
	// Keepalive option for deliveryservice
	KeepaliveOptions comm.KeepaliveOptions
	// SecOpts provides the TLS info for connections
	SecOpts comm.SecureOptions
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

	c.KeepaliveOptions = comm.DefaultKeepaliveOptions
	if viper.IsSet("peer.keepalive.deliveryClient.interval") {
		c.KeepaliveOptions.ClientInterval = viper.GetDuration("peer.keepalive.deliveryClient.interval")
	}
	if viper.IsSet("peer.keepalive.deliveryClient.timeout") {
		c.KeepaliveOptions.ClientTimeout = viper.GetDuration("peer.keepalive.deliveryClient.timeout")
	}

	c.SecOpts = comm.SecureOptions{
		UseTLS:            viper.GetBool("peer.tls.enabled"),
		RequireClientCert: viper.GetBool("peer.tls.clientAuthRequired"),
	}

	if c.SecOpts.RequireClientCert {
		certFile := config.GetPath("peer.tls.clientCert.file")
		if certFile == "" {
			certFile = config.GetPath("peer.tls.cert.file")
		}

		keyFile := config.GetPath("peer.tls.clientKey.file")
		if keyFile == "" {
			keyFile = config.GetPath("peer.tls.key.file")
		}

		keyPEM, err := ioutil.ReadFile(keyFile)
		if err != nil {
			panic(errors.WithMessagef(err, "unable to load key at '%s'", keyFile))
		}
		c.SecOpts.Key = keyPEM
		certPEM, err := ioutil.ReadFile(certFile)
		if err != nil {
			panic(errors.WithMessagef(err, "unable to load cert at '%s'", certFile))
		}
		c.SecOpts.Certificate = certPEM
	}
}
