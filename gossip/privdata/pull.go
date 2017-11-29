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
	"time"

	"github.com/hyperledger/fabric/core/common/privdata"
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/comm"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/gossip/filter"
	"github.com/hyperledger/fabric/gossip/util"
	fcommon "github.com/hyperledger/fabric/protos/common"
	proto "github.com/hyperledger/fabric/protos/gossip"
	"github.com/op/go-logging"
	"github.com/pkg/errors"
)

const (
	membershipPollingBackoff    = time.Second
	responseWaitTime            = time.Second * 5
	maxMembershipPollIterations = 5
)

// PrivateDataRetriever interfacce which defines API capable
// of retrieving required private data
type PrivateDataRetriever interface {
	// CollectionRWSet returns the bytes of CollectionPvtReadWriteSet for a given txID and collection from the transient store
	CollectionRWSet(dig *proto.PvtDataDigest) []util.PrivateRWSet
}

// gossip defines capabilities that the gossip module gives the Coordinator
type gossip interface {
	// Send sends a message to remote peers
	Send(msg *proto.GossipMessage, peers ...*comm.RemotePeer)

	// PeersOfChannel returns the NetworkMembers considered alive
	// and also subscribed to the channel given
	PeersOfChannel(common.ChainID) []discovery.NetworkMember

	// PeerFilter receives a SubChannelSelectionCriteria and returns a RoutingFilter that selects
	// only peer identities that match the given criteria, and that they published their channel participation
	PeerFilter(channel common.ChainID, messagePredicate api.SubChannelSelectionCriteria) (filter.RoutingFilter, error)

	// Accept returns a dedicated read-only channel for messages sent by other nodes that match a certain predicate.
	// If passThrough is false, the messages are processed by the gossip layer beforehand.
	// If passThrough is true, the gossip layer doesn't intervene and the messages
	// can be used to send a reply back to the sender
	Accept(acceptor common.MessageAcceptor, passThrough bool) (<-chan *proto.GossipMessage, <-chan proto.ReceivedMessage)
}

type puller struct {
	pubSub   *util.PubSub
	stopChan chan struct{}
	msgChan  <-chan proto.ReceivedMessage
	channel  string
	cs       privdata.CollectionStore
	gossip
	PrivateDataRetriever
}

// NewPuller creates new private data puller
func NewPuller(cs privdata.CollectionStore, g gossip, dataRetriever PrivateDataRetriever, channel string) *puller {
	p := &puller{
		pubSub:               util.NewPubSub(),
		stopChan:             make(chan struct{}),
		channel:              channel,
		cs:                   cs,
		gossip:               g,
		PrivateDataRetriever: dataRetriever,
	}
	_, p.msgChan = p.Accept(func(o interface{}) bool {
		msg := o.(proto.ReceivedMessage).GetGossipMessage()
		if !bytes.Equal(msg.Channel, []byte(p.channel)) {
			return false
		}
		return msg.IsPrivateDataMsg()
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

func (p *puller) handleRequest(message proto.ReceivedMessage) {
	logger.Debug("Got", message.GetGossipMessage(), "from", message.GetConnectionInfo().Endpoint)
	message.Respond(&proto.GossipMessage{
		Channel: []byte(p.channel),
		Tag:     proto.GossipMessage_CHAN_ONLY,
		Nonce:   message.GetGossipMessage().Nonce,
		Content: &proto.GossipMessage_PrivateRes{
			PrivateRes: &proto.RemotePvtDataResponse{
				Elements: p.createResponse(message),
			},
		},
	})
}

func (p *puller) createResponse(message proto.ReceivedMessage) []*proto.PvtDataElement {
	authInfo := message.GetConnectionInfo().Auth
	var returned []*proto.PvtDataElement
	defer func() {
		logger.Debug("Returning", message.GetConnectionInfo().Endpoint, len(returned), "elements")
	}()
	msg := message.GetGossipMessage()
	for _, dig := range msg.GetPrivateReq().Digests {
		colAP, err := p.cs.RetrieveCollectionAccessPolicy(fcommon.CollectionCriteria{
			Channel:    p.channel,
			Collection: dig.Collection,
			TxId:       dig.TxId,
			Namespace:  dig.Namespace,
		})
		if err != nil {
			logger.Debug("No policy found for channel", p.channel, ", collection", dig.Collection, "txID", dig.TxId, ":", err, "skipping...")
			continue
		}
		colFilter := colAP.AccessFilter()
		if colFilter == nil {
			logger.Debug("Collection ", dig.Collection, " has no access filter, txID", dig.TxId, "skipping...")
			continue
		}
		eligibleForCollection := colFilter(fcommon.SignedData{
			Identity:  message.GetConnectionInfo().Identity,
			Data:      authInfo.SignedData,
			Signature: authInfo.Signature,
		})

		if !eligibleForCollection {
			logger.Debug("Peer", message.GetConnectionInfo().Endpoint, "isn't eligible for txID", dig.TxId, "at collection", dig.Collection)
			continue
		}

		rwSets := p.CollectionRWSet(dig)
		logger.Debug("Found", len(rwSets), "for TxID", dig.TxId, ", collection", dig.Collection, "for", message.GetConnectionInfo().Endpoint)
		if len(rwSets) == 0 {
			continue
		}
		returned = append(returned, &proto.PvtDataElement{
			Digest:  dig,
			Payload: util.PrivateRWSets(rwSets...),
		})
	}
	return returned
}

func (p *puller) handleResponse(message proto.ReceivedMessage) {
	msg := message.GetGossipMessage().GetPrivateRes()
	logger.Debug("Got", msg, "from", message.GetConnectionInfo().Endpoint)
	for _, el := range msg.Elements {
		if el.Digest == nil {
			logger.Warning("Got nil digest from", message.GetConnectionInfo().Endpoint, "aborting")
			return
		}
		hash, err := el.Digest.Hash()
		if err != nil {
			logger.Warning("Failed hashing digest from", message.GetConnectionInfo().Endpoint, "aborting")
			return
		}
		p.pubSub.Publish(hash, el)
	}
}

func (p *puller) waitForMembership() []discovery.NetworkMember {
	polIteration := 0
	var members []discovery.NetworkMember
	for len(members) == 0 {
		members = p.PeersOfChannel(common.ChainID(p.channel))
		polIteration++
		if polIteration == maxMembershipPollIterations {
			return nil
		}
		time.Sleep(membershipPollingBackoff)
	}
	return members
}

func (p *puller) fetch(dig2src dig2sources) ([]*proto.PvtDataElement, error) {
	// computeFilters returns a map from a digest to a routing filter
	dig2Filter, err := p.computeFilters(dig2src)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	// Get a list of peers per channel
	allFilters := dig2Filter.flattenFilterValues()
	members := p.waitForMembership()
	logger.Debug("Total members in channel:", members)
	members = filter.AnyMatch(members, allFilters...)
	logger.Debug("Total members that fit some digest:", members)
	if len(members) == 0 {
		logger.Warning("Do not know any peer in the channel(", p.channel, ") that matches the policies , aborting")
		return nil, errors.New("Empty membership")
	}
	members = randomizeMemberList(members)
	var res []*proto.PvtDataElement
	// Distribute requests to peers, and obtain subscriptions for all their messages
	// matchDigestToPeer returns a map from a peer to the digests which we would ask it for
	var peer2digests peer2Digests
	// We expect all private RWSets represented as digests to be collected
	itemsLeftToCollect := len(dig2Filter)
	// As long as we still have some data to collect and new members to ask the data for:
	for itemsLeftToCollect > 0 && len(members) > 0 {
		peer2digests, members = p.assignDigestsToPeers(members, dig2Filter)
		logger.Debug("Matched", len(dig2Filter), "digests to", len(peer2digests), "peer(s)")
		subscriptions := p.scatterRequests(members, peer2digests)
		responses := p.gatherResponses(subscriptions)
		for _, resp := range responses {
			logger.Debug("Got empty response for", resp.Digest)
			if len(resp.Payload) == 0 {
				continue
			}
			delete(dig2Filter, *resp.Digest)
			itemsLeftToCollect--
		}
		res = append(res, responses...)
	}
	return res, nil
}

func (p *puller) gatherResponses(subscriptions []util.Subscription) []*proto.PvtDataElement {
	var res []*proto.PvtDataElement
	privateElements := make(chan *proto.PvtDataElement, len(subscriptions))
	var wg sync.WaitGroup
	wg.Add(len(subscriptions))
	// Listen for all subscriptions, and add then into a single channel
	for _, sub := range subscriptions {
		go func(sub util.Subscription) {
			defer wg.Done()
			el, err := sub.Listen()
			if err != nil {
				return
			}
			privateElements <- el.(*proto.PvtDataElement)
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

func (p *puller) scatterRequests(members []discovery.NetworkMember, peersDigestMapping peer2Digests) []util.Subscription {
	var subscriptions []util.Subscription
	for peer, digests := range peersDigestMapping {
		msg := &proto.GossipMessage{
			Tag:     proto.GossipMessage_CHAN_ONLY,
			Channel: []byte(p.channel),
			Nonce:   util.RandomUInt64(),
			Content: &proto.GossipMessage_PrivateReq{
				PrivateReq: &proto.RemotePvtDataRequest{
					Digests: digestsAsPointerSlice(digests),
				},
			},
		}

		// Subscribe to all digests prior to sending them
		for _, dig := range msg.GetPrivateReq().Digests {
			hash, err := dig.Hash()
			if err != nil {
				// Shouldn't happen as we just built this message ourselves
				logger.Warning("Failed creating digest", err)
				continue
			}
			sub := p.pubSub.Subscribe(hash, responseWaitTime)
			subscriptions = append(subscriptions, sub)
		}
		logger.Debug("Sending", peer.endpoint, "request", msg.GetPrivateReq().Digests)
		p.Send(msg, peer.AsRemotePeer())
	}
	return subscriptions
}

type peer2Digests map[remotePeer][]proto.PvtDataDigest
type noneSelectedPeers []discovery.NetworkMember

func (p *puller) assignDigestsToPeers(members []discovery.NetworkMember, dig2Filter digestToFilterMapping) (peer2Digests, noneSelectedPeers) {
	if logger.IsEnabledFor(logging.DEBUG) {
		logger.Debug("Matching", members, "to", dig2Filter.String())
	}
	res := make(map[remotePeer][]proto.PvtDataDigest)
	// Create a mapping between peer and digests to ask for
	for dig, collectionFilter := range dig2Filter {
		// Find a peer that is an endorser
		selectedPeer := filter.First(members, collectionFilter.endorser)
		if selectedPeer == nil {
			logger.Debug("No endorser found for", dig)
			// Find some peer that is in the collection
			selectedPeer = filter.First(members, collectionFilter.anyPeer)
		}
		if selectedPeer == nil {
			logger.Debug("No peer matches txID", dig.TxId, "collection", dig.Collection)
			continue
		}
		// Add the peer to the mapping from peer to digest slice
		peer := remotePeer{pkiID: string(selectedPeer.PKIID), endpoint: selectedPeer.Endpoint}
		res[peer] = append(res[peer], dig)
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
	anyPeer  filter.RoutingFilter
	endorser filter.RoutingFilter
}

type digestToFilterMapping map[proto.PvtDataDigest]collectionRoutingFilter

func (dig2f digestToFilterMapping) flattenFilterValues() []filter.RoutingFilter {
	var filters []filter.RoutingFilter
	for _, f := range dig2f {
		filters = append(filters, f.endorser)
		filters = append(filters, f.anyPeer)
	}
	return filters
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
	filters := make(map[proto.PvtDataDigest]collectionRoutingFilter)
	for digest, sources := range dig2src {
		collection, err := p.cs.RetrieveCollectionAccessPolicy(fcommon.CollectionCriteria{
			Channel:    p.channel,
			TxId:       digest.TxId,
			Collection: digest.Collection,
			Namespace:  digest.Namespace,
		})
		if err != nil {
			return nil, errors.WithMessage(err, fmt.Sprintf("failed obtaining collection policy for channel %s, txID %s, collection %s", p.channel, digest.TxId, digest.Collection))
		}
		f := collection.AccessFilter()
		if f == nil {
			return nil, errors.Errorf("Failed obtaining collection filter for channel %s, txID %s, collection %s", p.channel, digest.TxId, digest.Collection)
		}
		anyPeerInCollection, err := p.PeerFilter(common.ChainID(p.channel), func(peerSignature api.PeerSignature) bool {
			return f(fcommon.SignedData{
				Signature: peerSignature.Signature,
				Identity:  peerSignature.PeerIdentity,
				Data:      peerSignature.Message,
			})
		})

		if err != nil {
			return nil, errors.WithStack(err)
		}
		sources := sources
		endorserPeer, err := p.PeerFilter(common.ChainID(p.channel), func(peerSignature api.PeerSignature) bool {
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

		filters[*digest] = collectionRoutingFilter{
			anyPeer:  anyPeerInCollection,
			endorser: endorserPeer,
		}
	}
	return filters, nil
}

func randomizeMemberList(members []discovery.NetworkMember) []discovery.NetworkMember {
	rand.Seed(time.Now().UnixNano())
	res := make([]discovery.NetworkMember, len(members))
	for i, j := range rand.Perm(len(members)) {
		res[i] = members[j]
	}
	return res
}

func digestsAsPointerSlice(digests []proto.PvtDataDigest) []*proto.PvtDataDigest {
	res := make([]*proto.PvtDataDigest, len(digests))
	for i, dig := range digests {
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
