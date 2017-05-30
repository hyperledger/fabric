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
	"github.com/hyperledger/fabric/core/policy/mocks"
	"github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestCheckPolicyInvalidArgs(t *testing.T) {
	policyManagerGetter := &mocks.MockChannelPolicyManagerGetter{
		Managers: map[string]policies.Manager{
			"A": &mocks.MockChannelPolicyManager{&mocks.MockPolicy{&mocks.MockIdentityDeserializer{[]byte("Alice"), []byte("msg1")}}},
		},
	}
	pc := &policyChecker{channelPolicyManagerGetter: policyManagerGetter}

	err := pc.CheckPolicy("B", "admin", &peer.SignedProposal{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed to get policy manager for channel [B]")
}

func TestRegisterPolicyCheckerFactoryInvalidArgs(t *testing.T) {
	RegisterPolicyCheckerFactory(nil)
	assert.Panics(t, func() {
		GetPolicyChecker()
	})

	RegisterPolicyCheckerFactory(nil)
}

func TestRegisterPolicyCheckerFactory(t *testing.T) {
	policyManagerGetter := &mocks.MockChannelPolicyManagerGetter{
		Managers: map[string]policies.Manager{
			"A": &mocks.MockChannelPolicyManager{&mocks.MockPolicy{&mocks.MockIdentityDeserializer{[]byte("Alice"), []byte("msg1")}}},
		},
	}
	pc := &policyChecker{channelPolicyManagerGetter: policyManagerGetter}

	factory := &MockPolicyCheckerFactory{}
	factory.On("NewPolicyChecker").Return(pc)

	RegisterPolicyCheckerFactory(factory)
	pc2 := GetPolicyChecker()
	assert.Equal(t, pc, pc2)
}

func TestCheckPolicyBySignedDataInvalidArgs(t *testing.T) {
	policyManagerGetter := &mocks.MockChannelPolicyManagerGetter{
		Managers: map[string]policies.Manager{
			"A": &mocks.MockChannelPolicyManager{&mocks.MockPolicy{&mocks.MockIdentityDeserializer{[]byte("Alice"), []byte("msg1")}}},
		},
	}
	pc := &policyChecker{channelPolicyManagerGetter: policyManagerGetter}

	err := pc.CheckPolicyBySignedData("", "admin", []*common.SignedData{&common.SignedData{}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid channel ID name during check policy on signed data. Name must be different from nil.")

	err = pc.CheckPolicyBySignedData("A", "", []*common.SignedData{&common.SignedData{}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid policy name during check policy on signed data on channel [A]. Name must be different from nil.")

	err = pc.CheckPolicyBySignedData("A", "admin", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid signed data during check policy on channel [A] with policy [admin]")

	err = pc.CheckPolicyBySignedData("B", "admin", []*common.SignedData{&common.SignedData{}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed to get policy manager for channel [B]")

	err = pc.CheckPolicyBySignedData("A", "admin", []*common.SignedData{&common.SignedData{}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed evaluating policy on signed data during check policy on channel [A] with policy [admin]")
}

func TestPolicyCheckerInvalidArgs(t *testing.T) {
	policyManagerGetter := &mocks.MockChannelPolicyManagerGetter{
		map[string]policies.Manager{
			"A": &mocks.MockChannelPolicyManager{&mocks.MockPolicy{&mocks.MockIdentityDeserializer{[]byte("Alice"), []byte("msg1")}}},
			"B": &mocks.MockChannelPolicyManager{&mocks.MockPolicy{&mocks.MockIdentityDeserializer{[]byte("Bob"), []byte("msg2")}}},
			"C": &mocks.MockChannelPolicyManager{&mocks.MockPolicy{&mocks.MockIdentityDeserializer{[]byte("Alice"), []byte("msg3")}}},
		},
	}
	identityDeserializer := &mocks.MockIdentityDeserializer{[]byte("Alice"), []byte("msg1")}
	pc := NewPolicyChecker(
		policyManagerGetter,
		identityDeserializer,
		&mocks.MockMSPPrincipalGetter{Principal: []byte("Alice")},
	)

	// Check that (non-empty channel, empty policy) fails
	err := pc.CheckPolicy("A", "", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid policy name during check policy on channel [A]. Name must be different from nil.")

	// Check that (empty channel, empty policy) fails
	err = pc.CheckPolicy("", "", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid policy name during channelless check policy. Name must be different from nil.")

	// Check that (non-empty channel, non-empty policy, nil proposal) fails
	err = pc.CheckPolicy("A", "A", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid signed proposal during check policy on channel [A] with policy [A]")

	// Check that (empty channel, non-empty policy, nil proposal) fails
	err = pc.CheckPolicy("", "A", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid signed proposal during channelless check policy with policy [A]")
}

func TestPolicyChecker(t *testing.T) {
	policyManagerGetter := &mocks.MockChannelPolicyManagerGetter{
		map[string]policies.Manager{
			"A": &mocks.MockChannelPolicyManager{&mocks.MockPolicy{&mocks.MockIdentityDeserializer{[]byte("Alice"), []byte("msg1")}}},
			"B": &mocks.MockChannelPolicyManager{&mocks.MockPolicy{&mocks.MockIdentityDeserializer{[]byte("Bob"), []byte("msg2")}}},
			"C": &mocks.MockChannelPolicyManager{&mocks.MockPolicy{&mocks.MockIdentityDeserializer{[]byte("Alice"), []byte("msg3")}}},
		},
	}
	identityDeserializer := &mocks.MockIdentityDeserializer{[]byte("Alice"), []byte("msg1")}
	pc := NewPolicyChecker(
		policyManagerGetter,
		identityDeserializer,
		&mocks.MockMSPPrincipalGetter{Principal: []byte("Alice")},
	)

	// Validate Alice signatures against channel A's readers
	sProp, _ := utils.MockSignedEndorserProposalOrPanic("A", &peer.ChaincodeSpec{}, []byte("Alice"), []byte("msg1"))
	policyManagerGetter.Managers["A"].(*mocks.MockChannelPolicyManager).MockPolicy.(*mocks.MockPolicy).Deserializer.(*mocks.MockIdentityDeserializer).Msg = sProp.ProposalBytes
	sProp.Signature = sProp.ProposalBytes
	err := pc.CheckPolicy("A", "readers", sProp)
	assert.NoError(t, err)

	// Proposal from Alice for channel A should fail against channel B, where Alice is not involved
	err = pc.CheckPolicy("B", "readers", sProp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed evaluating policy on signed data during check policy on channel [B] with policy [readers]: [Invalid Identity]")

	// Proposal from Alice for channel A should fail against channel C, where Alice is involved but signature is not valid
	err = pc.CheckPolicy("C", "readers", sProp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed evaluating policy on signed data during check policy on channel [C] with policy [readers]: [Invalid Signature]")

	// Alice is a member of the local MSP, policy check must succeed
	identityDeserializer.Msg = sProp.ProposalBytes
	err = pc.CheckPolicyNoChannel(mgmt.Members, sProp)
	assert.NoError(t, err)

	sProp, _ = utils.MockSignedEndorserProposalOrPanic("A", &peer.ChaincodeSpec{}, []byte("Bob"), []byte("msg2"))
	// Bob is not a member of the local MSP, policy check must fail
	err = pc.CheckPolicyNoChannel(mgmt.Members, sProp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed deserializing proposal creator during channelless check policy with policy [Members]: [Invalid Identity]")
}

type MockPolicyCheckerFactory struct {
	mock.Mock
}

func (m *MockPolicyCheckerFactory) NewPolicyChecker() PolicyChecker {
	args := m.Called()
	return args.Get(0).(PolicyChecker)
}
