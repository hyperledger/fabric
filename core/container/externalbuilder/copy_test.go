/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package externalbuilder

import (
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"runtime"

	"github.com/hyperledger/fabric/common/flogging"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
)

var _ = Describe("copy", func() {
	var (
		logger                  *flogging.FabricLogger
		tempDir                 string
		srcRootDir, destRootDir string
		srcSubDir, destSubDir   string
	)

	BeforeEach(func() {
		var err error
		tempDir, err = ioutil.TempDir("", "copy-test-")
		Expect(err).NotTo(HaveOccurred())

		srcRootDir = filepath.Join(tempDir, "src")
		err = os.Mkdir(srcRootDir, 0o755)
		Expect(err).NotTo(HaveOccurred())

		err = ioutil.WriteFile(filepath.Join(srcRootDir, "file-in-root.txt"), []byte("root file contents"), 0o644)
		Expect(err).NotTo(HaveOccurred())

		err = os.Symlink("file-in-root.txt", filepath.Join(srcRootDir, "symlink-in-root.txt"))
		Expect(err).NotTo(HaveOccurred())

		srcSubDir = filepath.Join(srcRootDir, "subdir")
		err = os.Mkdir(srcSubDir, 0o755)
		Expect(err).NotTo(HaveOccurred())

		err = ioutil.WriteFile(filepath.Join(srcSubDir, "file-in-subdir.txt"), []byte("subdir file contents"), 0o644)
		Expect(err).NotTo(HaveOccurred())

		err = os.Symlink("file-in-subdir.txt", filepath.Join(srcSubDir, "symlink-in-subdir.txt"))
		Expect(err).NotTo(HaveOccurred())
		err = os.Symlink(filepath.Join("..", "file-in-root.txt"), filepath.Join(srcSubDir, "symlink-to-root.txt"))
		Expect(err).NotTo(HaveOccurred())

		destRootDir = filepath.Join(tempDir, "dest")
		destSubDir = filepath.Join(destRootDir, "subdir")

		logger = flogging.NewFabricLogger(zap.NewNop())
	})

	AfterEach(func() {
		os.RemoveAll(tempDir)
	})

	It("copies all files and subdirectories", func() {
		err := CopyDir(logger, srcRootDir, destRootDir)
		Expect(err).NotTo(HaveOccurred())

		Expect(destRootDir).To(BeADirectory())

		Expect(filepath.Join(destRootDir, "file-in-root.txt")).To(BeARegularFile())
		contents, err := ioutil.ReadFile(filepath.Join(destRootDir, "file-in-root.txt"))
		Expect(err).NotTo(HaveOccurred())
		Expect(contents).To(Equal([]byte("root file contents")))

		symlink, err := os.Readlink(filepath.Join(destRootDir, "symlink-in-root.txt"))
		Expect(err).NotTo(HaveOccurred())
		Expect(symlink).To(Equal("file-in-root.txt"))

		Expect(destSubDir).To(BeADirectory())

		Expect(filepath.Join(destSubDir, "file-in-subdir.txt")).To(BeARegularFile())
		contents, err = ioutil.ReadFile(filepath.Join(destSubDir, "file-in-subdir.txt"))
		Expect(err).NotTo(HaveOccurred())
		Expect(contents).To(Equal([]byte("subdir file contents")))

		symlink, err = os.Readlink(filepath.Join(destSubDir, "symlink-in-subdir.txt"))
		Expect(err).NotTo(HaveOccurred())
		Expect(symlink).To(Equal("file-in-subdir.txt"))

		symlink, err = os.Readlink(filepath.Join(destSubDir, "symlink-to-root.txt"))
		Expect(err).NotTo(HaveOccurred())
		Expect(symlink).To(Equal(filepath.Join("..", "file-in-root.txt")))
	})

	When("source contains an absolute symlink", func() {
		It("returns an error and removes the destination directory", func() {
			err := os.Symlink("/somewhere/else", filepath.Join(srcRootDir, "absolute-symlink.txt"))
			Expect(err).NotTo(HaveOccurred())

			err = CopyDir(logger, srcRootDir, destRootDir)
			Expect(err).To(MatchError(ContainSubstring("refusing to copy absolute symlink")))

			Expect(destRootDir).NotTo(BeAnExistingFile())
		})
	})

	When("source contains a relative symlink that points outside of the tree", func() {
		It("returns an error and removes the destination directory", func() {
			err := os.Symlink("../somewhere/else", filepath.Join(srcRootDir, "relative-symlink-outside.txt"))
			Expect(err).NotTo(HaveOccurred())

			err = CopyDir(logger, srcRootDir, destRootDir)
			Expect(err).To(MatchError(ContainSubstring("refusing to copy symlink")))

			Expect(destRootDir).NotTo(BeAnExistingFile())
		})
	})

	When("source contains a relative symlink that goes out and back into the tree", func() {
		It("returns an error and removes the destination directory", func() {
			srcRootName := filepath.Dir(srcRootDir)
			err := os.Symlink(filepath.Join("..", srcRootName, "file-in-root.txt"), filepath.Join(srcRootDir, "relative-symlink-outside-and-inside.txt"))
			Expect(err).NotTo(HaveOccurred())

			err = CopyDir(logger, srcRootDir, destRootDir)
			Expect(err).To(MatchError(ContainSubstring("refusing to copy symlink")))

			Expect(destRootDir).NotTo(BeAnExistingFile())
		})
	})

	When("source contains a relative symlink that points outside of the tree", func() {
		It("returns an error and removes the destination directory", func() {
			err := os.Symlink("../somewhere/else", filepath.Join(srcRootDir, "relative-symlink-outside.txt"))
			Expect(err).NotTo(HaveOccurred())

			err = CopyDir(logger, srcRootDir, destRootDir)
			Expect(err).To(MatchError(ContainSubstring("refusing to copy symlink")))

			Expect(destRootDir).NotTo(BeAnExistingFile())
		})
	})

	When("source contains a file other than a regular file, directory, or symlink", func() {
		It("returns an error and removes the destination directory", func() {
			if runtime.GOOS == "windows" {
				Skip("test not supported on Windows")
			}

			socket, err := net.Listen("unix", filepath.Join(srcRootDir, "uds-in-root.txt"))
			Expect(err).NotTo(HaveOccurred())
			defer socket.Close()

			err = CopyDir(logger, srcRootDir, destRootDir)
			Expect(err).To(MatchError(ContainSubstring("refusing to copy unsupported file")))

			Expect(destRootDir).NotTo(BeAnExistingFile())
		})
	})
})
