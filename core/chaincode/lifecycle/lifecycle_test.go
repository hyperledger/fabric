/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle_test

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	lb "github.com/hyperledger/fabric-protos-go/peer/lifecycle"
	"github.com/hyperledger/fabric/common/chaincode"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle/mock"
	"github.com/hyperledger/fabric/core/chaincode/persistence"
	"github.com/hyperledger/fabric/core/container"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ChaincodeParameters", func() {
	var lhs, rhs *lifecycle.ChaincodeParameters

	BeforeEach(func() {
		lhs = &lifecycle.ChaincodeParameters{
			EndorsementInfo: &lb.ChaincodeEndorsementInfo{},
			ValidationInfo:  &lb.ChaincodeValidationInfo{},
			Collections:     &pb.CollectionConfigPackage{},
		}

		rhs = &lifecycle.ChaincodeParameters{
			EndorsementInfo: &lb.ChaincodeEndorsementInfo{},
			ValidationInfo:  &lb.ChaincodeValidationInfo{},
			Collections:     &pb.CollectionConfigPackage{},
		}
	})

	Describe("Equal", func() {
		It("returns nil when the parameters match", func() {
			Expect(lhs.Equal(rhs)).NotTo(HaveOccurred())
		})

		Context("when the EndorsementPlugin differs from the current definition", func() {
			BeforeEach(func() {
				rhs.EndorsementInfo.EndorsementPlugin = "different"
			})

			It("returns an error", func() {
				Expect(lhs.Equal(rhs)).To(MatchError("expected EndorsementPlugin '' does not match passed EndorsementPlugin 'different'"))
			})
		})

		Context("when the InitRequired differs from the current definition", func() {
			BeforeEach(func() {
				rhs.EndorsementInfo.InitRequired = true
			})

			It("returns an error", func() {
				Expect(lhs.Equal(rhs)).To(MatchError("expected InitRequired 'false' does not match passed InitRequired 'true'"))
			})
		})

		Context("when the ValidationPlugin differs from the current definition", func() {
			BeforeEach(func() {
				rhs.ValidationInfo.ValidationPlugin = "different"
			})

			It("returns an error", func() {
				Expect(lhs.Equal(rhs)).To(MatchError("expected ValidationPlugin '' does not match passed ValidationPlugin 'different'"))
			})
		})

		Context("when the ValidationParameter differs from the current definition", func() {
			BeforeEach(func() {
				rhs.ValidationInfo.ValidationParameter = []byte("different")
			})

			It("returns an error", func() {
				Expect(lhs.Equal(rhs)).To(MatchError("expected ValidationParameter '' does not match passed ValidationParameter '646966666572656e74'"))
			})
		})

		Context("when the Collections differ from the current definition", func() {
			BeforeEach(func() {
				rhs.Collections = &pb.CollectionConfigPackage{
					Config: []*pb.CollectionConfig{
						{
							Payload: &pb.CollectionConfig_StaticCollectionConfig{
								StaticCollectionConfig: &pb.StaticCollectionConfig{Name: "foo"},
							},
						},
					},
				}
			})

			It("returns an error", func() {
				Expect(lhs.Equal(rhs)).To(MatchError("Collections do not match"))
			})
		})
	})
})

var _ = Describe("Resources", func() {
	var (
		resources               *lifecycle.Resources
		fakeChannelConfigSource *mock.ChannelConfigSource
		fakeChannelConfig       *mock.ChannelConfig
		fakeApplicationConfig   *mock.ApplicationConfig
		fakeOrgConfigs          []*mock.ApplicationOrgConfig
		fakePolicyManager       *mock.PolicyManager
	)

	BeforeEach(func() {
		fakeChannelConfigSource = &mock.ChannelConfigSource{}
		fakeChannelConfig = &mock.ChannelConfig{}
		fakeChannelConfigSource.GetStableChannelConfigReturns(fakeChannelConfig)
		fakeApplicationConfig = &mock.ApplicationConfig{}
		fakeChannelConfig.ApplicationConfigReturns(fakeApplicationConfig, true)
		fakeOrgConfigs = []*mock.ApplicationOrgConfig{{}, {}}
		fakeOrgConfigs[0].MSPIDReturns("first-mspid")
		fakeOrgConfigs[1].MSPIDReturns("second-mspid")
		fakePolicyManager = &mock.PolicyManager{}
		fakePolicyManager.GetPolicyReturns(nil, true)
		fakeChannelConfig.PolicyManagerReturns(fakePolicyManager)

		fakeApplicationConfig.OrganizationsReturns(map[string]channelconfig.ApplicationOrg{
			"org0": fakeOrgConfigs[0],
			"org1": fakeOrgConfigs[1],
		})

		resources = &lifecycle.Resources{
			ChannelConfigSource: fakeChannelConfigSource,
			Serializer:          &lifecycle.Serializer{},
		}
	})

	Describe("ChaincodeDefinitionIfDefined", func() {
		var (
			fakePublicState   MapLedgerShim
			fakeReadableState *mock.ReadWritableState
		)

		BeforeEach(func() {
			fakePublicState = map[string][]byte{}
			err := resources.Serializer.Serialize(lifecycle.NamespacesName, "cc-name", &lifecycle.ChaincodeDefinition{
				Sequence: 5,
				EndorsementInfo: &lb.ChaincodeEndorsementInfo{
					Version:           "version",
					EndorsementPlugin: "my endorsement plugin",
				},
				ValidationInfo: &lb.ChaincodeValidationInfo{
					ValidationPlugin:    "my validation plugin",
					ValidationParameter: []byte("some awesome policy"),
				},
				Collections: &pb.CollectionConfigPackage{},
			}, fakePublicState)
			Expect(err).NotTo(HaveOccurred())
			fakeReadableState = &mock.ReadWritableState{}
			fakeReadableState.GetStateStub = fakePublicState.GetState
		})

		It("returns that the chaincode is defined and that the chaincode definition can be converted to a string suitable for log messages", func() {
			exists, definition, err := resources.ChaincodeDefinitionIfDefined("cc-name", fakeReadableState)
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeTrue())
			Expect(definition.EndorsementInfo.Version).To(Equal("version"))
			Expect(fmt.Sprintf("{%s}", definition)).To(Equal("{sequence: 5, endorsement info: (version: 'version', plugin: 'my endorsement plugin', init required: false), validation info: (plugin: 'my validation plugin', policy: '736f6d6520617765736f6d6520706f6c696379'), collections: ()}"))
		})

		Context("when the requested chaincode is _lifecycle", func() {
			It("it returns true", func() {
				exists, definition, err := resources.ChaincodeDefinitionIfDefined("_lifecycle", fakeReadableState)
				Expect(err).NotTo(HaveOccurred())
				Expect(exists).To(BeTrue())
				Expect(definition).NotTo(BeNil())
				Expect(fakeReadableState.GetStateCallCount()).To(Equal(0))
			})
		})

		Context("when the metadata is not for a chaincode", func() {
			BeforeEach(func() {
				type badStruct struct{}
				err := resources.Serializer.Serialize(lifecycle.NamespacesName,
					"cc-name",
					&badStruct{},
					fakePublicState,
				)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns an error", func() {
				_, _, err := resources.ChaincodeDefinitionIfDefined("cc-name", fakeReadableState)
				Expect(err).To(MatchError("not a chaincode type: badStruct"))
			})
		})

		Context("when the ledger returns an error", func() {
			BeforeEach(func() {
				fakeReadableState.GetStateReturns(nil, fmt.Errorf("state-error"))
			})

			It("wraps and returns the error", func() {
				_, _, err := resources.ChaincodeDefinitionIfDefined("cc-name", fakeReadableState)
				Expect(err).To(MatchError("could not deserialize metadata for chaincode cc-name: could not query metadata for namespace namespaces/cc-name: state-error"))
			})
		})
	})

	Describe("LifecycleEndorsementPolicyAsBytes", func() {
		It("returns the endorsement policy for the lifecycle chaincode", func() {
			b, err := resources.LifecycleEndorsementPolicyAsBytes("channel-id")
			Expect(err).NotTo(HaveOccurred())
			Expect(b).NotTo(BeNil())
			policy := &cb.ApplicationPolicy{}
			err = proto.Unmarshal(b, policy)
			Expect(err).NotTo(HaveOccurred())
			Expect(policy.GetChannelConfigPolicyReference()).To(Equal("/Channel/Application/LifecycleEndorsement"))
		})

		Context("when the endorsement policy reference is not found", func() {
			BeforeEach(func() {
				fakePolicyManager.GetPolicyReturns(nil, false)
			})

			It("returns an error", func() {
				b, err := resources.LifecycleEndorsementPolicyAsBytes("channel-id")
				Expect(err).NotTo(HaveOccurred())
				policy := &cb.ApplicationPolicy{}
				err = proto.Unmarshal(b, policy)
				Expect(err).NotTo(HaveOccurred())
				Expect(policy.GetSignaturePolicy()).NotTo(BeNil())
			})

			Context("when the application config cannot be retrieved", func() {
				BeforeEach(func() {
					fakeChannelConfig.ApplicationConfigReturns(nil, false)
				})

				It("returns an error", func() {
					_, err := resources.LifecycleEndorsementPolicyAsBytes("channel-id")
					Expect(err).To(MatchError("could not get application config for channel 'channel-id'"))
				})
			})
		})

		Context("when the channel config cannot be retrieved", func() {
			BeforeEach(func() {
				fakeChannelConfigSource.GetStableChannelConfigReturns(nil)
			})

			It("returns an error", func() {
				_, err := resources.LifecycleEndorsementPolicyAsBytes("channel-id")
				Expect(err).To(MatchError("could not get channel config for channel 'channel-id'"))
			})
		})
	})
})

var _ = Describe("ExternalFunctions", func() {
	var (
		resources               *lifecycle.Resources
		ef                      *lifecycle.ExternalFunctions
		fakeCCStore             *mock.ChaincodeStore
		fakeChaincodeBuilder    *mock.ChaincodeBuilder
		fakeParser              *mock.PackageParser
		fakeListener            *mock.InstallListener
		fakeLister              *mock.InstalledChaincodesLister
		fakeChannelConfigSource *mock.ChannelConfigSource
		fakeChannelConfig       *mock.ChannelConfig
		fakeApplicationConfig   *mock.ApplicationConfig
		fakeOrgConfigs          []*mock.ApplicationOrgConfig
		fakePolicyManager       *mock.PolicyManager
	)

	BeforeEach(func() {
		fakeCCStore = &mock.ChaincodeStore{}
		fakeChaincodeBuilder = &mock.ChaincodeBuilder{}
		fakeParser = &mock.PackageParser{}
		fakeListener = &mock.InstallListener{}
		fakeLister = &mock.InstalledChaincodesLister{}
		fakeChannelConfigSource = &mock.ChannelConfigSource{}
		fakeChannelConfig = &mock.ChannelConfig{}
		fakeChannelConfigSource.GetStableChannelConfigReturns(fakeChannelConfig)
		fakeApplicationConfig = &mock.ApplicationConfig{}
		fakeChannelConfig.ApplicationConfigReturns(fakeApplicationConfig, true)
		fakeOrgConfigs = []*mock.ApplicationOrgConfig{{}, {}}
		fakeOrgConfigs[0].MSPIDReturns("first-mspid")
		fakeOrgConfigs[1].MSPIDReturns("second-mspid")
		fakePolicyManager = &mock.PolicyManager{}
		fakePolicyManager.GetPolicyReturns(nil, true)
		fakeChannelConfig.PolicyManagerReturns(fakePolicyManager)

		fakeApplicationConfig.OrganizationsReturns(map[string]channelconfig.ApplicationOrg{
			"org0": fakeOrgConfigs[0],
			"org1": fakeOrgConfigs[1],
		})

		resources = &lifecycle.Resources{
			PackageParser:       fakeParser,
			ChaincodeStore:      fakeCCStore,
			Serializer:          &lifecycle.Serializer{},
			ChannelConfigSource: fakeChannelConfigSource,
		}

		ef = &lifecycle.ExternalFunctions{
			Resources:                 resources,
			InstallListener:           fakeListener,
			InstalledChaincodesLister: fakeLister,
			ChaincodeBuilder:          fakeChaincodeBuilder,
			BuildRegistry:             &container.BuildRegistry{},
		}
	})

	Describe("InstallChaincode", func() {
		BeforeEach(func() {
			fakeParser.ParseReturns(&persistence.ChaincodePackage{
				Metadata: &persistence.ChaincodePackageMetadata{
					Type:  "cc-type",
					Path:  "cc-path",
					Label: "cc-label",
				},
			}, nil)
			fakeCCStore.SaveReturns("fake-hash", nil)
		})

		It("saves the chaincode", func() {
			cc, err := ef.InstallChaincode([]byte("cc-package"))
			Expect(err).NotTo(HaveOccurred())
			Expect(cc).To(Equal(&chaincode.InstalledChaincode{
				PackageID: "fake-hash",
				Label:     "cc-label",
			}))

			Expect(fakeParser.ParseCallCount()).To(Equal(1))
			Expect(fakeParser.ParseArgsForCall(0)).To(Equal([]byte("cc-package")))

			Expect(fakeCCStore.SaveCallCount()).To(Equal(1))
			name, msg := fakeCCStore.SaveArgsForCall(0)
			Expect(name).To(Equal("cc-label"))
			Expect(msg).To(Equal([]byte("cc-package")))

			Expect(fakeListener.HandleChaincodeInstalledCallCount()).To(Equal(1))
			md, packageID := fakeListener.HandleChaincodeInstalledArgsForCall(0)
			Expect(md).To(Equal(&persistence.ChaincodePackageMetadata{
				Type:  "cc-type",
				Path:  "cc-path",
				Label: "cc-label",
			}))
			Expect(packageID).To(Equal("fake-hash"))
		})

		It("builds the chaincode", func() {
			_, err := ef.InstallChaincode([]byte("cc-package"))
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeChaincodeBuilder.BuildCallCount()).To(Equal(1))
			ccid := fakeChaincodeBuilder.BuildArgsForCall(0)
			Expect(ccid).To(Equal("fake-hash"))
		})

		When("building the chaincode fails", func() {
			BeforeEach(func() {
				fakeChaincodeBuilder.BuildReturns(fmt.Errorf("fake-build-error"))
			})

			It("returns the wrapped error to the caller", func() {
				_, err := ef.InstallChaincode([]byte("cc-package"))
				Expect(err).To(MatchError("could not build chaincode: fake-build-error"))
			})

			It("deletes the chaincode from disk", func() {
				ef.InstallChaincode([]byte("cc-package"))
				Expect(fakeCCStore.DeleteCallCount()).To(Equal(1))
				Expect(fakeCCStore.DeleteArgsForCall(0)).To(Equal("fake-hash"))
			})
		})

		When("the chaincode is already being built and succeeds", func() {
			BeforeEach(func() {
				bs, ok := ef.BuildRegistry.BuildStatus("fake-hash")
				Expect(ok).To(BeFalse())
				bs.Notify(nil)
			})

			It("does not attempt to rebuild it itself", func() {
				_, err := ef.InstallChaincode([]byte("cc-package"))
				Expect(err).To(MatchError("chaincode already successfully installed (package ID 'fake-hash')"))
				Expect(fakeChaincodeBuilder.BuildCallCount()).To(Equal(0))
			})
		})

		When("the chaincode is already being built and fails", func() {
			BeforeEach(func() {
				bs, ok := ef.BuildRegistry.BuildStatus("fake-hash")
				Expect(ok).To(BeFalse())
				bs.Notify(fmt.Errorf("fake-other-builder-error"))
			})

			It("attempts to rebuild it itself", func() {
				_, err := ef.InstallChaincode([]byte("cc-package"))
				Expect(err).To(BeNil())
				Expect(fakeChaincodeBuilder.BuildCallCount()).To(Equal(1))
			})
		})

		Context("when the package does not have metadata", func() {
			BeforeEach(func() {
				fakeParser.ParseReturns(&persistence.ChaincodePackage{}, nil)
			})

			It("wraps and returns the error", func() {
				hash, err := ef.InstallChaincode([]byte("fake-package"))
				Expect(hash).To(BeNil())
				Expect(err.Error()).To(ContainSubstring("empty metadata for supplied chaincode"))
			})
		})

		Context("when saving the chaincode fails", func() {
			BeforeEach(func() {
				fakeCCStore.SaveReturns("", fmt.Errorf("fake-error"))
			})

			It("wraps and returns the error", func() {
				cc, err := ef.InstallChaincode([]byte("cc-package"))
				Expect(cc).To(BeNil())
				Expect(err).To(MatchError("could not save cc install package: fake-error"))
			})
		})

		Context("when parsing the chaincode package fails", func() {
			BeforeEach(func() {
				fakeParser.ParseReturns(nil, fmt.Errorf("parse-error"))
			})

			It("wraps and returns the error", func() {
				hash, err := ef.InstallChaincode([]byte("fake-package"))
				Expect(hash).To(BeNil())
				Expect(err).To(MatchError("could not parse as a chaincode install package: parse-error"))
			})
		})
	})

	Describe("GetInstalledChaincodePackage", func() {
		BeforeEach(func() {
			fakeCCStore.LoadReturns([]byte("code-package"), nil)
		})

		It("returns the chaincode install package", func() {
			pkgBytes, err := ef.GetInstalledChaincodePackage("some-package-id")
			Expect(err).NotTo(HaveOccurred())
			Expect(pkgBytes).To(Equal([]byte("code-package")))

			Expect(fakeCCStore.LoadCallCount()).To(Equal(1))
			packageID := fakeCCStore.LoadArgsForCall(0)
			Expect(packageID).To(Equal("some-package-id"))
		})

		Context("when loading the chaincode fails", func() {
			BeforeEach(func() {
				fakeCCStore.LoadReturns(nil, fmt.Errorf("fake-error"))
			})

			It("wraps and returns the error", func() {
				pkgBytes, err := ef.GetInstalledChaincodePackage("some-package-id")
				Expect(pkgBytes).To(BeNil())
				Expect(err).To(MatchError("could not load cc install package: fake-error"))
			})
		})
	})

	Describe("QueryInstalledChaincode", func() {
		BeforeEach(func() {
			fakeLister.GetInstalledChaincodeReturns(&chaincode.InstalledChaincode{
				Label:     "installed-cc2",
				PackageID: "installed-package-id2",
				References: map[string][]*chaincode.Metadata{
					"test-channel": {
						&chaincode.Metadata{
							Name:    "test-chaincode",
							Version: "test-version",
						},
						&chaincode.Metadata{
							Name:    "hello-chaincode",
							Version: "hello-version",
						},
					},
					"another-channel": {
						&chaincode.Metadata{
							Name:    "another-chaincode",
							Version: "another-version",
						},
					},
				},
			}, nil)
		})

		It("returns installed chaincode information", func() {
			result, err := ef.QueryInstalledChaincode("installed-package-id2")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(
				&chaincode.InstalledChaincode{
					Label:     "installed-cc2",
					PackageID: "installed-package-id2",
					References: map[string][]*chaincode.Metadata{
						"test-channel": {
							&chaincode.Metadata{
								Name:    "test-chaincode",
								Version: "test-version",
							},
							&chaincode.Metadata{
								Name:    "hello-chaincode",
								Version: "hello-version",
							},
						},
						"another-channel": {
							&chaincode.Metadata{
								Name:    "another-chaincode",
								Version: "another-version",
							},
						},
					},
				},
			))
		})

		Context("when the chaincode isn't installed", func() {
			BeforeEach(func() {
				fakeLister.GetInstalledChaincodeReturns(nil, errors.New("another-fake-error"))
			})

			It("returns an error", func() {
				result, err := ef.QueryInstalledChaincode("not-there")
				Expect(err.Error()).To(Equal("another-fake-error"))
				Expect(result).To(BeNil())
			})
		})
	})

	Describe("QueryInstalledChaincodes", func() {
		var chaincodes []*chaincode.InstalledChaincode

		BeforeEach(func() {
			chaincodes = []*chaincode.InstalledChaincode{
				{
					Label:     "installed-cc1",
					PackageID: "installed-package-id1",
				},
				{
					Label:     "installed-cc2",
					PackageID: "installed-package-id2",
					References: map[string][]*chaincode.Metadata{
						"test-channel": {
							&chaincode.Metadata{
								Name:    "test-chaincode",
								Version: "test-version",
							},
							&chaincode.Metadata{
								Name:    "hello-chaincode",
								Version: "hello-version",
							},
						},
						"another-channel": {
							&chaincode.Metadata{
								Name:    "another-chaincode",
								Version: "another-version",
							},
						},
					},
				},
			}

			fakeLister.ListInstalledChaincodesReturns(chaincodes)
		})

		It("passes through to the cache", func() {
			result := ef.QueryInstalledChaincodes()
			Expect(result).To(ConsistOf(
				&chaincode.InstalledChaincode{
					Label:     "installed-cc1",
					PackageID: "installed-package-id1",
				},
				&chaincode.InstalledChaincode{
					Label:     "installed-cc2",
					PackageID: "installed-package-id2",
					References: map[string][]*chaincode.Metadata{
						"test-channel": {
							&chaincode.Metadata{
								Name:    "test-chaincode",
								Version: "test-version",
							},
							&chaincode.Metadata{
								Name:    "hello-chaincode",
								Version: "hello-version",
							},
						},
						"another-channel": {
							&chaincode.Metadata{
								Name:    "another-chaincode",
								Version: "another-version",
							},
						},
					},
				},
			))
		})
	})

	Describe("ApproveChaincodeDefinitionForOrg", func() {
		var (
			fakePublicState *mock.ReadWritableState
			fakeOrgState    *mock.ReadWritableState

			fakeOrgKVStore    MapLedgerShim
			fakePublicKVStore MapLedgerShim

			testDefinition *lifecycle.ChaincodeDefinition
		)

		BeforeEach(func() {
			testDefinition = &lifecycle.ChaincodeDefinition{
				Sequence: 5,
				EndorsementInfo: &lb.ChaincodeEndorsementInfo{
					Version:           "version",
					EndorsementPlugin: "my endorsement plugin",
				},
				ValidationInfo: &lb.ChaincodeValidationInfo{
					ValidationPlugin:    "my validation plugin",
					ValidationParameter: []byte("some awesome policy"),
				},
				Collections: &pb.CollectionConfigPackage{},
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

			err := resources.Serializer.Serialize("namespaces", "cc-name", &lifecycle.ChaincodeDefinition{
				Sequence: 4,
			}, fakePublicKVStore)
			Expect(err).NotTo(HaveOccurred())
		})

		It("serializes the chaincode parameters to the org scoped collection", func() {
			err := ef.ApproveChaincodeDefinitionForOrg("my-channel", "cc-name", testDefinition, "hash", fakePublicState, fakeOrgState)
			Expect(err).NotTo(HaveOccurred())

			metadata, ok, err := resources.Serializer.DeserializeMetadata("namespaces", "cc-name#5", fakeOrgState)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
			committedDefinition := &lifecycle.ChaincodeParameters{}
			err = resources.Serializer.Deserialize("namespaces", "cc-name#5", metadata, committedDefinition, fakeOrgState)
			Expect(err).NotTo(HaveOccurred())
			Expect(committedDefinition.EndorsementInfo.Version).To(Equal("version"))
			Expect(committedDefinition.EndorsementInfo.EndorsementPlugin).To(Equal("my endorsement plugin"))
			Expect(proto.Equal(committedDefinition.ValidationInfo, &lb.ChaincodeValidationInfo{
				ValidationPlugin:    "my validation plugin",
				ValidationParameter: []byte("some awesome policy"),
			})).To(BeTrue())
			Expect(proto.Equal(committedDefinition.Collections, &pb.CollectionConfigPackage{})).To(BeTrue())

			metadata, ok, err = resources.Serializer.DeserializeMetadata("chaincode-sources", "cc-name#5", fakeOrgState)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
			localPackage := &lifecycle.ChaincodeLocalPackage{}
			err = resources.Serializer.Deserialize("chaincode-sources", "cc-name#5", metadata, localPackage, fakeOrgState)
			Expect(err).NotTo(HaveOccurred())
			Expect(localPackage).To(Equal(&lifecycle.ChaincodeLocalPackage{
				PackageID: "hash",
			}))
		})

		Context("when the peer sets defaults", func() {
			BeforeEach(func() {
				testDefinition.EndorsementInfo.EndorsementPlugin = ""
				testDefinition.ValidationInfo.ValidationPlugin = ""
				testDefinition.ValidationInfo.ValidationParameter = nil
			})

			It("uses appropriate defaults", func() {
				err := ef.ApproveChaincodeDefinitionForOrg("my-channel", "cc-name", testDefinition, "hash", fakePublicState, fakeOrgState)
				Expect(err).NotTo(HaveOccurred())

				metadata, ok, err := resources.Serializer.DeserializeMetadata("namespaces", "cc-name#5", fakeOrgState)
				Expect(err).NotTo(HaveOccurred())
				Expect(ok).To(BeTrue())
				committedDefinition := &lifecycle.ChaincodeParameters{}
				err = resources.Serializer.Deserialize("namespaces", "cc-name#5", metadata, committedDefinition, fakeOrgState)
				Expect(err).NotTo(HaveOccurred())
				Expect(committedDefinition.EndorsementInfo.Version).To(Equal("version"))
				Expect(committedDefinition.EndorsementInfo.EndorsementPlugin).To(Equal("escc"))
				Expect(proto.Equal(committedDefinition.ValidationInfo, &lb.ChaincodeValidationInfo{
					ValidationPlugin: "vscc",
					ValidationParameter: protoutil.MarshalOrPanic(
						&pb.ApplicationPolicy{
							Type: &pb.ApplicationPolicy_ChannelConfigPolicyReference{
								ChannelConfigPolicyReference: "/Channel/Application/Endorsement",
							},
						}),
				})).To(BeTrue())
				Expect(proto.Equal(committedDefinition.Collections, &pb.CollectionConfigPackage{})).To(BeTrue())
			})

			Context("when no default endorsement policy is defined on thc channel", func() {
				BeforeEach(func() {
					fakePolicyManager.GetPolicyReturns(nil, false)
				})

				It("returns an error", func() {
					err := ef.ApproveChaincodeDefinitionForOrg("my-channel", "cc-name", testDefinition, "hash", fakePublicState, fakeOrgState)
					Expect(err).To(MatchError(ContainSubstring("could not set defaults for chaincode definition in channel my-channel: policy '/Channel/Application/Endorsement' must be defined for channel 'my-channel' before chaincode operations can be attempted")))
				})
			})

			Context("when obtaining a stable channel config fails", func() {
				BeforeEach(func() {
					fakeChannelConfigSource.GetStableChannelConfigReturns(nil)
				})

				It("returns an error", func() {
					err := ef.ApproveChaincodeDefinitionForOrg("my-channel", "cc-name", testDefinition, "hash", fakePublicState, fakeOrgState)
					Expect(err).To(MatchError(ContainSubstring("could not get channel config for channel 'my-channel'")))
				})
			})
		})

		Context("when the current sequence is undefined and the requested sequence is 0", func() {
			BeforeEach(func() {
				fakePublicKVStore = map[string][]byte{}
			})

			It("returns an error", func() {
				err := ef.ApproveChaincodeDefinitionForOrg("my-channel", "unknown-name", &lifecycle.ChaincodeDefinition{}, "hash", fakePublicState, fakeOrgState)
				Expect(err).To(MatchError("requested sequence is 0, but first definable sequence number is 1"))
			})
		})

		Context("when the sequence number already has a committed definition", func() {
			BeforeEach(func() {
				err := resources.Serializer.Serialize("namespaces", "cc-name", &lifecycle.ChaincodeDefinition{
					Sequence: 5,
					EndorsementInfo: &lb.ChaincodeEndorsementInfo{
						Version:           "version",
						EndorsementPlugin: "my endorsement plugin",
					},
					ValidationInfo: &lb.ChaincodeValidationInfo{
						ValidationPlugin:    "my validation plugin",
						ValidationParameter: []byte("some awesome policy"),
					},
				}, fakePublicState)
				Expect(err).NotTo(HaveOccurred())

				resources.Serializer.Serialize("namespaces", "cc-name#5", testDefinition.Parameters(), fakeOrgState)
				resources.Serializer.Serialize("chaincode-sources", "cc-name#5", &lifecycle.ChaincodeLocalPackage{
					PackageID: "hash",
				}, fakeOrgState)
			})

			It("verifies that it should be impossible to approve already committed definition", func() {
				err := ef.ApproveChaincodeDefinitionForOrg("my-channel", "cc-name", testDefinition, "hash", fakePublicState, fakeOrgState)
				Expect(err).To(MatchError("attempted to redefine the current committed sequence (5) for namespace cc-name"))
			})

			Context("when the new definition being advanced", func() {
				BeforeEach(func() {
					testDefinition.Sequence++
				})

				AfterEach(func() {
					testDefinition.Sequence--
				})

				It("the approve should work without the error", func() {
					err := ef.ApproveChaincodeDefinitionForOrg("my-channel", "cc-name", testDefinition, "hash", fakePublicState, fakeOrgState)
					Expect(err).NotTo(HaveOccurred())
				})
			})

			Context("when the current definition is not found", func() {
				BeforeEach(func() {
					delete(fakePublicKVStore, "namespaces/metadata/cc-name")
				})

				It("returns an error", func() {
					err := ef.ApproveChaincodeDefinitionForOrg("my-channel", "cc-name", testDefinition, "hash", fakePublicState, fakeOrgState)
					Expect(err).To(MatchError("missing metadata for currently committed sequence number (5)"))
				})
			})

			Context("when the current definition is corrupt", func() {
				BeforeEach(func() {
					fakePublicKVStore["namespaces/metadata/cc-name"] = []byte("garbage")
				})

				It("returns an error", func() {
					err := ef.ApproveChaincodeDefinitionForOrg("my-channel", "cc-name", testDefinition, "hash", fakePublicState, fakeOrgState)
					Expect(err).To(Not(BeNil()))
					Expect(err.Error()).To(HavePrefix("could not fetch metadata for current definition: could not unmarshal metadata for namespace namespaces/cc-name"))
				})
			})

			Context("when the current definition is not a chaincode", func() {
				BeforeEach(func() {
					fakePublicKVStore = map[string][]byte{}
					type OtherStruct struct {
						Sequence int64
					}
					err := resources.Serializer.Serialize("namespaces", "cc-name", &OtherStruct{
						Sequence: 5,
					}, fakePublicState)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns an error", func() {
					err := ef.ApproveChaincodeDefinitionForOrg("my-channel", "cc-name", testDefinition, "hash", fakePublicState, fakeOrgState)
					Expect(err).To(MatchError("could not deserialize namespace cc-name as chaincode: type name mismatch 'ChaincodeDefinition' != 'OtherStruct'"))
				})
			})

			Context("when the Version in the new definition differs from the current definition", func() {
				BeforeEach(func() {
					fakePublicKVStore = map[string][]byte{}

					err := resources.Serializer.Serialize("namespaces", "cc-name", &lifecycle.ChaincodeDefinition{
						Sequence: 5,
						EndorsementInfo: &lb.ChaincodeEndorsementInfo{
							Version: "other-version",
						},
					}, fakePublicState)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns an error", func() {
					err := ef.ApproveChaincodeDefinitionForOrg("my-channel", "cc-name", testDefinition, "hash", fakePublicState, fakeOrgState)
					Expect(err).To(MatchError("attempted to redefine the current committed sequence (5) for namespace cc-name with different parameters: expected Version 'other-version' does not match passed Version 'version'"))
				})
			})
		})

		Context("when the sequence number already has an uncommitted definition", func() {
			BeforeEach(func() {
				err := ef.ApproveChaincodeDefinitionForOrg("my-channel", "cc-name", testDefinition, "hash", fakePublicState, fakeOrgState)
				Expect(err).NotTo(HaveOccurred())
			})

			Context("when uncommitted definition differs from update", func() {
				It("succeeds", func() {
					testDefinition.ValidationInfo.ValidationParameter = []byte("more awesome policy")
					err := ef.ApproveChaincodeDefinitionForOrg("my-channel", "cc-name", testDefinition, "hash", fakePublicState, fakeOrgState)
					Expect(err).NotTo(HaveOccurred())
				})
			})

			Context("when uncommitted definition is identical to update", func() {
				It("returns error", func() {
					err := ef.ApproveChaincodeDefinitionForOrg("my-channel", "cc-name", testDefinition, "hash", fakePublicState, fakeOrgState)
					Expect(err).To(MatchError("attempted to redefine uncommitted sequence (5) for namespace cc-name with unchanged content"))
				})
			})

			Context("when uncommitted definition has update of only package ID", func() {
				It("succeeds", func() {
					err := ef.ApproveChaincodeDefinitionForOrg("my-channel", "cc-name", testDefinition, "hash2", fakePublicState, fakeOrgState)
					Expect(err).NotTo(HaveOccurred())
				})
			})

			Context("when deserializing chaincode-source metadata fails", func() {
				BeforeEach(func() {
					fakeOrgKVStore["chaincode-sources/metadata/cc-name#5"] = []byte("garbage")
				})

				It("wraps and returns the error", func() {
					err := ef.ApproveChaincodeDefinitionForOrg("my-channel", "cc-name", testDefinition, "hash", fakePublicState, fakeOrgState)
					Expect(err).To(Not(BeNil()))
					Expect(err.Error()).To(HavePrefix("could not deserialize chaincode-source metadata for cc-name#5: could not unmarshal metadata for namespace chaincode-sources/cc-name#5"))
				})
			})

			Context("when deserializing chaincode package fails", func() {
				BeforeEach(func() {
					fakeOrgKVStore["chaincode-sources/fields/cc-name#5/PackageID"] = []byte("garbage")
				})

				It("wraps and returns the error", func() {
					err := ef.ApproveChaincodeDefinitionForOrg("my-channel", "cc-name", testDefinition, "hash", fakePublicState, fakeOrgState)
					Expect(err).To(Not(BeNil()))
					Expect(err.Error()).To(HavePrefix("could not deserialize chaincode package for cc-name#5: could not unmarshal state for key chaincode-sources/fields/cc-name#5/PackageID"))
				})
			})
		})

		Context("when the definition is for an expired sequence number", func() {
			BeforeEach(func() {
				testDefinition.Sequence = 3
			})

			It("fails", func() {
				err := ef.ApproveChaincodeDefinitionForOrg("my-channel", "cc-name", testDefinition, "hash", fakePublicState, fakeOrgState)
				Expect(err).To(MatchError("currently defined sequence 4 is larger than requested sequence 3"))
			})
		})

		Context("when the definition is for a distant sequence number", func() {
			BeforeEach(func() {
				testDefinition.Sequence = 9
			})

			It("fails", func() {
				err := ef.ApproveChaincodeDefinitionForOrg("my-channel", "cc-name", testDefinition, "hash", fakePublicState, fakeOrgState)
				Expect(err).To(MatchError("requested sequence 9 is larger than the next available sequence number 5"))
			})
		})

		Context("when querying the public state fails", func() {
			BeforeEach(func() {
				fakePublicState.GetStateReturns(nil, fmt.Errorf("get-state-error"))
			})

			It("wraps and returns the error", func() {
				err := ef.ApproveChaincodeDefinitionForOrg("my-channel", "cc-name", testDefinition, "hash", fakePublicState, fakeOrgState)
				Expect(err).To(MatchError("could not get current sequence: could not get state for key namespaces/fields/cc-name/Sequence: get-state-error"))
			})
		})

		Context("when writing to the org state fails for the parameters", func() {
			BeforeEach(func() {
				fakeOrgState.PutStateReturns(fmt.Errorf("put-state-error"))
			})

			It("wraps and returns the error", func() {
				err := ef.ApproveChaincodeDefinitionForOrg("my-channel", "cc-name", testDefinition, "hash", fakePublicState, fakeOrgState)
				Expect(err).To(MatchError("could not serialize chaincode parameters to state: could not write key into state: put-state-error"))
			})
		})

		Context("when writing to the org state fails for the package", func() {
			BeforeEach(func() {
				fakeOrgState.PutStateReturnsOnCall(4, fmt.Errorf("put-state-error"))
			})

			It("wraps and returns the error", func() {
				err := ef.ApproveChaincodeDefinitionForOrg("my-channel", "cc-name", testDefinition, "hash", fakePublicState, fakeOrgState)
				Expect(err).To(MatchError("could not serialize chaincode package info to state: could not write key into state: put-state-error"))
			})
		})
	})

	Describe("CheckCommitReadiness", func() {
		var (
			fakePublicState *mock.ReadWritableState
			fakeOrgStates   []*mock.ReadWritableState

			testDefinition *lifecycle.ChaincodeDefinition

			publicKVS, org0KVS, org1KVS MapLedgerShim
		)

		BeforeEach(func() {
			testDefinition = &lifecycle.ChaincodeDefinition{
				Sequence: 5,
				EndorsementInfo: &lb.ChaincodeEndorsementInfo{
					Version:           "version",
					EndorsementPlugin: "endorsement-plugin",
				},
				ValidationInfo: &lb.ChaincodeValidationInfo{
					ValidationPlugin:    "validation-plugin",
					ValidationParameter: []byte("validation-parameter"),
				},
			}

			publicKVS = MapLedgerShim(map[string][]byte{})
			fakePublicState = &mock.ReadWritableState{}
			fakePublicState.GetStateStub = publicKVS.GetState
			fakePublicState.PutStateStub = publicKVS.PutState

			resources.Serializer.Serialize("namespaces", "cc-name", &lifecycle.ChaincodeDefinition{
				Sequence: 4,
				EndorsementInfo: &lb.ChaincodeEndorsementInfo{
					Version:           "version",
					EndorsementPlugin: "endorsement-plugin",
				},
				ValidationInfo: &lb.ChaincodeValidationInfo{
					ValidationPlugin:    "validation-plugin",
					ValidationParameter: []byte("validation-parameter"),
				},
			}, publicKVS)

			org0KVS = MapLedgerShim(map[string][]byte{})
			org1KVS = MapLedgerShim(map[string][]byte{})
			fakeOrg0State := &mock.ReadWritableState{}
			fakeOrg0State.CollectionNameReturns("_implicit_org_org0")
			fakeOrg1State := &mock.ReadWritableState{}
			fakeOrg1State.CollectionNameReturns("_implicit_org_org1")
			fakeOrgStates = []*mock.ReadWritableState{
				fakeOrg0State,
				fakeOrg1State,
			}
			for i, kvs := range []MapLedgerShim{org0KVS, org1KVS} {
				kvs := kvs
				fakeOrgStates[i].GetStateStub = kvs.GetState
				fakeOrgStates[i].GetStateHashStub = kvs.GetStateHash
				fakeOrgStates[i].PutStateStub = kvs.PutState
			}

			resources.Serializer.Serialize("namespaces", "cc-name#5", testDefinition.Parameters(), fakeOrgStates[0])
			resources.Serializer.Serialize("namespaces", "cc-name#5", &lifecycle.ChaincodeParameters{}, fakeOrgStates[1])
		})

		It("checks the commit readiness of a chaincode definition and returns the approvals", func() {
			approvals, err := ef.CheckCommitReadiness("my-channel", "cc-name", testDefinition, fakePublicState, []lifecycle.OpaqueState{fakeOrgStates[0], fakeOrgStates[1]})
			Expect(err).NotTo(HaveOccurred())
			Expect(approvals).To(Equal(map[string]bool{
				"org0": true,
				"org1": false,
			}))
		})

		Context("when IsSerialized fails", func() {
			BeforeEach(func() {
				fakeOrgStates[0].GetStateHashReturns(nil, errors.New("bad bad failure"))
			})

			It("wraps and returns an error", func() {
				_, err := ef.CheckCommitReadiness("my-channel", "cc-name", testDefinition, fakePublicState, []lifecycle.OpaqueState{fakeOrgStates[0], fakeOrgStates[1]})
				Expect(err).To(MatchError(ContainSubstring("serialization check failed for key cc-name#5: could not get value for key namespaces/metadata/cc-name#5: bad bad failure")))
			})
		})

		Context("when the peer sets defaults", func() {
			BeforeEach(func() {
				testDefinition.EndorsementInfo.EndorsementPlugin = "escc"
				testDefinition.ValidationInfo.ValidationPlugin = "vscc"
				testDefinition.ValidationInfo.ValidationParameter = protoutil.MarshalOrPanic(
					&pb.ApplicationPolicy{
						Type: &pb.ApplicationPolicy_ChannelConfigPolicyReference{
							ChannelConfigPolicyReference: "/Channel/Application/Endorsement",
						},
					})

				resources.Serializer.Serialize("namespaces", "cc-name#5", testDefinition.Parameters(), fakeOrgStates[0])

				testDefinition.EndorsementInfo.EndorsementPlugin = ""
				testDefinition.ValidationInfo.ValidationPlugin = ""
				testDefinition.ValidationInfo.ValidationParameter = nil

				resources.Serializer.Serialize("namespaces", "cc-name#5", testDefinition.Parameters(), fakeOrgStates[1])
			})

			It("applies the chaincode definition and returns the approvals", func() {
				approvals, err := ef.CheckCommitReadiness("my-channel", "cc-name", testDefinition, fakePublicState, []lifecycle.OpaqueState{fakeOrgStates[0], fakeOrgStates[1]})
				Expect(err).NotTo(HaveOccurred())
				Expect(approvals).To(Equal(map[string]bool{
					"org0": true,
					"org1": false,
				}))
			})

			Context("when no default endorsement policy is defined on the channel", func() {
				BeforeEach(func() {
					fakePolicyManager.GetPolicyReturns(nil, false)
				})

				It("returns an error", func() {
					_, err := ef.CheckCommitReadiness("my-channel", "cc-name", testDefinition, fakePublicState, []lifecycle.OpaqueState{fakeOrgStates[0], fakeOrgStates[1]})
					Expect(err).To(MatchError(ContainSubstring("could not set defaults for chaincode definition in " +
						"channel my-channel: policy '/Channel/Application/Endorsement' must be defined " +
						"for channel 'my-channel' before chaincode operations can be attempted")))
				})
			})

			Context("when obtaining a stable channel config fails", func() {
				BeforeEach(func() {
					fakeChannelConfigSource.GetStableChannelConfigReturns(nil)
				})

				It("returns an error", func() {
					_, err := ef.CheckCommitReadiness("my-channel", "cc-name", testDefinition, fakePublicState, []lifecycle.OpaqueState{fakeOrgStates[0], fakeOrgStates[1]})
					Expect(err).To(MatchError(ContainSubstring("could not get channel config for channel 'my-channel'")))
				})
			})

			Context("when the public state is not readable", func() {
				BeforeEach(func() {
					fakePublicState.GetStateReturns(nil, fmt.Errorf("getstate-error"))
				})

				It("wraps and returns the error", func() {
					_, err := ef.CheckCommitReadiness("my-channel", "cc-name", testDefinition, fakePublicState, []lifecycle.OpaqueState{fakeOrgStates[0], fakeOrgStates[1]})
					Expect(err).To(MatchError("could not get current sequence: could not get state for key namespaces/fields/cc-name/Sequence: getstate-error"))
				})
			})

			Context("when the current sequence is not immediately prior to the new", func() {
				BeforeEach(func() {
					resources.Serializer.Serialize("namespaces", "cc-name", &lifecycle.ChaincodeDefinition{
						Sequence: 3,
						EndorsementInfo: &lb.ChaincodeEndorsementInfo{
							Version:           "version",
							EndorsementPlugin: "endorsement-plugin",
						},
						ValidationInfo: &lb.ChaincodeValidationInfo{
							ValidationPlugin:    "validation-plugin",
							ValidationParameter: []byte("validation-parameter"),
						},
					}, fakePublicState)
				})

				It("returns an error", func() {
					_, err := ef.CheckCommitReadiness("my-channel", "cc-name", testDefinition, fakePublicState, []lifecycle.OpaqueState{fakeOrgStates[0], fakeOrgStates[1]})
					Expect(err).To(MatchError("requested sequence is 5, but new definition must be sequence 4"))
				})
			})
		})
	})

	Describe("QueryApprovedChaincodeDefinition", func() {
		var (
			fakePublicState *mock.ReadWritableState
			fakeOrgStates   []*mock.ReadWritableState

			testDefinition            *lifecycle.ChaincodeDefinition
			testChaincodeLocalPackage *lifecycle.ChaincodeLocalPackage

			publicKVS, org0KVS, org1KVS MapLedgerShim
		)

		BeforeEach(func() {
			testDefinition = &lifecycle.ChaincodeDefinition{
				Sequence: 4,
				EndorsementInfo: &lb.ChaincodeEndorsementInfo{
					Version:           "version",
					EndorsementPlugin: "endorsement-plugin",
				},
				ValidationInfo: &lb.ChaincodeValidationInfo{
					ValidationPlugin:    "validation-plugin",
					ValidationParameter: []byte("validation-parameter"),
				},
			}

			testChaincodeLocalPackage = &lifecycle.ChaincodeLocalPackage{
				PackageID: "hash",
			}

			publicKVS = MapLedgerShim(map[string][]byte{})
			fakePublicState = &mock.ReadWritableState{}
			fakePublicState.GetStateStub = publicKVS.GetState
			fakePublicState.PutStateStub = publicKVS.PutState

			resources.Serializer.Serialize("namespaces", "cc-name", testDefinition, publicKVS)

			org0KVS = MapLedgerShim(map[string][]byte{})
			org1KVS = MapLedgerShim(map[string][]byte{})
			fakeOrg0State := &mock.ReadWritableState{}
			fakeOrg0State.CollectionNameReturns("_implicit_org_org0")
			fakeOrg1State := &mock.ReadWritableState{}
			fakeOrg1State.CollectionNameReturns("_implicit_org_org1")
			fakeOrgStates = []*mock.ReadWritableState{
				fakeOrg0State,
				fakeOrg1State,
			}
			for i, kvs := range []MapLedgerShim{org0KVS, org1KVS} {
				kvs := kvs
				fakeOrgStates[i].GetStateStub = kvs.GetState
				fakeOrgStates[i].GetStateHashStub = kvs.GetStateHash
				fakeOrgStates[i].PutStateStub = kvs.PutState
			}

			resources.Serializer.Serialize("namespaces", "cc-name#3", testDefinition.Parameters(), fakeOrgStates[0])
			resources.Serializer.Serialize("chaincode-sources", "cc-name#3", testChaincodeLocalPackage, fakeOrgStates[0])
			resources.Serializer.Serialize("namespaces", "cc-name#3", testDefinition.Parameters(), fakeOrgStates[1])
			resources.Serializer.Serialize("chaincode-sources", "cc-name#3", testChaincodeLocalPackage, fakeOrgStates[1])

			resources.Serializer.Serialize("namespaces", "cc-name#4", testDefinition.Parameters(), fakeOrgStates[0])
			resources.Serializer.Serialize("chaincode-sources", "cc-name#4", testChaincodeLocalPackage, fakeOrgStates[0])
			resources.Serializer.Serialize("namespaces", "cc-name#4", testDefinition.Parameters(), fakeOrgStates[1])
			resources.Serializer.Serialize("chaincode-sources", "cc-name#4", testChaincodeLocalPackage, fakeOrgStates[1])
		})

		It("returns the approved chaincode", func() {
			cc, err := ef.QueryApprovedChaincodeDefinition("my-channel", "cc-name", 3, fakePublicState, fakeOrgStates[0])
			Expect(err).NotTo(HaveOccurred())
			Expect(cc.Sequence).To(Equal(int64(3)))
			Expect(proto.Equal(cc.EndorsementInfo, &lb.ChaincodeEndorsementInfo{
				Version:           "version",
				EndorsementPlugin: "endorsement-plugin",
			})).To(BeTrue())
			Expect(proto.Equal(cc.ValidationInfo, &lb.ChaincodeValidationInfo{
				ValidationPlugin:    "validation-plugin",
				ValidationParameter: []byte("validation-parameter"),
			})).To(BeTrue())
			Expect(proto.Equal(cc.Collections, &pb.CollectionConfigPackage{})).To(BeTrue())
			Expect(proto.Equal(cc.Source, &lb.ChaincodeSource{
				Type: &lb.ChaincodeSource_LocalPackage{
					LocalPackage: &lb.ChaincodeSource_Local{
						PackageId: "hash",
					},
				},
			})).To(BeTrue())
		})

		Context("when the next sequence is not defined and the sequence argument is not provided", func() {
			It("returns the approved chaincode for the current sequence", func() {
				cc, err := ef.QueryApprovedChaincodeDefinition("my-channel", "cc-name", 0, fakePublicState, fakeOrgStates[0])
				Expect(err).NotTo(HaveOccurred())
				Expect(cc.Sequence).To(Equal(int64(4)))
				Expect(proto.Equal(cc.EndorsementInfo, &lb.ChaincodeEndorsementInfo{
					Version:           "version",
					EndorsementPlugin: "endorsement-plugin",
				})).To(BeTrue())
				Expect(proto.Equal(cc.ValidationInfo, &lb.ChaincodeValidationInfo{
					ValidationPlugin:    "validation-plugin",
					ValidationParameter: []byte("validation-parameter"),
				})).To(BeTrue())
				Expect(proto.Equal(cc.Collections, &pb.CollectionConfigPackage{})).To(BeTrue())
				Expect(proto.Equal(cc.Source, &lb.ChaincodeSource{
					Type: &lb.ChaincodeSource_LocalPackage{
						LocalPackage: &lb.ChaincodeSource_Local{
							PackageId: "hash",
						},
					},
				})).To(BeTrue())
			})
		})

		Context("when the next sequence is defined and the sequence argument is not provided", func() {
			BeforeEach(func() {
				testChaincodeLocalPackage.PackageID = ""
				resources.Serializer.Serialize("namespaces", "cc-name#5", testDefinition.Parameters(), fakeOrgStates[0])
				resources.Serializer.Serialize("chaincode-sources", "cc-name#5", testChaincodeLocalPackage, fakeOrgStates[0])
			})

			It("returns the approved chaincode for the next sequence", func() {
				cc, err := ef.QueryApprovedChaincodeDefinition("my-channel", "cc-name", 0, fakePublicState, fakeOrgStates[0])
				Expect(err).NotTo(HaveOccurred())
				Expect(cc.Sequence).To(Equal(int64(5)))
				Expect(proto.Equal(cc.EndorsementInfo, &lb.ChaincodeEndorsementInfo{
					Version:           "version",
					EndorsementPlugin: "endorsement-plugin",
				})).To(BeTrue())
				Expect(proto.Equal(cc.ValidationInfo, &lb.ChaincodeValidationInfo{
					ValidationPlugin:    "validation-plugin",
					ValidationParameter: []byte("validation-parameter"),
				})).To(BeTrue())
				Expect(proto.Equal(cc.Collections, &pb.CollectionConfigPackage{})).To(BeTrue())
				Expect(proto.Equal(cc.Source, &lb.ChaincodeSource{
					Type: &lb.ChaincodeSource_Unavailable_{
						Unavailable: &lb.ChaincodeSource_Unavailable{},
					},
				})).To(BeTrue())
			})

			Context("and deserializing namespace metadata for next sequence fails", func() {
				BeforeEach(func() {
					org0KVS["namespaces/metadata/cc-name#5"] = []byte("garbage")
				})

				It("wraps and returns the error", func() {
					cc, err := ef.QueryApprovedChaincodeDefinition("my-channel", "cc-name", 0, fakePublicState, fakeOrgStates[0])
					Expect(err).To(Not(BeNil()))
					Expect(err.Error()).To(HavePrefix("could not deserialize namespace metadata for next sequence 5: could not unmarshal metadata for namespace namespaces/cc-name#5"))
					Expect(cc).To(BeNil())
				})
			})
		})

		Context("when the next sequence is defined and the sequence argument is provided", func() {
			BeforeEach(func() {
				testChaincodeLocalPackage.PackageID = "hash"
				resources.Serializer.Serialize("namespaces", "cc-name#5", testDefinition.Parameters(), fakeOrgStates[0])
				resources.Serializer.Serialize("chaincode-sources", "cc-name#5", testChaincodeLocalPackage, fakeOrgStates[0])
			})

			It("returns the approved chaincode for the next sequence", func() {
				cc, err := ef.QueryApprovedChaincodeDefinition("my-channel", "cc-name", 5, fakePublicState, fakeOrgStates[0])
				Expect(err).NotTo(HaveOccurred())
				Expect(cc.Sequence).To(Equal(int64(5)))
				Expect(proto.Equal(cc.EndorsementInfo, &lb.ChaincodeEndorsementInfo{
					Version:           "version",
					EndorsementPlugin: "endorsement-plugin",
				})).To(BeTrue())
				Expect(proto.Equal(cc.ValidationInfo, &lb.ChaincodeValidationInfo{
					ValidationPlugin:    "validation-plugin",
					ValidationParameter: []byte("validation-parameter"),
				})).To(BeTrue())
				Expect(proto.Equal(cc.Collections, &pb.CollectionConfigPackage{})).To(BeTrue())
				Expect(proto.Equal(cc.Source, &lb.ChaincodeSource{
					Type: &lb.ChaincodeSource_LocalPackage{
						LocalPackage: &lb.ChaincodeSource_Local{
							PackageId: "hash",
						},
					},
				})).To(BeTrue())
			})
		})

		Context("when the requested sequence number is out of range", func() {
			It("wraps and returns the error", func() {
				cc, err := ef.QueryApprovedChaincodeDefinition("my-channel", "cc-name", 5, fakePublicState, fakeOrgStates[0])
				Expect(err).To(MatchError("could not fetch approved chaincode definition (name: 'cc-name', sequence: '5') on channel 'my-channel'"))
				Expect(cc).To(BeNil())
			})
		})

		Context("when the sequence argument is not provided and querying the public state fails", func() {
			BeforeEach(func() {
				fakePublicState.GetStateReturns(nil, fmt.Errorf("get-state-error"))
			})

			It("wraps and returns the error", func() {
				cc, err := ef.QueryApprovedChaincodeDefinition("my-channel", "cc-name", 0, fakePublicState, fakeOrgStates[0])
				Expect(err).To(MatchError("could not get current sequence: could not get state for key namespaces/fields/cc-name/Sequence: get-state-error"))
				Expect(cc).To(BeNil())
			})
		})

		Context("when deserializing namespace metadata for requested sequence fails", func() {
			BeforeEach(func() {
				org0KVS["namespaces/metadata/cc-name#4"] = []byte("garbage")
			})

			It("wraps and returns the error", func() {
				cc, err := ef.QueryApprovedChaincodeDefinition("my-channel", "cc-name", 4, fakePublicState, fakeOrgStates[0])
				Expect(err).To(Not(BeNil()))
				Expect(err.Error()).To(HavePrefix("could not deserialize namespace metadata for cc-name#4: could not unmarshal metadata for namespace namespaces/cc-name#4"))
				Expect(cc).To(BeNil())
			})
		})

		Context("when deserializing chaincode parameters fails", func() {
			BeforeEach(func() {
				org0KVS["namespaces/fields/cc-name#4/EndorsementInfo"] = []byte("garbage")
			})

			It("wraps and returns the error", func() {
				cc, err := ef.QueryApprovedChaincodeDefinition("my-channel", "cc-name", 4, fakePublicState, fakeOrgStates[0])
				Expect(err).To(Not(BeNil()))
				Expect(err.Error()).To(HavePrefix("could not deserialize chaincode parameters for cc-name#4: could not unmarshal state for key namespaces/fields/cc-name#4/EndorsementInfo"))
				Expect(cc).To(BeNil())
			})
		})

		Context("when deserializing chaincode-source metadata fails", func() {
			BeforeEach(func() {
				org0KVS["chaincode-sources/metadata/cc-name#4"] = []byte("garbage")
			})

			It("wraps and returns the error", func() {
				cc, err := ef.QueryApprovedChaincodeDefinition("my-channel", "cc-name", 4, fakePublicState, fakeOrgStates[0])
				Expect(err).To(Not(BeNil()))
				Expect(err.Error()).To(HavePrefix("could not deserialize chaincode-source metadata for cc-name#4: could not unmarshal metadata for namespace chaincode-sources/cc-name#4"))
				Expect(cc).To(BeNil())
			})
		})

		Context("when the requested cc parameters are found but the cc source is not found due to some inconsistency", func() {
			BeforeEach(func() {
				resources.Serializer.Serialize("namespaces", "cc-name#5", testDefinition.Parameters(), fakeOrgStates[0])
			})

			It("wraps and returns the error", func() {
				cc, err := ef.QueryApprovedChaincodeDefinition("my-channel", "cc-name", 5, fakePublicState, fakeOrgStates[0])
				Expect(err).To(MatchError("could not fetch approved chaincode definition (name: 'cc-name', sequence: '5') on channel 'my-channel'"))
				Expect(cc).To(BeNil())
			})
		})

		Context("when deserializing chaincode package fails", func() {
			BeforeEach(func() {
				org0KVS["chaincode-sources/fields/cc-name#4/PackageID"] = []byte("garbage")
			})

			It("wraps and returns the error", func() {
				cc, err := ef.QueryApprovedChaincodeDefinition("my-channel", "cc-name", 4, fakePublicState, fakeOrgStates[0])
				Expect(err).To(Not(BeNil()))
				Expect(err.Error()).To(HavePrefix("could not deserialize chaincode package for cc-name#4: could not unmarshal state for key chaincode-sources/fields/cc-name#4/PackageID"))
				Expect(cc).To(BeNil())
			})
		})

		Context("when the metadata is not for chaincode parameters", func() {
			BeforeEach(func() {
				type badStruct struct{}
				resources.Serializer.Serialize("namespaces", "cc-name#4", &badStruct{}, fakeOrgStates[0])
			})

			It("wraps and returns the error", func() {
				cc, err := ef.QueryApprovedChaincodeDefinition("my-channel", "cc-name", 4, fakePublicState, fakeOrgStates[0])
				Expect(err).To(MatchError("not a chaincode parameters type: badStruct"))
				Expect(cc).To(BeNil())
			})
		})

		Context("when the metadata is not for a chaincode local package", func() {
			BeforeEach(func() {
				type badStruct struct{}
				resources.Serializer.Serialize("chaincode-sources", "cc-name#4", &badStruct{}, fakeOrgStates[0])
			})

			It("wraps and returns the error", func() {
				cc, err := ef.QueryApprovedChaincodeDefinition("my-channel", "cc-name", 4, fakePublicState, fakeOrgStates[0])
				Expect(err).To(MatchError("not a chaincode local package type: badStruct"))
				Expect(cc).To(BeNil())
			})
		})
	})

	Describe("CommitChaincodeDefinition", func() {
		var (
			fakePublicState *mock.ReadWritableState
			fakeOrgStates   []*mock.ReadWritableState

			testDefinition *lifecycle.ChaincodeDefinition

			publicKVS, org0KVS, org1KVS MapLedgerShim
		)

		BeforeEach(func() {
			testDefinition = &lifecycle.ChaincodeDefinition{
				Sequence: 5,
				EndorsementInfo: &lb.ChaincodeEndorsementInfo{
					Version:           "version",
					EndorsementPlugin: "endorsement-plugin",
				},
				ValidationInfo: &lb.ChaincodeValidationInfo{
					ValidationPlugin:    "validation-plugin",
					ValidationParameter: []byte("validation-parameter"),
				},
			}

			publicKVS = MapLedgerShim(map[string][]byte{})
			fakePublicState = &mock.ReadWritableState{}
			fakePublicState.GetStateStub = publicKVS.GetState
			fakePublicState.PutStateStub = publicKVS.PutState

			resources.Serializer.Serialize("namespaces", "cc-name", &lifecycle.ChaincodeDefinition{
				Sequence: 4,
				EndorsementInfo: &lb.ChaincodeEndorsementInfo{
					Version:           "version",
					EndorsementPlugin: "endorsement-plugin",
				},
				ValidationInfo: &lb.ChaincodeValidationInfo{
					ValidationPlugin:    "validation-plugin",
					ValidationParameter: []byte("validation-parameter"),
				},
			}, publicKVS)

			org0KVS = MapLedgerShim(map[string][]byte{})
			org1KVS = MapLedgerShim(map[string][]byte{})
			fakeOrg0State := &mock.ReadWritableState{}
			fakeOrg0State.CollectionNameReturns("_implicit_org_org0")
			fakeOrg1State := &mock.ReadWritableState{}
			fakeOrg1State.CollectionNameReturns("_implicit_org_org1")
			fakeOrgStates = []*mock.ReadWritableState{
				fakeOrg0State,
				fakeOrg1State,
			}
			for i, kvs := range []MapLedgerShim{org0KVS, org1KVS} {
				kvs := kvs
				fakeOrgStates[i].GetStateStub = kvs.GetState
				fakeOrgStates[i].GetStateHashStub = kvs.GetStateHash
				fakeOrgStates[i].PutStateStub = kvs.PutState
			}

			resources.Serializer.Serialize("namespaces", "cc-name#5", testDefinition.Parameters(), fakeOrgStates[0])
			resources.Serializer.Serialize("namespaces", "cc-name#5", &lifecycle.ChaincodeParameters{}, fakeOrgStates[1])
		})

		It("applies the chaincode definition and returns the approvals", func() {
			approvals, err := ef.CommitChaincodeDefinition("my-channel", "cc-name", testDefinition, fakePublicState, []lifecycle.OpaqueState{fakeOrgStates[0], fakeOrgStates[1]})
			Expect(err).NotTo(HaveOccurred())
			Expect(approvals).To(Equal(map[string]bool{
				"org0": true,
				"org1": false,
			}))
		})

		Context("when IsSerialized fails", func() {
			BeforeEach(func() {
				fakeOrgStates[0].GetStateHashReturns(nil, errors.New("bad bad failure"))
			})

			It("wraps and returns an error", func() {
				_, err := ef.CommitChaincodeDefinition("my-channel", "cc-name", testDefinition, fakePublicState, []lifecycle.OpaqueState{fakeOrgStates[0], fakeOrgStates[1]})
				Expect(err).To(MatchError(ContainSubstring("serialization check failed for key cc-name#5: could not get value for key namespaces/metadata/cc-name#5: bad bad failure")))
			})
		})

		Context("when the peer sets defaults", func() {
			BeforeEach(func() {
				testDefinition.EndorsementInfo.EndorsementPlugin = "escc"
				testDefinition.ValidationInfo.ValidationPlugin = "vscc"
				testDefinition.ValidationInfo.ValidationParameter = protoutil.MarshalOrPanic(
					&pb.ApplicationPolicy{
						Type: &pb.ApplicationPolicy_ChannelConfigPolicyReference{
							ChannelConfigPolicyReference: "/Channel/Application/Endorsement",
						},
					})

				resources.Serializer.Serialize("namespaces", "cc-name#5", testDefinition.Parameters(), fakeOrgStates[0])

				testDefinition.EndorsementInfo.EndorsementPlugin = ""
				testDefinition.ValidationInfo.ValidationPlugin = ""
				testDefinition.ValidationInfo.ValidationParameter = nil

				resources.Serializer.Serialize("namespaces", "cc-name#5", testDefinition.Parameters(), fakeOrgStates[1])
			})

			It("applies the chaincode definition and returns the approvals", func() {
				approvals, err := ef.CommitChaincodeDefinition("my-channel", "cc-name", testDefinition, fakePublicState, []lifecycle.OpaqueState{fakeOrgStates[0], fakeOrgStates[1]})
				Expect(err).NotTo(HaveOccurred())
				Expect(approvals).To(Equal(map[string]bool{
					"org0": true,
					"org1": false,
				}))
			})

			Context("when no default endorsement policy is defined on thc channel", func() {
				BeforeEach(func() {
					fakePolicyManager.GetPolicyReturns(nil, false)
				})

				It("returns an error", func() {
					_, err := ef.CommitChaincodeDefinition("my-channel", "cc-name", testDefinition, fakePublicState, []lifecycle.OpaqueState{fakeOrgStates[0], fakeOrgStates[1]})
					Expect(err).To(MatchError(ContainSubstring("could not set defaults for chaincode definition in " +
						"channel my-channel: policy '/Channel/Application/Endorsement' must be defined " +
						"for channel 'my-channel' before chaincode operations can be attempted")))
				})
			})

			Context("when obtaining a stable channel config fails", func() {
				BeforeEach(func() {
					fakeChannelConfigSource.GetStableChannelConfigReturns(nil)
				})

				It("returns an error", func() {
					_, err := ef.CommitChaincodeDefinition("my-channel", "cc-name", testDefinition, fakePublicState, []lifecycle.OpaqueState{fakeOrgStates[0], fakeOrgStates[1]})
					Expect(err).To(MatchError(ContainSubstring("could not get channel config for channel 'my-channel'")))
				})
			})
		})

		Context("when the public state is not readable", func() {
			BeforeEach(func() {
				fakePublicState.GetStateReturns(nil, fmt.Errorf("getstate-error"))
			})

			It("wraps and returns the error", func() {
				_, err := ef.CommitChaincodeDefinition("my-channel", "cc-name", testDefinition, fakePublicState, []lifecycle.OpaqueState{fakeOrgStates[0], fakeOrgStates[1]})
				Expect(err).To(MatchError("could not get current sequence: could not get state for key namespaces/fields/cc-name/Sequence: getstate-error"))
			})
		})

		Context("when the public state is not writable", func() {
			BeforeEach(func() {
				fakePublicState.PutStateReturns(fmt.Errorf("putstate-error"))
			})

			It("wraps and returns the error", func() {
				_, err := ef.CommitChaincodeDefinition("my-channel", "cc-name", testDefinition, fakePublicState, []lifecycle.OpaqueState{fakeOrgStates[0], fakeOrgStates[1]})
				Expect(err).To(MatchError("could not serialize chaincode definition: could not write key into state: putstate-error"))
			})
		})

		Context("when the current sequence is not immediately prior to the new", func() {
			BeforeEach(func() {
				resources.Serializer.Serialize("namespaces", "cc-name", &lifecycle.ChaincodeDefinition{
					Sequence: 3,
					EndorsementInfo: &lb.ChaincodeEndorsementInfo{
						Version:           "version",
						EndorsementPlugin: "endorsement-plugin",
					},
					ValidationInfo: &lb.ChaincodeValidationInfo{
						ValidationPlugin:    "validation-plugin",
						ValidationParameter: []byte("validation-parameter"),
					},
				}, fakePublicState)
			})

			It("returns an error", func() {
				_, err := ef.CommitChaincodeDefinition("my-channel", "cc-name", testDefinition, fakePublicState, []lifecycle.OpaqueState{fakeOrgStates[0], fakeOrgStates[1]})
				Expect(err).To(MatchError("requested sequence is 5, but new definition must be sequence 4"))
			})
		})

		Context("when the current sequence is the new sequence", func() {
			BeforeEach(func() {
				resources.Serializer.Serialize("namespaces", "cc-name", &lifecycle.ChaincodeDefinition{
					Sequence: 5,
					EndorsementInfo: &lb.ChaincodeEndorsementInfo{
						Version:           "version",
						EndorsementPlugin: "endorsement-plugin",
					},
					ValidationInfo: &lb.ChaincodeValidationInfo{
						ValidationPlugin:    "validation-plugin",
						ValidationParameter: []byte("validation-parameter"),
					},
				}, fakePublicState)
			})

			It("returns an error", func() {
				_, err := ef.CommitChaincodeDefinition("my-channel", "cc-name", testDefinition, fakePublicState, []lifecycle.OpaqueState{fakeOrgStates[0], fakeOrgStates[1]})
				Expect(err).To(MatchError("requested sequence is 5, but new definition must be sequence 6"))
			})
		})
	})

	Describe("QueryChaincodeDefinition", func() {
		var (
			fakePublicState *mock.ReadWritableState
			fakeOrgStates   []*mock.ReadWritableState

			testDefinition *lifecycle.ChaincodeDefinition

			publicKVS, org0KVS, org1KVS MapLedgerShim
		)

		BeforeEach(func() {
			publicKVS = MapLedgerShim(map[string][]byte{})
			fakePublicState = &mock.ReadWritableState{}
			fakePublicState.GetStateStub = publicKVS.GetState
			fakePublicState.PutStateStub = publicKVS.PutState

			testDefinition = &lifecycle.ChaincodeDefinition{
				Sequence: 4,
				EndorsementInfo: &lb.ChaincodeEndorsementInfo{
					Version:           "version",
					EndorsementPlugin: "endorsement-plugin",
				},
				ValidationInfo: &lb.ChaincodeValidationInfo{
					ValidationPlugin:    "validation-plugin",
					ValidationParameter: []byte("validation-parameter"),
				},
				Collections: &pb.CollectionConfigPackage{},
			}

			resources.Serializer.Serialize("namespaces", "cc-name", testDefinition, publicKVS)

			org0KVS = MapLedgerShim(map[string][]byte{})
			org1KVS = MapLedgerShim(map[string][]byte{})
			fakeOrg0State := &mock.ReadWritableState{}
			fakeOrg1State := &mock.ReadWritableState{}
			fakeOrgStates = []*mock.ReadWritableState{
				fakeOrg0State,
				fakeOrg1State,
			}
			for i, kvs := range []MapLedgerShim{org0KVS, org1KVS} {
				kvs := kvs
				fakeOrgStates[i].GetStateStub = kvs.GetState
				fakeOrgStates[i].GetStateHashStub = kvs.GetStateHash
				fakeOrgStates[i].PutStateStub = kvs.PutState
			}

			resources.Serializer.Serialize("namespaces", "cc-name#4", testDefinition.Parameters(), fakeOrgStates[0])
			resources.Serializer.Serialize("namespaces", "cc-name#4", &lifecycle.ChaincodeParameters{}, fakeOrgStates[1])
		})

		It("returns the defined chaincode", func() {
			cc, err := ef.QueryChaincodeDefinition("cc-name", fakePublicState)
			Expect(err).NotTo(HaveOccurred())
			Expect(cc.Sequence).To(Equal(int64(4)))
			Expect(proto.Equal(cc.EndorsementInfo, &lb.ChaincodeEndorsementInfo{
				Version:           "version",
				EndorsementPlugin: "endorsement-plugin",
			})).To(BeTrue())
			Expect(proto.Equal(cc.ValidationInfo, &lb.ChaincodeValidationInfo{
				ValidationPlugin:    "validation-plugin",
				ValidationParameter: []byte("validation-parameter"),
			})).To(BeTrue())
			Expect(proto.Equal(cc.Collections, &pb.CollectionConfigPackage{})).To(BeTrue())
		})

		Context("when the chaincode is not defined", func() {
			BeforeEach(func() {
				fakePublicState.GetStateReturns(nil, nil)
			})

			It("returns an error", func() {
				cc, err := ef.QueryChaincodeDefinition("cc-name", fakePublicState)
				Expect(err).To(MatchError("namespace cc-name is not defined"))
				Expect(cc).To(BeNil())
			})
		})

		Context("when getting the metadata fails", func() {
			BeforeEach(func() {
				fakePublicState.GetStateReturns(nil, fmt.Errorf("metadata-error"))
			})

			It("returns an error", func() {
				cc, err := ef.QueryChaincodeDefinition("cc-name", fakePublicState) //, nil)
				Expect(err).To(MatchError("could not fetch metadata for namespace cc-name: could not query metadata for namespace namespaces/cc-name: metadata-error"))
				Expect(cc).To(BeNil())
			})
		})

		Context("when deserializing the definition fails", func() {
			BeforeEach(func() {
				publicKVS["namespaces/fields/cc-name/EndorsementInfo"] = []byte("garbage")
			})

			It("returns an error", func() {
				cc, err := ef.QueryChaincodeDefinition("cc-name", fakePublicState)
				Expect(err).To(Not(BeNil()))
				Expect(err.Error()).To(HavePrefix("could not deserialize namespace cc-name as chaincode: could not unmarshal state for key namespaces/fields/cc-name/EndorsementInfo"))
				Expect(cc).To(BeNil())
			})
		})
	})

	Describe("QueryOrgApprovals", func() {
		var (
			fakeOrgStates []*mock.ReadWritableState

			testDefinition *lifecycle.ChaincodeDefinition

			org0KVS, org1KVS MapLedgerShim
		)

		BeforeEach(func() {
			testDefinition = &lifecycle.ChaincodeDefinition{
				Sequence: 4,
				EndorsementInfo: &lb.ChaincodeEndorsementInfo{
					Version:           "version",
					EndorsementPlugin: "endorsement-plugin",
				},
				ValidationInfo: &lb.ChaincodeValidationInfo{
					ValidationPlugin:    "validation-plugin",
					ValidationParameter: []byte("validation-parameter"),
				},
				Collections: &pb.CollectionConfigPackage{},
			}

			org0KVS = MapLedgerShim(map[string][]byte{})
			org1KVS = MapLedgerShim(map[string][]byte{})
			fakeOrg0State := &mock.ReadWritableState{}
			fakeOrg0State.CollectionNameReturns("_implicit_org_org0")
			fakeOrg1State := &mock.ReadWritableState{}
			fakeOrg1State.CollectionNameReturns("_implicit_org_org1")
			fakeOrgStates = []*mock.ReadWritableState{
				fakeOrg0State,
				fakeOrg1State,
			}
			for i, kvs := range []MapLedgerShim{org0KVS, org1KVS} {
				kvs := kvs
				fakeOrgStates[i].GetStateStub = kvs.GetState
				fakeOrgStates[i].GetStateHashStub = kvs.GetStateHash
				fakeOrgStates[i].PutStateStub = kvs.PutState
			}

			resources.Serializer.Serialize("namespaces", "cc-name#4", testDefinition.Parameters(), fakeOrgStates[0])
			resources.Serializer.Serialize("namespaces", "cc-name#4", &lifecycle.ChaincodeParameters{}, fakeOrgStates[1])
		})

		It("returns the org approvals", func() {
			approvals, err := ef.QueryOrgApprovals("cc-name", testDefinition, []lifecycle.OpaqueState{fakeOrgStates[0], fakeOrgStates[1]})
			Expect(err).NotTo(HaveOccurred())
			Expect(approvals).To(Equal(map[string]bool{
				"org0": true,
				"org1": false,
			}))
		})

		Context("when the org state cannot be deserialized", func() {
			BeforeEach(func() {
				fakeOrgStates[0].GetStateHashReturns(nil, errors.New("owww that hurt"))
			})

			It("wraps and returns an error", func() {
				approvals, err := ef.QueryOrgApprovals("cc-name", testDefinition, []lifecycle.OpaqueState{fakeOrgStates[0], fakeOrgStates[1]})
				Expect(err).To(MatchError("serialization check failed for key cc-name#4: could not get value for key namespaces/metadata/cc-name#4: owww that hurt"))
				Expect(approvals).To(BeNil())
			})
		})
	})

	Describe("QueryNamespaceDefinitions", func() {
		var (
			fakePublicState *mock.ReadWritableState

			publicKVS MapLedgerShim
		)

		BeforeEach(func() {
			publicKVS = MapLedgerShim(map[string][]byte{})
			fakePublicState = &mock.ReadWritableState{}
			fakePublicState.GetStateStub = publicKVS.GetState
			fakePublicState.GetStateRangeStub = publicKVS.GetStateRange
			resources.Serializer.Serialize("namespaces", "cc-name", &lifecycle.ChaincodeDefinition{}, publicKVS)
			resources.Serializer.Serialize("namespaces", "other-name", &lifecycle.ChaincodeParameters{}, publicKVS)
		})

		It("returns the defined namespaces", func() {
			result, err := ef.QueryNamespaceDefinitions(fakePublicState)
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
				_, err := ef.QueryNamespaceDefinitions(fakePublicState)
				Expect(err).To(MatchError("could not query namespace metadata: could not get state range for namespace namespaces: state-range-error"))
			})
		})
	})
})
