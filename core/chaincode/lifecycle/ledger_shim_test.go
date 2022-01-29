/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hyperledger/fabric-protos-go/ledger/queryresult"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle/mock"
)

var _ = Describe("LedgerShims", func() {
	var fakeStub *mock.ChaincodeStub

	BeforeEach(func() {
		fakeStub = &mock.ChaincodeStub{}
	})

	Describe("ChaincodePublicLedgerShim", func() {
		var (
			cls          *lifecycle.ChaincodePublicLedgerShim
			fakeIterator *mock.StateIterator
		)

		BeforeEach(func() {
			fakeIterator = &mock.StateIterator{}
			fakeIterator.HasNextReturnsOnCall(0, true)
			fakeIterator.HasNextReturnsOnCall(1, false)
			fakeIterator.NextReturns(&queryresult.KV{
				Key:   "fake-prefix-key",
				Value: []byte("fake-value"),
			}, nil)

			fakeStub.GetStateByRangeReturns(fakeIterator, nil)

			cls = &lifecycle.ChaincodePublicLedgerShim{
				ChaincodeStubInterface: fakeStub,
			}
		})

		Describe("GetStateRange", func() {
			It("passes through to the stub", func() {
				res, err := cls.GetStateRange("fake-prefix")
				Expect(err).NotTo(HaveOccurred())
				Expect(res).To(Equal(map[string][]byte{
					"fake-prefix-key": []byte("fake-value"),
				}))
				Expect(fakeStub.GetStateByRangeCallCount()).To(Equal(1))
				start, end := fakeStub.GetStateByRangeArgsForCall(0)
				Expect(start).To(Equal("fake-prefix"))
				Expect(end).To(Equal("fake-prefix\x7f"))
			})

			Context("when the iterator cannot be retrieved", func() {
				BeforeEach(func() {
					fakeStub.GetStateByRangeReturns(nil, fmt.Errorf("error-by-range"))
				})

				It("wraps and returns the error", func() {
					_, err := cls.GetStateRange("fake-prefix")
					Expect(err).To(MatchError("could not get state iterator: error-by-range"))
				})
			})

			Context("when the iterator fails to iterate", func() {
				BeforeEach(func() {
					fakeIterator.NextReturns(nil, fmt.Errorf("fake-iterator-error"))
				})

				It("wraps and returns the error", func() {
					_, err := cls.GetStateRange("fake-prefix")
					Expect(err).To(MatchError("could not iterate over range: fake-iterator-error"))
				})
			})
		})
	})

	Describe("ChaincodePrivateLedgerShim", func() {
		var cls *lifecycle.ChaincodePrivateLedgerShim

		BeforeEach(func() {
			cls = &lifecycle.ChaincodePrivateLedgerShim{
				Stub:       fakeStub,
				Collection: "fake-collection",
			}
		})

		Describe("GetStateRange", func() {
			BeforeEach(func() {
				fakeStub.GetPrivateDataByRangeReturns(&mock.StateIterator{}, nil)
			})

			It("passes through to the stub private function", func() {
				res, err := cls.GetStateRange("fake-key")
				Expect(err).NotTo(HaveOccurred())
				Expect(res).To(BeEmpty())

				Expect(fakeStub.GetPrivateDataByRangeCallCount()).To(Equal(1))
				collection, start, end := fakeStub.GetPrivateDataByRangeArgsForCall(0)
				Expect(collection).To(Equal("fake-collection"))
				Expect(start).To(Equal("fake-key"))
				Expect(end).To(Equal("fake-key\x7f"))
			})

			Context("when getting the state iterator fails", func() {
				BeforeEach(func() {
					fakeStub.GetPrivateDataByRangeReturns(nil, fmt.Errorf("fake-range-error"))
				})

				It("wraps and returns the error", func() {
					_, err := cls.GetStateRange("fake-key")
					Expect(err).To(MatchError("could not get state iterator: fake-range-error"))
				})
			})
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
			BeforeEach(func() {
				fakeStub.GetPrivateDataHashReturns([]byte("fake-hash"), fmt.Errorf("fake-error"))
			})

			It("passes through to the chaincode stub", func() {
				res, err := cls.GetStateHash("fake-key")
				Expect(res).To(Equal([]byte("fake-hash")))
				Expect(err).To(MatchError("fake-error"))
				Expect(fakeStub.GetPrivateDataHashCallCount()).To(Equal(1))
				collection, key := fakeStub.GetPrivateDataHashArgsForCall(0)
				Expect(collection).To(Equal("fake-collection"))
				Expect(key).To(Equal("fake-key"))
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

	Describe("SimpleQueryExecutorShim", func() {
		var (
			sqes                    *lifecycle.SimpleQueryExecutorShim
			fakeSimpleQueryExecutor *mock.SimpleQueryExecutor
		)

		BeforeEach(func() {
			fakeSimpleQueryExecutor = &mock.SimpleQueryExecutor{}
			sqes = &lifecycle.SimpleQueryExecutorShim{
				Namespace:           "cc-namespace",
				SimpleQueryExecutor: fakeSimpleQueryExecutor,
			}
		})

		Describe("GetState", func() {
			BeforeEach(func() {
				fakeSimpleQueryExecutor.GetStateReturns([]byte("fake-state"), fmt.Errorf("fake-error"))
			})

			It("passes through to the query executor", func() {
				res, err := sqes.GetState("fake-prefix")
				Expect(res).To(Equal([]byte("fake-state")))
				Expect(err).To(MatchError("fake-error"))
			})
		})

		Describe("GetStateRange", func() {
			var resItr *mock.ResultsIterator

			BeforeEach(func() {
				resItr = &mock.ResultsIterator{}
				resItr.NextReturnsOnCall(0, &queryresult.KV{
					Key:   "fake-key",
					Value: []byte("key-value"),
				}, nil)
				fakeSimpleQueryExecutor.GetStateRangeScanIteratorReturns(resItr, nil)
			})

			It("passes through to the query executor", func() {
				res, err := sqes.GetStateRange("fake-key")
				Expect(err).NotTo(HaveOccurred())
				Expect(res).To(Equal(map[string][]byte{
					"fake-key": []byte("key-value"),
				}))

				Expect(fakeSimpleQueryExecutor.GetStateRangeScanIteratorCallCount()).To(Equal(1))
				namespace, start, end := fakeSimpleQueryExecutor.GetStateRangeScanIteratorArgsForCall(0)
				Expect(namespace).To(Equal("cc-namespace"))
				Expect(start).To(Equal("fake-key"))
				Expect(end).To(Equal("fake-key\x7f"))
			})

			Context("when the result iterator returns an error", func() {
				BeforeEach(func() {
					resItr.NextReturns(nil, fmt.Errorf("fake-error"))
				})

				It("returns the error", func() {
					_, err := sqes.GetStateRange("fake-key")
					Expect(err).To(MatchError("could not iterate over range: fake-error"))
				})
			})

			Context("when getting the state iterator fails", func() {
				BeforeEach(func() {
					fakeSimpleQueryExecutor.GetStateRangeScanIteratorReturns(nil, fmt.Errorf("fake-range-error"))
				})

				It("wraps and returns the error", func() {
					_, err := sqes.GetStateRange("fake-key")
					Expect(err).To(MatchError("could not get state iterator: fake-range-error"))
				})
			})
		})
	})

	Describe("PrivateQueryExecutorShim", func() {
		var (
			pqes                    *lifecycle.PrivateQueryExecutorShim
			fakeSimpleQueryExecutor *mock.SimpleQueryExecutor
		)

		BeforeEach(func() {
			fakeSimpleQueryExecutor = &mock.SimpleQueryExecutor{}
			pqes = &lifecycle.PrivateQueryExecutorShim{
				Namespace:  "cc-namespace",
				Collection: "collection",
				State:      fakeSimpleQueryExecutor,
			}
			fakeSimpleQueryExecutor.GetPrivateDataHashReturns([]byte("hash"), fmt.Errorf("fake-error"))
		})

		It("passes through to the underlying implementation", func() {
			data, err := pqes.GetStateHash("key")
			Expect(data).To(Equal([]byte("hash")))
			Expect(err).To(MatchError("fake-error"))
			Expect(fakeSimpleQueryExecutor.GetPrivateDataHashCallCount()).To(Equal(1))
			namespace, collection, key := fakeSimpleQueryExecutor.GetPrivateDataHashArgsForCall(0)
			Expect(namespace).To(Equal("cc-namespace"))
			Expect(collection).To(Equal("collection"))
			Expect(key).To(Equal("key"))
		})
	})
})
