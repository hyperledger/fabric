/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorsement

import (
	"bytes"
	"testing"

	"fmt"

	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/gossip/api"
	gcommon "github.com/hyperledger/fabric/gossip/common"
	disc "github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/discovery"
	"github.com/hyperledger/fabric/protos/msp"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/stretchr/testify/assert"
)

func TestPrincipalsFromCollectionConfig(t *testing.T) {
	t.Run("Empty config", func(t *testing.T) {
		res, err := principalsFromCollectionConfig(nil)
		assert.NoError(t, err)
		assert.Empty(t, res)
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
		assert.NoError(t, err)
		assertEqualPrincipalSets(t, policies.PrincipalSet(org1AndOrg2), res["foo"])
		assertEqualPrincipalSets(t, policies.PrincipalSet(org3AndOrg4), res["bar"])
		assert.Empty(t, res["baz"])
	})
}

func TestNewCollectionFilterInvalidInput(t *testing.T) {
	t.Run("Invalid collection", func(t *testing.T) {
		filter, err := principalsFromCollectionConfig([]byte{1, 2, 3})
		assert.Nil(t, filter)
		assert.Contains(t, err.Error(), "invalid collection bytes")
	})

	t.Run("Invalid collection type", func(t *testing.T) {
		collections := &common.CollectionConfigPackage{}
		collections.Config = []*common.CollectionConfig{
			{
				Payload: nil,
			},
		}
		filter, err := principalsFromCollectionConfig(utils.MarshalOrPanic(collections))
		assert.Nil(t, filter)
		assert.Contains(t, err.Error(), "expected a static collection")
	})

	t.Run("Invalid membership policy", func(t *testing.T) {
		collections := &common.CollectionConfigPackage{}
		collections.Config = []*common.CollectionConfig{
			{
				Payload: &common.CollectionConfig_StaticCollectionConfig{
					StaticCollectionConfig: &common.StaticCollectionConfig{
						Name: "foo",
					},
				},
			},
		}
		filter, err := principalsFromCollectionConfig(utils.MarshalOrPanic(collections))
		assert.Nil(t, filter)
		assert.Contains(t, err.Error(), "MemberOrgsPolicy of foo is nil")
	})

	t.Run("Missing policy", func(t *testing.T) {
		collections := &common.CollectionConfigPackage{}
		collections.Config = []*common.CollectionConfig{
			{
				Payload: &common.CollectionConfig_StaticCollectionConfig{
					StaticCollectionConfig: &common.StaticCollectionConfig{
						Name:             "foo",
						MemberOrgsPolicy: &common.CollectionPolicyConfig{},
					},
				},
			},
		}
		filter, err := principalsFromCollectionConfig(utils.MarshalOrPanic(collections))
		assert.Nil(t, filter)
		assert.Contains(t, err.Error(), "policy of foo is nil")
	})
}

func TestToIdentityFilter(t *testing.T) {
	col2principals := make(principalSetsByCollectionName)
	col2principals["foo"] = []*msp.MSPPrincipal{orgPrincipal("Org1MSP"), orgPrincipal("Org2MSP")}

	t.Run("collection doesn't exist in mapping", func(t *testing.T) {
		filter, err := col2principals.toIdentityFilter("mychannel", &principalEvaluatorMock{}, &discovery.ChaincodeCall{
			Name:            "mycc",
			CollectionNames: []string{"bar"},
		})
		assert.Nil(t, filter)
		assert.Equal(t, "collection bar doesn't exist in collection config for chaincode mycc", err.Error())
	})

	t.Run("collection exists in mapping", func(t *testing.T) {
		filter, err := col2principals.toIdentityFilter("mychannel", &principalEvaluatorMock{}, &discovery.ChaincodeCall{
			Name:            "mycc",
			CollectionNames: []string{"foo"},
		})
		assert.NoError(t, err)
		identity := utils.MarshalOrPanic(&msp.SerializedIdentity{
			Mspid: "Org2MSP",
		})
		assert.True(t, filter(identity))
		identity = utils.MarshalOrPanic(&msp.SerializedIdentity{
			Mspid: "Org3MSP",
		})
		assert.False(t, filter(identity))
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
	assert.False(t, filter(api.PeerIdentityType("p1")))
	assert.True(t, filter(api.PeerIdentityType("p2")))
	assert.False(t, filter(api.PeerIdentityType("p3")))
	assert.False(t, filter(api.PeerIdentityType("p4")))
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
		assert.True(t, authorized)
	})

	t.Run("Member is unauthorized", func(t *testing.T) {
		authorized := memberFilter(disc.NetworkMember{
			PKIid: gcommon.PKIidType(unauthorizedIdentity),
		})
		assert.False(t, authorized)
	})

	t.Run("Member is not found in mapping", func(t *testing.T) {
		authorized := memberFilter(disc.NetworkMember{
			PKIid: gcommon.PKIidType(notPresentIdentity),
		})
		assert.False(t, authorized)
	})

}

func TestIsIdentityAuthorizedByPrincipalSet(t *testing.T) {
	principals := []*msp.MSPPrincipal{orgPrincipal("Org1MSP"), orgPrincipal("Org2MSP")}
	t.Run("Authorized", func(t *testing.T) {
		identity := utils.MarshalOrPanic(&msp.SerializedIdentity{
			Mspid: "Org1MSP",
		})
		authorized := isIdentityAuthorizedByPrincipalSet("mychannel", &principalEvaluatorMock{}, principals, identity)
		assert.True(t, authorized)
	})

	t.Run("Unauthorized", func(t *testing.T) {
		identity := utils.MarshalOrPanic(&msp.SerializedIdentity{
			Mspid: "Org3MSP",
		})
		authorized := isIdentityAuthorizedByPrincipalSet("mychannel", &principalEvaluatorMock{}, principals, identity)
		assert.False(t, authorized)
	})
}

func TestFilterForPrincipalSets(t *testing.T) {
	org1AndOrg2 := []*msp.MSPPrincipal{orgPrincipal("Org1MSP"), orgPrincipal("Org2MSP")}
	org2AndOrg3 := []*msp.MSPPrincipal{orgPrincipal("Org2MSP"), orgPrincipal("Org3MSP")}
	org3AndOrg4 := []*msp.MSPPrincipal{orgPrincipal("Org3MSP"), orgPrincipal("Org4MSP")}

	identity := utils.MarshalOrPanic(&msp.SerializedIdentity{
		Mspid: "Org2MSP",
	})

	t.Run("Identity is authorized by all principals", func(t *testing.T) {
		filter := filterForPrincipalSets("mychannel", &principalEvaluatorMock{}, policies.PrincipalSets{org1AndOrg2, org2AndOrg3})
		assert.True(t, filter(identity))
	})

	t.Run("Identity is not authorized by all principals", func(t *testing.T) {
		filter := filterForPrincipalSets("mychannel", &principalEvaluatorMock{}, policies.PrincipalSets{org1AndOrg2, org3AndOrg4})
		assert.False(t, filter(identity))
	})
}

func buildCollectionConfig(col2principals map[string][]*msp.MSPPrincipal) []byte {
	collections := &common.CollectionConfigPackage{}
	for col, principals := range col2principals {
		collections.Config = append(collections.Config, &common.CollectionConfig{
			Payload: &common.CollectionConfig_StaticCollectionConfig{
				StaticCollectionConfig: &common.StaticCollectionConfig{
					Name: col,
					MemberOrgsPolicy: &common.CollectionPolicyConfig{
						Payload: &common.CollectionPolicyConfig_SignaturePolicy{
							SignaturePolicy: &common.SignaturePolicyEnvelope{
								Identities: principals,
							},
						},
					},
				},
			},
		})
	}
	return utils.MarshalOrPanic(collections)
}

func orgPrincipal(mspID string) *msp.MSPPrincipal {
	return &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal: utils.MarshalOrPanic(&msp.MSPRole{
			MspIdentifier: mspID,
			Role:          msp.MSPRole_PEER,
		}),
	}
}

func assertEqualPrincipalSets(t *testing.T, ps1, ps2 policies.PrincipalSet) {
	ps1s := fmt.Sprintf("%v", ps1)
	ps2s := fmt.Sprintf("%v", ps2)
	assert.Equal(t, ps1s, ps2s)
}
