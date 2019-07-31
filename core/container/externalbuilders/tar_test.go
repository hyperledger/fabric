/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package externalbuilders_test

import (
	"io/ioutil"
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/hyperledger/fabric/core/container/externalbuilders"
)

var _ = Describe("Tar", func() {
	Describe("Untar", func() {
		var (
			dst string
		)

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
			err = externalbuilders.Untar(file, dst)
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when the archive conains an odd file type", func() {
			It("returns an error", func() {
				file, err := os.Open("testdata/archive_with_symlink.tar.gz")
				defer file.Close()
				Expect(err).NotTo(HaveOccurred())
				err = externalbuilders.Untar(file, dst)
				Expect(err).To(MatchError("invalid file type '50' contained in archive for file 'c.file'"))
			})
		})

		Context("when the archive contains absolute paths", func() {
			It("returns an error", func() {
				file, err := os.Open("testdata/archive_with_absolute.tar.gz")
				Expect(err).NotTo(HaveOccurred())
				defer file.Close()
				err = externalbuilders.Untar(file, dst)
				Expect(err).To(MatchError("tar contains the absolute or escaping path '/home/yellickj/go/src/github.com/hyperledger/fabric/core/chaincode/externalbuilders/testdata/a/test.file'"))
			})
		})

		Context("when the file's directory cannot be created", func() {
			BeforeEach(func() {
				ioutil.WriteFile(dst+"/a", []byte("test"), 0700)
			})

			It("returns an error", func() {
				file, err := os.Open("testdata/normal_archive.tar.gz")
				Expect(err).NotTo(HaveOccurred())
				defer file.Close()
				err = externalbuilders.Untar(file, dst)
				Expect(err).To(MatchError(ContainSubstring("could not create directory 'a'")))
			})
		})

		Context("when the empty directory cannot be created", func() {
			BeforeEach(func() {
				ioutil.WriteFile(dst+"/d", []byte("test"), 0700)
			})

			It("returns an error", func() {
				file, err := os.Open("testdata/normal_archive.tar.gz")
				Expect(err).NotTo(HaveOccurred())
				defer file.Close()
				err = externalbuilders.Untar(file, dst)
				Expect(err).To(MatchError(ContainSubstring("could not create directory 'd/'")))
			})
		})
	})

	Describe("ValidPath()", func() {
		It("validates that a path is relative and a child", func() {
			Expect(externalbuilders.ValidPath("a/simple/path")).To(BeTrue())
			Expect(externalbuilders.ValidPath("../path/to/parent")).To(BeFalse())
			Expect(externalbuilders.ValidPath("a/path/../with/intermediates")).To(BeTrue())
			Expect(externalbuilders.ValidPath("a/path/../../../with/toomanyintermediates")).To(BeFalse())
			Expect(externalbuilders.ValidPath("a/path/with/trailing/../..")).To(BeTrue())
			Expect(externalbuilders.ValidPath("a/path/with/toomanytrailing/../../../../..")).To(BeFalse())
			Expect(externalbuilders.ValidPath("..")).To(BeFalse())
			Expect(externalbuilders.ValidPath("../other")).To(BeFalse())
			Expect(externalbuilders.ValidPath("...")).To(BeTrue())
			Expect(externalbuilders.ValidPath("..foo")).To(BeTrue())
			Expect(externalbuilders.ValidPath("/an/absolute/path")).To(BeFalse())
		})
	})

})
