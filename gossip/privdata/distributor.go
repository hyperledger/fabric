/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privdata

import (
	"fmt"
	"sync"
	"sync/atomic"

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
	"github.com/pkg/errors"
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
	Distribute(txID string, privData *rwset.TxPvtReadWriteSet, cs privdata.CollectionStore) error
}

// distributorImpl the implementation of the private data distributor interface
type distributorImpl struct {
	chainID string
	gossipAdapter
}

// NewDistributor a constructor for private data distributor capable to send
// private read write sets for underlying collection
func NewDistributor(chainID string, gossip gossipAdapter) PvtDataDistributor {
	return &distributorImpl{
		chainID:       chainID,
		gossipAdapter: gossip,
	}
}

// Distribute broadcast reliably private data read write set based on policies
func (d *distributorImpl) Distribute(txID string, privData *rwset.TxPvtReadWriteSet, cs privdata.CollectionStore) error {
	disseminationPlan, err := d.computeDisseminationPlan(txID, privData, cs)
	if err != nil {
		return errors.WithStack(err)
	}
	return d.disseminate(disseminationPlan)
}

type dissemination struct {
	msg      *proto.SignedGossipMessage
	criteria gossip2.SendCriteria
}

func (d *distributorImpl) computeDisseminationPlan(txID string, privData *rwset.TxPvtReadWriteSet, cs privdata.CollectionStore) ([]*dissemination, error) {
	var disseminationPlan []*dissemination
	for _, pvtRwset := range privData.NsPvtRwset {
		namespace := pvtRwset.Namespace
		for _, collection := range pvtRwset.CollectionPvtRwset {
			collectionName := collection.CollectionName
			cc := common.CollectionCriteria{
				Namespace:  namespace,
				Collection: collectionName,
				TxId:       txID,
				Channel:    d.chainID,
			}
			colAP, err := cs.RetrieveCollectionAccessPolicy(cc)
			if err != nil {
				logger.Error("Could not find collection access policy for", cc, "error", err)
				return nil, errors.WithMessage(err, fmt.Sprintf("collection access policy for %v not found", cc))
			}

			colFilter := colAP.AccessFilter()
			if colFilter == nil {
				logger.Error("Collection access policy for", cc, "has no filter")
				return nil, errors.Errorf("No collection access policy filter computed for %v", cc)
			}

			pvtDataMsg, err := d.createPrivateDataMessage(txID, namespace, collection.CollectionName, collection.Rwset)
			if err != nil {
				return nil, errors.WithStack(err)
			}

			dPlan, err := d.disseminationPlanForMsg(colAP, colFilter, pvtDataMsg)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			disseminationPlan = append(disseminationPlan, dPlan...)
		}
	}
	return disseminationPlan, nil
}

func (d *distributorImpl) disseminationPlanForMsg(colAP privdata.CollectionAccessPolicy, colFilter privdata.Filter, pvtDataMsg *proto.SignedGossipMessage) ([]*dissemination, error) {
	var disseminationPlan []*dissemination
	routingFilter, err := d.gossipAdapter.PeerFilter(gossipCommon.ChainID(d.chainID), func(signature api.PeerSignature) bool {
		return colFilter(common.SignedData{
			Data:      signature.Message,
			Signature: signature.Signature,
			Identity:  []byte(signature.PeerIdentity),
		})
	})

	if err != nil {
		logger.Error("Failed to retrieve peer routing filter for channel", d.chainID, ":", err)
		return nil, err
	}

	sc := gossip2.SendCriteria{
		Timeout:  viper.GetDuration("peer.gossip.pvtData.pushAckTimeout"),
		Channel:  gossipCommon.ChainID(d.chainID),
		MaxPeers: colAP.MaximumPeerCount(),
		MinAck:   colAP.RequiredPeerCount(),
		IsEligible: func(member discovery.NetworkMember) bool {
			return routingFilter(member)
		},
	}
	disseminationPlan = append(disseminationPlan, &dissemination{
		criteria: sc,
		msg:      pvtDataMsg,
	})
	return disseminationPlan, nil
}

func (d *distributorImpl) disseminate(disseminationPlan []*dissemination) error {
	var failures uint32
	var wg sync.WaitGroup
	wg.Add(len(disseminationPlan))
	for _, dis := range disseminationPlan {
		go func(dis *dissemination) {
			defer wg.Done()
			err := d.SendByCriteria(dis.msg, dis.criteria)
			if err != nil {
				atomic.AddUint32(&failures, 1)
				m := dis.msg.GetPrivateData().Payload
				logger.Error("Failed disseminating private RWSet for TxID", m.TxId, ", namespace", m.Namespace, "collection", m.CollectionName, ":", err)
			}
		}(dis)
	}
	wg.Wait()
	failureCount := atomic.LoadUint32(&failures)
	if failureCount != 0 {
		return errors.Errorf("Failed disseminating %d out of %d private RWSets", failureCount, len(disseminationPlan))
	}
	return nil
}

func (d *distributorImpl) createPrivateDataMessage(txID, namespace, collectionName string, rwset []byte) (*proto.SignedGossipMessage, error) {
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
					PrivateRwset:   rwset,
				},
			},
		},
	}

	pvtDataMsg, err := msg.NoopSign()
	if err != nil {
		return nil, err
	}
	return pvtDataMsg, nil
}
