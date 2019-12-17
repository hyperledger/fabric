/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cauthdsl

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	mb "github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric/msp"
	"github.com/stretchr/testify/assert"
)

var invalidSignature = []byte("badsigned")

type mockIdentity struct {
	idBytes []byte
}

func (id *mockIdentity) Anonymous() bool {
	panic("implement me")
}

func (id *mockIdentity) ExpiresAt() time.Time {
	return time.Time{}
}

func (id *mockIdentity) SatisfiesPrincipal(p *mb.MSPPrincipal) error {
	if !bytes.Equal(id.idBytes, p.Principal) {
		return errors.New("Principals do not match")
	}
	return nil
}

func (id *mockIdentity) GetIdentifier() *msp.IdentityIdentifier {
	return &msp.IdentityIdentifier{Mspid: "Mock", Id: string(id.idBytes)}
}

func (id *mockIdentity) GetMSPIdentifier() string {
	return "Mock"
}

func (id *mockIdentity) Validate() error {
	return nil
}

func (id *mockIdentity) GetOrganizationalUnits() []*msp.OUIdentifier {
	return nil
}

func (id *mockIdentity) Verify(msg []byte, sig []byte) error {
	if bytes.Equal(sig, invalidSignature) {
		return errors.New("Invalid signature")
	}
	return nil
}

func (id *mockIdentity) Serialize() ([]byte, error) {
	return id.idBytes, nil
}

func toIdentities(idBytesSlice [][]byte, deserializer msp.IdentityDeserializer) ([]msp.Identity, []bool) {
	identities := make([]msp.Identity, len(idBytesSlice))
	for i, idBytes := range idBytesSlice {
		id, _ := deserializer.DeserializeIdentity(idBytes)
		identities[i] = id
	}

	return identities, make([]bool, len(idBytesSlice))
}

type mockDeserializer struct {
	fail error
}

func (md *mockDeserializer) IsWellFormed(_ *mb.SerializedIdentity) error {
	return nil
}

func (md *mockDeserializer) DeserializeIdentity(serializedIdentity []byte) (msp.Identity, error) {
	if md.fail != nil {
		return nil, md.fail
	}
	return &mockIdentity{idBytes: serializedIdentity}, nil
}

var validSignature = []byte("signed")
var signers = [][]byte{[]byte("signer0"), []byte("signer1")}
var msgs = [][]byte{nil, nil}
var moreMsgs = [][]byte{nil, nil, nil}

func TestSimpleSignature(t *testing.T) {
	policy := Envelope(SignedBy(0), signers)

	spe, err := compile(policy.Rule, policy.Identities)
	if err != nil {
		t.Fatalf("Could not create a new SignaturePolicyEvaluator using the given policy, crypto-helper: %s", err)
	}

	if !spe(toIdentities([][]byte{signers[0]}, &mockDeserializer{})) {
		t.Errorf("Expected authentication to succeed with valid signatures")
	}
	if spe(toIdentities([][]byte{signers[1]}, &mockDeserializer{})) {
		t.Errorf("Expected authentication to fail because signers[1] is not authorized in the policy, despite his valid signature")
	}
}

func TestMultipleSignature(t *testing.T) {
	policy := Envelope(And(SignedBy(0), SignedBy(1)), signers)

	spe, err := compile(policy.Rule, policy.Identities)
	if err != nil {
		t.Fatalf("Could not create a new SignaturePolicyEvaluator using the given policy, crypto-helper: %s", err)
	}

	if !spe(toIdentities(signers, &mockDeserializer{})) {
		t.Errorf("Expected authentication to succeed with  valid signatures")
	}
	if spe(toIdentities([][]byte{signers[0], signers[0]}, &mockDeserializer{})) {
		t.Errorf("Expected authentication to fail because although there were two valid signatures, one was duplicated")
	}
}

func TestComplexNestedSignature(t *testing.T) {
	policy := Envelope(And(Or(And(SignedBy(0), SignedBy(1)), And(SignedBy(0), SignedBy(0))), SignedBy(0)), signers)

	spe, err := compile(policy.Rule, policy.Identities)
	if err != nil {
		t.Fatalf("Could not create a new SignaturePolicyEvaluator using the given policy, crypto-helper: %s", err)
	}

	if !spe(toIdentities(append(signers, [][]byte{[]byte("signer0")}...), &mockDeserializer{})) {
		t.Errorf("Expected authentication to succeed with valid signatures")
	}
	if !spe(toIdentities([][]byte{[]byte("signer0"), []byte("signer0"), []byte("signer0")}, &mockDeserializer{})) {
		t.Errorf("Expected authentication to succeed with valid signatures")
	}
	if spe(toIdentities(signers, &mockDeserializer{})) {
		t.Errorf("Expected authentication to fail with too few signatures")
	}
	if spe(toIdentities(append(signers, [][]byte{[]byte("signer1")}...), &mockDeserializer{})) {
		t.Errorf("Expected authentication failure as there was a signature from signer[0] missing")
	}
}

func TestNegatively(t *testing.T) {
	rpolicy := Envelope(And(SignedBy(0), SignedBy(1)), signers)
	rpolicy.Rule.Type = nil
	b, _ := proto.Marshal(rpolicy)
	policy := &cb.SignaturePolicyEnvelope{}
	_ = proto.Unmarshal(b, policy)
	_, err := compile(policy.Rule, policy.Identities)
	if err == nil {
		t.Fatal("Should have errored compiling because the Type field was nil")
	}
}

func TestNilSignaturePolicyEnvelope(t *testing.T) {
	_, err := compile(nil, nil)
	assert.Error(t, err, "Fail to compile")
}

func TestSignedByMspClient(t *testing.T) {
	e := SignedByMspClient("A")
	assert.Equal(t, 1, len(e.Identities))

	role := &mb.MSPRole{}
	err := proto.Unmarshal(e.Identities[0].Principal, role)
	assert.NoError(t, err)

	assert.Equal(t, role.MspIdentifier, "A")
	assert.Equal(t, role.Role, mb.MSPRole_CLIENT)

	e = SignedByAnyClient([]string{"A"})
	assert.Equal(t, 1, len(e.Identities))

	role = &mb.MSPRole{}
	err = proto.Unmarshal(e.Identities[0].Principal, role)
	assert.NoError(t, err)

	assert.Equal(t, role.MspIdentifier, "A")
	assert.Equal(t, role.Role, mb.MSPRole_CLIENT)
}

func TestSignedByMspPeer(t *testing.T) {
	e := SignedByMspPeer("A")
	assert.Equal(t, 1, len(e.Identities))

	role := &mb.MSPRole{}
	err := proto.Unmarshal(e.Identities[0].Principal, role)
	assert.NoError(t, err)

	assert.Equal(t, role.MspIdentifier, "A")
	assert.Equal(t, role.Role, mb.MSPRole_PEER)

	e = SignedByAnyPeer([]string{"A"})
	assert.Equal(t, 1, len(e.Identities))

	role = &mb.MSPRole{}
	err = proto.Unmarshal(e.Identities[0].Principal, role)
	assert.NoError(t, err)

	assert.Equal(t, role.MspIdentifier, "A")
	assert.Equal(t, role.Role, mb.MSPRole_PEER)
}

func TestReturnNil(t *testing.T) {
	policy := Envelope(And(SignedBy(-1), SignedBy(-2)), signers)

	spe, err := compile(policy.Rule, policy.Identities)
	assert.Nil(t, spe)
	assert.EqualError(t, err, "identity index out of range, requested -1, but identities length is 2")
}
