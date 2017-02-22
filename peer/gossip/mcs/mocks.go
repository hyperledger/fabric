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

package mcs

import (
	"bytes"

	"fmt"

	"errors"

	mockpolicies "github.com/hyperledger/fabric/common/mocks/policies"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protos/common"
)

type MockChannelPolicyManagerGetter struct{}

func (c *MockChannelPolicyManagerGetter) Manager(channelID string) (policies.Manager, bool) {
	return &mockpolicies.Manager{Policy: &mockpolicies.Policy{Err: nil}}, false
}

type mockChannelPolicyManagerGetter2 struct {
	managers map[string]policies.Manager
}

func (c *mockChannelPolicyManagerGetter2) Manager(channelID string) (policies.Manager, bool) {
	return c.managers[channelID], true
}

type mockChannelPolicyManager struct {
	mockPolicy policies.Policy
}

func (m *mockChannelPolicyManager) GetPolicy(id string) (policies.Policy, bool) {
	return m.mockPolicy, true
}

func (m *mockChannelPolicyManager) Manager(path []string) (policies.Manager, bool) {
	panic("Not implemented")
}

func (m *mockChannelPolicyManager) BasePath() string {
	panic("Not implemented")
}

func (m *mockChannelPolicyManager) PolicyNames() []string {
	panic("Not implemented")
}

type mockPolicy struct {
	deserializer msp.IdentityDeserializer
}

// Evaluate takes a set of SignedData and evaluates whether this set of signatures satisfies the policy
func (m *mockPolicy) Evaluate(signatureSet []*common.SignedData) error {
	fmt.Printf("Evaluate [%s], [% x], [% x]\n", string(signatureSet[0].Identity), string(signatureSet[0].Data), string(signatureSet[0].Signature))
	identity, err := m.deserializer.DeserializeIdentity(signatureSet[0].Identity)
	if err != nil {
		return err
	}

	return identity.Verify(signatureSet[0].Data, signatureSet[0].Signature)
}

type mockDeserializersManager struct {
	localDeserializer    msp.IdentityDeserializer
	channelDeserializers map[string]msp.IdentityDeserializer
}

func (m *mockDeserializersManager) GetLocalMSPIdentifier() string {
	return "mock"
}

func (m *mockDeserializersManager) GetLocalDeserializer() msp.IdentityDeserializer {
	return m.localDeserializer
}

func (m *mockDeserializersManager) GetChannelDeserializers() map[string]msp.IdentityDeserializer {
	return m.channelDeserializers
}

type mockIdentityDeserializer struct {
	identity []byte
	msg      []byte
}

func (d *mockIdentityDeserializer) DeserializeIdentity(serializedIdentity []byte) (msp.Identity, error) {
	fmt.Printf("id : [%s], [%s]\n", string(serializedIdentity), string(d.identity))
	if bytes.Equal(d.identity, serializedIdentity) {
		fmt.Printf("GOT : [%s], [%s]\n", string(serializedIdentity), string(d.identity))
		return &mockIdentity{msg: d.msg}, nil
	}

	return nil, errors.New("Invalid identity")
}

type mockIdentity struct {
	msg []byte
}

func (id *mockIdentity) SatisfiesPrincipal(*common.MSPPrincipal) error {
	return nil
}

func (id *mockIdentity) GetIdentifier() *msp.IdentityIdentifier {
	return &msp.IdentityIdentifier{Mspid: "mock", Id: "mock"}
}

func (id *mockIdentity) GetMSPIdentifier() string {
	return "mock"
}

func (id *mockIdentity) Validate() error {
	return nil
}

func (id *mockIdentity) GetOrganizationalUnits() []string {
	return []string{"dunno"}
}

func (id *mockIdentity) Verify(msg []byte, sig []byte) error {
	fmt.Printf("VERIFY [% x], [% x], [% x]\n", string(id.msg), string(msg), string(sig))
	if bytes.Equal(id.msg, msg) {
		if bytes.Equal(msg, sig) {
			return nil
		}
	}

	return errors.New("Invalid Signature")
}

func (id *mockIdentity) VerifyOpts(msg []byte, sig []byte, opts msp.SignatureOpts) error {
	return nil
}

func (id *mockIdentity) VerifyAttributes(proof []byte, spec *msp.AttributeProofSpec) error {
	return nil
}

func (id *mockIdentity) Serialize() ([]byte, error) {
	return []byte("cert"), nil
}
