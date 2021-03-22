/*
Copyright 2021 IBM All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gateway

import (
	"fmt"
	"sort"
	"sync"

	"github.com/golang/protobuf/proto"
	dp "github.com/hyperledger/fabric-protos-go/discovery"
	"github.com/hyperledger/fabric-protos-go/gossip"
	"github.com/hyperledger/fabric/common/flogging"
	gossipapi "github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/common"
	gossipdiscovery "github.com/hyperledger/fabric/gossip/discovery"
)

type Discovery interface {
	Config(channel string) (*dp.ConfigResult, error)
	IdentityInfo() gossipapi.PeerIdentitySet
	PeersForEndorsement(channel common.ChannelID, interest *dp.ChaincodeInterest) (*dp.EndorsementDescriptor, error)
	PeersOfChannel(common.ChannelID) gossipdiscovery.Members
}

type registry struct {
	localEndorser       *endorser
	discovery           Discovery
	logger              *flogging.FabricLogger
	endpointFactory     *endpointFactory
	remoteEndorsers     map[string]*endorser
	broadcastClients    map[string]*orderer
	tlsRootCerts        map[string][][]byte
	channelsInitialized map[string]bool
	configLock          sync.RWMutex
}

type endorserState struct {
	peer     *dp.Peer
	endpoint string
	height   uint64
}

// Returns a set of endorsers that satisfies the endorsement plan for the given chaincode on a channel.
func (reg *registry) endorsers(channel string, chaincode string) ([]*endorser, error) {
	err := reg.registerChannel(channel)
	if err != nil {
		return nil, err
	}

	var endorsers []*endorser

	interest := &dp.ChaincodeInterest{
		Chaincodes: []*dp.ChaincodeCall{{
			Name: chaincode,
		}},
	}

	descriptor, err := reg.discovery.PeersForEndorsement(common.ChannelID(channel), interest)
	if err != nil {
		return nil, err
	}

	e := descriptor.EndorsersByGroups

	reg.configLock.RLock()
	defer reg.configLock.RUnlock()

	// choose first layout for now - todo implement retry logic
	if len(descriptor.Layouts) > 0 {
		layout := descriptor.Layouts[0].QuantitiesByGroup
		var r []*endorserState
		for group, quantity := range layout {
			// Select n remoteEndorsers from each group sorted by block height

			// block heights
			var groupPeers []*endorserState
			for _, peer := range e[group].Peers {
				msg := &gossip.GossipMessage{}
				err := proto.Unmarshal(peer.StateInfo.Payload, msg)
				if err != nil {
					return nil, err
				}

				height := msg.GetStateInfo().Properties.LedgerHeight
				err = proto.Unmarshal(peer.MembershipInfo.Payload, msg)
				if err != nil {
					return nil, err
				}
				endpoint := msg.GetAliveMsg().Membership.Endpoint

				groupPeers = append(groupPeers, &endorserState{peer: peer, endpoint: endpoint, height: height})
			}
			// sort by decreasing height
			sort.Slice(groupPeers, sorter(groupPeers, reg.localEndorser.address))

			peerGroup := groupPeers[0:quantity]
			r = append(r, peerGroup...)
		}
		// sort the group of groups by decreasing height (since Evaluate will just choose the first one)
		sort.Slice(r, sorter(r, reg.localEndorser.address))

		for _, peer := range r {
			if peer.endpoint == reg.localEndorser.address {
				endorsers = append(endorsers, reg.localEndorser)
			} else if endorser, ok := reg.remoteEndorsers[peer.endpoint]; ok {
				endorsers = append(endorsers, endorser)
			} else {
				reg.logger.Warnf("Failed to find endorser at %s", peer.endpoint)
			}
		}
	}

	return endorsers, nil
}

func sorter(e []*endorserState, host string) func(i, j int) bool {
	return func(i, j int) bool {
		if e[i].height == e[j].height {
			// prefer host peer
			return e[i].endpoint == host
		}
		return e[i].height > e[j].height
	}
}

// Returns a set of broadcastClients that can order a transaction for the given channel.
func (reg *registry) orderers(channel string) ([]*orderer, error) {
	err := reg.registerChannel(channel)
	if err != nil {
		return nil, err
	}
	var orderers []*orderer

	// Get the config
	config, err := reg.discovery.Config(channel)
	if err != nil {
		return nil, err
	}

	reg.configLock.RLock()
	defer reg.configLock.RUnlock()

	for _, eps := range config.GetOrderers() {
		for _, ep := range eps.Endpoint {
			url := fmt.Sprintf("%s:%d", ep.Host, ep.Port)
			if ord, ok := reg.broadcastClients[url]; ok {
				orderers = append(orderers, ord)
			}
		}
	}

	return orderers, nil
}

func (reg *registry) registerChannel(channel string) error {
	// todo need to handle membership updates
	reg.configLock.Lock() // take a write lock to populate the registry maps
	defer reg.configLock.Unlock()

	if reg.channelsInitialized[channel] {
		return nil
	}
	// get config and peer discovery info for this channel

	// Get the config
	config, err := reg.discovery.Config(channel)
	if err != nil {
		return err
	}
	// get the tlscerts
	for msp, info := range config.GetMsps() {
		reg.tlsRootCerts[msp] = info.GetTlsRootCerts()
	}

	for mspid, eps := range config.GetOrderers() {
		for _, ep := range eps.Endpoint {
			address := fmt.Sprintf("%s:%d", ep.Host, ep.Port)
			if _, ok := reg.broadcastClients[address]; !ok {
				// this orderer is new - connect to it and add to the broadcastClients registry
				tlsRootCerts := reg.tlsRootCerts[mspid]
				orderer, err := reg.endpointFactory.newOrderer(address, mspid, tlsRootCerts)
				if err != nil {
					return err
				}
				reg.broadcastClients[address] = orderer
				reg.logger.Infof("Added orderer to registry: %s", address)
			}
		}
	}

	// get the remoteEndorsers for the channel
	peers := map[string]string{}
	members := reg.discovery.PeersOfChannel(common.ChannelID(channel))
	for _, member := range members {
		id := member.PKIid.String() // TODO this is fragile
		peers[id] = member.PreferredEndpoint()
	}
	for mspid, infoset := range reg.discovery.IdentityInfo().ByOrg() {
		for _, info := range infoset {
			pkid := info.PKIId.String()
			if address, ok := peers[pkid]; ok {
				// add the peer to the peer map - except the local peer, which seems to have an empty address
				if _, ok := reg.remoteEndorsers[address]; !ok && len(address) > 0 {
					// this peer is new - connect to it and add to the remoteEndorsers registry
					tlsRootCerts := reg.tlsRootCerts[mspid]
					endorser, err := reg.endpointFactory.newEndorser(address, mspid, tlsRootCerts)
					if err != nil {
						return err
					}
					reg.remoteEndorsers[address] = endorser
					reg.logger.Infof("Added peer to registry: %s", address)
				}
			}
		}
	}
	reg.channelsInitialized[channel] = true

	return nil
}
