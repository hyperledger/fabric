/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privdata

import (
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/chaincode/implicitcollection"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protoutil"
)

var logger = flogging.MustGetLogger("common.privdata")

// MembershipProvider can be used to check whether a peer is eligible to a collection or not
type MembershipProvider struct {
	mspID                       string
	selfSignedData              protoutil.SignedData
	IdentityDeserializerFactory func(chainID string) msp.IdentityDeserializer
	myImplicitCollectionName    string
}

// NewMembershipInfoProvider returns MembershipProvider
func NewMembershipInfoProvider(mspID string, selfSignedData protoutil.SignedData, identityDeserializerFunc func(chainID string) msp.IdentityDeserializer) *MembershipProvider {
	return &MembershipProvider{
		mspID:                       mspID,
		selfSignedData:              selfSignedData,
		IdentityDeserializerFactory: identityDeserializerFunc,
		myImplicitCollectionName:    implicitcollection.NameForOrg(mspID),
	}
}

// AmMemberOf checks whether the current peer is a member of the given collection config.
// It is used when a chaincode is upgraded to see if the peer's org has become eligible after a collection change.
func (m *MembershipProvider) AmMemberOf(channelName string, collectionPolicyConfig *peer.CollectionPolicyConfig) (bool, error) {
	deserializer := m.IdentityDeserializerFactory(channelName)

	// Do a simple check to see if the mspid matches any principal identities in the SignaturePolicy - FAB-17059
	if collectionPolicyConfig.GetSignaturePolicy() == nil {
		logger.Warningf("collection membership policy is nil")
		return false, nil
	}

	memberOrgs := getMemberOrgs(collectionPolicyConfig.GetSignaturePolicy().GetIdentities(), deserializer)

	_, ok := memberOrgs[m.mspID]
	return ok, nil
}

func (m *MembershipProvider) MyImplicitCollectionName() string {
	return m.myImplicitCollectionName
}
