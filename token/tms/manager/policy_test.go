/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package manager_test

import (
	mockid "github.com/hyperledger/fabric/token/identity/mock"
	"github.com/hyperledger/fabric/token/tms/manager"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
)

var _ = Describe("AllIssuingValidator", func() {
	var (
		fakeCreatorInfo          *mockid.PublicInfo
		fakeIdentityDeserializer *mockid.Deserializer
		fakeIdentity             *mockid.Identity
		policyValidator          *manager.AllIssuingValidator
	)

	BeforeEach(func() {
		fakeCreatorInfo = &mockid.PublicInfo{}
		fakeIdentityDeserializer = &mockid.Deserializer{}
		fakeIdentity = &mockid.Identity{}

		policyValidator = &manager.AllIssuingValidator{
			Deserializer: fakeIdentityDeserializer,
		}
	})

	Describe("Validate", func() {
		Context("when the creator is a member", func() {
			BeforeEach(func() {
				fakeIdentityDeserializer.DeserializeIdentityReturns(fakeIdentity, nil)
			})
			It("returns no error", func() {
				err := policyValidator.Validate(fakeCreatorInfo, "")

				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when the creator cannot be deserialized", func() {
			BeforeEach(func() {
				fakeIdentityDeserializer.DeserializeIdentityReturns(nil, errors.New("Deserialize, no-way-man"))
				fakeCreatorInfo.PublicReturns([]byte{1, 2, 3})
			})

			It("returns an error", func() {
				err := policyValidator.Validate(fakeCreatorInfo, "")

				Expect(err.Error()).To(Equal("identity [0x010203] cannot be deserialised: Deserialize, no-way-man"))
				Expect(fakeIdentityDeserializer.DeserializeIdentityCallCount()).To(Equal(1))
			})
		})

		Context("when identity validation fail", func() {
			BeforeEach(func() {
				fakeIdentity.ValidateReturns(errors.New("Validate, no-way-man"))
				fakeIdentityDeserializer.DeserializeIdentityReturns(fakeIdentity, nil)
				fakeCreatorInfo.PublicReturns([]byte{4, 5, 6})
			})

			It("returns an error", func() {
				err := policyValidator.Validate(fakeCreatorInfo, "")

				Expect(err.Error()).To(Equal("identity [0x040506] cannot be validated: Validate, no-way-man"))
				Expect(fakeIdentityDeserializer.DeserializeIdentityCallCount()).To(Equal(1))
				Expect(fakeIdentity.ValidateCallCount()).To(Equal(1))
			})

		})

	})
})
