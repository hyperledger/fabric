/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privdata

import (
	"testing"

	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/policydsl"
	"github.com/hyperledger/fabric/core/chaincode/implicitcollection"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
)

func TestMembershipInfoProvider(t *testing.T) {
	mspID := "peer0"
	peerSelfSignedData := protoutil.SignedData{
		Identity:  []byte("peer0"),
		Signature: []byte{1, 2, 3},
		Data:      []byte{4, 5, 6},
	}
	emptyPeerSelfSignedData := protoutil.SignedData{}

	identityDeserializer := func(chainID string) msp.IdentityDeserializer {
		return &mockDeserializer{}
	}

	// verify membership provider pass simple check returns true
	membershipProvider := NewMembershipInfoProvider(mspID, emptyPeerSelfSignedData, identityDeserializer)
	res, err := membershipProvider.AmMemberOf("test1", getAccessPolicy([]string{"peer0", "peer1"}))
	require.True(t, res)
	require.Nil(t, err)

	// verify membership provider fall back to default access policy evaluation returns false
	membershipProvider = NewMembershipInfoProvider(mspID, peerSelfSignedData, identityDeserializer)
	res, err = membershipProvider.AmMemberOf("test1", getAccessPolicy([]string{"peer2", "peer3"}))
	require.False(t, res)
	require.Nil(t, err)

	// verify membership provider returns false and nil when collection policy config is nil
	res, err = membershipProvider.AmMemberOf("test1", nil)
	require.False(t, res)
	require.Nil(t, err)

	// verify membership provider returns false and nil when collection policy config is invalid
	res, err = membershipProvider.AmMemberOf("test1", getBadAccessPolicy([]string{"signer0"}, 1))
	require.False(t, res)
	require.Nil(t, err)

	// verify membership provider with empty mspID and fall back to default access policy evaluation returns true
	membershipProvider = NewMembershipInfoProvider("", peerSelfSignedData, identityDeserializer)
	res, err = membershipProvider.AmMemberOf("test1", getAccessPolicy([]string{"peer0", "peer1"}))
	require.True(t, res)
	require.Nil(t, err)

	// verify membership provider with empty mspID and fall back to default access policy evaluation returns false
	res, err = membershipProvider.AmMemberOf("test1", getAccessPolicy([]string{"peer2", "peer3"}))
	require.False(t, res)
	require.Nil(t, err)
}

func TestMyImplicitCollectionName(t *testing.T) {
	require.Equal(
		t,
		implicitcollection.NameForOrg("my_org"),
		NewMembershipInfoProvider("my_org", protoutil.SignedData{}, nil).MyImplicitCollectionName(),
	)
}

func getAccessPolicy(signers []string) *peer.CollectionPolicyConfig {
	var data [][]byte
	for _, signer := range signers {
		data = append(data, []byte(signer))
	}
	policyEnvelope := policydsl.Envelope(policydsl.Or(policydsl.SignedBy(0), policydsl.SignedBy(1)), data)
	return createCollectionPolicyConfig(policyEnvelope)
}

func getBadAccessPolicy(signers []string, badIndex int32) *peer.CollectionPolicyConfig {
	var data [][]byte
	for _, signer := range signers {
		data = append(data, []byte(signer))
	}
	// use a out of range index to trigger error
	policyEnvelope := policydsl.Envelope(policydsl.Or(policydsl.SignedBy(0), policydsl.SignedBy(badIndex)), data)
	return createCollectionPolicyConfig(policyEnvelope)
}
