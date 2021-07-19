/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorsement

import (
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/pkg/errors"
)

func principalsFromCollectionConfig(ccp *peer.CollectionConfigPackage) (principalSetsByCollectionName, error) {
	principalSetsByCollections := make(principalSetsByCollectionName)
	if ccp == nil {
		return principalSetsByCollections, nil
	}
	for _, colConfig := range ccp.Config {
		staticCol := colConfig.GetStaticCollectionConfig()
		if staticCol == nil {
			// Right now we only support static collections, so if we got something else
			// we should refuse to process further
			return nil, errors.Errorf("expected a static collection but got %v instead", colConfig)
		}
		if staticCol.MemberOrgsPolicy == nil {
			return nil, errors.Errorf("MemberOrgsPolicy of %s is nil", staticCol.Name)
		}
		pol := staticCol.MemberOrgsPolicy.GetSignaturePolicy()
		if pol == nil {
			return nil, errors.Errorf("policy of %s is nil", staticCol.Name)
		}
		var principals policies.PrincipalSet
		// We now extract all principals from the policy
		for _, principal := range pol.Identities {
			principals = append(principals, principal)
		}
		principalSetsByCollections[staticCol.Name] = principals
	}
	return principalSetsByCollections, nil
}

type principalSetsByCollectionName map[string]policies.PrincipalSet

// toIdentityFilter converts this principalSetsByCollectionName mapping to a filter
// which accepts or rejects identities of peers.
func (psbc principalSetsByCollectionName) toIdentityFilter(channel string, evaluator principalEvaluator, cc *peer.ChaincodeCall) (identityFilter, error) {
	var principalSets policies.PrincipalSets
	for _, col := range cc.CollectionNames {
		// Each collection we're interested in should exist in the principalSetsByCollectionName mapping.
		// Otherwise, we have no way of computing a filter because we can't locate the principals the peer identities
		// need to satisfy.
		principalSet, exists := psbc[col]
		if !exists {
			return nil, errors.Errorf("collection %s doesn't exist in collection config for chaincode %s", col, cc.Name)
		}
		principalSets = append(principalSets, principalSet)
	}
	return filterForPrincipalSets(channel, evaluator, principalSets), nil
}

// filterForPrincipalSets creates a filter of peer identities out of the given PrincipalSets
func filterForPrincipalSets(channel string, evaluator principalEvaluator, sets policies.PrincipalSets) identityFilter {
	return func(identity api.PeerIdentityType) bool {
		// Iterate over all principal sets and ensure each principal set
		// authorizes the identity.
		for _, principalSet := range sets {
			if !isIdentityAuthorizedByPrincipalSet(channel, evaluator, principalSet, identity) {
				return false
			}
		}
		return true
	}
}

// isIdentityAuthorizedByPrincipalSet returns whether the given identity satisfies some principal out of the given PrincipalSet
func isIdentityAuthorizedByPrincipalSet(channel string, evaluator principalEvaluator, principalSet policies.PrincipalSet, identity api.PeerIdentityType) bool {
	// We look for a principal which authorizes the identity
	// among all principals in the principalSet
	for _, principal := range principalSet {
		err := evaluator.SatisfiesPrincipal(channel, identity, principal)
		if err != nil {
			continue
		}
		// Else, err is nil, so we found a principal which authorized
		// the given identity.
		return true
	}
	return false
}
