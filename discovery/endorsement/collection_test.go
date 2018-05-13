/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorsement

import (
	"testing"

	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/msp"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestForCollections(t *testing.T) {
	foos := policies.PrincipalSets{{orgPrincipal("foo")}}
	bars := policies.PrincipalSets{{orgPrincipal("bar")}}
	f := filterPrincipalSets(func(collectionName string, principalSets policies.PrincipalSets) (policies.PrincipalSets, error) {
		switch collectionName {
		case "foo":
			return foos, nil
		case "bar":
			return bars, nil
		default:
			return nil, errors.Errorf("collection %s doesn't exist", collectionName)
		}
	})

	res, err := f.forCollections("mycc", "foo")(nil)
	assert.NoError(t, err)
	assert.Equal(t, foos, res)

	res, err = f.forCollections("mycc", "bar")(nil)
	assert.NoError(t, err)
	assert.Equal(t, bars, res)

	res, err = f.forCollections("mycc", "baz")(nil)
	assert.Equal(t, "collection baz doesn't exist", err.Error())
}

func TestCollectionFilter(t *testing.T) {
	org1AndOrg2 := []*msp.MSPPrincipal{orgPrincipal("Org1MSP"), orgPrincipal("Org2MSP")}
	org1AndOrg3 := []*msp.MSPPrincipal{orgPrincipal("Org1MSP"), orgPrincipal("Org3MSP")}
	org3AndOrg4 := []*msp.MSPPrincipal{orgPrincipal("Org3MSP"), orgPrincipal("Org4MSP")}

	t.Run("Filter out a subset", func(t *testing.T) {
		// Scenario I:
		// Endorsement policy is: OR(
		// 							AND(Org1MSP.peer, Org2MSP.peer),
		//							AND(Org3MSP.peer, Org4MSP.peer),
		// But collection config is OR(Org3MSP.peer, Org4MSP.peer).
		// Therefore, only 1 principal set should be selected - the org3 and org4 combination
		config := buildCollectionConfig("foo", org3AndOrg4...)
		principalSets := policies.PrincipalSets{org1AndOrg2, org3AndOrg4}
		filter, err := newCollectionFilter(config)
		assert.NoError(t, err)
		principalSets, err = filter("foo", principalSets)
		assert.NoError(t, err)
		assert.Equal(t, policies.PrincipalSets{org3AndOrg4}, principalSets)

		principalSets, err = filter("bar", principalSets)
		assert.Nil(t, principalSets)
		assert.Contains(t, err.Error(), "collection bar wasn't found in configuration")
	})

	t.Run("Filter out all", func(t *testing.T) {
		// Scenario II:
		// Endorsement policy is: OR(
		// 							AND(Org1MSP.peer, Org2MSP.peer),
		//							AND(Org3MSP.peer, Org4MSP.peer),
		// But collection config is OR(Org1MSP.peer, Org3MSP.peer).
		// Therefore, no principal combination should be selected
		config := buildCollectionConfig("foo", org1AndOrg3...)
		principalSets := policies.PrincipalSets{org1AndOrg2, org3AndOrg4}
		filter, err := newCollectionFilter(config)
		assert.NoError(t, err)
		principalSets, err = filter("foo", principalSets)
		assert.NoError(t, err)
		assert.Empty(t, principalSets)
	})
}

func TestNewCollectionFilterInvalidInput(t *testing.T) {
	t.Run("Invalid collection", func(t *testing.T) {
		filter, err := newCollectionFilter([]byte{1, 2, 3})
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
		filter, err := newCollectionFilter(utils.MarshalOrPanic(collections))
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
		filter, err := newCollectionFilter(utils.MarshalOrPanic(collections))
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
		filter, err := newCollectionFilter(utils.MarshalOrPanic(collections))
		assert.Nil(t, filter)
		assert.Contains(t, err.Error(), "policy of foo is nil")
	})

	t.Run("Unsupported principal", func(t *testing.T) {
		principal := &msp.MSPPrincipal{
			PrincipalClassification: msp.MSPPrincipal_IDENTITY,
			Principal:               []byte("identity"),
		}
		filter, err := newCollectionFilter(buildCollectionConfig("foo", principal))
		assert.Nil(t, filter)
		assert.Contains(t, err.Error(), "failed constructing principal set for foo: principals given are")
	})
}

func TestCollectionFilterInvalid(t *testing.T) {
	t.Run("Collection that doesn't exist", func(t *testing.T) {
		filter, err := newCollectionFilter(nil)
		assert.NoError(t, err)
		_, err = filter("bla", nil)
		assert.Contains(t, err.Error(), "collection bla wasn't found in configuration")
	})

	t.Run("Given principals are invalid", func(t *testing.T) {
		principal := &msp.MSPPrincipal{
			PrincipalClassification: msp.MSPPrincipal_ROLE,
			Principal:               utils.MarshalOrPanic(&msp.MSPRole{MspIdentifier: "Org1MSP", Role: msp.MSPRole_MEMBER}),
		}
		filter, err := newCollectionFilter(buildCollectionConfig("foo", principal))
		assert.NoError(t, err)
		principalSets := policies.PrincipalSets{[]*msp.MSPPrincipal{{PrincipalClassification: msp.MSPPrincipal_IDENTITY, Principal: []byte("identity")}}}
		_, err = filter("foo", principalSets)
		assert.Contains(t, err.Error(), "principal set [principal_classification:IDENTITY principal:\"identity\" ] is invalid")
	})
}

func buildCollectionConfig(name string, principals ...*msp.MSPPrincipal) []byte {
	collections := &common.CollectionConfigPackage{}
	collections.Config = []*common.CollectionConfig{
		{
			Payload: &common.CollectionConfig_StaticCollectionConfig{
				StaticCollectionConfig: &common.StaticCollectionConfig{
					Name: name,
					MemberOrgsPolicy: &common.CollectionPolicyConfig{
						Payload: &common.CollectionPolicyConfig_SignaturePolicy{
							SignaturePolicy: &common.SignaturePolicyEnvelope{
								Identities: principals,
							},
						},
					},
				},
			},
		},
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
