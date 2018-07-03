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
			filesystemWriter *persistence.FilesystemWriter
			testDir          string
		)

		BeforeEach(func() {
			filesystemWriter = &persistence.FilesystemWriter{}

			var err error
			testDir, err = ioutil.TempDir("", "persistence-test")
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			os.RemoveAll(testDir)
		})

		It("writes a file", func() {
			path := filepath.Join(testDir, "write")
			err := filesystemWriter.WriteFile(path, []byte("test"), 0600)
			Expect(err).NotTo(HaveOccurred())

			_, err = os.Stat(path)
			Expect(err).NotTo(HaveOccurred())
		})

		It("stats a file", func() {
			path := filepath.Join(testDir, "stat")
			err := ioutil.WriteFile(path, []byte("test"), 0600)
			Expect(err).NotTo(HaveOccurred())

			_, err = filesystemWriter.Stat(path)
			Expect(err).NotTo(HaveOccurred())
		})

		It("removes a file", func() {
			path := filepath.Join(testDir, "remove")
			err := ioutil.WriteFile(path, []byte("test"), 0600)
			Expect(err).NotTo(HaveOccurred())

			_, err = os.Stat(path)
			Expect(err).NotTo(HaveOccurred())

			err = filesystemWriter.Remove(path)
			Expect(err).NotTo(HaveOccurred())

			_, err = os.Stat(path)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Save", func() {
		var (
			mockWriter *mock.IOWriter
			store      *persistence.Store
			pkgBytes   []byte
			hashString string
		)

		BeforeEach(func() {
			mockWriter = &mock.IOWriter{}
			mockWriter.StatReturns(nil, errors.New("gameball"))
			store = &persistence.Store{
				Writer: mockWriter,
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
				mockWriter.StatReturnsOnCall(0, nil, nil)
			})

			It("returns an error", func() {
				err := store.Save("testcc", "1.0", pkgBytes)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("chaincode metadata already exists at " + hashString + ".json"))
			})
		})

		Context("when the chaincode install package already exists", func() {
			BeforeEach(func() {
				mockWriter.StatReturnsOnCall(0, nil, errors.New("worldcup"))
				mockWriter.StatReturnsOnCall(1, nil, nil)
			})

			It("returns an error", func() {
				err := store.Save("testcc", "1.0", pkgBytes)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("ChaincodeInstallPackage already exists at " + hashString + ".bin"))
			})
		})

		Context("when writing the metadata file fails", func() {
			BeforeEach(func() {
				mockWriter.StatReturns(nil, errors.New("futbol"))
				mockWriter.WriteFileReturns(errors.New("soccer"))
			})

			It("returns an error", func() {
				err := store.Save("testcc", "1.0", pkgBytes)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("error writing metadata file"))
			})
		})

		Context("when writing chaincode install package file fails and the metadata file is removed successfully", func() {
			BeforeEach(func() {
				mockWriter.StatReturns(nil, errors.New("futbol1"))
				mockWriter.WriteFileReturnsOnCall(1, errors.New("soccer1"))
			})

			It("returns an error", func() {
				err := store.Save("testcc", "1.0", pkgBytes)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("error writing chaincode install package"))
			})
		})

		Context("when writing the chaincode install package file fails with the metadata remove also fails", func() {
			BeforeEach(func() {
				mockWriter.StatReturns(nil, errors.New("futbol2"))
				mockWriter.WriteFileReturnsOnCall(1, errors.New("soccer2"))
				mockWriter.RemoveReturns(errors.New("gooooool2"))
			})

			It("returns an error", func() {
				err := store.Save("testcc", "1.0", pkgBytes)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("error writing chaincode install package"))
			})
		})
	})
})
