/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorsement

import (
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/common/policies/inquire"
	"github.com/hyperledger/fabric/core/common/privdata"
	"github.com/pkg/errors"
)

type filterPrincipalSets func(collectionName string, principalSets policies.PrincipalSets) (policies.PrincipalSets, error)

func (f filterPrincipalSets) forCollections(ccName string, collections ...string) filterFunc {
	return func(principalSets policies.PrincipalSets) (policies.PrincipalSets, error) {
		var err error
		for _, col := range collections {
			principalSets, err = f(col, principalSets)
			if err != nil {
				logger.Warningf("Failed filtering collection for chaincode %s, collection %s: %v", ccName, col, err)
				return nil, err
			}
		}
		return principalSets, nil
	}
}

func newCollectionFilter(configBytes []byte) (filterPrincipalSets, error) {
	mapFilter := make(principalSetsByCollectionName)
	if len(configBytes) == 0 {
		return mapFilter.filter, nil
	}
	ccp, err := privdata.ParseCollectionConfig(configBytes)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid collection bytes")
	}
	for _, cfg := range ccp.Config {
		staticCol := cfg.GetStaticCollectionConfig()
		if staticCol == nil {
			// Right now we only support static collections, so if we got something else
			// we should refuse to process further
			return nil, errors.Errorf("expected a static collection but got %v instead", cfg)
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
		principalSet := inquire.NewComparablePrincipalSet(principals)
		if principalSet == nil {
			return nil, errors.Errorf("failed constructing principal set for %s: principals given are %v", staticCol.Name, pol.Identities)
		}
		mapFilter[staticCol.Name] = principalSet
	}
	return mapFilter.filter, nil
}

type principalSetsByCollectionName map[string]inquire.ComparablePrincipalSet

func (psbc principalSetsByCollectionName) filter(collectionName string, principalSets policies.PrincipalSets) (policies.PrincipalSets, error) {
	collectionPrincipals, exists := psbc[collectionName]
	if !exists {
		return nil, errors.Errorf("collection %s wasn't found in configuration", collectionName)
	}
	var res policies.PrincipalSets
	for _, ps := range principalSets {
		comparablePS := inquire.NewComparablePrincipalSet(ps)
		if comparablePS == nil {
			return nil, errors.Errorf("principal set %v is invalid", ps)
		}
		if comparablePS.IsCoveredBy(collectionPrincipals) {
			res = append(res, ps)
		}
	}
	return res, nil
}
