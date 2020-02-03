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
	mspID                       string
	selfSignedData              common.SignedData
	IdentityDeserializerFactory func(chainID string) msp.IdentityDeserializer
}

// NewMembershipInfoProvider returns MembershipProvider
func NewMembershipInfoProvider(mspID string, selfSignedData common.SignedData,
	identityDeserializerFunc func(chainID string) msp.IdentityDeserializer) *MembershipProvider {
	return &MembershipProvider{mspID: mspID, selfSignedData: selfSignedData, IdentityDeserializerFactory: identityDeserializerFunc}
}

// AmMemberOf checks whether the current peer is a member of the given collection config.
// If getPolicy returns an error, it will drop the error and return false - same as a RejectAll policy.
// It is used when a chaincode is upgraded to see if the peer's org has become eligible after	a collection
// change.
func (m *MembershipProvider) AmMemberOf(channelName string,
	collectionPolicyConfig *common.CollectionPolicyConfig) (bool, error) {
	deserializer := m.IdentityDeserializerFactory(channelName)

	// Do a simple check to see if the mspid matches any principal identities in the SignaturePolicy - FAB-17059
	if collectionPolicyConfig.GetSignaturePolicy() != nil {
		memberOrgs := getMemberOrgs(collectionPolicyConfig.GetSignaturePolicy().GetIdentities(), deserializer)

		for _, org := range memberOrgs {
			if m.mspID == org {
				return true, nil
			}
		}
	}

	// Fall back to default access policy evaluation otherwise
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
