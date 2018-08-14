/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package persistence_test

import (
	"io/ioutil"

	"github.com/hyperledger/fabric/core/chaincode/persistence"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("ChaincodePackageParser", func() {
	var (
		ccpp persistence.ChaincodePackageParser
	)

	Describe("ParseChaincodePackage", func() {
		It("parses a chaincode package", func() {
			data, err := ioutil.ReadFile("testdata/good-package.tar.gz")
			Expect(err).NotTo(HaveOccurred())

			ccPackage, err := ccpp.Parse(data)
			Expect(err).NotTo(HaveOccurred())
			Expect(ccPackage.Metadata).To(Equal(&persistence.ChaincodePackageMetadata{
				Type: "Fake-Type",
				Path: "Fake-Path",
			}))
		})

		Context("when the data is not gzipped", func() {
			It("fails", func() {
				_, err := ccpp.Parse([]byte("bad-data"))
				Expect(err).To(MatchError("error reading as gzip stream: unexpected EOF"))
			})
		})

		Context("when the chaincode package metadata is missing", func() {
			It("fails", func() {
				data, err := ioutil.ReadFile("testdata/missing-metadata.tar.gz")
				Expect(err).NotTo(HaveOccurred())

				_, err = ccpp.Parse(data)
				Expect(err).To(MatchError("did not find any package metadata (missing Chaincode-Package-Metadata.json)"))
			})
		})

		Context("when the chaincode package metadata is missing", func() {
			It("fails", func() {
				data, err := ioutil.ReadFile("testdata/bad-metadata.tar.gz")
				Expect(err).NotTo(HaveOccurred())

				_, err = ccpp.Parse(data)
				Expect(err).To(MatchError("could not unmarshal Chaincode-Package-Metadata.json as json: invalid character '\\n' in string literal"))
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
				Expect(err).To(MatchError("tar entry fake-code-package.link is not a regular file, type 50"))
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
			It("fails", func() {
				data, err := ioutil.ReadFile("testdata/too-many-files.tar.gz")
				Expect(err).NotTo(HaveOccurred())

				_, err = ccpp.Parse(data)
				Expect(err).To(MatchError("found too many files in archive, cannot identify which file is the code-package"))
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
