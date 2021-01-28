/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	protos "github.com/hyperledger/fabric-protos-go/discovery"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
)

// DiscoveryService defines capabilities directly accessed in the embedded peer's discovery service
type DiscoveryService interface {
	PeersForEndorsement(channel common.ChannelID, interest *protos.ChaincodeInterest) (*protos.EndorsementDescriptor, error)
	PeersOfChannel(common.ChannelID) discovery.Members
}
