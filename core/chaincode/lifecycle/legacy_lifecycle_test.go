/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle_test

import (
	"fmt"

	"github.com/hyperledger/fabric/core/chaincode/lifecycle"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle/mock"
	"github.com/hyperledger/fabric/core/common/ccprovider"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Lifecycle", func() {
	Describe("LegacyShim", func() {
		var (
			l                 *lifecycle.Lifecycle
			fakeLegacyImpl    *mock.LegacyLifecycle
			fakePublicState   MapLedgerShim
			fakeQueryExecutor *mock.SimpleQueryExecutor
		)

		BeforeEach(func() {
			fakeLegacyImpl = &mock.LegacyLifecycle{}

			l = &lifecycle.Lifecycle{
				Serializer: &lifecycle.Serializer{},
				LegacyImpl: fakeLegacyImpl,
			}

			fakePublicState = MapLedgerShim(map[string][]byte{})
			fakeQueryExecutor = &mock.SimpleQueryExecutor{}
			fakeQueryExecutor.GetStateStub = func(namespace, key string) ([]byte, error) {
				return fakePublicState.GetState(key)
			}
		})

		Describe("ChaincodeDefinition", func() {

			BeforeEach(func() {
				err := l.Serializer.Serialize(lifecycle.NamespacesName,
					"name",
					&lifecycle.DefinedChaincode{
						Version:             "version",
						Hash:                []byte("hash"),
						EndorsementPlugin:   "endorsement-plugin",
						ValidationPlugin:    "validation-plugin",
						ValidationParameter: []byte("validation-parameter"),
					},
					fakePublicState,
				)
				Expect(err).NotTo(HaveOccurred())
			})

			It("adapts the underlying lifecycle response", func() {
				def, err := l.ChaincodeDefinition("name", fakeQueryExecutor)
				Expect(err).NotTo(HaveOccurred())
				Expect(def).To(Equal(&lifecycle.LegacyDefinition{
					Name:                "name",
					Version:             "version",
					HashField:           []byte("hash"),
					EndorsementPlugin:   "endorsement-plugin",
					ValidationPlugin:    "validation-plugin",
					ValidationParameter: []byte("validation-parameter"),
				}))
			})

			Context("when the metadata is corrupt", func() {
				BeforeEach(func() {
					fakePublicState["namespaces/metadata/name"] = []byte("garbage")
				})

				It("wraps and returns that error", func() {
					_, err := l.ChaincodeDefinition("name", fakeQueryExecutor)
					Expect(err).To(MatchError("could not deserialize metadata for chaincode name: could not unmarshal metadata for namespace namespaces/name: proto: can't skip unknown wire type 7"))
				})
			})

			Context("when the metadata is not for a chaincode", func() {
				BeforeEach(func() {
					type badStruct struct{}
					err := l.Serializer.Serialize(lifecycle.NamespacesName,
						"name",
						&badStruct{},
						fakePublicState,
					)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns an error", func() {
					_, err := l.ChaincodeDefinition("name", fakeQueryExecutor)
					Expect(err).To(MatchError("not a chaincode type: badStruct"))
				})
			})

			Context("when the data is corrupt", func() {
				BeforeEach(func() {
					fakePublicState["namespaces/fields/name/Version"] = []byte("garbage")
				})

				It("wraps and returns that error", func() {
					_, err := l.ChaincodeDefinition("name", fakeQueryExecutor)
					Expect(err).To(MatchError("could not deserialize chaincode definition for chaincode name: could not unmarshal state for key namespaces/fields/name/Version: proto: can't skip unknown wire type 7"))
				})
			})

			Context("when the chaincode is not defined in the new lifecycle", func() {
				var (
					legacyChaincodeDefinition *ccprovider.ChaincodeData
				)

				BeforeEach(func() {
					delete(fakePublicState, "namespaces/metadata/name")
					legacyChaincodeDefinition = &ccprovider.ChaincodeData{
						Name:    "definition-name",
						Version: "definition-version",
					}

					fakeLegacyImpl.ChaincodeDefinitionReturns(legacyChaincodeDefinition, fmt.Errorf("fake-error"))
				})

				It("passes through the legacy implementation", func() {
					res, err := l.ChaincodeDefinition("cc-name", fakeQueryExecutor)
					Expect(err).To(MatchError("fake-error"))
					Expect(res).To(Equal(legacyChaincodeDefinition))
					Expect(fakeLegacyImpl.ChaincodeDefinitionCallCount()).To(Equal(1))
					name, qe := fakeLegacyImpl.ChaincodeDefinitionArgsForCall(0)
					Expect(name).To(Equal("cc-name"))
					Expect(qe).To(Equal(fakeQueryExecutor))
				})
			})
		})

		Describe("ChaincodeContainerInfo", func() {
			var (
				fakeQueryExecutor   *mock.SimpleQueryExecutor
				legacyContainerInfo *ccprovider.ChaincodeContainerInfo
			)

			BeforeEach(func() {
				fakeQueryExecutor = &mock.SimpleQueryExecutor{}

				legacyContainerInfo = &ccprovider.ChaincodeContainerInfo{
					Name:    "definition-name",
					Version: "definition-version",
				}

				fakeLegacyImpl.ChaincodeContainerInfoReturns(legacyContainerInfo, fmt.Errorf("fake-error"))
			})

			It("passes through the legacy implementation", func() {
				res, err := l.ChaincodeContainerInfo("cc-name", fakeQueryExecutor)
				Expect(err).To(MatchError("fake-error"))
				Expect(res).To(Equal(legacyContainerInfo))
				Expect(fakeLegacyImpl.ChaincodeContainerInfoCallCount()).To(Equal(1))
				name, qe := fakeLegacyImpl.ChaincodeContainerInfoArgsForCall(0)
				Expect(name).To(Equal("cc-name"))
				Expect(qe).To(Equal(fakeQueryExecutor))
			})
		})
	})

	Describe("LegacyDefinition", func() {
		var (
			ld *lifecycle.LegacyDefinition
		)

		BeforeEach(func() {
			ld = &lifecycle.LegacyDefinition{
				Name:                "name",
				Version:             "version",
				HashField:           []byte("hash"),
				EndorsementPlugin:   "endorsement-plugin",
				ValidationPlugin:    "validation-plugin",
				ValidationParameter: []byte("validation-parameter"),
			}
		})

		Describe("CCName", func() {
			It("returns the name", func() {
				Expect(ld.CCName()).To(Equal("name"))
			})
		})

		Describe("CCVersion", func() {
			It("returns the version", func() {
				Expect(ld.CCVersion()).To(Equal("version"))
			})
		})

		Describe("Hash", func() {
			It("returns the hash", func() {
				Expect(ld.Hash()).To(Equal([]byte("hash")))
			})
		})

		Describe("Endorsement", func() {
			It("returns the endorsment plugin name", func() {
				Expect(ld.Endorsement()).To(Equal("endorsement-plugin"))
			})
		})

		Describe("Validation", func() {
			It("returns the validation plugin name and parameter", func() {
				validationPlugin, validationParameter := ld.Validation()
				Expect(validationPlugin).To(Equal("validation-plugin"))
				Expect(validationParameter).To(Equal([]byte("validation-parameter")))
			})
		})
	})
})
