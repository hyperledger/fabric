/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle_test

import (
	"fmt"

	"github.com/hyperledger/fabric/core/chaincode/lifecycle"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle/mock"
	"github.com/hyperledger/fabric/core/chaincode/persistence"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	lb "github.com/hyperledger/fabric/protos/peer/lifecycle"

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
					&lifecycle.ChaincodeDefinition{
						EndorsementInfo: &lb.ChaincodeEndorsementInfo{
							Version:           "version",
							Id:                []byte("hash"),
							EndorsementPlugin: "endorsement-plugin",
						},
						ValidationInfo: &lb.ChaincodeValidationInfo{
							ValidationPlugin:    "validation-plugin",
							ValidationParameter: []byte("validation-parameter"),
						},
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
					Expect(err).To(MatchError("could not get definition for chaincode name: could not deserialize metadata for chaincode name: could not unmarshal metadata for namespace namespaces/name: proto: can't skip unknown wire type 7"))
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
					Expect(err).To(MatchError("could not get definition for chaincode name: not a chaincode type: badStruct"))
				})
			})

			Context("when the data is corrupt", func() {
				BeforeEach(func() {
					fakePublicState["namespaces/fields/name/ValidationInfo"] = []byte("garbage")
				})

				It("wraps and returns that error", func() {
					_, err := l.ChaincodeDefinition("name", fakeQueryExecutor)
					Expect(err).To(MatchError("could not get definition for chaincode name: could not deserialize chaincode definition for chaincode name: could not unmarshal state for key namespaces/fields/name/ValidationInfo: proto: can't skip unknown wire type 7"))
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
				legacyContainerInfo *ccprovider.ChaincodeContainerInfo
				fakeChaincodeStore  *mock.ChaincodeStore
				fakePackageParser   *mock.PackageParser
			)

			BeforeEach(func() {
				fakeChaincodeStore = &mock.ChaincodeStore{}
				fakeChaincodeStore.LoadReturns([]byte("package"), []*persistence.ChaincodeMetadata{{Name: "name", Version: "version"}}, nil)
				fakePackageParser = &mock.PackageParser{}
				fakePackageParser.ParseReturns(&persistence.ChaincodePackage{
					Metadata: &persistence.ChaincodePackageMetadata{
						Path: "fake-path",
						Type: "fake-type",
					},
				}, nil)

				l.ChaincodeStore = fakeChaincodeStore
				l.PackageParser = fakePackageParser

				err := l.Serializer.Serialize(lifecycle.NamespacesName,
					"name",
					&lifecycle.ChaincodeDefinition{
						EndorsementInfo: &lb.ChaincodeEndorsementInfo{
							Version:           "version",
							Id:                []byte("hash"),
							EndorsementPlugin: "endorsement-plugin",
						},
						ValidationInfo: &lb.ChaincodeValidationInfo{
							ValidationPlugin:    "validation-plugin",
							ValidationParameter: []byte("validation-parameter"),
						},
					},
					fakePublicState,
				)
				Expect(err).NotTo(HaveOccurred())

				legacyContainerInfo = &ccprovider.ChaincodeContainerInfo{
					Name:    "definition-name",
					Version: "definition-version",
				}

				fakeLegacyImpl.ChaincodeContainerInfoReturns(legacyContainerInfo, fmt.Errorf("fake-error"))
			})

			It("returns the current definition", func() {
				res, err := l.ChaincodeContainerInfo("name", fakeQueryExecutor)
				Expect(err).NotTo(HaveOccurred())
				Expect(res).To(Equal(&ccprovider.ChaincodeContainerInfo{
					Name:          "name",
					Version:       "version",
					Path:          "fake-path",
					Type:          "FAKE-TYPE",
					ContainerType: "DOCKER",
				}))

				Expect(fakeChaincodeStore.LoadCallCount()).To(Equal(1))
				Expect(fakeChaincodeStore.LoadArgsForCall(0)).To(Equal([]byte("hash")))
				Expect(fakePackageParser.ParseCallCount()).To(Equal(1))
				Expect(fakePackageParser.ParseArgsForCall(0)).To(Equal([]byte("package")))

			})

			Context("when the metadata is corrupt", func() {
				BeforeEach(func() {
					fakePublicState["namespaces/metadata/name"] = []byte("garbage")
				})

				It("wraps and returns that error", func() {
					_, err := l.ChaincodeContainerInfo("name", fakeQueryExecutor)
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
					_, err := l.ChaincodeContainerInfo("name", fakeQueryExecutor)
					Expect(err).To(MatchError("not a chaincode type: badStruct"))
				})
			})

			Context("when the data is corrupt", func() {
				BeforeEach(func() {
					fakePublicState["namespaces/fields/name/ValidationInfo"] = []byte("garbage")
				})

				It("wraps and returns that error", func() {
					_, err := l.ChaincodeContainerInfo("name", fakeQueryExecutor)
					Expect(err).To(MatchError("could not deserialize chaincode definition for chaincode name: could not unmarshal state for key namespaces/fields/name/ValidationInfo: proto: can't skip unknown wire type 7"))
				})
			})

			Context("when the definition does not exist in the new lifecycle", func() {
				It("passes through the legacy implementation", func() {
					res, err := l.ChaincodeContainerInfo("different-name", fakeQueryExecutor)
					Expect(err).To(MatchError("fake-error"))
					Expect(res).To(Equal(legacyContainerInfo))
					Expect(fakeLegacyImpl.ChaincodeContainerInfoCallCount()).To(Equal(1))
					name, qe := fakeLegacyImpl.ChaincodeContainerInfoArgsForCall(0)
					Expect(name).To(Equal("different-name"))
					Expect(qe).To(Equal(fakeQueryExecutor))
				})
			})

			Context("when the package cannot be retrieved", func() {
				BeforeEach(func() {
					fakeChaincodeStore.LoadReturns(nil, nil, fmt.Errorf("load-error"))
				})

				It("wraps and returns the error", func() {
					_, err := l.ChaincodeContainerInfo("name", fakeQueryExecutor)
					Expect(err).To(MatchError("could not load chaincode from chaincode store for name:version (68617368): load-error"))
				})
			})

			Context("when the package cannot be parsed", func() {
				BeforeEach(func() {
					fakePackageParser.ParseReturns(nil, fmt.Errorf("parse-error"))
				})

				It("wraps and returns the error", func() {
					_, err := l.ChaincodeContainerInfo("name", fakeQueryExecutor)
					Expect(err).To(MatchError("could not parse chaincode package for name:version (68617368): parse-error"))
				})
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
				RequiresInitField:   true,
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

		Describe("RequiresInit", func() {
			It("returns the endorsment init required field", func() {
				Expect(ld.RequiresInit()).To(BeTrue())
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
