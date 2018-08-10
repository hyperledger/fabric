/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privdata

import (
	"github.com/hyperledger/fabric/protos/common"
)

// MembershipInfoProvider interface defines an interface to check whether a peer is eligible to a collection or not
type MembershipInfoProvider interface {
	// AmMemberOf checks whether the current peer is a member of the given collection config
	AmMemberOf(collectionPolicyConfig *common.CollectionPolicyConfig) (bool, error)
}

type membershipProvider struct {
	selfSignedData common.SignedData
	cf             CollectionFilter
	channelName    string
}

func NewMembershipInfoProvider(channelName string, selfSignedData common.SignedData, filter CollectionFilter) MembershipInfoProvider {
	return &membershipProvider{channelName: channelName, selfSignedData: selfSignedData, cf: filter}
}

func (m *membershipProvider) AmMemberOf(collectionPolicyConfig *common.CollectionPolicyConfig) (bool, error) {
	filt, err := m.cf.AccessFilter(m.channelName, collectionPolicyConfig)
	if err != nil {
		return false, err
	}
	return filt(m.selfSignedData), nil
}
