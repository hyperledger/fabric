/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package idemix_test

import (
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/idemix"
	"github.com/hyperledger/fabric/bccsp/idemix/mock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
)

var _ = Describe("Credential Request", func() {

	Describe("when creating a credential request", func() {

		var (
			CredentialRequestSigner *idemix.CredentialRequestSigner
			fakeCredRequest         *mock.CredRequest
		)

		BeforeEach(func() {
			fakeCredRequest = &mock.CredRequest{}
			CredentialRequestSigner = &idemix.CredentialRequestSigner{CredRequest: fakeCredRequest}
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
					idemix.NewUserSecretKey(nil, false),
					nil,
					&bccsp.IdemixCredentialRequestSignerOpts{IssuerPK: idemix.NewIssuerPublicKey(nil)},
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
					idemix.NewUserSecretKey(nil, false),
					nil,
					&bccsp.IdemixCredentialRequestSignerOpts{IssuerPK: idemix.NewIssuerPublicKey(nil)},
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
						&bccsp.IdemixCredentialRequestSignerOpts{IssuerPK: idemix.NewIssuerPublicKey(nil)},
					)
					Expect(err).To(MatchError("invalid key, expected *userSecretKey"))
					Expect(signature).To(BeNil())
				})
			})

			Context("and the user secret key is not of type *userSecretKey", func() {
				It("returns error", func() {
					signature, err := CredentialRequestSigner.Sign(
						idemix.NewIssuerPublicKey(nil),
						nil,
						&bccsp.IdemixCredentialRequestSignerOpts{IssuerPK: idemix.NewIssuerPublicKey(nil)},
					)
					Expect(err).To(MatchError("invalid key, expected *userSecretKey"))
					Expect(signature).To(BeNil())
				})
			})

			Context("and the option is missing", func() {
				It("returns error", func() {
					signature, err := CredentialRequestSigner.Sign(
						idemix.NewUserSecretKey(nil, false),
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
						idemix.NewUserSecretKey(nil, false),
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
						idemix.NewUserSecretKey(nil, false),
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
						idemix.NewUserSecretKey(nil, false),
						nil,
						&bccsp.IdemixCredentialRequestSignerOpts{IssuerPK: idemix.NewUserSecretKey(nil, false)},
					)
					Expect(err).To(MatchError("invalid options, expected IssuerPK as *issuerPublicKey"))
					Expect(signature).To(BeNil())
				})
			})

			Context("and the digest is not nil", func() {
				It("returns error", func() {
					signature, err := CredentialRequestSigner.Sign(
						idemix.NewUserSecretKey(nil, false),
						[]byte{1, 2, 3, 4},
						&bccsp.IdemixCredentialRequestSignerOpts{IssuerPK: idemix.NewIssuerPublicKey(nil)},
					)
					Expect(err).To(MatchError("invalid digest, it must be empty"))
					Expect(signature).To(BeNil())
				})
			})

		})
	})

	Describe("when verifying a credential request", func() {

		var (
			CredentialRequestVerifier *idemix.CredentialRequestVerifier
			fakeCredRequest           *mock.CredRequest
		)

		BeforeEach(func() {
			fakeCredRequest = &mock.CredRequest{}
			CredentialRequestVerifier = &idemix.CredentialRequestVerifier{CredRequest: fakeCredRequest}
		})

		Context("and the underlying cryptographic algorithm succeed", func() {
			BeforeEach(func() {
				fakeCredRequest.VerifyReturns(nil)
			})

			It("returns no error and valid signature", func() {
				valid, err := CredentialRequestVerifier.Verify(
					idemix.NewIssuerPublicKey(nil),
					[]byte("fake signature"),
					nil,
					nil,
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
					idemix.NewIssuerPublicKey(nil),
					[]byte("fake signature"),
					nil,
					nil,
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
						nil,
					)
					Expect(err).To(MatchError("invalid key, expected *issuerPublicKey"))
					Expect(valid).To(BeFalse())
				})
			})

			Context("and the issuer public key is not of type *issuerPublicKey", func() {
				It("returns error", func() {
					valid, err := CredentialRequestVerifier.Verify(
						idemix.NewUserSecretKey(nil, false),
						[]byte("fake signature"),
						nil,
						nil,
					)
					Expect(err).To(MatchError("invalid key, expected *issuerPublicKey"))
					Expect(valid).To(BeFalse())
				})
			})

			Context("and the digest is not nil", func() {
				It("returns error", func() {
					valid, err := CredentialRequestVerifier.Verify(
						idemix.NewIssuerPublicKey(nil),
						[]byte("fake signature"),
						[]byte{1, 2, 3, 4},
						nil,
					)
					Expect(err).To(MatchError("invalid digest, it must be empty"))
					Expect(valid).To(BeFalse())
				})
			})

		})
	})
})
