/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorsement

import (
	"fmt"
	"testing"

	common2 "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/policydsl"

	"github.com/golang/protobuf/proto"
	discoveryprotos "github.com/hyperledger/fabric-protos-go/discovery"
	"github.com/hyperledger/fabric-protos-go/gossip"
	"github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/chaincode"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/common/policies/inquire"
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

var pkiID2MSPID = map[string]string{
	"p0":  "Org0MSP",
	"p1":  "Org1MSP",
	"p2":  "Org2MSP",
	"p3":  "Org3MSP",
	"p4":  "Org4MSP",
	"p5":  "Org5MSP",
	"p6":  "Org6MSP",
	"p7":  "Org7MSP",
	"p8":  "Org8MSP",
	"p9":  "Org9MSP",
	"p10": "Org10MSP",
	"p11": "Org11MSP",
	"p12": "Org12MSP",
	"p13": "Org13MSP",
	"p14": "Org14MSP",
	"p15": "Org15MSP",
}

func TestPeersForEndorsement(t *testing.T) {
	extractPeers := func(desc *discoveryprotos.EndorsementDescriptor) map[string]struct{} {
		res := map[string]struct{}{}
		for _, endorsers := range desc.EndorsersByGroups {
			for _, p := range endorsers.Peers {
				res[string(p.Identity)] = struct{}{}
				require.Equal(t, string(p.Identity), string(p.MembershipInfo.Payload))
				require.Equal(t, string(p.Identity), string(p.StateInfo.Payload))
			}
		}
		return res
	}
	cc := "chaincode"
	g := &gossipMock{}
	pf := &policyFetcherMock{}
	ccWithMissingPolicy := "chaincodeWithMissingPolicy"
	channel := common.ChannelID("test")
	alivePeers := peerSet{
		newPeer(0),
		newPeer(2),
		newPeer(4),
		newPeer(6),
		newPeer(8),
		newPeer(10),
		newPeer(11),
		newPeer(12),
	}

	identities := identitySet(pkiID2MSPID)

	chanPeers := peerSet{
		newPeer(0).withChaincode(cc, "1.0"),
		newPeer(3).withChaincode(cc, "1.0"),
		newPeer(6).withChaincode(cc, "1.0"),
		newPeer(9).withChaincode(cc, "1.0"),
		newPeer(11).withChaincode(cc, "1.0"),
		newPeer(12).withChaincode(cc, "1.0"),
	}
	g.On("Peers").Return(alivePeers.toMembers())
	g.On("IdentityInfo").Return(identities)

	// Scenario I: Policy isn't found
	t.Run("PolicyNotFound", func(t *testing.T) {
		pf.On("PoliciesByChaincode", ccWithMissingPolicy).Return(nil).Once()
		g.On("PeersOfChannel").Return(chanPeers.toMembers()).Once()
		mf := &metadataFetcher{}
		mf.On("Metadata").Return(&chaincode.Metadata{
			Name:    cc,
			Version: "1.0",
		}).Once()
		analyzer := NewEndorsementAnalyzer(g, pf, &principalEvaluatorMock{}, mf)
		desc, err := analyzer.PeersForEndorsement(channel, &peer.ChaincodeInterest{
			Chaincodes: []*peer.ChaincodeCall{
				{
					Name: ccWithMissingPolicy,
				},
			},
		})
		require.Nil(t, desc)
		require.Equal(t, "policy not found", err.Error())
	})

	t.Run("NotEnoughPeers", func(t *testing.T) {
		// Scenario II: Policy is found but not enough peers to satisfy the policy.
		// The policy requires a signature from:
		// p1 and p6, or
		// p11 x2 (twice), but we only have a single peer in the alive view for p11
		pb := principalBuilder{}
		policy := pb.newSet().addPrincipal(peerRole("p1")).addPrincipal(peerRole("p6")).
			newSet().addPrincipal(peerRole("p11")).addPrincipal(peerRole("p11")).buildPolicy()
		g.On("PeersOfChannel").Return(chanPeers.toMembers()).Once()
		mf := &metadataFetcher{}
		mf.On("Metadata").Return(&chaincode.Metadata{Name: cc, Version: "1.0"}).Once()
		analyzer := NewEndorsementAnalyzer(g, pf, &principalEvaluatorMock{}, mf)
		pf.On("PoliciesByChaincode", cc).Return(policy).Once()
		desc, err := analyzer.PeersForEndorsement(channel, &peer.ChaincodeInterest{
			Chaincodes: []*peer.ChaincodeCall{
				{
					Name: cc,
				},
			},
		})
		require.Nil(t, desc)
		require.Equal(t, err.Error(), "no peer combination can satisfy the endorsement policy")
	})

	t.Run("DisjointViews", func(t *testing.T) {
		pb := principalBuilder{}
		// Scenario III: Policy is found and there are enough peers to satisfy
		// only 1 type of principal combination: p0 and p6.
		// However, the combination of a signature from p10 and p12
		// cannot be satisfied because p10 is not in the channel view but only in the alive view
		policy := pb.newSet().addPrincipal(peerRole("p0")).addPrincipal(peerRole("p6")).
			newSet().addPrincipal(peerRole("p10")).addPrincipal(peerRole("p12")).buildPolicy()
		g.On("PeersOfChannel").Return(chanPeers.toMembers()).Once()
		mf := &metadataFetcher{}
		mf.On("Metadata").Return(&chaincode.Metadata{
			Name:    cc,
			Version: "1.0",
		}).Once()
		analyzer := NewEndorsementAnalyzer(g, pf, &principalEvaluatorMock{}, mf)
		pf.On("PoliciesByChaincode", cc).Return(policy).Once()
		desc, err := analyzer.PeersForEndorsement(channel, &peer.ChaincodeInterest{
			Chaincodes: []*peer.ChaincodeCall{
				{
					Name: cc,
				},
			},
		})
		require.NoError(t, err)
		require.NotNil(t, desc)
		require.Len(t, desc.Layouts, 1)
		require.Len(t, desc.Layouts[0].QuantitiesByGroup, 2)
		require.Equal(t, map[string]struct{}{
			peerIdentityString("p0"): {},
			peerIdentityString("p6"): {},
		}, extractPeers(desc))
	})

	t.Run("MultipleCombinations", func(t *testing.T) {
		// Scenario IV: Policy is found and there are enough peers to satisfy
		// 2 principal combinations:
		// p0 and p6, or
		// p12 alone
		pb := principalBuilder{}
		policy := pb.newSet().addPrincipal(peerRole("p0")).addPrincipal(peerRole("p6")).
			newSet().addPrincipal(peerRole("p12")).buildPolicy()
		g.On("PeersOfChannel").Return(chanPeers.toMembers()).Once()
		mf := &metadataFetcher{}
		mf.On("Metadata").Return(&chaincode.Metadata{
			Name:    cc,
			Version: "1.0",
		}).Once()
		analyzer := NewEndorsementAnalyzer(g, pf, &principalEvaluatorMock{}, mf)
		pf.On("PoliciesByChaincode", cc).Return(policy).Once()
		desc, err := analyzer.PeersForEndorsement(channel, &peer.ChaincodeInterest{
			Chaincodes: []*peer.ChaincodeCall{
				{
					Name: cc,
				},
			},
		})
		require.NoError(t, err)
		require.NotNil(t, desc)
		require.Len(t, desc.Layouts, 2)
		require.Len(t, desc.Layouts[0].QuantitiesByGroup, 2)
		require.Len(t, desc.Layouts[1].QuantitiesByGroup, 1)
		require.Equal(t, map[string]struct{}{
			peerIdentityString("p0"):  {},
			peerIdentityString("p6"):  {},
			peerIdentityString("p12"): {},
		}, extractPeers(desc))
	})

	t.Run("WrongVersionInstalled", func(t *testing.T) {
		// Scenario V: Policy is found, and there are enough peers to satisfy policy combinations,
		// but all peers have the wrong version installed on them.
		mf := &metadataFetcher{}
		mf.On("Metadata").Return(&chaincode.Metadata{
			Name:    cc,
			Version: "1.1",
		}).Once()
		pb := principalBuilder{}
		policy := pb.newSet().addPrincipal(peerRole("p0")).addPrincipal(peerRole("p6")).
			newSet().addPrincipal(peerRole("p12")).buildPolicy()
		g.On("PeersOfChannel").Return(chanPeers.toMembers()).Once()
		pf.On("PoliciesByChaincode", cc).Return(policy).Once()
		analyzer := NewEndorsementAnalyzer(g, pf, &principalEvaluatorMock{}, mf)
		desc, err := analyzer.PeersForEndorsement(channel, &peer.ChaincodeInterest{
			Chaincodes: []*peer.ChaincodeCall{
				{
					Name: cc,
				},
			},
		})
		require.Nil(t, desc)
		require.Equal(t, "required chaincodes are not installed on sufficient peers", err.Error())

		// Scenario VI: Policy is found, there are enough peers to satisfy policy combinations,
		// but some peers have the wrong chaincode version, and some don't even have it installed.
		chanPeers := peerSet{
			newPeer(0).withChaincode(cc, "0.6"),
			newPeer(3).withChaincode(cc, "1.0"),
			newPeer(6).withChaincode(cc, "1.0"),
			newPeer(9).withChaincode(cc, "1.0"),
			newPeer(12),
		}
		g.On("PeersOfChannel").Return(chanPeers.toMembers()).Once()
		pf.On("PoliciesByChaincode", cc).Return(policy).Once()
		mf.On("Metadata").Return(&chaincode.Metadata{
			Name:    cc,
			Version: "1.0",
		}).Once()
		desc, err = analyzer.PeersForEndorsement(channel, &peer.ChaincodeInterest{
			Chaincodes: []*peer.ChaincodeCall{
				{
					Name: cc,
				},
			},
		})
		require.Nil(t, desc)
		require.Equal(t, "required chaincodes are not installed on sufficient peers", err.Error())
	})

	t.Run("NoChaincodeMetadataFromLedger", func(t *testing.T) {
		// Scenario VII: Policy is found, there are enough peers to satisfy the policy,
		// but the chaincode metadata cannot be fetched from the ledger.
		g.On("PeersOfChannel").Return(chanPeers.toMembers()).Once()
		mf := &metadataFetcher{}
		mf.On("Metadata").Return(nil).Once()
		analyzer := NewEndorsementAnalyzer(g, pf, &principalEvaluatorMock{}, mf)
		desc, err := analyzer.PeersForEndorsement(channel, &peer.ChaincodeInterest{
			Chaincodes: []*peer.ChaincodeCall{
				{
					Name: cc,
				},
			},
		})
		require.Nil(t, desc)
		require.Equal(t, "No metadata was found for chaincode chaincode in channel test", err.Error())
	})

	t.Run("Collections", func(t *testing.T) {
		// Scenario VIII: Policy is found and there are enough peers to satisfy
		// 2 principal combinations: p0 and p6, or p12 alone.
		// However, the query contains a collection which has a policy that permits only p0 and p12,
		// and thus - the combination of p0 and p6 is filtered out and we're left with p12 only.
		collectionOrgs := []*msp.MSPPrincipal{
			peerRole("p0"),
			peerRole("p12"),
		}
		col2principals := map[string][]*msp.MSPPrincipal{
			"collection": collectionOrgs,
		}
		mf := &metadataFetcher{}
		mf.On("Metadata").Return(&chaincode.Metadata{
			Name:              cc,
			Version:           "1.0",
			CollectionsConfig: buildCollectionConfig(col2principals),
		}).Once()
		pb := principalBuilder{}
		policy := pb.newSet().addPrincipal(peerRole("p0")).
			addPrincipal(peerRole("p6")).newSet().
			addPrincipal(peerRole("p12")).buildPolicy()
		g.On("PeersOfChannel").Return(chanPeers.toMembers()).Once()
		pf.On("PoliciesByChaincode", cc).Return(policy).Once()
		analyzer := NewEndorsementAnalyzer(g, pf, &principalEvaluatorMock{}, mf)
		desc, err := analyzer.PeersForEndorsement(channel, &peer.ChaincodeInterest{
			Chaincodes: []*peer.ChaincodeCall{
				{
					Name:            cc,
					CollectionNames: []string{"collection"},
				},
			},
		})
		require.NoError(t, err)
		require.NotNil(t, desc)
		require.Len(t, desc.Layouts, 1)
		require.Len(t, desc.Layouts[0].QuantitiesByGroup, 1)
		require.Equal(t, map[string]struct{}{
			peerIdentityString("p12"): {},
		}, extractPeers(desc))
	})

	t.Run("Chaincode2Chaincode I", func(t *testing.T) {
		// Scenario IX: A chaincode-to-chaincode query is made.
		// Total organizations are 0, 2, 4, 6, 10, 12
		// and the endorsement policies of the chaincodes are as follows:
		// cc1: OR(AND(0, 2), AND(6, 10))
		// cc2: AND(6, 10, 12)
		// cc3: AND(4, 12)
		// Therefore, the result should be: 4, 6, 10, 12

		chanPeers := peerSet{}
		for _, id := range []int{0, 2, 4, 6, 10, 12} {
			peer := newPeer(id).withChaincode("cc1", "1.0").withChaincode("cc2", "1.0").withChaincode("cc3", "1.0")
			chanPeers = append(chanPeers, peer)
		}

		g.On("PeersOfChannel").Return(chanPeers.toMembers()).Once()

		mf := &metadataFetcher{}
		mf.On("Metadata").Return(&chaincode.Metadata{
			Name:    "cc1",
			Version: "1.0",
		}).Once()
		mf.On("Metadata").Return(&chaincode.Metadata{
			Name:    "cc2",
			Version: "1.0",
		}).Once()
		mf.On("Metadata").Return(&chaincode.Metadata{
			Name:    "cc3",
			Version: "1.0",
		}).Once()

		pb := principalBuilder{}
		cc1policy := pb.newSet().addPrincipal(peerRole("p0")).addPrincipal(peerRole("p2")).
			newSet().addPrincipal(peerRole("p6")).addPrincipal(peerRole("p10")).buildPolicy()

		pf.On("PoliciesByChaincode", "cc1").Return(cc1policy).Once()

		cc2policy := pb.newSet().addPrincipal(peerRole("p6")).
			addPrincipal(peerRole("p10")).addPrincipal(peerRole("p12")).buildPolicy()
		pf.On("PoliciesByChaincode", "cc2").Return(cc2policy).Once()

		cc3policy := pb.newSet().addPrincipal(peerRole("p4")).
			addPrincipal(peerRole("p12")).buildPolicy()
		pf.On("PoliciesByChaincode", "cc3").Return(cc3policy).Once()

		analyzer := NewEndorsementAnalyzer(g, pf, &principalEvaluatorMock{}, mf)
		desc, err := analyzer.PeersForEndorsement(channel, &peer.ChaincodeInterest{
			Chaincodes: []*peer.ChaincodeCall{
				{
					Name: "cc1",
				},
				{
					Name: "cc2",
				},
				{
					Name: "cc3",
				},
			},
		})
		require.NoError(t, err)
		require.NotNil(t, desc)
		require.Len(t, desc.Layouts, 1)
		require.Len(t, desc.Layouts[0].QuantitiesByGroup, 4)
		require.Equal(t, map[string]struct{}{
			peerIdentityString("p4"):  {},
			peerIdentityString("p6"):  {},
			peerIdentityString("p10"): {},
			peerIdentityString("p12"): {},
		}, extractPeers(desc))
	})

	t.Run("Chaincode2Chaincode II", func(t *testing.T) {
		// Scenario X: A chaincode-to-chaincode query is made.
		// and the endorsement policies of the chaincodes are as follows:
		// cc1: OR(0, 1)
		// cc2: AND(0, 1)
		// Therefore, the result should be: (0, 1)

		cc1 := "cc1"
		cc2 := "cc2"
		chanPeers := peerSet{
			newPeer(0).withChaincode(cc1, "1.0").withChaincode(cc2, "1.0"),
			newPeer(1).withChaincode(cc1, "1.0").withChaincode(cc2, "1.0"),
		}.toMembers()

		alivePeers := peerSet{
			newPeer(0),
			newPeer(1),
		}.toMembers()

		g := &gossipMock{}
		g.On("Peers").Return(alivePeers)
		g.On("IdentityInfo").Return(identities)
		g.On("PeersOfChannel").Return(chanPeers).Once()

		mf := &metadataFetcher{}
		mf.On("Metadata").Return(&chaincode.Metadata{
			Name:    "cc1",
			Version: "1.0",
		})
		mf.On("Metadata").Return(&chaincode.Metadata{
			Name:    "cc2",
			Version: "1.0",
		})

		pb := principalBuilder{}
		cc1policy := pb.newSet().addPrincipal(peerRole("p0")).
			newSet().addPrincipal(peerRole("p1")).buildPolicy()
		pf.On("PoliciesByChaincode", "cc1").Return(cc1policy).Once()

		cc2policy := pb.newSet().addPrincipal(peerRole("p0")).
			addPrincipal(peerRole("p1")).buildPolicy()
		pf.On("PoliciesByChaincode", "cc2").Return(cc2policy).Once()

		analyzer := NewEndorsementAnalyzer(g, pf, &principalEvaluatorMock{}, mf)
		desc, err := analyzer.PeersForEndorsement(channel, &peer.ChaincodeInterest{
			Chaincodes: []*peer.ChaincodeCall{
				{
					Name: "cc1",
				},
				{
					Name: "cc2",
				},
			},
		})
		require.NoError(t, err)
		require.NotNil(t, desc)
		require.Len(t, desc.Layouts, 1)
		require.Len(t, desc.Layouts[0].QuantitiesByGroup, 2)
		require.Equal(t, map[string]struct{}{
			peerIdentityString("p0"): {},
			peerIdentityString("p1"): {},
		}, extractPeers(desc))
	})

	t.Run("Collection specific EP", func(t *testing.T) {
		// Scenario XI: Policy is found and there are enough peers to satisfy
		// 2 principal combinations: p0 and p6, or p12 alone.
		// The collection has p0, p6, and p12 in it.
		// The chaincode EP is (p0 and p6) or p12.
		// However, the chaincode has a collection level EP that requires p6 and p12.
		// Thus, the only combination that can satisfy would be p6 and p12.
		collectionOrgs := []*msp.MSPPrincipal{
			peerRole("p0"),
			peerRole("p6"),
			peerRole("p12"),
		}
		col2principals := map[string][]*msp.MSPPrincipal{
			"collection": collectionOrgs,
		}

		mf := &metadataFetcher{}
		mf.On("Metadata").Return(&chaincode.Metadata{
			Name:              cc,
			Version:           "1.0",
			CollectionsConfig: buildCollectionConfig(col2principals),
		}).Once()
		pb := principalBuilder{}
		chaincodeEP := pb.newSet().addPrincipal(peerRole("p0")).
			addPrincipal(peerRole("p6")).newSet().
			addPrincipal(peerRole("p12")).buildPolicy()
		collectionEP := pb.newSet().addPrincipal(peerRole("p6")).
			addPrincipal(peerRole("p12")).buildPolicy()
		g.On("PeersOfChannel").Return(chanPeers.toMembers()).Once()
		pf := &policyFetcherMock{}
		pf.On("PoliciesByChaincode", cc).Return([]policies.InquireablePolicy{chaincodeEP, collectionEP}).Once()
		analyzer := NewEndorsementAnalyzer(g, pf, &principalEvaluatorMock{}, mf)
		desc, err := analyzer.PeersForEndorsement(channel, &peer.ChaincodeInterest{
			Chaincodes: []*peer.ChaincodeCall{
				{
					Name:            cc,
					CollectionNames: []string{"collection"},
				},
			},
		})
		require.NoError(t, err)
		require.NotNil(t, desc)
		require.Len(t, desc.Layouts, 1)
		require.Len(t, desc.Layouts[0].QuantitiesByGroup, 2)
		require.Equal(t, map[string]struct{}{
			peerIdentityString("p6"):  {},
			peerIdentityString("p12"): {},
		}, extractPeers(desc))
	})

	t.Run("Private data blind write", func(t *testing.T) {
		// Scenario XII: The collection has only p0 in it
		// The chaincode EP is p6 or p0.
		// The collection endorsement policy is p0 and p6.
		// However p6 is not in the collection at all (only p0),
		// so it doesn't have the pre-images.
		// To that end, the client indicates that it's a blind write
		// by turning on the "noPrivateRead" field in the request.
		// This might seem like a pathological case, but it's
		// effective because it is in the intersection of
		// several use cases.

		collectionOrgs := []*msp.MSPPrincipal{
			peerRole("p0"),
		}
		col2principals := map[string][]*msp.MSPPrincipal{
			"collection": collectionOrgs,
		}

		mf := &metadataFetcher{}
		mf.On("Metadata").Return(&chaincode.Metadata{
			Name:              cc,
			Version:           "1.0",
			CollectionsConfig: buildCollectionConfig(col2principals),
		}).Once()
		pb := principalBuilder{}
		chaincodeEP := pb.newSet().addPrincipal(peerRole("p0")).newSet(). // p0 or p6
											addPrincipal(peerRole("p6")).buildPolicy()
		collectionEP := pb.newSet().addPrincipal(peerRole("p0")). // p0 and p6
										addPrincipal(peerRole("p6")).buildPolicy()
		g.On("PeersOfChannel").Return(chanPeers.toMembers()).Once()
		pf := &policyFetcherMock{}
		pf.On("PoliciesByChaincode", cc).Return([]policies.InquireablePolicy{chaincodeEP, collectionEP}).Once()
		analyzer := NewEndorsementAnalyzer(g, pf, &principalEvaluatorMock{}, mf)
		desc, err := analyzer.PeersForEndorsement(channel, &peer.ChaincodeInterest{
			Chaincodes: []*peer.ChaincodeCall{
				{
					Name:            cc,
					CollectionNames: []string{"collection"},
					NoPrivateReads:  true, // This means a blind write
				},
			},
		})
		require.NoError(t, err)
		require.NotNil(t, desc)
		require.Len(t, desc.Layouts, 1)
		require.Len(t, desc.Layouts[0].QuantitiesByGroup, 2)
		require.Equal(t, map[string]struct{}{
			peerIdentityString("p0"): {},
			peerIdentityString("p6"): {},
		}, extractPeers(desc))
	})

	t.Run("Chaincode call with state based endorsement policy I", func(t *testing.T) {
		// Scenario XIII: A chaincode call with a state based endorsement policy
		// Total organizations are 0, 2, 4, 6, 10, 12
		// and the endorsement policies of the chaincode is:
		// cc1: OR(AND(0, 2), AND(6, 10))
		// However the chaincode call is accompanied with a hint
		// for a state based endorsement policy for organization 10
		// Therefore, the result should be: 6, 10

		chanPeers := peerSet{}
		for _, id := range []int{0, 2, 4, 6, 10, 12} {
			peer := newPeer(id).withChaincode("cc1", "1.0")
			chanPeers = append(chanPeers, peer)
		}

		g.On("PeersOfChannel").Return(chanPeers.toMembers()).Once()

		mf := &metadataFetcher{}
		mf.On("Metadata").Return(&chaincode.Metadata{
			Name:    "cc1",
			Version: "1.0",
		}).Once()

		pb := principalBuilder{}
		cc1policy := pb.newSet().addPrincipal(peerRole("p0")).addPrincipal(peerRole("p2")).
			newSet().addPrincipal(peerRole("p6")).addPrincipal(peerRole("p10")).buildPolicy()

		pf.On("PoliciesByChaincode", "cc1").Return(cc1policy).Once()

		analyzer := NewEndorsementAnalyzer(g, pf, &principalEvaluatorMock{}, mf)
		desc, err := analyzer.PeersForEndorsement(channel, &peer.ChaincodeInterest{
			Chaincodes: []*peer.ChaincodeCall{
				{
					Name: "cc1",
					KeyPolicies: []*common2.SignaturePolicyEnvelope{
						{
							Identities: []*msp.MSPPrincipal{peerRole("p10")},
							Rule:       policydsl.SignedBy(0),
						},
					},
				},
			},
		})
		require.NoError(t, err)
		require.NotNil(t, desc)
		require.Len(t, desc.Layouts, 1)
		require.Len(t, desc.Layouts[0].QuantitiesByGroup, 2)
		require.Equal(t, map[string]struct{}{
			peerIdentityString("p6"):  {},
			peerIdentityString("p10"): {},
		}, extractPeers(desc))
	})

	t.Run("Chaincode call with state based endorsement policy II", func(t *testing.T) {
		// Scenario XIV: A chaincode call with a state based endorsement policy
		// Total organizations are 0, 2, 4, 6, 10, 12
		// and the endorsement policies of the chaincode is:
		// cc1: OR(AND(0, 2), AND(6, 10))
		// However the chaincode call is accompanied with a hint
		// for a state based endorsement policy for organization 12
		// Therefore, the result should be: {0, 2, 12} or {6, 10, 12}

		chanPeers := peerSet{}
		for _, id := range []int{0, 2, 4, 6, 10, 12} {
			peer := newPeer(id).withChaincode("cc1", "1.0")
			chanPeers = append(chanPeers, peer)
		}

		g.On("PeersOfChannel").Return(chanPeers.toMembers()).Once()

		mf := &metadataFetcher{}
		mf.On("Metadata").Return(&chaincode.Metadata{
			Name:    "cc1",
			Version: "1.0",
		}).Once()

		pb := principalBuilder{}
		cc1policy := pb.newSet().addPrincipal(peerRole("p0")).addPrincipal(peerRole("p2")).
			newSet().addPrincipal(peerRole("p6")).addPrincipal(peerRole("p10")).buildPolicy()

		pf.On("PoliciesByChaincode", "cc1").Return(cc1policy).Once()

		analyzer := NewEndorsementAnalyzer(g, pf, &principalEvaluatorMock{}, mf)
		desc, err := analyzer.PeersForEndorsement(channel, &peer.ChaincodeInterest{
			Chaincodes: []*peer.ChaincodeCall{
				{
					Name: "cc1",
					KeyPolicies: []*common2.SignaturePolicyEnvelope{
						{
							Identities: []*msp.MSPPrincipal{peerRole("p12")},
							Rule:       policydsl.SignedBy(0),
						},
					},
				},
			},
		})
		require.NoError(t, err)
		require.NotNil(t, desc)
		require.Len(t, desc.Layouts, 2)
		require.Len(t, desc.Layouts[0].QuantitiesByGroup, 3)
		require.Len(t, desc.Layouts[1].QuantitiesByGroup, 3)
		require.Equal(t, map[string]struct{}{
			peerIdentityString("p0"):  {},
			peerIdentityString("p2"):  {},
			peerIdentityString("p6"):  {},
			peerIdentityString("p10"): {},
			peerIdentityString("p12"): {},
		}, extractPeers(desc))
		// Find ID of org 12

		// Ensure org 12 (and no other org) is found in both layouts
		var intersectionSize int
		for g1 := range desc.Layouts[0].QuantitiesByGroup {
			for g2 := range desc.Layouts[1].QuantitiesByGroup {
				if g1 == g2 {
					require.Equal(t, intersectionSize, 0)
					intersectionSize++
					require.Equal(t, peerIdentityString("p12"), string(desc.EndorsersByGroups[g1].Peers[0].Identity))
				}
			}
		}
	})

	t.Run("Chaincode call with state based endorsement policy III", func(t *testing.T) {
		// Scenario XV: A chaincode call with a state based endorsement policy
		// Total organizations are 0, 2, 4, 6, 10, 12
		// and the endorsement policies of the chaincode is:
		// cc1: OR(AND(0, 2), AND(6, 10))
		// However the chaincode call is accompanied with a hint
		// for a state based endorsement policy for both organizations 2 and 6
		// Therefore, the result should be: {0, 2, 6} or {2, 6, 10}

		chanPeers := peerSet{}
		for _, id := range []int{0, 2, 4, 6, 10, 12} {
			peer := newPeer(id).withChaincode("cc1", "1.0")
			chanPeers = append(chanPeers, peer)
		}

		g.On("PeersOfChannel").Return(chanPeers.toMembers()).Once()

		mf := &metadataFetcher{}
		mf.On("Metadata").Return(&chaincode.Metadata{
			Name:    "cc1",
			Version: "1.0",
		}).Once()

		pb := principalBuilder{}
		cc1policy := pb.newSet().addPrincipal(peerRole("p0")).addPrincipal(peerRole("p2")).
			newSet().addPrincipal(peerRole("p6")).addPrincipal(peerRole("p10")).buildPolicy()

		pf.On("PoliciesByChaincode", "cc1").Return(cc1policy).Once()

		analyzer := NewEndorsementAnalyzer(g, pf, &principalEvaluatorMock{}, mf)
		desc, err := analyzer.PeersForEndorsement(channel, &peer.ChaincodeInterest{
			Chaincodes: []*peer.ChaincodeCall{
				{
					Name: "cc1",
					KeyPolicies: []*common2.SignaturePolicyEnvelope{
						{
							Identities: []*msp.MSPPrincipal{peerRole("p2")},
							Rule:       policydsl.SignedBy(0),
						},
						{
							Identities: []*msp.MSPPrincipal{peerRole("p6")},
							Rule:       policydsl.SignedBy(0),
						},
					},
				},
			},
		})
		require.NoError(t, err)
		require.NotNil(t, desc)
		require.Len(t, desc.Layouts, 2)
		require.Len(t, desc.Layouts[0].QuantitiesByGroup, 3)
		require.Equal(t, map[string]struct{}{
			peerIdentityString("p0"):  {},
			peerIdentityString("p2"):  {},
			peerIdentityString("p6"):  {},
			peerIdentityString("p10"): {},
		}, extractPeers(desc))

		// Ensure orgs 2, 6 are found in both layouts
		intersection := make(map[string]struct{})
		for g1 := range desc.Layouts[0].QuantitiesByGroup {
			for g2 := range desc.Layouts[1].QuantitiesByGroup {
				if g1 == g2 {
					intersection[string(desc.EndorsersByGroups[g1].Peers[0].Identity)] = struct{}{}
				}
			}
		}

		require.Equal(t, map[string]struct{}{
			peerIdentityString("p2"): {},
			peerIdentityString("p6"): {},
		}, intersection)
	})

	t.Run("Chaincode call with DisregardNamespacePolicy set but no key policies or collection policies present", func(t *testing.T) {
		// Scenario XVI: A chaincode call with DisregardNamespacePolicy set
		// Total organizations are 0, 2, 4, 6, 10, 12
		// and there is a collection specified by the client but no collection policies exist
		// We expect an error because since DisregardNamespacePolicy is specified, and no collection policies are defined,
		// there is not a single endorsement policy to compute.

		chanPeers := peerSet{}
		for _, id := range []int{0, 2, 4, 6, 10, 12} {
			peer := newPeer(id).withChaincode("cc1", "1.0")
			chanPeers = append(chanPeers, peer)
		}

		g.On("PeersOfChannel").Return(chanPeers.toMembers()).Once()

		mf := &metadataFetcher{}
		mf.On("Metadata").Return(&chaincode.Metadata{
			Name:    "cc1",
			Version: "1.0",
		}).Once()

		pb := principalBuilder{}
		cc1policy := pb.newSet().addPrincipal(peerRole("p0")).addPrincipal(peerRole("p2")).
			newSet().addPrincipal(peerRole("p6")).addPrincipal(peerRole("p10")).buildPolicy()

		pf.On("PoliciesByChaincode", "cc1").Return(cc1policy).Once()

		analyzer := NewEndorsementAnalyzer(g, pf, &principalEvaluatorMock{}, mf)
		desc, err := analyzer.PeersForEndorsement(channel, &peer.ChaincodeInterest{
			Chaincodes: []*peer.ChaincodeCall{
				{
					Name:                     "cc1",
					DisregardNamespacePolicy: true,
				},
			},
		})
		require.EqualError(t, err, "requested to disregard chaincode cc1's policy but key and collection policies are missing,"+
			" either disable DisregardNamespacePolicy or specify at least one key policy or at least one collection policy")
		require.Nil(t, desc)
	})

	t.Run("Chaincode call with state based endorsement policy and no chaincode namespace policy", func(t *testing.T) {
		// Scenario XVII: A chaincode call with a state based endorsement policy and DisregardNamespacePolicy set
		// Total organizations are 0, 2, 4, 6, 10, 12
		// and the endorsement policies of the chaincode is:
		// cc1: OR(AND(0, 2), AND(6, 10))
		// However the chaincode call is accompanied with a hint
		// for a state based endorsement policy for both organizations 2 and 6
		// Therefore, the result should be: {2, 6}

		chanPeers := peerSet{}
		for _, id := range []int{0, 2, 4, 6, 10, 12} {
			peer := newPeer(id).withChaincode("cc1", "1.0")
			chanPeers = append(chanPeers, peer)
		}

		g.On("PeersOfChannel").Return(chanPeers.toMembers()).Once()

		mf := &metadataFetcher{}
		mf.On("Metadata").Return(&chaincode.Metadata{
			Name:    "cc1",
			Version: "1.0",
		}).Once()

		pb := principalBuilder{}
		cc1policy := pb.newSet().addPrincipal(peerRole("p0")).addPrincipal(peerRole("p2")).
			newSet().addPrincipal(peerRole("p6")).addPrincipal(peerRole("p10")).buildPolicy()

		pf.On("PoliciesByChaincode", "cc1").Return(cc1policy).Once()

		analyzer := NewEndorsementAnalyzer(g, pf, &principalEvaluatorMock{}, mf)
		desc, err := analyzer.PeersForEndorsement(channel, &peer.ChaincodeInterest{
			Chaincodes: []*peer.ChaincodeCall{
				{
					Name:                     "cc1",
					DisregardNamespacePolicy: true,
					KeyPolicies: []*common2.SignaturePolicyEnvelope{
						{
							Identities: []*msp.MSPPrincipal{peerRole("p2")},
							Rule:       policydsl.SignedBy(0),
						},
						{
							Identities: []*msp.MSPPrincipal{peerRole("p6")},
							Rule:       policydsl.SignedBy(0),
						},
					},
				},
			},
		})
		require.NoError(t, err)
		require.NotNil(t, desc)
		require.Len(t, desc.Layouts, 1)
		require.Len(t, desc.Layouts[0].QuantitiesByGroup, 2)
		require.Equal(t, map[string]struct{}{
			peerIdentityString("p2"): {},
			peerIdentityString("p6"): {},
		}, extractPeers(desc))
	})

	t.Run("Chaincode call with collection endorsement policy and no namespace endorsement policy", func(t *testing.T) {
		// Scenario XVIII: A chaincode call with collection endorsement policy and DisregardNamespacePolicy set
		// The chaincode EP is OR(AND(0, 2), AND(6, 10))
		// The collection endorsement policy is p0 and p2.
		// Additionally, the client sets DisregardNamespacePolicy which makes
		// discovery only use the collection policy and not the namespace policy.

		chanPeers := peerSet{}
		for _, id := range []int{0, 2, 4, 6, 10, 12} {
			peer := newPeer(id).withChaincode("cc1", "1.0")
			chanPeers = append(chanPeers, peer)
		}

		g.On("PeersOfChannel").Return(chanPeers.toMembers()).Once()

		collectionOrgs := []*msp.MSPPrincipal{
			peerRole("p0"),
			peerRole("p2"),
		}
		col2principals := map[string][]*msp.MSPPrincipal{
			"collection": collectionOrgs,
		}
		mf := &metadataFetcher{}
		mf.On("Metadata").Return(&chaincode.Metadata{
			Name:              "cc1",
			Version:           "1.0",
			CollectionsConfig: buildCollectionConfig(col2principals),
		}).Once()

		pb := principalBuilder{}
		cc1policy := pb.newSet().addPrincipal(peerRole("p0")).addPrincipal(peerRole("p2")).
			newSet().addPrincipal(peerRole("p6")).addPrincipal(peerRole("p10")).buildPolicy()

		collectionEP := pb.newSet().addPrincipal(peerRole("p0")). // p0 and p6
										addPrincipal(peerRole("p2")).buildPolicy()

		pf.On("PoliciesByChaincode", "cc1").Return([]policies.InquireablePolicy{cc1policy, collectionEP}).Once()

		analyzer := NewEndorsementAnalyzer(g, pf, &principalEvaluatorMock{}, mf)
		desc, err := analyzer.PeersForEndorsement(channel, &peer.ChaincodeInterest{
			Chaincodes: []*peer.ChaincodeCall{
				{
					Name:                     "cc1",
					DisregardNamespacePolicy: true,
					CollectionNames:          []string{"collection"},
				},
			},
		})
		require.NoError(t, err)
		require.NotNil(t, desc)
		require.Len(t, desc.Layouts, 1)
		require.Len(t, desc.Layouts[0].QuantitiesByGroup, 2)
		require.Equal(t, map[string]struct{}{
			peerIdentityString("p0"): {},
			peerIdentityString("p2"): {},
		}, extractPeers(desc))
	})

	t.Run("Identity based endorsement policy doesn't interfere with discovery", func(t *testing.T) {
		// Scenario XIX: Policy is based on either identities or on organizations
		// 2 principal combinations:
		// p0 and p6 (organizations), or p7 (identity)

		pb := principalBuilder{}
		policy := pb.newSet().addPrincipal(peerRole("p0")).addPrincipal(peerRole("p6")).
			newSet().addPrincipal(identity("p7")).buildPolicy()
		g.On("PeersOfChannel").Return(chanPeers.toMembers()).Once()
		mf := &metadataFetcher{}
		mf.On("Metadata").Return(&chaincode.Metadata{
			Name:    cc,
			Version: "1.0",
		}).Once()
		analyzer := NewEndorsementAnalyzer(g, pf, &principalEvaluatorMock{}, mf)

		pf.On("PoliciesByChaincode", cc).Return(policy).Once()
		desc, err := analyzer.PeersForEndorsement(channel, &peer.ChaincodeInterest{
			Chaincodes: []*peer.ChaincodeCall{
				{
					Name: cc,
				},
			},
		})
		require.NoError(t, err)
		require.NotNil(t, desc)
		require.Len(t, desc.Layouts, 1)
		require.Len(t, desc.Layouts[0].QuantitiesByGroup, 2)
		require.Equal(t, map[string]struct{}{
			peerIdentityString("p0"): {},
			peerIdentityString("p6"): {},
		}, extractPeers(desc))
	})
}

func TestPeersAuthorizedByCriteria(t *testing.T) {
	cc1 := "cc1"
	cc2 := "cc2"
	members := peerSet{
		newPeer(0).withChaincode(cc1, "1.0"),
		newPeer(3).withChaincode(cc1, "1.0"),
		newPeer(6).withChaincode(cc1, "1.0"),
		newPeer(9).withChaincode(cc1, "1.0"),
		newPeer(12).withChaincode(cc1, "1.0"),
	}.toMembers()

	members2 := append(discovery.Members{}, members...)
	members2 = append(members2, peerSet{newPeer(13).withChaincode(cc1, "1.1").withChaincode(cc2, "1.0")}.toMembers()...)
	members2 = append(members2, peerSet{newPeer(14).withChaincode(cc1, "1.1")}.toMembers()...)
	members2 = append(members2, peerSet{newPeer(15).withChaincode(cc2, "1.0")}.toMembers()...)

	alivePeers := peerSet{
		newPeer(0),
		newPeer(2),
		newPeer(4),
		newPeer(6),
		newPeer(8),
		newPeer(10),
		newPeer(11),
		newPeer(12),
		newPeer(13),
		newPeer(14),
		newPeer(15),
	}.toMembers()

	identities := identitySet(pkiID2MSPID)

	for _, tst := range []struct {
		name                 string
		arguments            *peer.ChaincodeInterest
		totalExistingMembers discovery.Members
		metadata             []*chaincode.Metadata
		expected             discovery.Members
	}{
		{
			name:                 "Nil interest",
			arguments:            nil,
			totalExistingMembers: members,
			expected:             members,
		},
		{
			name:                 "Empty interest invocation chain",
			arguments:            &peer.ChaincodeInterest{},
			totalExistingMembers: members,
			expected:             members,
		},
		{
			name: "Chaincodes only installed on some peers",
			arguments: &peer.ChaincodeInterest{
				Chaincodes: []*peer.ChaincodeCall{
					{Name: cc1},
					{Name: cc2},
				},
			},
			totalExistingMembers: members2,
			metadata: []*chaincode.Metadata{
				{
					Name:    "cc1",
					Version: "1.1",
				},
				{
					Name:    "cc2",
					Version: "1.0",
				},
			},
			expected: peerSet{newPeer(13).withChaincode(cc1, "1.1").withChaincode(cc2, "1.0")}.toMembers(),
		},
		{
			name: "Only some peers authorized by collection",
			arguments: &peer.ChaincodeInterest{
				Chaincodes: []*peer.ChaincodeCall{
					{Name: cc1, CollectionNames: []string{"collection"}},
				},
			},
			totalExistingMembers: members,
			metadata: []*chaincode.Metadata{
				{
					Name:    cc1,
					Version: "1.0",
					CollectionsConfig: buildCollectionConfig(map[string][]*msp.MSPPrincipal{
						"collection": {
							peerRole("p0"),
							peerRole("p12"),
						},
					}),
				},
				{
					Name:    cc1,
					Version: "1.0",
					CollectionsConfig: buildCollectionConfig(map[string][]*msp.MSPPrincipal{
						"collection": {
							peerRole("p3"),
							peerRole("p9"),
						},
					}),
				},
			},
			expected: peerSet{
				newPeer(0).withChaincode(cc1, "1.0"),
				newPeer(12).withChaincode(cc1, "1.0"),
			}.toMembers(),
		},
	} {
		t.Run(tst.name, func(t *testing.T) {
			g := &gossipMock{}
			pf := &policyFetcherMock{}
			mf := &metadataFetcher{}
			g.On("Peers").Return(alivePeers)
			g.On("IdentityInfo").Return(identities)
			g.On("PeersOfChannel").Return(tst.totalExistingMembers).Once()
			for _, md := range tst.metadata {
				mf.On("Metadata").Return(md).Once()
			}

			analyzer := NewEndorsementAnalyzer(g, pf, &principalEvaluatorMock{}, mf)
			actualMembers, err := analyzer.PeersAuthorizedByCriteria(common.ChannelID("mychannel"), tst.arguments)
			require.NoError(t, err)
			require.Equal(t, tst.expected, actualMembers)
		})
	}
}

func TestPop(t *testing.T) {
	slice := []inquire.ComparablePrincipalSets{{}, {}}
	require.Len(t, slice, 2)
	_, slice, err := popComparablePrincipalSets(slice)
	require.NoError(t, err)
	require.Len(t, slice, 1)
	_, slice, err = popComparablePrincipalSets(slice)
	require.NoError(t, err)
	require.Len(t, slice, 0)
	_, _, err = popComparablePrincipalSets(slice)
	require.Error(t, err)
	require.Equal(t, "no principal sets remained after filtering", err.Error())
}

func TestMergePrincipalSetsNilInput(t *testing.T) {
	_, err := mergePrincipalSets(nil)
	require.Error(t, err)
	require.Equal(t, "no principal sets remained after filtering", err.Error())
}

func TestComputePrincipalSetsNoPolicies(t *testing.T) {
	// Tests a hypothetical case where no chaincodes populate the chaincode interest.

	interest := &peer.ChaincodeInterest{
		Chaincodes: []*peer.ChaincodeCall{},
	}
	ea := &endorsementAnalyzer{}
	_, err := ea.computePrincipalSets(common.ChannelID("mychannel"), interest)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no principal sets remained after filtering")
}

func TestLoadMetadataAndFiltersCollectionNotPresentInConfig(t *testing.T) {
	interest := &peer.ChaincodeInterest{
		Chaincodes: []*peer.ChaincodeCall{
			{
				Name:            "mycc",
				CollectionNames: []string{"bar"},
			},
		},
	}

	org1AndOrg2 := []*msp.MSPPrincipal{
		orgPrincipal("Org1MSP"),
		orgPrincipal("Org2MSP"),
	}
	col2principals := map[string][]*msp.MSPPrincipal{
		"foo": org1AndOrg2,
	}
	config := buildCollectionConfig(col2principals)

	mdf := &metadataFetcher{}
	mdf.On("Metadata").Return(&chaincode.Metadata{
		Name:              "mycc",
		CollectionsConfig: config,
		Policy:            []byte{1, 2, 3},
	})

	_, err := loadMetadataAndFilters(metadataAndFilterContext{
		identityInfoByID: nil,
		evaluator:        nil,
		chainID:          common.ChannelID("mychannel"),
		fetch:            mdf,
		interest:         interest,
	})

	require.Equal(t, "collection bar doesn't exist in collection config for chaincode mycc", err.Error())
}

func TestLoadMetadataAndFiltersInvalidCollectionData(t *testing.T) {
	interest := &peer.ChaincodeInterest{
		Chaincodes: []*peer.ChaincodeCall{
			{
				Name:            "mycc",
				CollectionNames: []string{"col1"},
			},
		},
	}
	mdf := &metadataFetcher{}
	mdf.On("Metadata").Return(&chaincode.Metadata{
		Name:              "mycc",
		CollectionsConfig: &peer.CollectionConfigPackage{},
		Policy:            []byte{1, 2, 3},
	})

	_, err := loadMetadataAndFilters(metadataAndFilterContext{
		identityInfoByID: nil,
		evaluator:        nil,
		chainID:          common.ChannelID("mychannel"),
		fetch:            mdf,
		interest:         interest,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "collection col1 doesn't exist in collection config for chaincode mycc")
}

type peerSet []*peerInfo

func (p peerSet) toMembers() discovery.Members {
	var members discovery.Members
	for _, peer := range p {
		members = append(members, peer.NetworkMember)
	}
	return members
}

func identitySet(pkiID2MSPID map[string]string) api.PeerIdentitySet {
	var res api.PeerIdentitySet
	for pkiID, mspID := range pkiID2MSPID {
		sID := &msp.SerializedIdentity{
			Mspid:   pkiID2MSPID[pkiID],
			IdBytes: []byte(pkiID),
		}
		res = append(res, api.PeerIdentityInfo{
			Identity:     api.PeerIdentityType(protoutil.MarshalOrPanic(sID)),
			PKIId:        common.PKIidType(pkiID),
			Organization: api.OrgIdentityType(mspID),
		})
	}
	return res
}

type peerInfo struct {
	identity api.PeerIdentityType
	pkiID    common.PKIidType
	discovery.NetworkMember
}

func peerIdentityString(id string) string {
	return string(protoutil.MarshalOrPanic(&msp.SerializedIdentity{
		Mspid:   pkiID2MSPID[id],
		IdBytes: []byte(id),
	}))
}

func newPeer(i int) *peerInfo {
	p := fmt.Sprintf("p%d", i)
	identity := protoutil.MarshalOrPanic(&msp.SerializedIdentity{
		Mspid:   pkiID2MSPID[p],
		IdBytes: []byte(p),
	})
	return &peerInfo{
		pkiID:    common.PKIidType(p),
		identity: api.PeerIdentityType(identity),
		NetworkMember: discovery.NetworkMember{
			PKIid:            common.PKIidType(p),
			Endpoint:         p,
			InternalEndpoint: p,
			Envelope: &gossip.Envelope{
				Payload: []byte(identity),
			},
		},
	}
}

func peerRole(pkiID string) *msp.MSPPrincipal {
	return &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal: protoutil.MarshalOrPanic(&msp.MSPRole{
			MspIdentifier: pkiID2MSPID[pkiID],
			Role:          msp.MSPRole_PEER,
		}),
	}
}

func identity(pkiID string) *msp.MSPPrincipal {
	return &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_IDENTITY,
		Principal: protoutil.MarshalOrPanic(&msp.SerializedIdentity{
			Mspid:   pkiID2MSPID[pkiID],
			IdBytes: []byte(pkiID),
		}),
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

func (g *gossipMock) PeersOfChannel(_ common.ChannelID) discovery.Members {
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

func (pf *policyFetcherMock) PoliciesByChaincode(channel string, chaincode string, collections ...string) []policies.InquireablePolicy {
	arg := pf.Called(chaincode)
	if arg.Get(0) == nil {
		return nil
	}

	singlePolicy, isSinglePolicy := arg.Get(0).(policies.InquireablePolicy)
	if isSinglePolicy {
		return []policies.InquireablePolicy{singlePolicy}
	}

	return arg.Get(0).([]policies.InquireablePolicy)
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

type principalEvaluatorMock struct{}

func (pe *principalEvaluatorMock) SatisfiesPrincipal(_ string, identity []byte, principal *msp.MSPPrincipal) error {
	sId := &msp.SerializedIdentity{}
	if err := proto.Unmarshal(identity, sId); err != nil {
		return err
	}
	if principal.PrincipalClassification == msp.MSPPrincipal_IDENTITY {
		identityPrincipal := &msp.SerializedIdentity{}
		if err := proto.Unmarshal(principal.Principal, identityPrincipal); err != nil {
			return err
		}
		if proto.Equal(sId, identityPrincipal) {
			return nil
		}
		return fmt.Errorf("identities do not match")
	}
	// Else, it's either an OU type or a role type, so we only classify by MSP ID
	peerRole := &msp.MSPRole{}
	if err := proto.Unmarshal(principal.Principal, peerRole); err != nil {
		return err
	}
	if peerRole.MspIdentifier == sId.Mspid {
		return nil
	}
	return errors.New("bingo")
}

type metadataFetcher struct {
	mock.Mock
}

func (mf *metadataFetcher) Metadata(channel string, cc string, _ ...string) *chaincode.Metadata {
	arg := mf.Called().Get(0)
	if arg == nil {
		return nil
	}
	return arg.(*chaincode.Metadata)
}
