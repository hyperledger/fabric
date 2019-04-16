/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle_test

import (
	"fmt"

	"github.com/hyperledger/fabric/common/chaincode"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle/mock"
	"github.com/hyperledger/fabric/core/chaincode/persistence"
	p "github.com/hyperledger/fabric/core/chaincode/persistence/intf"
	persistenceintf "github.com/hyperledger/fabric/core/chaincode/persistence/intf"
	cb "github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	lb "github.com/hyperledger/fabric/protos/peer/lifecycle"
	"github.com/hyperledger/fabric/protoutil"

	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("ChaincodeParameters", func() {
	var (
		lhs, rhs *lifecycle.ChaincodeParameters
	)

	BeforeEach(func() {
		lhs = &lifecycle.ChaincodeParameters{
			EndorsementInfo: &lb.ChaincodeEndorsementInfo{},
			ValidationInfo:  &lb.ChaincodeValidationInfo{},
			Collections:     &cb.CollectionConfigPackage{},
		}

		rhs = &lifecycle.ChaincodeParameters{
			EndorsementInfo: &lb.ChaincodeEndorsementInfo{},
			ValidationInfo:  &lb.ChaincodeValidationInfo{},
			Collections:     &cb.CollectionConfigPackage{},
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
				Expect(lhs.Equal(rhs)).To(MatchError("EndorsementPlugin '' != 'different'"))
			})
		})

		Context("when the ValidationPlugin differs from the current definition", func() {
			BeforeEach(func() {
				rhs.ValidationInfo.ValidationPlugin = "different"
			})

			It("returns an error", func() {
				Expect(lhs.Equal(rhs)).To(MatchError("ValidationPlugin '' != 'different'"))
			})
		})

		Context("when the ValidationParameter differs from the current definition", func() {
			BeforeEach(func() {
				rhs.ValidationInfo.ValidationParameter = []byte("different")
			})

			It("returns an error", func() {
				Expect(lhs.Equal(rhs)).To(MatchError("ValidationParameter '' != '646966666572656e74'"))
			})
		})

		Context("when the Collections differ from the current definition", func() {
			BeforeEach(func() {
				rhs.Collections = &cb.CollectionConfigPackage{
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
				Expect(lhs.Equal(rhs)).To(MatchError("Collections do not match"))
			})
		})
	})
})

var _ = Describe("Resources", func() {
	var (
		resources *lifecycle.Resources
	)

	BeforeEach(func() {
		resources = &lifecycle.Resources{
			Serializer: &lifecycle.Serializer{},
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
				EndorsementInfo: &lb.ChaincodeEndorsementInfo{
					Version: "version",
				},
				ValidationInfo: &lb.ChaincodeValidationInfo{},
				Collections:    &cb.CollectionConfigPackage{},
			}, fakePublicState)
			Expect(err).NotTo(HaveOccurred())
			fakeReadableState = &mock.ReadWritableState{}
			fakeReadableState.GetStateStub = fakePublicState.GetState
		})

		It("returns that the chaincode is defined and the definition", func() {
			exists, definition, err := resources.ChaincodeDefinitionIfDefined("cc-name", fakeReadableState)
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeTrue())
			Expect(definition.EndorsementInfo.Version).To(Equal("version"))
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
})

var _ = Describe("ExternalFunctions", func() {
	var (
		resources               *lifecycle.Resources
		ef                      *lifecycle.ExternalFunctions
		fakeCCStore             *mock.ChaincodeStore
		fakeParser              *mock.PackageParser
		fakeListener            *mock.InstallListener
		fakeChannelConfigSource *mock.ChannelConfigSource
		fakeChannelConfig       *mock.ChannelConfig
		fakeApplicationConfig   *mock.ApplicationConfig
		fakeOrgConfigs          []*mock.ApplicationOrgConfig
		fakePolicyManager       *mock.PolicyManager
	)

	BeforeEach(func() {
		fakeCCStore = &mock.ChaincodeStore{}
		fakeParser = &mock.PackageParser{}
		fakeListener = &mock.InstallListener{}
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
			Resources:       resources,
			InstallListener: fakeListener,
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
			fakeCCStore.SaveReturns(p.PackageID("fake-hash"), nil)
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
			Expect(packageID).To(Equal(p.PackageID("fake-hash")))
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

	Describe("QueryInstalledChaincode", func() {
		BeforeEach(func() {
			fakeCCStore.LoadReturns([]byte("some stuff"), nil)
			fakeParser.ParseReturns(&persistence.ChaincodePackage{
				Metadata: &persistence.ChaincodePackageMetadata{
					Type:  "type",
					Label: "label",
					Path:  "path",
				},
			}, nil)
		})

		It("passes through to the backing chaincode store", func() {
			chaincodes, err := ef.QueryInstalledChaincode("packageid")
			Expect(err).NotTo(HaveOccurred())
			Expect(chaincodes).To(Equal(
				&chaincode.InstalledChaincode{
					PackageID: "packageid",
					Label:     "label",
				},
			))
		})

		Context("when loading the package fails", func() {
			BeforeEach(func() {
				fakeCCStore.LoadReturns(nil, errors.New("take a hike"))
			})

			It("returns an error", func() {
				_, err := ef.QueryInstalledChaincode("packageid")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("could not load chaincode with package id 'packageid': take a hike"))
			})
		})

		Context("when the code package cannot be found", func() {
			BeforeEach(func() {
				fakeCCStore.LoadReturns(nil, persistence.CodePackageNotFoundErr{PackageID: persistenceintf.PackageID("try hitchhiking")})
			})

			It("returns an error", func() {
				_, err := ef.QueryInstalledChaincode("packageid")
				Expect(err).To(MatchError("chaincode install package 'try hitchhiking' not found"))
			})
		})

		Context("when parsing the package fails", func() {
			BeforeEach(func() {
				fakeParser.ParseReturns(nil, errors.New("take a hike"))
			})

			It("returns an error", func() {
				_, err := ef.QueryInstalledChaincode("packageid")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("could not parse chaincode with package id 'packageid': take a hike"))
			})
		})

		Context("when the package does not have metadata", func() {
			BeforeEach(func() {
				fakeParser.ParseReturns(&persistence.ChaincodePackage{}, nil)
			})

			It("returns an error", func() {
				_, err := ef.QueryInstalledChaincode("packageid")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("empty metadata for chaincode with package id 'packageid'"))
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
					Hash:    []byte("cc1-hash"),
				},
				{
					Name:    "cc2-name",
					Version: "cc2-version",
					Hash:    []byte("cc2-hash"),
				},
			}

			fakeCCStore.ListInstalledChaincodesReturns(chaincodes, fmt.Errorf("fake-error"))
		})

		It("passes through to the backing chaincode store", func() {
			result, err := ef.QueryInstalledChaincodes()
			Expect(result).To(Equal(chaincodes))
			Expect(err).To(MatchError(fmt.Errorf("fake-error")))
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
				Collections: &cb.CollectionConfigPackage{},
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
			err := ef.ApproveChaincodeDefinitionForOrg("my-channel", "cc-name", testDefinition, p.PackageID("hash"), fakePublicState, fakeOrgState)
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
			Expect(proto.Equal(committedDefinition.Collections, &cb.CollectionConfigPackage{})).To(BeTrue())

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
				err := ef.ApproveChaincodeDefinitionForOrg("my-channel", "cc-name", testDefinition, p.PackageID("hash"), fakePublicState, fakeOrgState)
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
				Expect(proto.Equal(committedDefinition.Collections, &cb.CollectionConfigPackage{})).To(BeTrue())
			})

			Context("when no default endorsement policy is defined on thc channel", func() {
				BeforeEach(func() {
					fakePolicyManager.GetPolicyReturns(nil, false)
				})

				It("returns an error", func() {
					err := ef.ApproveChaincodeDefinitionForOrg("my-channel", "cc-name", testDefinition, p.PackageID("hash"), fakePublicState, fakeOrgState)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("could not set defaults for chaincode definition in channel my-channel: Policy '/Channel/Application/Endorsement' must be defined for channel 'my-channel' before chaincode operations can be attempted"))
				})
			})

			Context("when obtaining a stable channel config fails", func() {
				BeforeEach(func() {
					fakeChannelConfigSource.GetStableChannelConfigReturns(nil)
				})

				It("returns an error", func() {
					err := ef.ApproveChaincodeDefinitionForOrg("my-channel", "cc-name", testDefinition, p.PackageID("hash"), fakePublicState, fakeOrgState)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("could not get channel config for channel 'my-channel'"))
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

		Context("when the sequence number already has a definition", func() {
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
			})

			It("verifies that the definition matches before writing", func() {
				err := ef.ApproveChaincodeDefinitionForOrg("my-channel", "cc-name", testDefinition, "hash", fakePublicState, fakeOrgState)
				Expect(err).NotTo(HaveOccurred())
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
					Expect(err).To(MatchError("could not fetch metadata for current definition: could not unmarshal metadata for namespace namespaces/cc-name: proto: can't skip unknown wire type 7"))
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
					Expect(err).To(MatchError("attempted to define the current sequence (5) for namespace cc-name, but: Version 'other-version' != 'version'"))
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

	Describe("QueryApprovalStatus", func() {
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
			fakeOrgStates = []*mock.ReadWritableState{{}, {}}
			for i, kvs := range []MapLedgerShim{org0KVS, org1KVS} {
				kvs := kvs
				fakeOrgStates[i].GetStateStub = kvs.GetState
				fakeOrgStates[i].GetStateHashStub = kvs.GetStateHash
				fakeOrgStates[i].PutStateStub = kvs.PutState
			}

			resources.Serializer.Serialize("namespaces", "cc-name#5", testDefinition.Parameters(), fakeOrgStates[0])
			resources.Serializer.Serialize("namespaces", "cc-name#5", &lifecycle.ChaincodeParameters{}, fakeOrgStates[1])
		})

		It("applies the chaincode definition and returns the agreements", func() {
			agreements, err := ef.QueryApprovalStatus("my-channel", "cc-name", testDefinition, fakePublicState, []lifecycle.OpaqueState{fakeOrgStates[0], fakeOrgStates[1]})
			Expect(err).NotTo(HaveOccurred())
			Expect(agreements).To(Equal([]bool{true, false}))
		})

		Context("when IsSerialized fails", func() {
			BeforeEach(func() {
				fakeOrgStates[0].GetStateHashReturns(nil, errors.New("bad bad failure"))
			})

			It("wraps and returns an error", func() {
				_, err := ef.QueryApprovalStatus("my-channel", "cc-name", testDefinition, fakePublicState, []lifecycle.OpaqueState{fakeOrgStates[0], fakeOrgStates[1]})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("serialization check failed for key cc-name#5: could not get value for key namespaces/metadata/cc-name#5: bad bad failure"))
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

				fakeOrgStates = []*mock.ReadWritableState{{}, {}}
				for i, kvs := range []MapLedgerShim{org0KVS, org1KVS} {
					kvs := kvs
					fakeOrgStates[i].GetStateStub = kvs.GetState
					fakeOrgStates[i].GetStateHashStub = kvs.GetStateHash
					fakeOrgStates[i].PutStateStub = kvs.PutState
				}

				resources.Serializer.Serialize("namespaces", "cc-name#5", testDefinition.Parameters(), fakeOrgStates[0])

				testDefinition.EndorsementInfo.EndorsementPlugin = ""
				testDefinition.ValidationInfo.ValidationPlugin = ""
				testDefinition.ValidationInfo.ValidationParameter = nil

				resources.Serializer.Serialize("namespaces", "cc-name#5", testDefinition.Parameters(), fakeOrgStates[1])
			})

			It("applies the chaincode definition and returns the agreements", func() {
				agreements, err := ef.QueryApprovalStatus("my-channel", "cc-name", testDefinition, fakePublicState, []lifecycle.OpaqueState{fakeOrgStates[0], fakeOrgStates[1]})
				Expect(err).NotTo(HaveOccurred())
				Expect(agreements).To(Equal([]bool{true, false}))
			})

			Context("when no default endorsement policy is defined on thc channel", func() {
				BeforeEach(func() {
					fakePolicyManager.GetPolicyReturns(nil, false)
				})

				It("returns an error", func() {
					_, err := ef.QueryApprovalStatus("my-channel", "cc-name", testDefinition, fakePublicState, []lifecycle.OpaqueState{fakeOrgStates[0], fakeOrgStates[1]})
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("could not set defaults for chaincode definition in " +
						"channel my-channel: Policy '/Channel/Application/Endorsement' must be defined " +
						"for channel 'my-channel' before chaincode operations can be attempted"))
				})
			})

			Context("when obtaining a stable channel config fails", func() {
				BeforeEach(func() {
					fakeChannelConfigSource.GetStableChannelConfigReturns(nil)
				})

				It("returns an error", func() {
					_, err := ef.QueryApprovalStatus("my-channel", "cc-name", testDefinition, fakePublicState, []lifecycle.OpaqueState{fakeOrgStates[0], fakeOrgStates[1]})
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("could not get channel config for channel 'my-channel'"))

				})
			})

			Context("when the public state is not readable", func() {
				BeforeEach(func() {
					fakePublicState.GetStateReturns(nil, fmt.Errorf("getstate-error"))
				})

				It("wraps and returns the error", func() {
					_, err := ef.QueryApprovalStatus("my-channel", "cc-name", testDefinition, fakePublicState, []lifecycle.OpaqueState{fakeOrgStates[0], fakeOrgStates[1]})
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
					_, err := ef.QueryApprovalStatus("my-channel", "cc-name", testDefinition, fakePublicState, []lifecycle.OpaqueState{fakeOrgStates[0], fakeOrgStates[1]})
					Expect(err).To(MatchError("requested sequence is 5, but new definition must be sequence 4"))
				})
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
			fakeOrgStates = []*mock.ReadWritableState{{}, {}}
			for i, kvs := range []MapLedgerShim{org0KVS, org1KVS} {
				kvs := kvs
				fakeOrgStates[i].GetStateStub = kvs.GetState
				fakeOrgStates[i].GetStateHashStub = kvs.GetStateHash
				fakeOrgStates[i].PutStateStub = kvs.PutState
			}

			resources.Serializer.Serialize("namespaces", "cc-name#5", testDefinition.Parameters(), fakeOrgStates[0])
			resources.Serializer.Serialize("namespaces", "cc-name#5", &lifecycle.ChaincodeParameters{}, fakeOrgStates[1])
		})

		It("applies the chaincode definition and returns the agreements", func() {
			agreements, err := ef.CommitChaincodeDefinition("my-channel", "cc-name", testDefinition, fakePublicState, []lifecycle.OpaqueState{fakeOrgStates[0], fakeOrgStates[1]})
			Expect(err).NotTo(HaveOccurred())
			Expect(agreements).To(Equal([]bool{true, false}))
		})

		Context("when IsSerialized fails", func() {
			BeforeEach(func() {
				fakeOrgStates[0].GetStateHashReturns(nil, errors.New("bad bad failure"))
			})

			It("wraps and returns an error", func() {
				_, err := ef.CommitChaincodeDefinition("my-channel", "cc-name", testDefinition, fakePublicState, []lifecycle.OpaqueState{fakeOrgStates[0], fakeOrgStates[1]})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("serialization check failed for key cc-name#5: could not get value for key namespaces/metadata/cc-name#5: bad bad failure"))
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

				fakeOrgStates = []*mock.ReadWritableState{{}, {}}
				for i, kvs := range []MapLedgerShim{org0KVS, org1KVS} {
					kvs := kvs
					fakeOrgStates[i].GetStateStub = kvs.GetState
					fakeOrgStates[i].GetStateHashStub = kvs.GetStateHash
					fakeOrgStates[i].PutStateStub = kvs.PutState
				}

				resources.Serializer.Serialize("namespaces", "cc-name#5", testDefinition.Parameters(), fakeOrgStates[0])

				testDefinition.EndorsementInfo.EndorsementPlugin = ""
				testDefinition.ValidationInfo.ValidationPlugin = ""
				testDefinition.ValidationInfo.ValidationParameter = nil

				resources.Serializer.Serialize("namespaces", "cc-name#5", testDefinition.Parameters(), fakeOrgStates[1])
			})

			It("applies the chaincode definition and returns the agreements", func() {
				agreements, err := ef.CommitChaincodeDefinition("my-channel", "cc-name", testDefinition, fakePublicState, []lifecycle.OpaqueState{fakeOrgStates[0], fakeOrgStates[1]})
				Expect(err).NotTo(HaveOccurred())
				Expect(agreements).To(Equal([]bool{true, false}))
			})

			Context("when no default endorsement policy is defined on thc channel", func() {
				BeforeEach(func() {
					fakePolicyManager.GetPolicyReturns(nil, false)
				})

				It("returns an error", func() {
					_, err := ef.CommitChaincodeDefinition("my-channel", "cc-name", testDefinition, fakePublicState, []lifecycle.OpaqueState{fakeOrgStates[0], fakeOrgStates[1]})
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("could not set defaults for chaincode definition in " +
						"channel my-channel: Policy '/Channel/Application/Endorsement' must be defined " +
						"for channel 'my-channel' before chaincode operations can be attempted"))
				})
			})

			Context("when obtaining a stable channel config fails", func() {
				BeforeEach(func() {
					fakeChannelConfigSource.GetStableChannelConfigReturns(nil)
				})

				It("returns an error", func() {
					_, err := ef.CommitChaincodeDefinition("my-channel", "cc-name", testDefinition, fakePublicState, []lifecycle.OpaqueState{fakeOrgStates[0], fakeOrgStates[1]})
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("could not get channel config for channel 'my-channel'"))

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
	})

	Describe("QueryChaincodeDefinition", func() {
		var (
			fakePublicState *mock.ReadWritableState

			publicKVS MapLedgerShim
		)

		BeforeEach(func() {
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
		})

		It("returns the defined chaincode", func() {
			cc, err := ef.QueryChaincodeDefinition("cc-name", fakePublicState)
			Expect(err).NotTo(HaveOccurred())
			Expect(cc).To(Equal(&lifecycle.ChaincodeDefinition{
				Sequence: 4,
				EndorsementInfo: &lb.ChaincodeEndorsementInfo{
					Version:           "version",
					EndorsementPlugin: "endorsement-plugin",
				},
				ValidationInfo: &lb.ChaincodeValidationInfo{
					ValidationPlugin:    "validation-plugin",
					ValidationParameter: []byte("validation-parameter"),
				},
				Collections: &cb.CollectionConfigPackage{},
			}))
		})

		Context("when the chaincode is not defined", func() {
			BeforeEach(func() {
				fakePublicState.GetStateReturns(nil, nil)
			})

			It("returns an error", func() {
				_, err := ef.QueryChaincodeDefinition("cc-name", fakePublicState)
				Expect(err).To(MatchError("namespace cc-name is not defined"))
			})
		})

		Context("when getting the metadata fails", func() {
			BeforeEach(func() {
				fakePublicState.GetStateReturns(nil, fmt.Errorf("metadata-error"))
			})

			It("returns an error", func() {
				_, err := ef.QueryChaincodeDefinition("cc-name", fakePublicState)
				Expect(err).To(MatchError("could not fetch metadata for namespace cc-name: could not query metadata for namespace namespaces/cc-name: metadata-error"))
			})
		})

		Context("when deserializing the definition fails", func() {
			BeforeEach(func() {
				publicKVS["namespaces/fields/cc-name/EndorsementInfo"] = []byte("garbage")
			})

			It("returns an error", func() {
				_, err := ef.QueryChaincodeDefinition("cc-name", fakePublicState)
				Expect(err).To(MatchError("could not deserialize namespace cc-name as chaincode: could not unmarshal state for key namespaces/fields/cc-name/EndorsementInfo: proto: can't skip unknown wire type 7"))
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
