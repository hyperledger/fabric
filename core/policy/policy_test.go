/*
Copyright IBM Corp. 2017 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package policy

import (
	"testing"

	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/stretchr/testify/assert"
)

func TestPolicyChecker(t *testing.T) {
	policyManagerGetter := &mockChannelPolicyManagerGetter{
		map[string]policies.Manager{
			"A": &mockChannelPolicyManager{&mockPolicy{&mockIdentityDeserializer{[]byte("Alice"), []byte("msg1")}}},
			"B": &mockChannelPolicyManager{&mockPolicy{&mockIdentityDeserializer{[]byte("Bob"), []byte("msg2")}}},
			"C": &mockChannelPolicyManager{&mockPolicy{&mockIdentityDeserializer{[]byte("Alice"), []byte("msg3")}}},
		},
	}
	identityDeserializer := &mockIdentityDeserializer{[]byte("Alice"), []byte("msg1")}
	pc := NewPolicyChecker(
		policyManagerGetter,
		identityDeserializer,
		&mockMSPPrincipalGetter{Principal: []byte("Alice")},
	)

	// Check that (non-empty channel, empty policy) fails
	err := pc.CheckPolicy("A", "", nil)
	assert.Error(t, err)

	// Check that (empty channel, empty policy) fails
	err = pc.CheckPolicy("", "", nil)
	assert.Error(t, err)

	// Check that (non-empty channel, non-empty policy, nil proposal) fails
	err = pc.CheckPolicy("A", "A", nil)
	assert.Error(t, err)

	// Check that (empty channel, non-empty policy, nil proposal) fails
	err = pc.CheckPolicy("", "A", nil)
	assert.Error(t, err)

	// Validate Alice signatures against channel A's readers
	sProp, _ := utils.MockSignedEndorserProposalOrPanic("A", &peer.ChaincodeSpec{}, []byte("Alice"), []byte("msg1"))
	policyManagerGetter.managers["A"].(*mockChannelPolicyManager).mockPolicy.(*mockPolicy).deserializer.(*mockIdentityDeserializer).msg = sProp.ProposalBytes
	sProp.Signature = sProp.ProposalBytes
	err = pc.CheckPolicy("A", "readers", sProp)
	assert.NoError(t, err)

	// Proposal from Alice for channel A should fail against channel B, where Alice is not involved
	err = pc.CheckPolicy("B", "readers", sProp)
	assert.Error(t, err)

	// Proposal from Alice for channel A should fail against channel C, where Alice is involved but signature is not valid
	err = pc.CheckPolicy("C", "readers", sProp)
	assert.Error(t, err)

	// Alice is a member of the local MSP, policy check must succeed
	identityDeserializer.msg = sProp.ProposalBytes
	err = pc.CheckPolicyNoChannel("member", sProp)
	assert.NoError(t, err)

	sProp, _ = utils.MockSignedEndorserProposalOrPanic("A", &peer.ChaincodeSpec{}, []byte("Bob"), []byte("msg2"))
	// Bob is not a member of the local MSP, policy check must fail
	err = pc.CheckPolicyNoChannel("member", sProp)
	assert.Error(t, err)
}
