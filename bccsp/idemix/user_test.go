/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package idemix_test

import (
	"crypto/sha256"
	"errors"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/idemix"
	"github.com/hyperledger/fabric/bccsp/idemix/mock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("User", func() {

	Describe("when creating an User key", func() {
		var (
			UserKeyGen *idemix.UserKeyGen

			fakeUser          *mock.User
			fakeUserSecretKey bccsp.Key
		)

		BeforeEach(func() {
			fakeUser = &mock.User{}

			UserKeyGen = &idemix.UserKeyGen{}
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

				fakeUserSecretKey = idemix.NewUserSecretKey(fakeIdemixKey, false)
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
					fakeUserSecretKey = idemix.NewUserSecretKey(fakeIdemixKey, true)
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
					fakeUserSecretKey = idemix.NewUserSecretKey(fakeIdemixKey, false)
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

})
