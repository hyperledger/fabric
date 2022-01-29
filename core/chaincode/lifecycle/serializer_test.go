/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/golang/protobuf/proto"
	lb "github.com/hyperledger/fabric-protos-go/peer/lifecycle"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle/mock"
	"github.com/hyperledger/fabric/protoutil"
)

var _ = Describe("Serializer", func() {
	type TestStruct struct {
		Int    int64
		Bytes  []byte
		Proto  *lb.InstallChaincodeResult
		String string
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
			Int:   -3,
			Bytes: []byte("bytes"),
			Proto: &lb.InstallChaincodeResult{
				PackageId: "hash",
			},
			String: "theory",
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
			Expect(value).To(Equal(protoutil.MarshalOrPanic(&lb.StateData{
				Type: &lb.StateData_Int64{Int64: -3},
			})))

			key, value = fakeState.PutStateArgsForCall(1)
			Expect(key).To(Equal("namespaces/fields/fake/Bytes"))
			Expect(value).To(Equal(protoutil.MarshalOrPanic(&lb.StateData{
				Type: &lb.StateData_Bytes{Bytes: []byte("bytes")},
			})))

			key, value = fakeState.PutStateArgsForCall(2)
			Expect(key).To(Equal("namespaces/fields/fake/Proto"))
			Expect(value).To(Equal(protoutil.MarshalOrPanic(&lb.StateData{
				Type: &lb.StateData_Bytes{Bytes: protoutil.MarshalOrPanic(testStruct.Proto)},
			})))

			key, value = fakeState.PutStateArgsForCall(3)
			Expect(key).To(Equal("namespaces/fields/fake/String"))
			Expect(value).To(Equal(protoutil.MarshalOrPanic(&lb.StateData{
				Type: &lb.StateData_String_{String_: "theory"},
			})))

			key, value = fakeState.PutStateArgsForCall(4)
			Expect(key).To(Equal("namespaces/metadata/fake"))
			Expect(value).To(Equal(protoutil.MarshalOrPanic(&lb.StateMetadata{
				Datatype: "TestStruct",
				Fields:   []string{"Int", "Bytes", "Proto", "String"},
			})))

			Expect(fakeState.DelStateCallCount()).To(Equal(0))
		})

		Context("when the namespace contains extraneous keys", func() {
			BeforeEach(func() {
				kvs := map[string][]byte{
					"namespaces/fields/fake/ExtraneousKey1": protoutil.MarshalOrPanic(&lb.StateData{
						Type: &lb.StateData_Bytes{Bytes: []byte("value1")},
					}),
					"namespaces/fields/fake/ExtraneousKey2": protoutil.MarshalOrPanic(&lb.StateData{
						Type: &lb.StateData_Bytes{Bytes: []byte("value2")},
					}),
					"namespaces/metadata/fake": protoutil.MarshalOrPanic(&lb.StateMetadata{
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
			var kvs map[string][]byte

			BeforeEach(func() {
				kvs = map[string][]byte{
					"namespaces/fields/fake/Int": protoutil.MarshalOrPanic(&lb.StateData{
						Type: &lb.StateData_Int64{Int64: -3},
					}),
					"namespaces/fields/fake/Bytes": protoutil.MarshalOrPanic(&lb.StateData{
						Type: &lb.StateData_Bytes{Bytes: []byte("bytes")},
					}),
					"namespaces/fields/fake/Proto": protoutil.MarshalOrPanic(&lb.StateData{
						Type: &lb.StateData_Bytes{Bytes: protoutil.MarshalOrPanic(testStruct.Proto)},
					}),
					"namespaces/fields/fake/String": protoutil.MarshalOrPanic(&lb.StateData{
						Type: &lb.StateData_String_{String_: "theory"},
					}),
					"namespaces/metadata/fake": protoutil.MarshalOrPanic(&lb.StateMetadata{
						Datatype: "TestStruct",
						Fields:   []string{"Int", "Bytes", "Proto", "String"},
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
					kvs["namespaces/metadata/fake"] = protoutil.MarshalOrPanic(&lb.StateMetadata{
						Datatype: "TestStruct",
						Fields:   []string{"Proto", "Bytes"},
					})
					delete(kvs, "namespaces/fields/fake/Int")
				})

				It("writes the missing field and new metadata ", func() {
					err := s.Serialize("namespaces", "fake", testStruct, fakeState)
					Expect(err).NotTo(HaveOccurred())

					Expect(fakeState.PutStateCallCount()).To(Equal(3))
					key, value := fakeState.PutStateArgsForCall(0)
					Expect(value).To(Equal(protoutil.MarshalOrPanic(&lb.StateData{
						Type: &lb.StateData_Int64{Int64: -3},
					})))
					Expect(key).To(Equal("namespaces/fields/fake/Int"))
					key, value = fakeState.PutStateArgsForCall(1)
					Expect(key).To(Equal("namespaces/fields/fake/String"))
					Expect(value).To(Equal(protoutil.MarshalOrPanic(&lb.StateData{
						Type: &lb.StateData_String_{String_: "theory"},
					})))
					key, value = fakeState.PutStateArgsForCall(2)
					Expect(key).To(Equal("namespaces/metadata/fake"))
					Expect(value).To(Equal(protoutil.MarshalOrPanic(&lb.StateMetadata{
						Datatype: "TestStruct",
						Fields:   []string{"Int", "Bytes", "Proto", "String"},
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
					Expect(err.Error()).To(ContainSubstring("could not deserialize metadata for namespace namespaces/fake: could not unmarshal metadata for namespace namespaces/fake"))
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

		Context("when the argument contains a non-proto pointer", func() {
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
				fakeState.GetStateReturnsOnCall(0, protoutil.MarshalOrPanic(&lb.StateMetadata{
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
			kvs      map[string][]byte
			metadata *lb.StateMetadata
		)

		BeforeEach(func() {
			metadata = &lb.StateMetadata{
				Datatype: "TestStruct",
				Fields:   []string{"Int", "Bytes", "Proto"},
			}

			kvs = map[string][]byte{
				"namespaces/fields/fake/Int": protoutil.MarshalOrPanic(&lb.StateData{
					Type: &lb.StateData_Int64{Int64: -3},
				}),
				"namespaces/fields/fake/Bytes": protoutil.MarshalOrPanic(&lb.StateData{
					Type: &lb.StateData_Bytes{Bytes: []byte("bytes")},
				}),
				"namespaces/fields/fake/Proto": protoutil.MarshalOrPanic(&lb.StateData{
					Type: &lb.StateData_Bytes{Bytes: protoutil.MarshalOrPanic(testStruct.Proto)},
				}),
				"namespaces/fields/fake/String": protoutil.MarshalOrPanic(&lb.StateData{
					Type: &lb.StateData_String_{String_: "theory"},
				}),
			}

			fakeState.GetStateStub = func(key string) ([]byte, error) {
				fmt.Println("returning", kvs[key], "for", key)
				return kvs[key], nil
			}
		})

		It("populates the given struct with values from the state", func() {
			target := &TestStruct{}
			err := s.Deserialize("namespaces", "fake", metadata, target, fakeState)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeState.GetStateCallCount()).To(Equal(4))

			Expect(target.Int).To(Equal(int64(-3)))
			Expect(target.Bytes).To(Equal([]byte("bytes")))
			Expect(target.String).To(Equal("theory"))
			Expect(proto.Equal(target.Proto, testStruct.Proto)).To(BeTrue())
		})

		Context("when the field encoding is bad", func() {
			BeforeEach(func() {
				kvs["namespaces/fields/fake/Int"] = []byte("bad-data")
			})

			It("fails", func() {
				testStruct := &TestStruct{}
				err := s.Deserialize("namespaces", "fake", metadata, testStruct, fakeState)
				Expect(err.Error()).To(ContainSubstring("could not unmarshal state for key namespaces/fields/fake/Int"))
			})
		})

		Context("when the int is not the correct type", func() {
			BeforeEach(func() {
				kvs["namespaces/fields/fake/Int"] = kvs["namespaces/fields/fake/Proto"]
			})

			It("fails", func() {
				testStruct := &TestStruct{}
				err := s.Deserialize("namespaces", "fake", metadata, testStruct, fakeState)
				Expect(err).To(MatchError("expected key namespaces/fields/fake/Int to encode a value of type Int64, but was *lifecycle.StateData_Bytes"))
			})
		})

		Context("when the bytes are not the correct type", func() {
			BeforeEach(func() {
				kvs["namespaces/fields/fake/Bytes"] = kvs["namespaces/fields/fake/Int"]
			})

			It("fails", func() {
				testStruct := &TestStruct{}
				err := s.Deserialize("namespaces", "fake", metadata, testStruct, fakeState)
				Expect(err).To(MatchError("expected key namespaces/fields/fake/Bytes to encode a value of type []byte, but was *lifecycle.StateData_Int64"))
			})
		})

		Context("when the proto is not the correct type", func() {
			BeforeEach(func() {
				kvs["namespaces/fields/fake/Proto"] = kvs["namespaces/fields/fake/Int"]
			})

			It("fails", func() {
				testStruct := &TestStruct{}
				err := s.Deserialize("namespaces", "fake", metadata, testStruct, fakeState)
				Expect(err).To(MatchError("expected key namespaces/fields/fake/Proto to encode a value of type []byte, but was *lifecycle.StateData_Int64"))
			})
		})

		Context("when the bytes are not the correct type", func() {
			BeforeEach(func() {
				kvs["namespaces/fields/fake/String"] = kvs["namespaces/fields/fake/Int"]
			})

			It("fails", func() {
				testStruct := &TestStruct{}
				err := s.Deserialize("namespaces", "fake", metadata, testStruct, fakeState)
				Expect(err).To(MatchError("expected key namespaces/fields/fake/String to encode a value of type String, but was *lifecycle.StateData_Int64"))
			})
		})

		Context("when the state cannot be queried", func() {
			BeforeEach(func() {
				fakeState.GetStateReturns(nil, fmt.Errorf("state-error"))
			})

			It("fails", func() {
				testStruct := &TestStruct{}
				err := s.Deserialize("namespaces", "fake", metadata, testStruct, fakeState)
				Expect(err).To(MatchError("could not get state for key namespaces/fields/fake/Int: state-error"))
			})
		})

		Context("when the argument is not a pointer", func() {
			It("fails", func() {
				err := s.Deserialize("namespaces", "fake", metadata, 8, fakeState)
				Expect(err).To(MatchError("could not deserialize namespace namespaces/fake to unserializable type int: must be pointer to struct, but got non-pointer int"))
			})
		})

		Context("when the argument is a pointer to not-a-struct", func() {
			It("fails", func() {
				value := 7
				err := s.Deserialize("namespaces", "fake", metadata, &value, fakeState)
				Expect(err).To(MatchError("could not deserialize namespace namespaces/fake to unserializable type *int: must be pointers to struct, but got pointer to int"))
			})
		})

		Context("when the argument does not match the stored type", func() {
			It("it fails", func() {
				type Other struct{}
				err := s.Deserialize("namespaces", "fake", metadata, &Other{}, fakeState)
				Expect(err).To(MatchError("type name mismatch 'Other' != 'TestStruct'"))
			})
		})

		Context("when the argument contains an illegal field type", func() {
			BeforeEach(func() {
				kvs["namespaces/metadata/fake"] = protoutil.MarshalOrPanic(&lb.StateMetadata{
					Datatype: "BadStruct",
				})
			})

			It("it fails", func() {
				type BadStruct struct {
					BadField int
				}

				err := s.Deserialize("namespaces", "fake", metadata, &BadStruct{}, fakeState)
				Expect(err).To(MatchError("could not deserialize namespace namespaces/fake to unserializable type *lifecycle_test.BadStruct: unsupported structure field kind int for serialization for field BadField"))
			})
		})
	})

	Describe("Integration Round Trip of Serialize/Deserialize", func() {
		var KVStore map[string][]byte

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

			metadata, ok, err := s.DeserializeMetadata("namespace", "fake", fakeState)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
			deserialized := &TestStruct{}
			err = s.Deserialize("namespace", "fake", metadata, deserialized, fakeState)
			Expect(err).NotTo(HaveOccurred())

			Expect(testStruct.Int).To(Equal(deserialized.Int))
			Expect(proto.Equal(testStruct.Proto, deserialized.Proto))

			matched, err := s.IsSerialized("namespace", "fake", testStruct, fakeState)
			Expect(err).NotTo(HaveOccurred())
			Expect(matched).To(BeTrue())
		})
	})

	Describe("IsMetadataSerialized", func() {
		var kvs map[string][]byte

		BeforeEach(func() {
			kvs = map[string][]byte{
				"namespaces/metadata/fake": protoutil.MarshalOrPanic(&lb.StateMetadata{
					Datatype: "TestStruct",
					Fields:   []string{"Int", "Bytes", "Proto", "String"},
				}),
			}

			fakeState.GetStateHashStub = func(key string) ([]byte, error) {
				return util.ComputeSHA256(kvs[key]), nil
			}
		})

		It("checks to see if the metadata type is stored in the opaque state", func() {
			matched, err := s.IsMetadataSerialized("namespaces", "fake", testStruct, fakeState)
			Expect(err).NotTo(HaveOccurred())
			Expect(matched).To(BeTrue())

			Expect(fakeState.GetStateHashCallCount()).To(Equal(1))
			Expect(fakeState.GetStateHashArgsForCall(0)).To(Equal("namespaces/metadata/fake"))
		})

		Context("when the struct is not serializable", func() {
			It("wraps and returns the error", func() {
				_, err := s.IsMetadataSerialized("namespaces", "fake", nil, fakeState)
				Expect(err).To(MatchError("structure for namespace namespaces/fake is not serializable: must be pointer to struct, but got non-pointer invalid"))
			})
		})

		Context("when marshaling the metadata fails", func() {
			BeforeEach(func() {
				s.Marshaler = func(msg proto.Message) ([]byte, error) {
					return nil, fmt.Errorf("marshal-error")
				}
			})

			It("wraps and returns the error", func() {
				type Other struct{}
				_, err := s.IsMetadataSerialized("namespaces", "fake", &Other{}, fakeState)
				Expect(err).To(MatchError("could not marshal metadata for namespace namespaces/fake: marshal-error"))
			})
		})
	})

	Describe("IsSerialized", func() {
		var kvs map[string][]byte

		BeforeEach(func() {
			kvs = map[string][]byte{
				"namespaces/fields/fake/Int": protoutil.MarshalOrPanic(&lb.StateData{
					Type: &lb.StateData_Int64{Int64: -3},
				}),
				"namespaces/fields/fake/Bytes": protoutil.MarshalOrPanic(&lb.StateData{
					Type: &lb.StateData_Bytes{Bytes: []byte("bytes")},
				}),
				"namespaces/fields/fake/Proto": protoutil.MarshalOrPanic(&lb.StateData{
					Type: &lb.StateData_Bytes{Bytes: protoutil.MarshalOrPanic(testStruct.Proto)},
				}),
				"namespaces/fields/fake/String": protoutil.MarshalOrPanic(&lb.StateData{
					Type: &lb.StateData_String_{String_: "theory"},
				}),
				"namespaces/metadata/fake": protoutil.MarshalOrPanic(&lb.StateMetadata{
					Datatype: "TestStruct",
					Fields:   []string{"Int", "Bytes", "Proto", "String"},
				}),
			}

			fakeState.GetStateHashStub = func(key string) ([]byte, error) {
				return util.ComputeSHA256(kvs[key]), nil
			}
		})

		It("checks to see if the structure is stored in the opaque state", func() {
			matched, err := s.IsSerialized("namespaces", "fake", testStruct, fakeState)
			Expect(err).NotTo(HaveOccurred())
			Expect(matched).To(BeTrue())

			Expect(fakeState.GetStateHashCallCount()).To(Equal(5))
			Expect(fakeState.GetStateHashArgsForCall(0)).To(Equal("namespaces/metadata/fake"))
			Expect(fakeState.GetStateHashArgsForCall(1)).To(Equal("namespaces/fields/fake/Int"))
			Expect(fakeState.GetStateHashArgsForCall(2)).To(Equal("namespaces/fields/fake/Bytes"))
			Expect(fakeState.GetStateHashArgsForCall(3)).To(Equal("namespaces/fields/fake/Proto"))
			Expect(fakeState.GetStateHashArgsForCall(4)).To(Equal("namespaces/fields/fake/String"))
		})

		Context("when the namespace contains extraneous keys", func() {
			BeforeEach(func() {
				kvs["namespaces/metadata/fake"] = protoutil.MarshalOrPanic(&lb.StateMetadata{
					Datatype: "TestStruct",
					Fields:   []string{"Int", "Uint", "String", "Bytes", "Other"},
				})
				kvs["namespaces/fields/fake/Other"] = protoutil.MarshalOrPanic(&lb.StateData{
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
				kvs["namespaces/fields/fake/Int"] = protoutil.MarshalOrPanic(&lb.StateData{
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
				"namespaces/metadata/thing0": protoutil.MarshalOrPanic(&lb.StateMetadata{
					Datatype: "TestDatatype0",
				}),
				"namespaces/metadata/thing1": protoutil.MarshalOrPanic(&lb.StateMetadata{
					Datatype: "TestDatatype1",
				}),
				"namespaces/metadata/thing2": protoutil.MarshalOrPanic(&lb.StateMetadata{
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
				Expect(err.Error()).To(ContainSubstring("error unmarshalling metadata for key namespaces/metadata/bad"))
			})
		})
	})

	Describe("DeserializeMetadata", func() {
		BeforeEach(func() {
			fakeState.GetStateReturns(protoutil.MarshalOrPanic(&lb.StateMetadata{
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
				Expect(err.Error()).To(ContainSubstring("could not unmarshal metadata for namespace namespaces/fake"))
			})
		})
	})

	Describe("DeserializeFieldAsBytes", func() {
		BeforeEach(func() {
			fakeState.GetStateReturns(protoutil.MarshalOrPanic(&lb.StateData{
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
			fakeState.GetStateReturns(protoutil.MarshalOrPanic(&lb.StateData{
				Type: &lb.StateData_Bytes{Bytes: protoutil.MarshalOrPanic(&lb.InstallChaincodeResult{PackageId: "hash"})},
			}), nil)
		})

		It("deserializes the field to a string", func() {
			result := &lb.InstallChaincodeResult{}
			err := s.DeserializeFieldAsProto("namespaces", "fake", "field", fakeState, result)
			Expect(err).NotTo(HaveOccurred())
			Expect(proto.Equal(result, &lb.InstallChaincodeResult{PackageId: "hash"})).To(BeTrue())

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
				fakeState.GetStateReturns(protoutil.MarshalOrPanic(&lb.StateData{
					Type: &lb.StateData_Bytes{Bytes: []byte("garbage")},
				}), nil)
			})

			It("wraps and returns the error", func() {
				err := s.DeserializeFieldAsProto("namespaces", "fake", "field", fakeState, &lb.InstallChaincodeResult{})
				Expect(err).To(Not(BeNil()))
				Expect(err.Error()).To(HavePrefix("could not unmarshal key namespaces/fields/fake/field to *lifecycle.InstallChaincodeResult"))
			})
		})
	})

	Describe("DeserializeFieldAsInt64", func() {
		BeforeEach(func() {
			fakeState.GetStateReturns(protoutil.MarshalOrPanic(&lb.StateData{
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
				Expect(err.Error()).To(ContainSubstring("could not unmarshal state for key namespaces/fields/fake/field"))
			})
		})
	})

	Describe("DeserializeFieldAsString", func() {
		BeforeEach(func() {
			fakeState.GetStateReturns(protoutil.MarshalOrPanic(&lb.StateData{
				Type: &lb.StateData_String_{String_: "theory"},
			}), nil)
		})

		It("deserializes the field to a string", func() {
			result, err := s.DeserializeFieldAsString("namespaces", "fake", "field", fakeState)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("theory"))

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

			It("returns nil", func() {
				result, err := s.DeserializeFieldAsString("namespaces", "fake", "field", fakeState)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(""))
			})
		})

		Context("when the field has trailing data", func() {
			BeforeEach(func() {
				fakeState.GetStateReturns([]byte("bad-data"), nil)
			})

			It("returns an error", func() {
				_, err := s.DeserializeFieldAsString("namespaces", "fake", "field", fakeState)
				Expect(err.Error()).To(ContainSubstring("could not unmarshal state for key namespaces/fields/fake/field"))
			})
		})
	})
})
