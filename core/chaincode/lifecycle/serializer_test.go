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
	lb "github.com/hyperledger/fabric/protos/peer/lifecycle"
	"github.com/hyperledger/fabric/protos/utils"

	"github.com/golang/protobuf/proto"
)

var _ = Describe("Serializer", func() {
	type TestStruct struct {
		Int    int64
		Uint   uint64
		String string
		Bytes  []byte
	}

	var (
		s          *lifecycle.Serializer
		fakeState  *mock.ReadWritableState
		testStruct *TestStruct
	)

	BeforeEach(func() {
		fakeState = &mock.ReadWritableState{}

		s = &lifecycle.Serializer{}

		testStruct = &TestStruct{
			Int:    -3,
			Uint:   93,
			String: "string",
			Bytes:  []byte("bytes"),
		}
	})

	Describe("Serialize", func() {
		It("serializes the structure", func() {
			err := s.Serialize("namespaces", "fake", testStruct, fakeState)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeState.GetStateCallCount()).To(Equal(1))
			Expect(fakeState.GetStateArgsForCall(0)).To(Equal("namespaces/metadata/fake"))

			Expect(fakeState.PutStateCallCount()).To(Equal(5))

			key, value := fakeState.PutStateArgsForCall(0)
			Expect(key).To(Equal("namespaces/fields/fake/Int"))
			Expect(value).To(Equal(utils.MarshalOrPanic(&lb.StateData{
				Type: &lb.StateData_Int64{Int64: -3},
			})))

			key, value = fakeState.PutStateArgsForCall(1)
			Expect(key).To(Equal("namespaces/fields/fake/Uint"))
			Expect(value).To(Equal(utils.MarshalOrPanic(&lb.StateData{
				Type: &lb.StateData_Uint64{Uint64: 93},
			})))

			key, value = fakeState.PutStateArgsForCall(2)
			Expect(key).To(Equal("namespaces/fields/fake/String"))
			Expect(value).To(Equal(utils.MarshalOrPanic(&lb.StateData{
				Type: &lb.StateData_String_{String_: "string"},
			})))

			key, value = fakeState.PutStateArgsForCall(3)
			Expect(key).To(Equal("namespaces/fields/fake/Bytes"))
			Expect(value).To(Equal(utils.MarshalOrPanic(&lb.StateData{
				Type: &lb.StateData_Bytes{Bytes: []byte("bytes")},
			})))

			key, value = fakeState.PutStateArgsForCall(4)
			Expect(key).To(Equal("namespaces/metadata/fake"))
			Expect(value).To(Equal(utils.MarshalOrPanic(&lb.StateMetadata{
				Datatype: "TestStruct",
				Fields:   []string{"Int", "Uint", "String", "Bytes"},
			})))

			Expect(fakeState.DelStateCallCount()).To(Equal(0))
		})

		Context("when the namespace contains extraneous keys", func() {
			BeforeEach(func() {
				kvs := map[string][]byte{
					"namespaces/fields/fake/ExtraneousKey1": utils.MarshalOrPanic(&lb.StateData{
						Type: &lb.StateData_Bytes{Bytes: []byte("value1")},
					}),
					"namespaces/fields/fake/ExtraneousKey2": utils.MarshalOrPanic(&lb.StateData{
						Type: &lb.StateData_Bytes{Bytes: []byte("value2")},
					}),
					"namespaces/metadata/fake": utils.MarshalOrPanic(&lb.StateMetadata{
						Datatype: "Other",
						Fields:   []string{"ExtraneousKey1", "ExtraneousKey2"},
					}),
				}
				fakeState.GetStateStub = func(key string) ([]byte, error) {
					return kvs[key], nil
				}
			})

			It("deletes them before returning", func() {
				err := s.Serialize("namespaces", "fake", testStruct, fakeState)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeState.DelStateCallCount()).To(Equal(2))
				Expect(map[string]struct{}{
					fakeState.DelStateArgsForCall(0): {},
					fakeState.DelStateArgsForCall(1): {},
				}).To(Equal(map[string]struct{}{
					"namespaces/fields/fake/ExtraneousKey1": {},
					"namespaces/fields/fake/ExtraneousKey2": {},
				}))
			})

			Context("when deleting from  the state fails", func() {
				BeforeEach(func() {
					fakeState.DelStateReturns(fmt.Errorf("del-error"))
				})

				It("deletes them before returning", func() {
					err := s.Serialize("namespaces", "fake", testStruct, fakeState)
					Expect(err.Error()).To(MatchRegexp("could not delete unneeded key namespaces/fields/fake/ExtraneousKey.: del-error"))
				})
			})
		})

		Context("when the namespace already contains the keys and values", func() {
			var (
				kvs map[string][]byte
			)

			BeforeEach(func() {
				kvs = map[string][]byte{
					"namespaces/fields/fake/Bytes": utils.MarshalOrPanic(&lb.StateData{
						Type: &lb.StateData_Bytes{Bytes: []byte("bytes")},
					}),
					"namespaces/fields/fake/String": utils.MarshalOrPanic(&lb.StateData{
						Type: &lb.StateData_String_{String_: "string"},
					}),
					"namespaces/fields/fake/Uint": utils.MarshalOrPanic(&lb.StateData{
						Type: &lb.StateData_Uint64{Uint64: 93},
					}),
					"namespaces/fields/fake/Int": utils.MarshalOrPanic(&lb.StateData{
						Type: &lb.StateData_Int64{Int64: -3},
					}),
					"namespaces/metadata/fake": utils.MarshalOrPanic(&lb.StateMetadata{
						Datatype: "TestStruct",
						Fields:   []string{"Bytes", "String", "Uint", "Int"},
					}),
				}
				fakeState.GetStateStub = func(key string) ([]byte, error) {
					return kvs[key], nil
				}
			})

			It("does not perform writes", func() {
				err := s.Serialize("namespaces", "fake", testStruct, fakeState)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeState.PutStateCallCount()).To(Equal(0))
				Expect(fakeState.DelStateCallCount()).To(Equal(0))
			})

			Context("when some of the values are missing", func() {
				BeforeEach(func() {
					kvs["namespaces/metadata/fake"] = utils.MarshalOrPanic(&lb.StateMetadata{
						Datatype: "TestStruct",
						Fields:   []string{"Bytes", "String", "Uint"},
					})
					delete(kvs, "namespaces/fields/fake/Int")
				})

				It("writes the missing field and new metadata ", func() {
					err := s.Serialize("namespaces", "fake", testStruct, fakeState)
					Expect(err).NotTo(HaveOccurred())

					Expect(fakeState.PutStateCallCount()).To(Equal(2))
					key, value := fakeState.PutStateArgsForCall(0)
					Expect(value).To(Equal(utils.MarshalOrPanic(&lb.StateData{
						Type: &lb.StateData_Int64{Int64: -3},
					})))
					Expect(key).To(Equal("namespaces/fields/fake/Int"))
					key, value = fakeState.PutStateArgsForCall(1)
					Expect(key).To(Equal("namespaces/metadata/fake"))
					Expect(value).To(Equal(utils.MarshalOrPanic(&lb.StateMetadata{
						Datatype: "TestStruct",
						Fields:   []string{"Int", "Uint", "String", "Bytes"},
					})))
					Expect(fakeState.DelStateCallCount()).To(Equal(0))
				})
			})

			Context("when the namespace metadata is invalid", func() {
				BeforeEach(func() {
					kvs["namespaces/metadata/fake"] = []byte("bad-data")
				})

				It("wraps and returns the error", func() {
					err := s.Serialize("namespaces", "fake", testStruct, fakeState)
					Expect(err).To(MatchError("could not decode metadata for namespace namespaces/fake: unexpected EOF"))
				})
			})
		})

		Context("when the argument is not a pointer", func() {
			It("fails", func() {
				err := s.Serialize("namespaces", "fake", 8, fakeState)
				Expect(err).To(MatchError("can only serialize pointers to struct, but got non-pointer int"))
			})
		})

		Context("when the argument is a pointer to not-a-struct", func() {
			It("fails", func() {
				value := 7
				err := s.Serialize("namespaces", "fake", &value, fakeState)
				Expect(err).To(MatchError("can only serialize pointers to struct, but got pointer to int"))
			})
		})

		Context("when the argument contains an illegal field type", func() {
			It("it fails", func() {
				type BadStruct struct {
					BadField *TestStruct
				}

				err := s.Serialize("namespaces", "fake", &BadStruct{}, fakeState)
				Expect(err).To(MatchError("unsupported structure field kind ptr for serialization for field BadField"))
			})
		})

		Context("when the argument contains a non-byte slice", func() {
			It("it fails", func() {
				type BadStruct struct {
					BadField []uint64
				}

				err := s.Serialize("namespaces", "fake", &BadStruct{}, fakeState)
				Expect(err).To(MatchError("unsupported slice type uint64 for field BadField"))
			})
		})

		Context("when the state metadata cannot be retrieved", func() {
			BeforeEach(func() {
				fakeState.GetStateReturns(nil, fmt.Errorf("state-error"))
			})

			It("wraps and returns the error", func() {
				err := s.Serialize("namespaces", "fake", testStruct, fakeState)
				Expect(err).To(MatchError("could not query metadata for namespace namespaces/fake: state-error"))
			})
		})

		Context("when the field data cannot be retrieved", func() {
			BeforeEach(func() {
				fakeState.GetStateReturnsOnCall(0, utils.MarshalOrPanic(&lb.StateMetadata{
					Fields: []string{"field1"},
				}), nil)
				fakeState.GetStateReturnsOnCall(1, nil, fmt.Errorf("state-error"))
			})

			It("wraps and returns the error", func() {
				err := s.Serialize("namespaces", "fake", testStruct, fakeState)
				Expect(err).To(MatchError("could not get value for key namespaces/fields/fake/field1: state-error"))
			})
		})

		Context("when writing to the state for a field fails", func() {
			BeforeEach(func() {
				fakeState.PutStateReturns(fmt.Errorf("put-error"))
			})

			It("wraps and returns the error", func() {
				err := s.Serialize("namespaces", "fake", testStruct, fakeState)
				Expect(err).To(MatchError("could not write key into state: put-error"))
			})
		})

		Context("when writing to the state for metadata fails", func() {
			BeforeEach(func() {
				fakeState.PutStateReturns(fmt.Errorf("put-error"))
			})

			It("wraps and returns the error", func() {
				type Other struct{}
				err := s.Serialize("namespaces", "fake", &Other{}, fakeState)
				Expect(err).To(MatchError("could not store metadata for namespace namespaces/fake: put-error"))
			})
		})

		Context("when marshaling a field fails", func() {
			BeforeEach(func() {
				s.Marshaler = func(msg proto.Message) ([]byte, error) {
					return nil, fmt.Errorf("marshal-error")
				}
			})

			It("wraps and returns the error", func() {
				err := s.Serialize("namespaces", "fake", testStruct, fakeState)
				Expect(err).To(MatchError("could not marshal value for key namespaces/fields/fake/Int: marshal-error"))
			})
		})

		Context("when marshaling a the metadata fails", func() {
			BeforeEach(func() {
				s.Marshaler = func(msg proto.Message) ([]byte, error) {
					return nil, fmt.Errorf("marshal-error")
				}
			})

			It("wraps and returns the error", func() {
				type Other struct{}
				err := s.Serialize("namespaces", "fake", &Other{}, fakeState)
				Expect(err).To(MatchError("could not marshal metadata for namespace namespaces/fake: marshal-error"))
			})
		})
	})
})
