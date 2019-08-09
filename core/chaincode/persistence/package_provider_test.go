/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package persistence_test

import (
	"github.com/hyperledger/fabric/common/chaincode"
	"github.com/hyperledger/fabric/core/chaincode/persistence"
	"github.com/hyperledger/fabric/core/chaincode/persistence/mock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
)

var _ = Describe("PackageProvider", func() {
	Describe("ListInstalledChaincodesLegacy", func() {
		var (
			mockSPP         *mock.StorePackageProvider
			mockLPP         *mock.LegacyPackageProvider
			packageProvider *persistence.PackageProvider
		)

		BeforeEach(func() {
			mockSPP = &mock.StorePackageProvider{}
			mockLPP = &mock.LegacyPackageProvider{}
			installedChaincodesLegacy := []chaincode.InstalledChaincode{
				{
					Name:    "testLegacy",
					Version: "1.0",
					Hash:    []byte("hashLegacy"),
				},
			}
			mockLPP.ListInstalledChaincodesReturns(installedChaincodesLegacy, nil)

			packageProvider = &persistence.PackageProvider{
				Store:    mockSPP,
				LegacyPP: mockLPP,
			}
		})

		It("lists the installed chaincodes successfully", func() {
			installedChaincodes, err := packageProvider.ListInstalledChaincodesLegacy()
			Expect(err).NotTo(HaveOccurred())
			Expect(len(installedChaincodes)).To(Equal(1))
		})

		Context("when listing the installed chaincodes from the legacy package provider fails", func() {
			BeforeEach(func() {
				mockLPP.ListInstalledChaincodesReturns(nil, errors.New("football"))
			})

			It("lists the chaincodes from only the persistence store package provider ", func() {
				installedChaincodes, err := packageProvider.ListInstalledChaincodesLegacy()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("football"))
				Expect(len(installedChaincodes)).To(Equal(0))
			})
		})
	})
})
