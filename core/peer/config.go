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

	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/core/config"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

// Is the configuration cached?
var configurationCached = false

// Cached values and error values of the computed constants getLocalAddress(),
// getValidatorStreamAddress(), and getPeerEndpoint()
var localAddress string
var localAddressError error
var peerEndpoint *pb.PeerEndpoint
var peerEndpointError error

// Cached values of commonly used configuration constants.

// CacheConfiguration computes and caches commonly-used constants and
// computed constants as package variables. Routines which were previously
// global have been embedded here to preserve the original abstraction.
func CacheConfiguration() (err error) {
	// getLocalAddress returns the address:port the local peer is operating on.  Affected by env:peer.addressAutoDetect
	getLocalAddress := func() (string, error) {
		peerAddress := viper.GetString("peer.address")
		if peerAddress == "" {
			return "", fmt.Errorf("peer.address isn't set")
		}
		host, port, err := net.SplitHostPort(peerAddress)
		if err != nil {
			return "", errors.Errorf("peer.address isn't in host:port format: %s", peerAddress)
		}

		autoDetectedIPAndPort := net.JoinHostPort(GetLocalIP(), port)
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

	// getPeerEndpoint returns the PeerEndpoint for this Peer instance.  Affected by env:peer.addressAutoDetect
	getPeerEndpoint := func() (*pb.PeerEndpoint, error) {
		var peerAddress string
		peerAddress, err := getLocalAddress()
		if err != nil {
			return nil, err
		}
		return &pb.PeerEndpoint{Id: &pb.PeerID{Name: viper.GetString("peer.id")}, Address: peerAddress}, nil
	}

	localAddress, localAddressError = getLocalAddress()
	peerEndpoint, _ = getPeerEndpoint()

	configurationCached = true

	if localAddressError != nil {
		return localAddressError
	}
	return
}

// cacheConfiguration logs an error if error checks have failed.
func cacheConfiguration() {
	if err := CacheConfiguration(); err != nil {
		peerLogger.Errorf("Execution continues after CacheConfiguration() failure : %s", err)
	}
}

//Functional forms

// GetLocalAddress returns the peer.address property
func GetLocalAddress() (string, error) {
	if !configurationCached {
		cacheConfiguration()
	}
	return localAddress, localAddressError
}

// GetPeerEndpoint returns peerEndpoint from cached configuration
func GetPeerEndpoint() (*pb.PeerEndpoint, error) {
	if !configurationCached {
		cacheConfiguration()
	}
	return peerEndpoint, peerEndpointError
}

// GetServerConfig returns the gRPC server configuration for the peer
func GetServerConfig() (comm.ServerConfig, error) {
	secureOptions := &comm.SecureOptions{
		UseTLS: viper.GetBool("peer.tls.enabled"),
	}
	serverConfig := comm.ServerConfig{
		ConnectionTimeout: viper.GetDuration("peer.connectiontimeout"),
		SecOpts:           secureOptions,
	}
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
	// check to see if minInterval is set for the env
	if viper.IsSet("peer.keepalive.minInterval") {
		serverConfig.KaOpts.ServerMinInterval = viper.GetDuration("peer.keepalive.minInterval")
	}
	return serverConfig, nil
}

// GetServerRootCAs returns the root certificates which will be trusted for
// gRPC client connections to peers and orderers.
func GetServerRootCAs() ([][]byte, error) {
	var rootCAs [][]byte
	if config.GetPath("peer.tls.rootcert.file") != "" {
		rootCert, err := ioutil.ReadFile(config.GetPath("peer.tls.rootcert.file"))
		if err != nil {
			return nil, fmt.Errorf("error loading TLS root certificate (%s)", err)
		}
		rootCAs = append(rootCAs, rootCert)
	}

	for _, file := range viper.GetStringSlice("peer.tls.serverRootCAs.files") {
		rootCert, err := ioutil.ReadFile(
			config.TranslatePath(filepath.Dir(viper.ConfigFileUsed()), file))
		if err != nil {
			return nil,
				fmt.Errorf("error loading server root CAs: %s", err)
		}
		rootCAs = append(rootCAs, rootCert)
	}
	return rootCAs, nil
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

type addressOverride struct {
	From        string `mapstructure:"from"`
	To          string `mapstructure:"to"`
	CACertsFile string `mapstructure:"caCertsFile"`
}

type OrdererEndpoint struct {
	Address string
	PEMs    []byte
}

func GetOrdererAddressOverrides() (map[string]*comm.OrdererEndpoint, error) {
	var overrides []addressOverride
	err := viper.UnmarshalKey("peer.deliveryclient.addressOverrides", &overrides)
	if err != nil {
		return nil, err
	}

	var overrideMap map[string]*comm.OrdererEndpoint
	if len(overrides) > 0 {
		overrideMap = make(map[string]*comm.OrdererEndpoint)
		for _, override := range overrides {
			if override.CACertsFile == "" {
				overrideMap[override.From] = &comm.OrdererEndpoint{
					Address: override.To,
				}
				continue
			}

			pem, err := ioutil.ReadFile(override.CACertsFile)
			if err != nil {
				logger.Warningf("could not read file '%s' specified for caCertsFile of orderer endpoint override from '%s' to '%s', skipping: %s", override.CACertsFile, override.From, override.To, err)
				continue
			}

			overrideMap[override.From] = &comm.OrdererEndpoint{
				Address: override.To,
				PEMs:    pem,
			}
		}
	}
	return overrideMap, nil
}
