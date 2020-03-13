/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mocks

import (
	"bytes"
	"errors"
	"fmt"
	"time"

	mspproto "github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/mock"
)

type ChannelPolicyManagerGetter struct{}

func (c *ChannelPolicyManagerGetter) Manager(channelID string) policies.Manager {
	policyMgr := &PolicyManager{}
	policyMgr.GetPolicyReturns(nil, true)
	return policyMgr
}

type ChannelPolicyManagerGetterWithManager struct {
	Managers map[string]policies.Manager
}

func (c *ChannelPolicyManagerGetterWithManager) Manager(channelID string) policies.Manager {
	return c.Managers[channelID]
}

type ChannelPolicyManager struct {
	Policy policies.Policy
}

func (m *ChannelPolicyManager) GetPolicy(id string) (policies.Policy, bool) {
	return m.Policy, true
}

func (m *ChannelPolicyManager) Manager(path []string) (policies.Manager, bool) {
	panic("Not implemented")
}

type Policy struct {
	Deserializer msp.IdentityDeserializer
}

// EvaluateSignedData takes a set of SignedData and evaluates whether this set of signatures satisfies the policy
func (m *Policy) EvaluateSignedData(signatureSet []*protoutil.SignedData) error {
	fmt.Printf("Evaluate [%s], [% x], [% x]\n", string(signatureSet[0].Identity), string(signatureSet[0].Data), string(signatureSet[0].Signature))
	identity, err := m.Deserializer.DeserializeIdentity(signatureSet[0].Identity)
	if err != nil {
		return err
	}

	return identity.Verify(signatureSet[0].Data, signatureSet[0].Signature)
}

// EvaluateIdentities takes an array of identities and evaluates whether
// they satisfy the policy
func (m *Policy) EvaluateIdentities(identities []msp.Identity) error {
	panic("Implement me")
}

type DeserializersManager struct {
	LocalDeserializer    msp.IdentityDeserializer
	ChannelDeserializers map[string]msp.IdentityDeserializer
}

func (m *DeserializersManager) Deserialize(raw []byte) (*mspproto.SerializedIdentity, error) {
	return &mspproto.SerializedIdentity{Mspid: "mock", IdBytes: raw}, nil
}

func (m *DeserializersManager) GetLocalMSPIdentifier() string {
	return "mock"
}

func (m *DeserializersManager) GetLocalDeserializer() msp.IdentityDeserializer {
	return m.LocalDeserializer
}

func (m *DeserializersManager) GetChannelDeserializers() map[string]msp.IdentityDeserializer {
	return m.ChannelDeserializers
}

type IdentityDeserializerWithExpiration struct {
	*IdentityDeserializer
	Expiration time.Time
}

func (d *IdentityDeserializerWithExpiration) DeserializeIdentity(serializedIdentity []byte) (msp.Identity, error) {
	id, err := d.IdentityDeserializer.DeserializeIdentity(serializedIdentity)
	if err != nil {
		return nil, err
	}
	id.(*Identity).expirationDate = d.Expiration
	return id, nil
}

type IdentityDeserializer struct {
	Identity []byte
	Msg      []byte
	mock.Mock
}

func (d *IdentityDeserializer) DeserializeIdentity(serializedIdentity []byte) (msp.Identity, error) {
	fmt.Printf("id : [%s], [%s]\n", string(serializedIdentity), string(d.Identity))
	if bytes.Equal(d.Identity, serializedIdentity) {
		fmt.Printf("GOT : [%s], [%s]\n", string(serializedIdentity), string(d.Identity))
		return &Identity{Msg: d.Msg}, nil
	}

	return nil, errors.New("Invalid Identity")
}

func (d *IdentityDeserializer) IsWellFormed(identity *mspproto.SerializedIdentity) error {
	if len(d.ExpectedCalls) == 0 {
		return nil
	}
	return d.Called(identity).Error(0)
}

type Identity struct {
	expirationDate time.Time
	Msg            []byte
}

func (id *Identity) Anonymous() bool {
	panic("implement me")
}

func (id *Identity) ExpiresAt() time.Time {
	return id.expirationDate
}

func (id *Identity) SatisfiesPrincipal(*mspproto.MSPPrincipal) error {
	return nil
}

func (id *Identity) GetIdentifier() *msp.IdentityIdentifier {
	return &msp.IdentityIdentifier{Mspid: "mock", Id: "mock"}
}

func (id *Identity) GetMSPIdentifier() string {
	return "mock"
}

func (id *Identity) Validate() error {
	return nil
}

func (id *Identity) GetOrganizationalUnits() []*msp.OUIdentifier {
	return nil
}

func (id *Identity) Verify(msg []byte, sig []byte) error {
	fmt.Printf("VERIFY [% x], [% x], [% x]\n", string(id.Msg), string(msg), string(sig))
	if bytes.Equal(id.Msg, msg) {
		if bytes.Equal(msg, sig) {
			return nil
		}
	}

	return errors.New("Invalid Signature")
}

func (id *Identity) Serialize() ([]byte, error) {
	return []byte("cert"), nil
}
