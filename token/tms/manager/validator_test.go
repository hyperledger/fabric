/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package manager_test

import (
	"github.com/hyperledger/fabric/protos/token"
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
		tokenOwnerValidator      *manager.FabricTokenOwnerValidator
	)

	BeforeEach(func() {
		fakeCreatorInfo = &mockid.PublicInfo{}
		fakeIdentityDeserializer = &mockid.Deserializer{}
		fakeIdentity = &mockid.Identity{}

		policyValidator = &manager.AllIssuingValidator{
			Deserializer: fakeIdentityDeserializer,
		}
		tokenOwnerValidator = &manager.FabricTokenOwnerValidator{
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

	Describe("Token Owner Validation", func() {

		Context("when owner is valid", func() {
			BeforeEach(func() {
				fakeIdentityDeserializer.DeserializeIdentityReturns(fakeIdentity, nil)
			})
			It("returns nil", func() {
				err := tokenOwnerValidator.Validate(&token.TokenOwner{Type: token.TokenOwner_MSP_IDENTIFIER, Raw: []byte{0, 1, 2, 3}})
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("when owner is nil", func() {
			It("returns no error", func() {
				err := tokenOwnerValidator.Validate(nil)
				Expect(err).To(MatchError("identity cannot be nil"))
			})
		})

		Context("when owner has an invalid type", func() {
			It("returns no error", func() {
				err := tokenOwnerValidator.Validate(&token.TokenOwner{Type: 2})
				Expect(err).To(MatchError("identity's type '2' not recognized"))
			})
		})

		Context("when the owner cannot be deserialized", func() {
			BeforeEach(func() {
				fakeIdentityDeserializer.DeserializeIdentityReturns(nil, errors.New("Deserialize, no-way-man"))
			})

			It("returns an error", func() {
				err := tokenOwnerValidator.Validate(&token.TokenOwner{Type: token.TokenOwner_MSP_IDENTIFIER, Raw: []byte{0, 1, 2, 3}})
				Expect(err.Error()).To(BeEquivalentTo("identity [0x7261773a225c3030305c3030315c3030325c3030332220] cannot be deserialised: Deserialize, no-way-man"))
			})
		})

		Context("when identity validation fail", func() {
			BeforeEach(func() {
				fakeIdentity.ValidateReturns(errors.New("Validate, no-way-man"))
				fakeIdentityDeserializer.DeserializeIdentityReturns(fakeIdentity, nil)
			})

			It("returns an error", func() {
				err := tokenOwnerValidator.Validate(&token.TokenOwner{Type: token.TokenOwner_MSP_IDENTIFIER, Raw: []byte{0, 1, 2, 3}})
				Expect(err.Error()).To(BeEquivalentTo("identity [0x7261773a225c3030305c3030315c3030325c3030332220] cannot be validated: Validate, no-way-man"))
			})

		})

	})
})
