/*
Copyright 2021 IBM All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gateway

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"sync"

	"github.com/golang/protobuf/proto"
	dp "github.com/hyperledger/fabric-protos-go/discovery"
	"github.com/hyperledger/fabric-protos-go/gossip"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/scc"
	gossipapi "github.com/hyperledger/fabric/gossip/api"
	gossipcommon "github.com/hyperledger/fabric/gossip/common"
	gossipdiscovery "github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/internal/pkg/gateway/ledger"
	"github.com/pkg/errors"
)

type Discovery interface {
	Config(channel string) (*dp.ConfigResult, error)
	IdentityInfo() gossipapi.PeerIdentitySet
	PeersForEndorsement(channel gossipcommon.ChannelID, interest *peer.ChaincodeInterest) (*dp.EndorsementDescriptor, error)
	PeersOfChannel(gossipcommon.ChannelID) gossipdiscovery.Members
}

type registry struct {
	localEndorser      *endorser
	discovery          Discovery
	logger             *flogging.FabricLogger
	endpointFactory    *endpointFactory
	remoteEndorsers    map[string]*endorser
	broadcastClients   sync.Map // orderer address (string) -> client connection (orderer)
	channelInitialized map[string]bool
	configLock         sync.RWMutex
	channelOrderers    sync.Map // channel (string) -> orderer addresses (endpointConfig)
	systemChaincodes   scc.BuiltinSCCs
	localProvider      ledger.Provider
}

type endorserState struct {
	peer     *dp.Peer
	endorser *endorser
	height   uint64
}

// Returns an endorsementPlan for the given chaincode on a channel.
func (reg *registry) endorsementPlan(channel string, interest *peer.ChaincodeInterest, preferredEndorser *endorser) (*plan, error) {
	descriptor, err := reg.discovery.PeersForEndorsement(gossipcommon.ChannelID(channel), interest)
	if err != nil {
		logger.Errorw("PeersForEndorsement failed.", "error", err, "channel", channel, "ChaincodeInterest", proto.MarshalTextString(interest))
		return nil, errors.Wrap(err, "no combination of peers can be derived which satisfy the endorsement policy")
	}

	// There are two parts to an endorsement plan:
	// 1) List of endorsers in each group
	// 2) Layouts consisting of a number of endorsers from each group

	// Firstly, process the endorsers by group
	// Create a map of groupIds to list of endorsers, sorted by decreasing block height
	// Also build a map of endorser PKI ID to group ID
	groupEndorsers := map[string][]*endorser{}
	var preferredGroup string
	var unavailableEndorsers []string

	for group, peers := range descriptor.GetEndorsersByGroups() {
		var groupPeers []*endorserState
		for _, peer := range peers.GetPeers() {
			// extract block height
			msg := &gossip.GossipMessage{}
			err = proto.Unmarshal(peer.GetStateInfo().GetPayload(), msg)
			if err != nil {
				return nil, err
			}
			height := msg.GetStateInfo().GetProperties().GetLedgerHeight()

			// extract endpoint
			err = proto.Unmarshal(peer.GetMembershipInfo().GetPayload(), msg)
			if err != nil {
				return nil, err
			}
			member := msg.GetAliveMsg().GetMembership()

			// find the endorser in the registry for this endpoint
			endorser := reg.lookupEndorser(member.GetEndpoint(), member.GetPkiId(), channel)
			if endorser == nil {
				unavailableEndorsers = append(unavailableEndorsers, member.GetEndpoint())
				continue
			}
			if endorser == preferredEndorser {
				preferredGroup = group
			}
			groupPeers = append(groupPeers, &endorserState{peer: peer, endorser: endorser, height: height})
		}
		// sort by decreasing height
		sort.Slice(groupPeers, sorter(groupPeers, reg.localEndorser.address))

		if len(groupPeers) > 0 {
			var endorsers []*endorser
			for _, peer := range groupPeers {
				endorsers = append(endorsers, peer.endorser)
			}
			groupEndorsers[group] = endorsers
		}
	}

	// Second, process the layouts
	// Group them by groupId and arrange them so that layouts containing the 'preferredOrg' are at the front of the list
	var preferredLayouts []*layout
	var otherLayouts []*layout
layout:
	for _, lo := range descriptor.GetLayouts() {
		hasPreferredGroup := false
		quantityByGroup := map[string]int{}
		for group, quantity := range lo.GetQuantitiesByGroup() {
			// if there's no entry in this map, meaning there aren't any available endorsers for this group
			if len(groupEndorsers[group]) < int(quantity) {
				// The number of available endorsers less than the quantity required, so abandon this layout
				continue layout
			}
			quantityByGroup[group] = int(quantity)
			if group == preferredGroup {
				hasPreferredGroup = true
			}
		}
		if len(quantityByGroup) == 0 {
			// empty layout
			break
		}
		if hasPreferredGroup {
			preferredLayouts = append(preferredLayouts, &layout{required: quantityByGroup})
		} else {
			otherLayouts = append(otherLayouts, &layout{required: quantityByGroup})
		}
	}

	// shuffle the layouts - keep the preferred ones first
	rand.Shuffle(len(preferredLayouts), func(i, j int) { preferredLayouts[i], preferredLayouts[j] = preferredLayouts[j], preferredLayouts[i] })
	rand.Shuffle(len(otherLayouts), func(i, j int) { otherLayouts[i], otherLayouts[j] = otherLayouts[j], otherLayouts[i] })
	layouts := append(preferredLayouts, otherLayouts...)

	if len(layouts) == 0 {
		return nil, fmt.Errorf("failed to select a set of endorsers that satisfy the endorsement policy due to unavailability of peers: %v", unavailableEndorsers)
	}

	return newPlan(layouts, groupEndorsers), nil
}

// planForOrgs generates an endorsement plan with a single layout requiring one peer from each of the given orgs for the given chaincode on a channel.
func (reg *registry) planForOrgs(channel string, chaincode string, endorsingOrgs []string) (*plan, error) {
	endorsersByOrg := reg.endorsersByOrg(channel, chaincode)

	required := map[string]int{}
	groupEndorsers := map[string][]*endorser{}
	missingOrgs := []string{}
	for _, org := range endorsingOrgs {
		if es, ok := endorsersByOrg[org]; ok {
			for _, e := range es {
				groupEndorsers[org] = append(groupEndorsers[org], e.endorser)
			}
			required[org] = 1
		} else {
			missingOrgs = append(missingOrgs, org)
		}
	}
	if len(missingOrgs) > 0 {
		return nil, fmt.Errorf("failed to find any endorsing peers for org(s): %s", strings.Join(missingOrgs, ", "))
	}

	return newPlan([]*layout{{required: required}}, groupEndorsers), nil
}

func (reg *registry) endorsersByOrg(channel string, chaincode string) map[string][]*endorserState {
	endorsersByOrg := make(map[string][]*endorserState)

	for _, member := range reg.channelMembers(channel) {
		endorser := reg.lookupEndorser(member.PreferredEndpoint(), member.PKIid, channel)
		if endorser == nil {
			continue
		}
		if reg.hasChaincode(member, chaincode) {
			endorsersByOrg[endorser.mspid] = append(endorsersByOrg[endorser.mspid], &endorserState{endorser: endorser, height: member.Properties.GetLedgerHeight()})
		}
	}

	// sort by decreasing height in each org
	for _, es := range endorsersByOrg {
		sort.Slice(es, sorter(es, reg.localEndorser.address))
	}

	return endorsersByOrg
}

func (reg *registry) channelMembers(channel string) gossipdiscovery.Members {
	members := reg.discovery.PeersOfChannel(gossipcommon.ChannelID(channel))

	// Ensure local endorser ledger height is up-to-date
	for _, member := range members {
		if reg.isLocalEndorserID(member.PKIid) {
			if ledgerHeight, ok := reg.localLedgerHeight(channel); ok {
				member.Properties.LedgerHeight = ledgerHeight
			}

			break
		}
	}

	return members
}

func (reg *registry) isLocalEndorserID(pkiID gossipcommon.PKIidType) bool {
	return !pkiID.IsNotSameFilter(reg.localEndorser.pkiid)
}

func (reg *registry) localLedgerHeight(channel string) (height uint64, ok bool) {
	ledger, err := reg.localProvider.Ledger(channel)
	if err != nil {
		reg.logger.Warnw("local endorser is not a member of channel", "channel", channel, "err", err)
		return 0, false
	}

	info, err := ledger.GetBlockchainInfo()
	if err != nil {
		logger.Errorw("failed to get local ledger info", "err", err)
		return 0, false
	}

	return info.GetHeight(), true
}

// evaluator returns a plan representing a single endorsement, preferably from local org, if available
// targetOrgs specifies the orgs that are allowed receive the request, due to private data restrictions
func (reg *registry) evaluator(channel string, chaincode string, targetOrgs []string) (*plan, error) {
	endorsersByOrg := reg.endorsersByOrg(channel, chaincode)

	// If no targetOrgs are specified (i.e. no restrictions), then populate with all available orgs
	if len(targetOrgs) == 0 {
		for org := range endorsersByOrg {
			targetOrgs = append(targetOrgs, org)
		}
	}

	localOrgEndorsers := []*endorserState{}
	otherOrgEndorsers := []*endorserState{}
	for _, org := range targetOrgs {
		if es, ok := endorsersByOrg[org]; ok {
			if org == reg.localEndorser.mspid {
				localOrgEndorsers = es
			} else {
				otherOrgEndorsers = append(otherOrgEndorsers, es...)
			}
		}
	}
	// sort all the 'other orgs' endorsers by decreasing block height
	sort.Slice(otherOrgEndorsers, sorter(otherOrgEndorsers, ""))

	var allEndorsers []*endorser
	for _, e := range append(localOrgEndorsers, otherOrgEndorsers...) {
		allEndorsers = append(allEndorsers, e.endorser)
	}
	if len(allEndorsers) > 0 {
		layout := []*layout{{required: map[string]int{"g1": 1}}} // single layout, one group, one endorsement
		groupEndorsers := map[string][]*endorser{"g1": allEndorsers}
		return newPlan(layout, groupEndorsers), nil
	}
	return nil, fmt.Errorf("no peers available to evaluate chaincode %s in channel %s", chaincode, channel)
}

func sorter(e []*endorserState, host string) func(i, j int) bool {
	return func(i, j int) bool {
		if e[i].height == e[j].height {
			// prefer host peer
			return e[i].endorser.address == host
		}
		return e[i].height > e[j].height
	}
}

// Returns a set of broadcastClients that can order a transaction for the given channel.
func (reg *registry) orderers(channel string) ([]*orderer, int, error) {
	var orderers []*orderer
	var ordererEndpoints []*endpointConfig
	addr, exists := reg.channelOrderers.Load(channel)
	// if it doesn't exist, get the orderers config for this channel
	if exists {
		ordererEndpoints = addr.([]*endpointConfig)
	} else {
		// no entry in the map - get the orderer config from discovery
		channelOrderers, err := reg.config(channel)
		if err != nil {
			return nil, 0, err
		}
		// A config update may have saved this first, in which case don't overwrite it.
		addr, _ = reg.channelOrderers.LoadOrStore(channel, channelOrderers)
		ordererEndpoints = addr.([]*endpointConfig)
	}
	for _, ep := range ordererEndpoints {
		entry, exists := reg.broadcastClients.Load(ep.address)
		if !exists {
			// this orderer is new - connect to it and add to the broadcastClients registry
			client, err := reg.endpointFactory.newOrderer(ep.address, ep.mspid, ep.tlsRootCerts)
			if err != nil {
				// Failed to connect to this orderer for some reason.  Log the problem and skip to the next one.
				reg.logger.Warnw("Failed to connect to orderer", "address", ep.address, "err", err)
				continue
			}
			var loaded bool
			entry, loaded = reg.broadcastClients.LoadOrStore(ep.address, client)
			if loaded {
				// another goroutine got there first, close this new connection
				err = client.closeConnection()
				if err != nil {
					// Failed to close this new connection.  Log the problem.
					reg.logger.Warnw("Failed to close connection to orderer", "address", client.endpointConfig.logAddress, "err", err)
				}
			} else {
				reg.logger.Infow("Added orderer to registry", "address", client.endpointConfig.logAddress)
			}
		}
		orderers = append(orderers, entry.(*orderer))
	}

	return orderers, len(ordererEndpoints), nil
}

func (reg *registry) lookupEndorser(endpoint string, pkiID gossipcommon.PKIidType, channel string) *endorser {
	lookup := func() (*endorser, bool) {
		reg.configLock.RLock()
		defer reg.configLock.RUnlock()

		// find the endorser in the registry for this endpoint
		if reg.isLocalEndorserID(pkiID) {
			logger.Debugw("Found local endorser", "pkiID", pkiID)
			return reg.localEndorser, false
		}
		if endpoint == "" {
			reg.logger.Warnw("No endpoint for endorser with PKI ID %s", "pkiID", pkiID.String())
			return nil, false
		}
		if e, ok := reg.remoteEndorsers[endpoint]; ok {
			logger.Debugw("Found remote endorser", "endpoint", endpoint)
			return e, false
		}
		// not found - try to connect
		return nil, true
	}
	endorser, connect := lookup()
	if connect {
		reg.logger.Infow("Attempting to connect to endorser", "endpoint", endpoint)
		// for efficiency, try to connect to all peers in the channel in one pass
		err := reg.connectChannelPeers(channel, true)
		if err != nil {
			logger.Errorw("Failed to reconnect", "endpoint", endpoint, "err", err)
		}
		endorser, _ = lookup() // if it failed to reconnect, this will be nil
	}
	return endorser
}

// Connect to peers in channel, and add to the registry.  If a reconnection is required, then it must be removed
// from the registry first, using removeEndorser().
func (reg *registry) connectChannelPeers(channel string, force bool) error {
	reg.configLock.Lock() // take a write lock to populate the registry maps
	defer reg.configLock.Unlock()

	if !force && reg.channelInitialized[channel] {
		return nil
	}

	// get the remoteEndorsers for the channel
	peers := map[string]string{}
	for _, member := range reg.channelMembers(channel) {
		id := member.PKIid.String()
		peers[id] = member.PreferredEndpoint()
	}
	config, err := reg.discovery.Config(channel)
	if err != nil {
		return fmt.Errorf("failed to get config for channel [%s]: %w", channel, err)
	}
	for mspid, infoset := range reg.discovery.IdentityInfo().ByOrg() {
		var tlsRootCerts [][]byte
		if mspInfo, ok := config.GetMsps()[mspid]; ok {
			tlsRootCerts = append(tlsRootCerts, mspInfo.GetTlsRootCerts()...)
			tlsRootCerts = append(tlsRootCerts, mspInfo.GetTlsIntermediateCerts()...)
		}
		for _, info := range infoset {
			pkiID := info.PKIId
			if address, ok := peers[pkiID.String()]; ok {
				// add the peer to the peer map - except the local peer, which seems to have an empty address
				if _, ok := reg.remoteEndorsers[address]; !ok && len(address) > 0 {
					// this peer is new - connect to it and add to the remoteEndorsers registry
					endorser, err := reg.endpointFactory.newEndorser(pkiID, address, mspid, tlsRootCerts)
					if err != nil {
						return err
					}
					reg.remoteEndorsers[address] = endorser
					reg.logger.Infof("Added peer to registry: %s", address)
				}
			}
		}
	}
	reg.channelInitialized[channel] = true
	return nil
}

// removeEndorser closes the connection and removes from the registry, but if the next call to discovery returns that
// peer in its plan, then it will attempt to reconnect and add it back. This could happen if the peer had gone down,
// but the gossip has not notified the discovery service yet.
func (reg *registry) removeEndorser(endorser *endorser) {
	if endorser == reg.localEndorser {
		// nothing to close
		return
	}
	reg.configLock.Lock()
	defer reg.configLock.Unlock()

	err := endorser.closeConnection()
	if err != nil {
		reg.logger.Errorw("Failed to close connection to endorser", "address", endorser.address, "mspid", endorser.mspid, "err", err)
	}
	delete(reg.remoteEndorsers, endorser.address)
}

func (reg *registry) config(channel string) ([]*endpointConfig, error) {
	config, err := reg.discovery.Config(channel)
	if err != nil {
		return nil, fmt.Errorf("failed to get config for channel [%s]: %w", channel, err)
	}
	var channelOrderers []*endpointConfig
	for mspid, eps := range config.GetOrderers() {
		var tlsRootCerts [][]byte
		if mspInfo, ok := config.GetMsps()[mspid]; ok {
			tlsRootCerts = append(tlsRootCerts, mspInfo.GetTlsRootCerts()...)
			tlsRootCerts = append(tlsRootCerts, mspInfo.GetTlsIntermediateCerts()...)
		}
		for _, ep := range eps.Endpoint {
			address := fmt.Sprintf("%s:%d", ep.Host, ep.Port)
			channelOrderers = append(channelOrderers, &endpointConfig{address: address, mspid: mspid, tlsRootCerts: tlsRootCerts})
		}
	}
	return channelOrderers, nil
}

func (reg *registry) configUpdate(bundle *channelconfig.Bundle) {
	if ordererConfig, ok := bundle.OrdererConfig(); ok {
		// orderer config has changed - process the bundle
		channel := bundle.ConfigtxValidator().ChannelID()
		reg.logger.Infow("Updating orderer config", "channel", channel)
		var channelOrderers []*endpointConfig
		for _, org := range ordererConfig.Organizations() {
			mspid := org.MSPID()
			msp := org.MSP()
			tlsRootCerts := append([][]byte{}, msp.GetTLSRootCerts()...)
			tlsRootCerts = append(tlsRootCerts, msp.GetTLSIntermediateCerts()...)
			for _, address := range org.Endpoints() {
				channelOrderers = append(channelOrderers, &endpointConfig{address: address, mspid: mspid, tlsRootCerts: tlsRootCerts})
				reg.logger.Debugw("Channel orderer", "address", address, "mspid", mspid)
			}
		}
		if len(channelOrderers) > 0 {
			reg.closeStaleOrdererConnections(channel, channelOrderers)
			reg.channelOrderers.Store(channel, channelOrderers)
		}
	}
}

func (reg *registry) closeStaleOrdererConnections(channel string, channelOrderers []*endpointConfig) {
	// Load the list of orderers that is about to be overwritten, if loaded is false, then another goroutine got there first
	oldList, loaded := reg.channelOrderers.LoadAndDelete(channel)
	if loaded {
		currentEndpoints := map[string]struct{}{}
		reg.channelOrderers.Range(func(key, value interface{}) bool {
			for _, ep := range value.([]*endpointConfig) {
				currentEndpoints[ep.address] = struct{}{}
			}
			return true
		})
		for _, ep := range channelOrderers {
			currentEndpoints[ep.address] = struct{}{}
		}
		// if there are any in the oldEndpoints that are not in the currentEndpoints, then remove from registry and close connection
		for _, ep := range oldList.([]*endpointConfig) {
			if _, exists := currentEndpoints[ep.address]; !exists {
				client, found := reg.broadcastClients.LoadAndDelete(ep.address)
				if found {
					err := client.(*orderer).closeConnection()
					if err != nil {
						reg.logger.Errorw("Failed to close connection to orderer", "address", ep.logAddress, "mspid", ep.mspid, "err", err)
					}
				}
			}
		}
	}
}

func (reg *registry) hasChaincode(member gossipdiscovery.NetworkMember, chaincodeName string) bool {
	for _, installedChaincode := range member.Properties.GetChaincodes() {
		if installedChaincode.GetName() == chaincodeName {
			return true
		}
	}

	return reg.systemChaincodes.IsSysCC(chaincodeName)
}
