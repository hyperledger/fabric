/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package peer

import (
	"crypto/tls"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/hyperledger/fabric/internal/pkg/gateway/config"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestCacheConfigurationNegative(t *testing.T) {
	// set a bad peer.address
	viper.Set("peer.addressAutoDetect", true)
	viper.Set("peer.address", "testing.com")
	_, err := GlobalConfig()
	require.Error(t, err, "Expected error for bad configuration")

	viper.Set("peer.addressAutoDetect", false)
	viper.Set("peer.address", "")
	_, err = GlobalConfig()
	require.Error(t, err, "Expected error for bad configuration")

	viper.Set("peer.address", "wrongAddress")
	_, err = GlobalConfig()
	require.Error(t, err, "Expected error for bad configuration")
}

func TestPeerAddress(t *testing.T) {
	localIP, err := getLocalIP()
	require.NoError(t, err)

	tests := []struct {
		name                string
		settings            map[string]interface{}
		expectedPeerAddress string
	}{
		{
			name: "test1",
			settings: map[string]interface{}{
				"peer.addressAutoDetect": false,
				"peer.address":           "testing.com:7051",
			},
			expectedPeerAddress: "testing.com:7051",
		},
		{
			name: "test2",
			settings: map[string]interface{}{
				"peer.addressAutoDetect": true,
				"peer.address":           "testing.com:7051",
			},
			expectedPeerAddress: net.JoinHostPort(localIP, "7051"),
		},
		{
			name: "test3",
			settings: map[string]interface{}{
				"peer.addressAutoDetect": false,
				"peer.address":           "0.0.0.0:7051",
			},
			expectedPeerAddress: net.JoinHostPort(localIP, "7051"),
		},
		{
			name: "test4",
			settings: map[string]interface{}{
				"peer.addressAutoDetect": true,
				"peer.address":           "127.0.0.1:7051",
			},
			expectedPeerAddress: net.JoinHostPort(localIP, "7051"),
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			for k, v := range test.settings {
				viper.Set(k, v)
			}
			c, err := GlobalConfig()
			require.NoError(t, err, "GlobalConfig returned unexpected error")
			require.Equal(t, test.expectedPeerAddress, c.PeerAddress)
		})
	}
}

func TestGetServerConfig(t *testing.T) {
	tempdir := t.TempDir()

	// good config without TLS
	viper.Set("peer.tls.enabled", false)
	viper.Set("peer.connectiontimeout", "7s")
	sc, err := GetServerConfig()
	require.NoError(t, err)
	require.Equal(t, false, sc.SecOpts.UseTLS, "ServerConfig.SecOpts.UseTLS should be false")
	require.Equal(t, sc.ConnectionTimeout, 7*time.Second, "ServerConfig.ConnectionTimeout should be 7 seconds")

	// keepalive options
	require.Equal(t, comm.DefaultKeepaliveOptions, sc.KaOpts, "ServerConfig.KaOpts should be set to default values")
	viper.Set("peer.keepalive.interval", "60m")
	sc, _ = GetServerConfig()
	require.Equal(t, time.Duration(60)*time.Minute, sc.KaOpts.ServerInterval, "ServerConfig.KaOpts.ServerInterval should be set to 60 min")
	viper.Set("peer.keepalive.timeout", "30s")
	sc, _ = GetServerConfig()
	require.Equal(t, time.Duration(30)*time.Second, sc.KaOpts.ServerTimeout, "ServerConfig.KaOpts.ServerTimeout should be set to 30 sec")
	viper.Set("peer.keepalive.minInterval", "2m")
	sc, _ = GetServerConfig()
	require.Equal(t, time.Duration(2)*time.Minute, sc.KaOpts.ServerMinInterval, "ServerConfig.KaOpts.ServerMinInterval should be set to 2 min")

	// good config with TLS
	org1CA, err := tlsgen.NewCA()
	require.NoError(t, err)
	err = ioutil.WriteFile(filepath.Join(tempdir, "org1-ca-cert.pem"), org1CA.CertBytes(), 0o644)
	require.NoError(t, err)
	org2CA, err := tlsgen.NewCA()
	require.NoError(t, err)
	err = ioutil.WriteFile(filepath.Join(tempdir, "org2-ca-cert.pem"), org2CA.CertBytes(), 0o644)
	require.NoError(t, err)

	org1ServerKP, err := org1CA.NewServerCertKeyPair("localhost")
	require.NoError(t, err)
	err = ioutil.WriteFile(filepath.Join(tempdir, "org1-server1-cert.pem"), org1ServerKP.Cert, 0o644)
	require.NoError(t, err)
	err = ioutil.WriteFile(filepath.Join(tempdir, "org1-server1-key.pem"), org1ServerKP.Key, 0o600)
	require.NoError(t, err)

	viper.Set("peer.tls.enabled", true)
	viper.Set("peer.tls.cert.file", filepath.Join(tempdir, "org1-server1-cert.pem"))
	viper.Set("peer.tls.key.file", filepath.Join(tempdir, "org1-server1-key.pem"))
	viper.Set("peer.tls.rootcert.file", filepath.Join(tempdir, "org1-ca-cert.pem"))

	sc, err = GetServerConfig()
	require.NoError(t, err, "failed to build server config")
	require.Equal(t, true, sc.SecOpts.UseTLS, "ServerConfig.SecOpts.UseTLS should be true")
	require.Equal(t, false, sc.SecOpts.RequireClientCert, "ServerConfig.SecOpts.RequireClientCert should be false")
	viper.Set("peer.tls.clientAuthRequired", true)
	viper.Set("peer.tls.clientRootCAs.files", []string{
		filepath.Join(tempdir, "org1-ca-cert.pem"),
		filepath.Join(tempdir, "org2-ca-cert.pem"),
	})
	sc, _ = GetServerConfig()
	require.Equal(t, true, sc.SecOpts.RequireClientCert, "ServerConfig.SecOpts.RequireClientCert should be true")
	require.Equal(t, 2, len(sc.SecOpts.ClientRootCAs), "ServerConfig.SecOpts.ClientRootCAs should contain 2 entries")

	// GRPC max message size options
	require.Equal(t, comm.DefaultMaxRecvMsgSize, sc.MaxRecvMsgSize, "ServerConfig.MaxRecvMsgSize should be set to default value %v", comm.DefaultMaxRecvMsgSize)
	require.Equal(t, comm.DefaultMaxSendMsgSize, sc.MaxSendMsgSize, "ServerConfig.MaxSendMsgSize should be set to default value %v", comm.DefaultMaxSendMsgSize)
	viper.Set("peer.maxRecvMsgSize", "1024")
	viper.Set("peer.maxSendMsgSize", "1024")
	sc, _ = GetServerConfig()
	require.Equal(t, 1024, sc.MaxRecvMsgSize, "ServerConfig.MaxRecvMsgSize should be set to custom value 1024")
	require.Equal(t, 1024, sc.MaxSendMsgSize, "ServerConfig.MaxSendMsgSize should be set to custom value 1024")

	// bad config with TLS
	viper.Set("peer.tls.rootcert.file", "non-existent-file.pem")
	_, err = GetServerConfig()
	require.Error(t, err, "GetServerConfig should return error with bad root cert path")

	viper.Set("peer.tls.rootcert.file", filepath.Join(tempdir, "org1-ca-cert.pem"))
	viper.Set("peer.tls.cert.file", "non-existent-file.pem")
	_, err = GetServerConfig()
	require.Error(t, err, "GetServerConfig should return error with bad tls cert path")

	// disable TLS for remaining tests
	viper.Set("peer.tls.enabled", false)
	viper.Set("peer.tls.clientAuthRequired", false)
}

func TestGetClientCertificate(t *testing.T) {
	tempdir := t.TempDir()

	ca, err := tlsgen.NewCA()
	require.NoError(t, err)
	kp, err := ca.NewServerCertKeyPair("localhost")
	require.NoError(t, err)
	err = ioutil.WriteFile(filepath.Join(tempdir, "server1-cert.pem"), kp.Cert, 0o644)
	require.NoError(t, err)
	err = ioutil.WriteFile(filepath.Join(tempdir, "server1-key.pem"), kp.Key, 0o600)
	require.NoError(t, err)

	viper.Set("peer.tls.key.file", "")
	viper.Set("peer.tls.cert.file", "")
	viper.Set("peer.tls.clientKey.file", "")
	viper.Set("peer.tls.clientCert.file", "")

	// neither client nor server key pairs set - expect error
	_, err = GetClientCertificate()
	require.Error(t, err)

	viper.Set("peer.tls.key.file", "")
	viper.Set("peer.tls.cert.file", filepath.Join(tempdir, "server1-cert.pem"))
	// missing server key file - expect error
	_, err = GetClientCertificate()
	require.Error(t, err)

	viper.Set("peer.tls.key.file", filepath.Join(tempdir, "server1-key.pem"))
	viper.Set("peer.tls.cert.file", "")
	// missing server cert file - expect error
	_, err = GetClientCertificate()
	require.Error(t, err)

	// set server TLS settings to ensure we get the client TLS settings
	// when they are set properly
	viper.Set("peer.tls.key.file", filepath.Join(tempdir, "server1-key.pem"))
	viper.Set("peer.tls.cert.file", filepath.Join(tempdir, "server1-cert.pem"))

	// peer.tls.clientCert.file not set - expect error
	viper.Set("peer.tls.clientKey.file", filepath.Join(tempdir, "server1-key.pem"))
	_, err = GetClientCertificate()
	require.Error(t, err)

	// peer.tls.clientKey.file not set - expect error
	viper.Set("peer.tls.clientKey.file", "")
	viper.Set("peer.tls.clientCert.file", filepath.Join(tempdir, "server1-cert.pem"))
	_, err = GetClientCertificate()
	require.Error(t, err)

	// client auth required and clientKey/clientCert set
	expected, err := tls.LoadX509KeyPair(
		filepath.Join(tempdir, "server1-cert.pem"),
		filepath.Join(tempdir, "server1-key.pem"),
	)
	if err != nil {
		t.Fatalf("Failed to load test certificate (%s)", err)
	}
	viper.Set("peer.tls.clientKey.file", filepath.Join(tempdir, "server1-key.pem"))
	cert, err := GetClientCertificate()
	require.NoError(t, err)
	require.Equal(t, expected, cert)

	// client auth required and clientKey/clientCert not set - expect
	// client cert to be the server cert
	viper.Set("peer.tls.clientKey.file", "")
	viper.Set("peer.tls.clientCert.file", "")
	expected, err = tls.LoadX509KeyPair(
		filepath.Join(tempdir, "server1-cert.pem"),
		filepath.Join(tempdir, "server1-key.pem"),
	)
	require.NoError(t, err, "failed to load test certificate")
	cert, err = GetClientCertificate()
	require.NoError(t, err)
	require.Equal(t, expected, cert)
}

func TestGlobalConfig(t *testing.T) {
	defer viper.Reset()
	cwd, err := os.Getwd()
	require.NoError(t, err, "failed to get current working directory")
	viper.SetConfigFile(filepath.Join(cwd, "core.yaml"))

	// Capture the configuration from viper
	viper.Set("peer.addressAutoDetect", false)
	viper.Set("peer.address", "localhost:8080")
	viper.Set("peer.id", "testPeerID")
	viper.Set("peer.localMspId", "SampleOrg")
	viper.Set("peer.listenAddress", "0.0.0.0:7051")
	viper.Set("peer.authentication.timewindow", "15m")
	viper.Set("peer.tls.enabled", "false")
	viper.Set("peer.networkId", "testNetwork")
	viper.Set("peer.limits.concurrency.endorserService", 2500)
	viper.Set("peer.limits.concurrency.deliverService", 2500)
	viper.Set("peer.limits.concurrency.gatewayService", 500)
	viper.Set("peer.discovery.enabled", true)
	viper.Set("peer.profile.enabled", false)
	viper.Set("peer.profile.listenAddress", "peer.authentication.timewindow")
	viper.Set("peer.discovery.orgMembersAllowedAccess", false)
	viper.Set("peer.discovery.authCacheEnabled", true)
	viper.Set("peer.discovery.authCacheMaxSize", 1000)
	viper.Set("peer.discovery.authCachePurgeRetentionRatio", 0.75)
	viper.Set("peer.chaincodeListenAddress", "0.0.0.0:7052")
	viper.Set("peer.chaincodeAddress", "0.0.0.0:7052")
	viper.Set("peer.validatorPoolSize", 1)
	viper.Set("peer.gateway.enabled", true)
	viper.Set("peer.gateway.endorsementTimeout", 10*time.Second)
	viper.Set("peer.gateway.dialTimeout", 60*time.Second)

	viper.Set("vm.endpoint", "unix:///var/run/docker.sock")
	viper.Set("vm.docker.tls.enabled", false)
	viper.Set("vm.docker.attachStdout", false)
	viper.Set("vm.docker.hostConfig.NetworkMode", "TestingHost")
	viper.Set("vm.docker.tls.cert.file", "test/vm/tls/cert/file")
	viper.Set("vm.docker.tls.key.file", "test/vm/tls/key/file")
	viper.Set("vm.docker.tls.ca.file", "test/vm/tls/ca/file")

	viper.Set("operations.listenAddress", "127.0.0.1:9443")
	viper.Set("operations.tls.enabled", false)
	viper.Set("operations.tls.cert.file", "test/tls/cert/file")
	viper.Set("operations.tls.key.file", "test/tls/key/file")
	viper.Set("operations.tls.clientAuthRequired", false)
	viper.Set("operations.tls.clientRootCAs.files", []string{"relative/file1", "/absolute/file2"})

	viper.Set("metrics.provider", "disabled")
	viper.Set("metrics.statsd.network", "udp")
	viper.Set("metrics.statsd.address", "127.0.0.1:8125")
	viper.Set("metrics.statsd.writeInterval", "10s")
	viper.Set("metrics.statsd.prefix", "testPrefix")

	viper.Set("chaincode.pull", false)
	viper.Set("chaincode.externalBuilders", &[]ExternalBuilder{
		{
			Path: "relative/plugin_dir",
			Name: "relative",
		},
		{
			Path: "/absolute/plugin_dir",
			Name: "absolute",
		},
	})

	coreConfig, err := GlobalConfig()
	require.NoError(t, err)

	expectedConfig := &Config{
		LocalMSPID:                            "SampleOrg",
		ListenAddress:                         "0.0.0.0:7051",
		AuthenticationTimeWindow:              15 * time.Minute,
		PeerTLSEnabled:                        false,
		PeerAddress:                           "localhost:8080",
		PeerID:                                "testPeerID",
		NetworkID:                             "testNetwork",
		LimitsConcurrencyEndorserService:      2500,
		LimitsConcurrencyDeliverService:       2500,
		LimitsConcurrencyGatewayService:       500,
		DiscoveryEnabled:                      true,
		ProfileEnabled:                        false,
		ProfileListenAddress:                  "peer.authentication.timewindow",
		DiscoveryOrgMembersAllowed:            false,
		DiscoveryAuthCacheEnabled:             true,
		DiscoveryAuthCacheMaxSize:             1000,
		DiscoveryAuthCachePurgeRetentionRatio: 0.75,
		ChaincodeListenAddress:                "0.0.0.0:7052",
		ChaincodeAddress:                      "0.0.0.0:7052",
		ValidatorPoolSize:                     1,
		DeliverClientKeepaliveOptions:         comm.DefaultKeepaliveOptions,

		VMEndpoint:           "unix:///var/run/docker.sock",
		VMDockerTLSEnabled:   false,
		VMDockerAttachStdout: false,
		VMNetworkMode:        "TestingHost",

		ChaincodePull: false,
		ExternalBuilders: []ExternalBuilder{
			{
				Path: "relative/plugin_dir",
				Name: "relative",
			},
			{
				Path: "/absolute/plugin_dir",
				Name: "absolute",
			},
		},
		OperationsListenAddress:         "127.0.0.1:9443",
		OperationsTLSEnabled:            false,
		OperationsTLSCertFile:           filepath.Join(cwd, "test/tls/cert/file"),
		OperationsTLSKeyFile:            filepath.Join(cwd, "test/tls/key/file"),
		OperationsTLSClientAuthRequired: false,
		OperationsTLSClientRootCAs: []string{
			filepath.Join(cwd, "relative", "file1"),
			"/absolute/file2",
		},

		MetricsProvider:     "disabled",
		StatsdNetwork:       "udp",
		StatsdAaddress:      "127.0.0.1:8125",
		StatsdWriteInterval: 10 * time.Second,
		StatsdPrefix:        "testPrefix",

		DockerCert: filepath.Join(cwd, "test/vm/tls/cert/file"),
		DockerKey:  filepath.Join(cwd, "test/vm/tls/key/file"),
		DockerCA:   filepath.Join(cwd, "test/vm/tls/ca/file"),

		GatewayOptions: config.Options{
			Enabled:            true,
			EndorsementTimeout: 10 * time.Second,
			BroadcastTimeout:   10 * time.Second,
			DialTimeout:        60 * time.Second,
		},
	}

	require.Equal(t, coreConfig, expectedConfig)
}

func TestGlobalConfigDefault(t *testing.T) {
	defer viper.Reset()
	viper.Set("peer.address", "localhost:8080")

	coreConfig, err := GlobalConfig()
	require.NoError(t, err)

	expectedConfig := &Config{
		AuthenticationTimeWindow:      15 * time.Minute,
		PeerAddress:                   "localhost:8080",
		ValidatorPoolSize:             runtime.NumCPU(),
		VMNetworkMode:                 "host",
		DeliverClientKeepaliveOptions: comm.DefaultKeepaliveOptions,
		GatewayOptions:                config.GetOptions(viper.GetViper()),
	}

	require.Equal(t, expectedConfig, coreConfig)
}

func TestPropagateEnvironment(t *testing.T) {
	defer viper.Reset()
	viper.Set("peer.address", "localhost:8080")
	viper.Set("chaincode.externalBuilders", &[]ExternalBuilder{
		{
			Name:                 "testName",
			Environment:          []string{"KEY=VALUE"},
			PropagateEnvironment: []string{},
			Path:                 "/testPath",
		},
		{
			Name:                 "testName",
			PropagateEnvironment: []string{"KEY=VALUE"},
			Path:                 "/testPath",
		},
		{
			Name:                 "testName",
			Environment:          []string{"KEY=VALUE"},
			PropagateEnvironment: []string{"KEY=VALUE2"},
			Path:                 "/testPath",
		},
	})
	coreConfig, err := GlobalConfig()
	require.NoError(t, err)

	expectedConfig := &Config{
		AuthenticationTimeWindow:      15 * time.Minute,
		PeerAddress:                   "localhost:8080",
		ValidatorPoolSize:             runtime.NumCPU(),
		VMNetworkMode:                 "host",
		DeliverClientKeepaliveOptions: comm.DefaultKeepaliveOptions,
		ExternalBuilders: []ExternalBuilder{
			{
				Name:                 "testName",
				Environment:          []string{"KEY=VALUE"},
				PropagateEnvironment: []string{"KEY=VALUE"},
				Path:                 "/testPath",
			},
			{
				Name:                 "testName",
				PropagateEnvironment: []string{"KEY=VALUE"},
				Path:                 "/testPath",
			},
			{
				Name:                 "testName",
				Environment:          []string{"KEY=VALUE"},
				PropagateEnvironment: []string{"KEY=VALUE2"},
				Path:                 "/testPath",
			},
		},
		GatewayOptions: config.GetOptions(viper.GetViper()),
	}
	require.Equal(t, expectedConfig, coreConfig)
}

func TestExternalBuilderConfigAsEnvVar(t *testing.T) {
	defer viper.Reset()
	viper.Set("peer.address", "localhost:8080")
	viper.Set("chaincode.externalBuilders", "[{name: relative, path: relative/plugin_dir, propagateEnvironment: [ENVVAR_NAME_TO_PROPAGATE_FROM_PEER, GOPROXY]}, {name: absolute, path: /absolute/plugin_dir}]")
	coreConfig, err := GlobalConfig()
	require.NoError(t, err)

	require.Equal(t, []ExternalBuilder{
		{
			Path:                 "relative/plugin_dir",
			Name:                 "relative",
			PropagateEnvironment: []string{"ENVVAR_NAME_TO_PROPAGATE_FROM_PEER", "GOPROXY"},
		},
		{
			Path: "/absolute/plugin_dir",
			Name: "absolute",
		},
	}, coreConfig.ExternalBuilders)
}

func TestMissingExternalBuilderPath(t *testing.T) {
	defer viper.Reset()
	viper.Set("peer.address", "localhost:8080")
	viper.Set("chaincode.externalBuilders", &[]ExternalBuilder{
		{
			Name: "testName",
		},
	})
	_, err := GlobalConfig()
	require.EqualError(t, err, "invalid external builder configuration, path attribute missing in one or more builders")
}

func TestMissingExternalBuilderName(t *testing.T) {
	defer viper.Reset()
	viper.Set("peer.address", "localhost:8080")
	viper.Set("chaincode.externalBuilders", &[]ExternalBuilder{
		{
			Path: "relative/plugin_dir",
		},
	})
	_, err := GlobalConfig()
	require.EqualError(t, err, "external builder at path relative/plugin_dir has no name attribute")
}
