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
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/flogging/floggingtest"
	"github.com/hyperledger/fabric/msp"
	cb "github.com/hyperledger/fabric/protos/common"
	mb "github.com/hyperledger/fabric/protos/msp"
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
	if bytes.Compare(id.idBytes, p.Principal) == 0 {
		return nil
	} else {
		return errors.New("Principals do not match")
	}
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
	if bytes.Compare(sig, invalidSignature) == 0 {
		return errors.New("Invalid signature")
	} else {
		return nil
	}
}

func (id *mockIdentity) Serialize() ([]byte, error) {
	return id.idBytes, nil
}

func toSignedData(data [][]byte, identities [][]byte, signatures [][]byte, deserializer msp.IdentityDeserializer) ([]IdentityAndSignature, []bool) {
	signedData := make([]IdentityAndSignature, len(data))
	for i := range signedData {
		signedData[i] = &deserializeAndVerify{
			signedData: &cb.SignedData{
				Data:      data[i],
				Identity:  identities[i],
				Signature: signatures[i],
			},
			deserializer: deserializer,
		}
	}
	return signedData, make([]bool, len(signedData))
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

	spe, err := compile(policy.Rule, policy.Identities, &mockDeserializer{})
	if err != nil {
		t.Fatalf("Could not create a new SignaturePolicyEvaluator using the given policy, crypto-helper: %s", err)
	}

	if !spe(toSignedData([][]byte{nil}, [][]byte{signers[0]}, [][]byte{validSignature}, &mockDeserializer{})) {
		t.Errorf("Expected authentication to succeed with valid signatures")
	}
	if spe(toSignedData([][]byte{nil}, [][]byte{signers[0]}, [][]byte{invalidSignature}, &mockDeserializer{})) {
		t.Errorf("Expected authentication to fail given the invalid signature")
	}
	if spe(toSignedData([][]byte{nil}, [][]byte{signers[1]}, [][]byte{validSignature}, &mockDeserializer{})) {
		t.Errorf("Expected authentication to fail because signers[1] is not authorized in the policy, despite his valid signature")
	}
}

func TestMultipleSignature(t *testing.T) {
	policy := Envelope(And(SignedBy(0), SignedBy(1)), signers)

	spe, err := compile(policy.Rule, policy.Identities, &mockDeserializer{})
	if err != nil {
		t.Fatalf("Could not create a new SignaturePolicyEvaluator using the given policy, crypto-helper: %s", err)
	}

	if !spe(toSignedData(msgs, signers, [][]byte{validSignature, validSignature}, &mockDeserializer{})) {
		t.Errorf("Expected authentication to succeed with  valid signatures")
	}
	if spe(toSignedData(msgs, signers, [][]byte{validSignature, invalidSignature}, &mockDeserializer{})) {
		t.Errorf("Expected authentication to fail given one of two invalid signatures")
	}
	if spe(toSignedData(msgs, [][]byte{signers[0], signers[0]}, [][]byte{validSignature, validSignature}, &mockDeserializer{})) {
		t.Errorf("Expected authentication to fail because although there were two valid signatures, one was duplicated")
	}
}

func TestComplexNestedSignature(t *testing.T) {
	policy := Envelope(And(Or(And(SignedBy(0), SignedBy(1)), And(SignedBy(0), SignedBy(0))), SignedBy(0)), signers)

	spe, err := compile(policy.Rule, policy.Identities, &mockDeserializer{})
	if err != nil {
		t.Fatalf("Could not create a new SignaturePolicyEvaluator using the given policy, crypto-helper: %s", err)
	}

	if !spe(toSignedData(moreMsgs, append(signers, [][]byte{[]byte("signer0")}...), [][]byte{validSignature, validSignature, validSignature}, &mockDeserializer{})) {
		t.Errorf("Expected authentication to succeed with valid signatures")
	}
	if !spe(toSignedData(moreMsgs, [][]byte{[]byte("signer0"), []byte("signer0"), []byte("signer0")}, [][]byte{validSignature, validSignature, validSignature}, &mockDeserializer{})) {
		t.Errorf("Expected authentication to succeed with valid signatures")
	}
	if spe(toSignedData(msgs, signers, [][]byte{validSignature, validSignature}, &mockDeserializer{})) {
		t.Errorf("Expected authentication to fail with too few signatures")
	}
	if spe(toSignedData(moreMsgs, append(signers, [][]byte{[]byte("signer0")}...), [][]byte{validSignature, invalidSignature, validSignature}, &mockDeserializer{})) {
		t.Errorf("Expected authentication failure as the signature of signer[1] was invalid")
	}
	if spe(toSignedData(moreMsgs, append(signers, [][]byte{[]byte("signer1")}...), [][]byte{validSignature, validSignature, validSignature}, &mockDeserializer{})) {
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
	t.Run("Empty", func(t *testing.T) {
		result := deduplicate([]IdentityAndSignature{})
		assert.Equal(t, []IdentityAndSignature{}, result, "Should have no identities")
	})

	t.Run("NoDuplication", func(t *testing.T) {
		md := &mockDeserializer{}
		ids := []IdentityAndSignature{
			&deserializeAndVerify{
				signedData: &cb.SignedData{
					Identity: []byte("id1"),
				},
				deserializer: md,
			},
			&deserializeAndVerify{
				signedData: &cb.SignedData{
					Identity: []byte("id2"),
				},
				deserializer: md,
			},
			&deserializeAndVerify{
				signedData: &cb.SignedData{
					Identity: []byte("id3"),
				},
				deserializer: md,
			},
		}
		result := deduplicate(ids)
		assert.Equal(t, ids, result, "No identities should have been removed")
	})

	t.Run("AllDuplication", func(t *testing.T) {
		md := &mockDeserializer{}
		ids := []IdentityAndSignature{
			&deserializeAndVerify{
				signedData: &cb.SignedData{
					Identity: []byte("id1"),
				},
				deserializer: md,
			},
		}
		result := deduplicate([]IdentityAndSignature{ids[0], ids[0], ids[0]})
		assert.Equal(t, []IdentityAndSignature{ids[0]}, result, "All but the first identity should have been removed")
	})

	t.Run("DuplicationPreservesOrder", func(t *testing.T) {
		md := &mockDeserializer{}
		ids := []IdentityAndSignature{
			&deserializeAndVerify{
				signedData: &cb.SignedData{
					Identity: []byte("id1"),
				},
				deserializer: md,
			},
			&deserializeAndVerify{
				signedData: &cb.SignedData{
					Identity: []byte("id2"),
				},
				deserializer: md,
			},
		}
		result := deduplicate([]IdentityAndSignature{ids[1], ids[0], ids[0]})
		assert.Equal(t, result, []IdentityAndSignature{ids[1], ids[0]}, "The third identity should have been dropped")
	})

	t.Run("ComplexDuplication", func(t *testing.T) {
		md := &mockDeserializer{}
		ids := []IdentityAndSignature{
			&deserializeAndVerify{
				signedData: &cb.SignedData{
					Identity: []byte("id1"),
				},
				deserializer: md,
			},
			&deserializeAndVerify{
				signedData: &cb.SignedData{
					Identity: []byte("id2"),
				},
				deserializer: md,
			},
			&deserializeAndVerify{
				signedData: &cb.SignedData{
					Identity: []byte("id3"),
				},
				deserializer: md,
			},
		}
		result := deduplicate([]IdentityAndSignature{ids[1], ids[0], ids[0], ids[1], ids[2], ids[0], ids[2], ids[1]})
		assert.Equal(t, []IdentityAndSignature{ids[1], ids[0], ids[2]}, result, "Expected only three non-duplicate identities")
	})

	t.Run("BadIdentity", func(t *testing.T) {
		md := &mockDeserializer{fail: errors.New("error")}
		ids := []IdentityAndSignature{
			&deserializeAndVerify{
				signedData: &cb.SignedData{
					Identity: []byte("id1"),
				},
				deserializer: md,
			},
		}
		result := deduplicate([]IdentityAndSignature{ids[0]})
		assert.Equal(t, []IdentityAndSignature{}, result, "No valid identities")
	})
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

	spe, err := compile(policy.Rule, policy.Identities, &mockDeserializer{})
	assert.Nil(t, spe)
	assert.EqualError(t, err, "identity index out of range, requested -1, but identities length is 2")
}

func TestDeserializeIdentityError(t *testing.T) {
	// Prepare
	policy := Envelope(SignedBy(0), signers)
	spe, err := compile(policy.Rule, policy.Identities, &mockDeserializer{fail: errors.New("myError")})
	assert.NoError(t, err)

	logger, recorder := floggingtest.NewTestLogger(t)
	defer func(old *flogging.FabricLogger) {
		cauthdslLogger = old
	}(cauthdslLogger)
	cauthdslLogger = logger

	// Call
	signedData, used := toSignedData([][]byte{nil}, [][]byte{nil}, [][]byte{nil}, &mockDeserializer{fail: errors.New("myError")})
	ret := spe(signedData, used)

	// Check result (ret and log)
	assert.False(t, ret)
	assert.Contains(t, string(recorder.Buffer().Contents()), "Principal deserialization failure (myError) for identity")
}
