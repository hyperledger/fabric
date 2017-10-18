/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix

import (
	"testing"

	"bytes"

	amcl "github.com/manudrijvers/amcl/go"
	"github.com/stretchr/testify/assert"
)

func TestIdemix(t *testing.T) {
	AttributeNames := []string{"Attr1", "Attr2", "Attr3", "Attr4", "Attr5"}
	attrs := make([]*amcl.BIG, len(AttributeNames))
	for i := range AttributeNames {
		attrs[i] = amcl.NewBIGint(i)
	}

	// Test issuer key generation
	rng, err := GetRand()
	if err != nil {
		t.Fatalf("Error getting rng: \"%s\"", err)
		return
	}
	// Create a new key pair
	key, err := NewIssuerKey(AttributeNames, rng)
	if err != nil {
		t.Fatalf("Issuer key generation should have succeeded but gave error \"%s\"", err)
		return
	}

	// Check that the key is valid
	err = key.GetIPk().Check()
	if err != nil {
		t.Fatalf("Issuer public key should be valid")
		return
	}

	// Make sure Check() is invalid for a public key with invalid proof
	proofC := key.IPk.GetProofC()
	key.IPk.ProofC = BigToBytes(RandModOrder(rng))
	assert.Error(t, key.IPk.Check(), "public key with broken zero-knowledge proof should be invalid")

	// Make sure Check() is invalid for a public key with incorrect number of HAttrs
	hAttrs := key.IPk.GetHAttrs()
	key.IPk.HAttrs = key.IPk.HAttrs[:0]
	assert.Error(t, key.IPk.Check(), "public key with incorrect number of HAttrs should be invalid")
	key.IPk.HAttrs = hAttrs

	// Restore IPk to be valid
	key.IPk.ProofC = proofC
	h := key.IPk.GetHash()
	assert.NoError(t, key.IPk.Check(), "restored public key should be valid")
	assert.Zero(t, bytes.Compare(h, key.IPk.GetHash()), "IPK hash changed on ipk Check")

	// Create public with duplicate attribute names should fail
	_, err = NewIssuerKey([]string{"Attr1", "Attr2", "Attr1"}, rng)
	assert.Error(t, err, "issuer key generation should fail with duplicate attribute names")

	// Test issuance
	sk := RandModOrder(rng)
	randCred := RandModOrder(rng)
	ni := RandModOrder(rng)
	m := NewCredRequest(sk, randCred, ni, key.IPk, rng)

	cred, err := NewCredential(key, m, attrs, rng)
	assert.NoError(t, err, "Failed to issue a credential: \"%s\"", err)

	cred.Complete(randCred)

	assert.NoError(t, cred.Ver(sk, key.IPk), "credential should be valid")

	// Issuing a credential with the incorrect amount of attributes should fail
	_, err = NewCredential(key, m, []*amcl.BIG{}, rng)
	assert.Error(t, err, "issuing a credential with the incorrect amount of attributes should fail")

	// Breaking the ZK proof of the CredRequest should make it invalid
	proofC = m.GetProofC()
	m.ProofC = BigToBytes(RandModOrder(rng))
	assert.Error(t, m.Check(key.IPk), "CredRequest with broken ZK proof should not be valid")

	// Creating a credential from a broken CredRequest should fail
	_, err = NewCredential(key, m, attrs, rng)
	assert.Error(t, err, "creating a credential from an invalid CredRequest should fail")
	m.ProofC = proofC

	// A credential with nil attribute should be invalid
	attrsBackup := cred.GetAttrs()
	cred.Attrs = [][]byte{nil, nil, nil, nil, nil}
	assert.Error(t, cred.Ver(sk, key.IPk), "credential with nil attribute should be invalid")
	cred.Attrs = attrsBackup

	// Test signing no disclosure
	Nym, RandNym := MakeNym(sk, key.IPk, rng)

	disclosure := []byte{0, 0, 0, 0, 0}
	msg := []byte{1, 2, 3, 4, 5}
	sig, err := NewSignature(cred, sk, Nym, RandNym, key.IPk, disclosure, msg, rng)
	assert.NoError(t, err)

	err = sig.Ver(disclosure, key.IPk, msg, nil)
	if err != nil {
		t.Fatalf("Signature should be valid but verification returned error: %s", err)
		return
	}

	// Test signing selective disclosure
	disclosure = []byte{0, 1, 1, 1, 1}
	sig, err = NewSignature(cred, sk, Nym, RandNym, key.IPk, disclosure, msg, rng)
	assert.NoError(t, err)

	err = sig.Ver(disclosure, key.IPk, msg, attrs)
	if err != nil {
		t.Fatalf("Signature should be valid but verification returned error: %s", err)
		return
	}

	// Test NymSignatures
	nymsig, err := NewNymSignature(sk, Nym, RandNym, key.IPk, []byte("testing"), rng)
	assert.NoError(t, err)

	err = nymsig.Ver(Nym, key.IPk, []byte("testing"))
	if err != nil {
		t.Fatalf("NymSig should be valid but verification returned error: %s", err)
		return
	}
}
