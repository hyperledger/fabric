/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package handlers_test

import (
	"github.com/hyperledger/fabric/bccsp/idemix/handlers"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/idemix/handlers/mock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
)

var _ = Describe("Issuer", func() {

	Describe("when creating an issuer key-pair", func() {
		var (
			IssuerKeyGen *handlers.IssuerKeyGen

			fakeIssuer      *mock.Issuer
			IssuerSecretKey bccsp.Key
		)

		BeforeEach(func() {
			fakeIssuer = &mock.Issuer{}

			IssuerKeyGen = &handlers.IssuerKeyGen{}
			IssuerKeyGen.Issuer = fakeIssuer
		})

		Context("and the underlying cryptographic algorithm succeed", func() {
			var (
				sk                  bccsp.Key
				fakeIssuerSecretKey *mock.IssuerSecretKey
				SKI                 []byte
				pkBytes             []byte
			)
			BeforeEach(func() {
				SKI = []byte("a fake SKI")
				pkBytes = []byte("a fake public")

				fakeIssuerPublicKey := &mock.IssuerPublicKey{}
				fakeIssuerPublicKey.BytesReturns(pkBytes, nil)
				fakeIssuerPublicKey.HashReturns(SKI)

				fakeIssuerSecretKey = &mock.IssuerSecretKey{}
				fakeIssuerSecretKey.PublicReturns(fakeIssuerPublicKey)
				fakeIssuerSecretKey.BytesReturns([]byte("private"), nil)

				fakeIssuer.NewKeyReturns(fakeIssuerSecretKey, nil)

				IssuerSecretKey = handlers.NewIssuerSecretKey(fakeIssuerSecretKey, false)
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
					IssuerKeyGen.Exportable = true
					IssuerSecretKey = handlers.NewIssuerSecretKey(fakeIssuerSecretKey, true)
				})

				It("returns no error and a key", func() {
					var err error
					sk, err = IssuerKeyGen.KeyGen(&bccsp.IdemixIssuerKeyGenOpts{})
					Expect(err).NotTo(HaveOccurred())
					Expect(sk).To(BeEquivalentTo(IssuerSecretKey))

					raw, err := sk.Bytes()
					Expect(err).NotTo(HaveOccurred())
					Expect(raw).NotTo(BeNil())
					Expect(raw).To(BeEquivalentTo([]byte("private")))
				})
			})

			Context("and the secret key is not exportable", func() {
				BeforeEach(func() {
					IssuerKeyGen.Exportable = false
					IssuerSecretKey = handlers.NewIssuerSecretKey(fakeIssuerSecretKey, false)
				})

				It("returns no error and a key", func() {
					sk, err := IssuerKeyGen.KeyGen(&bccsp.IdemixIssuerKeyGenOpts{})
					Expect(err).NotTo(HaveOccurred())
					Expect(sk).To(BeEquivalentTo(IssuerSecretKey))

					raw, err := sk.Bytes()
					Expect(err).To(MatchError("not exportable"))
					Expect(raw).To(BeNil())
				})

			})
		})

		Context("and the underlying cryptographic algorithm fails", func() {
			BeforeEach(func() {
				fakeIssuer.NewKeyReturns(nil, errors.New("new-key error"))
			})

			It("returns an error", func() {
				keyPair, err := IssuerKeyGen.KeyGen(&bccsp.IdemixIssuerKeyGenOpts{})
				Expect(err).To(MatchError("new-key error"))
				Expect(keyPair).To(BeNil())
			})
		})

		Context("and the options are not well formed", func() {

			Context("and the option is nil", func() {
				It("returns error", func() {
					sk, err := IssuerKeyGen.KeyGen(nil)
					Expect(err).To(MatchError("invalid options, expected *bccsp.IdemixIssuerKeyGenOpts"))
					Expect(sk).To(BeNil())
				})
			})

			Context("and the option is not of type *bccsp.IdemixIssuerKeyGenOpts", func() {
				It("returns error", func() {
					sk, err := IssuerKeyGen.KeyGen(&bccsp.AESKeyGenOpts{})
					Expect(err).To(MatchError("invalid options, expected *bccsp.IdemixIssuerKeyGenOpts"))
					Expect(sk).To(BeNil())
				})
			})
		})
	})

	Describe("when importing an issuer public key", func() {
		var (
			IssuerPublicKeyImporter *handlers.IssuerPublicKeyImporter

			fakeIssuer      *mock.Issuer
			IssuerPublicKey bccsp.Key
		)

		BeforeEach(func() {
			fakeIssuer = &mock.Issuer{}

			IssuerPublicKeyImporter = &handlers.IssuerPublicKeyImporter{}
			IssuerPublicKeyImporter.Issuer = fakeIssuer
		})

		Context("and the underlying cryptographic algorithm succeed", func() {
			var (
				pk                  bccsp.Key
				fakeIssuerPublicKey *mock.IssuerPublicKey
				SKI                 []byte
				pkBytes             []byte
				fakeRaw             []byte
			)

			BeforeEach(func() {
				fakeRaw = []byte("a fake raw")
				SKI = []byte("a fake SKI")
				pkBytes = []byte("a fake public")

				fakeIssuerPublicKey = &mock.IssuerPublicKey{}
				fakeIssuerPublicKey.BytesReturns(pkBytes, nil)
				fakeIssuerPublicKey.HashReturns(SKI)

				fakeIssuer.NewPublicKeyFromBytesReturns(fakeIssuerPublicKey, nil)
			})

			Context("and the secret key is exportable", func() {
				BeforeEach(func() {
					IssuerPublicKey = handlers.NewIssuerPublicKey(fakeIssuerPublicKey)
				})

				It("returns no error and a key", func() {
					var err error
					pk, err = IssuerPublicKeyImporter.KeyImport(fakeRaw, &bccsp.IdemixIssuerPublicKeyImportOpts{})
					Expect(err).NotTo(HaveOccurred())
					Expect(pk).To(BeEquivalentTo(IssuerPublicKey))

					raw, err := pk.Bytes()
					Expect(err).NotTo(HaveOccurred())
					Expect(raw).NotTo(BeNil())
					Expect(raw).To(BeEquivalentTo(pkBytes))

					pk, err := pk.PublicKey()
					Expect(err).NotTo(HaveOccurred())
					Expect(pk).To(BeEquivalentTo(IssuerPublicKey))
				})
			})
		})

		Context("and the underlying cryptographic algorithm fails", func() {
			BeforeEach(func() {
				fakeIssuer.NewPublicKeyFromBytesReturns(nil, errors.New("new-key error"))
			})

			It("returns an error", func() {
				pk, err := IssuerPublicKeyImporter.KeyImport([]byte{1, 2, 3}, &bccsp.IdemixIssuerPublicKeyImportOpts{})
				Expect(err).To(MatchError("new-key error"))
				Expect(pk).To(BeNil())
			})
		})

		Context("and the arguments are not well formed", func() {

			Context("and the raw is nil", func() {
				It("returns error", func() {
					pk, err := IssuerPublicKeyImporter.KeyImport(nil, &bccsp.IdemixIssuerPublicKeyImportOpts{})
					Expect(err).To(MatchError("invalid raw, expected byte array"))
					Expect(pk).To(BeNil())
				})
			})

			Context("and the raw is empty", func() {
				It("returns error", func() {
					pk, err := IssuerPublicKeyImporter.KeyImport([]byte{}, &bccsp.IdemixIssuerPublicKeyImportOpts{})
					Expect(err).To(MatchError("invalid raw, it must not be nil"))
					Expect(pk).To(BeNil())
				})
			})

			Context("and the option is nil", func() {
				It("returns error", func() {
					pk, err := IssuerPublicKeyImporter.KeyImport([]byte{1, 2, 3}, nil)
					Expect(err).To(MatchError("invalid options, expected *bccsp.IdemixIssuerPublicKeyImportOpts"))
					Expect(pk).To(BeNil())
				})
			})

			Context("and the option is not of type *bccsp.IdemixIssuerPublicKeyImportOpts", func() {
				It("returns error", func() {
					pk, err := IssuerPublicKeyImporter.KeyImport([]byte{1, 2, 3}, &bccsp.IdemixNymPublicKeyImportOpts{})
					Expect(err).To(MatchError("invalid options, expected *bccsp.IdemixIssuerPublicKeyImportOpts"))
					Expect(pk).To(BeNil())
				})
			})
		})

	})
})
