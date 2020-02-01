/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package handlers_test

import (
	"crypto/sha256"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/idemix/handlers"
	"github.com/hyperledger/fabric/bccsp/idemix/handlers/mock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
)

var _ = Describe("User", func() {

	var (
		fakeUser          *mock.User
		fakeUserSecretKey bccsp.Key
	)

	BeforeEach(func() {
		fakeUser = &mock.User{}
	})

	Describe("when creating a user key", func() {
		var (
			UserKeyGen *handlers.UserKeyGen
		)

		BeforeEach(func() {
			UserKeyGen = &handlers.UserKeyGen{}
			UserKeyGen.User = fakeUser
		})

		Context("and the underlying cryptographic algorithm succeed", func() {
			var (
				sk            bccsp.Key
				fakeIdemixKey *mock.Big
				SKI           []byte
			)
			BeforeEach(func() {
				fakeIdemixKey = &mock.Big{}
				fakeIdemixKey.BytesReturns([]byte{1, 2, 3, 4}, nil)

				fakeUser.NewKeyReturns(fakeIdemixKey, nil)
				hash := sha256.New()
				hash.Write([]byte{1, 2, 3, 4})
				SKI = hash.Sum(nil)

				fakeUserSecretKey = handlers.NewUserSecretKey(fakeIdemixKey, false)
			})

			AfterEach(func() {
				Expect(sk.Private()).To(BeTrue())
				Expect(sk.Symmetric()).To(BeTrue())
				Expect(sk.SKI()).NotTo(BeNil())
				Expect(sk.SKI()).To(BeEquivalentTo(SKI))

				pk, err := sk.PublicKey()
				Expect(err).To(MatchError("cannot call this method on a symmetric key"))
				Expect(pk).To(BeNil())
			})

			Context("and the secret key is exportable", func() {
				BeforeEach(func() {
					UserKeyGen.Exportable = true
					fakeUserSecretKey = handlers.NewUserSecretKey(fakeIdemixKey, true)
				})

				It("returns no error and a key", func() {
					var err error
					sk, err = UserKeyGen.KeyGen(&bccsp.IdemixUserSecretKeyGenOpts{})
					Expect(err).NotTo(HaveOccurred())
					Expect(sk).To(BeEquivalentTo(fakeUserSecretKey))

					raw, err := sk.Bytes()
					Expect(err).NotTo(HaveOccurred())
					Expect(raw).NotTo(BeNil())
					Expect(raw).To(BeEquivalentTo([]byte{1, 2, 3, 4}))
				})
			})

			Context("and the secret key is not exportable", func() {
				BeforeEach(func() {
					UserKeyGen.Exportable = false
					fakeUserSecretKey = handlers.NewUserSecretKey(fakeIdemixKey, false)
				})

				It("returns no error and a key", func() {
					sk, err := UserKeyGen.KeyGen(&bccsp.IdemixUserSecretKeyGenOpts{})
					Expect(err).NotTo(HaveOccurred())
					Expect(sk).To(BeEquivalentTo(fakeUserSecretKey))

					raw, err := sk.Bytes()
					Expect(err).To(MatchError("not exportable"))
					Expect(raw).To(BeNil())
				})

			})
		})

		Context("and the underlying cryptographic algorithm fails", func() {
			BeforeEach(func() {
				fakeUser.NewKeyReturns(nil, errors.New("new-key error"))
			})

			It("returns an error", func() {
				keyPair, err := UserKeyGen.KeyGen(&bccsp.IdemixUserSecretKeyGenOpts{})
				Expect(err).To(MatchError("new-key error"))
				Expect(keyPair).To(BeNil())
			})
		})

	})

	Describe("when deriving a new pseudonym", func() {
		var (
			NymKeyDerivation    *handlers.NymKeyDerivation
			fakeIssuerPublicKey bccsp.Key
		)

		BeforeEach(func() {
			NymKeyDerivation = &handlers.NymKeyDerivation{}
			NymKeyDerivation.User = fakeUser
		})

		Context("and the underlying cryptographic algorithm succeed", func() {
			var (
				nym     bccsp.Key
				userKey *mock.Big
				fakeNym bccsp.Key
				result2 *mock.Big
				result1 *mock.Ecp
			)

			BeforeEach(func() {
				result2 = &mock.Big{}
				result2.BytesReturns([]byte{1, 2, 3, 4}, nil)
				result1 = &mock.Ecp{}
				result1.BytesReturns([]byte{5, 6, 7, 8}, nil)

				fakeUser.MakeNymReturns(result1, result2, nil)
			})

			AfterEach(func() {
				Expect(nym.Private()).To(BeTrue())
				Expect(nym.Symmetric()).To(BeFalse())
				Expect(nym.SKI()).NotTo(BeNil())

				pk, err := nym.PublicKey()
				Expect(err).NotTo(HaveOccurred())

				Expect(pk.Private()).To(BeFalse())
				Expect(pk.Symmetric()).To(BeFalse())
				Expect(pk.SKI()).NotTo(BeNil())
				raw, err := pk.Bytes()
				Expect(err).NotTo(HaveOccurred())
				Expect(raw).NotTo(BeNil())

				pk2, err := pk.PublicKey()
				Expect(err).NotTo(HaveOccurred())
				Expect(pk).To(BeEquivalentTo(pk2))
			})

			Context("and the secret key is exportable", func() {
				BeforeEach(func() {
					var err error
					NymKeyDerivation.Exportable = true
					fakeUserSecretKey = handlers.NewUserSecretKey(userKey, true)
					fakeIssuerPublicKey = handlers.NewIssuerPublicKey(nil)
					fakeNym, err = handlers.NewNymSecretKey(result2, result1, true)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns no error and a key", func() {
					var err error
					nym, err = NymKeyDerivation.KeyDeriv(fakeUserSecretKey, &bccsp.IdemixNymKeyDerivationOpts{IssuerPK: fakeIssuerPublicKey})
					Expect(err).NotTo(HaveOccurred())
					Expect(nym).To(BeEquivalentTo(fakeNym))

					raw, err := nym.Bytes()
					Expect(err).NotTo(HaveOccurred())
					Expect(raw).NotTo(BeNil())
				})
			})

			Context("and the secret key is not exportable", func() {
				BeforeEach(func() {
					var err error
					NymKeyDerivation.Exportable = false
					fakeUserSecretKey = handlers.NewUserSecretKey(userKey, false)
					fakeNym, err = handlers.NewNymSecretKey(result2, result1, false)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns no error and a key", func() {
					var err error
					nym, err = NymKeyDerivation.KeyDeriv(fakeUserSecretKey, &bccsp.IdemixNymKeyDerivationOpts{IssuerPK: fakeIssuerPublicKey})
					Expect(err).NotTo(HaveOccurred())
					Expect(nym).To(BeEquivalentTo(fakeNym))

					raw, err := nym.Bytes()
					Expect(err).To(HaveOccurred())
					Expect(raw).To(BeNil())
				})

			})
		})

		Context("and the underlying cryptographic algorithm fails", func() {
			BeforeEach(func() {
				fakeUserSecretKey = handlers.NewUserSecretKey(nil, true)
				fakeIssuerPublicKey = handlers.NewIssuerPublicKey(nil)
				fakeUser.MakeNymReturns(nil, nil, errors.New("make-nym error"))
			})

			It("returns an error", func() {
				nym, err := NymKeyDerivation.KeyDeriv(fakeUserSecretKey, &bccsp.IdemixNymKeyDerivationOpts{IssuerPK: fakeIssuerPublicKey})
				Expect(err).To(MatchError("make-nym error"))
				Expect(nym).To(BeNil())
			})
		})

		Context("and the options are not well formed", func() {

			Context("and the user secret key is nil", func() {
				It("returns error", func() {
					nym, err := NymKeyDerivation.KeyDeriv(nil, &bccsp.IdemixNymKeyDerivationOpts{})
					Expect(err).To(MatchError("invalid key, expected *userSecretKey"))
					Expect(nym).To(BeNil())
				})
			})

			Context("and the user secret key is not of type *userSecretKey", func() {
				It("returns error", func() {
					nym, err := NymKeyDerivation.KeyDeriv(handlers.NewIssuerPublicKey(nil), &bccsp.IdemixNymKeyDerivationOpts{})
					Expect(err).To(MatchError("invalid key, expected *userSecretKey"))
					Expect(nym).To(BeNil())
				})
			})

			Context("and the option is missing", func() {
				BeforeEach(func() {
					fakeUserSecretKey = handlers.NewUserSecretKey(nil, false)
				})

				It("returns error", func() {
					nym, err := NymKeyDerivation.KeyDeriv(fakeUserSecretKey, nil)
					Expect(err).To(MatchError("invalid options, expected *IdemixNymKeyDerivationOpts"))
					Expect(nym).To(BeNil())
				})
			})

			Context("and the option is not of type *bccsp.IdemixNymKeyDerivationOpts", func() {
				BeforeEach(func() {
					fakeUserSecretKey = handlers.NewUserSecretKey(nil, false)
				})

				It("returns error", func() {
					nym, err := NymKeyDerivation.KeyDeriv(fakeUserSecretKey, &bccsp.AESKeyGenOpts{})
					Expect(err).To(MatchError("invalid options, expected *IdemixNymKeyDerivationOpts"))
					Expect(nym).To(BeNil())
				})
			})

			Context("and the issuer public key is missing", func() {
				BeforeEach(func() {
					fakeUserSecretKey = handlers.NewUserSecretKey(nil, false)
				})

				It("returns error", func() {
					nym, err := NymKeyDerivation.KeyDeriv(fakeUserSecretKey, &bccsp.IdemixNymKeyDerivationOpts{})
					Expect(err).To(MatchError("invalid options, missing issuer public key"))
					Expect(nym).To(BeNil())
				})

			})

			Context("and the issuer public key is not of type *issuerPublicKey", func() {
				BeforeEach(func() {
					fakeUserSecretKey = handlers.NewUserSecretKey(nil, false)
				})

				It("returns error", func() {
					nym, err := NymKeyDerivation.KeyDeriv(fakeUserSecretKey, &bccsp.IdemixNymKeyDerivationOpts{IssuerPK: fakeUserSecretKey})
					Expect(err).To(MatchError("invalid options, expected IssuerPK as *issuerPublicKey"))
					Expect(nym).To(BeNil())
				})

			})
		})
	})

	Context("when importing a user key", func() {
		var (
			UserKeyImporter *handlers.UserKeyImporter
		)

		BeforeEach(func() {
			UserKeyImporter = &handlers.UserKeyImporter{Exportable: true, User: fakeUser}
		})

		Context("and the underlying cryptographic algorithm succeed", func() {

			BeforeEach(func() {
				sk := &mock.Big{}
				sk.BytesReturns([]byte("fake-pk-bytes"), nil)

				fakeUser.NewKeyFromBytesReturns(sk, nil)
			})

			It("import is successful", func() {
				k, err := UserKeyImporter.KeyImport([]byte("fake-raw"), nil)
				Expect(err).NotTo(HaveOccurred())

				bytes, err := k.Bytes()
				Expect(err).NotTo(HaveOccurred())
				Expect(bytes).To(BeEquivalentTo([]byte("fake-pk-bytes")))
			})
		})

		Context("and the underlying cryptographic algorithm fails", func() {

			BeforeEach(func() {
				fakeUser.NewKeyFromBytesReturns(nil, errors.New("new-public-key-nym-import-err"))
			})

			It("returns an error on nil raw", func() {
				k, err := UserKeyImporter.KeyImport(nil, nil)
				Expect(err).To(MatchError("invalid raw, expected byte array"))
				Expect(k).To(BeNil())
			})

			It("returns an error on empty raw", func() {
				k, err := UserKeyImporter.KeyImport([]byte{}, nil)
				Expect(err).To(MatchError("invalid raw, it must not be nil"))
				Expect(k).To(BeNil())
			})

			It("returns an error on invalid raw", func() {
				k, err := UserKeyImporter.KeyImport(UserKeyImporter, nil)
				Expect(err).To(MatchError("invalid raw, expected byte array"))
				Expect(k).To(BeNil())
			})

			It("returns an error", func() {
				k, err := UserKeyImporter.KeyImport([]byte("fake-raw"), nil)
				Expect(err).To(MatchError("new-public-key-nym-import-err"))
				Expect(k).To(BeNil())
			})

		})

	})

	Context("when importing a nym public key", func() {
		var (
			NymPublicKeyImporter *handlers.NymPublicKeyImporter
		)

		BeforeEach(func() {
			NymPublicKeyImporter = &handlers.NymPublicKeyImporter{User: fakeUser}
		})

		Context("and the underlying cryptographic algorithm succeed", func() {

			BeforeEach(func() {
				ecp := &mock.Ecp{}
				ecp.BytesReturns([]byte("fake-pk-bytes"), nil)

				fakeUser.NewPublicNymFromBytesReturns(ecp, nil)
			})

			It("import is successful", func() {
				k, err := NymPublicKeyImporter.KeyImport([]byte("fake-raw"), nil)
				Expect(err).NotTo(HaveOccurred())

				bytes, err := k.Bytes()
				Expect(err).NotTo(HaveOccurred())
				Expect(bytes).To(BeEquivalentTo([]byte("fake-pk-bytes")))
			})
		})

		Context("and the underlying cryptographic algorithm fails", func() {

			BeforeEach(func() {
				fakeUser.NewPublicNymFromBytesReturns(nil, errors.New("new-public-key-nym-import-err"))
			})

			It("returns an error on nil raw", func() {
				k, err := NymPublicKeyImporter.KeyImport(nil, nil)
				Expect(err).To(MatchError("invalid raw, expected byte array"))
				Expect(k).To(BeNil())
			})

			It("returns an error on empty raw", func() {
				k, err := NymPublicKeyImporter.KeyImport([]byte{}, nil)
				Expect(err).To(MatchError("invalid raw, it must not be nil"))
				Expect(k).To(BeNil())
			})

			It("returns an error on invalid raw", func() {
				k, err := NymPublicKeyImporter.KeyImport(NymPublicKeyImporter, nil)
				Expect(err).To(MatchError("invalid raw, expected byte array"))
				Expect(k).To(BeNil())
			})

			It("returns an error", func() {
				k, err := NymPublicKeyImporter.KeyImport([]byte("fake-raw"), nil)
				Expect(err).To(MatchError("new-public-key-nym-import-err"))
				Expect(k).To(BeNil())
			})

		})

	})

})
