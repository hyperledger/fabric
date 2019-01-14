/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle_test

import (
	"fmt"

	"github.com/hyperledger/fabric/common/chaincode"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle/mock"
	cb "github.com/hyperledger/fabric/protos/common"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/golang/protobuf/proto"
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
			Serializer:     &lifecycle.Serializer{},
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

	Describe("QueryInstalledChaincode", func() {
		BeforeEach(func() {
			fakeCCStore.RetrieveHashReturns([]byte("fake-hash"), nil)
		})

		It("passes through to the backing chaincode store", func() {
			hash, err := l.QueryInstalledChaincode("name", "version")
			Expect(err).NotTo(HaveOccurred())
			Expect(hash).To(Equal([]byte("fake-hash")))
			Expect(fakeCCStore.RetrieveHashCallCount()).To(Equal(1))
			name, version := fakeCCStore.RetrieveHashArgsForCall(0)
			Expect(name).To(Equal("name"))
			Expect(version).To(Equal("version"))
		})

		Context("when the backing chaincode store fails to retrieve the hash", func() {
			BeforeEach(func() {
				fakeCCStore.RetrieveHashReturns(nil, fmt.Errorf("fake-error"))
			})
			It("wraps and returns the error", func() {
				hash, err := l.QueryInstalledChaincode("name", "version")
				Expect(hash).To(BeNil())
				Expect(err).To(MatchError("could not retrieve hash for chaincode 'name:version': fake-error"))
			})
		})
	})

	Describe("QueryInstalledChaincodes", func() {
		var chaincodes []chaincode.InstalledChaincode

		BeforeEach(func() {
			chaincodes = []chaincode.InstalledChaincode{
				{
					Name:    "cc1-name",
					Version: "cc1-version",
					Id:      []byte("cc1-hash"),
				},
				{
					Name:    "cc2-name",
					Version: "cc2-version",
					Id:      []byte("cc2-hash"),
				},
			}

			fakeCCStore.ListInstalledChaincodesReturns(chaincodes, fmt.Errorf("fake-error"))
		})

		It("passes through to the backing chaincode store", func() {
			result, err := l.QueryInstalledChaincodes()
			Expect(result).To(Equal(chaincodes))
			Expect(err).To(MatchError(fmt.Errorf("fake-error")))
		})
	})

	Describe("DefineChaincodeForOrg", func() {
		var (
			fakePublicState *mock.ReadWritableState
			fakeOrgState    *mock.ReadWritableState

			fakeOrgKVStore    MapLedgerShim
			fakePublicKVStore MapLedgerShim

			testDefinition *lifecycle.ChaincodeDefinition
		)

		BeforeEach(func() {
			testDefinition = &lifecycle.ChaincodeDefinition{
				Name:     "cc-name",
				Sequence: 5,
				Parameters: &lifecycle.ChaincodeParameters{
					Version: "version",
				},
			}

			fakePublicState = &mock.ReadWritableState{}
			fakePublicKVStore = MapLedgerShim(map[string][]byte{})
			fakePublicState = &mock.ReadWritableState{}
			fakePublicState.PutStateStub = fakePublicKVStore.PutState
			fakePublicState.GetStateStub = fakePublicKVStore.GetState

			fakeOrgKVStore = MapLedgerShim(map[string][]byte{})
			fakeOrgState = &mock.ReadWritableState{}
			fakeOrgState.PutStateStub = fakeOrgKVStore.PutState
			fakeOrgState.GetStateStub = fakeOrgKVStore.GetState

			err := l.Serializer.Serialize("namespaces", "cc-name", &lifecycle.DefinedChaincode{
				Sequence: 4,
			}, fakePublicKVStore)
			Expect(err).NotTo(HaveOccurred())
		})

		It("serializes the chaincode parameters to the org scoped collection", func() {
			err := l.DefineChaincodeForOrg(testDefinition, fakePublicState, fakeOrgState)
			Expect(err).NotTo(HaveOccurred())

			committedDefinition := &lifecycle.ChaincodeParameters{}
			err = l.Serializer.Deserialize("namespaces", "cc-name#5", committedDefinition, fakeOrgState)
			Expect(err).NotTo(HaveOccurred())
			Expect(committedDefinition.Version).To(Equal("version"))
			Expect(committedDefinition.Hash).To(BeEmpty())
			Expect(committedDefinition.ValidationParameter).To(BeEmpty())
			Expect(proto.Equal(committedDefinition.Collections, &cb.CollectionConfigPackage{})).To(BeTrue())
		})

		Context("when the current sequence is undefined and the requested sequence is 0", func() {
			BeforeEach(func() {
				fakePublicKVStore = map[string][]byte{}
			})

			It("returns an error", func() {
				err := l.DefineChaincodeForOrg(&lifecycle.ChaincodeDefinition{}, fakePublicState, fakeOrgState)
				Expect(err).To(MatchError("requested sequence is 0, but first definable sequence number is 1"))
			})
		})

		Context("when the sequence number already has a definition", func() {
			BeforeEach(func() {
				fakePublicKVStore = map[string][]byte{}

				err := l.Serializer.Serialize("namespaces", "cc-name", &lifecycle.DefinedChaincode{
					Sequence: 5,
					Version:  "version",
				}, fakePublicState)
				Expect(err).NotTo(HaveOccurred())
			})

			It("verifies that the definition matches before writing", func() {
				err := l.DefineChaincodeForOrg(testDefinition, fakePublicState, fakeOrgState)
				Expect(err).NotTo(HaveOccurred())
			})

			Context("when the current definition is not a chaincode", func() {
				BeforeEach(func() {
					fakePublicKVStore = map[string][]byte{}
					type OtherStruct struct {
						Sequence int64
					}
					err := l.Serializer.Serialize("namespaces", "cc-name", &OtherStruct{
						Sequence: 5,
					}, fakePublicState)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns an error", func() {
					err := l.DefineChaincodeForOrg(testDefinition, fakePublicState, fakeOrgState)
					Expect(err).To(MatchError("could not deserialize namespace cc-name as chaincode: type name mismatch 'DefinedChaincode' != 'OtherStruct'"))
				})
			})

			Context("when the Version in the new definition differs from the current definition", func() {
				BeforeEach(func() {
					fakePublicKVStore = map[string][]byte{}

					err := l.Serializer.Serialize("namespaces", "cc-name", &lifecycle.DefinedChaincode{
						Sequence: 5,
						Version:  "other-version",
					}, fakePublicState)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns an error", func() {
					err := l.DefineChaincodeForOrg(testDefinition, fakePublicState, fakeOrgState)
					Expect(err).To(MatchError("attempted to define the current sequence (5) for namespace cc-name, but Version 'other-version' != 'version'"))
				})
			})

			Context("when the EndorsementPlugin differs from the current definition", func() {
				BeforeEach(func() {
					testDefinition.Parameters.EndorsementPlugin = "different"
				})

				It("returns an error", func() {
					err := l.DefineChaincodeForOrg(testDefinition, fakePublicState, fakeOrgState)
					Expect(err).To(MatchError("attempted to define the current sequence (5) for namespace cc-name, but EndorsementPlugin '' != 'different'"))
				})
			})

			Context("when the ValidationPlugin differs from the current definition", func() {
				BeforeEach(func() {
					testDefinition.Parameters.ValidationPlugin = "different"
				})

				It("returns an error", func() {
					err := l.DefineChaincodeForOrg(testDefinition, fakePublicState, fakeOrgState)
					Expect(err).To(MatchError("attempted to define the current sequence (5) for namespace cc-name, but ValidationPlugin '' != 'different'"))
				})
			})

			Context("when the ValidationParameter differs from the current definition", func() {
				BeforeEach(func() {
					testDefinition.Parameters.ValidationParameter = []byte("different")
				})

				It("returns an error", func() {
					err := l.DefineChaincodeForOrg(testDefinition, fakePublicState, fakeOrgState)
					Expect(err).To(MatchError("attempted to define the current sequence (5) for namespace cc-name, but ValidationParameter '' != '646966666572656e74'"))
				})
			})

			Context("when the Hash differs from the current definition", func() {
				BeforeEach(func() {
					testDefinition.Parameters.Hash = []byte("different")
				})

				It("returns an error", func() {
					err := l.DefineChaincodeForOrg(testDefinition, fakePublicState, fakeOrgState)
					Expect(err).To(MatchError("attempted to define the current sequence (5) for namespace cc-name, but Hash '' != '646966666572656e74'"))
				})
			})

			Context("when the Collections differ from the current definition", func() {
				BeforeEach(func() {
					testDefinition.Parameters.Collections = &cb.CollectionConfigPackage{
						Config: []*cb.CollectionConfig{
							{
								Payload: &cb.CollectionConfig_StaticCollectionConfig{
									StaticCollectionConfig: &cb.StaticCollectionConfig{Name: "foo"},
								},
							},
						},
					}
				})

				It("returns an error", func() {
					err := l.DefineChaincodeForOrg(testDefinition, fakePublicState, fakeOrgState)
					Expect(err).To(MatchError("attempted to define the current sequence (5) for namespace cc-name, but Collections do not match"))
				})
			})
		})

		Context("when the definition is for an expired sequence number", func() {
			BeforeEach(func() {
				testDefinition.Sequence = 3
			})

			It("fails", func() {
				err := l.DefineChaincodeForOrg(testDefinition, fakePublicState, fakeOrgState)
				Expect(err).To(MatchError("currently defined sequence 4 is larger than requested sequence 3"))
			})
		})

		Context("when the definition is for a distant sequence number", func() {
			BeforeEach(func() {
				testDefinition.Sequence = 9
			})

			It("fails", func() {
				err := l.DefineChaincodeForOrg(testDefinition, fakePublicState, fakeOrgState)
				Expect(err).To(MatchError("requested sequence 9 is larger than the next available sequence number 5"))
			})
		})

		Context("when querying the public state fails", func() {
			BeforeEach(func() {
				fakePublicState.GetStateReturns(nil, fmt.Errorf("get-state-error"))
			})

			It("wraps and returns the error", func() {
				err := l.DefineChaincodeForOrg(testDefinition, fakePublicState, fakeOrgState)
				Expect(err).To(MatchError("could not get current sequence: could not get state for key namespaces/fields/cc-name/Sequence: get-state-error"))
			})
		})

		Context("when writing to the public state fails", func() {
			BeforeEach(func() {
				fakeOrgState.PutStateReturns(fmt.Errorf("put-state-error"))
			})

			It("wraps and returns the error", func() {
				err := l.DefineChaincodeForOrg(testDefinition, fakePublicState, fakeOrgState)
				Expect(err).To(MatchError("could not serialize chaincode parameters to state: could not write key into state: put-state-error"))
			})
		})
	})

	Describe("DefineChaincode", func() {
		var (
			fakePublicState *mock.ReadWritableState
			fakeOrgStates   []*mock.ReadWritableState

			testDefinition *lifecycle.ChaincodeDefinition

			publicKVS, org0KVS, org1KVS MapLedgerShim
		)

		BeforeEach(func() {
			testDefinition = &lifecycle.ChaincodeDefinition{
				Name:     "cc-name",
				Sequence: 5,
				Parameters: &lifecycle.ChaincodeParameters{
					Version:             "version",
					Hash:                []byte("hash"),
					EndorsementPlugin:   "endorsement-plugin",
					ValidationPlugin:    "validation-plugin",
					ValidationParameter: []byte("validation-parameter"),
				},
			}

			publicKVS = MapLedgerShim(map[string][]byte{})
			fakePublicState = &mock.ReadWritableState{}
			fakePublicState.GetStateStub = publicKVS.GetState
			fakePublicState.PutStateStub = publicKVS.PutState

			l.Serializer.Serialize("namespaces", "cc-name", &lifecycle.DefinedChaincode{
				Sequence:            4,
				Version:             "version",
				Hash:                []byte("hash"),
				EndorsementPlugin:   "endorsement-plugin",
				ValidationPlugin:    "validation-plugin",
				ValidationParameter: []byte("validation-parameter"),
			}, publicKVS)

			org0KVS = MapLedgerShim(map[string][]byte{})
			org1KVS = MapLedgerShim(map[string][]byte{})
			fakeOrgStates = []*mock.ReadWritableState{{}, {}}
			for i, kvs := range []MapLedgerShim{org0KVS, org1KVS} {
				kvs := kvs
				fakeOrgStates[i].GetStateStub = kvs.GetState
				fakeOrgStates[i].GetStateHashStub = kvs.GetStateHash
				fakeOrgStates[i].PutStateStub = kvs.PutState
			}

			l.Serializer.Serialize("namespaces", "cc-name#5", testDefinition.Parameters, fakeOrgStates[0])
			l.Serializer.Serialize("namespaces", "cc-name#5", &lifecycle.ChaincodeParameters{}, fakeOrgStates[1])
		})

		It("applies the chaincode definition and returns the agreements", func() {
			agreements, err := l.DefineChaincode(testDefinition, fakePublicState, []lifecycle.OpaqueState{fakeOrgStates[0], fakeOrgStates[1]})
			Expect(err).NotTo(HaveOccurred())
			Expect(agreements).To(Equal([]bool{true, false}))
		})

		Context("when the public state is not readable", func() {
			BeforeEach(func() {
				fakePublicState.GetStateReturns(nil, fmt.Errorf("getstate-error"))
			})

			It("wraps and returns the error", func() {
				_, err := l.DefineChaincode(testDefinition, fakePublicState, []lifecycle.OpaqueState{fakeOrgStates[0], fakeOrgStates[1]})
				Expect(err).To(MatchError("could not get current sequence: could not get state for key namespaces/fields/cc-name/Sequence: getstate-error"))
			})
		})

		Context("when the public state is not writable", func() {
			BeforeEach(func() {
				fakePublicState.PutStateReturns(fmt.Errorf("putstate-error"))
			})

			It("wraps and returns the error", func() {
				_, err := l.DefineChaincode(testDefinition, fakePublicState, []lifecycle.OpaqueState{fakeOrgStates[0], fakeOrgStates[1]})
				Expect(err).To(MatchError("could not serialize chaincode definition: could not write key into state: putstate-error"))
			})
		})

		Context("when the current sequence is not immediately prior to the new", func() {
			BeforeEach(func() {
				l.Serializer.Serialize("namespaces", "cc-name", &lifecycle.DefinedChaincode{
					Sequence:            3,
					Version:             "version",
					Hash:                []byte("hash"),
					EndorsementPlugin:   "endorsement-plugin",
					ValidationPlugin:    "validation-plugin",
					ValidationParameter: []byte("validation-parameter"),
				}, fakePublicState)
			})

			It("returns an error", func() {
				_, err := l.DefineChaincode(testDefinition, fakePublicState, []lifecycle.OpaqueState{fakeOrgStates[0], fakeOrgStates[1]})
				Expect(err).To(MatchError("requested sequence is 5, but new definition must be sequence 4"))
			})
		})
	})

	Describe("QueryDefinedChaincode", func() {
		var (
			fakePublicState *mock.ReadWritableState

			publicKVS MapLedgerShim
		)

		BeforeEach(func() {
			publicKVS = MapLedgerShim(map[string][]byte{})
			fakePublicState = &mock.ReadWritableState{}
			fakePublicState.GetStateStub = publicKVS.GetState
			fakePublicState.PutStateStub = publicKVS.PutState

			l.Serializer.Serialize("namespaces", "cc-name", &lifecycle.DefinedChaincode{
				Sequence:            4,
				Version:             "version",
				Hash:                []byte("hash"),
				EndorsementPlugin:   "endorsement-plugin",
				ValidationPlugin:    "validation-plugin",
				ValidationParameter: []byte("validation-parameter"),
			}, publicKVS)
		})

		It("returns the defined chaincode", func() {
			cc, err := l.QueryDefinedChaincode("cc-name", fakePublicState)
			Expect(err).NotTo(HaveOccurred())
			Expect(cc).To(Equal(&lifecycle.DefinedChaincode{
				Sequence:            4,
				Version:             "version",
				Hash:                []byte("hash"),
				EndorsementPlugin:   "endorsement-plugin",
				ValidationPlugin:    "validation-plugin",
				ValidationParameter: []byte("validation-parameter"),
				Collections:         &cb.CollectionConfigPackage{},
			}))
		})

		Context("when the chaincode is not defined", func() {
			BeforeEach(func() {
				fakePublicState.GetStateReturns(nil, nil)
			})

			It("returns an error", func() {
				_, err := l.QueryDefinedChaincode("cc-name", fakePublicState)
				Expect(err).To(MatchError("could not deserialize namespace cc-name as chaincode: could not unmarshal metadata for namespace namespaces/cc-name: no existing serialized message found"))
			})
		})
	})

	Describe("QueryDefinedNamespaces", func() {
		var (
			fakePublicState *mock.ReadWritableState

			publicKVS MapLedgerShim
		)

		BeforeEach(func() {
			publicKVS = MapLedgerShim(map[string][]byte{})
			fakePublicState = &mock.ReadWritableState{}
			fakePublicState.GetStateStub = publicKVS.GetState
			fakePublicState.GetStateRangeStub = publicKVS.GetStateRange
			l.Serializer.Serialize("namespaces", "cc-name", &lifecycle.DefinedChaincode{}, publicKVS)
			l.Serializer.Serialize("namespaces", "other-name", &lifecycle.ChaincodeParameters{}, publicKVS)
		})

		It("returns the defined namespaces", func() {
			result, err := l.QueryDefinedNamespaces(fakePublicState)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(map[string]string{
				"cc-name":    "Chaincode",
				"other-name": "ChaincodeParameters",
			}))
		})

		Context("when the range cannot be retrieved", func() {
			BeforeEach(func() {
				fakePublicState.GetStateRangeReturns(nil, fmt.Errorf("state-range-error"))
			})

			It("returns an error", func() {
				_, err := l.QueryDefinedNamespaces(fakePublicState)
				Expect(err).To(MatchError("could not query namespace metadata: could not get state range for namespace namespaces: state-range-error"))
			})
		})
	})
})
