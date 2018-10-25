/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package idemix_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/x509"
	"math/big"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/idemix"
	"github.com/hyperledger/fabric/bccsp/idemix/mock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
)

var _ = Describe("Revocation", func() {

	Describe("when creating an revocation key-pair", func() {
		var (
			RevocationKeyGen *idemix.RevocationKeyGen

			fakeRevocation          *mock.Revocation
			fakeRevocationSecretKey bccsp.Key
		)

		BeforeEach(func() {
			fakeRevocation = &mock.Revocation{}

			RevocationKeyGen = &idemix.RevocationKeyGen{}
			RevocationKeyGen.Revocation = fakeRevocation
		})

		Context("and the underlying cryptographic algorithm succeed", func() {
			var (
				sk                  bccsp.Key
				idemixRevocationKey *ecdsa.PrivateKey
				SKI                 []byte
				pkBytes             []byte
			)
			BeforeEach(func() {
				idemixRevocationKey = &ecdsa.PrivateKey{
					PublicKey: ecdsa.PublicKey{
						Curve: elliptic.P256(),
						X:     big.NewInt(1), Y: big.NewInt(1)},
					D: big.NewInt(1)}

				raw := elliptic.Marshal(idemixRevocationKey.Curve, idemixRevocationKey.PublicKey.X, idemixRevocationKey.PublicKey.Y)
				hash := sha256.New()
				hash.Write(raw)
				SKI = hash.Sum(nil)

				var err error
				pkBytes, err = x509.MarshalPKIXPublicKey(&idemixRevocationKey.PublicKey)
				Expect(err).NotTo(HaveOccurred())

				fakeRevocation.NewKeyReturns(idemixRevocationKey, nil)

				fakeRevocationSecretKey = idemix.NewRevocationSecretKey(idemixRevocationKey, false)
			})

			AfterEach(func() {
				Expect(sk.Private()).To(BeTrue())
				Expect(sk.Symmetric()).To(BeFalse())
				Expect(sk.SKI()).NotTo(BeNil())
				Expect(sk.SKI()).To(BeEquivalentTo(SKI))

				pk, err := sk.PublicKey()
				Expect(err).NotTo(HaveOccurred())

				Expect(pk.Private()).To(BeFalse())
				Expect(pk.Symmetric()).To(BeFalse())
				Expect(pk.SKI()).NotTo(BeNil())
				Expect(pk.SKI()).To(BeEquivalentTo(SKI))
				raw, err := pk.Bytes()
				Expect(err).NotTo(HaveOccurred())
				Expect(raw).NotTo(BeNil())
				Expect(raw).To(BeEquivalentTo(pkBytes))

				pk2, err := pk.PublicKey()
				Expect(err).NotTo(HaveOccurred())
				Expect(pk).To(BeEquivalentTo(pk2))
			})

			Context("and the secret key is exportable", func() {
				BeforeEach(func() {
					RevocationKeyGen.Exportable = true
					fakeRevocationSecretKey = idemix.NewRevocationSecretKey(idemixRevocationKey, true)
				})

				It("returns no error and a key", func() {
					var err error
					sk, err = RevocationKeyGen.KeyGen(&bccsp.IdemixRevocationKeyGenOpts{})
					Expect(err).NotTo(HaveOccurred())
					Expect(sk).To(BeEquivalentTo(fakeRevocationSecretKey))

					raw, err := sk.Bytes()
					Expect(err).NotTo(HaveOccurred())
					Expect(raw).NotTo(BeNil())
					Expect(raw).To(BeEquivalentTo(idemixRevocationKey.D.Bytes()))
				})
			})

			Context("and the secret key is not exportable", func() {
				BeforeEach(func() {
					RevocationKeyGen.Exportable = false
					fakeRevocationSecretKey = idemix.NewRevocationSecretKey(idemixRevocationKey, false)
				})

				It("returns no error and a key", func() {
					sk, err := RevocationKeyGen.KeyGen(&bccsp.IdemixRevocationKeyGenOpts{})
					Expect(err).NotTo(HaveOccurred())
					Expect(sk).To(BeEquivalentTo(fakeRevocationSecretKey))

					raw, err := sk.Bytes()
					Expect(err).To(MatchError("not exportable"))
					Expect(raw).To(BeNil())
				})

			})
		})

		Context("and the underlying cryptographic algorithm fails", func() {
			BeforeEach(func() {
				fakeRevocation.NewKeyReturns(nil, errors.New("new-key error"))
			})

			It("returns an error", func() {
				keyPair, err := RevocationKeyGen.KeyGen(&bccsp.IdemixRevocationKeyGenOpts{})
				Expect(err).To(MatchError("new-key error"))
				Expect(keyPair).To(BeNil())
			})
		})

	})
})
