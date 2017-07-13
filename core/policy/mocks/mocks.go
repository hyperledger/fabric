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

package mocks

import (
	"bytes"

	"fmt"

	"errors"

	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protos/common"
	mspproto "github.com/hyperledger/fabric/protos/msp"
)

type MockChannelPolicyManagerGetter struct {
	Managers map[string]policies.Manager
}

func (c *MockChannelPolicyManagerGetter) Manager(channelID string) (policies.Manager, bool) {
	return c.Managers[channelID], true
}

type MockChannelPolicyManager struct {
	MockPolicy policies.Policy
}

func (m *MockChannelPolicyManager) GetPolicy(id string) (policies.Policy, bool) {
	return m.MockPolicy, true
}

func (m *MockChannelPolicyManager) Manager(path []string) (policies.Manager, bool) {
	panic("Not implemented")
}

func (m *MockChannelPolicyManager) BasePath() string {
	panic("Not implemented")
}

func (m *MockChannelPolicyManager) PolicyNames() []string {
	panic("Not implemented")
}

type MockPolicy struct {
	Deserializer msp.IdentityDeserializer
}

// Evaluate takes a set of SignedData and evaluates whether this set of signatures satisfies the policy
func (m *MockPolicy) Evaluate(signatureSet []*common.SignedData) error {
	fmt.Printf("Evaluate [%s], [% x], [% x]\n", string(signatureSet[0].Identity), string(signatureSet[0].Data), string(signatureSet[0].Signature))
	identity, err := m.Deserializer.DeserializeIdentity(signatureSet[0].Identity)
	if err != nil {
		return err
	}

	return identity.Verify(signatureSet[0].Data, signatureSet[0].Signature)
}

type MockIdentityDeserializer struct {
	Identity []byte
	Msg      []byte
}

func (d *MockIdentityDeserializer) DeserializeIdentity(serializedIdentity []byte) (msp.Identity, error) {
	fmt.Printf("[DeserializeIdentity] id : [%s], [%s]\n", string(serializedIdentity), string(d.Identity))
	if bytes.Equal(d.Identity, serializedIdentity) {
		fmt.Printf("GOT : [%s], [%s]\n", string(serializedIdentity), string(d.Identity))
		return &MockIdentity{identity: d.Identity, msg: d.Msg}, nil
	}

	return nil, errors.New("Invalid Identity")
}

type MockIdentity struct {
	identity []byte
	msg      []byte
}

func (id *MockIdentity) SatisfiesPrincipal(p *mspproto.MSPPrincipal) error {
	fmt.Printf("[SatisfiesPrincipal] id : [%s], [%s]\n", string(id.identity), string(p.Principal))
	if !bytes.Equal(id.identity, p.Principal) {
		return fmt.Errorf("Different identities [% x]!=[% x]", id.identity, p.Principal)
	}
	return nil
}

func (id *MockIdentity) GetIdentifier() *msp.IdentityIdentifier {
	return &msp.IdentityIdentifier{Mspid: "mock", Id: "mock"}
}

func (id *MockIdentity) GetMSPIdentifier() string {
	return "mock"
}

func (id *MockIdentity) Validate() error {
	return nil
}

func (id *MockIdentity) GetOrganizationalUnits() []*msp.OUIdentifier {
	return nil
}

func (id *MockIdentity) Verify(msg []byte, sig []byte) error {
	fmt.Printf("VERIFY [% x], [% x], [% x]\n", string(id.msg), string(msg), string(sig))
	if bytes.Equal(id.msg, msg) {
		if bytes.Equal(msg, sig) {
			return nil
		}
	}

	return errors.New("Invalid Signature")
}

func (id *MockIdentity) Serialize() ([]byte, error) {
	return []byte("cert"), nil
}

type MockMSPPrincipalGetter struct {
	Principal []byte
}

func (m *MockMSPPrincipalGetter) Get(role string) (*mspproto.MSPPrincipal, error) {
	return &mspproto.MSPPrincipal{Principal: m.Principal}, nil
}
