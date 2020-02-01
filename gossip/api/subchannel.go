/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package api

import "github.com/hyperledger/fabric/gossip/common"

// RoutingFilter defines which peers should receive a certain message,
// or which peers are eligible of receiving a certain message
type RoutingFilter func(peerIdentity PeerIdentityType) bool

// SubChannelSelectionCriteria describes a way of selecting peers from a sub-channel
// given their signatures
type SubChannelSelectionCriteria func(signature PeerSignature) bool

// RoutingFilterFactory defines an object that given a CollectionCriteria and a channel,
// it can ascertain which peers should be aware of the data related to the
// CollectionCriteria.
type RoutingFilterFactory interface {
	// Peers returns a RoutingFilter for given channelID and CollectionCriteria
	Peers(common.ChannelID, SubChannelSelectionCriteria) RoutingFilter
}
