/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle_test

import (
	"fmt"

	"github.com/hyperledger/fabric/common/chaincode"
	commonledger "github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle/mock"
	"github.com/hyperledger/fabric/core/chaincode/persistence"
	ccpersistence "github.com/hyperledger/fabric/core/chaincode/persistence/intf"
	"github.com/hyperledger/fabric/core/ledger"
	ledgermock "github.com/hyperledger/fabric/core/ledger/mock"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/ledger/queryresult"
	"github.com/hyperledger/fabric/protos/ledger/rwset/kvrwset"
	lb "github.com/hyperledger/fabric/protos/peer/lifecycle"
	"github.com/hyperledger/fabric/protoutil"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Cache", func() {
	var (
		c                   *lifecycle.Cache
		resources           *lifecycle.Resources
		fakeCCStore         *mock.ChaincodeStore
		fakeParser          *mock.PackageParser
		fakeMetadataHandler *mock.MetadataHandler
		channelCache        *lifecycle.ChannelCache
		localChaincodes     map[string]*lifecycle.LocalChaincode
		fakePublicState     MapLedgerShim
		fakePrivateState    MapLedgerShim
		fakeQueryExecutor   *mock.SimpleQueryExecutor
	)

	BeforeEach(func() {
		fakeCCStore = &mock.ChaincodeStore{}
		fakeParser = &mock.PackageParser{}
		resources = &lifecycle.Resources{
			PackageParser:  fakeParser,
			ChaincodeStore: fakeCCStore,
			Serializer:     &lifecycle.Serializer{},
		}

		fakeCCStore.ListInstalledChaincodesReturns([]chaincode.InstalledChaincode{
			{
				Hash:      []byte("hash"),
				PackageID: ccpersistence.PackageID("packageID"),
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

		var err error
		c = lifecycle.NewCache(resources, "my-mspid", fakeMetadataHandler)
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
						Collections: &cb.CollectionConfigPackage{},
					},
					Approved: true,
					Hashes: []string{
						string(util.ComputeSHA256([]byte("namespaces/metadata/chaincode-name#3"))),
						string(util.ComputeSHA256([]byte("namespaces/fields/chaincode-name#3/Sequence"))),
						string(util.ComputeSHA256([]byte("namespaces/fields/chaincode-name#3/EndorsementInfo"))),
						string(util.ComputeSHA256([]byte("namespaces/fields/chaincode-name#3/ValidationInfo"))),
						string(util.ComputeSHA256([]byte("namespaces/fields/chaincode-name#3/Collections"))),
					},
				},
			},
			InterestingHashes: map[string]string{
				string(util.ComputeSHA256([]byte("namespaces/metadata/chaincode-name#3"))):               "chaincode-name",
				string(util.ComputeSHA256([]byte("namespaces/fields/chaincode-name#3/Sequence"))):        "chaincode-name",
				string(util.ComputeSHA256([]byte("namespaces/fields/chaincode-name#3/EndorsementInfo"))): "chaincode-name",
				string(util.ComputeSHA256([]byte("namespaces/fields/chaincode-name#3/ValidationInfo"))):  "chaincode-name",
				string(util.ComputeSHA256([]byte("namespaces/fields/chaincode-name#3/Collections"))):     "chaincode-name",
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

	Describe("ChaincodeInfo", func() {
		BeforeEach(func() {
			channelCache.Chaincodes["chaincode-name"].InstallInfo = &lifecycle.ChaincodeInstallInfo{
				Type:      "cc-type",
				Path:      "cc-path",
				PackageID: ccpersistence.PackageID("hash"),
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
					Collections: &cb.CollectionConfigPackage{},
				},
				InstallInfo: &lifecycle.ChaincodeInstallInfo{
					Type:      "cc-type",
					Path:      "cc-path",
					PackageID: ccpersistence.PackageID("hash"),
				},
				Approved: true,
			}))
		})

		Context("when the chaincode has no cache", func() {
			It("returns an error", func() {
				_, err := c.ChaincodeInfo("channel-id", "missing-name")
				Expect(err).To(MatchError("unknown chaincode 'missing-name' for channel 'channel-id'"))
			})
		})

		Context("when the channel does not exist", func() {
			It("returns an error", func() {
				_, err := c.ChaincodeInfo("missing-channel-id", "missing-name")
				Expect(err).To(MatchError("unknown channel 'missing-channel-id'"))
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
				PackageID: ccpersistence.PackageID("packageID"),
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
				Expect(err.Error()).To(ContainSubstring("could not load chaincode with pakcage ID 'packageID'"))
			})
		})

		Context("when the chaincode package cannot be parsed", func() {
			BeforeEach(func() {
				fakeParser.ParseReturns(nil, fmt.Errorf("parse-error"))
			})

			It("wraps and returns the error", func() {
				err := c.InitializeLocalChaincodes()
				Expect(err.Error()).To(ContainSubstring("could not parse chaincode with pakcage ID 'packageID'"))
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

			Context("when the chaincode is installed afterwards", func() {
				It("gets its install info set", func() {
					err := c.Initialize("channel-id", fakeQueryExecutor)
					Expect(err).NotTo(HaveOccurred())
					c.HandleChaincodeInstalled(&persistence.ChaincodePackageMetadata{
						Type: "some-type",
						Path: "some-path",
					}, ccpersistence.PackageID("different-hash"))
					Expect(channelCache.Chaincodes["chaincode-name"].InstallInfo).To(Equal(&lifecycle.ChaincodeInstallInfo{
						Type:      "some-type",
						Path:      "some-path",
						PackageID: ccpersistence.PackageID("different-hash"),
					}))
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
				}, ccpersistence.PackageID("hash"))
			})

			It("updates the install info", func() {
				err := c.Initialize("channel-id", fakeQueryExecutor)
				Expect(err).NotTo(HaveOccurred())
				Expect(channelCache.Chaincodes["chaincode-name"].InstallInfo).To(Equal(&lifecycle.ChaincodeInstallInfo{
					Type:      "cc-type",
					Path:      "cc-path",
					PackageID: ccpersistence.PackageID("hash"),
				}))
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
				Expect(err).To(MatchError("could not check opaque org state for 'chaincode-name' on channel 'channel-id': could not get value for key namespaces/metadata/chaincode-name#7: private-data-error"))
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
							Collections: &cb.CollectionConfigPackage{},
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
							Collections: &cb.CollectionConfigPackage{},
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
							Collections: &cb.CollectionConfigPackage{},
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
					CollectionsConfig: &cb.CollectionConfigPackage{},
					Approved:          true,
					Installed:         true,
				},
				chaincode.Metadata{
					Name:              "ididntinstall",
					Version:           "3",
					Policy:            []byte("validation-parameter"),
					CollectionsConfig: &cb.CollectionConfigPackage{},
					Approved:          true,
					Installed:         false,
				},
				chaincode.Metadata{
					Name:              "idontapprove",
					Version:           "3",
					Policy:            []byte("validation-parameter"),
					CollectionsConfig: &cb.CollectionConfigPackage{},
					Approved:          false,
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
		var (
			fakeListener *ledgermock.ChaincodeLifecycleEventListener
		)

		BeforeEach(func() {
			fakeListener = &ledgermock.ChaincodeLifecycleEventListener{}
			c.RegisterListener("channel-id", fakeListener)

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
				install           func()
				define            func()
				approve           func()
				verifyNoEvent     func()
				verifyEvent       func()
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

				install = func() {
					c.HandleChaincodeInstalled(
						&persistence.ChaincodePackageMetadata{
							Type:  "cc-type",
							Path:  "cc-path",
							Label: "label",
						},
						ccpersistence.PackageID("packageID-1"),
					)
				}

				define = func() {
					err := resources.Serializer.Serialize(lifecycle.NamespacesName, "chaincode-name-1",
						&lifecycle.ChaincodeDefinition{
							Sequence:        1,
							EndorsementInfo: &lb.ChaincodeEndorsementInfo{Version: "version-1"},
						}, fakePublicState)
					Expect(err).NotTo(HaveOccurred())
					err = c.HandleStateUpdates(definitionTrigger)
					Expect(err).NotTo(HaveOccurred())
				}

				approve = func() {
					err := resources.Serializer.Serialize(lifecycle.NamespacesName, "chaincode-name-1#1",
						&lifecycle.ChaincodeParameters{
							EndorsementInfo: &lb.ChaincodeEndorsementInfo{Version: "version-1"},
						},
						fakePrivateState)
					Expect(err).NotTo(HaveOccurred())
					err = resources.Serializer.Serialize(lifecycle.ChaincodeSourcesName, "chaincode-name-1#1",
						&lifecycle.ChaincodeLocalPackage{
							PackageID: "packageID-1",
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

				verifyEvent = func() {
					Expect(fakeListener.HandleChaincodeDeployCallCount()).To(Equal(1))
					ccdef, dbArtifacts := fakeListener.HandleChaincodeDeployArgsForCall(0)
					Expect(ccdef.Name).To(Equal("chaincode-name-1"))
					Expect(ccdef.Version).To(Equal("version-1"))
					Expect(ccdef.Hash).To(Equal([]byte("packageID-1")))
					Expect(dbArtifacts).To(Equal([]byte("db-artifacts")))
				}
			})

			Context("when chaincode becomes invokable by the sequence of events define, install, and approve", func() {

				It("causes the event listener to receive event on approve step", func() {
					define()
					install()
					verifyNoEvent()
					approve()
					verifyEvent()
					Expect(fakeListener.ChaincodeDeployDoneCallCount()).To(Equal(0))
					c.StateCommitDone("channel-id")
					Expect(fakeListener.ChaincodeDeployDoneCallCount()).To(Equal(1))
				})
			})

			Context("when chaincode becomes invokable by the sequence of events install, define, and approve", func() {

				It("causes the event listener to receive event on approve step", func() {
					install()
					define()
					verifyNoEvent()
					approve()
					verifyEvent()
					Expect(fakeListener.ChaincodeDeployDoneCallCount()).To(Equal(0))
					c.StateCommitDone("channel-id")
					Expect(fakeListener.ChaincodeDeployDoneCallCount()).To(Equal(1))
				})
			})

			Context("when chaincode becomes invokable by the sequence of events install, approve, and define", func() {

				It("causes the event listener to receive event on define step", func() {
					install()
					approve()
					verifyNoEvent()
					define()
					verifyEvent()
					Expect(fakeListener.ChaincodeDeployDoneCallCount()).To(Equal(0))
					c.StateCommitDone("channel-id")
					Expect(fakeListener.ChaincodeDeployDoneCallCount()).To(Equal(1))
				})
			})

			Context("when chaincode becomes invokable by the sequence of events approve, install, and define", func() {

				It("causes the event listener to receive event on define step", func() {
					approve()
					install()
					verifyNoEvent()
					define()
					verifyEvent()
					Expect(fakeListener.ChaincodeDeployDoneCallCount()).To(Equal(0))
					c.StateCommitDone("channel-id")
					Expect(fakeListener.ChaincodeDeployDoneCallCount()).To(Equal(1))
				})
			})

			Context("when chaincode becomes invokable by the sequence of events define, approve, and install", func() {

				It("causes the event listener to receive event on install step", func() {
					define()
					approve()
					verifyNoEvent()
					install()
					verifyEvent()
					Expect(fakeListener.ChaincodeDeployDoneCallCount()).To(Equal(1))
				})
			})

			Context("when chaincode becomes invokable by the sequence of events approve, define, and install", func() {

				It("causes the event listener to receive event on install step", func() {
					approve()
					define()
					verifyNoEvent()
					install()
					verifyEvent()
					Expect(fakeListener.ChaincodeDeployDoneCallCount()).To(Equal(1))
				})
			})
		})
	})
})
