/*
Copyright 2021 IBM All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gateway

import (
	"fmt"
	"math/rand"
	"sync"

	"github.com/golang/protobuf/proto"
	dp "github.com/hyperledger/fabric-protos-go/discovery"
	"github.com/hyperledger/fabric-protos-go/gossip"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric-protos-go/peer"
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

type (
	endorserFactory func(address string, tlsRootCerts [][]byte) (peer.EndorserClient, error)
	ordererFactory  func(address string, tlsRootCerts [][]byte) (ab.AtomicBroadcast_BroadcastClient, error)
)

type registry struct {
	localEndorser       peer.EndorserClient
	discovery           Discovery
	selfEndpoint        string
	logger              *flogging.FabricLogger
	endorserFactory     endorserFactory
	ordererFactory      ordererFactory
	remoteEndorsers     map[string]peer.EndorserClient
	broadcastClients    map[string]ab.AtomicBroadcast_BroadcastClient
	tlsRootCerts        map[string][][]byte
	channelsInitialized map[string]bool
	configLock          sync.RWMutex
}

// Returns a set of endorsers that satisfies the endorsement plan for the given chaincode on a channel.
func (reg *registry) endorsers(channel string, chaincode string) ([]peer.EndorserClient, error) {
	err := reg.registerChannel(channel)
	if err != nil {
		return nil, err
	}

	var endorsers []peer.EndorserClient

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
		var r []*dp.Peer
		for group, quantity := range layout {
			// For now, select n remoteEndorsers from each group at random. Future story, sort by block height
			endorsers := e[group].Peers
			rand.Shuffle(len(endorsers), func(i, j int) {
				endorsers[i], endorsers[j] = endorsers[j], endorsers[i]
			})
			peerGroup := endorsers[0:quantity]
			r = append(r, peerGroup...)
		}

		for _, peer := range r {
			msg := &gossip.GossipMessage{}
			err := proto.Unmarshal(peer.MembershipInfo.Payload, msg)
			if err != nil {
				return nil, err
			}
			endpoint := msg.GetAliveMsg().Membership.Endpoint
			if endpoint == reg.selfEndpoint {
				endorsers = append(endorsers, reg.localEndorser)
			} else if endorser, ok := reg.remoteEndorsers[endpoint]; ok {
				endorsers = append(endorsers, endorser)
			} else {
				reg.logger.Warnf("Failed to find endorser at %s", endpoint)
			}
		}
	}

	return endorsers, nil
}

// Returns a set of broadcastClients that can order a transaction for the given channel.
func (reg *registry) orderers(channel string) ([]ab.AtomicBroadcast_BroadcastClient, error) {
	err := reg.registerChannel(channel)
	if err != nil {
		return nil, err
	}
	var orderers []ab.AtomicBroadcast_BroadcastClient

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
				orderer, err := reg.ordererFactory(address, tlsRootCerts)
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
					endorser, err := reg.endorserFactory(address, tlsRootCerts)
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
