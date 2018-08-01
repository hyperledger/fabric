/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package persistence_test

import (
	"github.com/hyperledger/fabric/core/chaincode/persistence"
	"github.com/hyperledger/fabric/core/chaincode/persistence/mock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
)

var _ = Describe("PackageProvider", func() {
	var _ = Describe("GetCodePackage", func() {
		var (
			mockSPP         *mock.StorePackageProvider
			mockLPP         *mock.LegacyPackageProvider
			packageProvider *persistence.PackageProvider
		)

		BeforeEach(func() {
			mockSPP = &mock.StorePackageProvider{}
			mockSPP.RetrieveHashReturns([]byte("testcchash"), nil)
			mockSPP.LoadReturns([]byte("storeCode"), "testcc", "1.0", nil)

			mockLPP = &mock.LegacyPackageProvider{}
			mockLPP.GetChaincodeCodePackageReturns([]byte("legacyCode"), nil)

			packageProvider = &persistence.PackageProvider{
				Store:    mockSPP,
				LegacyPP: mockLPP,
			}
		})

		It("gets the code package successfully", func() {
			pkgBytes, err := packageProvider.GetChaincodeCodePackage("testcc", "1.0")
			Expect(err).NotTo(HaveOccurred())
			Expect(pkgBytes).To(Equal([]byte("storeCode")))
		})

		Context("when the code package is not available in the store package provider", func() {
			BeforeEach(func() {
				mockSPP.RetrieveHashReturns(nil, &persistence.CodePackageNotFoundErr{})
			})

			It("gets the code package successfully from the legacy package provider", func() {
				pkgBytes, err := packageProvider.GetChaincodeCodePackage("testcc", "1.0")
				Expect(err).NotTo(HaveOccurred())
				Expect(pkgBytes).To(Equal([]byte("legacyCode")))
			})
		})

		Context("when retrieving the hash from the store package provider fails", func() {
			BeforeEach(func() {
				mockSPP.RetrieveHashReturns(nil, errors.New("chai"))
			})

			It("returns an error", func() {
				pkgBytes, err := packageProvider.GetChaincodeCodePackage("testcc", "1.0")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("error retrieving hash: chai"))
				Expect(pkgBytes).To(BeNil())
			})
		})

		Context("when the code package fails to load from the store package provider", func() {
			BeforeEach(func() {
				mockSPP.LoadReturns(nil, "", "", errors.New("mocha"))
			})

			It("returns an error", func() {
				pkgBytes, err := packageProvider.GetChaincodeCodePackage("testcc", "1.0")
				Expect(err).To(HaveOccurred())
				Expect(pkgBytes).To(BeNil())
			})
		})

		Context("when the code package is not available in either package provider", func() {
			BeforeEach(func() {
				mockSPP.RetrieveHashReturns(nil, &persistence.CodePackageNotFoundErr{})
				mockLPP.GetChaincodeCodePackageReturns(nil, errors.New("latte"))
			})

			It("returns an error", func() {
				pkgBytes, err := packageProvider.GetChaincodeCodePackage("testcc", "1.0")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("code package not found for chaincode with name 'testcc', version '1.0'"))
				Expect(len(pkgBytes)).To(Equal(0))
			})
		})
	})
})
