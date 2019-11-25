/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package externalbuilder

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/hyperledger/fabric/common/flogging"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
)

var _ = Describe("copy", func() {
	var (
		logger                             *flogging.FabricLogger
		srcRootDir, srcSubDir, destRootDir string
		srcRootFile, srcSubFile            *os.File
		err                                error
	)

	BeforeEach(func() {
		srcRootDir, err = ioutil.TempDir("", "copy-test-")
		Expect(err).NotTo(HaveOccurred())
		srcSubDir, err = ioutil.TempDir(srcRootDir, "sub-")
		Expect(err).NotTo(HaveOccurred())
		srcRootFile, err = ioutil.TempFile(srcRootDir, "file-")
		Expect(err).NotTo(HaveOccurred())
		srcSubFile, err = ioutil.TempFile(srcSubDir, "subfile-")
		Expect(err).NotTo(HaveOccurred())
		err = os.Chmod(srcSubFile.Name(), os.ModePerm)
		Expect(err).NotTo(HaveOccurred())

		logger = flogging.NewFabricLogger(zap.NewNop())
	})

	AfterEach(func() {
		os.RemoveAll(srcRootDir)
		os.RemoveAll(destRootDir)
	})

	When("dest dir does not exist", func() {
		BeforeEach(func() {
			destRootDir, err = ioutil.TempDir("", "dest-")
			Expect(err).NotTo(HaveOccurred())
			err = os.RemoveAll(destRootDir)
			Expect(err).NotTo(HaveOccurred())
		})

		It("make copy by simply moving", func() {
			err = MoveOrCopyDir(logger, srcRootDir, destRootDir)
			Expect(err).NotTo(HaveOccurred())

			_, err = os.Stat(srcRootDir)
			Expect(os.IsNotExist(err)).To(BeTrue())

			_, err = os.Stat(filepath.Join(destRootDir, filepath.Base(srcRootFile.Name())))
			Expect(err).NotTo(HaveOccurred())
			_, err = os.Stat(filepath.Join(destRootDir, filepath.Base(srcSubDir)))
			Expect(err).NotTo(HaveOccurred())
			f, err := os.Stat(filepath.Join(destRootDir, filepath.Base(srcSubDir), filepath.Base(srcSubFile.Name())))
			Expect(err).NotTo(HaveOccurred())
			Expect(f.Mode()).To(Equal(os.ModePerm))
		})
	})

	When("dest dir exits", func() {
		BeforeEach(func() {
			destRootDir, err = ioutil.TempDir("", "dest-")
			Expect(err).NotTo(HaveOccurred())
		})

		It("fails to move and try copy", func() {
			err = MoveOrCopyDir(logger, srcRootDir, destRootDir)
			Expect(err).NotTo(HaveOccurred())

			_, err = os.Stat(srcRootDir)
			Expect(err).NotTo(HaveOccurred())

			_, err = os.Stat(filepath.Join(destRootDir, filepath.Base(srcRootFile.Name())))
			Expect(err).NotTo(HaveOccurred())
			_, err = os.Stat(filepath.Join(destRootDir, filepath.Base(srcSubDir)))
			Expect(err).NotTo(HaveOccurred())
			f, err := os.Stat(filepath.Join(destRootDir, filepath.Base(srcSubDir), filepath.Base(srcSubFile.Name())))
			Expect(err).NotTo(HaveOccurred())
			Expect(f.Mode()).To(Equal(os.ModePerm))
		})
	})
})
