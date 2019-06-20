/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privdata

import (
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protos/common"
)

var logger = flogging.MustGetLogger("common.privdata")

// MembershipProvider can be used to check whether a peer is eligible to a collection or not
type MembershipProvider struct {
	selfSignedData              common.SignedData
	IdentityDeserializerFactory func(chainID string) msp.IdentityDeserializer
}

// NewMembershipInfoProvider returns MembershipProvider
func NewMembershipInfoProvider(selfSignedData common.SignedData, identityDeserializerFunc func(chainID string) msp.IdentityDeserializer) *MembershipProvider {
	return &MembershipProvider{selfSignedData: selfSignedData, IdentityDeserializerFactory: identityDeserializerFunc}
}

// AmMemberOf checks whether the current peer is a member of the given collection config.
// If getPolicy returns an error, it will drop the error and return false - same as a RejectAll policy.
func (m *MembershipProvider) AmMemberOf(channelName string, collectionPolicyConfig *common.CollectionPolicyConfig) (bool, error) {
	deserializer := m.IdentityDeserializerFactory(channelName)
	accessPolicy, err := getPolicy(collectionPolicyConfig, deserializer)
	if err != nil {
		// drop the error and return false - same as reject all policy
		logger.Errorf("Reject all due to error getting policy: %s", err)
		return false, nil
	}
	if err := accessPolicy.Evaluate([]*common.SignedData{&m.selfSignedData}); err != nil {
		return false, nil
	}
	return true, nil
}
