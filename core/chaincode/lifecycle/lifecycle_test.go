/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle_test

import (
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/hyperledger/fabric/core/chaincode/lifecycle"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle/mock"
)

var _ = Describe("Lifecycle", func() {
	var (
		l           *lifecycle.Lifecycle
		fakeCCStore *mock.ChaincodeStore
		fakeParser  *mock.PackageParser
	)

	BeforeEach(func() {
		fakeCCStore = &mock.ChaincodeStore{}
		fakeParser = &mock.PackageParser{}

		l = &lifecycle.Lifecycle{
			PackageParser:  fakeParser,
			ChaincodeStore: fakeCCStore,
		}
	})

	Describe("InstallChaincode", func() {
		BeforeEach(func() {
			fakeCCStore.SaveReturns([]byte("fake-hash"), nil)
		})

		It("saves the chaincode", func() {
			hash, err := l.InstallChaincode("name", "version", []byte("cc-package"))
			Expect(err).NotTo(HaveOccurred())
			Expect(hash).To(Equal([]byte("fake-hash")))

			Expect(fakeParser.ParseCallCount()).To(Equal(1))
			Expect(fakeParser.ParseArgsForCall(0)).To(Equal([]byte("cc-package")))

			Expect(fakeCCStore.SaveCallCount()).To(Equal(1))
			name, version, msg := fakeCCStore.SaveArgsForCall(0)
			Expect(name).To(Equal("name"))
			Expect(version).To(Equal("version"))
			Expect(msg).To(Equal([]byte("cc-package")))
		})

		Context("when saving the chaincode fails", func() {
			BeforeEach(func() {
				fakeCCStore.SaveReturns(nil, fmt.Errorf("fake-error"))
			})

			It("wraps and returns the error", func() {
				hash, err := l.InstallChaincode("name", "version", []byte("cc-package"))
				Expect(hash).To(BeNil())
				Expect(err).To(MatchError("could not save cc install package: fake-error"))
			})
		})

		Context("when parsing the chaincode package fails", func() {
			BeforeEach(func() {
				fakeParser.ParseReturns(nil, fmt.Errorf("parse-error"))
			})

			It("wraps and returns the error", func() {
				hash, err := l.InstallChaincode("name", "version", []byte("fake-package"))
				Expect(hash).To(BeNil())
				Expect(err).To(MatchError("could not parse as a chaincode install package: parse-error"))
			})
		})
	})
})
