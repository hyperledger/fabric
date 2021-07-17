/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorsement

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/gossip/api"
	gcommon "github.com/hyperledger/fabric/gossip/common"
	disc "github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
)

func TestPrincipalsFromCollectionConfig(t *testing.T) {
	t.Run("Empty config", func(t *testing.T) {
		res, err := principalsFromCollectionConfig(nil)
		require.NoError(t, err)
		require.Empty(t, res)
	})

	t.Run("Not empty config", func(t *testing.T) {
		org1AndOrg2 := []*msp.MSPPrincipal{orgPrincipal("Org1MSP"), orgPrincipal("Org2MSP")}
		org3AndOrg4 := []*msp.MSPPrincipal{orgPrincipal("Org3MSP"), orgPrincipal("Org4MSP")}
		col2principals := map[string][]*msp.MSPPrincipal{
			"foo": org1AndOrg2,
			"bar": org3AndOrg4,
		}
		config := buildCollectionConfig(col2principals)
		res, err := principalsFromCollectionConfig(config)
		require.NoError(t, err)
		assertEqualPrincipalSets(t, policies.PrincipalSet(org1AndOrg2), res["foo"])
		assertEqualPrincipalSets(t, policies.PrincipalSet(org3AndOrg4), res["bar"])
		require.Empty(t, res["baz"])
	})
}

func TestNewCollectionFilterInvalidInput(t *testing.T) {
	t.Run("Invalid collection type", func(t *testing.T) {
		collections := &peer.CollectionConfigPackage{}
		collections.Config = []*peer.CollectionConfig{
			{
				Payload: nil,
			},
		}
		filter, err := principalsFromCollectionConfig(collections)
		require.Nil(t, filter)
		require.Contains(t, err.Error(), "expected a static collection")
	})

	t.Run("Invalid membership policy", func(t *testing.T) {
		collections := &peer.CollectionConfigPackage{}
		collections.Config = []*peer.CollectionConfig{
			{
				Payload: &peer.CollectionConfig_StaticCollectionConfig{
					StaticCollectionConfig: &peer.StaticCollectionConfig{
						Name: "foo",
					},
				},
			},
		}
		filter, err := principalsFromCollectionConfig(collections)
		require.Nil(t, filter)
		require.Contains(t, err.Error(), "MemberOrgsPolicy of foo is nil")
	})

	t.Run("Missing policy", func(t *testing.T) {
		collections := &peer.CollectionConfigPackage{}
		collections.Config = []*peer.CollectionConfig{
			{
				Payload: &peer.CollectionConfig_StaticCollectionConfig{
					StaticCollectionConfig: &peer.StaticCollectionConfig{
						Name:             "foo",
						MemberOrgsPolicy: &peer.CollectionPolicyConfig{},
					},
				},
			},
		}
		filter, err := principalsFromCollectionConfig(collections)
		require.Nil(t, filter)
		require.Contains(t, err.Error(), "policy of foo is nil")
	})
}

func TestToIdentityFilter(t *testing.T) {
	col2principals := make(principalSetsByCollectionName)
	col2principals["foo"] = []*msp.MSPPrincipal{orgPrincipal("Org1MSP"), orgPrincipal("Org2MSP")}

	t.Run("collection doesn't exist in mapping", func(t *testing.T) {
		filter, err := col2principals.toIdentityFilter("mychannel", &principalEvaluatorMock{}, &peer.ChaincodeCall{
			Name:            "mycc",
			CollectionNames: []string{"bar"},
		})
		require.Nil(t, filter)
		require.Equal(t, "collection bar doesn't exist in collection config for chaincode mycc", err.Error())
	})

	t.Run("collection exists in mapping", func(t *testing.T) {
		filter, err := col2principals.toIdentityFilter("mychannel", &principalEvaluatorMock{}, &peer.ChaincodeCall{
			Name:            "mycc",
			CollectionNames: []string{"foo"},
		})
		require.NoError(t, err)
		identity := protoutil.MarshalOrPanic(&msp.SerializedIdentity{
			Mspid: "Org2MSP",
		})
		require.True(t, filter(identity))
		identity = protoutil.MarshalOrPanic(&msp.SerializedIdentity{
			Mspid: "Org3MSP",
		})
		require.False(t, filter(identity))
	})
}

func TestCombine(t *testing.T) {
	filter1 := identityFilter(func(identity api.PeerIdentityType) bool {
		return bytes.Equal([]byte("p1"), identity) || bytes.Equal([]byte("p2"), identity)
	})

	filter2 := identityFilter(func(identity api.PeerIdentityType) bool {
		return bytes.Equal([]byte("p2"), identity) || bytes.Equal([]byte("p3"), identity)
	})

	filter := identityFilters{filter1, filter2}.combine()
	require.False(t, filter(api.PeerIdentityType("p1")))
	require.True(t, filter(api.PeerIdentityType("p2")))
	require.False(t, filter(api.PeerIdentityType("p3")))
	require.False(t, filter(api.PeerIdentityType("p4")))
}

func TestToMemberFilter(t *testing.T) {
	unauthorizedIdentity := api.PeerIdentityType("unauthorizedIdentity")
	authorizedIdentity := api.PeerIdentityType("authorizedIdentity")
	notPresentIdentity := api.PeerIdentityType("api.PeerIdentityInfo")

	filter := identityFilter(func(identity api.PeerIdentityType) bool {
		return bytes.Equal(authorizedIdentity, identity)
	})
	identityInfoByID := map[string]api.PeerIdentityInfo{
		"authorizedIdentity":   {PKIId: gcommon.PKIidType(authorizedIdentity), Identity: authorizedIdentity},
		"unauthorizedIdentity": {PKIId: gcommon.PKIidType(unauthorizedIdentity), Identity: unauthorizedIdentity},
	}
	memberFilter := filter.toMemberFilter(identityInfoByID)

	t.Run("Member is authorized", func(t *testing.T) {
		authorized := memberFilter(disc.NetworkMember{
			PKIid: gcommon.PKIidType(authorizedIdentity),
		})
		require.True(t, authorized)
	})

	t.Run("Member is unauthorized", func(t *testing.T) {
		authorized := memberFilter(disc.NetworkMember{
			PKIid: gcommon.PKIidType(unauthorizedIdentity),
		})
		require.False(t, authorized)
	})

	t.Run("Member is not found in mapping", func(t *testing.T) {
		authorized := memberFilter(disc.NetworkMember{
			PKIid: gcommon.PKIidType(notPresentIdentity),
		})
		require.False(t, authorized)
	})
}

func TestIsIdentityAuthorizedByPrincipalSet(t *testing.T) {
	principals := []*msp.MSPPrincipal{orgPrincipal("Org1MSP"), orgPrincipal("Org2MSP")}
	t.Run("Authorized", func(t *testing.T) {
		identity := protoutil.MarshalOrPanic(&msp.SerializedIdentity{
			Mspid: "Org1MSP",
		})
		authorized := isIdentityAuthorizedByPrincipalSet("mychannel", &principalEvaluatorMock{}, principals, identity)
		require.True(t, authorized)
	})

	t.Run("Unauthorized", func(t *testing.T) {
		identity := protoutil.MarshalOrPanic(&msp.SerializedIdentity{
			Mspid: "Org3MSP",
		})
		authorized := isIdentityAuthorizedByPrincipalSet("mychannel", &principalEvaluatorMock{}, principals, identity)
		require.False(t, authorized)
	})
}

func TestFilterForPrincipalSets(t *testing.T) {
	org1AndOrg2 := []*msp.MSPPrincipal{orgPrincipal("Org1MSP"), orgPrincipal("Org2MSP")}
	org2AndOrg3 := []*msp.MSPPrincipal{orgPrincipal("Org2MSP"), orgPrincipal("Org3MSP")}
	org3AndOrg4 := []*msp.MSPPrincipal{orgPrincipal("Org3MSP"), orgPrincipal("Org4MSP")}

	identity := protoutil.MarshalOrPanic(&msp.SerializedIdentity{
		Mspid: "Org2MSP",
	})

	t.Run("Identity is authorized by all principals", func(t *testing.T) {
		filter := filterForPrincipalSets("mychannel", &principalEvaluatorMock{}, policies.PrincipalSets{org1AndOrg2, org2AndOrg3})
		require.True(t, filter(identity))
	})

	t.Run("Identity is not authorized by all principals", func(t *testing.T) {
		filter := filterForPrincipalSets("mychannel", &principalEvaluatorMock{}, policies.PrincipalSets{org1AndOrg2, org3AndOrg4})
		require.False(t, filter(identity))
	})
}

func buildCollectionConfig(col2principals map[string][]*msp.MSPPrincipal) *peer.CollectionConfigPackage {
	collections := &peer.CollectionConfigPackage{}
	for col, principals := range col2principals {
		collections.Config = append(collections.Config, &peer.CollectionConfig{
			Payload: &peer.CollectionConfig_StaticCollectionConfig{
				StaticCollectionConfig: &peer.StaticCollectionConfig{
					Name: col,
					MemberOrgsPolicy: &peer.CollectionPolicyConfig{
						Payload: &peer.CollectionPolicyConfig_SignaturePolicy{
							SignaturePolicy: &common.SignaturePolicyEnvelope{
								Identities: principals,
							},
						},
					},
				},
			},
		})
	}

	return collections
}

func orgPrincipal(mspID string) *msp.MSPPrincipal {
	return &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal: protoutil.MarshalOrPanic(&msp.MSPRole{
			MspIdentifier: mspID,
			Role:          msp.MSPRole_PEER,
		}),
	}
}

func assertEqualPrincipalSets(t *testing.T, ps1, ps2 policies.PrincipalSet) {
	ps1s := fmt.Sprintf("%v", ps1)
	ps2s := fmt.Sprintf("%v", ps2)
	require.Equal(t, ps1s, ps2s)
}
