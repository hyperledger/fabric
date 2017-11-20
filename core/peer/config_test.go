/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package peer

import (
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/hyperledger/fabric/core/comm"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestCacheConfigurationNegative(t *testing.T) {

	// set a bad peer.address
	viper.Set("peer.addressAutoDetect", true)
	viper.Set("peer.address", "testing.com")
	cacheConfiguration()
	err := CacheConfiguration()
	assert.Error(t, err, "Expected error for bad configuration")
}

func TestConfiguration(t *testing.T) {

	var ips []string
	// get the interface addresses
	if addresses, err := net.InterfaceAddrs(); err == nil {
		for _, address := range addresses {
			// eliminate loopback interfaces
			if ip, ok := address.(*net.IPNet); ok && !ip.IP.IsLoopback() {
				ips = append(ips, ip.IP.String()+":7051")
				t.Logf("found interface address [%s]", ip.IP.String())
			}
		}
	} else {
		t.Fatal("Failed to get interface addresses")
	}

	var tests = []struct {
		name             string
		settings         map[string]interface{}
		validAddresses   []string
		invalidAddresses []string
	}{
		{
			name: "test1",
			settings: map[string]interface{}{
				"peer.addressAutoDetect": false,
				"peer.address":           "testing.com:7051",
				"peer.id":                "testPeer",
			},
			validAddresses:   []string{"testing.com:7051"},
			invalidAddresses: ips,
		},
		{
			name: "test2",
			settings: map[string]interface{}{
				"peer.addressAutoDetect": true,
				"peer.address":           "testing.com:7051",
				"peer.id":                "testPeer",
			},
			validAddresses:   ips,
			invalidAddresses: []string{"testing.com:7051"},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			for k, v := range test.settings {
				viper.Set(k, v)
			}
			// reset the cache
			configurationCached = false
			// GetLocalAddress
			address, err := GetLocalAddress()
			assert.NoError(t, err, "GetLocalAddress returned unexpected error")
			assert.Contains(t, test.validAddresses, address,
				"GetLocalAddress returned unexpected address")
			assert.NotContains(t, test.invalidAddresses, address,
				"GetLocalAddress returned invalid address")
			// reset the cache
			configurationCached = false
			// GetPeerEndpoint
			pe, err := GetPeerEndpoint()
			assert.NoError(t, err, "GetPeerEndpoint returned unexpected error")
			assert.Equal(t, test.settings["peer.id"], pe.Id.Name,
				"GetPeerEndpoint returned the wrong peer ID")
			assert.Equal(t, address, pe.Address,
				"GetPeerEndpoint returned the wrong peer address")

			// now check with cached configuration
			err = CacheConfiguration()
			assert.NoError(t, err, "CacheConfiguration should not have returned an err")
			// check functions again
			// GetLocalAddress
			address, err = GetLocalAddress()
			assert.NoError(t, err, "GetLocalAddress should not have returned error")
			assert.Contains(t, test.validAddresses, address,
				"GetLocalAddress returned unexpected address")
			assert.NotContains(t, test.invalidAddresses, address,
				"GetLocalAddress returned invalid address")
			// GetPeerEndpoint
			pe, err = GetPeerEndpoint()
			assert.NoError(t, err, "GetPeerEndpoint returned unexpected error")
			assert.Equal(t, test.settings["peer.id"], pe.Id.Name,
				"GetPeerEndpoint returned the wrong peer ID")
			assert.Equal(t, address, pe.Address,
				"GetPeerEndpoint returned the wrong peer address")
		})
	}
}

func TestGetServerConfig(t *testing.T) {

	// good config without TLS
	viper.Set("peer.tls.enabled", false)
	sc, _ := GetServerConfig()
	assert.Equal(t, false, sc.SecOpts.UseTLS,
		"ServerConfig.SecOpts.UseTLS should be false")

	// keepalive options
	assert.Equal(t, comm.DefaultKeepaliveOptions(), sc.KaOpts,
		"ServerConfig.KaOpts should be set to default values")
	viper.Set("peer.keepalive.minInterval", "2m")
	sc, _ = GetServerConfig()
	assert.Equal(t, time.Duration(2)*time.Minute, sc.KaOpts.ServerMinInterval,
		"ServerConfig.KaOpts.ServerMinInterval should be set to 2 min")

	// good config with TLS
	viper.Set("peer.tls.enabled", true)
	viper.Set("peer.tls.cert.file", filepath.Join("testdata", "Org1-server1-cert.pem"))
	viper.Set("peer.tls.key.file", filepath.Join("testdata", "Org1-server1-key.pem"))
	viper.Set("peer.tls.rootcert.file", filepath.Join("testdata", "Org1-cert.pem"))
	sc, _ = GetServerConfig()
	assert.Equal(t, true, sc.SecOpts.UseTLS, "ServerConfig.SecOpts.UseTLS should be true")
	assert.Equal(t, false, sc.SecOpts.RequireClientCert,
		"ServerConfig.SecOpts.RequireClientCert should be false")
	viper.Set("peer.tls.clientAuthRequired", true)
	viper.Set("peer.tls.clientRootCAs.files",
		[]string{filepath.Join("testdata", "Org1-cert.pem"),
			filepath.Join("testdata", "Org2-cert.pem")})
	sc, _ = GetServerConfig()
	assert.Equal(t, true, sc.SecOpts.RequireClientCert,
		"ServerConfig.SecOpts.RequireClientCert should be true")
	assert.Equal(t, 2, len(sc.SecOpts.ClientRootCAs),
		"ServerConfig.SecOpts.ClientRootCAs should contain 2 entries")

	// bad config with TLS
	viper.Set("peer.tls.rootcert.file", filepath.Join("testdata", "Org11-cert.pem"))
	_, err := GetServerConfig()
	assert.Error(t, err, "GetServerConfig should return error with bad root cert path")
	viper.Set("peer.tls.cert.file", filepath.Join("testdata", "Org11-cert.pem"))
	_, err = GetServerConfig()
	assert.Error(t, err, "GetServerConfig should return error with bad tls cert path")

	// disable TLS for remaining tests
	viper.Set("peer.tls.enabled", false)
	viper.Set("peer.tls.clientAuthRequired", false)

}
