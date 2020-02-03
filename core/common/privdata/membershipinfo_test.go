/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privdata

import (
	"testing"

	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/stretchr/testify/assert"
)

func TestMembershipInfoProvider(t *testing.T) {
	mspID := "peer0"
	peerSelfSignedData := common.SignedData{
		Identity:  []byte("peer0"),
		Signature: []byte{1, 2, 3},
		Data:      []byte{4, 5, 6},
	}

	identityDeserializer := func(chainID string) msp.IdentityDeserializer {
		return &mockDeserializer{}
	}

	// verify membership provider returns true
	membershipProvider := NewMembershipInfoProvider(mspID, peerSelfSignedData, identityDeserializer)
	res, err := membershipProvider.AmMemberOf("test1", getAccessPolicy([]string{"peer0", "peer1"}))
	assert.True(t, res)
	assert.Nil(t, err)

	// verify membership provider returns false
	res, err = membershipProvider.AmMemberOf("test1", getAccessPolicy([]string{"peer2", "peer3"}))
	assert.False(t, res)
	assert.Nil(t, err)

	// verify membership provider returns false and nil when collection policy config is nil
	res, err = membershipProvider.AmMemberOf("test1", nil)
	assert.False(t, res)
	assert.Nil(t, err)

	// verify membership provider returns false and nil when collection policy config is invalid
	res, err = membershipProvider.AmMemberOf("test1", getBadAccessPolicy([]string{"signer0"}, 1))
	assert.False(t, res)
	assert.Nil(t, err)
}

func getAccessPolicy(signers []string) *common.CollectionPolicyConfig {
	var data [][]byte
	for _, signer := range signers {
		data = append(data, []byte(signer))
	}
	policyEnvelope := cauthdsl.Envelope(cauthdsl.Or(cauthdsl.SignedBy(0), cauthdsl.SignedBy(1)), data)
	return createCollectionPolicyConfig(policyEnvelope)
}

func getBadAccessPolicy(signers []string, badIndex int32) *common.CollectionPolicyConfig {
	var data [][]byte
	for _, signer := range signers {
		data = append(data, []byte(signer))
	}
	// use a out of range index to trigger error
	policyEnvelope := cauthdsl.Envelope(cauthdsl.Or(cauthdsl.SignedBy(0), cauthdsl.SignedBy(badIndex)), data)
	return createCollectionPolicyConfig(policyEnvelope)
}
