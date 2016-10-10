/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
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
	"net"

	"github.com/spf13/viper"

	pb "github.com/hyperledger/fabric/protos"
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
var syncStateSnapshotChannelSize int
var syncStateDeltasChannelSize int
var syncBlocksChannelSize int
var validatorEnabled bool

// Note: There is some kind of circular import issue that prevents us from
// importing the "core" package into the "peer" package. The
// 'peer.SecurityEnabled' bit is a duplicate of the 'core.SecurityEnabled'
// bit.
var securityEnabled bool

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
		var peerType pb.PeerEndpoint_Type
		peerAddress, err := getLocalAddress()
		if err != nil {
			return nil, err
		}
		if viper.GetBool("peer.validator.enabled") {
			peerType = pb.PeerEndpoint_VALIDATOR
		} else {
			peerType = pb.PeerEndpoint_NON_VALIDATOR
		}
		return &pb.PeerEndpoint{ID: &pb.PeerID{Name: viper.GetString("peer.id")}, Address: peerAddress, Type: peerType}, nil
	}

	localAddress, localAddressError = getLocalAddress()
	peerEndpoint, peerEndpointError = getPeerEndpoint()

	syncStateSnapshotChannelSize = viper.GetInt("peer.sync.state.snapshot.channelSize")
	syncStateDeltasChannelSize = viper.GetInt("peer.sync.state.deltas.channelSize")
	syncBlocksChannelSize = viper.GetInt("peer.sync.blocks.channelSize")
	validatorEnabled = viper.GetBool("peer.validator.enabled")

	securityEnabled = viper.GetBool("security.enabled")

	configurationCached = true

	if localAddressError != nil {
		return localAddressError
	} else if peerEndpointError != nil {
		return peerEndpointError
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

// SyncStateSnapshotChannelSize returns the peer.sync.state.snapshot.channelSize property
func SyncStateSnapshotChannelSize() int {
	if !configurationCached {
		cacheConfiguration()
	}
	return syncStateSnapshotChannelSize
}

// SyncStateDeltasChannelSize returns the peer.sync.state.deltas.channelSize property
func SyncStateDeltasChannelSize() int {
	if !configurationCached {
		cacheConfiguration()
	}
	return syncStateDeltasChannelSize
}

// SyncBlocksChannelSize returns the peer.sync.blocks.channelSize property
func SyncBlocksChannelSize() int {
	if !configurationCached {
		cacheConfiguration()
	}
	return syncBlocksChannelSize
}

// ValidatorEnabled returns the peer.validator.enabled property
func ValidatorEnabled() bool {
	if !configurationCached {
		cacheConfiguration()
	}
	return validatorEnabled
}

// SecurityEnabled returns the securityEnabled property from cached configuration
func SecurityEnabled() bool {
	if !configurationCached {
		cacheConfiguration()
	}
	return securityEnabled
}
