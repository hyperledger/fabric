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

	"github.com/hyperledger/fabric/common/viperutil"
	"github.com/hyperledger/fabric/core/config"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	gatewayconfig "github.com/hyperledger/fabric/internal/pkg/gateway/config"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

// ExternalBuilder represents the configuration structure of
// a chaincode external builder
type ExternalBuilder struct {
	// TODO: Remove Environment in 3.0
	// Deprecated: Environment is retained for backwards compatibility.
	// New deployments should use the new PropagateEnvironment field
	Environment          []string `yaml:"environmentWhitelist"`
	PropagateEnvironment []string `yaml:"propagateEnvironment"`
	Name                 string   `yaml:"name"`
	Path                 string   `yaml:"path"`
}

// Config is the struct that defines the Peer configurations.
type Config struct {
	// LocalMSPID is the identifier of the local MSP.
	LocalMSPID string
	// ListenAddress is the local address the peer will listen on. It must be
	// formatted as [host | ipaddr]:port.
	ListenAddress string
	// PeerID provides a name for this peer instance. It is used when naming
	// docker resources to segregate fabric networks and peers.
	PeerID string
	// PeerAddress is the address other peers and clients should use to
	// communicate with the peer. It must be formatted as [host | ipaddr]:port.
	// When used by the CLI, it represents the target peer endpoint.
	PeerAddress string
	// NetworkID specifies a name to use for logical separation of networks. It
	// is used when naming docker resources to segregate fabric networks and
	// peers.
	NetworkID string
	// ChaincodeListenAddress is the endpoint on which this peer will listen for
	// chaincode connections. If omitted, it defaults to the host portion of
	// PeerAddress and port 7052.
	ChaincodeListenAddress string
	// ChaincodeAddress specifies the endpoint chaincode launched by the peer
	// should use to connect to the peer. If omitted, it defaults to
	// ChaincodeListenAddress and falls back to ListenAddress.
	ChaincodeAddress string
	// ValidatorPoolSize indicates the number of goroutines that will execute
	// transaction validation in parallel. If omitted, it defaults to number of
	// hardware threads on the machine.
	ValidatorPoolSize int

	// ----- Peer Delivery Client Keepalive -----
	// DeliveryClient Keepalive settings for communication with ordering nodes.
	DeliverClientKeepaliveOptions comm.KeepaliveOptions

	// ----- Profile -----
	// TODO: create separate sub-struct for Profile config.

	// ProfileEnabled determines if the go pprof endpoint is enabled in the peer.
	ProfileEnabled bool
	// ProfileListenAddress is the address the pprof server should accept
	// connections on.
	ProfileListenAddress string

	// ----- Discovery -----

	// The discovery service is used by clients to query information about peers,
	// such as - which peers have joined a certain channel, what is the latest
	// channel config, and most importantly - given a chaincode and a channel, what
	// possible sets of peers satisfy the endorsement policy.
	// TODO: create separate sub-struct for Discovery config.

	// DiscoveryEnabled is used to enable the discovery service.
	DiscoveryEnabled bool
	// DiscoveryOrgMembersAllowed allows non-admins to perform non channel-scoped queries.
	DiscoveryOrgMembersAllowed bool
	// DiscoveryAuthCacheEnabled is used to enable the authentication cache.
	DiscoveryAuthCacheEnabled bool
	// DiscoveryAuthCacheMaxSize sets the maximum size of authentication cache.
	DiscoveryAuthCacheMaxSize int
	// DiscoveryAuthCachePurgeRetentionRatio set the proportion of entries remains in cache
	// after overpopulation purge.
	DiscoveryAuthCachePurgeRetentionRatio float64

	// ----- Limits -----
	// Limits is used to configure some internal resource limits.
	// TODO: create separate sub-struct for Limits config.

	// LimitsConcurrencyEndorserService sets the limits for concurrent requests sent to
	// endorser service that handles chaincode deployment, query and invocation,
	// including both user chaincodes and system chaincodes.
	LimitsConcurrencyEndorserService int

	// LimitsConcurrencyDeliverService sets the limits for concurrent event listeners
	// registered to deliver service for blocks and transaction events.
	LimitsConcurrencyDeliverService int

	// LimitsConcurrencyGatewayService sets the limits for concurrent requests to
	// gateway service that handles the submission and evaluation of transactions.
	LimitsConcurrencyGatewayService int

	// ----- TLS -----
	// Require server-side TLS.
	// TODO: create separate sub-struct for PeerTLS config.

	// PeerTLSEnabled enables/disables Peer TLS.
	PeerTLSEnabled bool

	// ----- Authentication -----
	// Authentication contains configuration parameters related to authenticating
	// client messages.
	// TODO: create separate sub-struct for Authentication config.

	// AuthenticationTimeWindow sets the acceptable time duration for current
	// server time and client's time as specified in a client request message.
	AuthenticationTimeWindow time.Duration

	// Endpoint of the vm management system. For docker can be one of the following in general
	// unix:///var/run/docker.sock
	// http://localhost:2375
	// https://localhost:2376
	VMEndpoint string

	// ----- vm.docker.tls -----
	// TODO: create separate sub-struct for VM.Docker.TLS config.

	// VMDockerTLSEnabled enables/disables TLS for dockers.
	VMDockerTLSEnabled   bool
	VMDockerAttachStdout bool
	// VMNetworkMode sets the networking mode for the container.
	VMNetworkMode string

	// ChaincodePull enables/disables force pulling of the base docker image.
	ChaincodePull bool
	// ExternalBuilders represents the builders and launchers for
	// chaincode. The external builder detection processing will iterate over the
	// builders in the order specified below.
	ExternalBuilders []ExternalBuilder

	// ----- Operations config -----
	// TODO: create separate sub-struct for Operations config.

	// OperationsListenAddress provides the host and port for the operations server
	OperationsListenAddress string
	// OperationsTLSEnabled enables/disables TLS for operations.
	OperationsTLSEnabled bool
	// OperationsTLSCertFile provides the path to PEM encoded server certificate for
	// the operations server.
	OperationsTLSCertFile string
	// OperationsTLSKeyFile provides the path to PEM encoded server key for the
	// operations server.
	OperationsTLSKeyFile string
	// OperationsTLSClientAuthRequired enables/disables the requirements for client
	// certificate authentication at the TLS layer to access all resource.
	OperationsTLSClientAuthRequired bool
	// OperationsTLSClientRootCAs provides the path to PEM encoded ca certiricates to
	// trust for client authentication.
	OperationsTLSClientRootCAs []string

	// ----- Metrics config -----
	// TODO: create separate sub-struct for Metrics config.

	// MetricsProvider provides the categories of metrics providers, which is one of
	// statsd, prometheus, or disabled.
	MetricsProvider string
	// StatsdNetwork indicate the network type used by statsd metrics. (tcp or udp).
	StatsdNetwork string
	// StatsdAaddress provides the address for statsd server.
	StatsdAaddress string
	// StatsdWriteInterval set the time interval at which locally cached counters and
	// gauges are pushed.
	StatsdWriteInterval time.Duration
	// StatsdPrefix provides the prefix that prepended to all emitted statsd metrics.
	StatsdPrefix string

	// ----- Docker config ------

	// DockerCert is the path to the PEM encoded TLS client certificate required to access
	// the docker daemon.
	DockerCert string
	// DockerKey is the path to the PEM encoded key required to access the docker daemon.
	DockerKey string
	// DockerCA is the path to the PEM encoded CA certificate for the docker daemon.
	DockerCA string

	// ----- Gateway config -----

	// The gateway service is used by client SDKs to
	// interact with fabric networks

	GatewayOptions gatewayconfig.Options
}

// GlobalConfig obtains a set of configuration from viper, build and returns
// the config struct.
func GlobalConfig() (*Config, error) {
	c := &Config{}
	if err := c.load(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Config) load() error {
	peerAddress, err := getLocalAddress()
	if err != nil {
		return err
	}

	configDir := filepath.Dir(viper.ConfigFileUsed())

	c.PeerAddress = peerAddress
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
	c.LimitsConcurrencyEndorserService = viper.GetInt("peer.limits.concurrency.endorserService")
	c.LimitsConcurrencyDeliverService = viper.GetInt("peer.limits.concurrency.deliverService")
	c.LimitsConcurrencyGatewayService = viper.GetInt("peer.limits.concurrency.gatewayService")
	c.DiscoveryEnabled = viper.GetBool("peer.discovery.enabled")
	c.ProfileEnabled = viper.GetBool("peer.profile.enabled")
	c.ProfileListenAddress = viper.GetString("peer.profile.listenAddress")
	c.DiscoveryOrgMembersAllowed = viper.GetBool("peer.discovery.orgMembersAllowedAccess")
	c.DiscoveryAuthCacheEnabled = viper.GetBool("peer.discovery.authCacheEnabled")
	c.DiscoveryAuthCacheMaxSize = viper.GetInt("peer.discovery.authCacheMaxSize")
	c.DiscoveryAuthCachePurgeRetentionRatio = viper.GetFloat64("peer.discovery.authCachePurgeRetentionRatio")
	c.ChaincodeListenAddress = viper.GetString("peer.chaincodeListenAddress")
	c.ChaincodeAddress = viper.GetString("peer.chaincodeAddress")

	c.ValidatorPoolSize = viper.GetInt("peer.validatorPoolSize")
	if c.ValidatorPoolSize <= 0 {
		c.ValidatorPoolSize = runtime.NumCPU()
	}

	c.DeliverClientKeepaliveOptions = comm.DefaultKeepaliveOptions
	if viper.IsSet("peer.keepalive.deliveryClient.interval") {
		c.DeliverClientKeepaliveOptions.ClientInterval = viper.GetDuration("peer.keepalive.deliveryClient.interval")
	}
	if viper.IsSet("peer.keepalive.deliveryClient.timeout") {
		c.DeliverClientKeepaliveOptions.ClientTimeout = viper.GetDuration("peer.keepalive.deliveryClient.timeout")
	}

	c.GatewayOptions = gatewayconfig.GetOptions(viper.GetViper())

	c.VMEndpoint = viper.GetString("vm.endpoint")
	c.VMDockerTLSEnabled = viper.GetBool("vm.docker.tls.enabled")
	c.VMDockerAttachStdout = viper.GetBool("vm.docker.attachStdout")

	c.VMNetworkMode = viper.GetString("vm.docker.hostConfig.NetworkMode")
	if c.VMNetworkMode == "" {
		c.VMNetworkMode = "host"
	}

	c.ChaincodePull = viper.GetBool("chaincode.pull")
	var externalBuilders []ExternalBuilder

	err = viper.UnmarshalKey("chaincode.externalBuilders", &externalBuilders, viper.DecodeHook(viperutil.YamlStringToStructHook(externalBuilders)))
	if err != nil {
		return err
	}

	c.ExternalBuilders = externalBuilders
	for builderIndex, builder := range c.ExternalBuilders {
		if builder.Path == "" {
			return fmt.Errorf("invalid external builder configuration, path attribute missing in one or more builders")
		}
		if builder.Name == "" {
			return fmt.Errorf("external builder at path %s has no name attribute", builder.Path)
		}
		if builder.Environment != nil && len(builder.PropagateEnvironment) == 0 {
			c.ExternalBuilders[builderIndex].PropagateEnvironment = builder.Environment
		}
	}

	c.OperationsListenAddress = viper.GetString("operations.listenAddress")
	c.OperationsTLSEnabled = viper.GetBool("operations.tls.enabled")
	c.OperationsTLSCertFile = config.GetPath("operations.tls.cert.file")
	c.OperationsTLSKeyFile = config.GetPath("operations.tls.key.file")
	c.OperationsTLSClientAuthRequired = viper.GetBool("operations.tls.clientAuthRequired")

	for _, rca := range viper.GetStringSlice("operations.tls.clientRootCAs.files") {
		c.OperationsTLSClientRootCAs = append(c.OperationsTLSClientRootCAs, config.TranslatePath(configDir, rca))
	}

	c.MetricsProvider = viper.GetString("metrics.provider")
	c.StatsdNetwork = viper.GetString("metrics.statsd.network")
	c.StatsdAaddress = viper.GetString("metrics.statsd.address")
	c.StatsdWriteInterval = viper.GetDuration("metrics.statsd.writeInterval")
	c.StatsdPrefix = viper.GetString("metrics.statsd.prefix")

	c.DockerCert = config.GetPath("vm.docker.tls.cert.file")
	c.DockerKey = config.GetPath("vm.docker.tls.key.file")
	c.DockerCA = config.GetPath("vm.docker.tls.ca.file")

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

	localIP, err := getLocalIP()
	if err != nil {
		peerLogger.Errorf("local IP address not auto-detectable: %s", err)
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

// getLocalIP returns the a loopback local IP of the host.
func getLocalIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}
	for _, address := range addrs {
		// check the address type and if it is not a loopback then display it
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String(), nil
			}
		}
	}
	return "", errors.Errorf("no non-loopback, IPv4 interface detected")
}

// GetServerConfig returns the gRPC server configuration for the peer
func GetServerConfig() (comm.ServerConfig, error) {
	serverConfig := comm.ServerConfig{
		ConnectionTimeout: viper.GetDuration("peer.connectiontimeout"),
		SecOpts: comm.SecureOptions{
			UseTLS: viper.GetBool("peer.tls.enabled"),
		},
	}
	if serverConfig.SecOpts.UseTLS {
		// get the certs from the file system
		serverKey, err := ioutil.ReadFile(config.GetPath("peer.tls.key.file"))
		if err != nil {
			return serverConfig, fmt.Errorf("error loading TLS key (%s)", err)
		}
		serverCert, err := ioutil.ReadFile(config.GetPath("peer.tls.cert.file"))
		if err != nil {
			return serverConfig, fmt.Errorf("error loading TLS certificate (%s)", err)
		}
		serverConfig.SecOpts.Certificate = serverCert
		serverConfig.SecOpts.Key = serverKey
		serverConfig.SecOpts.RequireClientCert = viper.GetBool("peer.tls.clientAuthRequired")
		if serverConfig.SecOpts.RequireClientCert {
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
			serverConfig.SecOpts.ClientRootCAs = clientRoots
		}
		// check for root cert
		if config.GetPath("peer.tls.rootcert.file") != "" {
			rootCert, err := ioutil.ReadFile(config.GetPath("peer.tls.rootcert.file"))
			if err != nil {
				return serverConfig, fmt.Errorf("error loading TLS root certificate (%s)", err)
			}
			serverConfig.SecOpts.ServerRootCAs = [][]byte{rootCert}
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

	serverConfig.MaxRecvMsgSize = comm.DefaultMaxRecvMsgSize
	serverConfig.MaxSendMsgSize = comm.DefaultMaxSendMsgSize

	if viper.IsSet("peer.maxRecvMsgSize") {
		serverConfig.MaxRecvMsgSize = int(viper.GetInt32("peer.maxRecvMsgSize"))
	}
	if viper.IsSet("peer.maxSendMsgSize") {
		serverConfig.MaxSendMsgSize = int(viper.GetInt32("peer.maxSendMsgSize"))
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
