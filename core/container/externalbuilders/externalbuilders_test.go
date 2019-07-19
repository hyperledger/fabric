/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package externalbuilders_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/container/externalbuilders"
)

var _ = Describe("Externalbuilders", func() {
	Describe("Context", func() {
		Describe("NewBuildContext()", func() {
			var (
				cci         *ccprovider.ChaincodeContainerInfo
				codePackage *os.File
			)

			BeforeEach(func() {
				var err error
				codePackage, err = os.Open("testdata/normal_archive.tar.gz")
				Expect(err).NotTo(HaveOccurred())

				cci = &ccprovider.ChaincodeContainerInfo{
					PackageID: "package-id:00000",
				}
			})

			AfterEach(func() {
				codePackage.Close()
			})

			It("creates a new context, including temporary locations", func() {
				buildContext, err := externalbuilders.NewBuildContext(cci, codePackage)
				Expect(err).NotTo(HaveOccurred())
				defer buildContext.Cleanup()

				Expect(buildContext.ScratchDir).NotTo(BeEmpty())
				_, err = os.Stat(buildContext.ScratchDir)
				Expect(err).NotTo(HaveOccurred())

				Expect(buildContext.SourceDir).NotTo(BeEmpty())
				_, err = os.Stat(buildContext.SourceDir)
				Expect(err).NotTo(HaveOccurred())

				_, err = os.Stat(filepath.Join(buildContext.SourceDir, "a/test.file"))
				Expect(err).NotTo(HaveOccurred())
			})

			Context("when the tmp dir cannot be created", func() {
				BeforeEach(func() {
					cci.PackageID = "/"
				})

				It("returns an error", func() {
					_, err := externalbuilders.NewBuildContext(cci, codePackage)
					Expect(err).To(MatchError(ContainSubstring("could not create temp dir")))
				})
			})

			Context("when the archive cannot be extracted", func() {
				BeforeEach(func() {
					var err error
					codePackage.Close()
					codePackage, err = os.Open("testdata/archive_with_absolute.tar.gz")
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns an error", func() {
					_, err := externalbuilders.NewBuildContext(cci, codePackage)
					Expect(err).To(MatchError(ContainSubstring("could not untar source package")))
				})
			})
		})
	})
})
