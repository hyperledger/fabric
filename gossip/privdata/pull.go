/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privdata

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"time"

	protosgossip "github.com/hyperledger/fabric-protos-go/gossip"
	commonutil "github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/common/privdata"
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/comm"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/gossip/filter"
	"github.com/hyperledger/fabric/gossip/metrics"
	privdatacommon "github.com/hyperledger/fabric/gossip/privdata/common"
	"github.com/hyperledger/fabric/gossip/protoext"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
)

const (
	membershipPollingBackoff    = time.Second
	responseWaitTime            = time.Second * 5
	maxMembershipPollIterations = 5
)

// Dig2PvtRWSetWithConfig
type Dig2PvtRWSetWithConfig map[privdatacommon.DigKey]*util.PrivateRWSetWithConfig

// PrivateDataRetriever interface which defines API capable
// of retrieving required private data
type PrivateDataRetriever interface {
	// CollectionRWSet returns the bytes of CollectionPvtReadWriteSet for a given txID and collection from the transient store
	CollectionRWSet(dig []*protosgossip.PvtDataDigest, blockNum uint64) (Dig2PvtRWSetWithConfig, bool, error)
}

// gossip defines capabilities that the gossip module gives the Coordinator
type gossip interface {
	// Send sends a message to remote peers
	Send(msg *protosgossip.GossipMessage, peers ...*comm.RemotePeer)

	// PeersOfChannel returns the NetworkMembers considered alive
	// and also subscribed to the channel given
	PeersOfChannel(common.ChannelID) []discovery.NetworkMember

	// PeerFilter receives a SubChannelSelectionCriteria and returns a RoutingFilter that selects
	// only peer identities that match the given criteria, and that they published their channel participation
	PeerFilter(channel common.ChannelID, messagePredicate api.SubChannelSelectionCriteria) (filter.RoutingFilter, error)

	// Accept returns a dedicated read-only channel for messages sent by other nodes that match a certain predicate.
	// If passThrough is false, the messages are processed by the gossip layer beforehand.
	// If passThrough is true, the gossip layer doesn't intervene and the messages
	// can be used to send a reply back to the sender
	Accept(acceptor common.MessageAcceptor, passThrough bool) (<-chan *protosgossip.GossipMessage, <-chan protoext.ReceivedMessage)
}

type puller struct {
	logger        util.Logger
	metrics       *metrics.PrivdataMetrics
	pubSub        *util.PubSub
	stopChan      chan struct{}
	msgChan       <-chan protoext.ReceivedMessage
	channel       string
	cs            privdata.CollectionStore
	btlPullMargin uint64
	gossip
	PrivateDataRetriever
	CollectionAccessFactory
}

// NewPuller creates new private data puller
func NewPuller(metrics *metrics.PrivdataMetrics, cs privdata.CollectionStore, g gossip,
	dataRetriever PrivateDataRetriever, factory CollectionAccessFactory, channel string, btlPullMargin uint64) *puller {
	p := &puller{
		logger:                  logger.With("channel", channel),
		metrics:                 metrics,
		pubSub:                  util.NewPubSub(),
		stopChan:                make(chan struct{}),
		channel:                 channel,
		cs:                      cs,
		btlPullMargin:           btlPullMargin,
		gossip:                  g,
		PrivateDataRetriever:    dataRetriever,
		CollectionAccessFactory: factory,
	}
	_, p.msgChan = p.Accept(func(o interface{}) bool {
		msg := o.(protoext.ReceivedMessage).GetGossipMessage()
		if !bytes.Equal(msg.Channel, []byte(p.channel)) {
			return false
		}
		return protoext.IsPrivateDataMsg(msg.GossipMessage)
	}, true)
	go p.listen()
	return p
}

func (p *puller) listen() {
	for {
		select {
		case <-p.stopChan:
			return
		case msg := <-p.msgChan:
			if msg == nil {
				// comm module stopped, hence this channel
				// closed
				return
			}
			if msg.GetGossipMessage().GetPrivateRes() != nil {
				p.handleResponse(msg)
			}
			if msg.GetGossipMessage().GetPrivateReq() != nil {
				p.handleRequest(msg)
			}
		}
	}
}

func (p *puller) handleRequest(message protoext.ReceivedMessage) {
	p.logger.Debug("Got", message.GetGossipMessage(), "from", message.GetConnectionInfo().Endpoint)
	message.Respond(&protosgossip.GossipMessage{
		Channel: []byte(p.channel),
		Tag:     protosgossip.GossipMessage_CHAN_ONLY,
		Nonce:   message.GetGossipMessage().Nonce,
		Content: &protosgossip.GossipMessage_PrivateRes{
			PrivateRes: &protosgossip.RemotePvtDataResponse{
				Elements: p.createResponse(message),
			},
		},
	})
}

func (p *puller) createResponse(message protoext.ReceivedMessage) []*protosgossip.PvtDataElement {
	authInfo := message.GetConnectionInfo().Auth
	var returned []*protosgossip.PvtDataElement
	connectionEndpoint := message.GetConnectionInfo().Endpoint

	defer func() {
		p.logger.Debug("Returning", connectionEndpoint, len(returned), "elements")
	}()

	msg := message.GetGossipMessage()
	// group all digest by block number
	block2dig := groupDigestsByBlockNum(msg.GetPrivateReq().Digests)

	for blockNum, digests := range block2dig {
		start := time.Now()
		dig2rwSets, wasFetchedFromLedger, err := p.CollectionRWSet(digests, blockNum)
		p.metrics.RetrieveDuration.With("channel", p.channel).Observe(time.Since(start).Seconds())
		if err != nil {
			p.logger.Warningf("could not obtain private collection rwset for block %d, because of %s, continue...", blockNum, err)
			continue
		}
		returned = append(returned, p.filterNotEligible(dig2rwSets, wasFetchedFromLedger, protoutil.SignedData{
			Identity:  message.GetConnectionInfo().Identity,
			Data:      authInfo.SignedData,
			Signature: authInfo.Signature,
		}, connectionEndpoint)...)
	}
	return returned
}

// groupDigestsByBlockNum group all digest by block sequence number
func groupDigestsByBlockNum(digests []*protosgossip.PvtDataDigest) map[uint64][]*protosgossip.PvtDataDigest {
	results := make(map[uint64][]*protosgossip.PvtDataDigest)
	for _, dig := range digests {
		results[dig.BlockSeq] = append(results[dig.BlockSeq], dig)
	}
	return results
}

func (p *puller) handleResponse(message protoext.ReceivedMessage) {
	msg := message.GetGossipMessage().GetPrivateRes()
	p.logger.Debug("Got", msg, "from", message.GetConnectionInfo().Endpoint)
	for _, el := range msg.Elements {
		if el.Digest == nil {
			p.logger.Warning("Got nil digest from", message.GetConnectionInfo().Endpoint, "aborting")
			return
		}
		hash, err := hashDigest(el.Digest)
		if err != nil {
			p.logger.Warning("Failed hashing digest from", message.GetConnectionInfo().Endpoint, "aborting")
			return
		}
		p.pubSub.Publish(hash, el)
	}
}

// hashDigest returns the SHA256 representation of the PvtDataDigest's bytes
func hashDigest(dig *protosgossip.PvtDataDigest) (string, error) {
	b, err := protoutil.Marshal(dig)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(commonutil.ComputeSHA256(b)), nil
}

func (p *puller) waitForMembership() []discovery.NetworkMember {
	polIteration := 0
	for {
		members := p.PeersOfChannel(common.ChannelID(p.channel))
		if len(members) != 0 {
			return members
		}
		polIteration++
		if polIteration == maxMembershipPollIterations {
			return nil
		}
		time.Sleep(membershipPollingBackoff)
	}
}

func (p *puller) fetch(dig2src dig2sources) (*privdatacommon.FetchedPvtDataContainer, error) {
	// computeFilters returns a map from a digest to a routing filter
	dig2Filter, err := p.computeFilters(dig2src)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return p.fetchPrivateData(dig2Filter)
}

func (p *puller) FetchReconciledItems(dig2collectionConfig privdatacommon.Dig2CollectionConfig) (*privdatacommon.FetchedPvtDataContainer, error) {
	// computeFilters returns a map from a digest to a routing filter
	dig2Filter, err := p.computeReconciliationFilters(dig2collectionConfig)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return p.fetchPrivateData(dig2Filter)
}

func (p *puller) fetchPrivateData(dig2Filter digestToFilterMapping) (*privdatacommon.FetchedPvtDataContainer, error) {
	// Get a list of peers per channel
	allFilters := dig2Filter.flattenFilterValues()
	members := p.waitForMembership()
	p.logger.Debug("Total members in channel:", members)
	members = filter.AnyMatch(members, allFilters...)
	p.logger.Debug("Total members that fit some digest:", members)
	if len(members) == 0 {
		p.logger.Warning("Do not know any peer in the channel(", p.channel, ") that matches the policies , aborting")
		return nil, errors.New("Empty membership")
	}
	members = randomizeMemberList(members)
	res := &privdatacommon.FetchedPvtDataContainer{}
	// Distribute requests to peers, and obtain subscriptions for all their messages
	// matchDigestToPeer returns a map from a peer to the digests which we would ask it for
	var peer2digests peer2Digests
	// We expect all private RWSets represented as digests to be collected
	itemsLeftToCollect := len(dig2Filter)
	// As long as we still have some data to collect and new members to ask the data for:
	for itemsLeftToCollect > 0 && len(members) > 0 {
		purgedPvt := p.getPurgedCollections(members, dig2Filter)
		// Need to remove purged digest from mapping
		for _, dig := range purgedPvt {
			res.PurgedElements = append(res.PurgedElements, &protosgossip.PvtDataDigest{
				TxId:       dig.TxId,
				BlockSeq:   dig.BlockSeq,
				SeqInBlock: dig.SeqInBlock,
				Namespace:  dig.Namespace,
				Collection: dig.Collection,
			})
			// remove digest so we won't even try to pull purged data
			delete(dig2Filter, dig)
			itemsLeftToCollect--
		}

		if itemsLeftToCollect == 0 {
			p.logger.Debug("No items left to collect")
			return res, nil
		}

		peer2digests, members = p.assignDigestsToPeers(members, dig2Filter)
		if len(peer2digests) == 0 {
			p.logger.Warningf("No available peers for digests request, "+
				"cannot pull missing private data for following digests [%+v], peer membership: [%+v]",
				dig2Filter.digests(), members)
			return res, nil
		}

		p.logger.Debug("Matched", len(dig2Filter), "digests to", len(peer2digests), "peer(s)")
		subscriptions := p.scatterRequests(peer2digests)
		responses := p.gatherResponses(subscriptions)
		for _, resp := range responses {
			if len(resp.Payload) == 0 {
				p.logger.Debug("Got empty response for", resp.Digest)
				continue
			}
			delete(dig2Filter, privdatacommon.DigKey{
				TxId:       resp.Digest.TxId,
				BlockSeq:   resp.Digest.BlockSeq,
				SeqInBlock: resp.Digest.SeqInBlock,
				Namespace:  resp.Digest.Namespace,
				Collection: resp.Digest.Collection,
			})
			itemsLeftToCollect--
		}
		res.AvailableElements = append(res.AvailableElements, responses...)
	}
	return res, nil
}

func (p *puller) gatherResponses(subscriptions []util.Subscription) []*protosgossip.PvtDataElement {
	var res []*protosgossip.PvtDataElement
	privateElements := make(chan *protosgossip.PvtDataElement, len(subscriptions))
	var wg sync.WaitGroup
	wg.Add(len(subscriptions))
	start := time.Now()
	// Listen for all subscriptions, and add then into a single channel
	for _, sub := range subscriptions {
		go func(sub util.Subscription) {
			defer wg.Done()
			el, err := sub.Listen()
			if err != nil {
				return
			}
			privateElements <- el.(*protosgossip.PvtDataElement)
			p.metrics.PullDuration.With("channel", p.channel).Observe(time.Since(start).Seconds())
		}(sub)
	}
	// Wait for all subscriptions to either return, or time out
	wg.Wait()
	// Close the channel, to not block when we iterate it.
	close(privateElements)
	// Aggregate elements to return them as a slice
	for el := range privateElements {
		res = append(res, el)
	}
	return res
}

func (p *puller) scatterRequests(peersDigestMapping peer2Digests) []util.Subscription {
	var subscriptions []util.Subscription
	for peer, digests := range peersDigestMapping {
		msg := &protosgossip.GossipMessage{
			Tag:     protosgossip.GossipMessage_CHAN_ONLY,
			Channel: []byte(p.channel),
			Nonce:   util.RandomUInt64(),
			Content: &protosgossip.GossipMessage_PrivateReq{
				PrivateReq: &protosgossip.RemotePvtDataRequest{
					Digests: digestsAsPointerSlice(digests),
				},
			},
		}

		// Subscribe to all digests prior to sending them
		for _, dig := range msg.GetPrivateReq().Digests {
			hash, err := hashDigest(dig)
			if err != nil {
				// Shouldn't happen as we just built this message ourselves
				p.logger.Warning("Failed creating digest", err)
				continue
			}
			sub := p.pubSub.Subscribe(hash, responseWaitTime)
			subscriptions = append(subscriptions, sub)
		}
		p.logger.Debug("Sending", peer.endpoint, "request", msg.GetPrivateReq().Digests)
		p.Send(msg, peer.AsRemotePeer())
	}
	return subscriptions
}

type (
	peer2Digests      map[remotePeer][]protosgossip.PvtDataDigest
	noneSelectedPeers []discovery.NetworkMember
)

func (p *puller) assignDigestsToPeers(members []discovery.NetworkMember, dig2Filter digestToFilterMapping) (peer2Digests, noneSelectedPeers) {
	if p.logger.IsEnabledFor(zapcore.DebugLevel) {
		p.logger.Debug("Matching", members, "to", dig2Filter.String())
	}
	res := make(map[remotePeer][]protosgossip.PvtDataDigest)
	// Create a mapping between peer and digests to ask for
	for dig, collectionFilter := range dig2Filter {
		// Find a peer that is a preferred peer
		selectedPeer := filter.First(members, collectionFilter.preferredPeer)
		if selectedPeer == nil {
			p.logger.Debug("No preferred peer found for", dig)
			// Find some peer that is in the collection
			selectedPeer = filter.First(members, collectionFilter.anyPeer)
		}
		if selectedPeer == nil {
			p.logger.Debug("No peer matches txID", dig.TxId, "collection", dig.Collection)
			continue
		}
		// Add the peer to the mapping from peer to digest slice
		peer := remotePeer{pkiID: string(selectedPeer.PKIID), endpoint: selectedPeer.Endpoint}
		res[peer] = append(res[peer], protosgossip.PvtDataDigest{
			TxId:       dig.TxId,
			BlockSeq:   dig.BlockSeq,
			SeqInBlock: dig.SeqInBlock,
			Namespace:  dig.Namespace,
			Collection: dig.Collection,
		})
	}

	var noneSelectedPeers []discovery.NetworkMember
	for _, member := range members {
		peer := remotePeer{endpoint: member.PreferredEndpoint(), pkiID: string(member.PKIid)}
		if _, selected := res[peer]; !selected {
			noneSelectedPeers = append(noneSelectedPeers, member)
		}
	}

	return res, noneSelectedPeers
}

type collectionRoutingFilter struct {
	anyPeer       filter.RoutingFilter
	preferredPeer filter.RoutingFilter
}

type digestToFilterMapping map[privdatacommon.DigKey]collectionRoutingFilter

func (dig2f digestToFilterMapping) flattenFilterValues() []filter.RoutingFilter {
	var filters []filter.RoutingFilter
	for _, f := range dig2f {
		filters = append(filters, f.preferredPeer)
		filters = append(filters, f.anyPeer)
	}
	return filters
}

func (dig2f digestToFilterMapping) digests() []protosgossip.PvtDataDigest {
	var digs []protosgossip.PvtDataDigest
	for d := range dig2f {
		digs = append(digs, protosgossip.PvtDataDigest{
			TxId:       d.TxId,
			BlockSeq:   d.BlockSeq,
			SeqInBlock: d.SeqInBlock,
			Namespace:  d.Namespace,
			Collection: d.Collection,
		})
	}
	return digs
}

// String returns a string representation of t he digestToFilterMapping
func (dig2f digestToFilterMapping) String() string {
	var buffer bytes.Buffer
	collection2TxID := make(map[string][]string)
	for dig := range dig2f {
		collection2TxID[dig.Collection] = append(collection2TxID[dig.Collection], dig.TxId)
	}
	for col, txIDs := range collection2TxID {
		buffer.WriteString(fmt.Sprintf("{%s: %v}", col, txIDs))
	}
	return buffer.String()
}

func (p *puller) computeFilters(dig2src dig2sources) (digestToFilterMapping, error) {
	filters := make(map[privdatacommon.DigKey]collectionRoutingFilter)
	for digest, sources := range dig2src {
		anyPeerInCollection, err := p.getLatestCollectionConfigRoutingFilter(digest.Namespace, digest.Collection)
		if err != nil {
			return nil, errors.WithStack(err)
		}

		sources := sources
		endorserPeer, err := p.PeerFilter(common.ChannelID(p.channel), func(peerSignature api.PeerSignature) bool {
			for _, endorsement := range sources {
				if bytes.Equal(endorsement.Endorser, []byte(peerSignature.PeerIdentity)) {
					return true
				}
			}
			return false
		})
		if err != nil {
			return nil, errors.WithStack(err)
		}

		filters[digest] = collectionRoutingFilter{
			anyPeer:       anyPeerInCollection,
			preferredPeer: endorserPeer,
		}
	}
	return filters, nil
}

func (p *puller) computeReconciliationFilters(dig2collectionConfig privdatacommon.Dig2CollectionConfig) (digestToFilterMapping, error) {
	filters := make(map[privdatacommon.DigKey]collectionRoutingFilter)
	for digest, originalCollectionConfig := range dig2collectionConfig {
		anyPeerInCollection, err := p.getLatestCollectionConfigRoutingFilter(digest.Namespace, digest.Collection)
		if err != nil {
			return nil, err
		}

		originalConfigFilter, err := p.cs.AccessFilter(p.channel, originalCollectionConfig.MemberOrgsPolicy)
		if err != nil {
			return nil, err
		}
		if originalConfigFilter == nil {
			return nil, errors.Errorf("Failed obtaining original collection filter for channel %s, config %s", p.channel, digest.Collection)
		}

		// get peers that were in the collection config while the missing data was created
		peerFromDataCreation, err := p.getMatchAllRoutingFilter(originalConfigFilter)
		if err != nil {
			return nil, err
		}

		// prefer peers that are in the collection from the time the data was created rather than ones that were added later.
		// the assumption is that the longer the peer is in the collection config, the chances it has the data are bigger.
		preferredPeer := func(member discovery.NetworkMember) bool {
			return peerFromDataCreation(member) && anyPeerInCollection(member)
		}

		filters[digest] = collectionRoutingFilter{
			anyPeer:       anyPeerInCollection,
			preferredPeer: preferredPeer,
		}
	}
	return filters, nil
}

func (p *puller) getLatestCollectionConfigRoutingFilter(chaincode string, collection string) (filter.RoutingFilter, error) {
	cc := privdata.CollectionCriteria{
		Channel:    p.channel,
		Collection: collection,
		Namespace:  chaincode,
	}

	latestCollectionConfig, err := p.cs.RetrieveCollectionAccessPolicy(cc)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed obtaining collection policy for channel %s, chaincode %s, config %s", p.channel, chaincode, collection)
	}

	filt := latestCollectionConfig.AccessFilter()
	if filt == nil {
		return nil, errors.Errorf("Failed obtaining collection filter for channel %s, chaincode %s, collection %s", p.channel, chaincode, collection)
	}

	anyPeerInCollection, err := p.getMatchAllRoutingFilter(filt)
	if err != nil {
		return nil, err
	}

	return anyPeerInCollection, nil
}

func (p *puller) getMatchAllRoutingFilter(filt privdata.Filter) (filter.RoutingFilter, error) {
	routingFilter, err := p.PeerFilter(common.ChannelID(p.channel), func(peerSignature api.PeerSignature) bool {
		return filt(protoutil.SignedData{
			Signature: peerSignature.Signature,
			Identity:  peerSignature.PeerIdentity,
			Data:      peerSignature.Message,
		})
	})
	return routingFilter, err
}

func (p *puller) getPurgedCollections(members []discovery.NetworkMember, dig2Filter digestToFilterMapping) []privdatacommon.DigKey {
	var res []privdatacommon.DigKey
	for dig := range dig2Filter {
		purged, err := p.purgedFilter(dig)
		if err != nil {
			p.logger.Debugf("Failed to obtain purged filter for digest %v error %v", dig, err)
			continue
		}

		membersWithPurgedData := filter.AnyMatch(members, purged)
		// at least one peer already purged the data
		if len(membersWithPurgedData) > 0 {
			p.logger.Debugf("Private data on channel [%s], chaincode [%s], collection name [%s] for txID = [%s],"+
				"has been purged at peers [%v]", p.channel, dig.Namespace,
				dig.Collection, dig.TxId, membersWithPurgedData)
			res = append(res, dig)
		}
	}
	return res
}

func (p *puller) purgedFilter(dig privdatacommon.DigKey) (filter.RoutingFilter, error) {
	cc := privdata.CollectionCriteria{
		Channel:    p.channel,
		Collection: dig.Collection,
		Namespace:  dig.Namespace,
	}
	colPersistConfig, err := p.cs.RetrieveCollectionPersistenceConfigs(cc)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return func(peer discovery.NetworkMember) bool {
		if peer.Properties == nil {
			p.logger.Debugf("No properties provided for peer %s", peer.Endpoint)
			return false
		}
		// BTL equals to zero has semantic of never expires
		if colPersistConfig.BlockToLive() == uint64(0) {
			return false
		}
		// handle overflow
		expirationSeqNum := addWithOverflow(dig.BlockSeq, colPersistConfig.BlockToLive())
		peerLedgerHeightWithMargin := addWithOverflow(peer.Properties.LedgerHeight, p.btlPullMargin)

		isPurged := peerLedgerHeightWithMargin >= expirationSeqNum
		if isPurged {
			p.logger.Debugf("skipping peer [%s], since pvt for channel [%s], txID = [%s], "+
				"collection [%s] has been purged or will soon be purged, BTL=[%d]",
				peer.Endpoint, p.channel, dig.TxId, cc.Collection, colPersistConfig.BlockToLive())
		}
		return isPurged
	}, nil
}

func (p *puller) filterNotEligible(dig2rwSets Dig2PvtRWSetWithConfig, shouldCheckLatestConfig bool, signedData protoutil.SignedData, endpoint string) []*protosgossip.PvtDataElement {
	var returned []*protosgossip.PvtDataElement
	for d, rwSets := range dig2rwSets {
		if rwSets == nil {
			p.logger.Errorf("No private rwset for [%s] channel, chaincode [%s], collection [%s], txID = [%s] is available, skipping...",
				p.channel, d.Namespace, d.Collection, d.TxId)
			continue
		}
		p.logger.Debug("Found", len(rwSets.RWSet), "for TxID", d.TxId, ", collection", d.Collection, "for", endpoint)
		if len(rwSets.RWSet) == 0 {
			continue
		}

		eligibleForCollection := shouldCheckLatestConfig && p.isEligibleByLatestConfig(p.channel, d.Collection, d.Namespace, signedData)

		if !eligibleForCollection {
			colAP, err := p.AccessPolicy(rwSets.CollectionConfig, p.channel)
			if err != nil {
				p.logger.Debug("No policy found for channel", p.channel, ", collection", d.Collection, "txID", d.TxId, ":", err, "skipping...")
				continue
			}
			colFilter := colAP.AccessFilter()
			if colFilter == nil {
				p.logger.Debug("Collection ", d.Collection, " has no access filter, txID", d.TxId, "skipping...")
				continue
			}
			eligibleForCollection = colFilter(signedData)
		}

		if !eligibleForCollection {
			p.logger.Debug("Peer", endpoint, "isn't eligible for txID", d.TxId, "at collection", d.Collection)
			continue
		}

		returned = append(returned, &protosgossip.PvtDataElement{
			Digest: &protosgossip.PvtDataDigest{
				TxId:       d.TxId,
				BlockSeq:   d.BlockSeq,
				Collection: d.Collection,
				Namespace:  d.Namespace,
				SeqInBlock: d.SeqInBlock,
			},
			Payload: util.PrivateRWSets(rwSets.RWSet...),
		})
	}
	return returned
}

func (p *puller) isEligibleByLatestConfig(channel string, collection string, chaincode string, signedData protoutil.SignedData) bool {
	cc := privdata.CollectionCriteria{
		Channel:    channel,
		Collection: collection,
		Namespace:  chaincode,
	}

	latestCollectionConfig, err := p.cs.RetrieveCollectionAccessPolicy(cc)
	if err != nil {
		return false
	}

	collectionFilter := latestCollectionConfig.AccessFilter()
	return collectionFilter(signedData)
}

func randomizeMemberList(members []discovery.NetworkMember) []discovery.NetworkMember {
	rand.Seed(time.Now().UnixNano())
	res := make([]discovery.NetworkMember, len(members))
	for i, j := range rand.Perm(len(members)) {
		res[i] = members[j]
	}
	return res
}

func digestsAsPointerSlice(digests []protosgossip.PvtDataDigest) []*protosgossip.PvtDataDigest {
	res := make([]*protosgossip.PvtDataDigest, len(digests))
	for i, dig := range digests {
		// re-introduce dig variable to allocate
		// new address for each iteration
		dig := dig
		res[i] = &dig
	}
	return res
}

type remotePeer struct {
	endpoint string
	pkiID    string
}

// AsRemotePeer converts this remotePeer to comm.RemotePeer
func (rp remotePeer) AsRemotePeer() *comm.RemotePeer {
	return &comm.RemotePeer{
		PKIID:    common.PKIidType(rp.pkiID),
		Endpoint: rp.endpoint,
	}
}

func addWithOverflow(a uint64, b uint64) uint64 {
	res := a + b
	if res < a {
		return math.MaxUint64
	}
	return res
}
