/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cauthdsl

import (
	"bytes"
	"errors"
	"testing"

	"github.com/hyperledger/fabric/msp"
	cb "github.com/hyperledger/fabric/protos/common"
	mb "github.com/hyperledger/fabric/protos/msp"

	"github.com/golang/protobuf/proto"
	logging "github.com/op/go-logging"
	"github.com/stretchr/testify/assert"
)

func init() {
	logging.SetLevel(logging.DEBUG, "")
}

var invalidSignature = []byte("badsigned")

type mockIdentity struct {
	idBytes []byte
}

func (id *mockIdentity) SatisfiesPrincipal(p *mb.MSPPrincipal) error {
	if bytes.Compare(id.idBytes, p.Principal) == 0 {
		return nil
	} else {
		return errors.New("Principals do not match")
	}
}

func (id *mockIdentity) GetIdentifier() *msp.IdentityIdentifier {
	return &msp.IdentityIdentifier{Mspid: "Mock", Id: "Bob"}
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
	if bytes.Compare(sig, invalidSignature) == 0 {
		return errors.New("Invalid signature")
	} else {
		return nil
	}
}

func (id *mockIdentity) Serialize() ([]byte, error) {
	return id.idBytes, nil
}

func toSignedData(data [][]byte, identities [][]byte, signatures [][]byte) ([]*cb.SignedData, []bool) {
	signedData := make([]*cb.SignedData, len(data))
	for i := range signedData {
		signedData[i] = &cb.SignedData{
			Data:      data[i],
			Identity:  identities[i],
			Signature: signatures[i],
		}
	}
	return signedData, make([]bool, len(signedData))
}

type mockDeserializer struct {
}

func (md *mockDeserializer) DeserializeIdentity(serializedIdentity []byte) (msp.Identity, error) {
	return &mockIdentity{idBytes: serializedIdentity}, nil
}

var validSignature = []byte("signed")
var signers = [][]byte{[]byte("signer0"), []byte("signer1")}
var msgs = [][]byte{nil, nil}
var moreMsgs = [][]byte{nil, nil, nil}

func TestSimpleSignature(t *testing.T) {
	policy := Envelope(SignedBy(0), signers)

	spe, err := compile(policy.Rule, policy.Identities, &mockDeserializer{})
	if err != nil {
		t.Fatalf("Could not create a new SignaturePolicyEvaluator using the given policy, crypto-helper: %s", err)
	}

	if !spe(toSignedData([][]byte{nil}, [][]byte{signers[0]}, [][]byte{validSignature})) {
		t.Errorf("Expected authentication to succeed with valid signatures")
	}
	if spe(toSignedData([][]byte{nil}, [][]byte{signers[0]}, [][]byte{invalidSignature})) {
		t.Errorf("Expected authentication to fail given the invalid signature")
	}
	if spe(toSignedData([][]byte{nil}, [][]byte{signers[1]}, [][]byte{validSignature})) {
		t.Errorf("Expected authentication to fail because signers[1] is not authorized in the policy, despite his valid signature")
	}
}

func TestMultipleSignature(t *testing.T) {
	policy := Envelope(And(SignedBy(0), SignedBy(1)), signers)

	spe, err := compile(policy.Rule, policy.Identities, &mockDeserializer{})
	if err != nil {
		t.Fatalf("Could not create a new SignaturePolicyEvaluator using the given policy, crypto-helper: %s", err)
	}

	if !spe(toSignedData(msgs, signers, [][]byte{validSignature, validSignature})) {
		t.Errorf("Expected authentication to succeed with  valid signatures")
	}
	if spe(toSignedData(msgs, signers, [][]byte{validSignature, invalidSignature})) {
		t.Errorf("Expected authentication to fail given one of two invalid signatures")
	}
	if spe(toSignedData(msgs, [][]byte{signers[0], signers[0]}, [][]byte{validSignature, validSignature})) {
		t.Errorf("Expected authentication to fail because although there were two valid signatures, one was duplicated")
	}
}

func TestComplexNestedSignature(t *testing.T) {
	policy := Envelope(And(Or(And(SignedBy(0), SignedBy(1)), And(SignedBy(0), SignedBy(0))), SignedBy(0)), signers)

	spe, err := compile(policy.Rule, policy.Identities, &mockDeserializer{})
	if err != nil {
		t.Fatalf("Could not create a new SignaturePolicyEvaluator using the given policy, crypto-helper: %s", err)
	}

	if !spe(toSignedData(moreMsgs, append(signers, [][]byte{[]byte("signer0")}...), [][]byte{validSignature, validSignature, validSignature})) {
		t.Errorf("Expected authentication to succeed with valid signatures")
	}
	if !spe(toSignedData(moreMsgs, [][]byte{[]byte("signer0"), []byte("signer0"), []byte("signer0")}, [][]byte{validSignature, validSignature, validSignature})) {
		t.Errorf("Expected authentication to succeed with valid signatures")
	}
	if spe(toSignedData(msgs, signers, [][]byte{validSignature, validSignature})) {
		t.Errorf("Expected authentication to fail with too few signatures")
	}
	if spe(toSignedData(moreMsgs, append(signers, [][]byte{[]byte("signer0")}...), [][]byte{validSignature, invalidSignature, validSignature})) {
		t.Errorf("Expected authentication failure as the signature of signer[1] was invalid")
	}
	if spe(toSignedData(moreMsgs, append(signers, [][]byte{[]byte("signer1")}...), [][]byte{validSignature, validSignature, validSignature})) {
		t.Errorf("Expected authentication failure as there was a signature from signer[0] missing")
	}
}

func TestNegatively(t *testing.T) {
	rpolicy := Envelope(And(SignedBy(0), SignedBy(1)), signers)
	rpolicy.Rule.Type = nil
	b, _ := proto.Marshal(rpolicy)
	policy := &cb.SignaturePolicyEnvelope{}
	_ = proto.Unmarshal(b, policy)
	_, err := compile(policy.Rule, policy.Identities, &mockDeserializer{})
	if err == nil {
		t.Fatal("Should have errored compiling because the Type field was nil")
	}
}

func TestNilSignaturePolicyEnvelope(t *testing.T) {
	_, err := compile(nil, nil, &mockDeserializer{})
	assert.Error(t, err, "Fail to compile")
}

func TestDeduplicate(t *testing.T) {
	ids := []*cb.SignedData{
		&cb.SignedData{
			Identity: []byte("id1"),
		},
		&cb.SignedData{
			Identity: []byte("id2"),
		},
		&cb.SignedData{
			Identity: []byte("id3"),
		},
	}

	t.Run("Empty", func(t *testing.T) {
		result := deduplicate([]*cb.SignedData{})
		assert.Equal(t, []*cb.SignedData{}, result, "Should have no identities")
	})

	t.Run("NoDuplication", func(t *testing.T) {
		result := deduplicate(ids)
		assert.Equal(t, ids, result, "No identities should have been removed")
	})

	t.Run("AllDuplication", func(t *testing.T) {
		result := deduplicate([]*cb.SignedData{ids[0], ids[0], ids[0]})
		assert.Equal(t, []*cb.SignedData{ids[0]}, result, "All but the first identity should have been removed")
	})

	t.Run("DuplicationPreservesOrder", func(t *testing.T) {
		result := deduplicate([]*cb.SignedData{ids[1], ids[0], ids[0]})
		assert.Equal(t, []*cb.SignedData{ids[1], ids[0]}, result, "The third identity should have been dropped")
	})

	t.Run("ComplexDuplication", func(t *testing.T) {
		result := deduplicate([]*cb.SignedData{ids[1], ids[0], ids[0], ids[1], ids[2], ids[0], ids[2], ids[1]})
		assert.Equal(t, []*cb.SignedData{ids[1], ids[0], ids[2]}, result, "Expected only three non-duplicate identities")
	})
}
