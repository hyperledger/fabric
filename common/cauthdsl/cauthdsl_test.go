/*
Copyright IBM Corp. 2016 All Rights Reserved.

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

package cauthdsl

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric/protos/common"
)

var invalidSignature = []byte("badsigned")
var validSignature = []byte("signed")
var signers = [][]byte{[]byte("signer0"), []byte("signer1")}
var msgs = [][]byte{nil, nil}

type mockCryptoHelper struct {
}

func (mch *mockCryptoHelper) VerifySignature(sd *cb.SignedData) error {
	if !bytes.Equal(sd.Signature, validSignature) {
		return fmt.Errorf("Bad signature")
	}
	return nil
}

func toSignedData(data [][]byte, identities [][]byte, signatures [][]byte) []*cb.SignedData {
	signedData := make([]*cb.SignedData, len(data))
	for i := range signedData {
		signedData[i] = &cb.SignedData{
			Data:      data[i],
			Identity:  identities[i],
			Signature: signatures[i],
		}
	}
	return signedData
}

func TestSimpleSignature(t *testing.T) {
	mch := &mockCryptoHelper{}
	policy := Envelope(SignedBy(0), signers)

	spe, err := compile(policy.Policy, policy.Identities, mch)
	if err != nil {
		t.Fatalf("Could not create a new SignaturePolicyEvaluator using the given policy, crypto-helper: %s", err)
	}

	if !spe([]*cb.SignedData{&cb.SignedData{Identity: signers[0], Signature: validSignature}}) {
		t.Errorf("Expected authentication to succeed with  valid signatures")
	}
	if spe([]*cb.SignedData{&cb.SignedData{Identity: signers[0], Signature: invalidSignature}}) {
		t.Errorf("Expected authentication to fail given the invalid signature")
	}
	if spe([]*cb.SignedData{&cb.SignedData{Identity: signers[1], Signature: validSignature}}) {
		t.Errorf("Expected authentication to fail because signers[1] is not authorized in the policy, despite his valid signature")
	}
}

func TestMultipleSignature(t *testing.T) {
	mch := &mockCryptoHelper{}
	policy := Envelope(And(SignedBy(0), SignedBy(1)), signers)

	spe, err := compile(policy.Policy, policy.Identities, mch)
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
	mch := &mockCryptoHelper{}
	policy := Envelope(And(Or(And(SignedBy(0), SignedBy(1)), And(SignedBy(0), SignedBy(0))), SignedBy(0)), signers)

	spe, err := compile(policy.Policy, policy.Identities, mch)
	if err != nil {
		t.Fatalf("Could not create a new SignaturePolicyEvaluator using the given policy, crypto-helper: %s", err)
	}

	if !spe(toSignedData(msgs, signers, [][]byte{validSignature, validSignature})) {
		t.Errorf("Expected authentication to succeed with valid signatures")
	}
	if spe(toSignedData(msgs, signers, [][]byte{invalidSignature, validSignature})) {
		t.Errorf("Expected authentication failure as only the signature of signer[1] was valid")
	}
	if !spe(toSignedData(msgs, [][]byte{signers[0], signers[0]}, [][]byte{validSignature, validSignature})) {
		t.Errorf("Expected authentication to succeed because the rule allows duplicated signatures for signer[0]")
	}
}

func TestNegatively(t *testing.T) {
	mch := &mockCryptoHelper{}
	rpolicy := Envelope(And(SignedBy(0), SignedBy(1)), signers)
	rpolicy.Policy.Type = nil
	b, _ := proto.Marshal(rpolicy)
	policy := &cb.SignaturePolicyEnvelope{}
	_ = proto.Unmarshal(b, policy)
	_, err := compile(policy.Policy, policy.Identities, mch)
	if err == nil {
		t.Fatal("Should have errored compiling because the Type field was nil")
	}
}
