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
	"fmt"
	"io/ioutil"
	"net"
	"path/filepath"

	"github.com/spf13/viper"

	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/core/config"
	pb "github.com/hyperledger/fabric/protos/peer"
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
	getLocalAddress := func() (peerAddress string, err error) {
		if viper.GetBool("peer.addressAutoDetect") {
			// Need to get the port from the peer.address setting, and append to the determined host IP
			_, port, err := net.SplitHostPort(viper.GetString("peer.address"))
			if err != nil {
				err = fmt.Errorf("Error auto detecting Peer's address: %s", err)
				return "", err
			}
			peerAddress = net.JoinHostPort(GetLocalIP(), port)
			peerLogger.Infof("Auto detected peer address: %s", peerAddress)
		} else {
			peerAddress = viper.GetString("peer.address")
		}
		return
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
		secureOptions.ServerCertificate = serverCert
		secureOptions.ServerKey = serverKey
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
	serverConfig.KaOpts = comm.DefaultKeepaliveOptions()
	// check to see if minInterval is set for the env
	if viper.IsSet("peer.keepalive.minInterval") {
		serverConfig.KaOpts.ServerMinInterval = viper.GetDuration("peer.keepalive.minInterval")
	}
	return serverConfig, nil
}
