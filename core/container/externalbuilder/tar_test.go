/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package externalbuilder_test

import (
	"io/ioutil"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hyperledger/fabric/core/container/externalbuilder"
)

var _ = Describe("Tar", func() {
	Describe("Untar", func() {
		var dst string

		BeforeEach(func() {
			var err error
			dst, err = ioutil.TempDir("", "untar-test")
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			os.RemoveAll(dst)
		})

		It("extracts a tar.gz to a destination", func() {
			file, err := os.Open("testdata/normal_archive.tar.gz")
			Expect(err).NotTo(HaveOccurred())
			defer file.Close()
			err = externalbuilder.Untar(file, dst)
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when the archive conains an odd file type", func() {
			It("returns an error", func() {
				file, err := os.Open("testdata/archive_with_symlink.tar.gz")
				Expect(err).NotTo(HaveOccurred())
				defer file.Close()
				err = externalbuilder.Untar(file, dst)
				Expect(err).To(MatchError("invalid file type '50' contained in archive for file 'c.file'"))
			})
		})

		Context("when the archive contains absolute paths", func() {
			It("returns an error", func() {
				file, err := os.Open("testdata/archive_with_absolute.tar.gz")
				Expect(err).NotTo(HaveOccurred())
				defer file.Close()
				err = externalbuilder.Untar(file, dst)
				Expect(err).To(MatchError("tar contains the absolute or escaping path '/home/yellickj/go/src/github.com/hyperledger/fabric/core/chaincode/externalbuilders/testdata/a/test.file'"))
			})
		})

		Context("when the file's directory cannot be created", func() {
			BeforeEach(func() {
				ioutil.WriteFile(dst+"/a", []byte("test"), 0o700)
			})

			It("returns an error", func() {
				file, err := os.Open("testdata/normal_archive.tar.gz")
				Expect(err).NotTo(HaveOccurred())
				defer file.Close()
				err = externalbuilder.Untar(file, dst)
				Expect(err).To(MatchError(ContainSubstring("could not create directory 'a'")))
			})
		})

		Context("when the empty directory cannot be created", func() {
			BeforeEach(func() {
				ioutil.WriteFile(dst+"/d", []byte("test"), 0o700)
			})

			It("returns an error", func() {
				file, err := os.Open("testdata/normal_archive.tar.gz")
				Expect(err).NotTo(HaveOccurred())
				defer file.Close()
				err = externalbuilder.Untar(file, dst)
				Expect(err).To(MatchError(ContainSubstring("could not create directory 'd/'")))
			})
		})
	})

	Describe("ValidPath()", func() {
		It("validates that a path is relative and a child", func() {
			Expect(externalbuilder.ValidPath("a/simple/path")).To(BeTrue())
			Expect(externalbuilder.ValidPath("../path/to/parent")).To(BeFalse())
			Expect(externalbuilder.ValidPath("a/path/../with/intermediates")).To(BeTrue())
			Expect(externalbuilder.ValidPath("a/path/../../../with/toomanyintermediates")).To(BeFalse())
			Expect(externalbuilder.ValidPath("a/path/with/trailing/../..")).To(BeTrue())
			Expect(externalbuilder.ValidPath("a/path/with/toomanytrailing/../../../../..")).To(BeFalse())
			Expect(externalbuilder.ValidPath("..")).To(BeFalse())
			Expect(externalbuilder.ValidPath("../other")).To(BeFalse())
			Expect(externalbuilder.ValidPath("...")).To(BeTrue())
			Expect(externalbuilder.ValidPath("..foo")).To(BeTrue())
			Expect(externalbuilder.ValidPath("/an/absolute/path")).To(BeFalse())
		})
	})
})
