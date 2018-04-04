/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorsement

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/hyperledger/fabric/common/chaincode"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	discovery2 "github.com/hyperledger/fabric/protos/discovery"
	"github.com/hyperledger/fabric/protos/gossip"
	"github.com/hyperledger/fabric/protos/msp"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestPeersForEndorsement(t *testing.T) {
	extractPeers := func(desc *discovery2.EndorsementDescriptor) map[string]struct{} {
		res := make(map[string]struct{})
		for _, endorsers := range desc.EndorsersByGroups {
			for _, p := range endorsers.Peers {
				res[string(p.Identity)] = struct{}{}
				assert.Equal(t, string(p.Identity), string(p.MembershipInfo.Payload))
				assert.Equal(t, string(p.Identity), string(p.StateInfo.Payload))
			}
		}
		return res
	}
	cc := "chaincode"
	mf := &metadataFetcher{}
	mf.On("Metadata").Return(&chaincode.Metadata{
		Name: cc, Version: "1.0",
	}).Times(4)
	g := &gossipMock{}
	pf := &policyFetcherMock{}
	ccWithMissingPolicy := "chaincodeWithMissingPolicy"
	channel := common.ChainID("test")
	alivePeers := peerSet{
		newPeer(0),
		newPeer(2),
		newPeer(4),
		newPeer(6),
		newPeer(8),
		newPeer(10),
		newPeer(12),
	}

	identities := alivePeers.toIdentitySet()

	chanPeers := peerSet{
		newPeer(0).withChaincode(cc, "1.0"),
		newPeer(3).withChaincode(cc, "1.0"),
		newPeer(6).withChaincode(cc, "1.0"),
		newPeer(9).withChaincode(cc, "1.0"),
		newPeer(12).withChaincode(cc, "1.0"),
	}
	g.On("PeersOfChannel").Return(chanPeers.toMembers()).Times(4)
	g.On("Peers").Return(alivePeers.toMembers())
	g.On("IdentityInfo").Return(identities).Times(6)

	// Scenario I: Policy isn't found
	pf.On("PolicyByChaincode", ccWithMissingPolicy).Return(nil).Once()
	analyzer := NewEndorsementAnalyzer(g, pf, &principalEvaluatorMock{}, mf)
	desc, err := analyzer.PeersForEndorsement(ccWithMissingPolicy, channel)
	assert.Nil(t, desc)
	assert.Equal(t, "policy not found", err.Error())

	// Scenario II: Policy is found but not enough peers to satisfy the policy.
	// The policy requires a signature from:
	// p1 and p6, or
	// p11
	pb := principalBuilder{}
	policy := pb.newSet().addPrincipal(&msp.MSPPrincipal{
		Principal: []byte("p1"),
	}).addPrincipal(&msp.MSPPrincipal{
		Principal: []byte("p6"),
	}).newSet().addPrincipal(&msp.MSPPrincipal{
		Principal: []byte("p11"),
	}).buildPolicy()

	analyzer = NewEndorsementAnalyzer(g, pf, policy.ToPrincipalEvaluator(), mf)
	pf.On("PolicyByChaincode", cc).Return(policy).Once()
	desc, err = analyzer.PeersForEndorsement(cc, channel)
	assert.Nil(t, desc)
	assert.Equal(t, err.Error(), "cannot satisfy any principal combination")

	// Scenario III: Policy is found and there are enough peers to satisfy
	// only 1 type of principal combination: p0 and p6.
	// However, the combination of a signature from p10 and p12
	// cannot be satisfied because p10 is not in the channel view but only in the alive view
	policy = pb.newSet().addPrincipal(&msp.MSPPrincipal{
		Principal: []byte("p0"),
	}).addPrincipal(&msp.MSPPrincipal{
		Principal: []byte("p6"),
	}).newSet().addPrincipal(&msp.MSPPrincipal{
		Principal: []byte("p10"),
	}).addPrincipal(&msp.MSPPrincipal{
		Principal: []byte("p12"),
	}).buildPolicy()

	analyzer = NewEndorsementAnalyzer(g, pf, policy.ToPrincipalEvaluator(), mf)
	pf.On("PolicyByChaincode", cc).Return(policy).Once()
	desc, err = analyzer.PeersForEndorsement(cc, channel)
	assert.NoError(t, err)
	assert.NotNil(t, desc)
	assert.Len(t, desc.Layouts, 1)
	assert.Len(t, desc.Layouts[0].QuantitiesByGroup, 2)
	assert.Equal(t, map[string]struct{}{
		"p0": {},
		"p6": {},
	}, extractPeers(desc))

	// Scenario IV: Policy is found and there are enough peers to satisfy
	// 2 principal combinations:
	// p0 and p6, or
	// p12 alone
	policy = pb.newSet().addPrincipal(&msp.MSPPrincipal{
		Principal: []byte("p0"),
	}).addPrincipal(&msp.MSPPrincipal{
		Principal: []byte("p6"),
	}).newSet().addPrincipal(&msp.MSPPrincipal{
		Principal: []byte("p12"),
	}).buildPolicy()

	analyzer = NewEndorsementAnalyzer(g, pf, policy.ToPrincipalEvaluator(), mf)
	pf.On("PolicyByChaincode", cc).Return(policy).Once()
	desc, err = analyzer.PeersForEndorsement(cc, channel)
	assert.NoError(t, err)
	assert.NotNil(t, desc)
	assert.Len(t, desc.Layouts, 2)
	assert.Len(t, desc.Layouts[0].QuantitiesByGroup, 2)
	assert.Len(t, desc.Layouts[1].QuantitiesByGroup, 1)
	assert.Equal(t, map[string]struct{}{
		"p0":  {},
		"p6":  {},
		"p12": {},
	}, extractPeers(desc))

	// Scenario V: Policy is found, but there are enough peers to satisfy policy combinations,
	// but all peers have the wrong version installed on them.
	mf.On("Metadata").Return(&chaincode.Metadata{
		Name: cc, Version: "1.1",
	}).Once()
	g.On("PeersOfChannel").Return(chanPeers.toMembers()).Once()
	pf.On("PolicyByChaincode", cc).Return(policy).Once()
	desc, err = analyzer.PeersForEndorsement(cc, channel)
	assert.Nil(t, desc)
	assert.Equal(t, err.Error(), "cannot satisfy any principal combination")

	// Scenario VI: Policy is found, there are enough peers to satisfy policy combinations,
	// but some peers have the wrong chaincode version, and some don't even have it installed.
	chanPeers[0].Properties.Chaincodes[0].Version = "0.6"
	chanPeers[4].Properties = nil
	g.On("PeersOfChannel").Return(chanPeers.toMembers()).Once()
	pf.On("PolicyByChaincode", cc).Return(policy).Once()
	mf.On("Metadata").Return(&chaincode.Metadata{
		Name: cc, Version: "1.0",
	}).Once()
	desc, err = analyzer.PeersForEndorsement(cc, channel)
	assert.Nil(t, desc)
	assert.Equal(t, err.Error(), "cannot satisfy any principal combination")

	// Scenario VII: Policy is found, there are enough peers to satisfy the policy,
	// but the chaincode metadata cannot be fetched from the ledger.
	g.On("PeersOfChannel").Return(chanPeers.toMembers()).Once()
	pf.On("PolicyByChaincode", cc).Return(policy).Once()
	mf.On("Metadata").Return(nil).Once()
	desc, err = analyzer.PeersForEndorsement(cc, channel)
	assert.Nil(t, desc)
	assert.Equal(t, err.Error(), "No metadata was found for chaincode chaincode in channel test")
}

type peerSet []*peerInfo

func (p peerSet) toMembers() discovery.Members {
	var members discovery.Members
	for _, peer := range p {
		members = append(members, peer.NetworkMember)
	}
	return members
}

func (p peerSet) toIdentitySet() api.PeerIdentitySet {
	var res api.PeerIdentitySet
	for _, peer := range p {
		res = append(res, api.PeerIdentityInfo{
			Identity: peer.identity,
			PKIId:    peer.pkiID,
		})
	}
	return res
}

type peerInfo struct {
	identity api.PeerIdentityType
	pkiID    common.PKIidType
	discovery.NetworkMember
}

func newPeer(i int) *peerInfo {
	p := fmt.Sprintf("p%d", i)
	return &peerInfo{
		pkiID:    common.PKIidType(p),
		identity: api.PeerIdentityType(p),
		NetworkMember: discovery.NetworkMember{
			PKIid:            common.PKIidType(p),
			Endpoint:         p,
			InternalEndpoint: p,
			Envelope: &gossip.Envelope{
				Payload: []byte(p),
			},
		},
	}
}

func (pi *peerInfo) withChaincode(name, version string) *peerInfo {
	if pi.Properties == nil {
		pi.Properties = &gossip.Properties{}
	}
	pi.Properties.Chaincodes = append(pi.Properties.Chaincodes, &gossip.Chaincode{
		Name:    name,
		Version: version,
	})
	return pi
}

type gossipMock struct {
	mock.Mock
}

func (g *gossipMock) IdentityInfo() api.PeerIdentitySet {
	return g.Called().Get(0).(api.PeerIdentitySet)
}

func (g *gossipMock) PeersOfChannel(_ common.ChainID) discovery.Members {
	members := g.Called().Get(0)
	return members.(discovery.Members)
}

func (g *gossipMock) Peers() discovery.Members {
	members := g.Called().Get(0)
	return members.(discovery.Members)
}

type policyFetcherMock struct {
	mock.Mock
}

func (pf *policyFetcherMock) PolicyByChaincode(channel string, chaincode string) policies.InquireablePolicy {
	arg := pf.Called(chaincode)
	if arg.Get(0) == nil {
		return nil
	}
	return arg.Get(0).(policies.InquireablePolicy)
}

type principalBuilder struct {
	ip inquireablePolicy
}

func (pb *principalBuilder) buildPolicy() inquireablePolicy {
	defer func() {
		pb.ip = nil
	}()
	return pb.ip
}

func (pb *principalBuilder) newSet() *principalBuilder {
	pb.ip = append(pb.ip, make(policies.PrincipalSet, 0))
	return pb
}

func (pb *principalBuilder) addPrincipal(principal *msp.MSPPrincipal) *principalBuilder {
	pb.ip[len(pb.ip)-1] = append(pb.ip[len(pb.ip)-1], principal)
	return pb
}

type inquireablePolicy []policies.PrincipalSet

func (ip inquireablePolicy) SatisfiedBy() []policies.PrincipalSet {
	return ip
}

func (ip inquireablePolicy) ToPrincipalEvaluator() *principalEvaluatorMock {
	return &principalEvaluatorMock{ip: ip}
}

type principalEvaluatorMock struct {
	ip []policies.PrincipalSet
}

func (pe *principalEvaluatorMock) SatisfiesPrincipal(channel string, identity []byte, principal *msp.MSPPrincipal) error {
	if bytes.Equal(identity, principal.Principal) {
		return nil
	}
	return errors.New("not satisfies")
}

type metadataFetcher struct {
	mock.Mock
}

func (mf *metadataFetcher) Metadata(channel string, cc string) *chaincode.Metadata {
	arg := mf.Called().Get(0)
	if arg == nil {
		return nil
	}
	return arg.(*chaincode.Metadata)
}
