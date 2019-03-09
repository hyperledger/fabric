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
			cei               *lifecycle.ChaincodeEndorsementInfo
			l                 *lifecycle.Lifecycle
			fakeLegacyImpl    *mock.LegacyLifecycle
			fakePublicState   MapLedgerShim
			fakeQueryExecutor *mock.SimpleQueryExecutor
			fakeCache         *mock.ChaincodeInfoCache
			testInfo          *lifecycle.LocalChaincodeInfo
		)

		BeforeEach(func() {
			fakeLegacyImpl = &mock.LegacyLifecycle{}

			l = &lifecycle.Lifecycle{
				Serializer: &lifecycle.Serializer{},
			}

			fakePublicState = MapLedgerShim(map[string][]byte{})
			fakeQueryExecutor = &mock.SimpleQueryExecutor{}
			fakeQueryExecutor.GetStateStub = func(namespace, key string) ([]byte, error) {
				return fakePublicState.GetState(key)
			}

			err := l.Serializer.Serialize(lifecycle.NamespacesName,
				"name",
				&lifecycle.ChaincodeDefinition{
					Sequence: 7,
				},
				fakePublicState,
			)
			Expect(err).NotTo(HaveOccurred())

			testInfo = &lifecycle.LocalChaincodeInfo{
				Definition: &lifecycle.ChaincodeDefinition{
					Sequence: 7,
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
				InstallInfo: &lifecycle.ChaincodeInstallInfo{
					Path: "fake-path",
					Type: "fake-type",
					Hash: []byte("hash"),
				},
				Approved: true,
			}

			fakeCache = &mock.ChaincodeInfoCache{}
			fakeCache.ChaincodeInfoReturns(testInfo, nil)

			cei = &lifecycle.ChaincodeEndorsementInfo{
				LegacyImpl: fakeLegacyImpl,
				Lifecycle:  l,
				Cache:      fakeCache,
			}

		})

		Describe("CachedChaincodeInfo", func() {
			It("returns the info from the cache", func() {
				info, ok, err := cei.CachedChaincodeInfo("channel-id", "name", fakeQueryExecutor)
				Expect(err).NotTo(HaveOccurred())
				Expect(ok).To(BeTrue())
				Expect(info).To(Equal(&lifecycle.LocalChaincodeInfo{
					Definition: &lifecycle.ChaincodeDefinition{
						Sequence: 7,
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
					InstallInfo: &lifecycle.ChaincodeInstallInfo{
						Type: "fake-type",
						Path: "fake-path",
						Hash: []byte("hash"),
					},
					Approved: true,
				}))
				Expect(fakeCache.ChaincodeInfoCallCount()).To(Equal(1))
				channelID, chaincodeName := fakeCache.ChaincodeInfoArgsForCall(0)
				Expect(channelID).To(Equal("channel-id"))
				Expect(chaincodeName).To(Equal("name"))
			})

			Context("when the cache returns an error", func() {
				BeforeEach(func() {
					fakeCache.ChaincodeInfoReturns(nil, fmt.Errorf("cache-error"))
				})

				It("wraps and returns the error", func() {
					_, _, err := cei.CachedChaincodeInfo("channel-id", "name", fakeQueryExecutor)
					Expect(err).To(MatchError("could not get approved chaincode info from cache: cache-error"))
				})
			})

			Context("when the cache has not been approved", func() {
				BeforeEach(func() {
					testInfo.Approved = false
				})

				It("returns an error", func() {
					_, _, err := cei.CachedChaincodeInfo("channel-id", "name", fakeQueryExecutor)
					Expect(err).To(MatchError("chaincode definition for 'name' at sequence 7 on channel 'channel-id' has not yet been approved by this org"))
				})
			})

			Context("when the chaincode has not been installed", func() {
				BeforeEach(func() {
					testInfo.InstallInfo = nil
				})

				It("returns an error", func() {
					_, _, err := cei.CachedChaincodeInfo("channel-id", "name", fakeQueryExecutor)
					Expect(err).To(MatchError("chaincode definition for 'name' exists, but chaincode is not installed"))
				})
			})

			Context("when the cache does not match the current sequence", func() {
				BeforeEach(func() {
					testInfo.Definition.Sequence = 5
				})

				It("returns an error", func() {
					_, _, err := cei.CachedChaincodeInfo("channel-id", "name", fakeQueryExecutor)
					Expect(err).To(MatchError("chaincode cache at sequence 5 but current sequence is 7, chaincode definition for 'name' changed during invoke"))
				})
			})

			Context("when the sequence cannot be fetched from the state", func() {
				BeforeEach(func() {
					fakeQueryExecutor.GetStateReturns(nil, fmt.Errorf("state-error"))
				})

				It("wraps and returns an error", func() {
					_, _, err := cei.CachedChaincodeInfo("channel-id", "name", fakeQueryExecutor)
					Expect(err).To(MatchError("could not get current sequence: could not get state for key namespaces/fields/name/Sequence: state-error"))
				})
			})
		})

		Describe("ChaincodeDefinition", func() {
			It("adapts the underlying lifecycle response", func() {
				def, err := cei.ChaincodeDefinition("channel-id", "name", fakeQueryExecutor)
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

			Context("when the cache returns an error", func() {
				BeforeEach(func() {
					fakeCache.ChaincodeInfoReturns(nil, fmt.Errorf("cache-error"))
				})

				It("returns the wrapped error", func() {
					_, err := cei.ChaincodeDefinition("channel-id", "name", fakeQueryExecutor)
					Expect(err).To(MatchError("could not get approved chaincode info from cache: cache-error"))
				})
			})

			Context("when the chaincode is not defined in the new lifecycle", func() {
				var (
					legacyChaincodeDefinition *ccprovider.ChaincodeData
				)

				BeforeEach(func() {
					delete(fakePublicState, "namespaces/fields/name/Sequence")
					legacyChaincodeDefinition = &ccprovider.ChaincodeData{
						Name:    "definition-name",
						Version: "definition-version",
					}

					fakeLegacyImpl.ChaincodeDefinitionReturns(legacyChaincodeDefinition, fmt.Errorf("fake-error"))
				})

				It("passes through the legacy implementation", func() {
					res, err := cei.ChaincodeDefinition("channel-id", "cc-name", fakeQueryExecutor)
					Expect(err).To(MatchError("fake-error"))
					Expect(res).To(Equal(legacyChaincodeDefinition))
					Expect(fakeLegacyImpl.ChaincodeDefinitionCallCount()).To(Equal(1))
					channelID, name, qe := fakeLegacyImpl.ChaincodeDefinitionArgsForCall(0)
					Expect(channelID).To(Equal("channel-id"))
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
			})

			It("returns the current definition", func() {
				res, err := cei.ChaincodeContainerInfo("channel-id", "name", fakeQueryExecutor)
				Expect(err).NotTo(HaveOccurred())
				Expect(res).To(Equal(&ccprovider.ChaincodeContainerInfo{
					Name:          "name",
					Version:       "version",
					Path:          "fake-path",
					Type:          "FAKE-TYPE",
					ContainerType: "DOCKER",
				}))
			})

			Context("when the definition does not exist in the new lifecycle", func() {
				BeforeEach(func() {
					legacyContainerInfo = &ccprovider.ChaincodeContainerInfo{
						Name:    "definition-name",
						Version: "definition-version",
					}

					fakeLegacyImpl.ChaincodeContainerInfoReturns(legacyContainerInfo, fmt.Errorf("fake-error"))
				})

				It("passes through the legacy implementation", func() {
					res, err := cei.ChaincodeContainerInfo("channel-id", "different-name", fakeQueryExecutor)
					Expect(err).To(MatchError("fake-error"))
					Expect(res).To(Equal(legacyContainerInfo))
					Expect(fakeLegacyImpl.ChaincodeContainerInfoCallCount()).To(Equal(1))
					channelID, name, qe := fakeLegacyImpl.ChaincodeContainerInfoArgsForCall(0)
					Expect(channelID).To(Equal("channel-id"))
					Expect(name).To(Equal("different-name"))
					Expect(qe).To(Equal(fakeQueryExecutor))
				})
			})

			Context("when the cache returns an error", func() {
				BeforeEach(func() {
					fakeCache.ChaincodeInfoReturns(nil, fmt.Errorf("cache-error"))
				})

				It("returns the wrapped error", func() {
					_, err := cei.ChaincodeContainerInfo("channel-id", "name", fakeQueryExecutor)
					Expect(err).To(MatchError("could not get approved chaincode info from cache: cache-error"))
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
