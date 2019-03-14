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
	"github.com/hyperledger/fabric/core/chaincode/persistence"
	p "github.com/hyperledger/fabric/core/chaincode/persistence/intf"
	"github.com/hyperledger/fabric/core/chaincode/persistence/mock"
	. "github.com/onsi/ginkgo"
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
			err := filesystemIO.WriteFile(path, []byte("test"), 0600)
			Expect(err).NotTo(HaveOccurred())

			_, err = os.Stat(path)
			Expect(err).NotTo(HaveOccurred())
		})

		It("stats a file", func() {
			path := filepath.Join(testDir, "stat")
			err := ioutil.WriteFile(path, []byte("test"), 0600)
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
			err := ioutil.WriteFile(path, []byte("test"), 0600)
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
			err := ioutil.WriteFile(path, []byte("test"), 0600)
			Expect(err).NotTo(HaveOccurred())

			_, err = os.Stat(path)
			Expect(err).NotTo(HaveOccurred())

			fileBytes, err := filesystemIO.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(fileBytes).To(Equal([]byte("test")))
		})

		It("reads a directory", func() {
			path := filepath.Join(testDir, "readdir")
			err := ioutil.WriteFile(path, []byte("test"), 0600)
			Expect(err).NotTo(HaveOccurred())

			_, err = os.Stat(path)
			Expect(err).NotTo(HaveOccurred())

			files, err := filesystemIO.ReadDir(testDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(files)).To(Equal(1))
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
			Expect(packageID).To(Equal(p.PackageID("testcc:3fec0187440286d404241e871b44725310b11aaf43d100b053eae712fcabc66d")))
			Expect(mockReadWriter.WriteFileCallCount()).To(Equal(1))
			pkgDataFile, pkgData, _ := mockReadWriter.WriteFileArgsForCall(0)
			Expect(pkgDataFile).To(Equal("testcc:3fec0187440286d404241e871b44725310b11aaf43d100b053eae712fcabc66d.bin"))
			Expect(pkgData).To(Equal([]byte("testpkg")))
		})

		Context("when the code package was previously installed successfully", func() {
			BeforeEach(func() {
				mockReadWriter.ExistsReturns(true, nil)
			})

			It("does nothing and returns the packageID", func() {
				packageID, err := store.Save("testcc", pkgBytes)
				Expect(err).NotTo(HaveOccurred())
				Expect(packageID).To(Equal(p.PackageID("testcc:3fec0187440286d404241e871b44725310b11aaf43d100b053eae712fcabc66d")))
				Expect(mockReadWriter.WriteFileCallCount()).To(Equal(0))
			})
		})

		Context("when writing the package fails", func() {
			BeforeEach(func() {
				mockReadWriter.WriteFileReturns(errors.New("soccer"))
			})

			It("returns an error", func() {
				packageID, err := store.Save("testcc", pkgBytes)
				Expect(packageID).To(Equal(p.PackageID("")))
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("error writing chaincode install package to testcc:3fec0187440286d404241e871b44725310b11aaf43d100b053eae712fcabc66d.bin: soccer"))
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
			ccInstallPkgBytes, err := store.Load(p.PackageID("hash"))
			Expect(err).NotTo(HaveOccurred())
			Expect(ccInstallPkgBytes).To(Equal([]byte("cornerkick")))
		})

		Context("when the package isn't there", func() {
			BeforeEach(func() {
				mockReadWriter.ExistsReturns(false, nil)
			})

			It("returns an error", func() {
				ccInstallPkgBytes, err := store.Load(p.PackageID("hash"))
				Expect(err).To(HaveOccurred())
				Expect(err).To(Equal(&persistence.CodePackageNotFoundErr{PackageID: p.PackageID("hash")}))
				Expect(err.Error()).To(Equal("chaincode install package 'hash' not found"))
				Expect(len(ccInstallPkgBytes)).To(Equal(0))
			})
		})

		Context("when an IO error occurred during stat", func() {
			BeforeEach(func() {
				mockReadWriter.ExistsReturns(false, errors.New("goodness me!"))
			})

			It("returns an error", func() {
				ccInstallPkgBytes, err := store.Load(p.PackageID("hash"))
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("could not determine whether chaincode install package 'hash' exists: goodness me!"))
				Expect(len(ccInstallPkgBytes)).To(Equal(0))
			})
		})

		Context("when reading the chaincode install package fails", func() {
			BeforeEach(func() {
				mockReadWriter.ReadFileReturnsOnCall(0, nil, errors.New("redcard"))
			})

			It("returns an error", func() {
				ccInstallPkgBytes, err := store.Load(p.PackageID("hash"))
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("error reading chaincode install package"))
				Expect(len(ccInstallPkgBytes)).To(Equal(0))
			})
		})
	})

	Describe("ListInstalledChaincodes", func() {
		var (
			mockReadWriter *mock.IOReadWriter
			store          *persistence.Store
		)

		BeforeEach(func() {
			mockReadWriter = &mock.IOReadWriter{}
			mockFileInfo := &mock.OSFileInfo{}
			mockFileInfo.NameReturns(fmt.Sprintf("%s:%x.bin", "label1", []byte("hash1")))
			mockFileInfo2 := &mock.OSFileInfo{}
			mockFileInfo2.NameReturns(fmt.Sprintf("%s:%x.bin", "label2", []byte("hash2")))
			mockReadWriter.ReadDirReturns([]os.FileInfo{mockFileInfo, mockFileInfo2}, nil)
			store = &persistence.Store{
				ReadWriter: mockReadWriter,
			}
		})

		It("returns the list of installed chaincodes", func() {
			installedChaincodes, err := store.ListInstalledChaincodes()
			Expect(err).NotTo(HaveOccurred())
			Expect(len(installedChaincodes)).To(Equal(2))
			Expect(installedChaincodes[0]).To(Equal(chaincode.InstalledChaincode{
				Hash:      []byte("hash1"),
				Label:     "label1",
				PackageID: p.PackageID("label1:6861736831"),
			}))
			Expect(installedChaincodes[1]).To(Equal(chaincode.InstalledChaincode{
				Hash:      []byte("hash2"),
				Label:     "label2",
				PackageID: p.PackageID("label2:6861736832"),
			}))
		})

		Context("when extraneous files are present", func() {
			BeforeEach(func() {
				mockFileInfo := &mock.OSFileInfo{}
				mockFileInfo.NameReturns(fmt.Sprintf("%s:%x.bin", "label1", []byte("hash1")))
				mockFileInfo2 := &mock.OSFileInfo{}
				mockFileInfo2.NameReturns(fmt.Sprintf("%s:%x.bin", "label2", []byte("hash2")))
				mockFileInfo3 := &mock.OSFileInfo{}
				mockFileInfo3.NameReturns(fmt.Sprintf("%s:%x.bin", "", "Musha rain dum a doo, dum a da"))
				mockFileInfo4 := &mock.OSFileInfo{}
				mockFileInfo4.NameReturns(fmt.Sprintf("%s:%x.bin", "", "barfity:barf.bin"))
				mockReadWriter.ReadDirReturns([]os.FileInfo{mockFileInfo, mockFileInfo2, mockFileInfo3}, nil)
			})

			It("returns the list of installed chaincodes", func() {
				installedChaincodes, err := store.ListInstalledChaincodes()
				Expect(err).NotTo(HaveOccurred())
				Expect(len(installedChaincodes)).To(Equal(2))
				Expect(installedChaincodes[0]).To(Equal(chaincode.InstalledChaincode{
					Hash:      []byte("hash1"),
					Label:     "label1",
					PackageID: p.PackageID("label1:6861736831"),
				}))
				Expect(installedChaincodes[1]).To(Equal(chaincode.InstalledChaincode{
					Hash:      []byte("hash2"),
					Label:     "label2",
					PackageID: p.PackageID("label2:6861736832"),
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
				Expect(len(installedChaincodes)).To(Equal(0))
			})
		})
	})

	Describe("GetChaincodeInstallPath", func() {
		var (
			store *persistence.Store
		)

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
})
