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
	"github.com/hyperledger/fabric/core/scc"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ChaincodeEndorsementInfoSource", func() {
	var (
		cei                     *lifecycle.ChaincodeEndorsementInfoSource
		resources               *lifecycle.Resources
		fakeLegacyImpl          *mock.LegacyLifecycle
		fakePublicState         MapLedgerShim
		fakeQueryExecutor       *mock.SimpleQueryExecutor
		fakeCache               *mock.ChaincodeInfoCache
		fakeChannelConfigSource *mock.ChannelConfigSource
		fakeChannelConfig       *mock.ChannelConfig
		fakeAppConfig           *mock.ApplicationConfig
		fakeCapabilities        *mock.ApplicationCapabilities
		testInfo                *lifecycle.LocalChaincodeInfo
		builtinSCCs             scc.BuiltinSCCs
	)

	BeforeEach(func() {
		fakeLegacyImpl = &mock.LegacyLifecycle{}
		fakeChannelConfigSource = &mock.ChannelConfigSource{}
		fakeChannelConfig = &mock.ChannelConfig{}
		fakeAppConfig = &mock.ApplicationConfig{}
		fakeCapabilities = &mock.ApplicationCapabilities{}
		fakeChannelConfigSource.GetStableChannelConfigReturns(fakeChannelConfig)
		fakeChannelConfig.ApplicationConfigReturns(fakeAppConfig, true)
		fakeAppConfig.CapabilitiesReturns(fakeCapabilities)
		fakeCapabilities.LifecycleV20Returns(true)

		resources = &lifecycle.Resources{
			Serializer:          &lifecycle.Serializer{},
			ChannelConfigSource: fakeChannelConfigSource,
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

		cei = &lifecycle.ChaincodeEndorsementInfoSource{
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

		Context("when running chaincode in devmode", func() {
			BeforeEach(func() {
				testInfo.InstallInfo = nil
				cei.UserRunsCC = true
			})

			It("set chaincode installInfo to {chaincodeName}:{chaincodeVersion}", func() {
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
						PackageID: "name:version",
					},
					Approved: true,
				}))
				Expect(fakeCache.ChaincodeInfoCallCount()).To(Equal(1))
				channelID, chaincodeName := fakeCache.ChaincodeInfoArgsForCall(0)
				Expect(channelID).To(Equal("channel-id"))
				Expect(chaincodeName).To(Equal("name"))
			})
		})
	})

	Describe("ChaincodeEndorsementInfo", func() {
		It("adapts the underlying lifecycle response", func() {
			def, err := cei.ChaincodeEndorsementInfo("channel-id", "name", fakeQueryExecutor)
			Expect(err).NotTo(HaveOccurred())
			Expect(def).To(Equal(&lifecycle.ChaincodeEndorsementInfo{
				Version:           "version",
				EndorsementPlugin: "endorsement-plugin",
				ChaincodeID:       "hash",
			}))
		})

		Context("when the chaincode is a builtin system chaincode", func() {
			BeforeEach(func() {
				builtinSCCs["test-syscc-name"] = struct{}{}
			})

			It("returns a static definition", func() {
				res, err := cei.ChaincodeEndorsementInfo("channel-id", "test-syscc-name", fakeQueryExecutor)
				Expect(err).NotTo(HaveOccurred())
				Expect(res).To(Equal(&lifecycle.ChaincodeEndorsementInfo{
					Version:           "syscc",
					ChaincodeID:       "test-syscc-name.syscc",
					EndorsementPlugin: "escc",
				}))
			})
		})

		Context("when the cache returns an error", func() {
			BeforeEach(func() {
				fakeCache.ChaincodeInfoReturns(nil, fmt.Errorf("cache-error"))
			})

			It("returns the wrapped error", func() {
				_, err := cei.ChaincodeEndorsementInfo("channel-id", "name", fakeQueryExecutor)
				Expect(err).To(MatchError("could not get approved chaincode info from cache: cache-error"))
			})
		})

		Context("when the chaincode is not defined in the new lifecycle", func() {
			BeforeEach(func() {
				delete(fakePublicState, "namespaces/fields/name/Sequence")
				fakeLegacyImpl.ChaincodeEndorsementInfoReturns(&lifecycle.ChaincodeEndorsementInfo{
					Version:           "legacy-version",
					EndorsementPlugin: "legacy-plugin",
					ChaincodeID:       "legacy-id",
				}, fmt.Errorf("fake-error"))
			})

			It("passes through the legacy implementation", func() {
				res, err := cei.ChaincodeEndorsementInfo("channel-id", "cc-name", fakeQueryExecutor)
				Expect(err).To(MatchError("fake-error"))
				Expect(res).To(Equal(&lifecycle.ChaincodeEndorsementInfo{
					Version:           "legacy-version",
					EndorsementPlugin: "legacy-plugin",
					ChaincodeID:       "legacy-id",
				}))
				Expect(fakeLegacyImpl.ChaincodeEndorsementInfoCallCount()).To(Equal(1))
				channelID, name, qe := fakeLegacyImpl.ChaincodeEndorsementInfoArgsForCall(0)
				Expect(channelID).To(Equal("channel-id"))
				Expect(name).To(Equal("cc-name"))
				Expect(qe).To(Equal(fakeQueryExecutor))
			})
		})
	})

	Context("when LifecycleV20 capability is not enabled", func() {
		var ccEndorsementInfo *lifecycle.ChaincodeEndorsementInfo

		BeforeEach(func() {
			ccEndorsementInfo = &lifecycle.ChaincodeEndorsementInfo{
				Version:           "legacy-version",
				EndorsementPlugin: "legacy-plugin",
				ChaincodeID:       "legacy-id",
			}
			fakeCapabilities.LifecycleV20Returns(false)
			fakeLegacyImpl.ChaincodeEndorsementInfoReturns(ccEndorsementInfo, nil)
		})

		It("returns the legacy chaincode info", func() {
			res, err := cei.ChaincodeEndorsementInfo("channel-id", "cc-name", fakeQueryExecutor)
			Expect(err).NotTo(HaveOccurred())
			Expect(res).To(Equal(ccEndorsementInfo))
			Expect(fakeLegacyImpl.ChaincodeEndorsementInfoCallCount()).To(Equal(1))
			channelID, name, qe := fakeLegacyImpl.ChaincodeEndorsementInfoArgsForCall(0)
			Expect(channelID).To(Equal("channel-id"))
			Expect(name).To(Equal("cc-name"))
			Expect(qe).To(Equal(fakeQueryExecutor))
		})
	})

	Context("when channel config is not found", func() {
		BeforeEach(func() {
			fakeChannelConfigSource.GetStableChannelConfigReturns(nil)
		})

		It("returns not get channel config error", func() {
			_, err := cei.ChaincodeEndorsementInfo("channel-id", "cc-name", fakeQueryExecutor)
			Expect(err).To(MatchError("could not get channel config for channel 'channel-id'"))
		})
	})

	Context("when application config is not found", func() {
		BeforeEach(func() {
			fakeChannelConfig.ApplicationConfigReturns(nil, false)
		})

		It("returns not get application config error", func() {
			_, err := cei.ChaincodeEndorsementInfo("channel-id", "cc-name", fakeQueryExecutor)
			Expect(err).To(MatchError("could not get application config for channel 'channel-id'"))
		})
	})
})
