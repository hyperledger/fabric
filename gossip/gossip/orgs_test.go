/*
Copyright IBM Corp. 2017 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

                 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package gossip

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/gossip/identity"
	"github.com/stretchr/testify/assert"
)

func init() {
	aliveTimeInterval := time.Duration(1000) * time.Millisecond
	discovery.SetAliveTimeInterval(aliveTimeInterval)
	discovery.SetAliveExpirationCheckInterval(aliveTimeInterval)
	discovery.SetAliveExpirationTimeout(aliveTimeInterval * 10)
	discovery.SetReconnectInterval(aliveTimeInterval)
	factory.InitFactories(nil)
}

type configurableCryptoService struct {
	m map[string]api.OrgIdentityType
}

func (c *configurableCryptoService) putInOrg(port int, org string) {
	identity := fmt.Sprintf("localhost:%d", port)
	c.m[identity] = api.OrgIdentityType(org)
}

// OrgByPeerIdentity returns the OrgIdentityType
// of a given peer identity
func (c *configurableCryptoService) OrgByPeerIdentity(identity api.PeerIdentityType) api.OrgIdentityType {
	org := c.m[string(identity)]
	return org
}

// VerifyByChannel verifies a peer's signature on a message in the context
// of a specific channel
func (cs *configurableCryptoService) VerifyByChannel(_ common.ChainID, identity api.PeerIdentityType, _, _ []byte) error {
	return nil
}

func (*configurableCryptoService) ValidateIdentity(peerIdentity api.PeerIdentityType) error {
	return nil
}

// GetPKIidOfCert returns the PKI-ID of a peer's identity
func (*configurableCryptoService) GetPKIidOfCert(peerIdentity api.PeerIdentityType) common.PKIidType {
	return common.PKIidType(peerIdentity)
}

// VerifyBlock returns nil if the block is properly signed,
// else returns error
func (*configurableCryptoService) VerifyBlock(chainID common.ChainID, signedBlock []byte) error {
	return nil
}

// Sign signs msg with this peer's signing key and outputs
// the signature if no error occurred.
func (*configurableCryptoService) Sign(msg []byte) ([]byte, error) {
	sig := make([]byte, len(msg))
	copy(sig, msg)
	return sig, nil
}

// Verify checks that signature is a valid signature of message under a peer's verification key.
// If the verification succeeded, Verify returns nil meaning no error occurred.
// If peerCert is nil, then the signature is verified against this peer's verification key.
func (*configurableCryptoService) Verify(peerIdentity api.PeerIdentityType, signature, message []byte) error {
	equal := bytes.Equal(signature, message)
	if !equal {
		return fmt.Errorf("Wrong signature:%v, %v", signature, message)
	}
	return nil
}

func newGossipInstanceWithExternalEndpoint(portPrefix int, id int, mcs *configurableCryptoService, externalEndpoint string, boot ...int) Gossip {
	port := id + portPrefix
	conf := &Config{
		BindPort:                   port,
		BootstrapPeers:             bootPeers(portPrefix, boot...),
		ID:                         fmt.Sprintf("p%d", id),
		MaxBlockCountToStore:       100,
		MaxPropagationBurstLatency: time.Duration(500) * time.Millisecond,
		MaxPropagationBurstSize:    20,
		PropagateIterations:        1,
		PropagatePeerNum:           3,
		PullInterval:               time.Duration(2) * time.Second,
		PullPeerNum:                5,
		InternalEndpoint:           fmt.Sprintf("localhost:%d", port),
		ExternalEndpoint:           externalEndpoint,
		PublishCertPeriod:          time.Duration(4) * time.Second,
		PublishStateInfoInterval:   time.Duration(1) * time.Second,
		RequestStateInfoInterval:   time.Duration(1) * time.Second,
	}

	idMapper := identity.NewIdentityMapper(mcs)
	g := NewGossipServiceWithServer(conf, mcs, mcs, idMapper, api.PeerIdentityType(conf.InternalEndpoint))

	return g
}

func TestMultipleOrgEndpointLeakage(t *testing.T) {
	t.Parallel()
	// Scenario: create 2 organizations, each with 5 peers.
	// The first org will have an anchor peer, but the second won't.
	// The first 2 peers of each org would have an external endpoint, the rest won't.
	// Have all peers join a channel with the 2 organizations.
	// Ensure that after membership is stabilized:
	// - The only peers that know peers of other organizations are peers that
	//   have an external endpoint configured.
	// - The peers with external endpoint do not know the internal endpoints
	//   of the peers of the other organizations.
	cs := &configurableCryptoService{m: make(map[string]api.OrgIdentityType)}
	portPrefix := 11610
	peersInOrg := 5
	orgA := "orgA"
	orgB := "orgB"
	orgs := []string{orgA, orgB}
	orgs2Peers := map[string][]Gossip{
		orgs[0]: {},
		orgs[1]: {},
	}
	expectedMembershipSize := map[string]int{}
	peers2Orgs := map[string]api.OrgIdentityType{}
	peersWithExternalEndpoints := make(map[string]struct{})

	shouldAKnowB := func(a common.PKIidType, b common.PKIidType) bool {
		orgOfPeerA := cs.OrgByPeerIdentity(api.PeerIdentityType(a))
		orgOfPeerB := cs.OrgByPeerIdentity(api.PeerIdentityType(b))
		_, aHasExternalEndpoint := peersWithExternalEndpoints[string(a)]
		_, bHasExternalEndpoint := peersWithExternalEndpoints[string(b)]
		bothHaveExternalEndpoints := aHasExternalEndpoint && bHasExternalEndpoint
		return bytes.Equal(orgOfPeerA, orgOfPeerB) || bothHaveExternalEndpoints
	}

	shouldKnowInternalEndpoint := func(a common.PKIidType, b common.PKIidType) bool {
		orgOfPeerA := cs.OrgByPeerIdentity(api.PeerIdentityType(a))
		orgOfPeerB := cs.OrgByPeerIdentity(api.PeerIdentityType(b))
		return bytes.Equal(orgOfPeerA, orgOfPeerB)
	}

	amountOfPeersShouldKnow := func(pkiID common.PKIidType) int {
		return expectedMembershipSize[string(pkiID)]
	}

	for orgIndex := 0; orgIndex < 2; orgIndex++ {
		for i := 0; i < peersInOrg; i++ {
			id := orgIndex*peersInOrg + i
			port := id + portPrefix
			org := orgs[orgIndex]
			endpoint := fmt.Sprintf("localhost:%d", port)
			peers2Orgs[endpoint] = api.OrgIdentityType(org)
			cs.putInOrg(port, org)
			membershipSizeExpected := peersInOrg - 1 // All the others in its org
			var peer Gossip
			var bootPeers []int
			if orgIndex == 0 {
				bootPeers = []int{0}
			}
			externalEndpoint := ""
			if i < 2 { // The first 2 peers of each org would have an external endpoint
				externalEndpoint = endpoint
				peersWithExternalEndpoints[externalEndpoint] = struct{}{}
				membershipSizeExpected += 2 // should know the extra 2 peers from the other org
			}
			expectedMembershipSize[endpoint] = membershipSizeExpected
			peer = newGossipInstanceWithExternalEndpoint(portPrefix, id, cs, externalEndpoint, bootPeers...)
			orgs2Peers[org] = append(orgs2Peers[org], peer)
		}
	}

	jcm := &joinChanMsg{
		members2AnchorPeers: map[string][]api.AnchorPeer{
			orgA: {
				{Host: "localhost", Port: 11611},
				{Host: "localhost", Port: 11616},
			},
			orgB: {
				{Host: "localhost", Port: 11615},
			},
		},
	}

	channel := common.ChainID("TEST")

	for _, peers := range orgs2Peers {
		for _, p := range peers {
			p.JoinChan(jcm, channel)
			p.UpdateChannelMetadata([]byte("bla"), channel)
		}
	}

	time.Sleep(time.Second * 10)

	for _, peers := range orgs2Peers {
		for _, p := range peers {
			peerNetMember := p.(*gossipServiceImpl).selfNetworkMember()
			pkiID := peerNetMember.PKIid
			peersKnown := p.Peers()
			assert.Equal(t, amountOfPeersShouldKnow(pkiID), len(peersKnown), "peer %v doesn't know the needed amount of peers", peerNetMember.Endpoint)
			for _, knownPeer := range peersKnown {
				assert.True(t, shouldAKnowB(pkiID, knownPeer.PKIid))
				if shouldKnowInternalEndpoint(pkiID, knownPeer.PKIid) {
					errMsg := fmt.Sprintf("peer: %v doesn't know internal endpoint of %v",
						peerNetMember.InternalEndpoint, string(knownPeer.PKIid))
					assert.NotZero(t, knownPeer.InternalEndpoint, errMsg)
				} else {
					errMsg := fmt.Sprintf("peer: %v knows internal endpoint of %v",
						peerNetMember.InternalEndpoint, string(knownPeer.PKIid))
					assert.Zero(t, knownPeer.InternalEndpoint, errMsg)
				}
			}
		}
	}

	for _, peers := range orgs2Peers {
		for _, p := range peers {
			p.Stop()
		}
	}
}
