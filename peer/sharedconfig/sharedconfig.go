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

package sharedconfig

import (
	"fmt"

	cb "github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"

	"github.com/golang/protobuf/proto"
	"github.com/op/go-logging"
)

// Peer config keys
const (
	// AnchorPeersKey is the cb.ConfigurationItem type key name for the AnchorPeers message
	AnchorPeersKey = "AnchorPeers"
)

var logger = logging.MustGetLogger("peer/sharedconfig")

// Descriptor stores the common peer configuration
// It is intended to be the primary accessor of DescriptorImpl
// It is intended to discourage use of the other exported DescriptorImpl methods
// which are used for updating the chain configuration by the configtx.Manager
type Descriptor interface {
	// AnchorPeers returns the list of anchor peers for the channel
	AnchorPeers() []*pb.AnchorPeer
}

type sharedConfig struct {
	anchorPeers []*pb.AnchorPeer
}

// DescriptorImpl is an implementation of Manager and configtx.ConfigHandler
// In general, it should only be referenced as an Impl for the configtx.Manager
type DescriptorImpl struct {
	pendingConfig *sharedConfig
	config        *sharedConfig
}

// NewDescriptorImpl creates a new DescriptorImpl with the given CryptoHelper
func NewDescriptorImpl() *DescriptorImpl {
	return &DescriptorImpl{
		config: &sharedConfig{},
	}
}

// AnchorPeers returns the list of valid orderer addresses to connect to to invoke Broadcast/Deliver
func (di *DescriptorImpl) AnchorPeers() []*pb.AnchorPeer {
	return di.config.anchorPeers
}

// BeginConfig is used to start a new configuration proposal
func (di *DescriptorImpl) BeginConfig() {
	logger.Debugf("Beginning a possible new peer shared configuration")
	if di.pendingConfig != nil {
		logger.Panicf("Programming error, cannot call begin in the middle of a proposal")
	}
	di.pendingConfig = &sharedConfig{}
}

// RollbackConfig is used to abandon a new configuration proposal
func (di *DescriptorImpl) RollbackConfig() {
	logger.Debugf("Rolling back proposed peer shared configuration")
	di.pendingConfig = nil
}

// CommitConfig is used to commit a new configuration proposal
func (di *DescriptorImpl) CommitConfig() {
	logger.Debugf("Committing new peer shared configuration")
	if di.pendingConfig == nil {
		logger.Panicf("Programming error, cannot call commit without an existing proposal")
	}
	di.config = di.pendingConfig
	di.pendingConfig = nil
}

// ProposeConfig is used to add new configuration to the configuration proposal
func (di *DescriptorImpl) ProposeConfig(configItem *cb.ConfigurationItem) error {
	if configItem.Type != cb.ConfigurationItem_Peer {
		return fmt.Errorf("Expected type of ConfigurationItem_Peer, got %v", configItem.Type)
	}

	switch configItem.Key {
	case AnchorPeersKey:
		anchorPeers := &pb.AnchorPeers{}
		if err := proto.Unmarshal(configItem.Value, anchorPeers); err != nil {
			return fmt.Errorf("Unmarshaling error for %s: %s", configItem.Key, err)
		}
		if logger.IsEnabledFor(logging.DEBUG) {
			logger.Debugf("Setting %s to %v", configItem.Key, anchorPeers.AnchorPeers)
		}
		di.pendingConfig.anchorPeers = anchorPeers.AnchorPeers
	default:
		logger.Warningf("Uknown Peer configuration item with key %s", configItem.Key)
	}
	return nil
}
