/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle_test

import (
	"fmt"

	"github.com/hyperledger/fabric-protos-go/ledger/queryresult"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset/kvrwset"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	lb "github.com/hyperledger/fabric-protos-go/peer/lifecycle"
	"github.com/hyperledger/fabric/common/chaincode"
	commonledger "github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle/mock"
	"github.com/hyperledger/fabric/core/chaincode/persistence"
	"github.com/hyperledger/fabric/core/container"
	"github.com/hyperledger/fabric/core/container/externalbuilder"
	"github.com/hyperledger/fabric/core/ledger"
	ledgermock "github.com/hyperledger/fabric/core/ledger/mock"
	"github.com/hyperledger/fabric/protoutil"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Cache", func() {
	var (
		c                       *lifecycle.Cache
		resources               *lifecycle.Resources
		fakeCCStore             *mock.ChaincodeStore
		fakeParser              *mock.PackageParser
		fakeChannelConfigSource *mock.ChannelConfigSource
		fakeChannelConfig       *mock.ChannelConfig
		fakeApplicationConfig   *mock.ApplicationConfig
		fakeCapabilities        *mock.ApplicationCapabilities
		fakePolicyManager       *mock.PolicyManager
		fakeMetadataHandler     *mock.MetadataHandler
		channelCache            *lifecycle.ChannelCache
		localChaincodes         map[string]*lifecycle.LocalChaincode
		fakePublicState         MapLedgerShim
		fakePrivateState        MapLedgerShim
		fakeQueryExecutor       *mock.SimpleQueryExecutor
		chaincodeCustodian      *lifecycle.ChaincodeCustodian
	)

	BeforeEach(func() {
		fakeCCStore = &mock.ChaincodeStore{}
		fakeParser = &mock.PackageParser{}
		fakeChannelConfigSource = &mock.ChannelConfigSource{}
		fakeChannelConfig = &mock.ChannelConfig{}
		fakeChannelConfigSource.GetStableChannelConfigReturns(fakeChannelConfig)
		fakeApplicationConfig = &mock.ApplicationConfig{}
		fakeChannelConfig.ApplicationConfigReturns(fakeApplicationConfig, true)
		fakeCapabilities = &mock.ApplicationCapabilities{}
		fakeCapabilities.LifecycleV20Returns(true)
		fakeApplicationConfig.CapabilitiesReturns(fakeCapabilities)
		fakePolicyManager = &mock.PolicyManager{}
		fakePolicyManager.GetPolicyReturns(nil, true)
		fakeChannelConfig.PolicyManagerReturns(fakePolicyManager)
		resources = &lifecycle.Resources{
			PackageParser:       fakeParser,
			ChaincodeStore:      fakeCCStore,
			ChannelConfigSource: fakeChannelConfigSource,
			Serializer:          &lifecycle.Serializer{},
		}

		fakeCCStore.ListInstalledChaincodesReturns([]chaincode.InstalledChaincode{
			{
				Hash:      []byte("hash"),
				PackageID: "packageID",
			},
		}, nil)

		fakeCCStore.LoadReturns([]byte("package-bytes"), nil)

		fakeParser.ParseReturns(&persistence.ChaincodePackage{
			Metadata: &persistence.ChaincodePackageMetadata{
				Path: "cc-path",
				Type: "cc-type",
			},
			CodePackage: []byte("code-package"),
			DBArtifacts: []byte("db-artifacts"),
		}, nil)

		fakeMetadataHandler = &mock.MetadataHandler{}

		chaincodeCustodian = lifecycle.NewChaincodeCustodian()

		var err error
		c = lifecycle.NewCache(resources, "my-mspid", fakeMetadataHandler, chaincodeCustodian, &externalbuilder.MetadataProvider{})
		Expect(err).NotTo(HaveOccurred())

		channelCache = &lifecycle.ChannelCache{
			Chaincodes: map[string]*lifecycle.CachedChaincodeDefinition{
				"chaincode-name": {
					Definition: &lifecycle.ChaincodeDefinition{
						Sequence: 3,
						EndorsementInfo: &lb.ChaincodeEndorsementInfo{
							Version: "chaincode-version",
						},
						ValidationInfo: &lb.ChaincodeValidationInfo{
							ValidationParameter: []byte("validation-parameter"),
						},
						Collections: &pb.CollectionConfigPackage{},
					},
					Approved: true,
					Hashes: []string{
						string(util.ComputeSHA256([]byte("namespaces/metadata/chaincode-name#3"))),
						string(util.ComputeSHA256([]byte("namespaces/fields/chaincode-name#3/Sequence"))),
						string(util.ComputeSHA256([]byte("namespaces/fields/chaincode-name#3/EndorsementInfo"))),
						string(util.ComputeSHA256([]byte("namespaces/fields/chaincode-name#3/ValidationInfo"))),
						string(util.ComputeSHA256([]byte("namespaces/fields/chaincode-name#3/Collections"))),
						string(util.ComputeSHA256([]byte("chaincode-sources/fields/chaincode-name#3/PackageID"))),
					},
				},
			},
			InterestingHashes: map[string]string{
				string(util.ComputeSHA256([]byte("namespaces/metadata/chaincode-name#3"))):                "chaincode-name",
				string(util.ComputeSHA256([]byte("namespaces/fields/chaincode-name#3/Sequence"))):         "chaincode-name",
				string(util.ComputeSHA256([]byte("namespaces/fields/chaincode-name#3/EndorsementInfo"))):  "chaincode-name",
				string(util.ComputeSHA256([]byte("namespaces/fields/chaincode-name#3/ValidationInfo"))):   "chaincode-name",
				string(util.ComputeSHA256([]byte("namespaces/fields/chaincode-name#3/Collections"))):      "chaincode-name",
				string(util.ComputeSHA256([]byte("chaincode-sources/fields/chaincode-name#3/PackageID"))): "chaincode-name",
			},
		}

		localChaincodes = map[string]*lifecycle.LocalChaincode{
			string(util.ComputeSHA256(protoutil.MarshalOrPanic(&lb.StateData{
				Type: &lb.StateData_String_{String_: "packageID"},
			}))): {
				References: map[string]map[string]*lifecycle.CachedChaincodeDefinition{
					"channel-id": {
						"chaincode-name": channelCache.Chaincodes["chaincode-name"],
					},
					"another-channel-id": {
						"chaincode-name": channelCache.Chaincodes["chaincode-name"],
					},
				},
				Info: &lifecycle.ChaincodeInstallInfo{
					Label:     "chaincode-label",
					PackageID: "packageID",
				},
			},
			string(util.ComputeSHA256(protoutil.MarshalOrPanic(&lb.StateData{
				Type: &lb.StateData_String_{String_: "notinstalled-packageID"},
			}))): {
				References: map[string]map[string]*lifecycle.CachedChaincodeDefinition{
					"channel-id": {
						"chaincode-name": channelCache.Chaincodes["chaincode-name"],
					},
				},
			},
		}

		lifecycle.SetChaincodeMap(c, "channel-id", channelCache)
		lifecycle.SetLocalChaincodesMap(c, localChaincodes)

		fakePublicState = MapLedgerShim(map[string][]byte{})
		fakePrivateState = MapLedgerShim(map[string][]byte{})
		fakeQueryExecutor = &mock.SimpleQueryExecutor{}
		fakeQueryExecutor.GetStateStub = func(namespace, key string) ([]byte, error) {
			return fakePublicState.GetState(key)
		}

		fakeQueryExecutor.GetStateRangeScanIteratorStub = func(namespace, begin, end string) (commonledger.ResultsIterator, error) {
			fakeResultsIterator := &mock.ResultsIterator{}
			i := 0
			for key, value := range fakePublicState {
				if key >= begin && key < end {
					fakeResultsIterator.NextReturnsOnCall(i, &queryresult.KV{
						Key:   key,
						Value: value,
					}, nil)
					i++
				}
			}
			return fakeResultsIterator, nil
		}

		fakeQueryExecutor.GetPrivateDataHashStub = func(namespace, collection, key string) ([]byte, error) {
			return fakePrivateState.GetStateHash(key)
		}
	})

	AfterEach(func() {
		chaincodeCustodian.Close()
	})

	Describe("ChaincodeInfo", func() {
		BeforeEach(func() {
			channelCache.Chaincodes["chaincode-name"].InstallInfo = &lifecycle.ChaincodeInstallInfo{
				Type:      "cc-type",
				Path:      "cc-path",
				PackageID: "hash",
			}
		})

		It("returns the cached chaincode definition", func() {
			localInfo, err := c.ChaincodeInfo("channel-id", "chaincode-name")
			Expect(err).NotTo(HaveOccurred())
			Expect(localInfo).To(Equal(&lifecycle.LocalChaincodeInfo{
				Definition: &lifecycle.ChaincodeDefinition{
					Sequence: 3,
					EndorsementInfo: &lb.ChaincodeEndorsementInfo{
						Version: "chaincode-version",
					},
					ValidationInfo: &lb.ChaincodeValidationInfo{
						ValidationParameter: []byte("validation-parameter"),
					},
					Collections: &pb.CollectionConfigPackage{},
				},
				InstallInfo: &lifecycle.ChaincodeInstallInfo{
					Type:      "cc-type",
					Path:      "cc-path",
					PackageID: "hash",
				},
				Approved: true,
			}))
		})

		Context("when the chaincode name is not in the cache", func() {
			It("returns an error", func() {
				_, err := c.ChaincodeInfo("channel-id", "missing-name")
				Expect(err).To(MatchError("unknown chaincode 'missing-name' for channel 'channel-id'"))
			})
		})

		Context("when the channel does not exist", func() {
			It("returns an error", func() {
				_, err := c.ChaincodeInfo("missing-channel-id", "chaincode-name")
				Expect(err).To(MatchError("unknown channel 'missing-channel-id'"))
			})
		})

		Context("when the chaincode name is the _lifecycle system chaincode", func() {
			It("returns info about _lifecycle", func() {
				localInfo, err := c.ChaincodeInfo("channel-id", "_lifecycle")
				Expect(err).NotTo(HaveOccurred())
				Expect(localInfo).To(Equal(&lifecycle.LocalChaincodeInfo{
					Definition: &lifecycle.ChaincodeDefinition{
						Sequence: 1,
						ValidationInfo: &lb.ChaincodeValidationInfo{
							ValidationParameter: lifecycle.LifecycleDefaultEndorsementPolicyBytes,
						},
					},
					InstallInfo: &lifecycle.ChaincodeInstallInfo{},
					Approved:    true,
				}))
			})
		})

		Context("when the application config cannot be found", func() {
			BeforeEach(func() {
				fakeChannelConfig.ApplicationConfigReturns(nil, false)
			})

			It("returns an error", func() {
				_, err := c.ChaincodeInfo("channel-id", "_lifecycle")
				Expect(err).To(MatchError("application config does not exist for channel 'channel-id'"))
			})
		})

		Context("when the application config V2_0 capabilities are not enabled", func() {
			BeforeEach(func() {
				fakeCapabilities.LifecycleV20Returns(false)
			})

			It("returns an error", func() {
				_, err := c.ChaincodeInfo("channel-id", "_lifecycle")
				Expect(err).To(MatchError("cannot use _lifecycle without V2_0 application capabilities enabled for channel 'channel-id'"))
			})
		})
	})

	Describe("ListInstalledChaincodes", func() {
		It("returns the installed chaincodes", func() {
			installedChaincodes := c.ListInstalledChaincodes()
			Expect(installedChaincodes).To(Equal([]*chaincode.InstalledChaincode{
				{
					Label:     "chaincode-label",
					PackageID: "packageID",
					References: map[string][]*chaincode.Metadata{
						"channel-id": {
							&chaincode.Metadata{
								Name:    "chaincode-name",
								Version: "chaincode-version",
							},
						},
						"another-channel-id": {
							&chaincode.Metadata{
								Name:    "chaincode-name",
								Version: "chaincode-version",
							},
						},
					},
				},
			}))
		})
	})

	Describe("GetInstalledChaincode", func() {
		It("returns the requested installed chaincode", func() {
			installedChaincode, err := c.GetInstalledChaincode("packageID")
			Expect(installedChaincode).To(Equal(&chaincode.InstalledChaincode{
				Label:     "chaincode-label",
				PackageID: "packageID",
				References: map[string][]*chaincode.Metadata{
					"channel-id": {
						&chaincode.Metadata{
							Name:    "chaincode-name",
							Version: "chaincode-version",
						},
					},
					"another-channel-id": {
						&chaincode.Metadata{
							Name:    "chaincode-name",
							Version: "chaincode-version",
						},
					},
				},
			}))
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when the chaincode is not installed", func() {
			It("returns an error", func() {
				installedChaincode, err := c.GetInstalledChaincode("notinstalled-packageID")
				Expect(installedChaincode).To(BeNil())
				Expect(err).To(MatchError("could not find chaincode with package id 'notinstalled-packageID'"))
			})
		})
	})

	Describe("InitializeLocalChaincodes", func() {
		It("loads the already installed chaincodes into the cache", func() {
			Expect(channelCache.Chaincodes["chaincode-name"].InstallInfo).To(BeNil())
			err := c.InitializeLocalChaincodes()
			Expect(err).NotTo(HaveOccurred())
			Expect(channelCache.Chaincodes["chaincode-name"].InstallInfo).To(Equal(&lifecycle.ChaincodeInstallInfo{
				Type:      "cc-type",
				Path:      "cc-path",
				PackageID: "packageID",
			}))
		})

		Context("when the installed chaincode does not match any current definition", func() {
			BeforeEach(func() {
				fakeCCStore.ListInstalledChaincodesReturns([]chaincode.InstalledChaincode{
					{
						Hash: []byte("other-hash"),
					},
				}, nil)
			})

			It("does not update the chaincode", func() {
				err := c.InitializeLocalChaincodes()
				Expect(err).NotTo(HaveOccurred())
				Expect(channelCache.Chaincodes["chaincode-name"].InstallInfo).To(BeNil())
			})
		})

		Context("when the chaincodes cannot be listed", func() {
			BeforeEach(func() {
				fakeCCStore.ListInstalledChaincodesReturns(nil, fmt.Errorf("list-error"))
			})

			It("wraps and returns the error", func() {
				err := c.InitializeLocalChaincodes()
				Expect(err).To(MatchError("could not list installed chaincodes: list-error"))
			})
		})

		Context("when the chaincodes cannot be loaded", func() {
			BeforeEach(func() {
				fakeCCStore.LoadReturns(nil, fmt.Errorf("load-error"))
			})

			It("wraps and returns the error", func() {
				err := c.InitializeLocalChaincodes()
				Expect(err.Error()).To(ContainSubstring("could not load chaincode with package ID 'packageID'"))
			})
		})

		Context("when the chaincode package cannot be parsed", func() {
			BeforeEach(func() {
				fakeParser.ParseReturns(nil, fmt.Errorf("parse-error"))
			})

			It("wraps and returns the error", func() {
				err := c.InitializeLocalChaincodes()
				Expect(err.Error()).To(ContainSubstring("could not parse chaincode with package ID 'packageID'"))
			})
		})
	})

	Describe("Initialize", func() {
		BeforeEach(func() {
			err := resources.Serializer.Serialize(lifecycle.NamespacesName, "chaincode-name", &lifecycle.ChaincodeDefinition{
				Sequence: 7,
			}, fakePublicState)
			Expect(err).NotTo(HaveOccurred())

			err = resources.Serializer.Serialize(lifecycle.NamespacesName, "chaincode-name#7", &lifecycle.ChaincodeParameters{}, fakePrivateState)
			Expect(err).NotTo(HaveOccurred())

			err = resources.Serializer.Serialize(lifecycle.ChaincodeSourcesName, "chaincode-name#7", &lifecycle.ChaincodeLocalPackage{PackageID: "hash"}, fakePrivateState)
			Expect(err).NotTo(HaveOccurred())
		})

		It("sets the definitions from the state", func() {
			err := c.Initialize("channel-id", fakeQueryExecutor)
			Expect(err).NotTo(HaveOccurred())
			Expect(channelCache.Chaincodes["chaincode-name"].Definition.Sequence).To(Equal(int64(7)))
			Expect(channelCache.Chaincodes["chaincode-name"].Approved).To(BeTrue())
			Expect(channelCache.Chaincodes["chaincode-name"].Hashes).To(Equal([]string{
				string(util.ComputeSHA256([]byte("namespaces/metadata/chaincode-name#7"))),
				string(util.ComputeSHA256([]byte("namespaces/fields/chaincode-name#7/EndorsementInfo"))),
				string(util.ComputeSHA256([]byte("namespaces/fields/chaincode-name#7/ValidationInfo"))),
				string(util.ComputeSHA256([]byte("namespaces/fields/chaincode-name#7/Collections"))),
				string(util.ComputeSHA256([]byte("chaincode-sources/fields/chaincode-name#7/PackageID"))),
			}))
			for _, hash := range channelCache.Chaincodes["chaincode-name"].Hashes {
				Expect(channelCache.InterestingHashes[hash]).To(Equal("chaincode-name"))
			}
		})

		Context("when the chaincode is not installed", func() {
			BeforeEach(func() {
				err := resources.Serializer.Serialize(lifecycle.NamespacesName, "chaincode-name", &lifecycle.ChaincodeDefinition{
					Sequence: 7,
				}, fakePublicState)
				Expect(err).NotTo(HaveOccurred())

				err = resources.Serializer.Serialize(lifecycle.NamespacesName, "chaincode-name#7", &lifecycle.ChaincodeParameters{}, fakePrivateState)
				Expect(err).NotTo(HaveOccurred())

				err = resources.Serializer.Serialize(lifecycle.ChaincodeSourcesName, "chaincode-name#7", &lifecycle.ChaincodeLocalPackage{
					PackageID: "different-hash",
				}, fakePrivateState)
				Expect(err).NotTo(HaveOccurred())
			})

			It("does not have install info set", func() {
				err := c.Initialize("channel-id", fakeQueryExecutor)
				Expect(err).NotTo(HaveOccurred())
				Expect(channelCache.Chaincodes["chaincode-name"].InstallInfo).To(BeNil())
			})

			It("does not attempt to launch", func() {
				err := c.Initialize("channel-id", fakeQueryExecutor)
				Expect(err).NotTo(HaveOccurred())

				fakeLauncher := &mock.ChaincodeLauncher{}
				go chaincodeCustodian.Work(nil, nil, fakeLauncher)
				Consistently(fakeLauncher.LaunchCallCount).Should(Equal(0))
			})

			Context("when the chaincode is installed afterwards", func() {
				It("gets its install info set and attempts to launch", func() {
					err := c.Initialize("channel-id", fakeQueryExecutor)
					Expect(err).NotTo(HaveOccurred())
					c.HandleChaincodeInstalled(&persistence.ChaincodePackageMetadata{
						Type: "some-type",
						Path: "some-path",
					}, "different-hash")
					Expect(channelCache.Chaincodes["chaincode-name"].InstallInfo).To(Equal(&lifecycle.ChaincodeInstallInfo{
						Type:      "some-type",
						Path:      "some-path",
						PackageID: "different-hash",
					}))

					fakeLauncher := &mock.ChaincodeLauncher{}
					go chaincodeCustodian.Work(nil, nil, fakeLauncher)
					Eventually(fakeLauncher.LaunchCallCount).Should(Equal(1))
				})
			})
		})

		Context("when the namespaces query fails", func() {
			BeforeEach(func() {
				fakeQueryExecutor.GetStateRangeScanIteratorReturns(nil, fmt.Errorf("range-error"))
			})

			It("wraps and returns the error", func() {
				err := c.Initialize("channel-id", fakeQueryExecutor)
				Expect(err).To(MatchError("could not query namespace metadata: could not get state range for namespace namespaces: could not get state iterator: range-error"))
			})
		})

		Context("when the namespace is not of type chaincode", func() {
			BeforeEach(func() {
				err := resources.Serializer.Serialize(lifecycle.NamespacesName, "chaincode-name", &lifecycle.ChaincodeParameters{}, fakePublicState)
				Expect(err).NotTo(HaveOccurred())
			})

			It("ignores the definition", func() {
				err := c.Initialize("channel-id", fakeQueryExecutor)
				Expect(err).NotTo(HaveOccurred())
				Expect(fakeQueryExecutor.GetStateCallCount()).To(Equal(0))
			})
		})

		Context("when the definition is not in the new state", func() {
			BeforeEach(func() {
				fakeQueryExecutor.GetStateReturns(nil, nil)
			})

			It("deletes the cached definition", func() {
				err := c.Initialize("channel-id", fakeQueryExecutor)
				Expect(err).NotTo(HaveOccurred())
				Expect(channelCache.Chaincodes["chaincode-name"]).To(BeNil())
			})
		})

		Context("when the org's definition differs", func() {
			BeforeEach(func() {
				err := resources.Serializer.Serialize(lifecycle.NamespacesName, "chaincode-name#7", &lifecycle.ChaincodeParameters{
					EndorsementInfo: &lb.ChaincodeEndorsementInfo{
						EndorsementPlugin: "different",
					},
				}, fakePrivateState)
				Expect(err).NotTo(HaveOccurred())
			})

			It("does not mark the definition approved", func() {
				err := c.Initialize("channel-id", fakeQueryExecutor)
				Expect(err).NotTo(HaveOccurred())
				Expect(channelCache.Chaincodes["chaincode-name"].Approved).To(BeFalse())
			})

			Context("when the org has no definition", func() {
				BeforeEach(func() {
					fakeQueryExecutor.GetPrivateDataHashReturns(nil, nil)
				})

				It("does not mark the definition approved", func() {
					err := c.Initialize("channel-id", fakeQueryExecutor)
					Expect(err).NotTo(HaveOccurred())
					Expect(channelCache.Chaincodes["chaincode-name"].Approved).To(BeFalse())
				})
			})
		})

		Context("when the chaincode is already installed", func() {
			BeforeEach(func() {
				c.HandleChaincodeInstalled(&persistence.ChaincodePackageMetadata{
					Type: "cc-type",
					Path: "cc-path",
				}, "hash")
			})

			It("updates the install info", func() {
				err := c.Initialize("channel-id", fakeQueryExecutor)
				Expect(err).NotTo(HaveOccurred())
				Expect(channelCache.Chaincodes["chaincode-name"].InstallInfo).To(Equal(&lifecycle.ChaincodeInstallInfo{
					Type:      "cc-type",
					Path:      "cc-path",
					PackageID: "hash",
				}))
			})

			It("tells the custodian first to build, then to launch it", func() {
				err := c.Initialize("channel-id", fakeQueryExecutor)
				Expect(err).NotTo(HaveOccurred())

				fakeLauncher := &mock.ChaincodeLauncher{}
				fakeBuilder := &mock.ChaincodeBuilder{}
				buildRegistry := &container.BuildRegistry{}
				go chaincodeCustodian.Work(buildRegistry, fakeBuilder, fakeLauncher)
				Eventually(fakeBuilder.BuildCallCount).Should(Equal(1))
				Eventually(fakeLauncher.LaunchCallCount).Should(Equal(1))
			})
		})

		Context("when the update is for an unknown channel", func() {
			It("creates the underlying map", func() {
				Expect(lifecycle.GetChaincodeMap(c, "new-channel")).To(BeNil())
				err := c.Initialize("new-channel", fakeQueryExecutor)
				Expect(err).NotTo(HaveOccurred())
				Expect(lifecycle.GetChaincodeMap(c, "new-channel")).NotTo(BeNil())
			})
		})

		Context("when the state returns an error", func() {
			BeforeEach(func() {
				fakeQueryExecutor.GetStateReturns(nil, fmt.Errorf("get-state-error"))
			})

			It("wraps and returns the error", func() {
				err := c.Initialize("channel-id", fakeQueryExecutor)
				Expect(err).To(MatchError("could not get chaincode definition for 'chaincode-name' on channel 'channel-id': could not deserialize metadata for chaincode chaincode-name: could not query metadata for namespace namespaces/chaincode-name: get-state-error"))
			})
		})

		Context("when the private state returns an error", func() {
			BeforeEach(func() {
				fakeQueryExecutor.GetPrivateDataHashReturns(nil, fmt.Errorf("private-data-error"))
			})

			It("wraps and returns the error", func() {
				err := c.Initialize("channel-id", fakeQueryExecutor)
				Expect(err).To(MatchError("could not check opaque org state for chaincode source hash for 'chaincode-name' on channel 'channel-id': private-data-error"))
			})

			Context("when the private state returns an error for the chaincode source metadata", func() {
				BeforeEach(func() {
					fakeQueryExecutor.GetPrivateDataHashStub = func(channel, collection, key string) ([]byte, error) {
						if key != "chaincode-sources/metadata/chaincode-name#7" {
							return fakePrivateState.GetStateHash(key)
						}
						return nil, fmt.Errorf("private-data-error")
					}
				})

				It("wraps and returns the error", func() {
					err := c.Initialize("channel-id", fakeQueryExecutor)
					Expect(err).To(MatchError("could not check opaque org state for chaincode source for 'chaincode-name' on channel 'channel-id': could not get state hash for metadata key chaincode-sources/metadata/chaincode-name#7: private-data-error"))
				})
			})

			Context("when the private state returns an error for the chaincode source", func() {
				BeforeEach(func() {
					fakeQueryExecutor.GetPrivateDataHashStub = func(channel, collection, key string) ([]byte, error) {
						if key != "chaincode-sources/fields/chaincode-name#7/PackageID" {
							return fakePrivateState.GetStateHash(key)
						}
						return nil, fmt.Errorf("private-data-error")
					}
				})

				It("wraps and returns the error", func() {
					err := c.Initialize("channel-id", fakeQueryExecutor)
					Expect(err).To(MatchError("could not check opaque org state for chaincode source hash for 'chaincode-name' on channel 'channel-id': private-data-error"))
				})
			})
		})

		Context("when the chaincode-source is not a local package", func() {
			BeforeEach(func() {
				fakePrivateState["chaincode-sources/metadata/chaincode-name#7"] = []byte("garbage")
			})

			It("does not set the install info", func() {
				err := c.Initialize("channel-id", fakeQueryExecutor)
				Expect(err).NotTo(HaveOccurred())
				Expect(channelCache.Chaincodes["chaincode-name"].InstallInfo).To(BeNil())
			})
		})
	})

	Describe("InitializeMetadata", func() {
		BeforeEach(func() {
			channelCache = &lifecycle.ChannelCache{
				Chaincodes: map[string]*lifecycle.CachedChaincodeDefinition{
					"installedAndApprovedCC": {
						Definition: &lifecycle.ChaincodeDefinition{
							Sequence: 3,
							EndorsementInfo: &lb.ChaincodeEndorsementInfo{
								Version: "chaincode-version",
							},
							ValidationInfo: &lb.ChaincodeValidationInfo{
								ValidationParameter: []byte("validation-parameter"),
							},
							Collections: &pb.CollectionConfigPackage{},
						},
						Approved: true,
					},
					"idontapprove": {
						Definition: &lifecycle.ChaincodeDefinition{
							Sequence: 3,
							EndorsementInfo: &lb.ChaincodeEndorsementInfo{
								Version: "chaincode-version",
							},
							ValidationInfo: &lb.ChaincodeValidationInfo{
								ValidationParameter: []byte("validation-parameter"),
							},
							Collections: &pb.CollectionConfigPackage{},
						},
						Approved: false,
					},
					"ididntinstall": {
						Definition: &lifecycle.ChaincodeDefinition{
							Sequence: 3,
							EndorsementInfo: &lb.ChaincodeEndorsementInfo{
								Version: "chaincode-version",
							},
							ValidationInfo: &lb.ChaincodeValidationInfo{
								ValidationParameter: []byte("validation-parameter"),
							},
							Collections: &pb.CollectionConfigPackage{},
						},
						Approved: true,
					},
				},
			}

			localChaincodes = map[string]*lifecycle.LocalChaincode{
				string(util.ComputeSHA256(protoutil.MarshalOrPanic(&lb.StateData{
					Type: &lb.StateData_String_{String_: "packageID"},
				}))): {
					References: map[string]map[string]*lifecycle.CachedChaincodeDefinition{
						"channel-id": {
							"installedAndApprovedCC": channelCache.Chaincodes["installedAndApprovedCC"],
							"idontapprove":           channelCache.Chaincodes["idontapprove"],
						},
					},
				},
			}

			lifecycle.SetChaincodeMap(c, "channel-id", channelCache)
			lifecycle.SetLocalChaincodesMap(c, localChaincodes)
			err := c.InitializeLocalChaincodes()
			Expect(err).NotTo(HaveOccurred())
		})

		It("initializes the chaincode metadata from the cache", func() {
			c.InitializeMetadata("channel-id")
			channel, metadata := fakeMetadataHandler.InitializeMetadataArgsForCall(0)
			Expect(channel).To(Equal("channel-id"))
			Expect(metadata).To(ConsistOf(
				chaincode.Metadata{
					Name:              "installedAndApprovedCC",
					Version:           "3",
					Policy:            []byte("validation-parameter"),
					CollectionsConfig: &pb.CollectionConfigPackage{},
					Approved:          true,
					Installed:         true,
				},
				chaincode.Metadata{
					Name:              "ididntinstall",
					Version:           "3",
					Policy:            []byte("validation-parameter"),
					CollectionsConfig: &pb.CollectionConfigPackage{},
					Approved:          true,
					Installed:         false,
				},
				chaincode.Metadata{
					Name:              "idontapprove",
					Version:           "3",
					Policy:            []byte("validation-parameter"),
					CollectionsConfig: &pb.CollectionConfigPackage{},
					Approved:          false,
					Installed:         true,
				},
				chaincode.Metadata{
					Name:              "_lifecycle",
					Version:           "1",
					Policy:            lifecycle.LifecycleDefaultEndorsementPolicyBytes,
					CollectionsConfig: nil,
					Approved:          true,
					Installed:         true,
				},
			))
		})

		Context("when the channel is unknown", func() {
			It("returns without initializing metadata", func() {
				c.InitializeMetadata("slurm")
				Expect(fakeMetadataHandler.InitializeMetadataCallCount()).To(Equal(0))
			})
		})
	})

	Describe("RegisterListener", func() {
		var fakeListener *ledgermock.ChaincodeLifecycleEventListener
		BeforeEach(func() {
			fakeListener = &ledgermock.ChaincodeLifecycleEventListener{}
			channelCache = &lifecycle.ChannelCache{
				Chaincodes: map[string]*lifecycle.CachedChaincodeDefinition{
					"definedInstalledAndApprovedCC": {
						Definition: &lifecycle.ChaincodeDefinition{
							Sequence: 3,
							EndorsementInfo: &lb.ChaincodeEndorsementInfo{
								Version: "chaincode-version",
							},
							ValidationInfo: &lb.ChaincodeValidationInfo{
								ValidationParameter: []byte("validation-parameter"),
							},
							Collections: &pb.CollectionConfigPackage{},
						},
						Approved: true,
					},
					"anotherDefinedInstalledAndApprovedCC": {
						Definition: &lifecycle.ChaincodeDefinition{
							Sequence: 3,
							EndorsementInfo: &lb.ChaincodeEndorsementInfo{
								Version: "chaincode-version",
							},
							ValidationInfo: &lb.ChaincodeValidationInfo{
								ValidationParameter: []byte("validation-parameter"),
							},
							Collections: &pb.CollectionConfigPackage{},
						},
						Approved: true,
					},
					"idontapprove": {
						Definition: &lifecycle.ChaincodeDefinition{
							Sequence: 3,
							EndorsementInfo: &lb.ChaincodeEndorsementInfo{
								Version: "chaincode-version",
							},
							ValidationInfo: &lb.ChaincodeValidationInfo{
								ValidationParameter: []byte("validation-parameter"),
							},
							Collections: &pb.CollectionConfigPackage{},
						},
						Approved: false,
					},
					"ididntinstall": {
						Definition: &lifecycle.ChaincodeDefinition{
							Sequence: 3,
							EndorsementInfo: &lb.ChaincodeEndorsementInfo{
								Version: "chaincode-version",
							},
							ValidationInfo: &lb.ChaincodeValidationInfo{
								ValidationParameter: []byte("validation-parameter"),
							},
							Collections: &pb.CollectionConfigPackage{},
						},
						Approved: true,
					},
				},
			}

			localChaincodes = map[string]*lifecycle.LocalChaincode{
				string(util.ComputeSHA256(protoutil.MarshalOrPanic(&lb.StateData{
					Type: &lb.StateData_String_{String_: "packageID"},
				}))): {
					References: map[string]map[string]*lifecycle.CachedChaincodeDefinition{
						"channel-id": {
							"definedInstalledAndApprovedCC": channelCache.Chaincodes["definedInstalledAndApprovedCC"],
							"idontapprove":                  channelCache.Chaincodes["idontapprove"],
						},
					},
				},

				string(util.ComputeSHA256(protoutil.MarshalOrPanic(&lb.StateData{
					Type: &lb.StateData_String_{String_: "anotherPackageID"},
				}))): {
					References: map[string]map[string]*lifecycle.CachedChaincodeDefinition{
						"channel-id": {
							"anotherDefinedInstalledAndApprovedCC": channelCache.Chaincodes["anotherDefinedInstalledAndApprovedCC"],
						},
					},
				},
			}

			fakeCCStore.ListInstalledChaincodesReturns([]chaincode.InstalledChaincode{
				{
					Hash:      []byte("hash"),
					PackageID: "packageID",
				},
				{
					Hash:      []byte("hash"),
					PackageID: "anotherPackageID",
				},
			}, nil)

			lifecycle.SetChaincodeMap(c, "channel-id", channelCache)
			lifecycle.SetLocalChaincodesMap(c, localChaincodes)
			err := c.InitializeLocalChaincodes()
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when channel does not exist", func() {
			It("returns error", func() {
				err := c.RegisterListener("non-existing-channel", fakeListener, true)
				Expect(err).To(MatchError("unknown channel 'non-existing-channel'"))
			})
		})

		Context("when listener wants existing chaincode info", func() {
			It("calls back the listener with only invocable chaincodes", func() {
				err := c.RegisterListener("channel-id", fakeListener, true)
				Expect(err).NotTo(HaveOccurred())
				Expect(fakeListener.HandleChaincodeDeployCallCount()).To(Equal(2))
				Expect(fakeListener.ChaincodeDeployDoneCallCount()).To(Equal(2))
				ccdef0, dbArtifacts0 := fakeListener.HandleChaincodeDeployArgsForCall(0)
				ccdef1, dbArtifacts1 := fakeListener.HandleChaincodeDeployArgsForCall(1)
				Expect(
					[]*ledger.ChaincodeDefinition{
						ccdef0,
						ccdef1,
					},
				).To(ConsistOf(
					[]*ledger.ChaincodeDefinition{
						{
							Name:              "definedInstalledAndApprovedCC",
							Version:           "chaincode-version",
							Hash:              []byte("packageID"),
							CollectionConfigs: &pb.CollectionConfigPackage{},
						},
						{
							Name:              "anotherDefinedInstalledAndApprovedCC",
							Version:           "chaincode-version",
							Hash:              []byte("anotherPackageID"),
							CollectionConfigs: &pb.CollectionConfigPackage{},
						},
					},
				))
				Expect([][]byte{dbArtifacts0, dbArtifacts1}).To(Equal([][]byte{[]byte("db-artifacts"), []byte("db-artifacts")}))
			})

			Context("when chaincode store returns error for one of the chaincodes", func() {
				BeforeEach(func() {
					fakeCCStore.LoadStub = func(packageID string) ([]byte, error) {
						if packageID == "packageID" {
							return nil, fmt.Errorf("loading-error")
						}
						return []byte("package-bytes"), nil
					}
				})
				It("suppresses the error", func() {
					err := c.RegisterListener("channel-id", fakeListener, true)
					Expect(err).NotTo(HaveOccurred())
					Expect(fakeListener.HandleChaincodeDeployCallCount()).To(Equal(1))
					Expect(fakeListener.ChaincodeDeployDoneCallCount()).To(Equal(1))
				})
			})

			Context("when chaincode package parser returns error for both the chaincodes", func() {
				BeforeEach(func() {
					fakeParser.ParseReturns(nil, fmt.Errorf("parsing-error"))
				})
				It("suppresses the error", func() {
					err := c.RegisterListener("channel-id", fakeListener, true)
					Expect(err).NotTo(HaveOccurred())
					Expect(fakeListener.HandleChaincodeDeployCallCount()).To(Equal(0))
					Expect(fakeListener.ChaincodeDeployDoneCallCount()).To(Equal(0))
				})
			})

			Context("when listener returns error", func() {
				BeforeEach(func() {
					fakeListener.HandleChaincodeDeployReturns(fmt.Errorf("listener-error"))
				})
				It("suppresses the error", func() {
					err := c.RegisterListener("channel-id", fakeListener, true)
					Expect(err).NotTo(HaveOccurred())
					Expect(fakeListener.HandleChaincodeDeployCallCount()).To(Equal(2))
					Expect(fakeListener.HandleChaincodeDeployCallCount()).To(Equal(2))
				})
			})
		})
	})

	Describe("StateListener", func() {
		Describe("InterestedInNamespaces", func() {
			It("returns _lifecycle", func() {
				Expect(c.InterestedInNamespaces()).To(Equal([]string{"_lifecycle"}))
			})
		})

		Describe("HandleStateUpdates", func() {
			var (
				fakePublicState   MapLedgerShim
				trigger           *ledger.StateUpdateTrigger
				fakeQueryExecutor *mock.SimpleQueryExecutor
			)

			BeforeEach(func() {
				fakePublicState = map[string][]byte{}
				fakeQueryExecutor = &mock.SimpleQueryExecutor{}
				fakeQueryExecutor.GetStateStub = func(namespace, key string) ([]byte, error) {
					return fakePublicState.GetState(key)
				}

				err := resources.Serializer.Serialize(lifecycle.NamespacesName, "chaincode-name", &lifecycle.ChaincodeDefinition{
					Sequence: 7,
				}, fakePublicState)

				Expect(err).NotTo(HaveOccurred())

				trigger = &ledger.StateUpdateTrigger{
					LedgerID: "channel-id",
					StateUpdates: ledger.StateUpdates(map[string]*ledger.KVStateUpdates{
						"_lifecycle": {
							PublicUpdates: []*kvrwset.KVWrite{
								{Key: "namespaces/fields/chaincode-name/Sequence"},
							},
							CollHashUpdates: map[string][]*kvrwset.KVWriteHash{},
						},
					}),
					PostCommitQueryExecutor: fakeQueryExecutor,
				}
			})

			It("updates the modified chaincodes", func() {
				err := c.HandleStateUpdates(trigger)
				Expect(err).NotTo(HaveOccurred())
				Expect(channelCache.Chaincodes["chaincode-name"].Definition.Sequence).To(Equal(int64(7)))
			})

			Context("when the update is not to the sequence", func() {
				BeforeEach(func() {
					trigger.StateUpdates["_lifecycle"].PublicUpdates[0].Key = "namespaces/fields/chaincode-name/EndorsementInfo"
				})

				It("no update occurs", func() {
					err := c.HandleStateUpdates(trigger)
					Expect(err).NotTo(HaveOccurred())
					Expect(channelCache.Chaincodes["chaincode-name"].Definition.Sequence).To(Equal(int64(3)))
					Expect(fakeQueryExecutor.GetStateCallCount()).To(Equal(0))
				})
			})

			Context("when the update encounters an error", func() {
				BeforeEach(func() {
					fakeQueryExecutor.GetStateReturns(nil, fmt.Errorf("state-error"))
				})

				It("wraps and returns the error", func() {
					err := c.HandleStateUpdates(trigger)
					Expect(err.Error()).To(Equal("error updating cache: could not get chaincode definition for 'chaincode-name' on channel 'channel-id': could not deserialize metadata for chaincode chaincode-name: could not query metadata for namespace namespaces/chaincode-name: state-error"))
				})
			})

			Context("when the update is to private data", func() {
				BeforeEach(func() {
					trigger.StateUpdates["_lifecycle"].PublicUpdates = nil
					trigger.StateUpdates["_lifecycle"].CollHashUpdates["_implicit_org_my-mspid"] = []*kvrwset.KVWriteHash{
						{KeyHash: util.ComputeSHA256([]byte("namespaces/fields/chaincode-name#3/EndorsementInfo"))},
					}
				})

				It("updates the corresponding chaincode", func() {
					err := c.HandleStateUpdates(trigger)
					Expect(err).NotTo(HaveOccurred())
					Expect(channelCache.Chaincodes["chaincode-name"].Definition.Sequence).To(Equal(int64(7)))
				})
			})

			Context("when the private update is not in our implicit collection", func() {
				BeforeEach(func() {
					trigger.StateUpdates["_lifecycle"].PublicUpdates = nil
					trigger.StateUpdates["_lifecycle"].CollHashUpdates["_implicit_org_other-mspid"] = []*kvrwset.KVWriteHash{
						{KeyHash: util.ComputeSHA256([]byte("namespaces/fields/chaincode-name#3/EndorsementInfo"))},
					}
				})

				It("it does not mark the chaincode dirty", func() {
					err := c.HandleStateUpdates(trigger)
					Expect(err).NotTo(HaveOccurred())
					Expect(channelCache.Chaincodes["chaincode-name"].Definition.Sequence).To(Equal(int64(3)))
					Expect(fakeQueryExecutor.GetStateCallCount()).To(Equal(0))
				})
			})

			Context("when the private update is not in an implicit collection", func() {
				BeforeEach(func() {
					trigger.StateUpdates["_lifecycle"].PublicUpdates = nil
					trigger.StateUpdates["_lifecycle"].CollHashUpdates["random-collection"] = []*kvrwset.KVWriteHash{
						{KeyHash: util.ComputeSHA256([]byte("namespaces/fields/chaincode-name#3/EndorsementInfo"))},
					}
				})

				It("it does not mark the chaincode dirty", func() {
					err := c.HandleStateUpdates(trigger)
					Expect(err).NotTo(HaveOccurred())
					Expect(channelCache.Chaincodes["chaincode-name"].Definition.Sequence).To(Equal(int64(3)))
					Expect(fakeQueryExecutor.GetStateCallCount()).To(Equal(0))
				})
			})

			Context("when the state update contains no updates to _lifecycle", func() {
				BeforeEach(func() {
					trigger.StateUpdates = ledger.StateUpdates(map[string]*ledger.KVStateUpdates{})
				})

				It("returns an error", func() {
					err := c.HandleStateUpdates(trigger)
					Expect(err).To(MatchError("no state updates for promised namespace _lifecycle"))
				})
			})
		})
	})

	Describe("EventsOnCacheUpdates", func() {
		var fakeListener *ledgermock.ChaincodeLifecycleEventListener

		BeforeEach(func() {
			fakeListener = &ledgermock.ChaincodeLifecycleEventListener{}
			c.RegisterListener("channel-id", fakeListener, false)
		})

		Context("when initializing cache", func() {
			BeforeEach(func() {
				fakeQueryExecutor.GetPrivateDataHashStub = func(namespace, collection, key string) ([]byte, error) {
					return fakePrivateState.GetStateHash(key)
				}
				err := resources.Serializer.Serialize(lifecycle.NamespacesName, "chaincode-name", &lifecycle.ChaincodeDefinition{
					Sequence: 4,
				}, fakePublicState)
				Expect(err).NotTo(HaveOccurred())

				err = resources.Serializer.Serialize(lifecycle.NamespacesName, "chaincode-name#4", &lifecycle.ChaincodeParameters{},
					fakePrivateState)
				Expect(err).NotTo(HaveOccurred())

				err = resources.Serializer.Serialize(lifecycle.ChaincodeSourcesName, "chaincode-name#4", &lifecycle.ChaincodeLocalPackage{PackageID: "packageID"},
					fakePrivateState)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should not invoke listener", func() {
				err := c.InitializeLocalChaincodes()
				Expect(err).NotTo(HaveOccurred())
				err = c.Initialize("channel-id", fakeQueryExecutor)
				Expect(err).NotTo(HaveOccurred())
				chaincodeInfo, err := c.ChaincodeInfo("channel-id", "chaincode-name")
				Expect(err).NotTo(HaveOccurred())
				Expect(chaincodeInfo.Approved).To(BeTrue())
				Expect(chaincodeInfo.Definition).NotTo(BeNil())
				Expect(chaincodeInfo.InstallInfo).NotTo(BeNil())
				Expect(fakeListener.HandleChaincodeDeployCallCount()).To(Equal(0))
				Expect(fakeListener.ChaincodeDeployDoneCallCount()).To(Equal(0))
			})
		})

		Context("when new chaincode becomes available", func() {
			var (
				definitionTrigger *ledger.StateUpdateTrigger
				approvalTrigger   *ledger.StateUpdateTrigger
				install           func(string)
				define            func(string, int64)
				approve           func(string, string, int64)
				verifyNoEvent     func()
				verifyEvent       func(string, string)
			)
			BeforeEach(func() {
				definitionTrigger = &ledger.StateUpdateTrigger{
					LedgerID: "channel-id",
					StateUpdates: ledger.StateUpdates(map[string]*ledger.KVStateUpdates{
						"_lifecycle": {
							PublicUpdates: []*kvrwset.KVWrite{
								{Key: "namespaces/fields/chaincode-name-1/Sequence"},
							},
							CollHashUpdates: map[string][]*kvrwset.KVWriteHash{},
						},
					}),
					PostCommitQueryExecutor: fakeQueryExecutor,
				}

				approvalTrigger = &ledger.StateUpdateTrigger{
					LedgerID: "channel-id",
					StateUpdates: ledger.StateUpdates(map[string]*ledger.KVStateUpdates{
						"_lifecycle": {
							PublicUpdates: []*kvrwset.KVWrite{},
							CollHashUpdates: map[string][]*kvrwset.KVWriteHash{
								"_implicit_org_my-mspid": {
									{
										KeyHash: util.ComputeSHA256([]byte("namespaces/fields/chaincode-name-1#1/EndorsementInfo")),
									},
								},
							},
						},
					}),
					PostCommitQueryExecutor: fakeQueryExecutor,
				}

				install = func(packageID string) {
					c.HandleChaincodeInstalled(
						&persistence.ChaincodePackageMetadata{
							Type:  "cc-type",
							Path:  "cc-path",
							Label: "label",
						},
						packageID,
					)
				}

				define = func(chaincodeName string, sequence int64) {
					err := resources.Serializer.Serialize(lifecycle.NamespacesName, chaincodeName,
						&lifecycle.ChaincodeDefinition{
							Sequence:        sequence,
							EndorsementInfo: &lb.ChaincodeEndorsementInfo{Version: "version-1"},
						}, fakePublicState)
					Expect(err).NotTo(HaveOccurred())
					err = c.HandleStateUpdates(definitionTrigger)
					Expect(err).NotTo(HaveOccurred())
				}

				approve = func(packageID, chaincodeName string, sequence int64) {
					err := resources.Serializer.Serialize(lifecycle.NamespacesName, fmt.Sprintf("%s#%d", chaincodeName, sequence),
						&lifecycle.ChaincodeParameters{
							EndorsementInfo: &lb.ChaincodeEndorsementInfo{Version: "version-1"},
						},
						fakePrivateState)
					Expect(err).NotTo(HaveOccurred())
					err = resources.Serializer.Serialize(lifecycle.ChaincodeSourcesName, fmt.Sprintf("%s#%d", chaincodeName, sequence),

						&lifecycle.ChaincodeLocalPackage{
							PackageID: packageID,
						},
						fakePrivateState)
					Expect(err).NotTo(HaveOccurred())
					err = c.HandleStateUpdates(approvalTrigger)
					Expect(err).NotTo(HaveOccurred())
				}

				verifyNoEvent = func() {
					Expect(fakeListener.HandleChaincodeDeployCallCount()).To(Equal(0))
					Expect(fakeListener.ChaincodeDeployDoneCallCount()).To(Equal(0))
				}

				verifyEvent = func(packageID, chaincodeName string) {
					Expect(fakeListener.HandleChaincodeDeployCallCount()).To(Equal(1))
					ccdef, dbArtifacts := fakeListener.HandleChaincodeDeployArgsForCall(0)
					Expect(ccdef.Name).To(Equal(chaincodeName))
					Expect(ccdef.Version).To(Equal("version-1"))
					Expect(ccdef.Hash).To(Equal([]byte(packageID)))
					Expect(dbArtifacts).To(Equal([]byte("db-artifacts")))
				}
			})

			Context("when chaincode becomes invokable by the sequence of events define, install, and approve", func() {
				It("causes the event listener to receive event on approve step", func() {
					define("chaincode-name-1", 1)
					install("packageID-1")
					verifyNoEvent()
					approve("packageID-1", "chaincode-name-1", 1)
					verifyEvent("packageID-1", "chaincode-name-1")
					Expect(fakeListener.ChaincodeDeployDoneCallCount()).To(Equal(0))
					c.StateCommitDone("channel-id")
					Expect(fakeListener.ChaincodeDeployDoneCallCount()).To(Equal(1))
				})
			})

			Context("when chaincode becomes invokable by the sequence of events install, define, and approve", func() {
				It("causes the event listener to receive event on approve step", func() {
					install("packageID-1")
					define("chaincode-name-1", 1)
					verifyNoEvent()
					approve("packageID-1", "chaincode-name-1", 1)
					verifyEvent("packageID-1", "chaincode-name-1")
					Expect(fakeListener.ChaincodeDeployDoneCallCount()).To(Equal(0))
					c.StateCommitDone("channel-id")
					Expect(fakeListener.ChaincodeDeployDoneCallCount()).To(Equal(1))
				})
			})

			Context("when chaincode becomes invokable by the sequence of events install, approve, and define", func() {
				It("causes the event listener to receive event on define step", func() {
					install("packageID-1")
					approve("packageID-1", "chaincode-name-1", 1)
					verifyNoEvent()
					define("chaincode-name-1", 1)
					verifyEvent("packageID-1", "chaincode-name-1")
					Expect(fakeListener.ChaincodeDeployDoneCallCount()).To(Equal(0))
					c.StateCommitDone("channel-id")
					Expect(fakeListener.ChaincodeDeployDoneCallCount()).To(Equal(1))
				})
			})

			Context("when chaincode becomes invokable by the sequence of events approve, install, and define", func() {
				It("causes the event listener to receive event on define step", func() {
					approve("packageID-1", "chaincode-name-1", 1)
					install("packageID-1")
					verifyNoEvent()
					define("chaincode-name-1", 1)
					verifyEvent("packageID-1", "chaincode-name-1")
					Expect(fakeListener.ChaincodeDeployDoneCallCount()).To(Equal(0))
					c.StateCommitDone("channel-id")
					Expect(fakeListener.ChaincodeDeployDoneCallCount()).To(Equal(1))
				})
			})

			Context("when chaincode becomes invokable by the sequence of events define, approve, and install", func() {
				It("causes the event listener to receive event on install step", func() {
					define("chaincode-name-1", 1)
					approve("packageID-1", "chaincode-name-1", 1)
					verifyNoEvent()
					install("packageID-1")
					verifyEvent("packageID-1", "chaincode-name-1")
					Expect(fakeListener.ChaincodeDeployDoneCallCount()).To(Equal(1))
				})
			})

			Context("when chaincode becomes invokable by the sequence of events approve, define, and install", func() {
				It("causes the event listener to receive event on install step", func() {
					approve("packageID-1", "chaincode-name-1", 1)
					define("chaincode-name-1", 1)
					verifyNoEvent()
					install("packageID-1")
					verifyEvent("packageID-1", "chaincode-name-1")
					Expect(fakeListener.ChaincodeDeployDoneCallCount()).To(Equal(1))
				})
			})

			Context("when chaincode definition is updated by the sequence of events install, approve, and define for existing chaincode name", func() {
				BeforeEach(func() {
					channelCache.Chaincodes["chaincode-name"].InstallInfo = &lifecycle.ChaincodeInstallInfo{
						Label:     "chaincode-label",
						PackageID: "packageID",
					}
					definitionTrigger.StateUpdates["_lifecycle"].PublicUpdates[0].Key = "namespaces/fields/chaincode-name/Sequence"

					approvalTrigger.StateUpdates["_lifecycle"].CollHashUpdates["_implicit_org_my-mspid"] = []*kvrwset.KVWriteHash{
						{KeyHash: util.ComputeSHA256([]byte("namespaces/fields/chaincode-name#4/EndorsementInfo"))},
					}
				})

				It("receives the event and cleans up stale chaincode definition references", func() {
					install("packageID-1")
					approve("packageID-1", "chaincode-name", 4)
					define("chaincode-name", 4)
					c.StateCommitDone("channel-id")
					verifyEvent("packageID-1", "chaincode-name")
					Expect(fakeListener.ChaincodeDeployDoneCallCount()).To(Equal(1))

					installedCC, err := c.GetInstalledChaincode("packageID")
					Expect(err).NotTo(HaveOccurred())
					Expect(installedCC.References["channel-id"]).To(HaveLen(0))
					Expect(installedCC.References["another-channel-id"]).To(HaveLen(1))
					installedCC, err = c.GetInstalledChaincode("packageID-1")
					Expect(err).NotTo(HaveOccurred())
					Expect(installedCC.References["channel-id"]).To(HaveLen(1))
				})
			})

			Context("when an existing chaincode definition with a package is updated", func() {
				BeforeEach(func() {
					channelCache.Chaincodes["chaincode-name"].InstallInfo = &lifecycle.ChaincodeInstallInfo{
						Label:     "chaincode-label",
						PackageID: "packageID",
					}

					localChaincodes = map[string]*lifecycle.LocalChaincode{
						string(util.ComputeSHA256(protoutil.MarshalOrPanic(&lb.StateData{
							Type: &lb.StateData_String_{String_: "packageID"},
						}))): {
							References: map[string]map[string]*lifecycle.CachedChaincodeDefinition{
								"channel-id": {
									"chaincode-name": channelCache.Chaincodes["chaincode-name"],
								},
							},
							Info: &lifecycle.ChaincodeInstallInfo{
								Label:     "chaincode-label",
								PackageID: "packageID",
							},
						},
					}
					lifecycle.SetLocalChaincodesMap(c, localChaincodes)

					err := resources.Serializer.Serialize(lifecycle.NamespacesName, "chaincode-name", &lifecycle.ChaincodeDefinition{
						Sequence: 3,
					}, fakePublicState)
					Expect(err).NotTo(HaveOccurred())
				})

				Context("by an approve event with an empty package ID for the current sequence number", func() {
					BeforeEach(func() {
						approvalTrigger.StateUpdates["_lifecycle"].CollHashUpdates["_implicit_org_my-mspid"] = []*kvrwset.KVWriteHash{
							{KeyHash: util.ComputeSHA256([]byte("chaincode-sources/fields/chaincode-name#3/PackageID"))},
						}
					})

					It("receives the event, cleans up stale chaincode definition references and stops the unreferenced chaincode", func() {
						approve("", "chaincode-name", 3)

						installedCC, err := c.GetInstalledChaincode("packageID")
						Expect(err).NotTo(HaveOccurred())
						Expect(installedCC.References["channel-id"]).To(HaveLen(0))
						Expect(err).NotTo(HaveOccurred())

						fakeLauncher := &mock.ChaincodeLauncher{}
						go chaincodeCustodian.Work(nil, nil, fakeLauncher)
						Eventually(fakeLauncher.StopCallCount).Should(Equal(1))
					})
				})

				Context("by approve and commit events with an empty package ID for the next sequence number", func() {
					BeforeEach(func() {
						approvalTrigger.StateUpdates["_lifecycle"].CollHashUpdates["_implicit_org_my-mspid"] = []*kvrwset.KVWriteHash{
							{KeyHash: util.ComputeSHA256([]byte("chaincode-sources/fields/chaincode-name#4/PackageID"))},
						}

						definitionTrigger.StateUpdates["_lifecycle"].PublicUpdates[0].Key = "namespaces/fields/chaincode-name/Sequence"
					})

					It("receives the event, cleans up stale chaincode definition references and stops the unreferenced chaincode", func() {
						approve("", "chaincode-name", 4)
						define("chaincode-name", 4)
						c.StateCommitDone("channel-id")

						installedCC, err := c.GetInstalledChaincode("packageID")
						Expect(err).NotTo(HaveOccurred())
						Expect(installedCC.References["channel-id"]).To(HaveLen(0))
						Expect(err).NotTo(HaveOccurred())

						fakeLauncher := &mock.ChaincodeLauncher{}
						go chaincodeCustodian.Work(nil, nil, fakeLauncher)
						Eventually(fakeLauncher.StopCallCount).Should(Equal(1))
					})
				})
			})
		})
	})
})
