/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package peer

import (
	"bytes"
	"crypto/tls"
	"fmt"
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
		{
			name: "test3",
			settings: map[string]interface{}{
				"peer.addressAutoDetect": false,
				"peer.address":           "0.0.0.0:7051",
				"peer.id":                "testPeer",
			},
			validAddresses:   []string{fmt.Sprintf("%s:7051", GetLocalIP())},
			invalidAddresses: []string{"0.0.0.0:7051"},
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
	viper.Set("peer.connectiontimeout", "7s")
	sc, _ := GetServerConfig()
	assert.Equal(t, false, sc.SecOpts.UseTLS, "ServerConfig.SecOpts.UseTLS should be false")
	assert.Equal(t, sc.ConnectionTimeout, 7*time.Second, "ServerConfig.ConnectionTimeout should be 7 seconds")

	// keepalive options
	assert.Equal(t, comm.DefaultKeepaliveOptions, sc.KaOpts,
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

func TestGetServerRootCAs(t *testing.T) {
	var tests = []struct {
		name          string
		rootCert      string
		serverRootCAs []string
		count         int
		shouldFail    bool
	}{
		{
			name:          "no roots",
			rootCert:      "",
			serverRootCAs: []string{},
			count:         0,
		},
		{
			name:          "rootCert only",
			rootCert:      filepath.Join("testdata", "Org1-cert.pem"),
			serverRootCAs: []string{},
			count:         1,
		},
		{
			name:     "serverRootCAs only",
			rootCert: "",
			serverRootCAs: []string{
				filepath.Join("testdata", "Org2-cert.pem"),
				filepath.Join("testdata", "Org3-cert.pem"),
			},
			count: 2,
		},
		{
			name:     "rootCert and serverRootCAs",
			rootCert: filepath.Join("testdata", "Org1-cert.pem"),
			serverRootCAs: []string{
				filepath.Join("testdata", "Org2-cert.pem"),
				filepath.Join("testdata", "Org3-cert.pem"),
			},
			count: 3,
		},
		{
			name:          "bad rootCert",
			rootCert:      filepath.Join("testdata", "Org11-cert.pem"),
			serverRootCAs: []string{},
			shouldFail:    true,
		},
		{
			name:          "bad serverRootCAs",
			rootCert:      "",
			serverRootCAs: []string{filepath.Join("testdata", "Org11-cert.pem")},
			shouldFail:    true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			viper.Set("peer.tls.rootcert.file", test.rootCert)
			viper.Set("peer.tls.serverRootCAs.files", test.serverRootCAs)
			roots, err := GetServerRootCAs()
			if test.shouldFail {
				assert.Error(t, err, "Expected an error")
			} else {
				assert.NoError(t, err, "Error should not have occurred")
				assert.Equal(t, test.count, len(roots))
			}
		})
	}
}

func TestGetClientCertificate(t *testing.T) {
	viper.Set("peer.tls.key.file", "")
	viper.Set("peer.tls.cert.file", "")
	viper.Set("peer.tls.clientKey.file", "")
	viper.Set("peer.tls.clientCert.file", "")

	// neither client nor server key pairs set - expect error
	_, err := GetClientCertificate()
	assert.Error(t, err)

	viper.Set("peer.tls.key.file", "")
	viper.Set("peer.tls.cert.file",
		filepath.Join("testdata", "Org1-server1-cert.pem"))
	// missing server key file - expect error
	_, err = GetClientCertificate()
	assert.Error(t, err)

	viper.Set("peer.tls.key.file",
		filepath.Join("testdata", "Org1-server1-key.pem"))
	viper.Set("peer.tls.cert.file", "")
	// missing server cert file - expect error
	_, err = GetClientCertificate()
	assert.Error(t, err)

	// set server TLS settings to ensure we get the client TLS settings
	// when they are set properly
	viper.Set("peer.tls.key.file",
		filepath.Join("testdata", "Org1-server1-key.pem"))
	viper.Set("peer.tls.cert.file",
		filepath.Join("testdata", "Org1-server1-cert.pem"))

	// peer.tls.clientCert.file not set - expect error
	viper.Set("peer.tls.clientKey.file",
		filepath.Join("testdata", "Org2-server1-key.pem"))
	_, err = GetClientCertificate()
	assert.Error(t, err)

	// peer.tls.clientKey.file not set - expect error
	viper.Set("peer.tls.clientKey.file", "")
	viper.Set("peer.tls.clientCert.file",
		filepath.Join("testdata", "Org2-server1-cert.pem"))
	_, err = GetClientCertificate()
	assert.Error(t, err)

	// client auth required and clientKey/clientCert set
	expected, err := tls.LoadX509KeyPair(
		filepath.Join("testdata", "Org2-server1-cert.pem"),
		filepath.Join("testdata", "Org2-server1-key.pem"))
	if err != nil {
		t.Fatalf("Failed to load test certificate (%s)", err)
	}
	viper.Set("peer.tls.clientKey.file",
		filepath.Join("testdata", "Org2-server1-key.pem"))
	cert, err := GetClientCertificate()
	assert.NoError(t, err)
	assert.Equal(t, expected, cert)

	// client auth required and clientKey/clientCert not set - expect
	// client cert to be the server cert
	viper.Set("peer.tls.clientKey.file", "")
	viper.Set("peer.tls.clientCert.file", "")
	expected, err = tls.LoadX509KeyPair(
		filepath.Join("testdata", "Org1-server1-cert.pem"),
		filepath.Join("testdata", "Org1-server1-key.pem"))
	if err != nil {
		t.Fatalf("Failed to load test certificate (%s)", err)
	}
	cert, err = GetClientCertificate()
	assert.NoError(t, err)
	assert.Equal(t, expected, cert)
}

func TestGetOrdererAddressOverrides(t *testing.T) {
	conf := `
  peer:
    deliveryclient:
      addressOverrides:
        - from: myaddress0
          to: youraddress0
        - from: myaddress1
          to: youraddress1
          caCertsFile: testdata/missing.pem
        - from: myaddress2
          to: youraddress2
          caCertsFile: testdata/Org1-cert.pem`

	viper.SetConfigType("yaml")
	err := viper.ReadConfig(bytes.NewBuffer([]byte(conf)))
	if err != nil {
		t.Fatalf("Failed to read test config: %s", err)
	}

	expected := map[string]*comm.OrdererEndpoint{
		"myaddress0": {
			Address: "youraddress0",
		},
		"myaddress2": {
			Address: "youraddress2",
			PEMs: []byte(`-----BEGIN CERTIFICATE-----
MIIB8TCCAZegAwIBAgIQU59imQ+xl+FmwuiFyUgFezAKBggqhkjOPQQDAjBYMQsw
CQYDVQQGEwJVUzETMBEGA1UECBMKQ2FsaWZvcm5pYTEWMBQGA1UEBxMNU2FuIEZy
YW5jaXNjbzENMAsGA1UEChMET3JnMTENMAsGA1UEAxMET3JnMTAeFw0xNzA1MDgw
OTMwMzRaFw0yNzA1MDYwOTMwMzRaMFgxCzAJBgNVBAYTAlVTMRMwEQYDVQQIEwpD
YWxpZm9ybmlhMRYwFAYDVQQHEw1TYW4gRnJhbmNpc2NvMQ0wCwYDVQQKEwRPcmcx
MQ0wCwYDVQQDEwRPcmcxMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEFkpP6EqE
87ghFi25UWLvgPatxDiYKYaVSPvpo/XDJ0+9uUmK/C2r5Bvvxx1t8eTROwN77tEK
r+jbJIxX3ZYQMKNDMEEwDgYDVR0PAQH/BAQDAgGmMA8GA1UdJQQIMAYGBFUdJQAw
DwYDVR0TAQH/BAUwAwEB/zANBgNVHQ4EBgQEAQIDBDAKBggqhkjOPQQDAgNIADBF
AiEA1Xkrpq+wrmfVVuY12dJfMQlSx+v0Q3cYce9BE1i2mioCIAzqyduK/lHPI81b
nWiU9JF9dRQ69dEV9dxd/gzamfFU
-----END CERTIFICATE-----
`),
		},
	}
	overrides, err := GetOrdererAddressOverrides()
	if err != nil {
		t.Fatalf("Failed to get overrides: %s", err)
	}
	assert.Equal(t, expected, overrides)
}
