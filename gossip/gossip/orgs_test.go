/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gossip

import (
	"bytes"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/gossip/util"
	proto "github.com/hyperledger/fabric/protos/gossip"
	"github.com/stretchr/testify/assert"
)

func init() {
	util.SetupTestLogging()
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

func (c *configurableCryptoService) Expiration(peerIdentity api.PeerIdentityType) (time.Time, error) {
	return time.Now().Add(time.Hour), nil
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
func (c *configurableCryptoService) VerifyByChannel(_ common.ChainID, identity api.PeerIdentityType, _, _ []byte) error {
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
func (*configurableCryptoService) VerifyBlock(chainID common.ChainID, seqNum uint64, signedBlock []byte) error {
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
	selfID := api.PeerIdentityType(conf.InternalEndpoint)
	g := NewGossipServiceWithServer(conf, mcs, mcs, selfID,
		nil)

	return g
}

func TestMultipleOrgEndpointLeakage(t *testing.T) {
	t.Parallel()
	defer testWG.Done()
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
			p.UpdateLedgerHeight(1, channel)
		}
	}

	membershipCheck := func() bool {
		for _, peers := range orgs2Peers {
			for _, p := range peers {
				peerNetMember := p.(*gossipServiceImpl).selfNetworkMember()
				pkiID := peerNetMember.PKIid
				peersKnown := p.Peers()
				peersToKnow := amountOfPeersShouldKnow(pkiID)
				if peersToKnow != len(peersKnown) {
					t.Logf("peer %#v doesn't know the needed amount of peers, extected %#v, actual %#v", peerNetMember.Endpoint, peersToKnow, len(peersKnown))
					return false
				}
				for _, knownPeer := range peersKnown {
					if !shouldAKnowB(pkiID, knownPeer.PKIid) {
						assert.Fail(t, fmt.Sprintf("peer %#v doesn't know %#v", peerNetMember.Endpoint, knownPeer.Endpoint))
						return false
					}
					internalEndpointLen := len(knownPeer.InternalEndpoint)
					if shouldKnowInternalEndpoint(pkiID, knownPeer.PKIid) {
						if internalEndpointLen == 0 {
							t.Logf("peer: %v doesn't know internal endpoint of %v", peerNetMember.InternalEndpoint, string(knownPeer.PKIid))
							return false
						}
					} else {
						if internalEndpointLen != 0 {
							assert.Fail(t, fmt.Sprintf("peer: %v knows internal endpoint of %v (%#v)", peerNetMember.InternalEndpoint, string(knownPeer.PKIid), knownPeer.InternalEndpoint))
							return false
						}
					}
				}
			}
		}
		return true
	}

	waitUntilOrFail(t, membershipCheck)

	for _, peers := range orgs2Peers {
		for _, p := range peers {
			p.Stop()
		}
	}
}

func TestConfidentiality(t *testing.T) {
	t.Parallel()
	defer testWG.Done()
	// Scenario: create 4 organizations: {A, B, C, D}, each with 3 peers.
	// Make only the first 2 peers have an external endpoint.
	// Also, add the peers to the following channels:
	// Channel C0: { orgA, orgB }
	// Channel C1: { orgA, orgC }
	// Channel C2: { orgB, orgC }
	// Channel C3: { orgB, orgD }
	//  [ A ]-C0-[ B ]-C3-[ D ]
	//   |      /
	//   |     /
	//   C1   C2
	//   |   /
	//   |  /
	//   [ C ]
	// Subscribe to all membership messages for each peer,
	// and fail the test if a message is sent to a peer in org X,
	// from a peer in org Y about a peer in org Z not in {X, Y}
	// or if any org other than orgB knows peers in orgD (and vice versa).

	portPrefix := 12610
	peersInOrg := 3
	externalEndpointsInOrg := 2

	// orgA: {12610, 12611, 12612}
	// orgB: {12613, 12614, 12615}
	// orgC: {12616, 12617, 12618}
	// orgD: {12619, 12620, 12621}
	peersWithExternalEndpoints := map[string]struct{}{}

	orgs := []string{"A", "B", "C", "D"}
	channels := []string{"C0", "C1", "C2", "C3"}
	isOrgInChan := func(org string, channel string) bool {
		switch org {
		case "A":
			return channel == "C0" || channel == "C1"
		case "B":
			return channel == "C0" || channel == "C2" || channel == "C3"
		case "C":
			return channel == "C1" || channel == "C2"
		case "D":
			return channel == "C3"
		}

		return false
	}

	// Create the message crypto service
	cs := &configurableCryptoService{m: make(map[string]api.OrgIdentityType)}
	for i, org := range orgs {
		for j := 0; j < peersInOrg; j++ {
			port := portPrefix + i*peersInOrg + j
			cs.putInOrg(port, org)
		}
	}

	var peers []Gossip
	orgs2Peers := map[string][]Gossip{
		"A": {},
		"B": {},
		"C": {},
		"D": {},
	}

	anchorPeersByOrg := map[string]api.AnchorPeer{}

	for i, org := range orgs {
		for j := 0; j < peersInOrg; j++ {
			id := i*peersInOrg + j
			port := id + portPrefix
			endpoint := fmt.Sprintf("localhost:%d", port)
			externalEndpoint := ""
			if j < externalEndpointsInOrg { // The first peers of each org would have an external endpoint
				externalEndpoint = endpoint
				peersWithExternalEndpoints[string(endpoint)] = struct{}{}
			}
			peer := newGossipInstanceWithExternalEndpoint(portPrefix, id, cs, externalEndpoint)
			peers = append(peers, peer)
			orgs2Peers[org] = append(orgs2Peers[org], peer)
			t.Log(endpoint, "id:", id, "externalEndpoint:", externalEndpoint)
			// The first peer of the org will be used as the anchor peer
			if j == 0 {
				anchorPeersByOrg[org] = api.AnchorPeer{
					Host: "localhost",
					Port: port,
				}
			}
		}
	}

	msgs2Inspect := make(chan *msg, 3000)
	defer close(msgs2Inspect)
	go inspectMsgs(t, msgs2Inspect, cs, peersWithExternalEndpoints)
	finished := int32(0)
	var wg sync.WaitGroup

	msgSelector := func(o interface{}) bool {
		msg := o.(proto.ReceivedMessage).GetGossipMessage()
		identitiesPull := msg.IsPullMsg() && msg.GetPullMsgType() == proto.PullMsgType_IDENTITY_MSG
		return msg.IsAliveMsg() || msg.IsStateInfoMsg() || msg.IsStateInfoSnapshot() || msg.GetMemRes() != nil || identitiesPull
	}
	// Listen to all peers membership messages and forward them to the inspection channel
	// where they will be inspected, and the test would fail if a confidentiality violation is found
	for _, p := range peers {
		wg.Add(1)
		_, msgs := p.Accept(msgSelector, true)
		peerNetMember := p.(*gossipServiceImpl).selfNetworkMember()
		targetORg := string(cs.OrgByPeerIdentity(api.PeerIdentityType(peerNetMember.InternalEndpoint)))
		go func(targetOrg string, msgs <-chan proto.ReceivedMessage) {
			defer wg.Done()
			for receivedMsg := range msgs {
				m := &msg{
					src:           string(cs.OrgByPeerIdentity(receivedMsg.GetConnectionInfo().Identity)),
					dst:           targetORg,
					GossipMessage: receivedMsg.GetGossipMessage().GossipMessage,
				}
				if atomic.LoadInt32(&finished) == int32(1) {
					return
				}
				msgs2Inspect <- m
			}
		}(targetORg, msgs)
	}

	// Now, construct the join channel messages
	joinChanMsgsByChan := map[string]*joinChanMsg{}
	for _, ch := range channels {
		jcm := &joinChanMsg{members2AnchorPeers: map[string][]api.AnchorPeer{}}
		for _, org := range orgs {
			if isOrgInChan(org, ch) {
				jcm.members2AnchorPeers[org] = append(jcm.members2AnchorPeers[org], anchorPeersByOrg[org])
			}
		}
		joinChanMsgsByChan[ch] = jcm
	}

	// Next, make the peers join the channels
	for org, peers := range orgs2Peers {
		for _, ch := range channels {
			if isOrgInChan(org, ch) {
				for _, p := range peers {
					p.JoinChan(joinChanMsgsByChan[ch], common.ChainID(ch))
					p.UpdateLedgerHeight(1, common.ChainID(ch))
					go func(p Gossip) {
						for i := 0; i < 5; i++ {
							time.Sleep(time.Second)
							p.UpdateLedgerHeight(1, common.ChainID(ch))
						}
					}(p)
				}
			}
		}
	}

	// Sleep a bit, to let peers gossip with each other
	time.Sleep(time.Second * 7)

	assertMembership := func() bool {
		for _, org := range orgs {
			for i, p := range orgs2Peers[org] {
				members := p.Peers()
				expMemberSize := expectedMembershipSize(peersInOrg, externalEndpointsInOrg, org, i < externalEndpointsInOrg)
				peerNetMember := p.(*gossipServiceImpl).selfNetworkMember()
				membersCount := len(members)
				if membersCount < expMemberSize {
					return false
				}
				// Make sure no one knows too much
				assert.True(t, membersCount <= expMemberSize, "%s knows too much (%d > %d) peers: %v",
					membersCount, expMemberSize, peerNetMember.PKIid, members)
			}
		}
		return true
	}

	waitUntilOrFail(t, assertMembership)
	stopPeers(peers)
	wg.Wait()
	atomic.StoreInt32(&finished, int32(1))
}

func expectedMembershipSize(peersInOrg, externalEndpointsInOrg int, org string, hasExternalEndpoint bool) int {
	// x <-- peersInOrg
	// y <-- externalEndpointsInOrg
	//  (x+2y)[ A ]-C0-[ B ]---C3--[ D ] (x+y)
	//          |      /(x+3y)
	//          |     /
	//          C1   C2
	//          |   /
	//          |  /
	//        [ C ]  x+2y

	m := map[string]func(x, y int) int{
		"A": func(x, y int) int {
			return x + 2*y
		},
		"B": func(x, y int) int {
			return x + 3*y
		},
		"C": func(x, y int) int {
			return x + 2*y
		},
		"D": func(x, y int) int {
			return x + y
		},
	}

	// If the peer doesn't have an external endpoint,
	// it doesn't know peers from foreign organizations that have one
	if !hasExternalEndpoint {
		externalEndpointsInOrg = 0
	}
	// Deduct 1 because the peer itself doesn't count
	return m[org](peersInOrg, externalEndpointsInOrg) - 1
}

func extractOrgsFromMsg(msg *proto.GossipMessage, sec api.SecurityAdvisor) []string {
	if msg.IsAliveMsg() {
		return []string{string(sec.OrgByPeerIdentity(api.PeerIdentityType(msg.GetAliveMsg().Membership.PkiId)))}
	}

	orgs := map[string]struct{}{}

	if msg.IsPullMsg() {
		if msg.IsDigestMsg() || msg.IsDataReq() {
			var digests []string
			if msg.IsDigestMsg() {
				digests = util.BytesToStrings(msg.GetDataDig().Digests)
			} else {
				digests = util.BytesToStrings(msg.GetDataReq().Digests)
			}

			for _, dig := range digests {
				org := sec.OrgByPeerIdentity(api.PeerIdentityType(dig))
				orgs[string(org)] = struct{}{}
			}
		}

		if msg.IsDataUpdate() {
			for _, identityMsg := range msg.GetDataUpdate().Data {
				gMsg, _ := identityMsg.ToGossipMessage()
				id := string(gMsg.GetPeerIdentity().Cert)
				org := sec.OrgByPeerIdentity(api.PeerIdentityType(id))
				orgs[string(org)] = struct{}{}
			}
		}
	}

	if msg.GetMemRes() != nil {
		alive := msg.GetMemRes().Alive
		dead := msg.GetMemRes().Dead
		for _, envp := range append(alive, dead...) {
			msg, _ := envp.ToGossipMessage()
			orgs[string(sec.OrgByPeerIdentity(api.PeerIdentityType(msg.GetAliveMsg().Membership.PkiId)))] = struct{}{}
		}
	}

	res := []string{}
	for org := range orgs {
		res = append(res, org)
	}
	return res
}

func inspectMsgs(t *testing.T, msgChan chan *msg, sec api.SecurityAdvisor, peersWithExternalEndpoints map[string]struct{}) {
	for msg := range msgChan {
		// If the destination org is the same as the source org,
		// the message can contain any organizations
		if msg.src == msg.dst {
			continue
		}
		if msg.IsStateInfoMsg() || msg.IsStateInfoSnapshot() {
			inspectStateInfoMsg(t, msg, peersWithExternalEndpoints)
			continue
		}
		// Else, it's a cross-organizational message.
		// Denote src organization as s and dst organization as d.
		// The total organizations of the message must be a subset of s U d.
		orgs := extractOrgsFromMsg(msg.GossipMessage, sec)
		s := []string{msg.src, msg.dst}
		assert.True(t, isSubset(orgs, s), "%v isn't a subset of %v", orgs, s)

		// Ensure no one but B knows about D and vice versa
		if msg.dst == "D" {
			assert.NotContains(t, "A", orgs)
			assert.NotContains(t, "C", orgs)
		}

		if msg.dst == "A" || msg.dst == "C" {
			assert.NotContains(t, "D", orgs)
		}

		// If this is an identity snapshot, make sure that only identities of peers
		// with external endpoints pass between the organizations.
		isIdentityPull := msg.IsPullMsg() && msg.GetPullMsgType() == proto.PullMsgType_IDENTITY_MSG
		if !(isIdentityPull && msg.IsDataUpdate()) {
			continue
		}
		for _, envp := range msg.GetDataUpdate().Data {
			identityMsg, _ := envp.ToGossipMessage()
			pkiID := identityMsg.GetPeerIdentity().PkiId
			_, hasExternalEndpoint := peersWithExternalEndpoints[string(pkiID)]
			assert.True(t, hasExternalEndpoint,
				"Peer %s doesn't have an external endpoint but its identity was gossiped", string(pkiID))
		}
	}
}

func inspectStateInfoMsg(t *testing.T, m *msg, peersWithExternalEndpoints map[string]struct{}) {
	if m.IsStateInfoMsg() {
		pkiID := m.GetStateInfo().PkiId
		_, hasExternalEndpoint := peersWithExternalEndpoints[string(pkiID)]
		assert.True(t, hasExternalEndpoint, "peer %s has no external endpoint but crossed an org", string(pkiID))
		return
	}

	for _, envp := range m.GetStateSnapshot().Elements {
		msg, _ := envp.ToGossipMessage()
		pkiID := msg.GetStateInfo().PkiId
		_, hasExternalEndpoint := peersWithExternalEndpoints[string(pkiID)]
		assert.True(t, hasExternalEndpoint, "peer %s has no external endpoint but crossed an org", string(pkiID))
	}
}

type msg struct {
	src string
	dst string
	*proto.GossipMessage
}

func isSubset(a []string, b []string) bool {
	for _, s1 := range a {
		found := false
		for _, s2 := range b {
			if s1 == s2 {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
