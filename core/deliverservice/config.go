/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package deliverservice

import (
	"encoding/pem"
	"io/ioutil"
	"time"

	"github.com/hyperledger/fabric/core/config"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/hyperledger/fabric/internal/pkg/peer/orderers"

	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

const (
	DefaultReConnectBackoffThreshold   = time.Hour * 1
	DefaultReConnectTotalTimeThreshold = time.Second * 60 * 60
	DefaultConnectionTimeout           = time.Second * 3
)

// DeliverServiceConfig is the struct that defines the deliverservice configuration.
type DeliverServiceConfig struct {
	// PeerTLSEnabled enables/disables Peer TLS.
	PeerTLSEnabled bool
	// BlockGossipEnabled enables block forwarding via gossip
	BlockGossipEnabled bool
	// ReConnectBackoffThreshold sets the delivery service maximal delay between consencutive retries.
	ReConnectBackoffThreshold time.Duration
	// ReconnectTotalTimeThreshold sets the total time the delivery service may spend in reconnection attempts
	// until its retry logic gives up and returns an error.
	ReconnectTotalTimeThreshold time.Duration
	// ConnectionTimeout sets the delivery service <-> ordering service node connection timeout
	ConnectionTimeout time.Duration
	// Keepalive option for deliveryservice
	KeepaliveOptions comm.KeepaliveOptions
	// SecOpts provides the TLS info for connections
	SecOpts comm.SecureOptions

	// OrdererEndpointOverrides is a map of orderer addresses which should be
	// re-mapped to a different orderer endpoint.
	OrdererEndpointOverrides map[string]*orderers.Endpoint
}

type AddressOverride struct {
	From        string
	To          string
	CACertsFile string
}

// GlobalConfig obtains a set of configuration from viper, build and returns the config struct.
func GlobalConfig() *DeliverServiceConfig {
	c := &DeliverServiceConfig{}
	c.loadDeliverServiceConfig()
	return c
}

func LoadOverridesMap() (map[string]*orderers.Endpoint, error) {
	var overrides []AddressOverride
	err := viper.UnmarshalKey("peer.deliveryclient.addressOverrides", &overrides)
	if err != nil {
		return nil, errors.WithMessage(err, "could not unmarshal peer.deliveryclient.addressOverrides")
	}

	if len(overrides) == 0 {
		return nil, nil
	}

	overrideMap := map[string]*orderers.Endpoint{}
	for _, override := range overrides {
		var rootCerts [][]byte
		if override.CACertsFile != "" {
			pem, err := ioutil.ReadFile(override.CACertsFile)
			if err != nil {
				logger.Warningf("could not read file '%s' specified for caCertsFile of orderer endpoint override from '%s' to '%s': %s", override.CACertsFile, override.From, override.To, err)
				continue
			}
			rootCerts = extractCerts(pem)
			if len(rootCerts) == 0 {
				logger.Warningf("Attempted to create a cert pool for override of orderer address '%s' to '%s' but did not find any valid certs in '%s'", override.From, override.To, override.CACertsFile)
				continue
			}
		}
		overrideMap[override.From] = &orderers.Endpoint{
			Address:   override.To,
			RootCerts: rootCerts,
		}
	}

	return overrideMap, nil
}

// extractCerts is a hacky way of breaking apart a collection of PEM encoded
// certificates. This is used to preserve the semantics of
// x509.CertPool#AppendCertsFromPEM after removing the CertPool from the
// orderers.Endpoint.
func extractCerts(pemCerts []byte) [][]byte {
	var certs [][]byte
	for len(pemCerts) > 0 {
		var block *pem.Block
		block, pemCerts = pem.Decode(pemCerts)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" || len(block.Headers) != 0 {
			continue
		}
		certs = append(certs, pem.EncodeToMemory(block))
	}
	return certs
}

func (c *DeliverServiceConfig) loadDeliverServiceConfig() {
	enabledKey := "peer.deliveryclient.blockGossipEnabled"
	enabledConfigOptionMissing := !viper.IsSet(enabledKey)
	if enabledConfigOptionMissing {
		logger.Infof("peer.deliveryclient.blockGossipEnabled is not set, defaulting to true.")
	}
	c.BlockGossipEnabled = enabledConfigOptionMissing || viper.GetBool(enabledKey)

	c.PeerTLSEnabled = viper.GetBool("peer.tls.enabled")

	c.ReConnectBackoffThreshold = viper.GetDuration("peer.deliveryclient.reConnectBackoffThreshold")
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

	overridesMap, err := LoadOverridesMap()
	if err != nil {
		panic(err)
	}

	c.OrdererEndpointOverrides = overridesMap
}
