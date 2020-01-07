/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"testing"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric/common/chaincode"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/common/policies/inquire"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/assert"
)

type mockMetadataRetriever struct {
	res *chaincode.Metadata
}

func (r *mockMetadataRetriever) Metadata(channel string, cc string, collections ...string) *chaincode.Metadata {
	return r.res
}

func TestSupport(t *testing.T) {
	emptySignaturePolicyEnvelope := &common.SignaturePolicyEnvelope{}
	ccmd1 := &chaincode.Metadata{Policy: protoutil.MarshalOrPanic(emptySignaturePolicyEnvelope)}
	notEmptySignaturePolicyEnvelope := &common.SignaturePolicyEnvelope{
		Rule:       &common.SignaturePolicy{},
		Identities: []*msp.MSPPrincipal{{Principal: []byte("principal-1")}},
	}
	ccmd2 := &chaincode.Metadata{Policy: protoutil.MarshalOrPanic(notEmptySignaturePolicyEnvelope)}
	notEmptySignaturePolicyEnvelope2 := &common.SignaturePolicyEnvelope{
		Rule:       &common.SignaturePolicy{},
		Identities: []*msp.MSPPrincipal{{Principal: []byte("principal-2")}},
	}
	ccmd3 := &chaincode.Metadata{Policy: protoutil.MarshalOrPanic(notEmptySignaturePolicyEnvelope),
		CollectionPolicies: map[string][]byte{"col1": protoutil.MarshalOrPanic(notEmptySignaturePolicyEnvelope2)}}

	tests := []struct {
		name           string
		input          *chaincode.Metadata
		expectedReturn []policies.InquireablePolicy
	}{
		{
			name:           "Nil instantiatedChaincode",
			input:          nil,
			expectedReturn: nil,
		},
		{
			name:           "Invalid policy bytes",
			input:          &chaincode.Metadata{Policy: []byte{1, 2, 3}},
			expectedReturn: nil,
		},
		{
			name:           "Empty signature policy envelope",
			input:          ccmd1,
			expectedReturn: nil,
		},
		{
			name:           "Not Empty signature policy envelope",
			input:          ccmd2,
			expectedReturn: []policies.InquireablePolicy{inquire.NewInquireableSignaturePolicy(notEmptySignaturePolicyEnvelope)},
		},
		{
			name:           "Not Empty signature policy envelopes with existing collection policy",
			input:          ccmd3,
			expectedReturn: []policies.InquireablePolicy{inquire.NewInquireableSignaturePolicy(notEmptySignaturePolicyEnvelope2)},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			sup := NewDiscoverySupport(&mockMetadataRetriever{res: test.input})
			var res []policies.InquireablePolicy
			if test.input == ccmd3 {
				res = sup.PoliciesByChaincode("", "", "col1")
			} else {
				res = sup.PoliciesByChaincode("", "")
			}
			if test.expectedReturn == nil {
				assert.Nil(t, res)
			} else {
				assert.Equal(t, len(res), 1)
				assert.Equal(t, res[0].SatisfiedBy(), test.expectedReturn[0].SatisfiedBy())
			}
		})
	}
}
