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
)

// resourceGroup represents the ConfigGroup at the base of the resource configuration.
type resourceGroup struct {
	apisGroup         *apisGroup
	peerPoliciesGroup *peerPoliciesGroup
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

	for subGroupName, subGroup := range root.Groups {
		var err error
		switch subGroupName {
		case APIsGroupKey:
			rg.apisGroup, err = newAPIsGroup(subGroup)
		case PeerPoliciesGroupKey:
			rg.peerPoliciesGroup, err = newPeerPoliciesGroup(subGroup)
		default:
			err = errors.New("unknown sub-group")
		}
		if err != nil {
			return nil, errors.Wrapf(err, "error processing group %s", subGroupName)
		}
	}

	return rg, nil
}
