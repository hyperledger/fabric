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
	"github.com/pkg/errors"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
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
			path := filepath.Join(testDir, "read")
			err := ioutil.WriteFile(path, []byte("test"), 0600)
			Expect(err).NotTo(HaveOccurred())

			_, err = os.Stat(path)
			Expect(err).NotTo(HaveOccurred())

			fileBytes, err := filesystemIO.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(fileBytes).To(Equal([]byte("test")))
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
			mockReadWriter.StatReturns(nil, errors.New("gameball"))
			store = &persistence.Store{
				ReadWriter: mockReadWriter,
			}

			pkgBytes = []byte("testpkg")
			hashString = hex.EncodeToString(util.ComputeSHA256(pkgBytes))
		})

		It("saves successfully", func() {
			err := store.Save("testcc", "1.0", pkgBytes)
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when the metadata file already exists", func() {
			BeforeEach(func() {
				mockReadWriter.StatReturnsOnCall(0, nil, nil)
			})

			It("returns an error", func() {
				err := store.Save("testcc", "1.0", pkgBytes)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("chaincode metadata already exists at " + hashString + ".json"))
			})
		})

		Context("when the chaincode install package already exists", func() {
			BeforeEach(func() {
				mockReadWriter.StatReturnsOnCall(0, nil, errors.New("worldcup"))
				mockReadWriter.StatReturnsOnCall(1, nil, nil)
			})

			It("returns an error", func() {
				err := store.Save("testcc", "1.0", pkgBytes)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("ChaincodeInstallPackage already exists at " + hashString + ".bin"))
			})
		})

		Context("when writing the metadata file fails", func() {
			BeforeEach(func() {
				mockReadWriter.StatReturns(nil, errors.New("futbol"))
				mockReadWriter.WriteFileReturns(errors.New("soccer"))
			})

			It("returns an error", func() {
				err := store.Save("testcc", "1.0", pkgBytes)
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
				err := store.Save("testcc", "1.0", pkgBytes)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("error writing chaincode install package"))
			})
		})

		Context("when writing the chaincode install package file fails with the metadata remove also fails", func() {
			BeforeEach(func() {
				mockReadWriter.StatReturns(nil, errors.New("futbol2"))
				mockReadWriter.WriteFileReturnsOnCall(1, errors.New("soccer2"))
				mockReadWriter.RemoveReturns(errors.New("gooooool2"))
			})

			It("returns an error", func() {
				err := store.Save("testcc", "1.0", pkgBytes)
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
			mockReadWriter.ReadFileReturnsOnCall(1, []byte(`{"Name":"vuvuzela","Version":"2.0"}`), nil)
			store = &persistence.Store{
				ReadWriter: mockReadWriter,
			}
		})

		It("loads successfully", func() {
			ccInstallPkgBytes, name, version, err := store.Load([]byte("hash"))
			Expect(err).NotTo(HaveOccurred())
			Expect(ccInstallPkgBytes).To(Equal([]byte("cornerkick")))
			Expect(name).To(Equal("vuvuzela"))
			Expect(version).To(Equal("2.0"))
		})

		Context("when reading the chaincode install package fails", func() {
			BeforeEach(func() {
				mockReadWriter.ReadFileReturnsOnCall(0, nil, errors.New("redcard"))
			})

			It("returns an error", func() {
				ccInstallPkgBytes, name, version, err := store.Load([]byte("hash"))
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("error reading chaincode install package"))
				Expect(len(ccInstallPkgBytes)).To(Equal(0))
				Expect(name).To(Equal(""))
				Expect(version).To(Equal(""))
			})
		})

		Context("when reading the metadata fails", func() {
			BeforeEach(func() {
				mockReadWriter.ReadFileReturnsOnCall(1, nil, errors.New("yellowcard"))
			})

			It("returns an error", func() {
				ccInstallPkgBytes, name, version, err := store.Load([]byte("hash"))
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("error reading metadata"))
				Expect(len(ccInstallPkgBytes)).To(Equal(0))
				Expect(name).To(Equal(""))
				Expect(version).To(Equal(""))
			})
		})

		Context("when unmarshaling the metadata fails", func() {
			BeforeEach(func() {
				mockReadWriter.ReadFileReturnsOnCall(1, nil, nil)
			})

			It("returns an error", func() {
				ccInstallPkgBytes, name, version, err := store.Load([]byte("hash"))
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("error unmarshaling metadata"))
				Expect(len(ccInstallPkgBytes)).To(Equal(0))
				Expect(name).To(Equal(""))
				Expect(version).To(Equal(""))
			})
		})
	})
})
