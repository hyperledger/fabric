/*
Copyright IBM Corp. 2017 All Rights Reserved.

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

package service

import (
	"reflect"

	"github.com/hyperledger/fabric/protos/peer"
)

// Config enumerates the configuration methods required by gossip
type Config interface {
	// ChainID returns the chainID for this channel
	ChainID() string

	// AnchorPeers should return the current list of anchor peers
	AnchorPeers() []*peer.AnchorPeer

	// Sequence should return the sequence number of the current configuration
	Sequence() uint64
}

// ConfigProcessor receives config updates
type ConfigProcessor interface {
	// ProcessConfig should be invoked whenever a channel's configuration is initialized or updated
	ProcessConfigUpdate(config Config)
}

type configStore struct {
	anchorPeers []*peer.AnchorPeer
}

type configEventReceiver interface {
	configUpdated(config Config)
}

type configEventer struct {
	lastConfig *configStore
	receiver   configEventReceiver
}

func newConfigEventer(receiver configEventReceiver) *configEventer {
	return &configEventer{
		receiver: receiver,
	}
}

// ProcessConfigUpdate should be invoked whenever a channel's configuration is intialized or updated
// it invokes the associated method in configEventReceiver when configuration is updated
// but only if the configuration value actually changed
// Note, that a changing sequence number is ignored as changing configuration
func (ce *configEventer) ProcessConfigUpdate(config Config) {
	logger.Debugf("Processing new config for chain %s", config.ChainID())

	changed := false

	newAnchorPeers := config.AnchorPeers()

	if ce.lastConfig != nil {
		if !reflect.DeepEqual(newAnchorPeers, ce.lastConfig.anchorPeers) {
			changed = true
		}
	} else {
		changed = true
	}

	if changed {
		newConfig := &configStore{
			anchorPeers: config.AnchorPeers(),
		}
		ce.lastConfig = newConfig

		logger.Debugf("Calling out because config was updated for chain %s", config.ChainID())
		ce.receiver.configUpdated(config)
	}
}
