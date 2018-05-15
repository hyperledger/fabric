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
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protos/common"
	proto "github.com/hyperledger/fabric/protos/gossip"
	"github.com/hyperledger/fabric/protos/ledger/rwset"
	"github.com/hyperledger/fabric/protos/transientstore"
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
	Distribute(txID string, privData *transientstore.TxPvtReadWriteSetWithConfigInfo, blkHt uint64) error
}

// IdentityDeserializerFactory is a factory interface to create
// IdentityDeserializer for given channel
type IdentityDeserializerFactory interface {
	// GetIdentityDeserializer returns an IdentityDeserializer
	// instance for the specified chain
	GetIdentityDeserializer(chainID string) msp.IdentityDeserializer
}

// distributorImpl the implementation of the private data distributor interface
type distributorImpl struct {
	chainID string
	gossipAdapter
	CollectionAccessFactory
}

// CollectionAccessFactory an interface to generate collection access policy
type CollectionAccessFactory interface {
	// AccessPolicy based on collection configuration
	AccessPolicy(config *common.CollectionConfig, chainID string) (privdata.CollectionAccessPolicy, error)
}

// policyAccessFactory the implementation of CollectionAccessFactory
type policyAccessFactory struct {
	IdentityDeserializerFactory
}

func (p *policyAccessFactory) AccessPolicy(config *common.CollectionConfig, chainID string) (privdata.CollectionAccessPolicy, error) {
	colAP := &privdata.SimpleCollection{}
	switch cconf := config.Payload.(type) {
	case *common.CollectionConfig_StaticCollectionConfig:
		err := colAP.Setup(cconf.StaticCollectionConfig, p.GetIdentityDeserializer(chainID))
		if err != nil {
			return nil, errors.WithMessage(err, fmt.Sprintf("error setting up collection  %#v", cconf.StaticCollectionConfig.Name))
		}
	default:
		return nil, errors.New("unexpected collection type")
	}
	return colAP, nil
}

// NewCollectionAccessFactory
func NewCollectionAccessFactory(factory IdentityDeserializerFactory) CollectionAccessFactory {
	return &policyAccessFactory{
		IdentityDeserializerFactory: factory,
	}
}

// NewDistributor a constructor for private data distributor capable to send
// private read write sets for underlying collection
func NewDistributor(chainID string, gossip gossipAdapter, factory CollectionAccessFactory) PvtDataDistributor {
	return &distributorImpl{
		chainID:                 chainID,
		gossipAdapter:           gossip,
		CollectionAccessFactory: factory,
	}
}

// Distribute broadcast reliably private data read write set based on policies
func (d *distributorImpl) Distribute(txID string, privData *transientstore.TxPvtReadWriteSetWithConfigInfo, blkHt uint64) error {
	disseminationPlan, err := d.computeDisseminationPlan(txID, privData, blkHt)
	if err != nil {
		return errors.WithStack(err)
	}
	return d.disseminate(disseminationPlan)
}

type dissemination struct {
	msg      *proto.SignedGossipMessage
	criteria gossip2.SendCriteria
}

func (d *distributorImpl) computeDisseminationPlan(txID string,
	privDataWithConfig *transientstore.TxPvtReadWriteSetWithConfigInfo,
	blkHt uint64) ([]*dissemination, error) {
	privData := privDataWithConfig.PvtRwset
	var disseminationPlan []*dissemination
	for _, pvtRwset := range privData.NsPvtRwset {
		namespace := pvtRwset.Namespace
		configPackage, found := privDataWithConfig.CollectionConfigs[namespace]
		if !found {
			logger.Error("Collection config package for", namespace, "chaincode is not provided")
			return nil, errors.New(fmt.Sprint("collection config package for", namespace, "chaincode is not provided"))
		}

		for _, collection := range pvtRwset.CollectionPvtRwset {
			colCP, err := d.getCollectionConfig(configPackage, collection)
			collectionName := collection.CollectionName
			if err != nil {
				logger.Error("Could not find collection access policy for", namespace, " and collection", collectionName, "error", err)
				return nil, errors.WithMessage(err, fmt.Sprint("could not find collection access policy for", namespace, " and collection", collectionName, "error", err))
			}

			colAP, err := d.AccessPolicy(colCP, d.chainID)
			if err != nil {
				logger.Error("Could not obtain collection access policy, collection name", collectionName, "due to", err)
				return nil, errors.Wrap(err, fmt.Sprint("Could not obtain collection access policy, collection name", collectionName, "due to", err))
			}

			colFilter := colAP.AccessFilter()
			if colFilter == nil {
				logger.Error("Collection access policy for", collectionName, "has no filter")
				return nil, errors.Errorf("No collection access policy filter computed for %v", collectionName)
			}

			pvtDataMsg, err := d.createPrivateDataMessage(txID, namespace, collection, &common.CollectionConfigPackage{Config: []*common.CollectionConfig{colCP}}, blkHt)
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

func (d *distributorImpl) getCollectionConfig(config *common.CollectionConfigPackage, collection *rwset.CollectionPvtReadWriteSet) (*common.CollectionConfig, error) {
	for _, c := range config.Config {
		if staticConfig := c.GetStaticCollectionConfig(); staticConfig != nil {
			if staticConfig.Name == collection.CollectionName {
				return c, nil
			}
		}
	}
	return nil, errors.New(fmt.Sprint("no configuration for collection", collection.CollectionName, "found"))
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

func (d *distributorImpl) createPrivateDataMessage(txID, namespace string,
	collection *rwset.CollectionPvtReadWriteSet,
	ccp *common.CollectionConfigPackage,
	blkHt uint64) (*proto.SignedGossipMessage, error) {
	msg := &proto.GossipMessage{
		Channel: []byte(d.chainID),
		Nonce:   util.RandomUInt64(),
		Tag:     proto.GossipMessage_CHAN_ONLY,
		Content: &proto.GossipMessage_PrivateData{
			PrivateData: &proto.PrivateDataMessage{
				Payload: &proto.PrivatePayload{
					Namespace:         namespace,
					CollectionName:    collection.CollectionName,
					TxId:              txID,
					PrivateRwset:      collection.Rwset,
					PrivateSimHeight:  blkHt,
					CollectionConfigs: ccp,
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
