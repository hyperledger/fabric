/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package persistence_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/hyperledger/fabric/common/chaincode"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/chaincode/persistence"
	"github.com/hyperledger/fabric/core/chaincode/persistence/mock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
)

var _ = Describe("Persistence", func() {
	Describe("FilesystemWriter", func() {
		var (
			filesystemIO *persistence.FilesystemIO
			testDir      string
		)

		BeforeEach(func() {
			filesystemIO = &persistence.FilesystemIO{}

			var err error
			testDir, err = ioutil.TempDir("", "persistence-test")
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			os.RemoveAll(testDir)
		})

		It("writes a file", func() {
			path := filepath.Join(testDir, "write")
			err := filesystemIO.WriteFile(testDir, "write", []byte("test"))
			Expect(err).NotTo(HaveOccurred())

			_, err = os.Stat(path)
			Expect(err).NotTo(HaveOccurred())
		})

		When("an empty path is supplied to WriteFile", func() {
			It("returns error", func() {
				err := filesystemIO.WriteFile("", "write", []byte("test"))
				Expect(err.Error()).To(Equal("empty path not allowed"))
			})
		})

		It("stats a file", func() {
			path := filepath.Join(testDir, "stat")
			err := ioutil.WriteFile(path, []byte("test"), 0o600)
			Expect(err).NotTo(HaveOccurred())

			exists, err := filesystemIO.Exists(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeTrue())
		})

		It("stats a non-existent file", func() {
			exists, err := filesystemIO.Exists("not quite")
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeFalse())
		})

		It("removes a file", func() {
			path := filepath.Join(testDir, "remove")
			err := ioutil.WriteFile(path, []byte("test"), 0o600)
			Expect(err).NotTo(HaveOccurred())

			_, err = os.Stat(path)
			Expect(err).NotTo(HaveOccurred())

			err = filesystemIO.Remove(path)
			Expect(err).NotTo(HaveOccurred())

			_, err = os.Stat(path)
			Expect(err).To(HaveOccurred())
		})

		It("reads a file", func() {
			path := filepath.Join(testDir, "readfile")
			err := ioutil.WriteFile(path, []byte("test"), 0o600)
			Expect(err).NotTo(HaveOccurred())

			_, err = os.Stat(path)
			Expect(err).NotTo(HaveOccurred())

			fileBytes, err := filesystemIO.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(fileBytes).To(Equal([]byte("test")))
		})

		It("reads a directory", func() {
			path := filepath.Join(testDir, "readdir")
			err := ioutil.WriteFile(path, []byte("test"), 0o600)
			Expect(err).NotTo(HaveOccurred())

			_, err = os.Stat(path)
			Expect(err).NotTo(HaveOccurred())

			files, err := filesystemIO.ReadDir(testDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(files).To(HaveLen(1))
		})

		It("makes a directory (and any necessary parent directories)", func() {
			path := filepath.Join(testDir, "make", "dir")
			err := filesystemIO.MakeDir(path, 0o755)
			Expect(err).NotTo(HaveOccurred())

			_, err = os.Stat(path)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("NewStore", func() {
		var (
			err     error
			tempDir string
			store   *persistence.Store
		)

		BeforeEach(func() {
			tempDir, err = ioutil.TempDir("", "NewStore")
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			os.RemoveAll(tempDir)
		})

		It("creates a persistence store with the specified path and creates the directory on the filesystem", func() {
			store = persistence.NewStore(tempDir)
			Expect(store.Path).To(Equal(tempDir))
			_, err = os.Stat(tempDir)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Initialize", func() {
		var (
			mockReadWriter *mock.IOReadWriter
			store          *persistence.Store
		)

		BeforeEach(func() {
			mockReadWriter = &mock.IOReadWriter{}
			mockReadWriter.ExistsReturns(false, nil)
			mockReadWriter.MakeDirReturns(nil)

			store = &persistence.Store{
				ReadWriter: mockReadWriter,
			}
		})

		It("creates the directory for the persistence store", func() {
			store.Initialize()
			Expect(mockReadWriter.ExistsCallCount()).To(Equal(1))
			Expect(mockReadWriter.MakeDirCallCount()).To(Equal(1))
		})

		Context("when the directory already exists", func() {
			BeforeEach(func() {
				mockReadWriter.ExistsReturns(true, nil)
			})

			It("returns without creating the directory", func() {
				store.Initialize()
				Expect(mockReadWriter.ExistsCallCount()).To(Equal(1))
				Expect(mockReadWriter.MakeDirCallCount()).To(Equal(0))
			})
		})

		Context("when the existence of the directory cannot be determined", func() {
			BeforeEach(func() {
				mockReadWriter.ExistsReturns(false, errors.New("blurg"))
			})

			It("returns without creating the directory", func() {
				Expect(store.Initialize).Should(Panic())
				Expect(mockReadWriter.ExistsCallCount()).To(Equal(1))
				Expect(mockReadWriter.MakeDirCallCount()).To(Equal(0))
			})
		})

		Context("when the directory cannot be created", func() {
			BeforeEach(func() {
				mockReadWriter.MakeDirReturns(errors.New("blarg"))
			})

			It("returns without creating the directory", func() {
				Expect(store.Initialize).Should(Panic())
				Expect(mockReadWriter.ExistsCallCount()).To(Equal(1))
				Expect(mockReadWriter.MakeDirCallCount()).To(Equal(1))
			})
		})
	})

	Describe("Save", func() {
		var (
			mockReadWriter *mock.IOReadWriter
			store          *persistence.Store
			pkgBytes       []byte
		)

		BeforeEach(func() {
			mockReadWriter = &mock.IOReadWriter{}
			mockReadWriter.ExistsReturns(false, nil)
			mockReadWriter.WriteFileReturns(nil)

			store = &persistence.Store{
				ReadWriter: mockReadWriter,
			}

			pkgBytes = []byte("testpkg")
		})

		It("saves a new code package successfully", func() {
			packageID, err := store.Save("testcc", pkgBytes)
			Expect(err).NotTo(HaveOccurred())
			Expect(packageID).To(Equal("testcc:3fec0187440286d404241e871b44725310b11aaf43d100b053eae712fcabc66d"))
			Expect(mockReadWriter.WriteFileCallCount()).To(Equal(1))
			pkgDataFilePath, pkgDataFileName, pkgData := mockReadWriter.WriteFileArgsForCall(0)
			Expect(pkgDataFilePath).To(Equal(""))
			Expect(pkgDataFileName).To(Equal("testcc.3fec0187440286d404241e871b44725310b11aaf43d100b053eae712fcabc66d.tar.gz"))
			Expect(pkgData).To(Equal([]byte("testpkg")))
		})

		Context("when the code package was previously installed successfully", func() {
			BeforeEach(func() {
				mockReadWriter.ExistsReturns(true, nil)
			})

			It("does nothing and returns the packageID", func() {
				packageID, err := store.Save("testcc", pkgBytes)
				Expect(err).NotTo(HaveOccurred())
				Expect(packageID).To(Equal("testcc:3fec0187440286d404241e871b44725310b11aaf43d100b053eae712fcabc66d"))
				Expect(mockReadWriter.WriteFileCallCount()).To(Equal(0))
			})
		})

		Context("when writing the package fails", func() {
			BeforeEach(func() {
				mockReadWriter.WriteFileReturns(errors.New("soccer"))
			})

			It("returns an error", func() {
				packageID, err := store.Save("testcc", pkgBytes)
				Expect(packageID).To(Equal(""))
				Expect(err).To(MatchError(ContainSubstring("error writing chaincode install package to testcc.3fec0187440286d404241e871b44725310b11aaf43d100b053eae712fcabc66d.tar.gz: soccer")))
			})
		})
	})

	Describe("Delete", func() {
		var (
			mockReadWriter *mock.IOReadWriter
			store          *persistence.Store
		)

		BeforeEach(func() {
			mockReadWriter = &mock.IOReadWriter{}
			store = &persistence.Store{
				ReadWriter: mockReadWriter,
				Path:       "foo",
			}
		})

		It("removes the chaincode from the filesystem", func() {
			err := store.Delete("hash")
			Expect(err).NotTo(HaveOccurred())

			Expect(mockReadWriter.RemoveCallCount()).To(Equal(1))
			Expect(mockReadWriter.RemoveArgsForCall(0)).To(Equal("foo/hash.tar.gz"))
		})

		When("remove returns an error", func() {
			BeforeEach(func() {
				mockReadWriter.RemoveReturns(fmt.Errorf("fake-remove-error"))
			})

			It("returns the error", func() {
				err := store.Delete("hash")
				Expect(err).To(MatchError("fake-remove-error"))
			})
		})
	})

	Describe("Load", func() {
		var (
			mockReadWriter *mock.IOReadWriter
			store          *persistence.Store
		)

		BeforeEach(func() {
			mockReadWriter = &mock.IOReadWriter{}
			mockReadWriter.ReadFileReturnsOnCall(0, []byte("cornerkick"), nil)
			mockReadWriter.ExistsReturns(true, nil)
			store = &persistence.Store{
				ReadWriter: mockReadWriter,
			}
		})

		It("loads successfully and returns the chaincode names/versions", func() {
			ccInstallPkgBytes, err := store.Load("hash")
			Expect(err).NotTo(HaveOccurred())
			Expect(ccInstallPkgBytes).To(Equal([]byte("cornerkick")))
		})

		Context("when the package isn't there", func() {
			BeforeEach(func() {
				mockReadWriter.ExistsReturns(false, nil)
			})

			It("returns an error", func() {
				ccInstallPkgBytes, err := store.Load("hash")
				Expect(err).To(Equal(&persistence.CodePackageNotFoundErr{PackageID: "hash"}))
				Expect(err).To(MatchError("chaincode install package 'hash' not found"))
				Expect(ccInstallPkgBytes).To(HaveLen(0))
			})
		})

		Context("when an IO error occurred during stat", func() {
			BeforeEach(func() {
				mockReadWriter.ExistsReturns(false, errors.New("goodness me!"))
			})

			It("returns an error", func() {
				ccInstallPkgBytes, err := store.Load("hash")
				Expect(err).To(MatchError("could not determine whether chaincode install package 'hash' exists: goodness me!"))
				Expect(ccInstallPkgBytes).To(HaveLen(0))
			})
		})

		Context("when reading the chaincode install package fails", func() {
			BeforeEach(func() {
				mockReadWriter.ReadFileReturnsOnCall(0, nil, errors.New("redcard"))
			})

			It("returns an error", func() {
				ccInstallPkgBytes, err := store.Load("hash")
				Expect(err).To(MatchError(ContainSubstring("error reading chaincode install package")))
				Expect(ccInstallPkgBytes).To(HaveLen(0))
			})
		})
	})

	Describe("ListInstalledChaincodes", func() {
		var (
			mockReadWriter *mock.IOReadWriter
			store          *persistence.Store
			hash1, hash2   []byte
		)

		BeforeEach(func() {
			hash1 = util.ComputeSHA256([]byte("hash1"))
			hash2 = util.ComputeSHA256([]byte("hash2"))
			mockReadWriter = &mock.IOReadWriter{}
			mockFileInfo := &mock.OSFileInfo{}
			mockFileInfo.NameReturns(fmt.Sprintf("%s.%x.tar.gz", "label1", hash1))
			mockFileInfo2 := &mock.OSFileInfo{}
			mockFileInfo2.NameReturns(fmt.Sprintf("%s.%x.tar.gz", "label2", hash2))
			mockReadWriter.ReadDirReturns([]os.FileInfo{mockFileInfo, mockFileInfo2}, nil)
			store = &persistence.Store{
				ReadWriter: mockReadWriter,
			}
		})

		It("returns the list of installed chaincodes", func() {
			installedChaincodes, err := store.ListInstalledChaincodes()
			Expect(err).NotTo(HaveOccurred())
			Expect(installedChaincodes).To(HaveLen(2))
			Expect(installedChaincodes[0]).To(Equal(chaincode.InstalledChaincode{
				Hash:      hash1,
				Label:     "label1",
				PackageID: fmt.Sprintf("label1:%x", hash1),
			}))
			Expect(installedChaincodes[1]).To(Equal(chaincode.InstalledChaincode{
				Hash:      hash2,
				Label:     "label2",
				PackageID: fmt.Sprintf("label2:%x", hash2),
			}))
		})

		Context("when extraneous files are present", func() {
			var hash1, hash2 []byte

			BeforeEach(func() {
				hash1 = util.ComputeSHA256([]byte("hash1"))
				hash2 = util.ComputeSHA256([]byte("hash2"))
				mockFileInfo := &mock.OSFileInfo{}
				mockFileInfo.NameReturns(fmt.Sprintf("%s.%x.tar.gz", "label1", hash1))
				mockFileInfo2 := &mock.OSFileInfo{}
				mockFileInfo2.NameReturns(fmt.Sprintf("%s.%x.tar.gz", "label2", hash2))
				mockFileInfo3 := &mock.OSFileInfo{}
				mockFileInfo3.NameReturns(fmt.Sprintf("%s.%x.tar.gz", "", "Musha rain dum a doo, dum a da"))
				mockFileInfo4 := &mock.OSFileInfo{}
				mockFileInfo4.NameReturns(fmt.Sprintf("%s.%x.tar.gz", "", "barfity:barf.tar.gz"))
				mockReadWriter.ReadDirReturns([]os.FileInfo{mockFileInfo, mockFileInfo2, mockFileInfo3}, nil)
			})

			It("returns the list of installed chaincodes", func() {
				installedChaincodes, err := store.ListInstalledChaincodes()
				Expect(err).NotTo(HaveOccurred())
				Expect(installedChaincodes).To(HaveLen(2))
				Expect(installedChaincodes[0]).To(Equal(chaincode.InstalledChaincode{
					Hash:      hash1,
					Label:     "label1",
					PackageID: fmt.Sprintf("label1:%x", hash1),
				}))
				Expect(installedChaincodes[1]).To(Equal(chaincode.InstalledChaincode{
					Hash:      hash2,
					Label:     "label2",
					PackageID: fmt.Sprintf("label2:%x", hash2),
				}))
			})
		})

		Context("when the directory can't be read", func() {
			BeforeEach(func() {
				mockReadWriter.ReadDirReturns([]os.FileInfo{}, errors.New("I'm illiterate and so obviously I can't read"))
			})

			It("returns an error", func() {
				installedChaincodes, err := store.ListInstalledChaincodes()
				Expect(err).To(HaveOccurred())
				Expect(installedChaincodes).To(HaveLen(0))
			})
		})
	})

	Describe("GetChaincodeInstallPath", func() {
		var store *persistence.Store

		BeforeEach(func() {
			store = &persistence.Store{
				Path: "testPath",
			}
		})

		It("returns the path where chaincodes are installed", func() {
			path := store.GetChaincodeInstallPath()
			Expect(path).To(Equal("testPath"))
		})
	})

	DescribeTable("CCFileName",
		func(packageID, expectedName string) {
			Expect(persistence.CCFileName(packageID)).To(Equal(expectedName))
		},
		Entry("label with dot and without hash", "aaa.bbb", "aaa.bbb.tar.gz"),
		Entry("label and hash with colon delimeter", "aaa:bbb", "aaa.bbb.tar.gz"),
		Entry("missing label with colon delimeter", ":bbb", ".bbb.tar.gz"),
		Entry("missing hash with colon delimeter", "aaa:", "aaa..tar.gz"),
	)
})
