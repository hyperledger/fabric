/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle_test

import (
	"fmt"

	lb "github.com/hyperledger/fabric-protos-go/peer/lifecycle"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle/mock"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/scc"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("ChaincodeEndorsementInfo", func() {
	var (
		cei               *lifecycle.ChaincodeEndorsementInfo
		resources         *lifecycle.Resources
		fakeLegacyImpl    *mock.LegacyLifecycle
		fakePublicState   MapLedgerShim
		fakeQueryExecutor *mock.SimpleQueryExecutor
		fakeCache         *mock.ChaincodeInfoCache
		testInfo          *lifecycle.LocalChaincodeInfo
		builtinSCCs       scc.BuiltinSCCs
	)

	BeforeEach(func() {
		fakeLegacyImpl = &mock.LegacyLifecycle{}

		resources = &lifecycle.Resources{
			Serializer: &lifecycle.Serializer{},
		}

		fakePublicState = MapLedgerShim(map[string][]byte{})
		fakeQueryExecutor = &mock.SimpleQueryExecutor{}
		fakeQueryExecutor.GetStateStub = func(namespace, key string) ([]byte, error) {
			return fakePublicState.GetState(key)
		}

		builtinSCCs = map[string]struct{}{}

		err := resources.Serializer.Serialize(lifecycle.NamespacesName,
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
					EndorsementPlugin: "endorsement-plugin",
				},
				ValidationInfo: &lb.ChaincodeValidationInfo{
					ValidationPlugin:    "validation-plugin",
					ValidationParameter: []byte("validation-parameter"),
				},
			},
			InstallInfo: &lifecycle.ChaincodeInstallInfo{
				Path:      "fake-path",
				Type:      "fake-type",
				PackageID: "hash",
			},
			Approved: true,
		}

		fakeCache = &mock.ChaincodeInfoCache{}
		fakeCache.ChaincodeInfoReturns(testInfo, nil)

		cei = &lifecycle.ChaincodeEndorsementInfo{
			LegacyImpl:  fakeLegacyImpl,
			Resources:   resources,
			Cache:       fakeCache,
			BuiltinSCCs: builtinSCCs,
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
						EndorsementPlugin: "endorsement-plugin",
					},
					ValidationInfo: &lb.ChaincodeValidationInfo{
						ValidationPlugin:    "validation-plugin",
						ValidationParameter: []byte("validation-parameter"),
					},
				},
				InstallInfo: &lifecycle.ChaincodeInstallInfo{
					Type:      "fake-type",
					Path:      "fake-path",
					PackageID: "hash",
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
				Expect(err).To(MatchError("could not get current sequence for chaincode 'name' on channel 'channel-id': could not get state for key namespaces/fields/name/Sequence: state-error"))
			})
		})

		Context("when the query executor is nil", func() {
			It("uses the dummy query executor and returns an error", func() {
				_, _, err := cei.CachedChaincodeInfo("", "name", nil)
				Expect(err).To(MatchError("could not get current sequence for chaincode 'name' on channel '': could not get state for key namespaces/fields/name/Sequence: invalid channel-less operation"))
			})
		})
	})

	Describe("ChaincodeDefinition", func() {
		It("adapts the underlying lifecycle response", func() {
			def, err := cei.ChaincodeDefinition("channel-id", "name", fakeQueryExecutor)
			Expect(err).NotTo(HaveOccurred())
			Expect(def).To(Equal(&lifecycle.LegacyDefinition{
				Version:           "version",
				EndorsementPlugin: "endorsement-plugin",
				ChaincodeIDField:  "hash",
			}))
		})

		Context("when the chaincode is a builtin system chaincode", func() {
			BeforeEach(func() {
				builtinSCCs["test-syscc-name"] = struct{}{}
			})

			It("returns a static definition", func() {
				res, err := cei.ChaincodeDefinition("channel-id", "test-syscc-name", fakeQueryExecutor)
				Expect(err).NotTo(HaveOccurred())
				Expect(res).To(Equal(&lifecycle.LegacyDefinition{
					Version:           "syscc",
					EndorsementPlugin: "escc",
					ChaincodeIDField:  "test-syscc-name.syscc",
				}))
			})
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
})

var _ = Describe("LegacyDefinition", func() {
	var (
		ld *lifecycle.LegacyDefinition
	)

	BeforeEach(func() {
		ld = &lifecycle.LegacyDefinition{
			Version:           "version",
			EndorsementPlugin: "endorsement-plugin",
			RequiresInitField: true,
			ChaincodeIDField:  "chaincode-id",
		}
	})

	Describe("CCVersion", func() {
		It("returns the version", func() {
			Expect(ld.CCVersion()).To(Equal("version"))
		})
	})

	Describe("ChaincodeID", func() {
		It("returns the version", func() {
			Expect(ld.ChaincodeID()).To(Equal("chaincode-id"))
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
})
