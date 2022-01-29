/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package persistence_test

import (
	"io/ioutil"

	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/core/chaincode/persistence"
	"github.com/hyperledger/fabric/core/chaincode/persistence/mock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	tm "github.com/stretchr/testify/mock"
)

var _ = Describe("FallbackPackageLocator", func() {
	var (
		cpl               *persistence.ChaincodePackageLocator
		fakeLegacyLocator *mock.LegacyCCPackageLocator
		fpl               *persistence.FallbackPackageLocator
	)

	BeforeEach(func() {
		cpl = &persistence.ChaincodePackageLocator{
			ChaincodeDir: "testdata",
		}
		fakeLegacyLocator = &mock.LegacyCCPackageLocator{}
		fpl = &persistence.FallbackPackageLocator{
			ChaincodePackageLocator: cpl,
			LegacyCCPackageLocator:  fakeLegacyLocator,
		}
	})

	Describe("GetChaincodePackage", func() {
		It("gets the chaincode package metadata and stream", func() {
			md, mdBytes, stream, err := fpl.GetChaincodePackage("good-package")
			Expect(err).NotTo(HaveOccurred())
			defer stream.Close()
			Expect(md).To(Equal(&persistence.ChaincodePackageMetadata{
				Type:  "Fake-Type",
				Path:  "Fake-Path",
				Label: "Real-Label",
			}))
			Expect(mdBytes).To(MatchJSON(`{"type":"Fake-Type","path":"Fake-Path","label":"Real-Label","extra_field":"extra-field-value"}`))
			code, err := ioutil.ReadAll(stream)
			Expect(err).NotTo(HaveOccurred())
			Expect(code).To(Equal([]byte("package")))
			Expect(fakeLegacyLocator.GetChaincodeDepSpecCallCount()).To(Equal(0))
		})

		Context("when the package has bad metadata", func() {
			It("wraps and returns the error", func() {
				_, _, _, err := fpl.GetChaincodePackage("bad-metadata")
				Expect(err).To(MatchError(ContainSubstring("error retrieving chaincode package metadata 'bad-metadata'")))
			})
		})

		Context("when the package has bad code", func() {
			It("wraps and returns the error", func() {
				_, _, _, err := fpl.GetChaincodePackage("missing-codepackage")
				Expect(err).To(MatchError(ContainSubstring("error retrieving chaincode package code 'missing-codepackage'")))
			})
		})

		Context("when the package is not in the new package store", func() {
			BeforeEach(func() {
				fakeLegacyLocator.GetChaincodeDepSpecReturns(
					&pb.ChaincodeDeploymentSpec{
						ChaincodeSpec: &pb.ChaincodeSpec{
							ChaincodeId: &pb.ChaincodeID{
								Path: "legacy-path",
							},
							Type: pb.ChaincodeSpec_GOLANG,
						},
						CodePackage: []byte("legacy-code"),
					},
					nil)
			})

			It("falls back to the legacy retriever", func() {
				md, mdBytes, stream, err := fpl.GetChaincodePackage("legacy-package")
				Expect(err).NotTo(HaveOccurred())
				defer stream.Close()
				Expect(md).To(Equal(&persistence.ChaincodePackageMetadata{
					Path: "legacy-path",
					Type: "GOLANG",
				}))
				Expect(mdBytes).To(MatchJSON(`{"type":"GOLANG","path":"legacy-path","label":""}`))
				code, err := ioutil.ReadAll(stream)
				Expect(err).NotTo(HaveOccurred())
				Expect(code).To(Equal([]byte("legacy-code")))
			})

			Context("when the legacy provider returns an error", func() {
				BeforeEach(func() {
					fakeLegacyLocator.GetChaincodeDepSpecReturns(nil, errors.Errorf("fake-error"))
				})

				It("wraps and returns the error", func() {
					_, _, _, err := fpl.GetChaincodePackage("legacy-package")
					Expect(err).To(MatchError("could not get legacy chaincode package 'legacy-package': fake-error"))
				})
			})
		})
	})
})

var _ = Describe("ChaincodePackageParser", func() {
	var (
		mockMetaProvider *mock.MetadataProvider
		ccpp             persistence.ChaincodePackageParser
	)

	BeforeEach(func() {
		mockMetaProvider = &mock.MetadataProvider{}
		mockMetaProvider.On("GetDBArtifacts", tm.Anything).Return([]byte("DB artefacts"), nil)

		ccpp.MetadataProvider = mockMetaProvider
	})

	Describe("ParseChaincodePackage", func() {
		It("parses a chaincode package", func() {
			data, err := ioutil.ReadFile("testdata/good-package.tar.gz")
			Expect(err).NotTo(HaveOccurred())

			ccPackage, err := ccpp.Parse(data)
			Expect(err).NotTo(HaveOccurred())
			Expect(ccPackage.Metadata).To(Equal(&persistence.ChaincodePackageMetadata{
				Type:  "Fake-Type",
				Path:  "Fake-Path",
				Label: "Real-Label",
			}))
			Expect(ccPackage.DBArtifacts).To(Equal([]byte("DB artefacts")))
		})

		Context("when the data is not gzipped", func() {
			It("fails", func() {
				_, err := ccpp.Parse([]byte("bad-data"))
				Expect(err).To(MatchError("error reading as gzip stream: unexpected EOF"))
			})
		})

		Context("when the retrieval of the DB metadata fails", func() {
			BeforeEach(func() {
				mockMetaProvider = &mock.MetadataProvider{}
				mockMetaProvider.On("GetDBArtifacts", tm.Anything).Return(nil, errors.New("not good"))

				ccpp.MetadataProvider = mockMetaProvider
			})

			It("fails", func() {
				data, err := ioutil.ReadFile("testdata/good-package.tar.gz")
				Expect(err).NotTo(HaveOccurred())

				ccPackage, err := ccpp.Parse(data)
				Expect(ccPackage).To(BeNil())
				Expect(err).To(MatchError(ContainSubstring("error retrieving DB artifacts from code package")))
			})
		})

		Context("when the chaincode package metadata is missing", func() {
			It("fails", func() {
				data, err := ioutil.ReadFile("testdata/missing-metadata.tar.gz")
				Expect(err).NotTo(HaveOccurred())

				_, err = ccpp.Parse(data)
				Expect(err).To(MatchError("did not find any package metadata (missing metadata.json)"))
			})
		})

		Context("when the chaincode package metadata is corrupt", func() {
			It("fails", func() {
				data, err := ioutil.ReadFile("testdata/bad-metadata.tar.gz")
				Expect(err).NotTo(HaveOccurred())

				_, err = ccpp.Parse(data)
				Expect(err).To(MatchError("could not unmarshal metadata.json as json: invalid character '\\n' in string literal"))
			})
		})

		Context("when the label is empty or missing", func() {
			It("fails", func() {
				data, err := ioutil.ReadFile("testdata/empty-label.tar.gz")
				Expect(err).NotTo(HaveOccurred())

				_, err = ccpp.Parse(data)
				Expect(err.Error()).To(ContainSubstring("invalid label ''. Label must be non-empty, can only consist of alphanumerics, symbols from '.+-_', and can only begin with alphanumerics"))
			})
		})

		Context("when the label contains forbidden characters", func() {
			It("fails", func() {
				data, err := ioutil.ReadFile("testdata/bad-label.tar.gz")
				Expect(err).NotTo(HaveOccurred())

				_, err = ccpp.Parse(data)
				Expect(err.Error()).To(ContainSubstring("invalid label 'Bad-Label!'. Label must be non-empty, can only consist of alphanumerics, symbols from '.+-_', and can only begin with alphanumerics"))
			})
		})

		Context("when the tar file is corrupted", func() {
			It("fails", func() {
				data, err := ioutil.ReadFile("testdata/corrupted-package.tar.gz")
				Expect(err).NotTo(HaveOccurred())

				_, err = ccpp.Parse(data)
				Expect(err).To(MatchError("could not read Chaincode-Package-Metadata.json from tar: unexpected EOF"))
			})
		})

		Context("when the tar has non-regular files", func() {
			It("fails", func() {
				data, err := ioutil.ReadFile("testdata/non-regular-file.tar.gz")
				Expect(err).NotTo(HaveOccurred())

				_, err = ccpp.Parse(data)
				Expect(err).To(MatchError("tar entry code.tar.gz is not a regular file, type 50"))
			})
		})

		Context("when the tar has a corrupt header entry", func() {
			It("fails", func() {
				data, err := ioutil.ReadFile("testdata/corrupted-header.tar.gz")
				Expect(err).NotTo(HaveOccurred())

				_, err = ccpp.Parse(data)
				Expect(err).To(MatchError("error inspecting next tar header: flate: corrupt input before offset 86"))
			})
		})

		Context("when the tar has too many entries", func() {
			It("logs a warning but otherwise allows it", func() {
				data, err := ioutil.ReadFile("testdata/too-many-files.tar.gz")
				Expect(err).NotTo(HaveOccurred())

				_, err = ccpp.Parse(data)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when the tar is missing a code-package", func() {
			It("fails", func() {
				data, err := ioutil.ReadFile("testdata/missing-codepackage.tar.gz")
				Expect(err).NotTo(HaveOccurred())

				_, err = ccpp.Parse(data)
				Expect(err).To(MatchError("did not find a code package inside the package"))
			})
		})
	})
})

var _ = Describe("ChaincodePackageLocator", func() {
	var locator *persistence.ChaincodePackageLocator

	BeforeEach(func() {
		locator = &persistence.ChaincodePackageLocator{
			ChaincodeDir: "/fake-dir",
		}
	})

	Describe("ChaincodePackageStreamer", func() {
		It("creates a ChaincodePackageStreamer for the given packageID", func() {
			streamer := locator.ChaincodePackageStreamer("test-package")
			Expect(streamer).To(Equal(&persistence.ChaincodePackageStreamer{
				PackagePath: "/fake-dir/test-package.tar.gz",
			}))
		})
	})
})

var _ = Describe("ChaincodePackageStreamer", func() {
	var streamer *persistence.ChaincodePackageStreamer

	BeforeEach(func() {
		streamer = &persistence.ChaincodePackageStreamer{
			PackagePath: "testdata/good-package.tar.gz",
		}
	})

	Describe("Metadata", func() {
		It("reads the metadata from the package", func() {
			md, err := streamer.Metadata()
			Expect(err).NotTo(HaveOccurred())
			Expect(md).To(Equal(&persistence.ChaincodePackageMetadata{
				Type:  "Fake-Type",
				Path:  "Fake-Path",
				Label: "Real-Label",
			}))
		})

		Context("when the metadata file cannot be found", func() {
			BeforeEach(func() {
				streamer.PackagePath = "testdata/missing-metadata.tar.gz"
			})

			It("wraps and returns the error", func() {
				_, err := streamer.Metadata()
				Expect(err).To(MatchError("could not get metadata file: did not find file 'metadata.json' in package"))
			})
		})

		Context("when the metadata file cannot be parsed", func() {
			BeforeEach(func() {
				streamer.PackagePath = "testdata/bad-metadata.tar.gz"
			})

			It("wraps and returns the error", func() {
				_, err := streamer.Metadata()
				Expect(err).To(MatchError("could not parse metadata file: invalid character '\\n' in string literal"))
			})
		})
	})

	Describe("Code", func() {
		It("reads a file from the package", func() {
			code, err := streamer.Code()
			Expect(err).NotTo(HaveOccurred())
			codeBytes, err := ioutil.ReadAll(code)
			code.Close()
			Expect(err).NotTo(HaveOccurred())
			Expect(codeBytes).To(Equal([]byte("package")))
		})

		Context("when the file cannot be found because the code is not a regular file", func() {
			BeforeEach(func() {
				streamer.PackagePath = "testdata/missing-codepackage.tar.gz"
			})

			It("wraps and returns the error", func() {
				_, err := streamer.Code()
				Expect(err).To(MatchError("could not get code package: did not find file 'code.tar.gz' in package"))
			})
		})
	})

	Describe("File", func() {
		It("reads a file from the package", func() {
			code, err := streamer.File("code.tar.gz")
			Expect(err).NotTo(HaveOccurred())
			codeBytes, err := ioutil.ReadAll(code)
			code.Close()
			Expect(err).NotTo(HaveOccurred())
			Expect(codeBytes).To(Equal([]byte("package")))
		})

		Context("when the file is not a regular file", func() {
			BeforeEach(func() {
				streamer.PackagePath = "testdata/non-regular-file.tar.gz"
			})

			It("wraps and returns the error", func() {
				_, err := streamer.File("code.tar.gz")
				Expect(err).To(MatchError("tar entry code.tar.gz is not a regular file, type 50"))
			})
		})

		Context("when the code cannot be found because the archive is corrupt", func() {
			BeforeEach(func() {
				streamer.PackagePath = "testdata/bad-archive.tar.gz"
			})

			It("wraps and returns the error", func() {
				_, err := streamer.File("code.tar.gz")
				Expect(err).To(MatchError("could not open chaincode package at 'testdata/bad-archive.tar.gz': open testdata/bad-archive.tar.gz: no such file or directory"))
			})
		})

		Context("when the code cannot be found because the header is corrupt", func() {
			BeforeEach(func() {
				streamer.PackagePath = "testdata/corrupted-header.tar.gz"
			})

			It("wraps and returns the error", func() {
				_, err := streamer.File("code.tar.gz")
				Expect(err).To(MatchError("error inspecting next tar header: flate: corrupt input before offset 86"))
			})
		})

		Context("when the code cannot be found because the gzip is corrupt", func() {
			BeforeEach(func() {
				streamer.PackagePath = "testdata/corrupted-gzip.tar.gz"
			})

			It("wraps and returns the error", func() {
				_, err := streamer.File("code.tar.gz")
				Expect(err).To(MatchError("error reading as gzip stream: unexpected EOF"))
			})
		})
	})
})
