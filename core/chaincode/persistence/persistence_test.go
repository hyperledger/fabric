/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package persistence_test

import (
	"encoding/hex"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/chaincode/persistence"
	"github.com/hyperledger/fabric/core/chaincode/persistence/mock"
	"github.com/hyperledger/fabric/core/container/ccintf"
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

			_, err = filesystemIO.Stat(path)
			Expect(err).NotTo(HaveOccurred())
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
			hashString     string
		)

		BeforeEach(func() {
			mockReadWriter = &mock.IOReadWriter{}
			mockReadWriter.ReadFileReturns(nil, errors.New("gameball"))
			mockReadWriter.StatReturns(nil, errors.New("ballgame"))
			store = &persistence.Store{
				ReadWriter: mockReadWriter,
			}

			pkgBytes = []byte("testpkg")
			hashString = hex.EncodeToString(util.ComputeSHA256(pkgBytes))
		})

		It("saves a new code package successfully", func() {
			hash, err := store.Save("testcc", "1.0", pkgBytes)
			Expect(err).NotTo(HaveOccurred())
			Expect(hash).To(Equal(util.ComputeSHA256([]byte("testpkg"))))
		})

		Context("when the existing metadata file contains invalid content", func() {
			BeforeEach(func() {
				mockReadWriter.ReadFileReturns([]byte("handball"), nil)
			})

			It("returns an error", func() {
				hash, err := store.Save("testcc", "1.0", pkgBytes)
				Expect(hash).To(BeNil())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("error reading existing chaincode metadata"))
			})
		})

		Context("when the code package was previously installed successfully", func() {
			BeforeEach(func() {
				mockReadWriter.ReadFileReturnsOnCall(0, []byte(`[{"Name":"vuvuzela","Version":"1.0"}]`), nil)
				mockReadWriter.StatReturns(nil, nil)
			})

			It("appends the name and version to the metadata and returns the hash", func() {
				hash, err := store.Save("testcc", "1.0", pkgBytes)
				Expect(err).NotTo(HaveOccurred())
				Expect(hash).To(Equal(util.ComputeSHA256([]byte("testpkg"))))
				Expect(mockReadWriter.WriteFileCallCount()).To(Equal(1))
				metadataPath, metadataJSON, _ := mockReadWriter.WriteFileArgsForCall(0)
				Expect(metadataPath).To(Equal(hashString + ".json"))
				Expect(metadataJSON).To(Equal([]byte(`[{"Name":"vuvuzela","Version":"1.0"},{"Name":"testcc","Version":"1.0"}]`)))
			})
		})

		Context("when a chaincode with the same name and version was previously saved", func() {
			BeforeEach(func() {
				mockReadWriter.ReadFileReturnsOnCall(0, []byte(`[{"Name":"vuvuzela","Version":"1.0"}]`), nil)
				mockReadWriter.StatReturns(nil, nil)
			})

			It("returns an error", func() {
				hash, err := store.Save("vuvuzela", "1.0", pkgBytes)
				Expect(err).To(HaveOccurred())
				Expect(hash).To(BeNil())
				Expect(err.Error()).To(ContainSubstring("chaincode already installed with name 'vuvuzela' and version '1.0'"))
				Expect(mockReadWriter.WriteFileCallCount()).To(Equal(0))
			})
		})

		Context("when writing the metadata file fails", func() {
			BeforeEach(func() {
				mockReadWriter.StatReturns(nil, errors.New("futbol"))
				mockReadWriter.WriteFileReturns(errors.New("soccer"))
			})

			It("returns an error", func() {
				hash, err := store.Save("testcc", "1.0", pkgBytes)
				Expect(hash).To(BeNil())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("error writing metadata file"))
			})
		})

		Context("when writing chaincode install package file fails and the metadata file is removed successfully", func() {
			BeforeEach(func() {
				mockReadWriter.StatReturns(nil, errors.New("futbol1"))
				mockReadWriter.WriteFileReturnsOnCall(1, errors.New("soccer1"))
			})

			It("returns an error", func() {
				hash, err := store.Save("testcc", "1.0", pkgBytes)
				Expect(hash).To(BeNil())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("error writing chaincode install package"))
			})
		})

		Context("when writing the chaincode install package file fails and the metadata remove also fails", func() {
			BeforeEach(func() {
				mockReadWriter.StatReturns(nil, errors.New("futbol2"))
				mockReadWriter.WriteFileReturnsOnCall(1, errors.New("soccer2"))
				mockReadWriter.RemoveReturns(errors.New("gooooool2"))
			})

			It("returns an error", func() {
				hash, err := store.Save("testcc", "1.0", pkgBytes)
				Expect(hash).To(BeNil())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("error writing chaincode install package"))
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
			mockReadWriter.ReadFileReturnsOnCall(1, []byte(`[{"Name":"vuvuzela","Version":"2.0"},{"Name":"airhorn","Version":"3.0"}]`), nil)
			store = &persistence.Store{
				ReadWriter: mockReadWriter,
			}
		})

		It("loads successfully and returns the chaincode names/versions", func() {
			ccInstallPkgBytes, metadata, err := store.Load([]byte("hash"))
			Expect(err).NotTo(HaveOccurred())
			Expect(ccInstallPkgBytes).To(Equal([]byte("cornerkick")))
			Expect(len(metadata)).To(Equal(2))
			Expect(metadata[0].Name).To(Equal("vuvuzela"))
			Expect(metadata[0].Version).To(Equal("2.0"))
			Expect(metadata[1].Name).To(Equal("airhorn"))
			Expect(metadata[1].Version).To(Equal("3.0"))
		})

		Context("when reading the chaincode install package fails", func() {
			BeforeEach(func() {
				mockReadWriter.ReadFileReturnsOnCall(0, nil, errors.New("redcard"))
			})

			It("returns an error", func() {
				ccInstallPkgBytes, metadata, err := store.Load([]byte("hash"))
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("error reading chaincode install package"))
				Expect(len(ccInstallPkgBytes)).To(Equal(0))
				Expect(metadata).To(BeNil())
			})
		})

		Context("when reading the metadata fails", func() {
			BeforeEach(func() {
				mockReadWriter.ReadFileReturnsOnCall(1, nil, errors.New("yellowcard"))
			})

			It("returns an error", func() {
				ccInstallPkgBytes, metadata, err := store.Load([]byte("hash"))
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("error reading metadata"))
				Expect(len(ccInstallPkgBytes)).To(Equal(0))
				Expect(metadata).To(BeNil())
			})
		})

		Context("when unmarshaling the metadata fails", func() {
			BeforeEach(func() {
				mockReadWriter.ReadFileReturnsOnCall(1, nil, nil)
			})

			It("returns an error", func() {
				ccInstallPkgBytes, metadata, err := store.Load([]byte("hash"))
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("error unmarshaling metadata"))
				Expect(len(ccInstallPkgBytes)).To(Equal(0))
				Expect(metadata).To(BeNil())
			})
		})
	})

	Describe("RetrieveHash", func() {
		var (
			mockReadWriter *mock.IOReadWriter
			store          *persistence.Store
		)

		BeforeEach(func() {
			mockReadWriter = &mock.IOReadWriter{}
			mockFileInfo := &mock.OSFileInfo{}
			mockFileInfo.NameReturns(hex.EncodeToString([]byte("hash1")) + ".json")
			mockFileInfo2 := &mock.OSFileInfo{}
			mockFileInfo2.NameReturns(hex.EncodeToString([]byte("hash2")) + ".json")
			mockReadWriter.ReadDirReturns([]os.FileInfo{mockFileInfo, mockFileInfo2}, nil)
			mockReadWriter.ReadFileReturnsOnCall(0, []byte(`[{"Name":"test1","Version":"1.0"}]`), nil)
			mockReadWriter.ReadFileReturnsOnCall(1, []byte(`[{"Name":"test2","Version":"2.0"}]`), nil)
			store = &persistence.Store{
				ReadWriter: mockReadWriter,
			}
		})

		It("retrieves the hash successfully", func() {
			hash, err := store.RetrieveHash(ccintf.CCID("test2-2.0"))
			Expect(err).NotTo(HaveOccurred())
			Expect(hash).To(Equal([]byte("hash2")))
		})

		Context("when reading the directory fails", func() {
			BeforeEach(func() {
				mockReadWriter.ReadDirReturns(nil, errors.New("offsides"))
			})

			It("returns an error", func() {
				hash, err := store.RetrieveHash(ccintf.CCID("test1-1.0"))
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("error reading chaincode directory"))
				Expect(hash).To(BeNil())
			})
		})

		Context("when reading the metadata fails", func() {
			BeforeEach(func() {
				mockReadWriter.ReadFileReturnsOnCall(0, nil, errors.New("handball"))
			})

			It("returns an error", func() {
				hash, err := store.RetrieveHash(ccintf.CCID("test1-1.0"))
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("chaincode install package 'test1-1.0' not found"))
				Expect(hash).To(BeNil())
			})
		})

		Context("when decoding the hash string fails", func() {
			BeforeEach(func() {
				mockFileInfo := &mock.OSFileInfo{}
				mockFileInfo.NameReturns("?.json")
				mockReadWriter.ReadDirReturns([]os.FileInfo{mockFileInfo}, nil)
			})

			It("returns an error", func() {
				hash, err := store.RetrieveHash(ccintf.CCID("test1-1.0"))
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("error decoding hash from hex string: ?"))
				Expect(hash).To(BeNil())
			})
		})

		Context("when reading a different metadata file fails but the desired chaincode metadata file exists", func() {
			BeforeEach(func() {
				mockReadWriter.ReadFileReturnsOnCall(0, nil, errors.New("penaltykick"))
				mockReadWriter.ReadFileReturnsOnCall(1, []byte(`[{"Name":"test2","Version":"2.0"}]`), nil)
			})

			It("returns sucessfully", func() {
				hash, err := store.RetrieveHash(ccintf.CCID("test2-2.0"))
				Expect(err).NotTo(HaveOccurred())
				Expect(hash).To(Equal([]byte("hash2")))
			})
		})

		Context("when no chaincode install package exists with the given name and version", func() {
			It("returns an error", func() {
				hash, err := store.RetrieveHash(ccintf.CCID("test3-1.0"))
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("chaincode install package 'test3-1.0' not found"))
				Expect(hash).To(BeNil())
			})
		})
	})

	Describe("GetInstalledChaincodes", func() {
		var (
			mockReadWriter *mock.IOReadWriter
			store          *persistence.Store
		)

		BeforeEach(func() {
			mockReadWriter = &mock.IOReadWriter{}
			mockFileInfo := &mock.OSFileInfo{}
			mockFileInfo.NameReturns(hex.EncodeToString([]byte("hash1")) + ".json")
			mockFileInfo2 := &mock.OSFileInfo{}
			mockFileInfo2.NameReturns(hex.EncodeToString([]byte("hash2")) + ".json")
			mockReadWriter.ReadDirReturns([]os.FileInfo{mockFileInfo, mockFileInfo2}, nil)
			mockReadWriter.ReadFileReturnsOnCall(0, []byte(`[{"Name":"test1","Version":"1.0"},{"Name":"test1","Version":"2.0"}]`), nil)
			mockReadWriter.ReadFileReturnsOnCall(1, []byte(`[{"Name":"test2","Version":"2.0"}]`), nil)
			store = &persistence.Store{
				ReadWriter: mockReadWriter,
			}
		})

		It("returns the list of installed chaincodes", func() {
			installedChaincodes, err := store.ListInstalledChaincodes()
			Expect(err).NotTo(HaveOccurred())
			Expect(len(installedChaincodes)).To(Equal(3))
		})

		Context("when the hash cannot be decoded from the filename", func() {
			BeforeEach(func() {
				mockFileInfo := &mock.OSFileInfo{}
				mockFileInfo.NameReturns("?.json")
				mockReadWriter.ReadDirReturns([]os.FileInfo{mockFileInfo}, nil)
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
