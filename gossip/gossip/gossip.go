/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gossip

import (
	"fmt"
	"time"

	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/filter"
	"github.com/hyperledger/fabric/gossip/protoext"
)

// emittedGossipMessage encapsulates signed gossip message to compose
// with routing filter to be used while message is forwarded
type emittedGossipMessage struct {
	*protoext.SignedGossipMessage
	filter func(id common.PKIidType) bool
}

// SendCriteria defines how to send a specific message
type SendCriteria struct {
	Timeout    time.Duration        // Timeout defines the time to wait for acknowledgements
	MinAck     int                  // MinAck defines the amount of peers to collect acknowledgements from
	MaxPeers   int                  // MaxPeers defines the maximum number of peers to send the message to
	IsEligible filter.RoutingFilter // IsEligible defines whether a specific peer is eligible of receiving the message
	Channel    common.ChannelID     // Channel specifies a channel to send this message on. \
	// Only peers that joined the channel would receive this message
}

// String returns a string representation of this SendCriteria
func (sc SendCriteria) String() string {
	return fmt.Sprintf("channel: %s, tout: %v, minAck: %d, maxPeers: %d", sc.Channel, sc.Timeout, sc.MinAck, sc.MaxPeers)
}
