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
		scc = &lifecycle.SCC{
			Dispatcher: &dispatcher.Dispatcher{
				Protobuf: &dispatcher.ProtobufImpl{},
			},
			Functions:           fakeSCCFuncs,
			OrgMSPID:            "fake-mspid",
			ChannelConfigSource: fakeChannelConfigSource,
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

		Describe("InstallChaincode", func() {
			var (
				arg          *lb.InstallChaincodeArgs
				marshaledArg []byte
			)

			BeforeEach(func() {
				fakeStub.GetChannelIDReturns("")

				arg = &lb.InstallChaincodeArgs{
					Name:                    "name",
					Version:                 "version",
					ChaincodeInstallPackage: []byte("chaincode-package"),
				}

				var err error
				marshaledArg, err = proto.Marshal(arg)
				Expect(err).NotTo(HaveOccurred())

				fakeStub.GetArgsReturns([][]byte{[]byte("InstallChaincode"), marshaledArg})

				fakeSCCFuncs.InstallChaincodeReturns([]byte("fake-hash"), nil)
			})

			It("passes the arguments to and returns the results from the backing scc function implementation", func() {
				res := scc.Invoke(fakeStub)
				Expect(res.Status).To(Equal(int32(200)))
				payload := &lb.InstallChaincodeResult{}
				err := proto.Unmarshal(res.Payload, payload)
				Expect(err).NotTo(HaveOccurred())
				Expect(payload.Hash).To(Equal([]byte("fake-hash")))

				Expect(fakeSCCFuncs.InstallChaincodeCallCount()).To(Equal(1))
				name, version, ccInstallPackage := fakeSCCFuncs.InstallChaincodeArgsForCall(0)
				Expect(name).To(Equal("name"))
				Expect(version).To(Equal("version"))
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
					Name:    "name",
					Version: "version",
				}

				var err error
				marshaledArg, err = proto.Marshal(arg)
				Expect(err).NotTo(HaveOccurred())

				fakeStub.GetArgsReturns([][]byte{[]byte("QueryInstalledChaincode"), marshaledArg})

				fakeSCCFuncs.QueryInstalledChaincodeReturns([]byte("fake-hash"), nil)
			})

			It("passes the arguments to and returns the results from the backing scc function implementation", func() {
				res := scc.Invoke(fakeStub)
				Expect(res.Status).To(Equal(int32(200)))
				payload := &lb.QueryInstalledChaincodeResult{}
				err := proto.Unmarshal(res.Payload, payload)
				Expect(err).NotTo(HaveOccurred())
				Expect(payload.Hash).To(Equal([]byte("fake-hash")))

				Expect(fakeSCCFuncs.QueryInstalledChaincodeCallCount()).To(Equal(1))
				name, version := fakeSCCFuncs.QueryInstalledChaincodeArgsForCall(0)
				Expect(name).To(Equal("name"))
				Expect(version).To(Equal("version"))
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
						Name:    "cc0-name",
						Version: "cc0-version",
						Id:      []byte("cc0-hash"),
					},
					{
						Name:    "cc1-name",
						Version: "cc1-version",
						Id:      []byte("cc1-hash"),
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

				Expect(payload.InstalledChaincodes[0].Name).To(Equal(fmt.Sprintf("cc0-name")))
				Expect(payload.InstalledChaincodes[0].Version).To(Equal(fmt.Sprintf("cc0-version")))
				Expect(payload.InstalledChaincodes[0].Hash).To(Equal([]byte(fmt.Sprintf("cc0-hash"))))

				Expect(payload.InstalledChaincodes[1].Name).To(Equal(fmt.Sprintf("cc1-name")))
				Expect(payload.InstalledChaincodes[1].Version).To(Equal(fmt.Sprintf("cc1-version")))
				Expect(payload.InstalledChaincodes[1].Hash).To(Equal([]byte(fmt.Sprintf("cc1-hash"))))

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
					Name:                "name",
					Version:             "version",
					Hash:                []byte("hash"),
					EndorsementPlugin:   "endorsement-plugin",
					ValidationPlugin:    "validation-plugin",
					ValidationParameter: []byte("validation-parameter"),
					Collections:         &cb.CollectionConfigPackage{},
					InitRequired:        true,
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
				name, cd, pubState, privState := fakeSCCFuncs.ApproveChaincodeDefinitionForOrgArgsForCall(0)
				Expect(name).To(Equal("name"))
				Expect(cd).To(Equal(&lifecycle.ChaincodeDefinition{
					Sequence: 7,
					EndorsementInfo: &lb.ChaincodeEndorsementInfo{
						Version:           "version",
						Id:                []byte("hash"),
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
				Expect(privState).To(BeAssignableToTypeOf(&lifecycle.ChaincodePrivateLedgerShim{}))
				Expect(privState.(*lifecycle.ChaincodePrivateLedgerShim).Collection).To(Equal("_implicit_org_fake-mspid"))
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
					Name:                "name",
					Version:             "version",
					Hash:                []byte("hash"),
					EndorsementPlugin:   "endorsement-plugin",
					ValidationPlugin:    "validation-plugin",
					ValidationParameter: []byte("validation-parameter"),
					Collections:         &cb.CollectionConfigPackage{},
					InitRequired:        true,
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
				name, cd, pubState, orgStates := fakeSCCFuncs.CommitChaincodeDefinitionArgsForCall(0)
				Expect(name).To(Equal("name"))
				Expect(cd).To(Equal(&lifecycle.ChaincodeDefinition{
					Sequence: 7,
					EndorsementInfo: &lb.ChaincodeEndorsementInfo{
						Version:           "version",
						Id:                []byte("hash"),
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
						Id:                []byte("hash"),
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
					Hash:                []byte("hash"),
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
