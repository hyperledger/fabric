/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle_test

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/chaincode"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle/mock"
	"github.com/hyperledger/fabric/core/chaincode/persistence"
	persistenceintf "github.com/hyperledger/fabric/core/chaincode/persistence/intf"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/dispatcher"
	cb "github.com/hyperledger/fabric/protos/common"
	lb "github.com/hyperledger/fabric/protos/peer/lifecycle"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("SCC", func() {
	var (
		scc                     *lifecycle.SCC
		fakeSCCFuncs            *mock.SCCFunctions
		fakeChannelConfigSource *mock.ChannelConfigSource
		fakeChannelConfig       *mock.ChannelConfig
		fakeApplicationConfig   *mock.ApplicationConfig
		fakeCapabilities        *mock.ApplicationCapabilities
		fakeACLProvider         *mock.ACLProvider
	)

	BeforeEach(func() {
		fakeSCCFuncs = &mock.SCCFunctions{}
		fakeChannelConfigSource = &mock.ChannelConfigSource{}
		fakeChannelConfig = &mock.ChannelConfig{}
		fakeChannelConfigSource.GetStableChannelConfigReturns(fakeChannelConfig)
		fakeApplicationConfig = &mock.ApplicationConfig{}
		fakeChannelConfig.ApplicationConfigReturns(fakeApplicationConfig, true)
		fakeCapabilities = &mock.ApplicationCapabilities{}
		fakeCapabilities.LifecycleV20Returns(true)
		fakeApplicationConfig.CapabilitiesReturns(fakeCapabilities)
		fakeACLProvider = &mock.ACLProvider{}
		scc = &lifecycle.SCC{
			Dispatcher: &dispatcher.Dispatcher{
				Protobuf: &dispatcher.ProtobufImpl{},
			},
			Functions:           fakeSCCFuncs,
			OrgMSPID:            "fake-mspid",
			ChannelConfigSource: fakeChannelConfigSource,
			ACLProvider:         fakeACLProvider,
		}
	})

	Describe("Name", func() {
		It("returns the name", func() {
			Expect(scc.Name()).To(Equal("_lifecycle"))
		})
	})

	Describe("Path", func() {
		It("returns the path", func() {
			Expect(scc.Path()).To(Equal("github.com/hyperledger/fabric/core/chaincode/lifecycle"))
		})
	})

	Describe("InitArgs", func() {
		It("returns no args", func() {
			Expect(scc.InitArgs()).To(BeNil())
		})
	})

	Describe("Chaincode", func() {
		It("returns a reference to itself", func() {
			Expect(scc.Chaincode()).To(Equal(scc))
		})
	})

	Describe("InvokableExternal", func() {
		It("is invokable externally", func() {
			Expect(scc.InvokableExternal()).To(BeTrue())
		})
	})

	Describe("InvokableCC2CC", func() {
		It("is invokable chaincode to chaincode", func() {
			Expect(scc.InvokableCC2CC()).To(BeTrue())
		})
	})

	Describe("Enabled", func() {
		It("is enabled", func() {
			Expect(scc.Enabled()).To(BeTrue())
		})
	})

	Describe("Init", func() {
		It("does nothing", func() {
			Expect(scc.Init(nil)).To(Equal(shim.Success(nil)))
		})
	})

	Describe("Invoke", func() {
		var (
			fakeStub *mock.ChaincodeStub
		)

		BeforeEach(func() {
			fakeStub = &mock.ChaincodeStub{}
			fakeStub.GetChannelIDReturns("test-channel")
		})

		Context("when no arguments are provided", func() {
			It("returns an error", func() {
				Expect(scc.Invoke(fakeStub)).To(Equal(shim.Error("lifecycle scc must be invoked with arguments")))
			})
		})

		Context("when too many arguments are provided", func() {
			BeforeEach(func() {
				fakeStub.GetArgsReturns([][]byte{nil, nil, nil})
			})

			It("returns an error", func() {
				Expect(scc.Invoke(fakeStub)).To(Equal(shim.Error("lifecycle scc operations require exactly two arguments but received 3")))
			})
		})

		Context("when an unknown function is provided as the first argument", func() {
			BeforeEach(func() {
				fakeStub.GetArgsReturns([][]byte{[]byte("bad-function"), nil})
			})

			It("returns an error", func() {
				Expect(scc.Invoke(fakeStub)).To(Equal(shim.Error("failed to invoke backing implementation of 'bad-function': receiver *lifecycle.Invocation.bad-function does not exist")))
			})
		})

		Context("when the ACL provider disapproves of the function", func() {
			BeforeEach(func() {
				fakeStub.GetArgsReturns([][]byte{[]byte("any-function"), nil})
				fakeACLProvider.CheckACLReturns(fmt.Errorf("acl-error"))
			})

			It("returns an error", func() {
				Expect(scc.Invoke(fakeStub)).To(Equal(shim.Error("Failed to authorize invocation due to failed ACL check: acl-error")))
			})

			Context("when the signed data for the tx cannot be retrieved", func() {
				BeforeEach(func() {
					fakeStub.GetSignedProposalReturns(nil, fmt.Errorf("shim-error"))
				})

				It("returns an error", func() {
					Expect(scc.Invoke(fakeStub)).To(Equal(shim.Error("Failed getting signed proposal from stub: [shim-error]")))
				})
			})
		})

		Describe("InstallChaincode", func() {
			var (
				arg          *lb.InstallChaincodeArgs
				marshaledArg []byte
			)

			BeforeEach(func() {
				fakeStub.GetChannelIDReturns("")

				arg = &lb.InstallChaincodeArgs{
					ChaincodeInstallPackage: []byte("chaincode-package"),
				}

				var err error
				marshaledArg, err = proto.Marshal(arg)
				Expect(err).NotTo(HaveOccurred())

				fakeStub.GetArgsReturns([][]byte{[]byte("InstallChaincode"), marshaledArg})

				fakeSCCFuncs.InstallChaincodeReturns(&chaincode.InstalledChaincode{
					Label:     "label",
					PackageID: persistenceintf.PackageID("packageid"),
				}, nil)
			})

			It("passes the arguments to and returns the results from the backing scc function implementation", func() {
				res := scc.Invoke(fakeStub)
				Expect(res.Status).To(Equal(int32(200)))
				payload := &lb.InstallChaincodeResult{}
				err := proto.Unmarshal(res.Payload, payload)
				Expect(err).NotTo(HaveOccurred())
				Expect(payload.PackageId).To(Equal("packageid"))

				Expect(fakeSCCFuncs.InstallChaincodeCallCount()).To(Equal(1))
				ccInstallPackage := fakeSCCFuncs.InstallChaincodeArgsForCall(0)
				Expect(ccInstallPackage).To(Equal([]byte("chaincode-package")))
			})

			Context("when the underlying function implementation fails", func() {
				BeforeEach(func() {
					fakeSCCFuncs.InstallChaincodeReturns(nil, fmt.Errorf("underlying-error"))
				})

				It("wraps and returns the error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'InstallChaincode': underlying-error"))
				})
			})
		})

		Describe("QueryInstalledChaincode", func() {
			var (
				arg          *lb.QueryInstalledChaincodeArgs
				marshaledArg []byte
			)

			BeforeEach(func() {
				arg = &lb.QueryInstalledChaincodeArgs{
					PackageId: "awesome_package",
				}

				var err error
				marshaledArg, err = proto.Marshal(arg)
				Expect(err).NotTo(HaveOccurred())

				fakeStub.GetArgsReturns([][]byte{[]byte("QueryInstalledChaincode"), marshaledArg})

				fakeSCCFuncs.QueryInstalledChaincodeReturns(&chaincode.InstalledChaincode{
					PackageID: persistenceintf.PackageID("awesome_package"),
					Label:     "awesome_package_label",
				}, nil)
			})

			It("passes the arguments to and returns the results from the backing scc function implementation", func() {
				res := scc.Invoke(fakeStub)
				Expect(res.Status).To(Equal(int32(200)))
				payload := &lb.QueryInstalledChaincodeResult{}
				err := proto.Unmarshal(res.Payload, payload)
				Expect(err).NotTo(HaveOccurred())
				Expect(payload.Label).To(Equal("awesome_package_label"))

				Expect(fakeSCCFuncs.QueryInstalledChaincodeCallCount()).To(Equal(1))
				name := fakeSCCFuncs.QueryInstalledChaincodeArgsForCall(0)
				Expect(name).To(Equal(persistenceintf.PackageID("awesome_package")))
			})

			Context("when the code package cannot be found", func() {
				BeforeEach(func() {
					fakeSCCFuncs.QueryInstalledChaincodeReturns(nil, persistence.CodePackageNotFoundErr{PackageID: persistenceintf.PackageID("less_awesome_package")})
				})

				It("returns 404 Not Found", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(404)))
					Expect(res.Message).To(Equal("chaincode install package 'less_awesome_package' not found"))
				})
			})

			Context("when the underlying function implementation fails", func() {
				BeforeEach(func() {
					fakeSCCFuncs.QueryInstalledChaincodeReturns(nil, fmt.Errorf("underlying-error"))
				})

				It("wraps and returns the error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'QueryInstalledChaincode': underlying-error"))
				})
			})
		})

		Describe("QueryInstalledChaincodes", func() {
			var (
				arg          *lb.QueryInstalledChaincodesArgs
				marshaledArg []byte
			)

			BeforeEach(func() {
				arg = &lb.QueryInstalledChaincodesArgs{}

				var err error
				marshaledArg, err = proto.Marshal(arg)
				Expect(err).NotTo(HaveOccurred())

				fakeStub.GetArgsReturns([][]byte{[]byte("QueryInstalledChaincodes"), marshaledArg})

				fakeSCCFuncs.QueryInstalledChaincodesReturns([]chaincode.InstalledChaincode{
					{
						Hash:      []byte("cc0-hash"),
						Label:     "cc0-label",
						PackageID: persistenceintf.PackageID("cc0-package-id"),
					},
					{
						Hash:      []byte("cc1-hash"),
						Label:     "cc1-label",
						PackageID: persistenceintf.PackageID("cc1-package-id"),
					},
				}, nil)
			})

			It("passes the arguments to and returns the results from the backing scc function implementation", func() {
				res := scc.Invoke(fakeStub)
				Expect(res.Status).To(Equal(int32(200)))
				payload := &lb.QueryInstalledChaincodesResult{}
				err := proto.Unmarshal(res.Payload, payload)
				Expect(err).NotTo(HaveOccurred())

				Expect(payload.InstalledChaincodes).To(HaveLen(2))

				Expect(payload.InstalledChaincodes[0].Label).To(Equal("cc0-label"))
				Expect(payload.InstalledChaincodes[0].PackageId).To(Equal("cc0-package-id"))

				Expect(payload.InstalledChaincodes[1].Label).To(Equal("cc1-label"))
				Expect(payload.InstalledChaincodes[1].PackageId).To(Equal("cc1-package-id"))

				Expect(fakeSCCFuncs.QueryInstalledChaincodesCallCount()).To(Equal(1))
			})

			Context("when the underlying function implementation fails", func() {
				BeforeEach(func() {
					fakeSCCFuncs.QueryInstalledChaincodesReturns(nil, fmt.Errorf("underlying-error"))
				})

				It("wraps and returns the error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'QueryInstalledChaincodes': underlying-error"))
				})
			})
		})

		Describe("ApproveChaincodeDefinitionForMyOrg", func() {
			var (
				err          error
				arg          *lb.ApproveChaincodeDefinitionForMyOrgArgs
				marshaledArg []byte
			)

			BeforeEach(func() {
				arg = &lb.ApproveChaincodeDefinitionForMyOrgArgs{
					Sequence:            7,
					Name:                "cc_name",
					Version:             "version_1.0",
					EndorsementPlugin:   "endorsement-plugin",
					ValidationPlugin:    "validation-plugin",
					ValidationParameter: []byte("validation-parameter"),
					Collections: &cb.CollectionConfigPackage{
						Config: []*cb.CollectionConfig{
							{
								Payload: &cb.CollectionConfig_StaticCollectionConfig{
									StaticCollectionConfig: &cb.StaticCollectionConfig{
										Name: "test-collection",
									},
								},
							},
						},
					},
					InitRequired: true,
					Source: &lb.ChaincodeSource{
						Type: &lb.ChaincodeSource_LocalPackage{
							LocalPackage: &lb.ChaincodeSource_Local{
								PackageId: "hash",
							},
						},
					},
				}

				marshaledArg, err = proto.Marshal(arg)
				Expect(err).NotTo(HaveOccurred())

				fakeStub.GetArgsReturns([][]byte{[]byte("ApproveChaincodeDefinitionForMyOrg"), marshaledArg})
			})

			It("passes the arguments to and returns the results from the backing scc function implementation", func() {
				res := scc.Invoke(fakeStub)
				Expect(res.Status).To(Equal(int32(200)))
				payload := &lb.ApproveChaincodeDefinitionForMyOrgResult{}
				err = proto.Unmarshal(res.Payload, payload)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeSCCFuncs.ApproveChaincodeDefinitionForOrgCallCount()).To(Equal(1))
				chname, ccname, cd, packageID, pubState, privState := fakeSCCFuncs.ApproveChaincodeDefinitionForOrgArgsForCall(0)
				Expect(chname).To(Equal("test-channel"))
				Expect(ccname).To(Equal("cc_name"))
				Expect(cd).To(Equal(&lifecycle.ChaincodeDefinition{
					Sequence: 7,
					EndorsementInfo: &lb.ChaincodeEndorsementInfo{
						Version:           "version_1.0",
						EndorsementPlugin: "endorsement-plugin",
						InitRequired:      true,
					},
					ValidationInfo: &lb.ChaincodeValidationInfo{
						ValidationPlugin:    "validation-plugin",
						ValidationParameter: []byte("validation-parameter"),
					},
					Collections: &cb.CollectionConfigPackage{
						Config: []*cb.CollectionConfig{
							{
								Payload: &cb.CollectionConfig_StaticCollectionConfig{
									StaticCollectionConfig: &cb.StaticCollectionConfig{
										Name: "test-collection",
									},
								},
							},
						},
					},
				}))
				Expect(packageID).To(Equal(persistenceintf.PackageID("hash")))
				Expect(pubState).To(Equal(fakeStub))
				Expect(privState).To(BeAssignableToTypeOf(&lifecycle.ChaincodePrivateLedgerShim{}))
				Expect(privState.(*lifecycle.ChaincodePrivateLedgerShim).Collection).To(Equal("_implicit_org_fake-mspid"))
			})

			Context("when the chaincode name contains invalid characters", func() {
				BeforeEach(func() {
					arg.Name = "!nvalid"

					marshaledArg, err = proto.Marshal(arg)
					Expect(err).NotTo(HaveOccurred())
					fakeStub.GetArgsReturns([][]byte{[]byte("ApproveChaincodeDefinitionForMyOrg"), marshaledArg})
				})

				It("wraps and returns the error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'ApproveChaincodeDefinitionForMyOrg': invalid chaincode name '!nvalid'. Names can only consist of alphanumerics, '_', and '-' and cannot begin with '_'"))
				})
			})

			Context("when the chaincode version contains invalid characters", func() {
				BeforeEach(func() {
					arg.Version = "$money$"

					marshaledArg, err = proto.Marshal(arg)
					Expect(err).NotTo(HaveOccurred())
					fakeStub.GetArgsReturns([][]byte{[]byte("ApproveChaincodeDefinitionForMyOrg"), marshaledArg})
				})

				It("wraps and returns the error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'ApproveChaincodeDefinitionForMyOrg': invalid chaincode version '$money$'. Versions can only consist of alphanumerics, '_', '-', '+', and '.'"))
				})
			})

			Context("when the chaincode name matches an existing system chaincode name", func() {
				BeforeEach(func() {
					arg.Name = "cscc"

					marshaledArg, err = proto.Marshal(arg)
					Expect(err).NotTo(HaveOccurred())
					fakeStub.GetArgsReturns([][]byte{[]byte("ApproveChaincodeDefinitionForMyOrg"), marshaledArg})
				})

				It("wraps and returns the error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'ApproveChaincodeDefinitionForMyOrg': chaincode name 'cscc' is the name of a system chaincode"))
				})
			})

			Context("when a collection name contains invalid characters", func() {
				BeforeEach(func() {
					arg.Collections = &cb.CollectionConfigPackage{
						Config: []*cb.CollectionConfig{
							{
								Payload: &cb.CollectionConfig_StaticCollectionConfig{
									StaticCollectionConfig: &cb.StaticCollectionConfig{
										Name: "collection@test",
									},
								},
							},
						},
					}

					marshaledArg, err = proto.Marshal(arg)
					Expect(err).NotTo(HaveOccurred())
					fakeStub.GetArgsReturns([][]byte{[]byte("ApproveChaincodeDefinitionForMyOrg"), marshaledArg})
				})

				It("wraps and returns the error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'ApproveChaincodeDefinitionForMyOrg': invalid collection name 'collection@test'. Names can only consist of alphanumerics, '_', and '-' and cannot begin with '_'"))
				})
			})

			Context("when a collection name begins with an invalid character", func() {
				BeforeEach(func() {
					arg.Collections = &cb.CollectionConfigPackage{
						Config: []*cb.CollectionConfig{
							{
								Payload: &cb.CollectionConfig_StaticCollectionConfig{
									StaticCollectionConfig: &cb.StaticCollectionConfig{
										Name: "_collection",
									},
								},
							},
						},
					}

					marshaledArg, err = proto.Marshal(arg)
					Expect(err).NotTo(HaveOccurred())
					fakeStub.GetArgsReturns([][]byte{[]byte("ApproveChaincodeDefinitionForMyOrg"), marshaledArg})
				})

				It("wraps and returns the error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'ApproveChaincodeDefinitionForMyOrg': invalid collection name '_collection'. Names can only consist of alphanumerics, '_', and '-' and cannot begin with '_'"))
				})
			})

			Context("when the underlying function implementation fails", func() {
				BeforeEach(func() {
					fakeSCCFuncs.ApproveChaincodeDefinitionForOrgReturns(fmt.Errorf("underlying-error"))
				})

				It("wraps and returns the error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'ApproveChaincodeDefinitionForMyOrg': underlying-error"))
				})
			})

			Context("when the lifecycle capability is not enabled", func() {
				BeforeEach(func() {
					fakeCapabilities.LifecycleV20Returns(false)
				})

				It("returns an error", func() {
					Expect(scc.Invoke(fakeStub)).To(Equal(shim.Error("cannot use new lifecycle for channel 'test-channel' as it does not have the required capabilities enabled")))
				})
			})
		})

		Describe("CommitChaincodeDefinition", func() {
			var (
				err            error
				arg            *lb.CommitChaincodeDefinitionArgs
				marshaledArg   []byte
				fakeOrgConfigs []*mock.ApplicationOrgConfig
			)

			BeforeEach(func() {
				arg = &lb.CommitChaincodeDefinitionArgs{
					Sequence:            7,
					Name:                "cc-name2",
					Version:             "version-2+2",
					EndorsementPlugin:   "endorsement-plugin",
					ValidationPlugin:    "validation-plugin",
					ValidationParameter: []byte("validation-parameter"),
					Collections: &cb.CollectionConfigPackage{
						Config: []*cb.CollectionConfig{
							{
								Payload: &cb.CollectionConfig_StaticCollectionConfig{
									StaticCollectionConfig: &cb.StaticCollectionConfig{
										Name: "test_collection",
									},
								},
							},
						},
					},
					InitRequired: true,
				}

				marshaledArg, err = proto.Marshal(arg)
				Expect(err).NotTo(HaveOccurred())

				fakeStub.GetArgsReturns([][]byte{[]byte("CommitChaincodeDefinition"), marshaledArg})

				fakeOrgConfigs = []*mock.ApplicationOrgConfig{{}, {}}
				fakeOrgConfigs[0].MSPIDReturns("fake-mspid")
				fakeOrgConfigs[1].MSPIDReturns("other-mspid")

				fakeApplicationConfig.OrganizationsReturns(map[string]channelconfig.ApplicationOrg{
					"org0": fakeOrgConfigs[0],
					"org1": fakeOrgConfigs[1],
				})

				fakeSCCFuncs.CommitChaincodeDefinitionReturns([]bool{true, true}, nil)
			})

			It("passes the arguments to and returns the results from the backing scc function implementation", func() {
				res := scc.Invoke(fakeStub)
				Expect(res.Message).To(Equal(""))
				Expect(res.Status).To(Equal(int32(200)))
				payload := &lb.CommitChaincodeDefinitionResult{}
				err = proto.Unmarshal(res.Payload, payload)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeSCCFuncs.CommitChaincodeDefinitionCallCount()).To(Equal(1))
				chname, ccname, cd, pubState, orgStates := fakeSCCFuncs.CommitChaincodeDefinitionArgsForCall(0)
				Expect(chname).To(Equal("test-channel"))
				Expect(ccname).To(Equal("cc-name2"))
				Expect(cd).To(Equal(&lifecycle.ChaincodeDefinition{
					Sequence: 7,
					EndorsementInfo: &lb.ChaincodeEndorsementInfo{
						Version:           "version-2+2",
						EndorsementPlugin: "endorsement-plugin",
						InitRequired:      true,
					},
					ValidationInfo: &lb.ChaincodeValidationInfo{
						ValidationPlugin:    "validation-plugin",
						ValidationParameter: []byte("validation-parameter"),
					},
					Collections: &cb.CollectionConfigPackage{
						Config: []*cb.CollectionConfig{
							{
								Payload: &cb.CollectionConfig_StaticCollectionConfig{
									StaticCollectionConfig: &cb.StaticCollectionConfig{
										Name: "test_collection",
									},
								},
							},
						},
					},
				}))
				Expect(pubState).To(Equal(fakeStub))
				Expect(len(orgStates)).To(Equal(2))
				Expect(orgStates[0]).To(BeAssignableToTypeOf(&lifecycle.ChaincodePrivateLedgerShim{}))
				Expect(orgStates[1]).To(BeAssignableToTypeOf(&lifecycle.ChaincodePrivateLedgerShim{}))
				collection0 := orgStates[0].(*lifecycle.ChaincodePrivateLedgerShim).Collection
				collection1 := orgStates[1].(*lifecycle.ChaincodePrivateLedgerShim).Collection
				Expect([]string{collection0, collection1}).To(ConsistOf("_implicit_org_fake-mspid", "_implicit_org_other-mspid"))
			})

			Context("when the chaincode name begins with an invalid character", func() {
				BeforeEach(func() {
					arg.Name = "_invalid"

					marshaledArg, err = proto.Marshal(arg)
					Expect(err).NotTo(HaveOccurred())
					fakeStub.GetArgsReturns([][]byte{[]byte("CommitChaincodeDefinition"), marshaledArg})
				})

				It("wraps and returns the error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'CommitChaincodeDefinition': invalid chaincode name '_invalid'. Names can only consist of alphanumerics, '_', and '-' and cannot begin with '_'"))
				})
			})

			Context("when the chaincode version contains invalid characters", func() {
				BeforeEach(func() {
					arg.Version = "$money$"

					marshaledArg, err = proto.Marshal(arg)
					Expect(err).NotTo(HaveOccurred())
					fakeStub.GetArgsReturns([][]byte{[]byte("CommitChaincodeDefinition"), marshaledArg})
				})

				It("wraps and returns the error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'CommitChaincodeDefinition': invalid chaincode version '$money$'. Versions can only consist of alphanumerics, '_', '-', '+', and '.'"))
				})
			})

			Context("when the chaincode name matches an existing system chaincode name", func() {
				BeforeEach(func() {
					arg.Name = "qscc"

					marshaledArg, err = proto.Marshal(arg)
					Expect(err).NotTo(HaveOccurred())
					fakeStub.GetArgsReturns([][]byte{[]byte("CommitChaincodeDefinition"), marshaledArg})
				})

				It("wraps and returns the error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'CommitChaincodeDefinition': chaincode name 'qscc' is the name of a system chaincode"))
				})
			})

			Context("when a collection name contains invalid characters", func() {
				BeforeEach(func() {
					arg.Collections = &cb.CollectionConfigPackage{
						Config: []*cb.CollectionConfig{
							{
								Payload: &cb.CollectionConfig_StaticCollectionConfig{
									StaticCollectionConfig: &cb.StaticCollectionConfig{
										Name: "collection(test",
									},
								},
							},
						},
					}

					marshaledArg, err = proto.Marshal(arg)
					Expect(err).NotTo(HaveOccurred())
					fakeStub.GetArgsReturns([][]byte{[]byte("CommitChaincodeDefinition"), marshaledArg})
				})

				It("wraps and returns the error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'CommitChaincodeDefinition': invalid collection name 'collection(test'. Names can only consist of alphanumerics, '_', and '-' and cannot begin with '_'"))
				})
			})

			Context("when a collection name begins with an invalid character", func() {
				BeforeEach(func() {
					arg.Collections = &cb.CollectionConfigPackage{
						Config: []*cb.CollectionConfig{
							{
								Payload: &cb.CollectionConfig_StaticCollectionConfig{
									StaticCollectionConfig: &cb.StaticCollectionConfig{
										Name: "&collection",
									},
								},
							},
						},
					}

					marshaledArg, err = proto.Marshal(arg)
					Expect(err).NotTo(HaveOccurred())
					fakeStub.GetArgsReturns([][]byte{[]byte("CommitChaincodeDefinition"), marshaledArg})
				})

				It("wraps and returns the error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'CommitChaincodeDefinition': invalid collection name '&collection'. Names can only consist of alphanumerics, '_', and '-' and cannot begin with '_'"))
				})
			})

			Context("when there is no agreement from this peer's org", func() {
				BeforeEach(func() {
					fakeSCCFuncs.CommitChaincodeDefinitionReturns([]bool{false, false}, nil)
				})

				It("returns an error indicating the lack of agreement", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'CommitChaincodeDefinition': chaincode definition not agreed to by this org (fake-mspid)"))
				})
			})

			Context("when there is no match for this peer's org's MSPID", func() {
				BeforeEach(func() {
					fakeOrgConfigs[0].MSPIDReturns("other-mspid")
				})

				It("returns an error indicating the lack of agreement", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'CommitChaincodeDefinition': impossibly, this peer's org is processing requests for a channel it is not a member of"))
				})
			})

			Context("when there is no channel config", func() {
				BeforeEach(func() {
					fakeChannelConfigSource.GetStableChannelConfigReturns(nil)
				})

				It("returns an error indicating the lack of agreement", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("could not get channelconfig for channel 'test-channel'"))
				})
			})

			Context("when there is no application config", func() {
				BeforeEach(func() {
					fakeChannelConfig.ApplicationConfigReturns(nil, false)
				})

				It("returns an error indicating the lack of agreement", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("could not get application config for channel 'test-channel'"))
				})

				Context("when there is no application config because there is no channel", func() {
					BeforeEach(func() {
						fakeStub.GetChannelIDReturns("")
					})

					It("returns an error indicating the lack of agreement", func() {
						res := scc.Invoke(fakeStub)
						Expect(res.Status).To(Equal(int32(500)))
						Expect(res.Message).To(Equal("failed to invoke backing implementation of 'CommitChaincodeDefinition': no application config for channel ''"))
					})
				})
			})

			Context("when the underlying function implementation fails", func() {
				BeforeEach(func() {
					fakeSCCFuncs.CommitChaincodeDefinitionReturns(nil, fmt.Errorf("underlying-error"))
				})

				It("wraps and returns the error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'CommitChaincodeDefinition': underlying-error"))
				})
			})
		})

		Describe("QueryApprovalStatus", func() {
			var (
				err            error
				arg            *lb.QueryApprovalStatusArgs
				marshaledArg   []byte
				fakeOrgConfigs []*mock.ApplicationOrgConfig
			)

			BeforeEach(func() {
				arg = &lb.QueryApprovalStatusArgs{
					Sequence:            7,
					Name:                "name",
					Version:             "version",
					EndorsementPlugin:   "endorsement-plugin",
					ValidationPlugin:    "validation-plugin",
					ValidationParameter: []byte("validation-parameter"),
					Collections:         &cb.CollectionConfigPackage{},
					InitRequired:        true,
				}

				marshaledArg, err = proto.Marshal(arg)
				Expect(err).NotTo(HaveOccurred())

				fakeStub.GetArgsReturns([][]byte{[]byte("QueryApprovalStatus"), marshaledArg})

				fakeOrgConfigs = []*mock.ApplicationOrgConfig{{}, {}}
				fakeOrgConfigs[0].MSPIDReturns("fake-mspid")
				fakeOrgConfigs[1].MSPIDReturns("other-mspid")

				fakeApplicationConfig.OrganizationsReturns(map[string]channelconfig.ApplicationOrg{
					"org0": fakeOrgConfigs[0],
					"org1": fakeOrgConfigs[1],
				})

				fakeSCCFuncs.QueryApprovalStatusReturns([]bool{true, true}, nil)
			})

			It("passes the arguments to and returns the results from the backing scc function implementation", func() {
				res := scc.Invoke(fakeStub)
				Expect(res.Message).To(Equal(""))
				Expect(res.Status).To(Equal(int32(200)))
				payload := &lb.QueryApprovalStatusResults{}
				err = proto.Unmarshal(res.Payload, payload)
				Expect(err).NotTo(HaveOccurred())

				orgApprovals := payload.GetApproved()
				Expect(orgApprovals).NotTo(BeNil())
				Expect(len(orgApprovals)).To(Equal(2))
				Expect(orgApprovals["fake-mspid"]).To(BeTrue())
				Expect(orgApprovals["other-mspid"]).To(BeTrue())

				Expect(fakeSCCFuncs.QueryApprovalStatusCallCount()).To(Equal(1))
				chname, ccname, cd, pubState, orgStates := fakeSCCFuncs.QueryApprovalStatusArgsForCall(0)
				Expect(chname).To(Equal("test-channel"))
				Expect(ccname).To(Equal("name"))
				Expect(cd).To(Equal(&lifecycle.ChaincodeDefinition{
					Sequence: 7,
					EndorsementInfo: &lb.ChaincodeEndorsementInfo{
						Version:           "version",
						EndorsementPlugin: "endorsement-plugin",
						InitRequired:      true,
					},
					ValidationInfo: &lb.ChaincodeValidationInfo{
						ValidationPlugin:    "validation-plugin",
						ValidationParameter: []byte("validation-parameter"),
					},
					Collections: arg.Collections,
				}))
				Expect(pubState).To(Equal(fakeStub))
				Expect(len(orgStates)).To(Equal(2))
				Expect(orgStates[0]).To(BeAssignableToTypeOf(&lifecycle.ChaincodePrivateLedgerShim{}))
				Expect(orgStates[1]).To(BeAssignableToTypeOf(&lifecycle.ChaincodePrivateLedgerShim{}))
				collection0 := orgStates[0].(*lifecycle.ChaincodePrivateLedgerShim).Collection
				collection1 := orgStates[1].(*lifecycle.ChaincodePrivateLedgerShim).Collection
				Expect([]string{collection0, collection1}).To(ConsistOf("_implicit_org_fake-mspid", "_implicit_org_other-mspid"))
			})

			Context("when there is no application config", func() {
				BeforeEach(func() {
					fakeChannelConfig.ApplicationConfigReturns(nil, false)
				})

				It("returns an error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("could not get application config for channel 'test-channel'"))
				})

				Context("when there is no application config because there is no channel", func() {
					BeforeEach(func() {
						fakeStub.GetChannelIDReturns("")
					})

					It("returns an error", func() {
						res := scc.Invoke(fakeStub)
						Expect(res.Status).To(Equal(int32(500)))
						Expect(res.Message).To(Equal("failed to invoke backing implementation of 'QueryApprovalStatus': no application config for channel ''"))
					})
				})
			})

			Context("when the underlying function implementation fails", func() {
				BeforeEach(func() {
					fakeSCCFuncs.QueryApprovalStatusReturns(nil, fmt.Errorf("underlying-error"))
				})

				It("wraps and returns the error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'QueryApprovalStatus': underlying-error"))
				})
			})
		})

		Describe("QueryChaincodeDefinition", func() {
			var (
				arg          *lb.QueryChaincodeDefinitionArgs
				marshaledArg []byte
			)

			BeforeEach(func() {
				arg = &lb.QueryChaincodeDefinitionArgs{
					Name: "cc-name",
				}

				var err error
				marshaledArg, err = proto.Marshal(arg)
				Expect(err).NotTo(HaveOccurred())

				fakeStub.GetArgsReturns([][]byte{[]byte("QueryChaincodeDefinition"), marshaledArg})
				fakeSCCFuncs.QueryChaincodeDefinitionReturns(&lifecycle.ChaincodeDefinition{
					Sequence: 2,
					EndorsementInfo: &lb.ChaincodeEndorsementInfo{
						Version:           "version",
						EndorsementPlugin: "endorsement-plugin",
					},
					ValidationInfo: &lb.ChaincodeValidationInfo{
						ValidationPlugin:    "validation-plugin",
						ValidationParameter: []byte("validation-parameter"),
					},
					Collections: &cb.CollectionConfigPackage{},
				}, nil)
			})

			It("passes the arguments to and returns the results from the backing scc function implementation", func() {
				res := scc.Invoke(fakeStub)
				Expect(res.Status).To(Equal(int32(200)))
				payload := &lb.QueryChaincodeDefinitionResult{}
				err := proto.Unmarshal(res.Payload, payload)
				Expect(err).NotTo(HaveOccurred())
				Expect(proto.Equal(payload, &lb.QueryChaincodeDefinitionResult{
					Sequence:            2,
					Version:             "version",
					EndorsementPlugin:   "endorsement-plugin",
					ValidationPlugin:    "validation-plugin",
					ValidationParameter: []byte("validation-parameter"),
					Collections:         &cb.CollectionConfigPackage{},
				})).To(BeTrue())

				Expect(fakeSCCFuncs.QueryChaincodeDefinitionCallCount()).To(Equal(1))
				name, pubState := fakeSCCFuncs.QueryChaincodeDefinitionArgsForCall(0)
				Expect(name).To(Equal("cc-name"))
				Expect(pubState).To(Equal(fakeStub))
			})

			Context("when the underlying function implementation fails", func() {
				BeforeEach(func() {
					fakeSCCFuncs.QueryChaincodeDefinitionReturns(nil, fmt.Errorf("underlying-error"))
				})

				It("wraps and returns the error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'QueryChaincodeDefinition': underlying-error"))
				})
			})

			Context("when the namespace cannot be found", func() {
				BeforeEach(func() {
					fakeSCCFuncs.QueryChaincodeDefinitionReturns(nil, lifecycle.ErrNamespaceNotDefined{Namespace: "nicetry"})
				})

				It("returns 404 Not Found", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(404)))
					Expect(res.Message).To(Equal("namespace nicetry is not defined"))
				})
			})
		})

		Describe("QueryNamespaceDefinitions", func() {
			var (
				arg          *lb.QueryNamespaceDefinitionsArgs
				marshaledArg []byte
			)

			BeforeEach(func() {
				arg = &lb.QueryNamespaceDefinitionsArgs{}

				var err error
				marshaledArg, err = proto.Marshal(arg)
				Expect(err).NotTo(HaveOccurred())

				fakeStub.GetArgsReturns([][]byte{[]byte("QueryNamespaceDefinitions"), marshaledArg})
				fakeSCCFuncs.QueryNamespaceDefinitionsReturns(map[string]string{
					"foo": "Chaincode",
					"bar": "Token",
				}, nil)
			})

			It("passes the arguments to and returns the results from the backing scc function implementation", func() {
				res := scc.Invoke(fakeStub)
				Expect(res.Status).To(Equal(int32(200)))
				payload := &lb.QueryNamespaceDefinitionsResult{}
				err := proto.Unmarshal(res.Payload, payload)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeSCCFuncs.QueryNamespaceDefinitionsCallCount()).To(Equal(1))
				Expect(fakeSCCFuncs.QueryNamespaceDefinitionsArgsForCall(0)).To(Equal(&lifecycle.ChaincodePublicLedgerShim{ChaincodeStubInterface: fakeStub}))
			})

			Context("when the underlying function implementation fails", func() {
				BeforeEach(func() {
					fakeSCCFuncs.QueryNamespaceDefinitionsReturns(nil, fmt.Errorf("underlying-error"))
				})

				It("wraps and returns the error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'QueryNamespaceDefinitions': underlying-error"))
				})
			})
		})
	})
})
