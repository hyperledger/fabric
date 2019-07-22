/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package externalbuilders_test

import (
	"io/ioutil"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/container/ccintf"
	"github.com/hyperledger/fabric/core/container/externalbuilders"
)

var _ = Describe("Externalbuilders", func() {
	var (
		codePackage *os.File
		ccci        *ccprovider.ChaincodeContainerInfo
	)

	BeforeEach(func() {
		var err error
		codePackage, err = os.Open("testdata/normal_archive.tar.gz")
		Expect(err).NotTo(HaveOccurred())

		ccci = &ccprovider.ChaincodeContainerInfo{
			PackageID: "fake-package-id",
			Path:      "fake-path",
			Type:      "fake-type",
		}
	})

	AfterEach(func() {
		codePackage.Close()
	})

	Describe("NewBuildContext()", func() {

		It("creates a new context, including temporary locations", func() {
			buildContext, err := externalbuilders.NewBuildContext(ccci, codePackage)
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
				ccci.PackageID = "/"
			})

			It("returns an error", func() {
				_, err := externalbuilders.NewBuildContext(ccci, codePackage)
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
				_, err := externalbuilders.NewBuildContext(ccci, codePackage)
				Expect(err).To(MatchError(ContainSubstring("could not untar source package")))
			})
		})
	})

	Describe("Builder", func() {
		var (
			builder      *externalbuilders.Builder
			buildContext *externalbuilders.BuildContext
		)

		BeforeEach(func() {
			builder = &externalbuilders.Builder{
				Location: "testdata",
			}

			var err error
			buildContext, err = externalbuilders.NewBuildContext(ccci, codePackage)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			buildContext.Cleanup()
		})

		Describe("Detect", func() {
			It("detects when the package is handled by the external builder", func() {
				result := builder.Detect(buildContext)
				Expect(result).To(BeTrue())
			})

			Context("when the builder does not support the package", func() {
				BeforeEach(func() {
					buildContext.CCCI.PackageID = "unsupported-package-id"
				})

				It("returns false", func() {
					result := builder.Detect(buildContext)
					Expect(result).To(BeFalse())
				})
			})
		})

		Describe("Build", func() {
			It("builds the package by invoking external builder", func() {
				err := builder.Build(buildContext)
				Expect(err).NotTo(HaveOccurred())
			})

			Context("when the builder exits with a non-zero status", func() {
				BeforeEach(func() {
					buildContext.CCCI.PackageID = "unsupported-package-id"
				})

				It("returns an error", func() {
					err := builder.Build(buildContext)
					Expect(err).To(MatchError("builder 'testdata' failed: exit status 3"))
				})
			})
		})

		Describe("Launch", func() {
			It("launches the package by invoking external builder", func() {
				err := builder.Launch(buildContext, &ccintf.PeerConnection{
					Address: "fake-peer-address",
					TLSConfig: &ccintf.TLSConfig{
						ClientCert: []byte("fake-client-cert"),
						ClientKey:  []byte("fake-client-key"),
						RootCert:   []byte("fake-root-cert"),
					},
				})
				Expect(err).NotTo(HaveOccurred())

				data1, err := ioutil.ReadFile(filepath.Join(buildContext.LaunchDir, "chaincode.json"))
				Expect(err).NotTo(HaveOccurred())
				Expect(data1).To(Equal([]byte(`{"PeerAddress":"fake-peer-address","ClientCert":"ZmFrZS1jbGllbnQtY2VydA==","ClientKey":"ZmFrZS1jbGllbnQta2V5","RootCert":"ZmFrZS1yb290LWNlcnQ="}`)))
			})

			Context("when the builder exits with a non-zero status", func() {
				BeforeEach(func() {
					buildContext.CCCI.PackageID = "unsupported-package-id"
				})

				It("returns an error", func() {
					err := builder.Build(buildContext)
					Expect(err).To(MatchError("builder 'testdata' failed: exit status 3"))
				})
			})
		})
	})
})
