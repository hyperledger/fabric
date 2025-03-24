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

	"github.com/hyperledger/fabric-lib-go/bccsp/factory"
	"github.com/hyperledger/fabric-lib-go/common/metrics/disabled"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	proto "github.com/hyperledger/fabric-protos-go-apiv2/gossip"
	"github.com/hyperledger/fabric/gossip/api"
	gcomm "github.com/hyperledger/fabric/gossip/comm"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/gossip/algo"
	"github.com/hyperledger/fabric/gossip/gossip/channel"
	"github.com/hyperledger/fabric/gossip/metrics"
	"github.com/hyperledger/fabric/gossip/protoext"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/stretchr/testify/require"
)

func init() {
	util.SetupTestLogging()
	factory.InitFactories(nil)
}

type configurableCryptoService struct {
	sync.RWMutex
	m map[string]api.OrgIdentityType
}

func (c *configurableCryptoService) Expiration(peerIdentity api.PeerIdentityType) (time.Time, error) {
	return time.Now().Add(time.Hour), nil
}

func (c *configurableCryptoService) putInOrg(port int, org string) {
	identity := fmt.Sprintf("127.0.0.1:%d", port)
	c.Lock()
	c.m[identity] = api.OrgIdentityType(org)
	c.Unlock()
}

// OrgByPeerIdentity returns the OrgIdentityType
// of a given peer identity
func (c *configurableCryptoService) OrgByPeerIdentity(identity api.PeerIdentityType) api.OrgIdentityType {
	c.RLock()
	org := c.m[string(identity)]
	c.RUnlock()
	return org
}

// VerifyByChannel verifies a peer's signature on a message in the context
// of a specific channel
func (c *configurableCryptoService) VerifyByChannel(_ common.ChannelID, identity api.PeerIdentityType, _, _ []byte) error {
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
func (*configurableCryptoService) VerifyBlock(channelID common.ChannelID, seqNum uint64, signedBlock *cb.Block) error {
	return nil
}

// VerifyBlockAttestation returns nil if the block attestation is properly signed,
// else returns error
func (*configurableCryptoService) VerifyBlockAttestation(channelID string, signedBlock *cb.Block) error {
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

func newGossipInstanceWithGRPCWithExternalEndpoint(id int, port int, gRPCServer *comm.GRPCServer,
	certs *common.TLSCertificates, secureDialOpts api.PeerSecureDialOpts, mcs *configurableCryptoService,
	externalEndpoint string, boot ...int) *gossipGRPC {
	conf := &Config{
		BootstrapPeers:               bootPeersWithPorts(boot...),
		ID:                           fmt.Sprintf("p%d", id),
		MaxBlockCountToStore:         10,
		MaxPropagationBurstLatency:   time.Duration(500) * time.Millisecond,
		MaxPropagationBurstSize:      20,
		PropagateIterations:          1,
		PropagatePeerNum:             3,
		PullInterval:                 time.Duration(2) * time.Second,
		PullPeerNum:                  5,
		InternalEndpoint:             fmt.Sprintf("127.0.0.1:%d", port),
		ExternalEndpoint:             externalEndpoint,
		PublishCertPeriod:            time.Duration(4) * time.Second,
		PublishStateInfoInterval:     time.Duration(1) * time.Second,
		RequestStateInfoInterval:     time.Duration(1) * time.Second,
		TimeForMembershipTracker:     5 * time.Second,
		TLSCerts:                     certs,
		DigestWaitTime:               algo.DefDigestWaitTime,
		RequestWaitTime:              algo.DefRequestWaitTime,
		ResponseWaitTime:             algo.DefResponseWaitTime,
		DialTimeout:                  gcomm.DefDialTimeout,
		ConnTimeout:                  gcomm.DefConnTimeout,
		RecvBuffSize:                 gcomm.DefRecvBuffSize,
		SendBuffSize:                 gcomm.DefSendBuffSize,
		MsgExpirationTimeout:         channel.DefMsgExpirationTimeout,
		AliveTimeInterval:            discoveryConfig.AliveTimeInterval,
		AliveExpirationTimeout:       discoveryConfig.AliveExpirationTimeout,
		AliveExpirationCheckInterval: discoveryConfig.AliveExpirationCheckInterval,
		ReconnectInterval:            discoveryConfig.ReconnectInterval,
		MaxConnectionAttempts:        discoveryConfig.MaxConnectionAttempts,
		MsgExpirationFactor:          discoveryConfig.MsgExpirationFactor,
	}
	selfID := api.PeerIdentityType(conf.InternalEndpoint)
	g := New(conf, gRPCServer.Server(), mcs, mcs, selfID,
		secureDialOpts, metrics.NewGossipMetrics(&disabled.Provider{}), nil)
	go func() {
		gRPCServer.Start()
	}()
	return &gossipGRPC{Node: g, grpc: gRPCServer}
}

func TestMultipleOrgEndpointLeakage(t *testing.T) {
	// Scenario: create 2 organizations, each with 5 peers.
	// Both organizations will have an anchor peer each
	// The first 2 peers of each org would have an external endpoint, the rest won't.
	// Have all peers join a channel with the 2 organizations.
	// Ensure that after membership is stabilized:
	// - The only peers that know peers of other organizations are peers that
	//   have an external endpoint configured.
	// - The peers with external endpoint do not know the internal endpoints
	//   of the peers of the other organizations.
	cs := &configurableCryptoService{m: make(map[string]api.OrgIdentityType)}
	peersInOrg := 5
	orgA := "orgA"
	orgB := "orgB"
	channel := common.ChannelID("TEST")
	orgs := []string{orgA, orgB}
	peers := []*gossipGRPC{}

	expectedMembershipSize := map[string]int{}
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

	var ports []int
	var grpcs []*comm.GRPCServer
	var certs []*common.TLSCertificates
	var secDialOpts []api.PeerSecureDialOpts

	for range orgs {
		for i := 0; i < peersInOrg; i++ {
			port, grpc, cert, secDialOpt, _ := util.CreateGRPCLayer()
			ports = append(ports, port)
			grpcs = append(grpcs, grpc)
			certs = append(certs, cert)
			secDialOpts = append(secDialOpts, secDialOpt)
		}
	}

	for orgIndex, org := range orgs {
		for i := 0; i < peersInOrg; i++ {
			id := orgIndex*peersInOrg + i
			endpoint := fmt.Sprintf("127.0.0.1:%d", ports[id])
			cs.putInOrg(ports[id], org)
			expectedMembershipSize[endpoint] = peersInOrg - 1 // All the others in its org
			externalEndpoint := ""
			if i < 2 {
				// The first 2 peers of each org would have an external endpoint.
				// orgA: 11610, 11611
				// orgB: 11615, 11616
				externalEndpoint = endpoint
				peersWithExternalEndpoints[externalEndpoint] = struct{}{}
				expectedMembershipSize[endpoint] += 2 // should know the extra 2 peers from the other org
			}
			peer := newGossipInstanceWithGRPCWithExternalEndpoint(id, ports[id], grpcs[id], certs[id], secDialOpts[id],
				cs, externalEndpoint)
			peers = append(peers, peer)
		}
	}

	jcm := &joinChanMsg{
		members2AnchorPeers: map[string][]api.AnchorPeer{
			orgA: {
				{Host: "127.0.0.1", Port: ports[1]},
			},
			orgB: {
				{Host: "127.0.0.1", Port: ports[5]},
			},
		},
	}

	for _, p := range peers {
		p.JoinChan(jcm, channel)
		p.UpdateLedgerHeight(1, channel)
	}

	membershipCheck := func() bool {
		for _, p := range peers {
			peerNetMember := p.Node.selfNetworkMember()
			pkiID := peerNetMember.PKIid
			peersKnown := p.Peers()
			peersToKnow := expectedMembershipSize[string(pkiID)]
			if peersToKnow != len(peersKnown) {
				t.Logf("peer %#v doesn't know the needed amount of peers, expected %#v, actual %#v", peerNetMember.Endpoint, peersToKnow, len(peersKnown))
				return false
			}
			for _, knownPeer := range peersKnown {
				if !shouldAKnowB(pkiID, knownPeer.PKIid) {
					require.Fail(t, fmt.Sprintf("peer %#v doesn't know %#v", peerNetMember.Endpoint, knownPeer.Endpoint))
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
						require.Fail(t, fmt.Sprintf("peer: %v knows internal endpoint of %v (%#v)", peerNetMember.InternalEndpoint, string(knownPeer.PKIid), knownPeer.InternalEndpoint))
						return false
					}
				}
			}
		}
		return true
	}

	waitUntilOrFail(t, membershipCheck, "waiting for all instances to form membership view")

	for _, p := range peers {
		p.Stop()
	}
}

func TestConfidentiality(t *testing.T) {
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

	var ports []int
	var grpcs []*comm.GRPCServer
	var certs []*common.TLSCertificates
	var secDialOpts []api.PeerSecureDialOpts

	for range orgs {
		for j := 0; j < peersInOrg; j++ {
			port, grpc, cert, secDialOpt, _ := util.CreateGRPCLayer()
			ports = append(ports, port)
			grpcs = append(grpcs, grpc)
			certs = append(certs, cert)
			secDialOpts = append(secDialOpts, secDialOpt)
		}
	}

	// Create the message crypto service
	cs := &configurableCryptoService{m: make(map[string]api.OrgIdentityType)}
	for i, org := range orgs {
		for j := 0; j < peersInOrg; j++ {
			port := ports[i*peersInOrg+j]
			cs.putInOrg(port, org)
		}
	}

	var peers []*gossipGRPC
	orgs2Peers := map[string][]*gossipGRPC{
		"A": {},
		"B": {},
		"C": {},
		"D": {},
	}

	anchorPeersByOrg := map[string]api.AnchorPeer{}

	for i, org := range orgs {
		for j := 0; j < peersInOrg; j++ {
			id := i*peersInOrg + j
			endpoint := fmt.Sprintf("127.0.0.1:%d", ports[id])
			externalEndpoint := ""
			if j < externalEndpointsInOrg { // The first peers of each org would have an external endpoint
				externalEndpoint = endpoint
				peersWithExternalEndpoints[endpoint] = struct{}{}
			}
			peer := newGossipInstanceWithGRPCWithExternalEndpoint(id, ports[id], grpcs[id], certs[id], secDialOpts[id],
				cs, externalEndpoint)
			peers = append(peers, peer)
			orgs2Peers[org] = append(orgs2Peers[org], peer)
			t.Log(endpoint, "id:", id, "externalEndpoint:", externalEndpoint)
			// The first peer of the org will be used as the anchor peer
			if j == 0 {
				anchorPeersByOrg[org] = api.AnchorPeer{
					Host: "127.0.0.1",
					Port: ports[id],
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
		msg := o.(protoext.ReceivedMessage).GetGossipMessage()
		identitiesPull := protoext.IsPullMsg(msg.GossipMessage) && protoext.GetPullMsgType(msg.GossipMessage) == proto.PullMsgType_IDENTITY_MSG
		return protoext.IsAliveMsg(msg.GossipMessage) || protoext.IsStateInfoMsg(msg.GossipMessage) || protoext.IsStateInfoSnapshot(msg.GossipMessage) || msg.GetMemRes() != nil || identitiesPull
	}
	// Listen to all peers membership messages and forward them to the inspection channel
	// where they will be inspected, and the test would fail if a confidentiality violation is found
	for _, p := range peers {
		wg.Add(1)
		_, msgs := p.Accept(msgSelector, true)
		peerNetMember := p.Node.selfNetworkMember()
		targetORg := string(cs.OrgByPeerIdentity(api.PeerIdentityType(peerNetMember.InternalEndpoint)))
		go func(targetOrg string, msgs <-chan protoext.ReceivedMessage) {
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
					p.JoinChan(joinChanMsgsByChan[ch], common.ChannelID(ch))
					p.UpdateLedgerHeight(1, common.ChannelID(ch))
					go func(p *gossipGRPC, ch string) {
						for i := 0; i < 5; i++ {
							time.Sleep(time.Second)
							p.UpdateLedgerHeight(1, common.ChannelID(ch))
						}
					}(p, ch)
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
				peerNetMember := p.Node.selfNetworkMember()
				membersCount := len(members)
				if membersCount < expMemberSize {
					return false
				}
				// Make sure no one knows too much
				require.True(t, membersCount <= expMemberSize, "%s knows too much (%d > %d) peers: %v",
					membersCount, expMemberSize, peerNetMember.PKIid, members)
			}
		}
		return true
	}

	waitUntilOrFail(t, assertMembership, "waiting for all instances to form unified membership view")
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
	if protoext.IsAliveMsg(msg) {
		return []string{string(sec.OrgByPeerIdentity(msg.GetAliveMsg().Membership.PkiId))}
	}

	orgs := map[string]struct{}{}

	if protoext.IsPullMsg(msg) {
		if protoext.IsDigestMsg(msg) || protoext.IsDataReq(msg) {
			var digests []string
			if protoext.IsDigestMsg(msg) {
				digests = util.BytesToStrings(msg.GetDataDig().Digests)
			} else {
				digests = util.BytesToStrings(msg.GetDataReq().Digests)
			}

			for _, dig := range digests {
				org := sec.OrgByPeerIdentity(api.PeerIdentityType(dig))
				orgs[string(org)] = struct{}{}
			}
		}

		if protoext.IsDataUpdate(msg) {
			for _, identityMsg := range msg.GetDataUpdate().Data {
				gMsg, _ := protoext.EnvelopeToGossipMessage(identityMsg)
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
			msg, _ := protoext.EnvelopeToGossipMessage(envp)
			orgs[string(sec.OrgByPeerIdentity(msg.GetAliveMsg().Membership.PkiId))] = struct{}{}
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
		if protoext.IsStateInfoMsg(msg.GossipMessage) || protoext.IsStateInfoSnapshot(msg.GossipMessage) {
			inspectStateInfoMsg(t, msg, peersWithExternalEndpoints)
			continue
		}
		// Else, it's a cross-organizational message.
		// Denote src organization as s and dst organization as d.
		// The total organizations of the message must be a subset of s U d.
		orgs := extractOrgsFromMsg(msg.GossipMessage, sec)
		s := []string{msg.src, msg.dst}
		require.True(t, isSubset(orgs, s), "%v isn't a subset of %v", orgs, s)

		// Ensure no one but B knows about D and vice versa
		if msg.dst == "D" {
			require.NotContains(t, "A", orgs)
			require.NotContains(t, "C", orgs)
		}

		if msg.dst == "A" || msg.dst == "C" {
			require.NotContains(t, "D", orgs)
		}

		// If this is an identity snapshot, make sure that only identities of peers
		// with external endpoints pass between the organizations.
		isIdentityPull := protoext.IsPullMsg(msg.GossipMessage) && protoext.GetPullMsgType(msg.GossipMessage) == proto.PullMsgType_IDENTITY_MSG
		if !(isIdentityPull && protoext.IsDataUpdate(msg.GossipMessage)) {
			continue
		}
		for _, envp := range msg.GetDataUpdate().Data {
			identityMsg, _ := protoext.EnvelopeToGossipMessage(envp)
			pkiID := identityMsg.GetPeerIdentity().PkiId
			_, hasExternalEndpoint := peersWithExternalEndpoints[string(pkiID)]
			require.True(t, hasExternalEndpoint,
				"Peer %s doesn't have an external endpoint but its identity was gossiped", string(pkiID))
		}
	}
}

func inspectStateInfoMsg(t *testing.T, m *msg, peersWithExternalEndpoints map[string]struct{}) {
	if protoext.IsStateInfoMsg(m.GossipMessage) {
		pkiID := m.GetStateInfo().PkiId
		_, hasExternalEndpoint := peersWithExternalEndpoints[string(pkiID)]
		require.True(t, hasExternalEndpoint, "peer %s has no external endpoint but crossed an org", string(pkiID))
		return
	}

	for _, envp := range m.GetStateSnapshot().Elements {
		msg, _ := protoext.EnvelopeToGossipMessage(envp)
		pkiID := msg.GetStateInfo().PkiId
		_, hasExternalEndpoint := peersWithExternalEndpoints[string(pkiID)]
		require.True(t, hasExternalEndpoint, "peer %s has no external endpoint but crossed an org", string(pkiID))
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
