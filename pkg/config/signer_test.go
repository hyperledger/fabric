/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"log"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/msp"
	. "github.com/onsi/gomega"
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func TestNewSigningIdentity(t *testing.T) {
	t.Parallel()

	publicKey, privateKey := generatePublicAndPrivateKey()

	t.Run("success", func(t *testing.T) {
		gt := NewGomegaWithT(t)
		signingIdentity, err := NewSigningIdentity(publicKey, privateKey, "test-msp")
		gt.Expect(err).NotTo(HaveOccurred())
		gt.Expect(signingIdentity.MSPId()).To(Equal("test-msp"))
		cert, err := signingIdentity.Cert()
		gt.Expect(err).ToNot(HaveOccurred())
		gt.Expect(cert.Subject.CommonName).To(Equal("Wile E. Coyote"))
	})

	tests := []struct {
		spec        string
		publicKey   []byte
		privateKey  []byte
		mspID       string
		expectedErr string
		matchErr    bool
	}{
		{
			spec:        "nil public key",
			publicKey:   nil,
			privateKey:  privateKey,
			mspID:       "test-msp",
			expectedErr: "failed to get cert from pem: decoding pem bytes: []",
			matchErr:    true,
		},
		{
			spec:        "invalid public key",
			publicKey:   []byte("apple"),
			privateKey:  privateKey,
			mspID:       "test-msp",
			expectedErr: "failed to get cert from pem: decoding pem bytes",
			matchErr:    false,
		},
		{
			spec:        "public key is not a certificate",
			publicKey:   privateKey,
			privateKey:  privateKey,
			mspID:       "test-msp",
			expectedErr: "failed to get cert from pem: decoding pem bytes",
			matchErr:    false,
		},
		{
			spec:        "nil private key",
			publicKey:   publicKey,
			privateKey:  nil,
			mspID:       "test-msp",
			expectedErr: "failed to decode private key from pem",
			matchErr:    true,
		},
		{
			spec:        "empty mspID",
			publicKey:   publicKey,
			privateKey:  privateKey,
			expectedErr: "failed to create new signingIdentity, mspID can not be empty",
			matchErr:    true,
		},
	}

	for _, tc := range tests {
		tc := tc // capture range variable
		t.Run(tc.spec, func(t *testing.T) {
			t.Parallel()

			gt := NewGomegaWithT(t)

			_, err := NewSigningIdentity(tc.publicKey, tc.privateKey, tc.mspID)
			if tc.matchErr {
				gt.Expect(err).To(MatchError(tc.expectedErr))
			} else {
				gt.Expect(err.Error()).To(ContainSubstring(tc.expectedErr))
			}
		})
	}
}

func TestECDSAPublicKeyImport(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)
	x509cert := &x509.Certificate{PublicKey: struct{}{}}
	_, err := ecdsaPublicKeyImport(x509cert)
	gt.Expect(err).To(MatchError("certificate does not contain valid ECDSA public key"))
}

func TestECDSAPrivateKeyImport(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)
	_, err := ecdsaPrivateKeyImport(nil)
	gt.Expect(err.Error()).To(ContainSubstring("invalid key type. The DER must contain an ecdsa.PrivateKey"))
}

func TestSerialize(t *testing.T) {
	t.Parallel()

	publicKey, privateKey := generatePublicAndPrivateKey()

	gt := NewGomegaWithT(t)
	signingIdentity, err := NewSigningIdentity(publicKey, privateKey, "test-msp")
	gt.Expect(err).NotTo(HaveOccurred())

	sBytes, err := signingIdentity.Serialize()
	gt.Expect(err).NotTo(HaveOccurred())
	serializedIdentity := &msp.SerializedIdentity{}
	err = proto.Unmarshal(sBytes, serializedIdentity)
	gt.Expect(err).ToNot(HaveOccurred())
	gt.Expect(serializedIdentity.Mspid).To(Equal("test-msp"))
}

func TestSign(t *testing.T) {
	t.Parallel()

	publicKey, privateKey := generatePublicAndPrivateKey()

	tests := []struct {
		spec        string
		reader      io.Reader
		digest      []byte
		expectedErr string
	}{
		{
			spec:        "success",
			reader:      rand.Reader,
			digest:      []byte("banana"),
			expectedErr: "",
		},
		{
			spec:        "nil reader",
			reader:      nil,
			expectedErr: "reader can not be nil",
		},
	}

	gt := NewGomegaWithT(t)
	signingIdentity, err := NewSigningIdentity(publicKey, privateKey, "test-msp")
	gt.Expect(err).NotTo(HaveOccurred())

	for _, tc := range tests {
		tc := tc //capture range variable
		t.Run(tc.spec, func(t *testing.T) {
			t.Parallel()

			gt := NewGomegaWithT(t)
			_, err = signingIdentity.Sign(tc.reader, tc.digest)
			if tc.expectedErr == "" {
				gt.Expect(err).NotTo(HaveOccurred())
			} else {
				gt.Expect(err).To(MatchError(tc.expectedErr))
			}
		})
	}
}

func TestToLowS(t *testing.T) {
	t.Parallel()

	curve := elliptic.P256()
	halfOrder := new(big.Int).Div(curve.Params().N, big.NewInt(2))

	for _, test := range []struct {
		name        string
		sig         ecdsaSignature
		expectedSig ecdsaSignature
	}{
		{
			name: "HighS",
			sig: ecdsaSignature{
				R: big.NewInt(1),
				// set S to halfOrder + 1
				S: new(big.Int).Add(halfOrder, big.NewInt(1)),
			},
			// expected signature should be (sig.R, -sig.S mod N)
			expectedSig: ecdsaSignature{
				R: big.NewInt(1),
				S: new(big.Int).Mod(new(big.Int).Neg(new(big.Int).Add(halfOrder, big.NewInt(1))), curve.Params().N),
			},
		},
		{
			name: "LowS",
			sig: ecdsaSignature{
				R: big.NewInt(1),
				// set S to halfOrder - 1
				S: new(big.Int).Sub(halfOrder, big.NewInt(1)),
			},
			// expected signature should be sig
			expectedSig: ecdsaSignature{
				R: big.NewInt(1),
				S: new(big.Int).Sub(halfOrder, big.NewInt(1)),
			},
		},
		{
			name: "HalfOrder",
			sig: ecdsaSignature{
				R: big.NewInt(1),
				// set S to halfOrder
				S: halfOrder,
			},
			// expected signature should be sig
			expectedSig: ecdsaSignature{
				R: big.NewInt(1),
				S: halfOrder,
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			gt := NewGomegaWithT(t)
			curve := elliptic.P256()
			key := ecdsa.PublicKey{
				Curve: curve,
			}
			gt.Expect(toLowS(key, test.sig), test.expectedSig)
		})
	}
}

func generatePublicAndPrivateKey() ([]byte, []byte) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		log.Fatalf("Failed to generate private key: %s", err)
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		log.Fatalf("Failed to generate serial number: %s", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   "Wile E. Coyote",
			Organization: []string{"Acme Co"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		log.Fatalf("Failed to create certificate: %s", err)
	}
	publicKey := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})

	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		log.Fatalf("Unable to marshal private key: %v", err)
	}
	privateKey := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privBytes})

	return publicKey, privateKey
}
