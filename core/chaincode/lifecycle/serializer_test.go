/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle_test

import (
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/hyperledger/fabric/common/util"
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
		Proto  *lb.InstallChaincodeResult
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
			Proto: &lb.InstallChaincodeResult{
				Hash: []byte("hash"),
			},
		}
	})

	Describe("Serialize", func() {
		It("serializes the structure", func() {
			err := s.Serialize("namespaces", "fake", testStruct, fakeState)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeState.GetStateCallCount()).To(Equal(1))
			Expect(fakeState.GetStateArgsForCall(0)).To(Equal("namespaces/metadata/fake"))

			Expect(fakeState.PutStateCallCount()).To(Equal(6))

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
			Expect(key).To(Equal("namespaces/fields/fake/Proto"))
			Expect(value).To(Equal(utils.MarshalOrPanic(&lb.StateData{
				Type: &lb.StateData_Bytes{Bytes: utils.MarshalOrPanic(testStruct.Proto)},
			})))

			key, value = fakeState.PutStateArgsForCall(5)
			Expect(key).To(Equal("namespaces/metadata/fake"))
			Expect(value).To(Equal(utils.MarshalOrPanic(&lb.StateMetadata{
				Datatype: "TestStruct",
				Fields:   []string{"Int", "Uint", "String", "Bytes", "Proto"},
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
					"namespaces/fields/fake/Proto": utils.MarshalOrPanic(&lb.StateData{
						Type: &lb.StateData_Bytes{Bytes: utils.MarshalOrPanic(testStruct.Proto)},
					}),
					"namespaces/metadata/fake": utils.MarshalOrPanic(&lb.StateMetadata{
						Datatype: "TestStruct",
						Fields:   []string{"Bytes", "String", "Uint", "Int", "Proto"},
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
						Fields:   []string{"Bytes", "String", "Uint", "Proto"},
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
						Fields:   []string{"Int", "Uint", "String", "Bytes", "Proto"},
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
					Expect(err).To(MatchError("could not deserialize metadata for namespace namespaces/fake: could not unmarshal metadata for namespace namespaces/fake: unexpected EOF"))
				})
			})
		})

		Context("when the argument is not a pointer", func() {
			It("fails", func() {
				err := s.Serialize("namespaces", "fake", 8, fakeState)
				Expect(err).To(MatchError("structure for namespace namespaces/fake is not serializable: must be pointer to struct, but got non-pointer int"))
			})
		})

		Context("when the argument is a pointer to not-a-struct", func() {
			It("fails", func() {
				value := 7
				err := s.Serialize("namespaces", "fake", &value, fakeState)
				Expect(err).To(MatchError("structure for namespace namespaces/fake is not serializable: must be pointers to struct, but got pointer to int"))
			})
		})

		Context("when the argument contains an illegal field type", func() {
			It("it fails", func() {
				type BadStruct struct {
					BadField int
				}

				err := s.Serialize("namespaces", "fake", &BadStruct{}, fakeState)
				Expect(err).To(MatchError("structure for namespace namespaces/fake is not serializable: unsupported structure field kind int for serialization for field BadField"))
			})
		})

		Context("when the argument contains a non-byte slice", func() {
			It("it fails", func() {
				type BadStruct struct {
					BadField []uint64
				}

				err := s.Serialize("namespaces", "fake", &BadStruct{}, fakeState)
				Expect(err).To(MatchError("structure for namespace namespaces/fake is not serializable: unsupported slice type uint64 for field BadField"))
			})
		})

		Context("when the argument contains a proto pointer", func() {
			It("it fails", func() {
				type BadStruct struct {
					BadField *int
				}

				err := s.Serialize("namespaces", "fake", &BadStruct{}, fakeState)
				Expect(err).To(MatchError("structure for namespace namespaces/fake is not serializable: unsupported pointer type int for field BadField (must be proto)"))
			})
		})

		Context("when the state metadata cannot be retrieved", func() {
			BeforeEach(func() {
				fakeState.GetStateReturns(nil, fmt.Errorf("state-error"))
			})

			It("wraps and returns the error", func() {
				err := s.Serialize("namespaces", "fake", testStruct, fakeState)
				Expect(err).To(MatchError("could not deserialize metadata for namespace namespaces/fake: could not query metadata for namespace namespaces/fake: state-error"))
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
					if _, ok := msg.(*lb.InstallChaincodeResult); !ok {
						return proto.Marshal(msg)
					}
					return nil, fmt.Errorf("marshal-error")
				}
			})

			It("wraps and returns the error", func() {
				err := s.Serialize("namespaces", "fake", testStruct, fakeState)
				Expect(err).To(MatchError("could not marshal field Proto: marshal-error"))
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

	Describe("Deserialize", func() {
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
				"namespaces/fields/fake/Proto": utils.MarshalOrPanic(&lb.StateData{
					Type: &lb.StateData_Bytes{Bytes: utils.MarshalOrPanic(testStruct.Proto)},
				}),
				"namespaces/metadata/fake": utils.MarshalOrPanic(&lb.StateMetadata{
					Datatype: "TestStruct",
					Fields:   []string{"Bytes", "String", "Uint", "Int", "Proto"},
				}),
			}

			fakeState.GetStateStub = func(key string) ([]byte, error) {
				fmt.Println("returning", kvs[key], "for", key)
				return kvs[key], nil
			}
		})

		It("populates the given struct with values from the state", func() {
			target := &TestStruct{}
			err := s.Deserialize("namespaces", "fake", target, fakeState)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeState.GetStateCallCount()).To(Equal(6))
			Expect(fakeState.GetStateArgsForCall(0)).To(Equal("namespaces/metadata/fake"))

			Expect(target.Int).To(Equal(int64(-3)))
			Expect(target.Uint).To(Equal(uint64(93)))
			Expect(target.String).To(Equal("string"))
			Expect(target.Bytes).To(Equal([]byte("bytes")))
			Expect(proto.Equal(target.Proto, testStruct.Proto)).To(BeTrue())
		})

		Context("when the metadata encoding is bad", func() {
			BeforeEach(func() {
				kvs["namespaces/metadata/fake"] = []byte("bad-data")
			})

			It("fails", func() {
				testStruct := &TestStruct{}
				err := s.Deserialize("namespaces", "fake", testStruct, fakeState)
				Expect(err).To(MatchError("could not unmarshal metadata for namespace namespaces/fake: could not unmarshal metadata for namespace namespaces/fake: unexpected EOF"))
			})
		})

		Context("when the field encoding is bad", func() {
			BeforeEach(func() {
				kvs["namespaces/fields/fake/Uint"] = []byte("bad-data")
			})

			It("fails", func() {
				testStruct := &TestStruct{}
				err := s.Deserialize("namespaces", "fake", testStruct, fakeState)
				Expect(err).To(MatchError("could not unmarshal state for key namespaces/fields/fake/Uint: unexpected EOF"))
			})
		})

		Context("when the uint is not the correct type", func() {
			BeforeEach(func() {
				kvs["namespaces/fields/fake/Uint"] = kvs["namespaces/fields/fake/Bytes"]
			})

			It("fails", func() {
				testStruct := &TestStruct{}
				err := s.Deserialize("namespaces", "fake", testStruct, fakeState)
				Expect(err).To(MatchError("expected key namespaces/fields/fake/Uint to encode a value of type Uint64, but was *lifecycle.StateData_Bytes"))
			})
		})

		Context("when the int is not the correct type", func() {
			BeforeEach(func() {
				kvs["namespaces/fields/fake/Int"] = kvs["namespaces/fields/fake/Bytes"]
			})

			It("fails", func() {
				testStruct := &TestStruct{}
				err := s.Deserialize("namespaces", "fake", testStruct, fakeState)
				Expect(err).To(MatchError("expected key namespaces/fields/fake/Int to encode a value of type Int64, but was *lifecycle.StateData_Bytes"))
			})
		})

		Context("when the string is not the correct type", func() {
			BeforeEach(func() {
				kvs["namespaces/fields/fake/String"] = kvs["namespaces/fields/fake/Bytes"]
			})

			It("fails", func() {
				testStruct := &TestStruct{}
				err := s.Deserialize("namespaces", "fake", testStruct, fakeState)
				Expect(err).To(MatchError("expected key namespaces/fields/fake/String to encode a value of type String, but was *lifecycle.StateData_Bytes"))
			})
		})

		Context("when the bytes is not the correct type", func() {
			BeforeEach(func() {
				kvs["namespaces/fields/fake/Bytes"] = kvs["namespaces/fields/fake/String"]
			})

			It("fails", func() {
				testStruct := &TestStruct{}
				err := s.Deserialize("namespaces", "fake", testStruct, fakeState)
				Expect(err).To(MatchError("expected key namespaces/fields/fake/Bytes to encode a value of type []byte, but was *lifecycle.StateData_String_"))
			})
		})

		Context("when the proto is not the correct type", func() {
			BeforeEach(func() {
				kvs["namespaces/fields/fake/Proto"] = kvs["namespaces/fields/fake/String"]
			})

			It("fails", func() {
				testStruct := &TestStruct{}
				err := s.Deserialize("namespaces", "fake", testStruct, fakeState)
				Expect(err).To(MatchError("expected key namespaces/fields/fake/Proto to encode a value of type []byte, but was *lifecycle.StateData_String_"))
			})
		})

		Context("when the metadata cannot be queried", func() {
			BeforeEach(func() {
				fakeState.GetStateReturns(nil, fmt.Errorf("state-error"))
			})

			It("fails", func() {
				testStruct := &TestStruct{}
				err := s.Deserialize("namespaces", "fake", testStruct, fakeState)
				Expect(err).To(MatchError("could not unmarshal metadata for namespace namespaces/fake: could not query metadata for namespace namespaces/fake: state-error"))
			})
		})

		Context("when the state cannot be queried", func() {
			BeforeEach(func() {
				fakeState.GetStateReturnsOnCall(0, kvs["namespaces/metadata/fake"], nil)
				fakeState.GetStateReturnsOnCall(1, nil, fmt.Errorf("state-error"))
			})

			It("fails", func() {
				testStruct := &TestStruct{}
				err := s.Deserialize("namespaces", "fake", testStruct, fakeState)
				Expect(err).To(MatchError("could not get state for key namespaces/fields/fake/Int: state-error"))
			})
		})

		Context("when no data is stored for the message", func() {
			BeforeEach(func() {
				fakeState.GetStateReturns(nil, nil)
			})

			It("fails", func() {
				testStruct := &TestStruct{}
				err := s.Deserialize("namespaces", "fake", testStruct, fakeState)
				Expect(err).To(MatchError("metadata for namespace namespaces/fake does not exist"))
			})
		})

		Context("when the argument is not a pointer", func() {
			It("fails", func() {
				err := s.Deserialize("namespaces", "fake", 8, fakeState)
				Expect(err).To(MatchError("could not deserialize namespace namespaces/fake to unserializable type int: must be pointer to struct, but got non-pointer int"))
			})
		})

		Context("when the argument is a pointer to not-a-struct", func() {
			It("fails", func() {
				value := 7
				err := s.Deserialize("namespaces", "fake", &value, fakeState)
				Expect(err).To(MatchError("could not deserialize namespace namespaces/fake to unserializable type *int: must be pointers to struct, but got pointer to int"))
			})
		})

		Context("when the argument does not match the stored type", func() {
			It("it fails", func() {
				type Other struct{}
				err := s.Deserialize("namespaces", "fake", &Other{}, fakeState)
				Expect(err).To(MatchError("type name mismatch 'Other' != 'TestStruct'"))
			})
		})

		Context("when the argument contains an illegal field type", func() {
			BeforeEach(func() {
				kvs["namespaces/metadata/fake"] = utils.MarshalOrPanic(&lb.StateMetadata{
					Datatype: "BadStruct",
				})
			})

			It("it fails", func() {
				type BadStruct struct {
					BadField int
				}

				err := s.Deserialize("namespaces", "fake", &BadStruct{}, fakeState)
				Expect(err).To(MatchError("could not deserialize namespace namespaces/fake to unserializable type *lifecycle_test.BadStruct: unsupported structure field kind int for serialization for field BadField"))
			})
		})

		Context("when the argument contains a non-byte slice", func() {
			BeforeEach(func() {
				kvs["namespaces/metadata/fake"] = utils.MarshalOrPanic(&lb.StateMetadata{
					Datatype: "BadStruct",
				})
			})

			It("it fails", func() {
				type BadStruct struct {
					BadField []uint64
				}

				err := s.Deserialize("namespaces", "fake", &BadStruct{}, fakeState)
				Expect(err).To(MatchError("could not deserialize namespace namespaces/fake to unserializable type *lifecycle_test.BadStruct: unsupported slice type uint64 for field BadField"))
			})
		})
	})

	Describe("Integration Round Trip of Serialize/Deserialize", func() {
		var (
			KVStore map[string][]byte
		)

		BeforeEach(func() {
			KVStore = map[string][]byte{}

			fakeState.PutStateStub = func(key string, value []byte) error {
				KVStore[key] = value
				return nil
			}

			fakeState.GetStateStub = func(key string) ([]byte, error) {
				return KVStore[key], nil
			}

			fakeState.GetStateHashStub = func(key string) ([]byte, error) {
				return util.ComputeSHA256(KVStore[key]), nil
			}
		})

		It("deserializes to the same value that was serialized in", func() {
			err := s.Serialize("namespace", "fake", testStruct, fakeState)
			Expect(err).NotTo(HaveOccurred())

			deserialized := &TestStruct{}
			err = s.Deserialize("namespace", "fake", deserialized, fakeState)
			Expect(err).NotTo(HaveOccurred())

			Expect(testStruct.Int).To(Equal(deserialized.Int))
			Expect(testStruct.Uint).To(Equal(deserialized.Uint))
			Expect(testStruct.Bytes).To(Equal(deserialized.Bytes))
			Expect(testStruct.String).To(Equal(deserialized.String))
			Expect(proto.Equal(testStruct.Proto, deserialized.Proto))

			matched, err := s.IsSerialized("namespace", "fake", testStruct, fakeState)
			Expect(err).NotTo(HaveOccurred())
			Expect(matched).To(BeTrue())
		})
	})

	Describe("IsSerialized", func() {
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
				"namespaces/fields/fake/Proto": utils.MarshalOrPanic(&lb.StateData{
					Type: &lb.StateData_Bytes{Bytes: utils.MarshalOrPanic(testStruct.Proto)},
				}),
				"namespaces/metadata/fake": utils.MarshalOrPanic(&lb.StateMetadata{
					Datatype: "TestStruct",
					Fields:   []string{"Int", "Uint", "String", "Bytes", "Proto"},
				}),
			}

			fakeState.GetStateHashStub = func(key string) ([]byte, error) {
				return util.ComputeSHA256(kvs[key]), nil
			}
		})

		It("checks to see if the structure is stored in the opaque state", func() {
			matched, err := s.IsSerialized("namespaces", "fake", testStruct, fakeState)
			Expect(err).NotTo(HaveOccurred())
			_ = matched
			Expect(matched).To(BeTrue())

			Expect(fakeState.GetStateHashCallCount()).To(Equal(6))
			Expect(fakeState.GetStateHashArgsForCall(0)).To(Equal("namespaces/metadata/fake"))
			Expect(fakeState.GetStateHashArgsForCall(1)).To(Equal("namespaces/fields/fake/Int"))
			Expect(fakeState.GetStateHashArgsForCall(2)).To(Equal("namespaces/fields/fake/Uint"))
			Expect(fakeState.GetStateHashArgsForCall(3)).To(Equal("namespaces/fields/fake/String"))
			Expect(fakeState.GetStateHashArgsForCall(4)).To(Equal("namespaces/fields/fake/Bytes"))
			Expect(fakeState.GetStateHashArgsForCall(5)).To(Equal("namespaces/fields/fake/Proto"))
		})

		Context("when the namespace contains extraneous keys", func() {
			BeforeEach(func() {
				kvs["namespaces/metadata/fake"] = utils.MarshalOrPanic(&lb.StateMetadata{
					Datatype: "TestStruct",
					Fields:   []string{"Int", "Uint", "String", "Bytes", "Other"},
				})
				kvs["namespaces/fields/fake/Other"] = utils.MarshalOrPanic(&lb.StateData{
					Type: &lb.StateData_Bytes{Bytes: []byte("value1")},
				})
			})

			It("returns a mismatch", func() {
				matched, err := s.IsSerialized("namespaces", "fake", testStruct, fakeState)
				Expect(err).NotTo(HaveOccurred())
				Expect(matched).To(BeFalse())
			})
		})

		Context("when the namespace contains different keys", func() {
			BeforeEach(func() {
				kvs["namespaces/fields/fake/Int"] = utils.MarshalOrPanic(&lb.StateData{
					Type: &lb.StateData_Bytes{Bytes: []byte("value1")},
				})
			})

			It("returns a mismatch", func() {
				matched, err := s.IsSerialized("namespaces", "fake", testStruct, fakeState)
				Expect(err).NotTo(HaveOccurred())
				Expect(matched).To(BeFalse())
			})
		})

		Context("when the argument is not a pointer", func() {
			It("fails", func() {
				_, err := s.IsSerialized("namespaces", "fake", 8, fakeState)
				Expect(err).To(MatchError("structure for namespace namespaces/fake is not serializable: must be pointer to struct, but got non-pointer int"))
			})
		})

		Context("when the namespace does not contains the keys and values", func() {
			BeforeEach(func() {
				kvs = map[string][]byte{}
			})

			It("returns a mismatch", func() {
				matched, err := s.IsSerialized("namespaces", "fake", testStruct, fakeState)
				Expect(err).NotTo(HaveOccurred())
				Expect(matched).To(BeFalse())
			})
		})

		Context("when the state metadata cannot be retrieved", func() {
			BeforeEach(func() {
				fakeState.GetStateHashReturns(nil, fmt.Errorf("state-error"))
			})

			It("wraps and returns the error", func() {
				_, err := s.IsSerialized("namespaces", "fake", testStruct, fakeState)
				Expect(err).To(MatchError("could not get value for key namespaces/metadata/fake: state-error"))
			})
		})

		Context("when inner marshaling of a proto field fails", func() {
			BeforeEach(func() {
				s.Marshaler = func(msg proto.Message) ([]byte, error) {
					if _, ok := msg.(*lb.StateMetadata); ok {
						return proto.Marshal(msg)
					}
					if _, ok := msg.(*lb.StateData); ok {
						return proto.Marshal(msg)
					}
					return nil, fmt.Errorf("marshal-error")
				}
			})

			It("wraps and returns the error", func() {
				_, err := s.IsSerialized("namespaces", "fake", testStruct, fakeState)
				Expect(err).To(MatchError("could not marshal field Proto: marshal-error"))
			})
		})

		Context("when marshaling a field fails", func() {
			BeforeEach(func() {
				s.Marshaler = func(msg proto.Message) ([]byte, error) {
					if _, ok := msg.(*lb.StateData); ok {
						return nil, fmt.Errorf("marshal-error")
					}
					return proto.Marshal(msg)
				}
			})

			It("wraps and returns the error", func() {
				_, err := s.IsSerialized("namespaces", "fake", testStruct, fakeState)
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
				_, err := s.IsSerialized("namespaces", "fake", &Other{}, fakeState)
				Expect(err).To(MatchError("could not marshal metadata for namespace namespaces/fake: marshal-error"))
			})
		})
	})

	Describe("DeserializeAllMetadata", func() {
		BeforeEach(func() {
			fakeState.GetStateRangeReturns(map[string][]byte{
				"namespaces/metadata/thing0": utils.MarshalOrPanic(&lb.StateMetadata{
					Datatype: "TestDatatype0",
				}),
				"namespaces/metadata/thing1": utils.MarshalOrPanic(&lb.StateMetadata{
					Datatype: "TestDatatype1",
				}),
				"namespaces/metadata/thing2": utils.MarshalOrPanic(&lb.StateMetadata{
					Datatype: "TestDatatype2",
				}),
			}, nil)
		})

		It("deserializes all the metadata", func() {
			result, err := s.DeserializeAllMetadata("namespaces", fakeState)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(result)).To(Equal(3))
			Expect(proto.Equal(result["thing0"], &lb.StateMetadata{Datatype: "TestDatatype0"})).To(BeTrue())
			Expect(proto.Equal(result["thing1"], &lb.StateMetadata{Datatype: "TestDatatype1"})).To(BeTrue())
			Expect(proto.Equal(result["thing2"], &lb.StateMetadata{Datatype: "TestDatatype2"})).To(BeTrue())

			Expect(fakeState.GetStateRangeCallCount()).To(Equal(1))
			Expect(fakeState.GetStateRangeArgsForCall(0)).To(Equal("namespaces/metadata/"))
		})

		Context("when GetStateRange returns an error", func() {
			BeforeEach(func() {
				fakeState.GetStateRangeReturns(nil, fmt.Errorf("get-state-range-error"))
			})

			It("wraps and returns the error", func() {
				_, err := s.DeserializeAllMetadata("namespaces", fakeState)
				Expect(err).To(MatchError("could not get state range for namespace namespaces: get-state-range-error"))
			})
		})

		Context("when GetState returns nil", func() {
			BeforeEach(func() {
				fakeState.GetStateRangeReturns(nil, nil)
			})

			It("returns an empty map", func() {
				result, err := s.DeserializeAllMetadata("namespaces", fakeState)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeEmpty())
			})
		})

		Context("when the metadata is invalid", func() {
			BeforeEach(func() {
				fakeState.GetStateRangeReturns(map[string][]byte{"namespaces/metadata/bad": []byte("bad-data")}, nil)
			})

			It("returns an error", func() {
				_, err := s.DeserializeAllMetadata("namespaces", fakeState)
				Expect(err).To(MatchError("error unmarshaling metadata for key namespaces/metadata/bad: unexpected EOF"))
			})
		})
	})

	Describe("DeserializeMetadata", func() {
		BeforeEach(func() {
			fakeState.GetStateReturns(utils.MarshalOrPanic(&lb.StateMetadata{
				Datatype: "TestDatatype",
			}), nil)
		})

		It("deserializes the metadata", func() {
			result, ok, err := s.DeserializeMetadata("namespaces", "fake", fakeState)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
			Expect(proto.Equal(result, &lb.StateMetadata{Datatype: "TestDatatype"})).To(BeTrue())

			Expect(fakeState.GetStateCallCount()).To(Equal(1))
			Expect(fakeState.GetStateArgsForCall(0)).To(Equal("namespaces/metadata/fake"))
		})

		Context("when GetState returns an error", func() {
			BeforeEach(func() {
				fakeState.GetStateReturns(nil, fmt.Errorf("get-state-error"))
			})

			It("wraps and returns the error", func() {
				_, _, err := s.DeserializeMetadata("namespaces", "fake", fakeState)
				Expect(err).To(MatchError("could not query metadata for namespace namespaces/fake: get-state-error"))
			})
		})

		Context("when GetState returns nil", func() {
			BeforeEach(func() {
				fakeState.GetStateReturns(nil, nil)
			})

			It("returns an error", func() {
				_, ok, err := s.DeserializeMetadata("namespaces", "fake", fakeState)
				Expect(err).NotTo(HaveOccurred())
				Expect(ok).To(BeFalse())
			})
		})

		Context("when the metadata is invalid", func() {
			BeforeEach(func() {
				fakeState.GetStateReturns([]byte("bad-data"), nil)
			})

			It("returns an error", func() {
				_, _, err := s.DeserializeMetadata("namespaces", "fake", fakeState)
				Expect(err).To(MatchError("could not unmarshal metadata for namespace namespaces/fake: unexpected EOF"))
			})
		})
	})

	Describe("DeserializeFieldAsString", func() {
		BeforeEach(func() {
			fakeState.GetStateReturns(utils.MarshalOrPanic(&lb.StateData{
				Type: &lb.StateData_String_{String_: "string"},
			}), nil)
		})

		It("deserializes the field to a string", func() {
			result, err := s.DeserializeFieldAsString("namespaces", "fake", "field", fakeState)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("string"))

			Expect(fakeState.GetStateCallCount()).To(Equal(1))
			Expect(fakeState.GetStateArgsForCall(0)).To(Equal("namespaces/fields/fake/field"))
		})

		Context("when GetState returns an error", func() {
			BeforeEach(func() {
				fakeState.GetStateReturns(nil, fmt.Errorf("get-state-error"))
			})

			It("wraps and returns the error", func() {
				_, err := s.DeserializeFieldAsString("namespaces", "fake", "field", fakeState)
				Expect(err).To(MatchError("could not get state for key namespaces/fields/fake/field: get-state-error"))
			})
		})

		Context("when GetState returns nil", func() {
			BeforeEach(func() {
				fakeState.GetStateReturns(nil, nil)
			})

			It("returns the empty string", func() {
				result, err := s.DeserializeFieldAsString("namespaces", "fake", "field", fakeState)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(""))
			})
		})
	})

	Describe("DeserializeFieldAsBytes", func() {
		BeforeEach(func() {
			fakeState.GetStateReturns(utils.MarshalOrPanic(&lb.StateData{
				Type: &lb.StateData_Bytes{Bytes: []byte("bytes")},
			}), nil)
		})

		It("deserializes the field to a string", func() {
			result, err := s.DeserializeFieldAsBytes("namespaces", "fake", "field", fakeState)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal([]byte("bytes")))

			Expect(fakeState.GetStateCallCount()).To(Equal(1))
			Expect(fakeState.GetStateArgsForCall(0)).To(Equal("namespaces/fields/fake/field"))
		})

		Context("when GetState returns an error", func() {
			BeforeEach(func() {
				fakeState.GetStateReturns(nil, fmt.Errorf("get-state-error"))
			})

			It("wraps and returns the error", func() {
				_, err := s.DeserializeFieldAsBytes("namespaces", "fake", "field", fakeState)
				Expect(err).To(MatchError("could not get state for key namespaces/fields/fake/field: get-state-error"))
			})
		})

		Context("when GetState returns nil", func() {
			BeforeEach(func() {
				fakeState.GetStateReturns(nil, nil)
			})

			It("returns nil", func() {
				result, err := s.DeserializeFieldAsBytes("namespaces", "fake", "field", fakeState)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeNil())
			})
		})
	})

	Describe("DeserializeFieldAsProto", func() {
		BeforeEach(func() {
			fakeState.GetStateReturns(utils.MarshalOrPanic(&lb.StateData{
				Type: &lb.StateData_Bytes{Bytes: utils.MarshalOrPanic(&lb.InstallChaincodeResult{Hash: []byte("hash")})},
			}), nil)
		})

		It("deserializes the field to a string", func() {
			result := &lb.InstallChaincodeResult{}
			err := s.DeserializeFieldAsProto("namespaces", "fake", "field", fakeState, result)
			Expect(err).NotTo(HaveOccurred())
			Expect(proto.Equal(result, &lb.InstallChaincodeResult{Hash: []byte("hash")})).To(BeTrue())

			Expect(fakeState.GetStateCallCount()).To(Equal(1))
			Expect(fakeState.GetStateArgsForCall(0)).To(Equal("namespaces/fields/fake/field"))
		})

		Context("when GetState returns an error", func() {
			BeforeEach(func() {
				fakeState.GetStateReturns(nil, fmt.Errorf("get-state-error"))
			})

			It("wraps and returns the error", func() {
				err := s.DeserializeFieldAsProto("namespaces", "fake", "field", fakeState, &lb.InstallChaincodeResult{})
				Expect(err).To(MatchError("could not get state for key namespaces/fields/fake/field: get-state-error"))
			})
		})

		Context("when GetState returns nil", func() {
			BeforeEach(func() {
				fakeState.GetStateReturns(nil, nil)
			})

			It("does nothing", func() {
				result := &lb.InstallChaincodeResult{}
				err := s.DeserializeFieldAsProto("namespaces", "fake", "field", fakeState, result)
				Expect(err).NotTo(HaveOccurred())
				Expect(proto.Equal(result, &lb.InstallChaincodeResult{})).To(BeTrue())
			})
		})

		Context("when the db does not encode a proto", func() {
			BeforeEach(func() {
				fakeState.GetStateReturns(utils.MarshalOrPanic(&lb.StateData{
					Type: &lb.StateData_Bytes{Bytes: []byte("garbage")},
				}), nil)
			})

			It("wraps and returns the error", func() {
				err := s.DeserializeFieldAsProto("namespaces", "fake", "field", fakeState, &lb.InstallChaincodeResult{})
				Expect(err).To(MatchError("could not unmarshal key namespaces/fields/fake/field to *lifecycle.InstallChaincodeResult: proto: can't skip unknown wire type 7"))
			})
		})
	})

	Describe("DeserializeFieldAsInt64", func() {
		BeforeEach(func() {
			fakeState.GetStateReturns(utils.MarshalOrPanic(&lb.StateData{
				Type: &lb.StateData_Int64{Int64: -3},
			}), nil)
		})

		It("deserializes the field to a string", func() {
			result, err := s.DeserializeFieldAsInt64("namespaces", "fake", "field", fakeState)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(int64(-3)))

			Expect(fakeState.GetStateCallCount()).To(Equal(1))
			Expect(fakeState.GetStateArgsForCall(0)).To(Equal("namespaces/fields/fake/field"))
		})

		Context("when GetState returns an error", func() {
			BeforeEach(func() {
				fakeState.GetStateReturns(nil, fmt.Errorf("get-state-error"))
			})

			It("wraps and returns the error", func() {
				_, err := s.DeserializeFieldAsInt64("namespaces", "fake", "field", fakeState)
				Expect(err).To(MatchError("could not get state for key namespaces/fields/fake/field: get-state-error"))
			})
		})

		Context("when GetState returns nil", func() {
			BeforeEach(func() {
				fakeState.GetStateReturns(nil, nil)
			})

			It("returns nil", func() {
				result, err := s.DeserializeFieldAsInt64("namespaces", "fake", "field", fakeState)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(int64(0)))
			})
		})

		Context("when the field has trailing data", func() {
			BeforeEach(func() {
				fakeState.GetStateReturns([]byte("bad-data"), nil)
			})

			It("returns an error", func() {
				_, err := s.DeserializeFieldAsInt64("namespaces", "fake", "field", fakeState)
				Expect(err).To(MatchError("could not unmarshal state for key namespaces/fields/fake/field: unexpected EOF"))
			})
		})
	})

	Describe("DeserializeFieldAsUint64", func() {
		BeforeEach(func() {
			fakeState.GetStateReturns(utils.MarshalOrPanic(&lb.StateData{
				Type: &lb.StateData_Uint64{Uint64: 93},
			}), nil)
		})

		It("deserializes the field to a string", func() {
			result, err := s.DeserializeFieldAsUint64("namespaces", "fake", "field", fakeState)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(uint64(93)))

			Expect(fakeState.GetStateCallCount()).To(Equal(1))
			Expect(fakeState.GetStateArgsForCall(0)).To(Equal("namespaces/fields/fake/field"))
		})

		Context("when GetState returns an error", func() {
			BeforeEach(func() {
				fakeState.GetStateReturns(nil, fmt.Errorf("get-state-error"))
			})

			It("wraps and returns the error", func() {
				_, err := s.DeserializeFieldAsUint64("namespaces", "fake", "field", fakeState)
				Expect(err).To(MatchError("could not get state for key namespaces/fields/fake/field: get-state-error"))
			})
		})

		Context("when GetState returns nil", func() {
			BeforeEach(func() {
				fakeState.GetStateReturns(nil, nil)
			})

			It("returns nil", func() {
				result, err := s.DeserializeFieldAsUint64("namespaces", "fake", "field", fakeState)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(uint64(0)))
			})
		})

		Context("when the field is not validly encoded", func() {
			BeforeEach(func() {
				fakeState.GetStateReturns([]byte("bad-data"), nil)
			})

			It("returns an error", func() {
				_, err := s.DeserializeFieldAsUint64("namespaces", "fake", "field", fakeState)
				Expect(err).To(MatchError("could not unmarshal state for key namespaces/fields/fake/field: unexpected EOF"))
			})
		})
	})
})
