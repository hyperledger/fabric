/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package handlers_test

import (
	"errors"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/idemix/handlers"
	"github.com/hyperledger/fabric/bccsp/idemix/handlers/mock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Signature", func() {

	Describe("when creating a signature", func() {

		var (
			Signer              *handlers.Signer
			fakeSignatureScheme *mock.SignatureScheme
			nymSK               bccsp.Key
		)

		BeforeEach(func() {
			fakeSignatureScheme = &mock.SignatureScheme{}
			Signer = &handlers.Signer{SignatureScheme: fakeSignatureScheme}

			var err error
			sk := &mock.Big{}
			sk.BytesReturns([]byte{1, 2, 3, 4}, nil)
			nymSK, err = handlers.NewNymSecretKey(sk, nil, false)
			Expect(err).NotTo(HaveOccurred())
		})

		Context("and the underlying cryptographic algorithm succeed", func() {
			var (
				fakeSignature []byte
			)
			BeforeEach(func() {
				fakeSignature = []byte("fake signature")
				fakeSignatureScheme.SignReturns(fakeSignature, nil)
			})

			It("returns no error and a signature", func() {
				signature, err := Signer.Sign(
					handlers.NewUserSecretKey(nil, false),
					[]byte("a digest"),
					&bccsp.IdemixSignerOpts{
						Nym:      nymSK,
						IssuerPK: handlers.NewIssuerPublicKey(nil),
					},
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(signature).To(BeEquivalentTo(fakeSignature))

			})
		})

		Context("and the underlying cryptographic algorithm fails", func() {
			BeforeEach(func() {
				fakeSignatureScheme.SignReturns(nil, errors.New("sign error"))
			})

			It("returns an error", func() {
				signature, err := Signer.Sign(
					handlers.NewUserSecretKey(nil, false),
					[]byte("a digest"),
					&bccsp.IdemixSignerOpts{
						Nym:      nymSK,
						IssuerPK: handlers.NewIssuerPublicKey(nil),
					},
				)
				Expect(err).To(MatchError("sign error"))
				Expect(signature).To(BeNil())
			})
		})

		Context("and the parameters are not well formed", func() {

			Context("and the user secret key is nil", func() {
				It("returns error", func() {
					signature, err := Signer.Sign(
						nil,
						[]byte("a digest"),
						&bccsp.IdemixSignerOpts{
							Nym:      nymSK,
							IssuerPK: handlers.NewIssuerPublicKey(nil),
						},
					)
					Expect(err).To(MatchError("invalid key, expected *userSecretKey"))
					Expect(signature).To(BeNil())
				})
			})

			Context("and the user secret key is not of type *userSecretKey", func() {
				It("returns error", func() {
					signature, err := Signer.Sign(
						handlers.NewIssuerPublicKey(nil),
						[]byte("a digest"),
						&bccsp.IdemixSignerOpts{
							Nym:      nymSK,
							IssuerPK: handlers.NewIssuerPublicKey(nil),
						},
					)
					Expect(err).To(MatchError("invalid key, expected *userSecretKey"))
					Expect(signature).To(BeNil())
				})
			})

			Context("and the option is nil", func() {
				It("returns error", func() {
					signature, err := Signer.Sign(
						handlers.NewUserSecretKey(nil, false),
						[]byte("a digest"),
						nil,
					)
					Expect(err).To(MatchError("invalid options, expected *IdemixSignerOpts"))
					Expect(signature).To(BeNil())
				})
			})

			Context("and the option is not of type *IdemixSignerOpts", func() {
				It("returns error", func() {
					signature, err := Signer.Sign(
						handlers.NewUserSecretKey(nil, false),
						[]byte("a digest"),
						&bccsp.IdemixCRISignerOpts{},
					)
					Expect(err).To(MatchError("invalid options, expected *IdemixSignerOpts"))
					Expect(signature).To(BeNil())
				})
			})

			Context("and the nym is nil", func() {
				It("returns error", func() {
					signature, err := Signer.Sign(
						handlers.NewUserSecretKey(nil, false),
						[]byte("a digest"),
						&bccsp.IdemixSignerOpts{
							IssuerPK: handlers.NewIssuerPublicKey(nil),
						},
					)
					Expect(err).To(MatchError("invalid options, missing nym key"))
					Expect(signature).To(BeNil())
				})
			})

			Context("and the nym is not of type *nymSecretKey", func() {
				It("returns error", func() {
					signature, err := Signer.Sign(
						handlers.NewUserSecretKey(nil, false),
						[]byte("a digest"),
						&bccsp.IdemixSignerOpts{
							Nym:      handlers.NewIssuerPublicKey(nil),
							IssuerPK: handlers.NewIssuerPublicKey(nil),
						},
					)
					Expect(err).To(MatchError("invalid nym key, expected *nymSecretKey"))
					Expect(signature).To(BeNil())
				})
			})

			Context("and the IssuerPk is nil", func() {
				It("returns error", func() {
					signature, err := Signer.Sign(
						handlers.NewUserSecretKey(nil, false),
						[]byte("a digest"),
						&bccsp.IdemixSignerOpts{
							Nym: nymSK,
						},
					)
					Expect(err).To(MatchError("invalid options, missing issuer public key"))
					Expect(signature).To(BeNil())
				})
			})

			Context("and the IssuerPk is not of type *issuerPublicKey", func() {
				It("returns error", func() {
					signature, err := Signer.Sign(
						handlers.NewUserSecretKey(nil, false),
						[]byte("a digest"),
						&bccsp.IdemixSignerOpts{
							Nym:      nymSK,
							IssuerPK: handlers.NewUserSecretKey(nil, false),
						},
					)
					Expect(err).To(MatchError("invalid issuer public key, expected *issuerPublicKey"))
					Expect(signature).To(BeNil())
				})
			})
		})
	})

	Describe("when verifying a signature", func() {

		var (
			Verifier            *handlers.Verifier
			fakeSignatureScheme *mock.SignatureScheme
		)

		BeforeEach(func() {
			fakeSignatureScheme = &mock.SignatureScheme{}
			Verifier = &handlers.Verifier{SignatureScheme: fakeSignatureScheme}
		})

		Context("and the underlying cryptographic algorithm succeed", func() {
			BeforeEach(func() {
				fakeSignatureScheme.VerifyReturns(nil)
			})

			It("returns no error and valid signature", func() {
				valid, err := Verifier.Verify(
					handlers.NewIssuerPublicKey(nil),
					[]byte("a signature"),
					[]byte("a digest"),
					&bccsp.IdemixSignerOpts{
						RevocationPublicKey: handlers.NewRevocationPublicKey(nil),
					},
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(valid).To(BeTrue())
			})
		})

		Context("and the underlying cryptographic algorithm fails", func() {
			BeforeEach(func() {
				fakeSignatureScheme.VerifyReturns(errors.New("verify error"))
			})

			It("returns an error", func() {
				valid, err := Verifier.Verify(
					handlers.NewIssuerPublicKey(nil),
					[]byte("a signature"),
					[]byte("a digest"),
					&bccsp.IdemixSignerOpts{
						RevocationPublicKey: handlers.NewRevocationPublicKey(nil),
					},
				)
				Expect(err).To(MatchError("verify error"))
				Expect(valid).To(BeFalse())
			})
		})

		Context("and the parameters are not well formed", func() {

			Context("and the issuer public key is nil", func() {
				It("returns error", func() {
					valid, err := Verifier.Verify(
						nil,
						[]byte("fake signature"),
						nil,
						&bccsp.IdemixSignerOpts{IssuerPK: handlers.NewIssuerPublicKey(nil)},
					)
					Expect(err).To(MatchError("invalid key, expected *issuerPublicKey"))
					Expect(valid).To(BeFalse())
				})
			})

			Context("and the issuer public key is not of type *issuerPublicKey", func() {
				It("returns error", func() {
					valid, err := Verifier.Verify(
						handlers.NewUserSecretKey(nil, false),
						[]byte("fake signature"),
						nil,
						&bccsp.IdemixSignerOpts{IssuerPK: handlers.NewIssuerPublicKey(nil)},
					)
					Expect(err).To(MatchError("invalid key, expected *issuerPublicKey"))
					Expect(valid).To(BeFalse())
				})
			})

			Context("and the signature is empty", func() {
				It("returns error", func() {
					valid, err := Verifier.Verify(
						handlers.NewIssuerPublicKey(nil),
						nil,
						[]byte("a digest"),
						&bccsp.IdemixSignerOpts{
							RevocationPublicKey: handlers.NewRevocationPublicKey(nil),
						},
					)
					Expect(err).To(MatchError("invalid signature, it must not be empty"))
					Expect(valid).To(BeFalse())
				})
			})

			Context("and the option is empty", func() {
				It("returns error", func() {
					valid, err := Verifier.Verify(
						handlers.NewIssuerPublicKey(nil),
						[]byte("a signature"),
						[]byte("a digest"),
						nil,
					)
					Expect(err).To(MatchError("invalid options, expected *IdemixSignerOpts"))
					Expect(valid).To(BeFalse())
				})
			})

			Context("and the option is not of type *IdemixSignerOpts", func() {
				It("returns error", func() {
					valid, err := Verifier.Verify(
						handlers.NewIssuerPublicKey(nil),
						[]byte("a signature"),
						[]byte("a digest"),
						&bccsp.IdemixCredentialRequestSignerOpts{},
					)
					Expect(err).To(MatchError("invalid options, expected *IdemixSignerOpts"))
					Expect(valid).To(BeFalse())
				})
			})

			Context("and the option's revocation public key is empty", func() {
				It("returns error", func() {
					valid, err := Verifier.Verify(
						handlers.NewIssuerPublicKey(nil),
						[]byte("fake signature"),
						nil,
						&bccsp.IdemixSignerOpts{},
					)
					Expect(err).To(MatchError("invalid options, expected *revocationPublicKey"))
					Expect(valid).To(BeFalse())
				})
			})

			Context("and the option's revocation public key is not of type *revocationPublicKey", func() {
				It("returns error", func() {
					valid, err := Verifier.Verify(
						handlers.NewIssuerPublicKey(nil),
						[]byte("fake signature"),
						nil,
						&bccsp.IdemixSignerOpts{RevocationPublicKey: handlers.NewUserSecretKey(nil, false)},
					)
					Expect(err).To(MatchError("invalid options, expected *revocationPublicKey"))
					Expect(valid).To(BeFalse())
				})
			})
		})
	})
})
