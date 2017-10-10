/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package resourcesconfig

import (
	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/pkg/errors"
)

// peerPoliciesGroup is a free-form group, which supports only policies
type peerPoliciesGroup struct{}

func newPeerPoliciesGroup(group *cb.ConfigGroup) (*peerPoliciesGroup, error) {
	return &peerPoliciesGroup{}, verifyNoMoreValues(group)
}

func verifyNoMoreValues(subGroup *cb.ConfigGroup) error {
	if len(subGroup.Values) > 0 {
		return errors.Errorf("sub-groups not allowed to have values")
	}
	for _, subGroup := range subGroup.Groups {
		if err := verifyNoMoreValues(subGroup); err != nil {
			return err
		}
	}
	return nil
}
