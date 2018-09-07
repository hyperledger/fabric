/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/protos/discovery"
	"github.com/hyperledger/fabric/protos/gossip"
	"github.com/hyperledger/fabric/protos/msp"
	"github.com/pkg/errors"
)

var (
	configTypes = []discovery.QueryType{discovery.ConfigQueryType, discovery.PeerMembershipQueryType, discovery.ChaincodeQueryType, discovery.LocalMembershipQueryType}
)

// Client interacts with the discovery server
type Client struct {
	createConnection Dialer
	signRequest      Signer
}

// NewRequest creates a new request
func NewRequest() *Request {
	r := &Request{
		invocationChainMapping: make(map[int][]InvocationChain),
		queryMapping:           make(map[discovery.QueryType]map[string]int),
		Request:                &discovery.Request{},
	}
	// pre-populate types
	for _, queryType := range configTypes {
		r.queryMapping[queryType] = make(map[string]int)
	}
	return r
}

// Request aggregates several queries inside it
type Request struct {
	lastChannel string
	lastIndex   int
	// map from query type to channel to expected index in response
	queryMapping map[discovery.QueryType]map[string]int
	// map from expected index in response to invocation chains
	invocationChainMapping map[int][]InvocationChain
	*discovery.Request
}

// AddConfigQuery adds to the request a config query
func (req *Request) AddConfigQuery() *Request {
	ch := req.lastChannel
	q := &discovery.Query_ConfigQuery{
		ConfigQuery: &discovery.ConfigQuery{},
	}
	req.Queries = append(req.Queries, &discovery.Query{
		Channel: ch,
		Query:   q,
	})
	req.addQueryMapping(discovery.ConfigQueryType, ch)
	return req
}

// AddEndorsersQuery adds to the request a query for given chaincodes
// interests are the chaincode interests that the client wants to query for.
// All interests for a given channel should be supplied in an aggregated slice
func (req *Request) AddEndorsersQuery(interests ...*discovery.ChaincodeInterest) (*Request, error) {
	if err := validateInterests(interests...); err != nil {
		return nil, err
	}
	ch := req.lastChannel
	q := &discovery.Query_CcQuery{
		CcQuery: &discovery.ChaincodeQuery{
			Interests: interests,
		},
	}
	req.Queries = append(req.Queries, &discovery.Query{
		Channel: ch,
		Query:   q,
	})
	var invocationChains []InvocationChain
	for _, interest := range interests {
		invocationChains = append(invocationChains, interest.Chaincodes)
	}
	req.addChaincodeQueryMapping(invocationChains)
	req.addQueryMapping(discovery.ChaincodeQueryType, ch)
	return req, nil
}

// AddLocalPeersQuery adds to the request a local peer query
func (req *Request) AddLocalPeersQuery() *Request {
	q := &discovery.Query_LocalPeers{
		LocalPeers: &discovery.LocalPeerQuery{},
	}
	req.Queries = append(req.Queries, &discovery.Query{
		Query: q,
	})
	var ic InvocationChain
	req.addQueryMapping(discovery.LocalMembershipQueryType, channnelAndInvocationChain("", ic))
	return req
}

// AddPeersQuery adds to the request a peer query
func (req *Request) AddPeersQuery(invocationChain ...*discovery.ChaincodeCall) *Request {
	ch := req.lastChannel
	q := &discovery.Query_PeerQuery{
		PeerQuery: &discovery.PeerMembershipQuery{
			Filter: &discovery.ChaincodeInterest{
				Chaincodes: invocationChain,
			},
		},
	}
	req.Queries = append(req.Queries, &discovery.Query{
		Channel: ch,
		Query:   q,
	})
	var ic InvocationChain
	if len(invocationChain) > 0 {
		ic = InvocationChain(invocationChain)
	}
	req.addChaincodeQueryMapping([]InvocationChain{ic})
	req.addQueryMapping(discovery.PeerMembershipQueryType, channnelAndInvocationChain(ch, ic))
	return req
}

func channnelAndInvocationChain(ch string, ic InvocationChain) string {
	return fmt.Sprintf("%s %s", ch, ic.String())
}

// OfChannel sets the next queries added to be in the given channel's context
func (req *Request) OfChannel(ch string) *Request {
	req.lastChannel = ch
	return req
}

func (req *Request) addChaincodeQueryMapping(invocationChains []InvocationChain) {
	req.invocationChainMapping[req.lastIndex] = invocationChains
}

func (req *Request) addQueryMapping(queryType discovery.QueryType, key string) {
	req.queryMapping[queryType][key] = req.lastIndex
	req.lastIndex++
}

// Send sends the request and returns the response, or error on failure
func (c *Client) Send(ctx context.Context, req *Request, auth *discovery.AuthInfo) (Response, error) {
	reqToBeSent := *req.Request
	reqToBeSent.Authentication = auth
	payload, err := proto.Marshal(&reqToBeSent)
	if err != nil {
		return nil, errors.Wrap(err, "failed marshaling Request to bytes")
	}

	sig, err := c.signRequest(payload)
	if err != nil {
		return nil, errors.Wrap(err, "failed signing Request")
	}

	conn, err := c.createConnection()
	if err != nil {
		return nil, errors.Wrap(err, "failed connecting to discovery service")
	}

	cl := discovery.NewDiscoveryClient(conn)
	resp, err := cl.Discover(ctx, &discovery.SignedRequest{
		Payload:   payload,
		Signature: sig,
	})
	if err != nil {
		return nil, errors.Wrap(err, "discovery service refused our Request")
	}
	if n := len(resp.Results); n != req.lastIndex {
		return nil, errors.Errorf("Sent %d queries but received %d responses back", req.lastIndex, n)
	}
	return req.computeResponse(resp)
}

type resultOrError interface {
}

type response map[key]resultOrError

type localResponse struct {
	response
}

func (cr *localResponse) Peers() ([]*Peer, error) {
	return parsePeers(discovery.LocalMembershipQueryType, cr.response, "")
}

type channelResponse struct {
	response
	channel string
}

func (cr *channelResponse) Config() (*discovery.ConfigResult, error) {
	res, exists := cr.response[key{
		queryType: discovery.ConfigQueryType,
		k:         cr.channel,
	}]

	if !exists {
		return nil, ErrNotFound
	}

	if config, isConfig := res.(*discovery.ConfigResult); isConfig {
		return config, nil
	}

	return nil, res.(error)
}

func parsePeers(queryType discovery.QueryType, r response, channel string, invocationChain ...*discovery.ChaincodeCall) ([]*Peer, error) {
	peerKeys := key{
		queryType: queryType,
		k:         fmt.Sprintf("%s %s", channel, InvocationChain(invocationChain).String()),
	}
	res, exists := r[peerKeys]

	if !exists {
		return nil, ErrNotFound
	}

	if peers, isPeers := res.([]*Peer); isPeers {
		return peers, nil
	}

	return nil, res.(error)
}

func (cr *channelResponse) Peers(invocationChain ...*discovery.ChaincodeCall) ([]*Peer, error) {
	return parsePeers(discovery.PeerMembershipQueryType, cr.response, cr.channel, invocationChain...)
}

func (cr *channelResponse) Endorsers(invocationChain InvocationChain, f Filter) (Endorsers, error) {
	// If we have a key that has no chaincode field,
	// it means it's an error returned from the service
	if err, exists := cr.response[key{
		queryType: discovery.ChaincodeQueryType,
		k:         cr.channel,
	}]; exists {
		return nil, err.(error)
	}

	// Else, the service returned a response that isn't an error
	res, exists := cr.response[key{
		queryType:       discovery.ChaincodeQueryType,
		k:               cr.channel,
		invocationChain: invocationChain.String(),
	}]

	if !exists {
		return nil, ErrNotFound
	}

	desc := res.(*endorsementDescriptor)
	rand.Seed(time.Now().Unix())
	// We iterate over all layouts to find one that we have enough peers to select
	for _, index := range rand.Perm(len(desc.layouts)) {
		layout := desc.layouts[index]
		endorsers, canLayoutBeSatisfied := selectPeersForLayout(desc.endorsersByGroups, layout, f)
		if canLayoutBeSatisfied {
			return endorsers, nil
		}
	}
	return nil, errors.New("no endorsement combination can be satisfied")
}

type filter struct {
	ef ExclusionFilter
	ps PrioritySelector
}

// NewFilter returns an endorser filter that uses the given exclusion filter and priority selector
// to filter and sort the endorsers
func NewFilter(ps PrioritySelector, ef ExclusionFilter) Filter {
	return &filter{
		ef: ef,
		ps: ps,
	}
}

// Filter returns a filtered and sorted list of endorsers
func (f *filter) Filter(endorsers Endorsers) Endorsers {
	return endorsers.Shuffle().Filter(f.ef).Sort(f.ps)
}

// NoFilter returns a noop Filter
var NoFilter = NewFilter(NoPriorities, NoExclusion)

func selectPeersForLayout(endorsersByGroups map[string][]*Peer, layout map[string]int, f Filter) (Endorsers, bool) {
	var endorsers []*Peer
	for grp, count := range layout {
		endorsersOfGrp := f.Filter(Endorsers(endorsersByGroups[grp]))

		// We couldn't select enough peers for this layout because the current group
		// requires more peers than we have available to be selected
		if len(endorsersOfGrp) < count {
			return nil, false
		}
		endorsersOfGrp = endorsersOfGrp[:count]
		endorsers = append(endorsers, endorsersOfGrp...)
	}
	// The current (randomly chosen) layout can be satisfied, so return it
	// instead of checking the next one.
	return endorsers, true
}

func (resp response) ForLocal() LocalResponse {
	return &localResponse{
		response: resp,
	}
}

func (resp response) ForChannel(ch string) ChannelResponse {
	return &channelResponse{
		channel:  ch,
		response: resp,
	}
}

type key struct {
	queryType       discovery.QueryType
	k               string
	invocationChain string
}

func (req *Request) computeResponse(r *discovery.Response) (response, error) {
	var err error
	resp := make(response)
	for configType, channel2index := range req.queryMapping {
		switch configType {
		case discovery.ConfigQueryType:
			err = resp.mapConfig(channel2index, r)
		case discovery.ChaincodeQueryType:
			err = resp.mapEndorsers(channel2index, r, req.invocationChainMapping)
		case discovery.PeerMembershipQueryType:
			err = resp.mapPeerMembership(channel2index, r, discovery.PeerMembershipQueryType)
		case discovery.LocalMembershipQueryType:
			err = resp.mapPeerMembership(channel2index, r, discovery.LocalMembershipQueryType)
		}
		if err != nil {
			return nil, err
		}
	}

	return resp, err
}

func (resp response) mapConfig(channel2index map[string]int, r *discovery.Response) error {
	for ch, index := range channel2index {
		config, err := r.ConfigAt(index)
		if config == nil && err == nil {
			return errors.Errorf("expected QueryResult of either ConfigResult or Error but got %v instead", r.Results[index])
		}
		key := key{
			queryType: discovery.ConfigQueryType,
			k:         ch,
		}

		if err != nil {
			resp[key] = errors.New(err.Content)
			continue
		}

		resp[key] = config
	}
	return nil
}

func (resp response) mapPeerMembership(key2Index map[string]int, r *discovery.Response, qt discovery.QueryType) error {
	for k, index := range key2Index {
		membersRes, err := r.MembershipAt(index)
		if membersRes == nil && err == nil {
			return errors.Errorf("expected QueryResult of either PeerMembershipResult or Error but got %v instead", r.Results[index])
		}

		key := key{
			queryType: qt,
			k:         k,
		}

		if err != nil {
			resp[key] = errors.New(err.Content)
			continue
		}

		peers, err2 := peersForChannel(membersRes, qt)
		if err2 != nil {
			return errors.Wrap(err2, "failed constructing peer membership out of response")
		}

		resp[key] = peers
	}
	return nil
}

func peersForChannel(membersRes *discovery.PeerMembershipResult, qt discovery.QueryType) ([]*Peer, error) {
	var peers []*Peer
	for org, peersOfCurrentOrg := range membersRes.PeersByOrg {
		for _, peer := range peersOfCurrentOrg.Peers {
			aliveMsg, err := peer.MembershipInfo.ToGossipMessage()
			if err != nil {
				return nil, errors.Wrap(err, "failed unmarshaling alive message")
			}
			var stateInfoMsg *gossip.SignedGossipMessage
			if isStateInfoExpected(qt) {
				stateInfoMsg, err = peer.StateInfo.ToGossipMessage()
				if err != nil {
					return nil, errors.Wrap(err, "failed unmarshaling stateInfo message")
				}
				if err := validateStateInfoMessage(stateInfoMsg); err != nil {
					return nil, errors.Wrap(err, "failed validating stateInfo message")
				}
			}
			if err := validateAliveMessage(aliveMsg); err != nil {
				return nil, errors.Wrap(err, "failed validating alive message")
			}
			peers = append(peers, &Peer{
				MSPID:            org,
				Identity:         peer.Identity,
				AliveMessage:     aliveMsg,
				StateInfoMessage: stateInfoMsg,
			})
		}
	}
	return peers, nil
}

func isStateInfoExpected(qt discovery.QueryType) bool {
	return qt != discovery.LocalMembershipQueryType
}

func (resp response) mapEndorsers(
	channel2index map[string]int,
	r *discovery.Response,
	chaincodeQueryMapping map[int][]InvocationChain) error {
	for ch, index := range channel2index {
		ccQueryRes, err := r.EndorsersAt(index)
		if ccQueryRes == nil && err == nil {
			return errors.Errorf("expected QueryResult of either ChaincodeQueryResult or Error but got %v instead", r.Results[index])
		}

		if err != nil {
			key := key{
				queryType: discovery.ChaincodeQueryType,
				k:         ch,
			}
			resp[key] = errors.New(err.Content)
			continue
		}

		if err := resp.mapEndorsersOfChannel(ccQueryRes, ch, chaincodeQueryMapping[index]); err != nil {
			return errors.Wrapf(err, "failed assembling endorsers of channel %s", ch)
		}
	}
	return nil
}

func (resp response) mapEndorsersOfChannel(ccRs *discovery.ChaincodeQueryResult, channel string, invocationChain []InvocationChain) error {
	if len(ccRs.Content) < len(invocationChain) {
		return errors.Errorf("expected %d endorsement descriptors but got only %d", len(invocationChain), len(ccRs.Content))
	}
	for i, desc := range ccRs.Content {
		expectedCCName := invocationChain[i][0].Name
		if desc.Chaincode != expectedCCName {
			return errors.Errorf("expected chaincode %s but got endorsement descriptor for %s", expectedCCName, desc.Chaincode)
		}
		key := key{
			queryType:       discovery.ChaincodeQueryType,
			k:               channel,
			invocationChain: invocationChain[i].String(),
		}

		descriptor, err := resp.createEndorsementDescriptor(desc, channel)
		if err != nil {
			return err
		}
		resp[key] = descriptor
	}

	return nil
}

func (resp response) createEndorsementDescriptor(desc *discovery.EndorsementDescriptor, channel string) (*endorsementDescriptor, error) {
	descriptor := &endorsementDescriptor{
		layouts:           []map[string]int{},
		endorsersByGroups: make(map[string][]*Peer),
	}
	for _, l := range desc.Layouts {
		currentLayout := make(map[string]int)
		descriptor.layouts = append(descriptor.layouts, currentLayout)
		for grp, count := range l.QuantitiesByGroup {
			if _, exists := desc.EndorsersByGroups[grp]; !exists {
				return nil, errors.Errorf("group %s isn't mapped to endorsers, but exists in a layout", grp)
			}
			currentLayout[grp] = int(count)
		}
	}

	for grp, peers := range desc.EndorsersByGroups {
		var endorsers []*Peer
		for _, p := range peers.Peers {
			peer, err := endorser(p, desc.Chaincode, channel)
			if err != nil {
				return nil, errors.Wrap(err, "failed creating endorser object")
			}
			endorsers = append(endorsers, peer)
		}
		descriptor.endorsersByGroups[grp] = endorsers
	}

	return descriptor, nil
}

func endorser(peer *discovery.Peer, chaincode, channel string) (*Peer, error) {
	if peer.MembershipInfo == nil || peer.StateInfo == nil {
		return nil, errors.Errorf("received empty envelope(s) for endorsers for chaincode %s, channel %s", chaincode, channel)
	}
	aliveMsg, err := peer.MembershipInfo.ToGossipMessage()
	if err != nil {
		return nil, errors.Wrap(err, "failed unmarshaling gossip envelope to alive message")
	}
	stateInfMsg, err := peer.StateInfo.ToGossipMessage()
	if err != nil {
		return nil, errors.Wrap(err, "failed unmarshaling gossip envelope to state info message")
	}
	if err := validateAliveMessage(aliveMsg); err != nil {
		return nil, errors.Wrap(err, "failed validating alive message")
	}
	if err := validateStateInfoMessage(stateInfMsg); err != nil {
		return nil, errors.Wrap(err, "failed validating stateInfo message")
	}
	sID := &msp.SerializedIdentity{}
	if err := proto.Unmarshal(peer.Identity, sID); err != nil {
		return nil, errors.Wrap(err, "failed unmarshaling peer's identity")
	}
	return &Peer{
		Identity:         peer.Identity,
		StateInfoMessage: stateInfMsg,
		AliveMessage:     aliveMsg,
		MSPID:            sID.Mspid,
	}, nil
}

type endorsementDescriptor struct {
	endorsersByGroups map[string][]*Peer
	layouts           []map[string]int
}

// NewClient creates a new Client instance
func NewClient(createConnection Dialer, s Signer, signerCacheSize uint) *Client {
	return &Client{
		createConnection: createConnection,
		signRequest:      NewMemoizeSigner(s, signerCacheSize).Sign,
	}
}

func validateAliveMessage(message *gossip.SignedGossipMessage) error {
	am := message.GetAliveMsg()
	if am == nil {
		return errors.New("message isn't an alive message")
	}
	m := am.Membership
	if m == nil {
		return errors.New("membership is empty")
	}
	if am.Timestamp == nil {
		return errors.New("timestamp is nil")
	}
	return nil
}

func validateStateInfoMessage(message *gossip.SignedGossipMessage) error {
	si := message.GetStateInfo()
	if si == nil {
		return errors.New("message isn't a stateInfo message")
	}
	if si.Timestamp == nil {
		return errors.New("timestamp is nil")
	}
	if si.Properties == nil {
		return errors.New("properties is nil")
	}
	return nil
}

func validateInterests(interests ...*discovery.ChaincodeInterest) error {
	if len(interests) == 0 {
		return errors.New("no chaincode interests given")
	}
	for _, interest := range interests {
		if interest == nil {
			return errors.New("chaincode interest is nil")
		}
		if err := InvocationChain(interest.Chaincodes).ValidateInvocationChain(); err != nil {
			return err
		}
	}
	return nil
}

// InvocationChain aggregates ChaincodeCalls
type InvocationChain []*discovery.ChaincodeCall

// String returns a string representation of this invocation chain
func (ic InvocationChain) String() string {
	s, _ := json.Marshal(ic)
	return string(s)
}

// ValidateInvocationChain validates the InvocationChain's structure
func (ic InvocationChain) ValidateInvocationChain() error {
	if len(ic) == 0 {
		return errors.New("invocation chain should not be empty")
	}
	for _, cc := range ic {
		if cc.Name == "" {
			return errors.New("chaincode name should not be empty")
		}
	}
	return nil
}
