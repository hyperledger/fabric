/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package handlers_test

import (
	"crypto/rand"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/idemix/handlers"
	"github.com/hyperledger/fabric/bccsp/idemix/handlers/mock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
)

var _ = Describe("Credential Request", func() {

	Describe("when creating a credential request", func() {

		var (
			CredentialRequestSigner *handlers.CredentialRequestSigner
			fakeCredRequest         *mock.CredRequest
		)

		BeforeEach(func() {
			fakeCredRequest = &mock.CredRequest{}
			CredentialRequestSigner = &handlers.CredentialRequestSigner{CredRequest: fakeCredRequest}
		})

		Context("and the underlying cryptographic algorithm succeed", func() {
			var (
				fakeSignature []byte
			)
			BeforeEach(func() {
				fakeSignature = []byte("fake signature")
				fakeCredRequest.SignReturns(fakeSignature, nil)
			})

			It("returns no error and a signature", func() {
				signature, err := CredentialRequestSigner.Sign(
					handlers.NewUserSecretKey(nil, false),
					nil,
					&bccsp.IdemixCredentialRequestSignerOpts{IssuerPK: handlers.NewIssuerPublicKey(nil)},
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(signature).To(BeEquivalentTo(fakeSignature))

			})
		})

		Context("and the underlying cryptographic algorithm fails", func() {
			BeforeEach(func() {
				fakeCredRequest.SignReturns(nil, errors.New("sign error"))
			})

			It("returns an error", func() {
				signature, err := CredentialRequestSigner.Sign(
					handlers.NewUserSecretKey(nil, false),
					nil,
					&bccsp.IdemixCredentialRequestSignerOpts{IssuerPK: handlers.NewIssuerPublicKey(nil)},
				)
				Expect(err).To(MatchError("sign error"))
				Expect(signature).To(BeNil())
			})
		})

		Context("and the options are not well formed", func() {

			Context("and the user secret key is nil", func() {
				It("returns error", func() {
					signature, err := CredentialRequestSigner.Sign(
						nil,
						nil,
						&bccsp.IdemixCredentialRequestSignerOpts{IssuerPK: handlers.NewIssuerPublicKey(nil)},
					)
					Expect(err).To(MatchError("invalid key, expected *userSecretKey"))
					Expect(signature).To(BeNil())
				})
			})

			Context("and the user secret key is not of type *userSecretKey", func() {
				It("returns error", func() {
					signature, err := CredentialRequestSigner.Sign(
						handlers.NewIssuerPublicKey(nil),
						nil,
						&bccsp.IdemixCredentialRequestSignerOpts{IssuerPK: handlers.NewIssuerPublicKey(nil)},
					)
					Expect(err).To(MatchError("invalid key, expected *userSecretKey"))
					Expect(signature).To(BeNil())
				})
			})

			Context("and the option is missing", func() {
				It("returns error", func() {
					signature, err := CredentialRequestSigner.Sign(
						handlers.NewUserSecretKey(nil, false),
						nil,
						nil,
					)
					Expect(err).To(MatchError("invalid options, expected *IdemixCredentialRequestSignerOpts"))
					Expect(signature).To(BeNil())
				})
			})

			Context("and the option is not of type *bccsp.IdemixCredentialRequestSignerOpts", func() {
				It("returns error", func() {
					signature, err := CredentialRequestSigner.Sign(
						handlers.NewUserSecretKey(nil, false),
						nil,
						&bccsp.IdemixSignerOpts{},
					)
					Expect(err).To(MatchError("invalid options, expected *IdemixCredentialRequestSignerOpts"))
					Expect(signature).To(BeNil())
				})
			})

			Context("and the issuer public key is missing", func() {
				It("returns error", func() {
					signature, err := CredentialRequestSigner.Sign(
						handlers.NewUserSecretKey(nil, false),
						nil,
						&bccsp.IdemixCredentialRequestSignerOpts{IssuerPK: nil},
					)
					Expect(err).To(MatchError("invalid options, missing issuer public key"))
					Expect(signature).To(BeNil())
				})
			})

			Context("and the issuer public key is not of type *issuerPublicKey", func() {
				It("returns error", func() {
					signature, err := CredentialRequestSigner.Sign(
						handlers.NewUserSecretKey(nil, false),
						nil,
						&bccsp.IdemixCredentialRequestSignerOpts{IssuerPK: handlers.NewUserSecretKey(nil, false)},
					)
					Expect(err).To(MatchError("invalid options, expected IssuerPK as *issuerPublicKey"))
					Expect(signature).To(BeNil())
				})
			})

		})
	})

	Describe("when verifying a credential request", func() {

		var (
			CredentialRequestVerifier *handlers.CredentialRequestVerifier
			IssuerNonce               []byte
			fakeCredRequest           *mock.CredRequest
		)

		BeforeEach(func() {
			fakeCredRequest = &mock.CredRequest{}
			IssuerNonce = make([]byte, 32)
			n, err := rand.Read(IssuerNonce)
			Expect(n).To(BeEquivalentTo(32))
			Expect(err).NotTo(HaveOccurred())
			CredentialRequestVerifier = &handlers.CredentialRequestVerifier{CredRequest: fakeCredRequest}
		})

		Context("and the underlying cryptographic algorithm succeed", func() {
			BeforeEach(func() {
				fakeCredRequest.VerifyReturns(nil)
			})

			It("returns no error and valid signature", func() {
				valid, err := CredentialRequestVerifier.Verify(
					handlers.NewIssuerPublicKey(nil),
					[]byte("fake signature"),
					nil,
					&bccsp.IdemixCredentialRequestSignerOpts{IssuerNonce: IssuerNonce},
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(valid).To(BeTrue())
			})
		})

		Context("and the underlying cryptographic algorithm fails", func() {
			BeforeEach(func() {
				fakeCredRequest.VerifyReturns(errors.New("verify error"))
			})

			It("returns an error", func() {
				valid, err := CredentialRequestVerifier.Verify(
					handlers.NewIssuerPublicKey(nil),
					[]byte("fake signature"),
					nil,
					&bccsp.IdemixCredentialRequestSignerOpts{IssuerNonce: IssuerNonce},
				)
				Expect(err).To(MatchError("verify error"))
				Expect(valid).To(BeFalse())
			})
		})

		Context("and the parameters are not well formed", func() {

			Context("and the issuer public key is nil", func() {
				It("returns error", func() {
					valid, err := CredentialRequestVerifier.Verify(
						nil,
						[]byte("fake signature"),
						nil,
						&bccsp.IdemixCredentialRequestSignerOpts{IssuerNonce: IssuerNonce},
					)
					Expect(err).To(MatchError("invalid key, expected *issuerPublicKey"))
					Expect(valid).To(BeFalse())
				})
			})

			Context("and the issuer public key is not of type *issuerPublicKey", func() {
				It("returns error", func() {
					valid, err := CredentialRequestVerifier.Verify(
						handlers.NewUserSecretKey(nil, false),
						[]byte("fake signature"),
						nil,
						&bccsp.IdemixCredentialRequestSignerOpts{IssuerNonce: IssuerNonce},
					)
					Expect(err).To(MatchError("invalid key, expected *issuerPublicKey"))
					Expect(valid).To(BeFalse())
				})
			})

			Context("and nil options are passed", func() {
				It("returns error", func() {
					valid, err := CredentialRequestVerifier.Verify(
						handlers.NewIssuerPublicKey(nil),
						[]byte("fake signature"),
						nil,
						nil,
					)
					Expect(err).To(MatchError("invalid options, expected *IdemixCredentialRequestSignerOpts"))
					Expect(valid).To(BeFalse())
				})
			})

			Context("and non-valid options are passed", func() {
				It("returns error", func() {
					valid, err := CredentialRequestVerifier.Verify(
						handlers.NewIssuerPublicKey(nil),
						[]byte("fake signature"),
						nil,
						&bccsp.IdemixCRISignerOpts{},
					)
					Expect(err).To(MatchError("invalid options, expected *IdemixCredentialRequestSignerOpts"))
					Expect(valid).To(BeFalse())
				})
			})
		})
	})
})

var _ = Describe("Credential", func() {

	Describe("when creating a credential", func() {

		var (
			CredentialSigner *handlers.CredentialSigner
			fakeCredential   *mock.Credential
		)

		BeforeEach(func() {
			fakeCredential = &mock.Credential{}
			CredentialSigner = &handlers.CredentialSigner{Credential: fakeCredential}
		})

		Context("and the underlying cryptographic algorithm succeed", func() {
			var (
				fakeSignature []byte
			)
			BeforeEach(func() {
				fakeSignature = []byte("fake signature")
				fakeCredential.SignReturns(fakeSignature, nil)
			})

			It("returns no error and a signature", func() {
				signature, err := CredentialSigner.Sign(
					handlers.NewIssuerSecretKey(nil, false),
					nil,
					&bccsp.IdemixCredentialSignerOpts{},
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(signature).To(BeEquivalentTo(fakeSignature))

			})
		})

		Context("and the underlying cryptographic algorithm fails", func() {
			BeforeEach(func() {
				fakeCredential.SignReturns(nil, errors.New("sign error"))
			})

			It("returns an error", func() {
				signature, err := CredentialSigner.Sign(
					handlers.NewIssuerSecretKey(nil, false),
					nil,
					&bccsp.IdemixCredentialSignerOpts{},
				)
				Expect(err).To(MatchError("sign error"))
				Expect(signature).To(BeNil())
			})
		})

		Context("and the parameters are not well formed", func() {

			Context("and the issuer secret key is nil", func() {
				It("returns error", func() {
					signature, err := CredentialSigner.Sign(
						nil,
						nil,
						&bccsp.IdemixCredentialSignerOpts{},
					)
					Expect(err).To(MatchError("invalid key, expected *issuerSecretKey"))
					Expect(signature).To(BeNil())
				})
			})

			Context("and the user secret key is not of type *issuerSecretKey", func() {
				It("returns error", func() {
					signature, err := CredentialSigner.Sign(
						handlers.NewIssuerPublicKey(nil),
						nil,
						&bccsp.IdemixCredentialSignerOpts{},
					)
					Expect(err).To(MatchError("invalid key, expected *issuerSecretKey"))
					Expect(signature).To(BeNil())
				})
			})

			Context("and the opt is nil", func() {
				It("returns error", func() {
					signature, err := CredentialSigner.Sign(
						handlers.NewIssuerSecretKey(nil, false),
						nil,
						nil,
					)
					Expect(err).To(MatchError("invalid options, expected *IdemixCredentialSignerOpts"))
					Expect(signature).To(BeNil())
				})
			})

			Context("and the opt is not of type *IdemixCredentialSignerOpts", func() {
				It("returns error", func() {
					signature, err := CredentialSigner.Sign(
						handlers.NewIssuerSecretKey(nil, false),
						nil,
						&bccsp.IdemixCredentialRequestSignerOpts{},
					)
					Expect(err).To(MatchError("invalid options, expected *IdemixCredentialSignerOpts"))
					Expect(signature).To(BeNil())
				})
			})
		})
	})

	Describe("when verifying a credential", func() {

		var (
			CredentialVerifier *handlers.CredentialVerifier
			fakeCredential     *mock.Credential
		)

		BeforeEach(func() {
			fakeCredential = &mock.Credential{}
			CredentialVerifier = &handlers.CredentialVerifier{Credential: fakeCredential}
		})

		Context("and the underlying cryptographic algorithm succeed", func() {
			BeforeEach(func() {
				fakeCredential.VerifyReturns(nil)
			})

			It("returns no error and valid signature", func() {
				valid, err := CredentialVerifier.Verify(
					handlers.NewUserSecretKey(nil, false),
					[]byte("fake signature"),
					nil,
					&bccsp.IdemixCredentialSignerOpts{IssuerPK: handlers.NewIssuerPublicKey(nil)},
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(valid).To(BeTrue())
			})
		})

		Context("and the underlying cryptographic algorithm fails", func() {
			BeforeEach(func() {
				fakeCredential.VerifyReturns(errors.New("verify error"))
			})

			It("returns an error", func() {
				valid, err := CredentialVerifier.Verify(
					handlers.NewUserSecretKey(nil, false),
					[]byte("fake signature"),
					nil,
					&bccsp.IdemixCredentialSignerOpts{IssuerPK: handlers.NewIssuerPublicKey(nil)},
				)
				Expect(err).To(MatchError("verify error"))
				Expect(valid).To(BeFalse())
			})
		})

		Context("and the parameters are not well formed", func() {

			Context("and the user secret key is nil", func() {
				It("returns error", func() {
					valid, err := CredentialVerifier.Verify(
						nil,
						[]byte("fake signature"),
						nil,
						&bccsp.IdemixCredentialSignerOpts{IssuerPK: handlers.NewIssuerPublicKey(nil)},
					)
					Expect(err).To(MatchError("invalid key, expected *userSecretKey"))
					Expect(valid).To(BeFalse())
				})
			})

			Context("and the user secret key is not of type *userSecretKey", func() {
				It("returns error", func() {
					valid, err := CredentialVerifier.Verify(
						handlers.NewIssuerPublicKey(nil),
						[]byte("fake signature"),
						nil,
						&bccsp.IdemixCredentialSignerOpts{IssuerPK: handlers.NewIssuerPublicKey(nil)},
					)
					Expect(err).To(MatchError("invalid key, expected *userSecretKey"))
					Expect(valid).To(BeFalse())
				})
			})

			Context("and the signature is empty", func() {
				It("returns error", func() {
					valid, err := CredentialVerifier.Verify(
						handlers.NewUserSecretKey(nil, false),
						nil,
						nil,
						&bccsp.IdemixCredentialSignerOpts{IssuerPK: handlers.NewIssuerPublicKey(nil)},
					)
					Expect(err).To(MatchError("invalid signature, it must not be empty"))
					Expect(valid).To(BeFalse())
				})
			})

			Context("and the option is empty", func() {
				It("returns error", func() {
					valid, err := CredentialVerifier.Verify(
						handlers.NewUserSecretKey(nil, false),
						[]byte("fake signature"),
						nil,
						nil,
					)
					Expect(err).To(MatchError("invalid options, expected *IdemixCredentialSignerOpts"))
					Expect(valid).To(BeFalse())
				})
			})

			Context("and the option is not of type *IdemixCredentialSignerOpts", func() {
				It("returns error", func() {
					valid, err := CredentialVerifier.Verify(
						handlers.NewUserSecretKey(nil, false),
						[]byte("fake signature"),
						nil,
						&bccsp.IdemixCredentialRequestSignerOpts{},
					)
					Expect(err).To(MatchError("invalid options, expected *IdemixCredentialSignerOpts"))
					Expect(valid).To(BeFalse())
				})
			})

			Context("and the option's issuer public key is empty", func() {
				It("returns error", func() {
					valid, err := CredentialVerifier.Verify(
						handlers.NewUserSecretKey(nil, false),
						[]byte("fake signature"),
						nil,
						&bccsp.IdemixCredentialSignerOpts{},
					)
					Expect(err).To(MatchError("invalid options, missing issuer public key"))
					Expect(valid).To(BeFalse())
				})
			})

			Context("and the option's issuer public key is not of type *issuerPublicKey", func() {
				It("returns error", func() {
					valid, err := CredentialVerifier.Verify(
						handlers.NewUserSecretKey(nil, false),
						[]byte("fake signature"),
						nil,
						&bccsp.IdemixCredentialSignerOpts{IssuerPK: handlers.NewUserSecretKey(nil, false)},
					)
					Expect(err).To(MatchError("invalid issuer public key, expected *issuerPublicKey"))
					Expect(valid).To(BeFalse())
				})
			})
		})
	})
})
