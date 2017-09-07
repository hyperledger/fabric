/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privdata

import (
	"time"

	"github.com/hyperledger/fabric/core/common/privdata"
	"github.com/hyperledger/fabric/gossip/api"
	gossipCommon "github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/gossip/filter"
	gossip2 "github.com/hyperledger/fabric/gossip/gossip"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/hyperledger/fabric/protos/common"
	proto "github.com/hyperledger/fabric/protos/gossip"
	"github.com/hyperledger/fabric/protos/ledger/rwset"
	"github.com/spf13/viper"
)

// gossipAdapter an adapter for API's required from gossip module
type gossipAdapter interface {
	// SendByCriteria sends a given message to all peers that match the given SendCriteria
	SendByCriteria(message *proto.SignedGossipMessage, criteria gossip2.SendCriteria) error

	// PeerFilter receives a SubChannelSelectionCriteria and returns a RoutingFilter that selects
	// only peer identities that match the given criteria, and that they published their channel participation
	PeerFilter(channel gossipCommon.ChainID, messagePredicate api.SubChannelSelectionCriteria) (filter.RoutingFilter, error)
}

// PvtDataDistributor interface to defines API of distributing private data
type PvtDataDistributor interface {
	// Distribute broadcast reliably private data read write set based on policies
	Distribute(txID string, privData *rwset.TxPvtReadWriteSet, ps privdata.PolicyStore, pp privdata.PolicyParser) error
}

// distributorImpl the implementation of the private data distributor interface
type distributorImpl struct {
	chainID  string
	minAck   int
	maxPeers int
	gossipAdapter
}

// NewDistributor a constructor for private data distributor capable to send
// private read write sets for underlying collection
func NewDistributor(chainID string, gossip gossipAdapter) PvtDataDistributor {
	return &distributorImpl{
		chainID:       chainID,
		gossipAdapter: gossip,
		minAck:        viper.GetInt("peer.gossip.pvtData.minAck"),
		maxPeers:      viper.GetInt("peer.gossip.pvtData.maxPeers"),
	}
}

// Distribute broadcast reliably private data read write set based on policies
func (d *distributorImpl) Distribute(txID string, privData *rwset.TxPvtReadWriteSet, ps privdata.PolicyStore, pp privdata.PolicyParser) error {
	for _, pvtRwset := range privData.NsPvtRwset {
		namespace := pvtRwset.Namespace
		for _, collection := range pvtRwset.CollectionPvtRwset {
			collectionName := collection.CollectionName
			policyFilter := pp.Parse(ps.CollectionPolicy(common.CollectionCriteria{
				Namespace:  namespace,
				Collection: collectionName,
				TxId:       txID,
				Channel:    d.chainID,
			}))

			routingFilter, err := d.gossipAdapter.PeerFilter(gossipCommon.ChainID(d.chainID), func(signature api.PeerSignature) bool {
				return policyFilter(common.SignedData{
					Data:      signature.Message,
					Signature: signature.Signature,
					Identity:  []byte(signature.PeerIdentity),
				})
			})

			if err != nil {
				logger.Error("Failed to retrieve peer routing filter, due to", err, "collection name", collectionName)
				return err
			}

			msg := &proto.GossipMessage{
				Channel: []byte(d.chainID),
				Nonce:   util.RandomUInt64(),
				Tag:     proto.GossipMessage_CHAN_ONLY,
				Content: &proto.GossipMessage_PrivateData{
					PrivateData: &proto.PrivateDataMessage{
						Payload: &proto.PrivatePayload{
							Namespace:      namespace,
							CollectionName: collectionName,
							TxId:           txID,
							PrivateRwset:   collection.Rwset,
						},
					},
				},
			}

			pvtDataMsg, err := msg.NoopSign()
			if err != nil {
				return err
			}

			err = d.gossipAdapter.SendByCriteria(pvtDataMsg, gossip2.SendCriteria{
				Timeout:  time.Second,
				Channel:  gossipCommon.ChainID(d.chainID),
				MaxPeers: d.maxPeers,
				MinAck:   d.minAck,
				IsEligible: func(member discovery.NetworkMember) bool {
					return routingFilter(member)
				},
			})

			if err != nil {
				logger.Warning("Could not send private data for channel", d.chainID,
					"collection name", collectionName, "due to", err)

				return err
			}
		}
	}
	return nil
}
