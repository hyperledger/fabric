/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privdata

import (
	"bytes"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	proto2 "github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/common/privdata"
	"github.com/hyperledger/fabric/gossip/api"
	gossipCommon "github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/gossip/filter"
	gossip2 "github.com/hyperledger/fabric/gossip/gossip"
	"github.com/hyperledger/fabric/gossip/metrics"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protos/common"
	proto "github.com/hyperledger/fabric/protos/gossip"
	"github.com/hyperledger/fabric/protos/ledger/rwset"
	"github.com/hyperledger/fabric/protos/transientstore"
	"github.com/pkg/errors"
)

// gossipAdapter an adapter for API's required from gossip module
type gossipAdapter interface {
	// SendByCriteria sends a given message to all peers that match the given SendCriteria
	SendByCriteria(message *proto.SignedGossipMessage, criteria gossip2.SendCriteria) error

	// PeerFilter receives a SubChannelSelectionCriteria and returns a RoutingFilter that selects
	// only peer identities that match the given criteria, and that they published their channel participation
	PeerFilter(channel gossipCommon.ChainID, messagePredicate api.SubChannelSelectionCriteria) (filter.RoutingFilter, error)

	// IdentityInfo returns information known peer identities
	IdentityInfo() api.PeerIdentitySet

	// PeersOfChannel returns the NetworkMembers considered alive
	// and also subscribed to the channel given
	PeersOfChannel(gossipCommon.ChainID) []discovery.NetworkMember
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
	pushAckTimeout time.Duration
	metrics        *metrics.PrivdataMetrics
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
func NewDistributor(chainID string, gossip gossipAdapter, factory CollectionAccessFactory,
	metrics *metrics.PrivdataMetrics, pushAckTimeout time.Duration) PvtDataDistributor {
	return &distributorImpl{
		chainID:                 chainID,
		gossipAdapter:           gossip,
		CollectionAccessFactory: factory,
		pushAckTimeout:          pushAckTimeout,
		metrics:                 metrics,
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

			logger.Debugf("Computing dissemination plan for collection [%s]", collectionName)
			dPlan, err := d.disseminationPlanForMsg(colAP, colFilter, pvtDataMsg)
			if err != nil {
				return nil, errors.WithMessage(err, fmt.Sprint("could not build private data dissemination plan for chaincode ", namespace, " and collection ", collectionName))
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

	m := pvtDataMsg.GetPrivateData().Payload

	eligiblePeers := d.eligiblePeersOfChannel(routingFilter)

	// With the shift to per peer dissemination in FAB-15389, we must first check
	// that there are enough eligible peers to satisfy RequiredPeerCount.
	if (len(eligiblePeers)) < colAP.RequiredPeerCount() {
		return nil, errors.Errorf("required to disseminate to at least %d peers, but know of only %d eligible peers", colAP.RequiredPeerCount(), len(eligiblePeers))
	}

	// Group eligible peers by org so that we can disseminate across orgs first
	identitySetsByOrg := d.identitiesOfEligiblePeersByOrg(eligiblePeers, colAP)

	// peerEndpoints are used for dissemination debug only
	peerEndpoints := map[string]string{}
	for _, peer := range eligiblePeers {
		epToAdd := peer.Endpoint
		if epToAdd == "" {
			epToAdd = peer.InternalEndpoint
		}
		peerEndpoints[string(peer.PKIid)] = epToAdd
	}

	// Initialize maximumPeerRemainingCount and requiredPeerRemainingCount,
	// these will be decremented until we've selected enough peers for dissemination
	maximumPeerRemainingCount := colAP.MaximumPeerCount()
	requiredPeerRemainingCount := colAP.RequiredPeerCount()

	remainingPeersAcrossOrgs := []api.PeerIdentityInfo{}
	selectedPeerEndpointsForDebug := []string{}

	rand.Seed(time.Now().Unix())

	// PHASE 1 - Select one peer from each eligible org
	if maximumPeerRemainingCount > 0 {
		for _, selectionPeersForOrg := range identitySetsByOrg {

			// Peers are tagged as a required peer (acksRequired=1) for RequiredPeerCount up front before dissemination.
			// TODO It would be better to attempt dissemination to MaxPeerCount first, and then verify that enough sends were acknowledged to meet RequiredPeerCount.
			acksRequired := 1
			if requiredPeerRemainingCount == 0 {
				acksRequired = 0
			}

			selectedPeerIndex := rand.Intn(len(selectionPeersForOrg))
			peer2SendPerOrg := selectionPeersForOrg[selectedPeerIndex]
			selectedPeerEndpointsForDebug = append(selectedPeerEndpointsForDebug, peerEndpoints[string(peer2SendPerOrg.PKIId)])
			sc := gossip2.SendCriteria{
				Timeout:  d.pushAckTimeout,
				Channel:  gossipCommon.ChainID(d.chainID),
				MaxPeers: 1,
				MinAck:   acksRequired,
				IsEligible: func(member discovery.NetworkMember) bool {
					return bytes.Equal(member.PKIid, peer2SendPerOrg.PKIId)
				},
			}
			disseminationPlan = append(disseminationPlan, &dissemination{
				criteria: sc,
				msg: &proto.SignedGossipMessage{
					Envelope:      proto2.Clone(pvtDataMsg.Envelope).(*proto.Envelope),
					GossipMessage: proto2.Clone(pvtDataMsg.GossipMessage).(*proto.GossipMessage),
				},
			})

			// Add unselected peers to remainingPeersAcrossOrgs
			for i, peer := range selectionPeersForOrg {
				if i != selectedPeerIndex {
					remainingPeersAcrossOrgs = append(remainingPeersAcrossOrgs, peer)
				}
			}

			if requiredPeerRemainingCount > 0 {
				requiredPeerRemainingCount--
			}

			maximumPeerRemainingCount--
			if maximumPeerRemainingCount == 0 {
				logger.Debug("MaximumPeerCount satisfied")
				logger.Debugf("Disseminating private RWSet for TxID [%s] namespace [%s] collection [%s] to peers: %v", m.TxId, m.Namespace, m.CollectionName, selectedPeerEndpointsForDebug)
				return disseminationPlan, nil
			}
		}
	}

	// PHASE 2 - Select additional peers to satisfy colAP.MaximumPeerCount() if there are still peers in the remainingPeersAcrossOrgs pool
	numRemainingPeersToSelect := maximumPeerRemainingCount
	if len(remainingPeersAcrossOrgs) < maximumPeerRemainingCount {
		numRemainingPeersToSelect = len(remainingPeersAcrossOrgs)
	}
	if numRemainingPeersToSelect > 0 {
		logger.Debugf("MaximumPeerCount not yet satisfied after picking one peer per org, selecting %d more peer(s) for dissemination", numRemainingPeersToSelect)
	}
	for maximumPeerRemainingCount > 0 && len(remainingPeersAcrossOrgs) > 0 {
		required := 1
		if requiredPeerRemainingCount == 0 {
			required = 0
		}
		selectedPeerIndex := rand.Intn(len(remainingPeersAcrossOrgs))
		peer2Send := remainingPeersAcrossOrgs[selectedPeerIndex]
		selectedPeerEndpointsForDebug = append(selectedPeerEndpointsForDebug, peerEndpoints[string(peer2Send.PKIId)])
		sc := gossip2.SendCriteria{
			Timeout:  d.pushAckTimeout,
			Channel:  gossipCommon.ChainID(d.chainID),
			MaxPeers: 1,
			MinAck:   required,
			IsEligible: func(member discovery.NetworkMember) bool {
				return bytes.Equal(member.PKIid, peer2Send.PKIId)
			},
		}
		disseminationPlan = append(disseminationPlan, &dissemination{
			criteria: sc,
			msg: &proto.SignedGossipMessage{
				Envelope:      proto2.Clone(pvtDataMsg.Envelope).(*proto.Envelope),
				GossipMessage: proto2.Clone(pvtDataMsg.GossipMessage).(*proto.GossipMessage),
			},
		})
		if requiredPeerRemainingCount > 0 {
			requiredPeerRemainingCount--
		}

		maximumPeerRemainingCount--

		// remove the selected peer from remaining peers
		remainingPeersAcrossOrgs = append(remainingPeersAcrossOrgs[:selectedPeerIndex], remainingPeersAcrossOrgs[selectedPeerIndex+1:]...)
	}

	logger.Debugf("Disseminating private RWSet for TxID [%s] namespace [%s] collection [%s] to peers: %v", m.TxId, m.Namespace, m.CollectionName, selectedPeerEndpointsForDebug)
	return disseminationPlan, nil
}

// identitiesOfEligiblePeersByOrg returns the peers eligible for a collection (aka PeerIdentitySet) grouped in a hash map keyed by orgid
func (d *distributorImpl) identitiesOfEligiblePeersByOrg(eligiblePeers []discovery.NetworkMember, colAP privdata.CollectionAccessPolicy) map[string]api.PeerIdentitySet {
	return d.gossipAdapter.IdentityInfo().
		Filter(func(info api.PeerIdentityInfo) bool {
			for _, orgID := range colAP.MemberOrgs() {
				if bytes.Equal(info.Organization, []byte(orgID)) {
					return true
				}
			}
			// peer not in the org
			return false
		}).Filter(func(info api.PeerIdentityInfo) bool {
		for _, peer := range eligiblePeers {
			if bytes.Equal(info.PKIId, peer.PKIid) {
				return true
			}
		}
		// peer not in the channel
		return false
	}).ByOrg()
}

func (d *distributorImpl) eligiblePeersOfChannel(routingFilter filter.RoutingFilter) []discovery.NetworkMember {
	var eligiblePeers []discovery.NetworkMember
	for _, peer := range d.gossipAdapter.PeersOfChannel(gossipCommon.ChainID(d.chainID)) {
		if routingFilter(peer) {
			eligiblePeers = append(eligiblePeers, peer)
		}
	}
	return eligiblePeers
}

func (d *distributorImpl) disseminate(disseminationPlan []*dissemination) error {
	var failures uint32
	var wg sync.WaitGroup
	wg.Add(len(disseminationPlan))
	start := time.Now()
	for _, dis := range disseminationPlan {
		go func(dis *dissemination) {
			defer wg.Done()
			defer d.reportSendDuration(start)
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
		return errors.Errorf("Failed disseminating %d out of %d private dissemination plans", failureCount, len(disseminationPlan))
	}
	return nil
}

func (d *distributorImpl) reportSendDuration(startTime time.Time) {
	d.metrics.SendDuration.With("channel", d.chainID).Observe(time.Since(startTime).Seconds())
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
