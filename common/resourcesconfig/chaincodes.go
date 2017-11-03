/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package resourcesconfig

import (
	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/pkg/errors"
)

// chaincodesGroup represents the ConfigGroup named Chaincodes off the resources group
type chaincodesGroup struct {
	chaincodeGroups map[string]*ChaincodeGroup
}

func (cg *chaincodesGroup) ChaincodeByName(name string) (ChaincodeDefinition, bool) {
	cc, ok := cg.chaincodeGroups[name]
	return cc, ok
}

func newChaincodesGroup(group *cb.ConfigGroup) (*chaincodesGroup, error) {
	if len(group.Values) > 0 {
		return nil, errors.New("chaincodes group does not support values")
	}

	chaincodeGroups := make(map[string]*ChaincodeGroup)

	for key, subGroup := range group.Groups {
		chaincodeGroup, err := newChaincodeGroup(key, subGroup)
		if err != nil {
			return nil, errors.Wrapf(err, "could not construct chaincode sub-group %s", key)
		}
		chaincodeGroups[key] = chaincodeGroup
	}

	return &chaincodesGroup{
		chaincodeGroups: chaincodeGroups,
	}, nil
}
