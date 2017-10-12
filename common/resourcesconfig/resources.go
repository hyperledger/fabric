/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package resourcesconfig

import (
	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/pkg/errors"
)

const (
	PeerPoliciesGroupKey = "PeerPolicies"
	APIsGroupKey         = "APIs"
	ChaincodesGroupKey   = "Chaincodes"
)

// resourceGroup represents the ConfigGroup at the base of the resource configuration.
// Note, the members are always initialized, so that there is no need for nil checks.
type resourceGroup struct {
	apisGroup         *apisGroup
	peerPoliciesGroup *peerPoliciesGroup
	chaincodesGroup   *chaincodesGroup
}

func newResourceGroup(root *cb.ConfigGroup) (*resourceGroup, error) {
	if len(root.Values) > 0 {
		return nil, errors.New("/Resources group does not support any values")
	}

	// initialize the elements with empty implementations, override if actually set
	rg := &resourceGroup{
		apisGroup:         &apisGroup{},
		peerPoliciesGroup: &peerPoliciesGroup{},
	}
	rg.chaincodesGroup, _ = newChaincodesGroup(&cb.ConfigGroup{}) // Initialize to empty

	for subGroupName, subGroup := range root.Groups {
		var err error
		switch subGroupName {
		case APIsGroupKey:
			rg.apisGroup, err = newAPIsGroup(subGroup)
		case PeerPoliciesGroupKey:
			rg.peerPoliciesGroup, err = newPeerPoliciesGroup(subGroup)
		case ChaincodesGroupKey:
			rg.chaincodesGroup, err = newChaincodesGroup(subGroup)
		default:
			err = errors.New("unknown sub-group")
		}
		if err != nil {
			return nil, errors.Wrapf(err, "error processing group %s", subGroupName)
		}
	}

	return rg, nil
}
