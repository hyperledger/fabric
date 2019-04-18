/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// The 'viper' package for configuration handling is very flexible, but has
// been found to have extremely poor performance when configuration values are
// accessed repeatedly. The function CacheConfiguration() defined here caches
// all configuration values that are accessed frequently.  These parameters
// are now presented as function calls that access local configuration
// variables.  This seems to be the most robust way to represent these
// parameters in the face of the numerous ways that configuration files are
// loaded and used (e.g, normal usage vs. test cases).

// The CacheConfiguration() function is allowed to be called globally to
// ensure that the correct values are always cached; See for example how
// certain parameters are forced in 'ChaincodeDevMode' in main.go.

package peer

import (
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net"
	"path/filepath"
	"runtime"
	"time"

	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/core/config"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

// Config is the global struct which holds all configurations for Peer.
// config struct is defined and populated at the first place when it's
// being initiated.
// TODO: Currently all Config struct is still being populated from viper.
// 		 We do intent to move away from viper in the future.
type Config struct {
	// Identifier of the local MSP
	// !!!!! IMPORTANT !!!!!!
	// Deployers need to change the value of the localMSPID string. In particular,
	// the name of the local MSP ID of a peer needs to match the name of one of
	// the MSPs in each of the channels that this peer is a member of. Otherwise
	// this peer's messages will not be identified as valid by other nodes.
	LocalMSPID string
	// The address at local network interface this Peer will listen on.
	// By default, it will listen on all network interfaces
	ListenAddress string
	// The Peer id is used for identifying this Peer instance
	PeerID string
	// When used as peer config, this represents the endpoint to other peers in
	// the same organization. For peers in other organization, see
	// gossip.externalEndpoint for more info.
	// When used as CLI config, this means the peer's endpoint to interact with.
	PeerAddress string
	// The networkId allows for logical separation of networks
	NetworkID string
	// The endpoint this peer uses to listen for inbound chaincode connections. If
	// this is not set, the listen address is selected to be the peer's address with
	// port 7052
	ChaincodeListenAddress string
	// The endpoint the chaincode for this peer uses to connect to the peer. If this
	// is not specified, the chaincodeListenAddress address is selected. And if
	// chaincodeListenAddress is not specified, address is selected from peer listenAddress.
	ChaincodeAddress string
	// Number of goroutines that will execute transaction validation in parallel.
	// By default, the peer chooses the number of CPUs on the machine. Set this
	// variable to override that choice.
	// NOTE: overriding this value might negatively influence the performance of
	// the peer so please change this value only if you know what you're doing!
	ValidatorPoolSize int

	// ----- Profile -----
	// Used with Go profiling tools only in none production environment. In production,
	// it should be disabled (eg enabled : false)
	ProfileEnabled       bool
	ProfileListenAddress string

	// ----- Discovery -----
	// The discovery service is used by clients to query information about peers,
	// such as - which peers have joined a certain channel, what is the latest
	// channel config, and most importantly - given a chaincode and a channel, what
	// possible sets of peers satisfy the endorsement policy.

	// Enable discovery service
	DiscoveryEnabled bool
	// Whether to allow non-admins to perform non channel scoped queries.
	// When this is false, it means that only peer admins can perform non channel scoped queries.
	DiscoveryOrgMembersAllowed bool
	// Whether the authentication cache is enabled or not
	DiscoveryAuthCacheEnabled bool
	// The maximum size of the cache, after which a purge takes place
	DiscoveryAuthCacheMaxSize int
	// The proportion (0 to 1) of entries that remain in the cache after the cache is
	// purged due to overpopulation
	DiscoveryAuthCachePurgeRetentionRatio float64

	// ----- Limits -----
	// Limits is used to configure some internal resource limits

	// Concurrency limits the number of concurrently running system chaincode requests.
	// This option is only supported for qscc at this time.
	LimitsConcurrencyQSCC int

	// ----- TLS -----
	// Require server-side TLS
	PeerTLSEnabled bool

	// ----- Authentication -----
	// Authentication contains configuration parameters related to authenticating
	// client messages.

	// The acceptable difference between current server time and client's time as
	// specified in a client request message
	AuthenticationTimeWindow time.Duration

	// ----- AdminService -----
	// The admin service is used for adminstrative operations such as control over logger
	// levels, etc. Only peer administrators can use the service.

	// The interface and port on which the admin server will listen on. If this is
	// not set, or the port number is equal to the port of the peer listen address -
	// the admin service is attached to the peer's service (defaults to 7051)
	AdminListenAddress string

	// Endpoint of the vm management system. For docker can be one of the following in general
	// unix:///var/run/docker.sock
	// http://localhost:2375
	// https://localhost:2376
	VMEndpoint string

	// ----- vm.docker.tls -----
	// settings for docker vms
	VMDockerTLSEnabled   bool
	VMDockerAttachStdout bool

	// Enables/disables force pulling of the base docker images (listed below)
	// during user chaincode instantiation.
	// Useful when using moving image tags (such as :latest)
	ChaincodePull bool

	//Operations config
	OperationsListenAddress         string
	OperationsTLSEnabled            bool
	OperationsTLSCertFile           string
	OperationsTLSKeyFile            string
	OperationsTLSClientAuthRequired bool
	OperationsTLSClientRootCAs      []string

	//Metrics config
	MetricsProvider     string
	StatsdNetwork       string
	StatsdAaddress      string
	StatsdWriteInterval time.Duration
	StatsdPrefix        string
}

func GlobalConfig() (*Config, error) {
	c := &Config{}
	if err := c.load(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Config) load() error {
	preeAddress, err := getLocalAddress()
	if err != nil {
		return err
	}
	c.PeerAddress = preeAddress
	c.PeerID = viper.GetString("peer.id")
	c.LocalMSPID = viper.GetString("peer.localMspId")
	c.ListenAddress = viper.GetString("peer.listenAddress")

	c.AuthenticationTimeWindow = viper.GetDuration("peer.authentication.timewindow")
	if c.AuthenticationTimeWindow == 0 {
		defaultTimeWindow := 15 * time.Minute
		logger.Warningf("`peer.authentication.timewindow` not set; defaulting to %s", defaultTimeWindow)
		c.AuthenticationTimeWindow = defaultTimeWindow
	}

	c.PeerTLSEnabled = viper.GetBool("peer.tls.enabled")
	c.NetworkID = viper.GetString("peer.networkId")
	c.LimitsConcurrencyQSCC = viper.GetInt("peer.limits.concurrency.qscc")
	c.DiscoveryEnabled = viper.GetBool("peer.discovery.enabled")
	c.ProfileEnabled = viper.GetBool("peer.profile.enabled")
	c.ProfileListenAddress = viper.GetString("peer.profile.listenAddress")
	c.DiscoveryOrgMembersAllowed = viper.GetBool("peer.discovery.orgMembersAllowedAccess")
	c.DiscoveryAuthCacheEnabled = viper.GetBool("peer.discovery.authCacheEnabled")
	c.DiscoveryAuthCacheMaxSize = viper.GetInt("peer.discovery.authCacheMaxSize")
	c.DiscoveryAuthCachePurgeRetentionRatio = viper.GetFloat64("peer.discovery.authCachePurgeRetentionRatio")
	c.ChaincodeListenAddress = viper.GetString("peer.chaincodeListenAddress")
	c.ChaincodeAddress = viper.GetString("peer.chaincodeAddress")
	c.AdminListenAddress = viper.GetString("peer.adminService.listenAddress")

	c.ValidatorPoolSize = viper.GetInt("peer.validatorPoolSize")
	if c.ValidatorPoolSize <= 0 {
		c.ValidatorPoolSize = runtime.NumCPU()
	}

	c.VMEndpoint = viper.GetString("vm.endpoint")
	c.VMDockerTLSEnabled = viper.GetBool("vm.docker.tls.enabled")
	c.VMDockerAttachStdout = viper.GetBool("vm.docker.attachStdout")

	c.ChaincodePull = viper.GetBool("chaincode.pull")

	c.OperationsListenAddress = viper.GetString("operations.listenAddress")
	c.OperationsTLSEnabled = viper.GetBool("operations.tls.enabled")
	c.OperationsTLSCertFile = viper.GetString("operations.tls.cert.file")
	c.OperationsTLSKeyFile = viper.GetString("operations.tls.key.file")
	c.OperationsTLSClientAuthRequired = viper.GetBool("operations.tls.clientAuthRequired")
	c.OperationsTLSClientRootCAs = viper.GetStringSlice("operations.tls.clientRootCAs.files")

	c.MetricsProvider = viper.GetString("metrics.provider")
	c.StatsdNetwork = viper.GetString("metrics.statsd.network")
	c.StatsdAaddress = viper.GetString("metrics.statsd.address")
	c.StatsdWriteInterval = viper.GetDuration("metrics.statsd.writeInterval")
	c.StatsdPrefix = viper.GetString("metrics.statsd.prefix")

	return nil
}

// getLocalAddress returns the address:port the local peer is operating on.  Affected by env:peer.addressAutoDetect
func getLocalAddress() (string, error) {
	peerAddress := viper.GetString("peer.address")
	if peerAddress == "" {
		return "", fmt.Errorf("peer.address isn't set")
	}
	host, port, err := net.SplitHostPort(peerAddress)
	if err != nil {
		return "", errors.Errorf("peer.address isn't in host:port format: %s", peerAddress)
	}

	localIP, err := GetLocalIP()
	if err != nil {
		peerLogger.Errorf("Local ip address not auto-detectable: %s", err)
		return "", err
	}
	autoDetectedIPAndPort := net.JoinHostPort(localIP, port)
	peerLogger.Info("Auto-detected peer address:", autoDetectedIPAndPort)
	// If host is the IPv4 address "0.0.0.0" or the IPv6 address "::",
	// then fallback to auto-detected address
	if ip := net.ParseIP(host); ip != nil && ip.IsUnspecified() {
		peerLogger.Info("Host is", host, ", falling back to auto-detected address:", autoDetectedIPAndPort)
		return autoDetectedIPAndPort, nil
	}

	if viper.GetBool("peer.addressAutoDetect") {
		peerLogger.Info("Auto-detect flag is set, returning", autoDetectedIPAndPort)
		return autoDetectedIPAndPort, nil
	}
	peerLogger.Info("Returning", peerAddress)
	return peerAddress, nil

}

// GetServerConfig returns the gRPC server configuration for the peer
func GetServerConfig() (comm.ServerConfig, error) {
	secureOptions := &comm.SecureOptions{
		UseTLS: viper.GetBool("peer.tls.enabled"),
	}
	serverConfig := comm.ServerConfig{SecOpts: secureOptions}
	if secureOptions.UseTLS {
		// get the certs from the file system
		serverKey, err := ioutil.ReadFile(config.GetPath("peer.tls.key.file"))
		if err != nil {
			return serverConfig, fmt.Errorf("error loading TLS key (%s)", err)
		}
		serverCert, err := ioutil.ReadFile(config.GetPath("peer.tls.cert.file"))
		if err != nil {
			return serverConfig, fmt.Errorf("error loading TLS certificate (%s)", err)
		}
		secureOptions.Certificate = serverCert
		secureOptions.Key = serverKey
		secureOptions.RequireClientCert = viper.GetBool("peer.tls.clientAuthRequired")
		if secureOptions.RequireClientCert {
			var clientRoots [][]byte
			for _, file := range viper.GetStringSlice("peer.tls.clientRootCAs.files") {
				clientRoot, err := ioutil.ReadFile(
					config.TranslatePath(filepath.Dir(viper.ConfigFileUsed()), file))
				if err != nil {
					return serverConfig,
						fmt.Errorf("error loading client root CAs (%s)", err)
				}
				clientRoots = append(clientRoots, clientRoot)
			}
			secureOptions.ClientRootCAs = clientRoots
		}
		// check for root cert
		if config.GetPath("peer.tls.rootcert.file") != "" {
			rootCert, err := ioutil.ReadFile(config.GetPath("peer.tls.rootcert.file"))
			if err != nil {
				return serverConfig, fmt.Errorf("error loading TLS root certificate (%s)", err)
			}
			secureOptions.ServerRootCAs = [][]byte{rootCert}
		}
	}
	// get the default keepalive options
	serverConfig.KaOpts = comm.DefaultKeepaliveOptions
	// check to see if interval is set for the env
	if viper.IsSet("peer.keepalive.interval") {
		serverConfig.KaOpts.ServerInterval = viper.GetDuration("peer.keepalive.interval")
	}
	// check to see if timeout is set for the env
	if viper.IsSet("peer.keepalive.timeout") {
		serverConfig.KaOpts.ServerTimeout = viper.GetDuration("peer.keepalive.timeout")
	}
	// check to see if minInterval is set for the env
	if viper.IsSet("peer.keepalive.minInterval") {
		serverConfig.KaOpts.ServerMinInterval = viper.GetDuration("peer.keepalive.minInterval")
	}
	return serverConfig, nil
}

// GetClientCertificate returns the TLS certificate to use for gRPC client
// connections
func GetClientCertificate() (tls.Certificate, error) {
	cert := tls.Certificate{}

	keyPath := viper.GetString("peer.tls.clientKey.file")
	certPath := viper.GetString("peer.tls.clientCert.file")

	if keyPath != "" || certPath != "" {
		// need both keyPath and certPath to be set
		if keyPath == "" || certPath == "" {
			return cert, errors.New("peer.tls.clientKey.file and " +
				"peer.tls.clientCert.file must both be set or must both be empty")
		}
		keyPath = config.GetPath("peer.tls.clientKey.file")
		certPath = config.GetPath("peer.tls.clientCert.file")

	} else {
		// use the TLS server keypair
		keyPath = viper.GetString("peer.tls.key.file")
		certPath = viper.GetString("peer.tls.cert.file")

		if keyPath != "" || certPath != "" {
			// need both keyPath and certPath to be set
			if keyPath == "" || certPath == "" {
				return cert, errors.New("peer.tls.key.file and " +
					"peer.tls.cert.file must both be set or must both be empty")
			}
			keyPath = config.GetPath("peer.tls.key.file")
			certPath = config.GetPath("peer.tls.cert.file")
		} else {
			return cert, errors.New("must set either " +
				"[peer.tls.key.file and peer.tls.cert.file] or " +
				"[peer.tls.clientKey.file and peer.tls.clientCert.file]" +
				"when peer.tls.clientAuthEnabled is set to true")
		}
	}
	// get the keypair from the file system
	clientKey, err := ioutil.ReadFile(keyPath)
	if err != nil {
		return cert, errors.WithMessage(err,
			"error loading client TLS key")
	}
	clientCert, err := ioutil.ReadFile(certPath)
	if err != nil {
		return cert, errors.WithMessage(err,
			"error loading client TLS certificate")
	}
	cert, err = tls.X509KeyPair(clientCert, clientKey)
	if err != nil {
		return cert, errors.WithMessage(err,
			"error parsing client TLS key pair")
	}
	return cert, nil
}
