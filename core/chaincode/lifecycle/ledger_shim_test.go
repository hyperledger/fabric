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

var _ = Describe("LedgerShims", func() {
	var (
		fakeStub *mock.ChaincodeStub
	)

	BeforeEach(func() {
		fakeStub = &mock.ChaincodeStub{}
	})

	Describe("ChaincodePrivateLedgerShim", func() {
		var (
			cls *lifecycle.ChaincodePrivateLedgerShim
		)

		BeforeEach(func() {
			cls = &lifecycle.ChaincodePrivateLedgerShim{
				Stub:       fakeStub,
				Collection: "fake-collection",
			}
		})

		Describe("GetState", func() {
			BeforeEach(func() {
				fakeStub.GetPrivateDataReturns([]byte("fake-value"), fmt.Errorf("fake-getstate-error"))
			})

			It("passes through to the stub private data implementation", func() {
				res, err := cls.GetState("fake-key")
				Expect(res).To(Equal([]byte("fake-value")))
				Expect(err).To(MatchError(fmt.Errorf("fake-getstate-error")))
				Expect(fakeStub.GetPrivateDataCallCount()).To(Equal(1))
				collection, key := fakeStub.GetPrivateDataArgsForCall(0)
				Expect(collection).To(Equal("fake-collection"))
				Expect(key).To(Equal("fake-key"))
			})
		})

		Describe("GetStateHash", func() {
			It("panics", func() {
				Expect(func() { cls.GetStateHash("fake-key") }).To(Panic())
			})
		})

		Describe("PutState", func() {
			BeforeEach(func() {
				fakeStub.PutPrivateDataReturns(fmt.Errorf("fake-putstate-error"))
			})

			It("passes through to the stub private data implementation", func() {
				err := cls.PutState("fake-key", []byte("fake-value"))
				Expect(err).To(MatchError(fmt.Errorf("fake-putstate-error")))
				Expect(fakeStub.PutPrivateDataCallCount()).To(Equal(1))
				collection, key, value := fakeStub.PutPrivateDataArgsForCall(0)
				Expect(collection).To(Equal("fake-collection"))
				Expect(key).To(Equal("fake-key"))
				Expect(value).To(Equal([]byte("fake-value")))
			})
		})

		Describe("DelState", func() {
			BeforeEach(func() {
				fakeStub.DelPrivateDataReturns(fmt.Errorf("fake-delstate-error"))
			})

			It("passes through to the stub private data implementation", func() {
				err := cls.DelState("fake-key")
				Expect(err).To(MatchError(fmt.Errorf("fake-delstate-error")))
				Expect(fakeStub.DelPrivateDataCallCount()).To(Equal(1))
				collection, key := fakeStub.DelPrivateDataArgsForCall(0)
				Expect(collection).To(Equal("fake-collection"))
				Expect(key).To(Equal("fake-key"))
			})
		})
	})
})
