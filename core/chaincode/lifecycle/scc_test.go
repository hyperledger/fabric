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
	)

	BeforeEach(func() {
		fakeSCCFuncs = &mock.SCCFunctions{}
		fakeChannelConfigSource = &mock.ChannelConfigSource{}
		fakeChannelConfig = &mock.ChannelConfig{}
		fakeChannelConfigSource.GetStableChannelConfigReturns(fakeChannelConfig)
		fakeApplicationConfig = &mock.ApplicationConfig{}
		fakeChannelConfig.ApplicationConfigReturns(fakeApplicationConfig, true)
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
			Expect(scc.Name()).To(Equal("+lifecycle"))
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

		Describe("DefineChaincodeForMyOrg", func() {
			var (
				err          error
				arg          *lb.DefineChaincodeForMyOrgArgs
				marshaledArg []byte
			)

			BeforeEach(func() {
				arg = &lb.DefineChaincodeForMyOrgArgs{
					Sequence:            7,
					Name:                "name",
					Version:             "version",
					Hash:                []byte("hash"),
					EndorsementPlugin:   "endorsement-plugin",
					ValidationPlugin:    "validation-plugin",
					ValidationParameter: []byte("validation-parameter"),
				}

				marshaledArg, err = proto.Marshal(arg)
				Expect(err).NotTo(HaveOccurred())

				fakeStub.GetArgsReturns([][]byte{[]byte("DefineChaincodeForMyOrg"), marshaledArg})
			})

			It("passes the arguments to and returns the results from the backing scc function implementation", func() {
				res := scc.Invoke(fakeStub)
				Expect(res.Status).To(Equal(int32(200)))
				payload := &lb.DefineChaincodeForMyOrgResult{}
				err = proto.Unmarshal(res.Payload, payload)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeSCCFuncs.DefineChaincodeForOrgCallCount()).To(Equal(1))
				cd, pubState, privState := fakeSCCFuncs.DefineChaincodeForOrgArgsForCall(0)
				Expect(cd).To(Equal(&lifecycle.ChaincodeDefinition{
					Sequence: 7,
					Name:     "name",
					Parameters: &lifecycle.ChaincodeParameters{
						Version:             "version",
						Hash:                []byte("hash"),
						EndorsementPlugin:   "endorsement-plugin",
						ValidationPlugin:    "validation-plugin",
						ValidationParameter: []byte("validation-parameter"),
					},
				}))
				Expect(pubState).To(Equal(fakeStub))
				Expect(privState).To(BeAssignableToTypeOf(&lifecycle.ChaincodePrivateLedgerShim{}))
				Expect(privState.(*lifecycle.ChaincodePrivateLedgerShim).Collection).To(Equal("_implicit_org_fake-mspid"))
			})

			Context("when the underlying function implementation fails", func() {
				BeforeEach(func() {
					fakeSCCFuncs.DefineChaincodeForOrgReturns(fmt.Errorf("underlying-error"))
				})

				It("wraps and returns the error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'DefineChaincodeForMyOrg': underlying-error"))
				})
			})
		})

		Describe("DefineChaincode", func() {
			var (
				err            error
				arg            *lb.DefineChaincodeArgs
				marshaledArg   []byte
				fakeOrgConfigs []*mock.ApplicationOrgConfig
			)

			BeforeEach(func() {
				arg = &lb.DefineChaincodeArgs{
					Sequence:            7,
					Name:                "name",
					Version:             "version",
					Hash:                []byte("hash"),
					EndorsementPlugin:   "endorsement-plugin",
					ValidationPlugin:    "validation-plugin",
					ValidationParameter: []byte("validation-parameter"),
				}

				marshaledArg, err = proto.Marshal(arg)
				Expect(err).NotTo(HaveOccurred())

				fakeStub.GetArgsReturns([][]byte{[]byte("DefineChaincode"), marshaledArg})

				fakeOrgConfigs = []*mock.ApplicationOrgConfig{{}, {}}
				fakeOrgConfigs[0].MSPIDReturns("fake-mspid")
				fakeOrgConfigs[1].MSPIDReturns("other-mspid")

				fakeApplicationConfig.OrganizationsReturns(map[string]channelconfig.ApplicationOrg{
					"org0": fakeOrgConfigs[0],
					"org1": fakeOrgConfigs[1],
				})

				fakeSCCFuncs.DefineChaincodeReturns([]bool{true, true}, nil)
			})

			It("passes the arguments to and returns the results from the backing scc function implementation", func() {
				res := scc.Invoke(fakeStub)
				Expect(res.Status).To(Equal(int32(200)))
				payload := &lb.DefineChaincodeResult{}
				err = proto.Unmarshal(res.Payload, payload)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeSCCFuncs.DefineChaincodeCallCount()).To(Equal(1))
				cd, pubState, orgStates := fakeSCCFuncs.DefineChaincodeArgsForCall(0)
				Expect(cd).To(Equal(&lifecycle.ChaincodeDefinition{
					Sequence: 7,
					Name:     "name",
					Parameters: &lifecycle.ChaincodeParameters{
						Version:             "version",
						Hash:                []byte("hash"),
						EndorsementPlugin:   "endorsement-plugin",
						ValidationPlugin:    "validation-plugin",
						ValidationParameter: []byte("validation-parameter"),
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

			Context("when there is no agreement from this peer's org", func() {
				BeforeEach(func() {
					fakeSCCFuncs.DefineChaincodeReturns([]bool{false, false}, nil)
				})

				It("returns an error indicating the lack of agreement", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'DefineChaincode': chaincode definition not agreed to by this org (fake-mspid)"))
				})
			})

			Context("when there is no match for this peer's org's MSPID", func() {
				BeforeEach(func() {
					fakeOrgConfigs[0].MSPIDReturns("other-mspid")
				})

				It("returns an error indicating the lack of agreement", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'DefineChaincode': impossibly, this peer's org is processing requests for a channel it is not a member of"))
				})
			})

			Context("when there is no channel config", func() {
				BeforeEach(func() {
					fakeChannelConfigSource.GetStableChannelConfigReturns(nil)
				})

				It("returns an error indicating the lack of agreement", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'DefineChaincode': could not get channelconfig for channel "))
				})
			})

			Context("when there is no application config", func() {
				BeforeEach(func() {
					fakeChannelConfig.ApplicationConfigReturns(nil, false)
				})

				It("returns an error indicating the lack of agreement", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'DefineChaincode': could not get application config for channel "))
				})
			})

			Context("when the underlying function implementation fails", func() {
				BeforeEach(func() {
					fakeSCCFuncs.DefineChaincodeReturns(nil, fmt.Errorf("underlying-error"))
				})

				It("wraps and returns the error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'DefineChaincode': underlying-error"))
				})
			})
		})

		Describe("QueryDefinedChaincode", func() {
			var (
				arg          *lb.QueryDefinedChaincodeArgs
				marshaledArg []byte
			)

			BeforeEach(func() {
				arg = &lb.QueryDefinedChaincodeArgs{
					Name: "cc-name",
				}

				var err error
				marshaledArg, err = proto.Marshal(arg)
				Expect(err).NotTo(HaveOccurred())

				fakeStub.GetArgsReturns([][]byte{[]byte("QueryDefinedChaincode"), marshaledArg})
				fakeSCCFuncs.QueryDefinedChaincodeReturns(&lifecycle.DefinedChaincode{
					Sequence:            2,
					Version:             "version",
					EndorsementPlugin:   "endorsement-plugin",
					ValidationPlugin:    "validation-plugin",
					ValidationParameter: []byte("validation-parameter"),
					Hash:                []byte("hash"),
				}, nil)
			})

			It("passes the arguments to and returns the results from the backing scc function implementation", func() {
				res := scc.Invoke(fakeStub)
				Expect(res.Status).To(Equal(int32(200)))
				payload := &lb.QueryDefinedChaincodeResult{}
				err := proto.Unmarshal(res.Payload, payload)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeSCCFuncs.QueryDefinedChaincodeCallCount()).To(Equal(1))
				name, pubState := fakeSCCFuncs.QueryDefinedChaincodeArgsForCall(0)
				Expect(name).To(Equal("cc-name"))
				Expect(pubState).To(Equal(fakeStub))
			})

			Context("when the underlying function implementation fails", func() {
				BeforeEach(func() {
					fakeSCCFuncs.QueryDefinedChaincodeReturns(nil, fmt.Errorf("underlying-error"))
				})

				It("wraps and returns the error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'QueryDefinedChaincode': underlying-error"))
				})
			})
		})

		Describe("QueryDefinedNamespaces", func() {
			var (
				arg          *lb.QueryDefinedNamespacesArgs
				marshaledArg []byte
			)

			BeforeEach(func() {
				arg = &lb.QueryDefinedNamespacesArgs{}

				var err error
				marshaledArg, err = proto.Marshal(arg)
				Expect(err).NotTo(HaveOccurred())

				fakeStub.GetArgsReturns([][]byte{[]byte("QueryDefinedNamespaces"), marshaledArg})
				fakeSCCFuncs.QueryDefinedNamespacesReturns(map[string]string{
					"foo": "Chaincode",
					"bar": "Token",
				}, nil)
			})

			It("passes the arguments to and returns the results from the backing scc function implementation", func() {
				res := scc.Invoke(fakeStub)
				Expect(res.Status).To(Equal(int32(200)))
				payload := &lb.QueryDefinedNamespacesResult{}
				err := proto.Unmarshal(res.Payload, payload)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeSCCFuncs.QueryDefinedNamespacesCallCount()).To(Equal(1))
				Expect(fakeSCCFuncs.QueryDefinedNamespacesArgsForCall(0)).To(Equal(&lifecycle.ChaincodePublicLedgerShim{ChaincodeStubInterface: fakeStub}))
			})

			Context("when the underlying function implementation fails", func() {
				BeforeEach(func() {
					fakeSCCFuncs.QueryDefinedNamespacesReturns(nil, fmt.Errorf("underlying-error"))
				})

				It("wraps and returns the error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'QueryDefinedNamespaces': underlying-error"))
				})
			})
		})
	})

})
