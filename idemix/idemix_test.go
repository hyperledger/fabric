/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix

import (
	"bytes"
	"testing"

	"github.com/hyperledger/fabric-amcl/amcl/FP256BN"
	"github.com/stretchr/testify/assert"
)

func TestIdemix(t *testing.T) {
	// Test weak BB sigs:
	// Test KeyGen
	rng, err := GetRand()
	assert.NoError(t, err)
	wbbsk, wbbpk := WBBKeyGen(rng)

	// Get random message
	testmsg := RandModOrder(rng)

	// Test Signing
	wbbsig := WBBSign(wbbsk, testmsg)

	// Test Verification
	err = WBBVerify(wbbpk, wbbsig, testmsg)
	assert.NoError(t, err)

	// Test idemix functionality
	AttributeNames := []string{"Attr1", "Attr2", "Attr3", "Attr4", "Attr5"}
	attrs := make([]*FP256BN.BIG, len(AttributeNames))
	for i := range AttributeNames {
		attrs[i] = FP256BN.NewBIGint(i)
	}

	// Test issuer key generation
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
	err = key.GetIpk().Check()
	if err != nil {
		t.Fatalf("Issuer public key should be valid")
		return
	}

	// Make sure Check() is invalid for a public key with invalid proof
	proofC := key.Ipk.GetProofC()
	key.Ipk.ProofC = BigToBytes(RandModOrder(rng))
	assert.Error(t, key.Ipk.Check(), "public key with broken zero-knowledge proof should be invalid")

	// Make sure Check() is invalid for a public key with incorrect number of HAttrs
	hAttrs := key.Ipk.GetHAttrs()
	key.Ipk.HAttrs = key.Ipk.HAttrs[:0]
	assert.Error(t, key.Ipk.Check(), "public key with incorrect number of HAttrs should be invalid")
	key.Ipk.HAttrs = hAttrs

	// Restore IPk to be valid
	key.Ipk.ProofC = proofC
	h := key.Ipk.GetHash()
	assert.NoError(t, key.Ipk.Check(), "restored public key should be valid")
	assert.Zero(t, bytes.Compare(h, key.Ipk.GetHash()), "IPK hash changed on ipk Check")

	// Create public with duplicate attribute names should fail
	_, err = NewIssuerKey([]string{"Attr1", "Attr2", "Attr1"}, rng)
	assert.Error(t, err, "issuer key generation should fail with duplicate attribute names")

	// Test issuance
	sk := RandModOrder(rng)
	ni := RandModOrder(rng)
	m := NewCredRequest(sk, BigToBytes(ni), key.Ipk, rng)

	cred, err := NewCredential(key, m, attrs, rng)
	assert.NoError(t, err, "Failed to issue a credential: \"%s\"", err)

	assert.NoError(t, cred.Ver(sk, key.Ipk), "credential should be valid")

	// Issuing a credential with the incorrect amount of attributes should fail
	_, err = NewCredential(key, m, []*FP256BN.BIG{}, rng)
	assert.Error(t, err, "issuing a credential with the incorrect amount of attributes should fail")

	// Breaking the ZK proof of the CredRequest should make it invalid
	proofC = m.GetProofC()
	m.ProofC = BigToBytes(RandModOrder(rng))
	assert.Error(t, m.Check(key.Ipk), "CredRequest with broken ZK proof should not be valid")

	// Creating a credential from a broken CredRequest should fail
	_, err = NewCredential(key, m, attrs, rng)
	assert.Error(t, err, "creating a credential from an invalid CredRequest should fail")
	m.ProofC = proofC

	// A credential with nil attribute should be invalid
	attrsBackup := cred.GetAttrs()
	cred.Attrs = [][]byte{nil, nil, nil, nil, nil}
	assert.Error(t, cred.Ver(sk, key.Ipk), "credential with nil attribute should be invalid")
	cred.Attrs = attrsBackup

	// Generate a revocation key pair
	revocationKey, err := GenerateLongTermRevocationKey()
	assert.NoError(t, err)

	// Create CRI that contains no revocation mechanism
	epoch := 0
	cri, err := CreateCRI(revocationKey, []*FP256BN.BIG{}, epoch, ALG_NO_REVOCATION, rng)
	assert.NoError(t, err)
	err = VerifyEpochPK(&revocationKey.PublicKey, cri.EpochPk, cri.EpochPkSig, int(cri.Epoch), RevocationAlgorithm(cri.RevocationAlg))
	assert.NoError(t, err)

	// make sure that epoch pk is not valid in future epoch
	err = VerifyEpochPK(&revocationKey.PublicKey, cri.EpochPk, cri.EpochPkSig, int(cri.Epoch)+1, RevocationAlgorithm(cri.RevocationAlg))
	assert.Error(t, err)

	// Test bad input
	_, err = CreateCRI(nil, []*FP256BN.BIG{}, epoch, ALG_NO_REVOCATION, rng)
	assert.Error(t, err)
	_, err = CreateCRI(revocationKey, []*FP256BN.BIG{}, epoch, ALG_NO_REVOCATION, nil)
	assert.Error(t, err)

	// Test signing no disclosure
	Nym, RandNym := MakeNym(sk, key.Ipk, rng)

	disclosure := []byte{0, 0, 0, 0, 0}
	msg := []byte{1, 2, 3, 4, 5}
	rhindex := 4
	sig, err := NewSignature(cred, sk, Nym, RandNym, key.Ipk, disclosure, msg, rhindex, cri, rng)
	assert.NoError(t, err)

	err = sig.Ver(disclosure, key.Ipk, msg, nil, 0, &revocationKey.PublicKey, epoch)
	if err != nil {
		t.Fatalf("Signature should be valid but verification returned error: %s", err)
		return
	}

	// Test signing selective disclosure
	disclosure = []byte{0, 1, 1, 1, 0}
	sig, err = NewSignature(cred, sk, Nym, RandNym, key.Ipk, disclosure, msg, rhindex, cri, rng)
	assert.NoError(t, err)

	err = sig.Ver(disclosure, key.Ipk, msg, attrs, rhindex, &revocationKey.PublicKey, epoch)
	assert.NoError(t, err)

	// Test NymSignatures
	nymsig, err := NewNymSignature(sk, Nym, RandNym, key.Ipk, []byte("testing"), rng)
	assert.NoError(t, err)

	err = nymsig.Ver(Nym, key.Ipk, []byte("testing"))
	if err != nil {
		t.Fatalf("NymSig should be valid but verification returned error: %s", err)
		return
	}
}
