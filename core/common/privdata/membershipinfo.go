/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privdata

import (
	"github.com/hyperledger/fabric/protos/common"
)

// MembershipProvider can be used to check whether a peer is eligible to a collection or not
type MembershipProvider struct {
	selfSignedData common.SignedData
	cf             CollectionFilter
}

// NewMembershipInfoProvider returns MembershipProvider
func NewMembershipInfoProvider(selfSignedData common.SignedData, filter CollectionFilter) *MembershipProvider {
	return &MembershipProvider{selfSignedData: selfSignedData, cf: filter}
}

// AmMemberOf checks whether the current peer is a member of the given collection config
func (m *MembershipProvider) AmMemberOf(channelName string, collectionPolicyConfig *common.CollectionPolicyConfig) (bool, error) {
	filt, err := m.cf.AccessFilter(channelName, collectionPolicyConfig)
	if err != nil {
		return false, err
	}
	return filt(m.selfSignedData), nil
}
