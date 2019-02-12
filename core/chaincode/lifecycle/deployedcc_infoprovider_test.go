/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle_test

import (
	"fmt"

	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle/mock"
	"github.com/hyperledger/fabric/core/ledger"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/ledger/rwset/kvrwset"

	"github.com/golang/protobuf/proto"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Lifecycle", func() {
	Describe("DeployedCCInfoProvider", func() {
		var (
			l                       *lifecycle.Lifecycle
			fakeLegacyProvider      *mock.LegacyDeployedCCInfoProvider
			fakeQueryExecutor       *mock.SimpleQueryExecutor
			fakeChannelConfigSource *mock.ChannelConfigSource
			fakeChannelConfig       *mock.ChannelConfig
			fakeApplicationConfig   *mock.ApplicationConfig
			fakeOrgConfigs          []*mock.ApplicationOrgConfig

			fakePublicState MapLedgerShim
		)

		BeforeEach(func() {
			fakeLegacyProvider = &mock.LegacyDeployedCCInfoProvider{}
			fakeChannelConfigSource = &mock.ChannelConfigSource{}
			fakeChannelConfig = &mock.ChannelConfig{}
			fakeChannelConfigSource.GetStableChannelConfigReturns(fakeChannelConfig)
			fakeApplicationConfig = &mock.ApplicationConfig{}
			fakeChannelConfig.ApplicationConfigReturns(fakeApplicationConfig, true)
			fakeOrgConfigs = []*mock.ApplicationOrgConfig{{}, {}}
			fakeOrgConfigs[0].MSPIDReturns("first-mspid")
			fakeOrgConfigs[1].MSPIDReturns("second-mspid")

			fakeApplicationConfig.OrganizationsReturns(map[string]channelconfig.ApplicationOrg{
				"org0": fakeOrgConfigs[0],
				"org1": fakeOrgConfigs[1],
			})

			l = &lifecycle.Lifecycle{
				LegacyDeployedCCInfoProvider: fakeLegacyProvider,
				ChannelConfigSource:          fakeChannelConfigSource,
				Serializer:                   &lifecycle.Serializer{},
			}

			fakePublicState = MapLedgerShim(map[string][]byte{})
			fakeQueryExecutor = &mock.SimpleQueryExecutor{}
			fakeQueryExecutor.GetStateStub = func(namespace, key string) ([]byte, error) {
				return fakePublicState.GetState(key)
			}

			err := l.Serializer.Serialize(lifecycle.NamespacesName, "cc-name", &lifecycle.DefinedChaincode{
				Version: "version",
				Hash:    []byte("hash"),
			}, fakePublicState)
			Expect(err).NotTo(HaveOccurred())
		})

		Describe("Namespaces", func() {
			BeforeEach(func() {
				fakeLegacyProvider.NamespacesReturns([]string{"a", "b", "c"})
			})

			It("appends its own namespaces the legacy impl", func() {
				res := l.Namespaces()
				Expect(res).To(Equal([]string{"+lifecycle", "a", "b", "c"}))
				Expect(fakeLegacyProvider.NamespacesCallCount()).To(Equal(1))
			})
		})

		Describe("ChaincodeInNewLifecycle", func() {
			It("returns whether the chaincode is part of the new lifecycle", func() {
				exists, state, err := l.ChaincodeInNewLifecycle("cc-name", fakeQueryExecutor)
				Expect(err).NotTo(HaveOccurred())
				Expect(exists).To(BeTrue())
				Expect(state).NotTo(BeNil())
				Expect(fakeQueryExecutor.GetStateCallCount()).To(Equal(1))
			})

			Context("when the requested chaincode is +lifecycle", func() {
				It("it returns true", func() {
					exists, state, err := l.ChaincodeInNewLifecycle("+lifecycle", fakeQueryExecutor)
					Expect(err).NotTo(HaveOccurred())
					Expect(exists).To(BeTrue())
					Expect(state).NotTo(BeNil())
					Expect(fakeQueryExecutor.GetStateCallCount()).To(Equal(0))
				})
			})

			Context("when the metadata is not for a chaincode", func() {
				BeforeEach(func() {
					type badStruct struct{}
					err := l.Serializer.Serialize(lifecycle.NamespacesName,
						"cc-name",
						&badStruct{},
						fakePublicState,
					)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns an error", func() {
					_, _, err := l.ChaincodeInNewLifecycle("cc-name", fakeQueryExecutor)
					Expect(err).To(MatchError("not a chaincode type: badStruct"))
				})
			})

			Context("when the ledger returns an error", func() {
				BeforeEach(func() {
					fakeQueryExecutor.GetStateReturns(nil, fmt.Errorf("state-error"))
				})

				It("wraps and returns the error", func() {
					_, _, err := l.ChaincodeInNewLifecycle("cc-name", fakeQueryExecutor)
					Expect(err).To(MatchError("could not deserialize metadata for chaincode cc-name: could not query metadata for namespace namespaces/cc-name: state-error"))
				})
			})

		})

		Describe("UpdatedChaincodes", func() {
			var (
				updates map[string][]*kvrwset.KVWrite
			)

			BeforeEach(func() {
				updates = map[string][]*kvrwset.KVWrite{
					"+lifecycle": {
						{Key: "some/random/value"},
						{Key: "namespaces/fields/cc-name/Sequence"},
						{Key: "prefix/namespaces/fields/cc-name/Sequence"},
						{Key: "namespaces/fields/Sequence/infix"},
						{Key: "namespaces/fields/cc-name/Sequence/Postfix"},
					},
					"other-namespace": nil,
				}
				fakeLegacyProvider.UpdatedChaincodesReturns([]*ledger.ChaincodeLifecycleInfo{
					{Name: "foo"},
					{Name: "bar"},
				}, nil)
			})

			It("checks its own namespace, then passes through to the legacy impl", func() {
				res, err := l.UpdatedChaincodes(updates)
				Expect(res).To(Equal([]*ledger.ChaincodeLifecycleInfo{
					{Name: "cc-name"},
					{Name: "foo"},
					{Name: "bar"},
				}))
				Expect(err).NotTo(HaveOccurred())
				Expect(fakeLegacyProvider.UpdatedChaincodesCallCount()).To(Equal(1))
				Expect(fakeLegacyProvider.UpdatedChaincodesArgsForCall(0)).To(Equal(updates))
			})

			Context("when the legacy provider returns an error", func() {
				BeforeEach(func() {
					fakeLegacyProvider.UpdatedChaincodesReturns(nil, fmt.Errorf("legacy-error"))
				})

				It("wraps and returns the error", func() {
					_, err := l.UpdatedChaincodes(updates)
					Expect(err).To(MatchError("error invoking legacy deployed cc info provider: legacy-error"))
				})
			})
		})

		Describe("ChaincodeInfo", func() {
			It("returns the info found in the new lifecycle", func() {
				res, err := l.ChaincodeInfo("channel-name", "cc-name", fakeQueryExecutor)
				Expect(err).NotTo(HaveOccurred())
				Expect(res.Name).To(Equal("cc-name"))
				Expect(res.Version).To(Equal("version"))
				Expect(res.Hash).To(Equal([]byte("hash")))
				Expect(proto.Equal(res.ExplicitCollectionConfigPkg, &cb.CollectionConfigPackage{})).To(BeTrue())
				Expect(len(res.ImplicitCollections)).To(Equal(2))
			})

			Context("when the requested chaincode is +lifecycle", func() {
				It("returns the implicit collections only", func() {
					res, err := l.ChaincodeInfo("channel-name", "+lifecycle", fakeQueryExecutor)
					Expect(err).NotTo(HaveOccurred())
					Expect(res.Name).To(Equal("+lifecycle"))
					Expect(len(res.ImplicitCollections)).To(Equal(2))
					Expect(res.ExplicitCollectionConfigPkg).To(BeNil())
				})
			})

			Context("when the implicit collections return an error", func() {
				BeforeEach(func() {
					fakeChannelConfig.ApplicationConfigReturns(nil, false)
				})

				It("wraps and returns the error", func() {
					_, err := l.ChaincodeInfo("channel-name", "cc-name", fakeQueryExecutor)
					Expect(err).To(MatchError("could not create implicit collections for channel: could not get application config for channel channel-name"))
				})
			})

			Context("when the ledger returns an error", func() {
				BeforeEach(func() {
					fakeQueryExecutor.GetStateReturns(nil, fmt.Errorf("state-error"))
				})

				It("wraps and returns the error", func() {
					_, err := l.ChaincodeInfo("channel-name", "cc-name", fakeQueryExecutor)
					Expect(err).To(MatchError("could not get info about chaincode: could not deserialize metadata for chaincode cc-name: could not query metadata for namespace namespaces/cc-name: state-error"))
				})
			})

			Context("when the chaincode cannot be found in the new lifecycle", func() {
				BeforeEach(func() {
					fakeLegacyProvider.ChaincodeInfoReturns(&ledger.DeployedChaincodeInfo{
						Name:    "legacy-name",
						Hash:    []byte("hash"),
						Version: "cc-version",
					}, fmt.Errorf("chaincode-info-error"))
				})

				It("passes through to the legacy impl", func() {
					res, err := l.ChaincodeInfo("channel-name", "legacy-name", fakeQueryExecutor)
					Expect(res).To(Equal(&ledger.DeployedChaincodeInfo{
						Name:    "legacy-name",
						Hash:    []byte("hash"),
						Version: "cc-version",
					}))
					Expect(err).To(MatchError("chaincode-info-error"))
					Expect(fakeLegacyProvider.ChaincodeInfoCallCount()).To(Equal(1))
					channelID, ccName, qe := fakeLegacyProvider.ChaincodeInfoArgsForCall(0)
					Expect(channelID).To(Equal("channel-name"))
					Expect(ccName).To(Equal("legacy-name"))
					Expect(qe).To(Equal(fakeQueryExecutor))
				})
			})

			Context("when the data is corrupt", func() {
				BeforeEach(func() {
					fakePublicState["namespaces/fields/cc-name/Version"] = []byte("garbage")
				})

				It("wraps and returns that error", func() {
					_, err := l.ChaincodeInfo("channel-name", "cc-name", fakeQueryExecutor)
					Expect(err).To(MatchError("could not deserialize chaincode definition for chaincode cc-name: could not unmarshal state for key namespaces/fields/cc-name/Version: proto: can't skip unknown wire type 7"))
				})
			})
		})

		Describe("CollectionInfo", func() {
			var (
				collInfo *cb.StaticCollectionConfig
			)

			BeforeEach(func() {
				collInfo = &cb.StaticCollectionConfig{}
				fakeLegacyProvider.CollectionInfoReturns(collInfo, fmt.Errorf("collection-info-error"))
			})

			It("passes through to the legacy impl", func() {
				res, err := l.CollectionInfo("channel-name", "legacy-name", "collection-name", fakeQueryExecutor)
				Expect(res).To(Equal(collInfo))
				Expect(err).To(MatchError("collection-info-error"))
				Expect(fakeLegacyProvider.CollectionInfoCallCount()).To(Equal(1))
				channelID, ccName, collName, qe := fakeLegacyProvider.CollectionInfoArgsForCall(0)
				Expect(channelID).To(Equal("channel-name"))
				Expect(ccName).To(Equal("legacy-name"))
				Expect(collName).To(Equal("collection-name"))
				Expect(qe).To(Equal(fakeQueryExecutor))
			})
		})

		Describe("ImplicitCollections", func() {
			It("returns an implicit collection for every org", func() {
				res, err := l.ImplicitCollections("channel-id", "cc-name", fakeQueryExecutor)
				Expect(err).NotTo(HaveOccurred())
				Expect(len(res)).To(Equal(2))
				var firstOrg, secondOrg *cb.StaticCollectionConfig
				for _, collection := range res {
					switch collection.Name {
					case "_implicit_org_first-mspid":
						firstOrg = collection
					case "_implicit_org_second-mspid":
						secondOrg = collection
					}
				}
				Expect(firstOrg).NotTo(BeNil())
				Expect(secondOrg).NotTo(BeNil())
			})

			Context("when the chaincode does not exist", func() {
				It("returns nil, nil", func() {
					res, err := l.ImplicitCollections("channel-id", "missing-name", fakeQueryExecutor)
					Expect(err).NotTo(HaveOccurred())
					Expect(res).To(BeNil())
				})
			})

			Context("when the ledger returns an error", func() {
				BeforeEach(func() {
					fakeQueryExecutor.GetStateReturns(nil, fmt.Errorf("state-error"))
				})

				It("wraps and returns the error", func() {
					_, err := l.ImplicitCollections("channel-id", "missing-name", fakeQueryExecutor)
					Expect(err).To(MatchError("could not get info about chaincode: could not deserialize metadata for chaincode missing-name: could not query metadata for namespace namespaces/missing-name: state-error"))
				})
			})

			Context("when there is no channel config", func() {
				BeforeEach(func() {
					fakeChannelConfigSource.GetStableChannelConfigReturns(nil)
				})

				It("returns an error", func() {
					_, err := l.ImplicitCollections("channel-id", "cc-name", fakeQueryExecutor)
					Expect(err).To(MatchError("could not get channelconfig for channel channel-id"))
				})
			})

			Context("when there is no application config", func() {
				BeforeEach(func() {
					fakeChannelConfig.ApplicationConfigReturns(nil, false)
				})

				It("returns an error", func() {
					_, err := l.ImplicitCollections("channel-id", "cc-name", fakeQueryExecutor)
					Expect(err).To(MatchError("could not get application config for channel channel-id"))
				})
			})
		})

	})
})
