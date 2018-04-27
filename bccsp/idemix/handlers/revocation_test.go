/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package handlers_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/x509"
	"math/big"

	"github.com/hyperledger/fabric/bccsp/idemix/handlers"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/idemix/handlers/mock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
)

var _ = Describe("Revocation", func() {

	Describe("when creating an revocation key-pair", func() {
		var (
			RevocationKeyGen *handlers.RevocationKeyGen

			fakeRevocation          *mock.Revocation
			fakeRevocationSecretKey bccsp.Key
		)

		BeforeEach(func() {
			fakeRevocation = &mock.Revocation{}

			RevocationKeyGen = &handlers.RevocationKeyGen{}
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

				fakeRevocationSecretKey = handlers.NewRevocationSecretKey(idemixRevocationKey, false)
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
					fakeRevocationSecretKey = handlers.NewRevocationSecretKey(idemixRevocationKey, true)
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
					fakeRevocationSecretKey = handlers.NewRevocationSecretKey(idemixRevocationKey, false)
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

var _ = Describe("CRI", func() {

	Describe("when creating a CRI", func() {

		var (
			CriSigner      *handlers.CriSigner
			fakeRevocation *mock.Revocation
		)

		BeforeEach(func() {
			fakeRevocation = &mock.Revocation{}
			CriSigner = &handlers.CriSigner{Revocation: fakeRevocation}
		})

		Context("and the underlying cryptographic algorithm succeed", func() {
			var (
				fakeSignature []byte
			)
			BeforeEach(func() {
				fakeSignature = []byte("fake signature")
				fakeRevocation.SignReturns(fakeSignature, nil)
			})

			It("returns no error and a signature", func() {
				signature, err := CriSigner.Sign(
					handlers.NewRevocationSecretKey(nil, false),
					bccsp.IdemixEmptyDigest(),
					&bccsp.IdemixCRISignerOpts{},
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(signature).To(BeEquivalentTo(fakeSignature))

			})
		})

		Context("and the underlying cryptographic algorithm fails", func() {
			BeforeEach(func() {
				fakeRevocation.SignReturns(nil, errors.New("sign error"))
			})

			It("returns an error", func() {
				signature, err := CriSigner.Sign(
					handlers.NewRevocationSecretKey(nil, false),
					bccsp.IdemixEmptyDigest(),
					&bccsp.IdemixCRISignerOpts{},
				)
				Expect(err).To(MatchError("sign error"))
				Expect(signature).To(BeNil())
			})
		})

		Context("and the parameters are not well formed", func() {

			Context("and the revocation secret key is nil", func() {
				It("returns error", func() {
					signature, err := CriSigner.Sign(
						nil,
						nil,
						nil,
					)
					Expect(err).To(MatchError("invalid key, expected *revocationSecretKey"))
					Expect(signature).To(BeNil())
				})
			})

			Context("and the revocation secret key is not of type *revocationSecretKey", func() {
				It("returns error", func() {
					signature, err := CriSigner.Sign(
						handlers.NewIssuerPublicKey(nil),
						nil,
						nil,
					)
					Expect(err).To(MatchError("invalid key, expected *revocationSecretKey"))
					Expect(signature).To(BeNil())
				})
			})

			Context("and the underlying cryptographic algorithm fails", func() {
				BeforeEach(func() {
				})

				It("returns an error", func() {
					signature, err := CriSigner.Sign(
						handlers.NewRevocationSecretKey(nil, false),
						nil,
						nil,
					)
					Expect(err).To(MatchError("invalid options, expected *IdemixCRISignerOpts"))
					Expect(signature).To(BeNil())
				})
			})

			Context("and the digest is not the idemix empty digest", func() {
				It("returns an error", func() {
					signature, err := CriSigner.Sign(
						handlers.NewRevocationSecretKey(nil, false),
						[]byte{1, 2, 3, 4},
						&bccsp.IdemixCRISignerOpts{},
					)
					Expect(err).To(MatchError("invalid digest, the idemix empty digest is expected"))
					Expect(signature).To(BeNil())
				})
			})
		})
	})

	Describe("when verifying a CRI", func() {

		var (
			CriVerifier    *handlers.CriVerifier
			fakeRevocation *mock.Revocation
		)

		BeforeEach(func() {
			fakeRevocation = &mock.Revocation{}
			CriVerifier = &handlers.CriVerifier{Revocation: fakeRevocation}
		})

		Context("and the underlying cryptographic algorithm succeed", func() {
			BeforeEach(func() {
				fakeRevocation.VerifyReturns(nil)
			})

			It("returns no error and valid signature", func() {
				valid, err := CriVerifier.Verify(
					handlers.NewRevocationPublicKey(nil),
					[]byte("fake signature"),
					bccsp.IdemixEmptyDigest(),
					&bccsp.IdemixCRISignerOpts{},
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(valid).To(BeTrue())
			})
		})

		Context("and the underlying cryptographic algorithm fails", func() {
			BeforeEach(func() {
				fakeRevocation.VerifyReturns(errors.New("verify error"))
			})

			It("returns an error", func() {
				valid, err := CriVerifier.Verify(
					handlers.NewRevocationPublicKey(nil),
					[]byte("fake signature"),
					bccsp.IdemixEmptyDigest(),
					&bccsp.IdemixCRISignerOpts{},
				)
				Expect(err).To(MatchError("verify error"))
				Expect(valid).To(BeFalse())
			})
		})

		Context("and the parameters are not well formed", func() {

			Context("and the user secret key is nil", func() {
				It("returns error", func() {
					valid, err := CriVerifier.Verify(
						nil,
						[]byte("fake signature"),
						nil,
						&bccsp.IdemixCRISignerOpts{},
					)
					Expect(err).To(MatchError("invalid key, expected *revocationPublicKey"))
					Expect(valid).To(BeFalse())
				})
			})

			Context("and the user secret key is not of type *revocationPublicKey", func() {
				It("returns error", func() {
					valid, err := CriVerifier.Verify(
						handlers.NewIssuerPublicKey(nil),
						[]byte("fake signature"),
						nil,
						&bccsp.IdemixCRISignerOpts{},
					)
					Expect(err).To(MatchError("invalid key, expected *revocationPublicKey"))
					Expect(valid).To(BeFalse())
				})
			})

			Context("and the digest is not the idemix empty digest", func() {
				It("returns error", func() {
					valid, err := CriVerifier.Verify(
						handlers.NewRevocationPublicKey(nil),
						[]byte("fake signature"),
						[]byte{1, 2, 3, 4},
						&bccsp.IdemixCRISignerOpts{},
					)
					Expect(err).To(MatchError("invalid digest, the idemix empty digest is expected"))
					Expect(valid).To(BeFalse())
				})
			})

			Context("and the signature is empty", func() {
				It("returns error", func() {
					valid, err := CriVerifier.Verify(
						handlers.NewRevocationPublicKey(nil),
						nil,
						bccsp.IdemixEmptyDigest(),
						&bccsp.IdemixCRISignerOpts{},
					)
					Expect(err).To(MatchError("invalid signature, it must not be empty"))
					Expect(valid).To(BeFalse())
				})
			})

			Context("and the option is empty", func() {
				It("returns error", func() {
					valid, err := CriVerifier.Verify(
						handlers.NewRevocationPublicKey(nil),
						[]byte("fake signature"),
						nil,
						nil,
					)
					Expect(err).To(MatchError("invalid options, expected *IdemixCRISignerOpts"))
					Expect(valid).To(BeFalse())
				})
			})

			Context("and the option is not of type *IdemixCRISignerOpts", func() {
				It("returns error", func() {
					valid, err := CriVerifier.Verify(
						handlers.NewRevocationPublicKey(nil),
						[]byte("fake signature"),
						nil,
						&bccsp.IdemixCredentialRequestSignerOpts{},
					)
					Expect(err).To(MatchError("invalid options, expected *IdemixCRISignerOpts"))
					Expect(valid).To(BeFalse())
				})
			})

		})
	})
})
