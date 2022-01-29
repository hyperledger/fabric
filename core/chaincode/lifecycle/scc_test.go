/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle_test

import (
	"bytes"
	"encoding/gob"
	"fmt"

	"github.com/hyperledger/fabric/core/ledger"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-chaincode-go/shim"
	mspprotos "github.com/hyperledger/fabric-protos-go/msp"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	lb "github.com/hyperledger/fabric-protos-go/peer/lifecycle"
	"github.com/hyperledger/fabric/common/chaincode"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/policydsl"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle/mock"
	"github.com/hyperledger/fabric/core/chaincode/persistence"
	"github.com/hyperledger/fabric/core/dispatcher"
	"github.com/hyperledger/fabric/msp"
	"github.com/pkg/errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("SCC", func() {
	var (
		scc                        *lifecycle.SCC
		fakeSCCFuncs               *mock.SCCFunctions
		fakeChannelConfigSource    *mock.ChannelConfigSource
		fakeChannelConfig          *mock.ChannelConfig
		fakeApplicationConfig      *mock.ApplicationConfig
		fakeCapabilities           *mock.ApplicationCapabilities
		fakeACLProvider            *mock.ACLProvider
		fakeMSPManager             *mock.MSPManager
		fakeQueryExecutorProvider  *mock.QueryExecutorProvider
		fakeQueryExecutor          *mock.SimpleQueryExecutor
		fakeDeployedCCInfoProvider *mock.LegacyDeployedCCInfoProvider
		fakeStub                   *mock.ChaincodeStub
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
		fakeMSPManager = &mock.MSPManager{}
		fakeChannelConfig.MSPManagerReturns(fakeMSPManager)
		fakeQueryExecutorProvider = &mock.QueryExecutorProvider{}
		fakeQueryExecutor = &mock.SimpleQueryExecutor{}
		fakeQueryExecutorProvider.TxQueryExecutorReturns(fakeQueryExecutor)
		fakeStub = &mock.ChaincodeStub{}
		fakeDeployedCCInfoProvider = &mock.LegacyDeployedCCInfoProvider{}

		scc = &lifecycle.SCC{
			Dispatcher: &dispatcher.Dispatcher{
				Protobuf: &dispatcher.ProtobufImpl{},
			},
			Functions:              fakeSCCFuncs,
			OrgMSPID:               "fake-mspid",
			ChannelConfigSource:    fakeChannelConfigSource,
			ACLProvider:            fakeACLProvider,
			QueryExecutorProvider:  fakeQueryExecutorProvider,
			DeployedCCInfoProvider: fakeDeployedCCInfoProvider,
		}
	})

	Describe("Name", func() {
		It("returns the name", func() {
			Expect(scc.Name()).To(Equal("_lifecycle"))
		})
	})

	Describe("Chaincode", func() {
		It("returns a reference to itself", func() {
			Expect(scc.Chaincode()).To(Equal(scc))
		})
	})

	Describe("Init", func() {
		It("does nothing", func() {
			Expect(scc.Init(nil)).To(Equal(shim.Success(nil)))
		})
	})

	Describe("Invoke", func() {
		BeforeEach(func() {
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
				arg = &lb.InstallChaincodeArgs{
					ChaincodeInstallPackage: []byte("chaincode-package"),
				}

				var err error
				marshaledArg, err = proto.Marshal(arg)
				Expect(err).NotTo(HaveOccurred())

				fakeStub.GetArgsReturns([][]byte{[]byte("InstallChaincode"), marshaledArg})

				fakeSCCFuncs.InstallChaincodeReturns(&chaincode.InstalledChaincode{
					Label:     "label",
					PackageID: "package-id",
				}, nil)
			})

			It("passes the arguments to and returns the results from the backing scc function implementation", func() {
				res := scc.Invoke(fakeStub)
				Expect(res.Status).To(Equal(int32(200)))
				payload := &lb.InstallChaincodeResult{}
				err := proto.Unmarshal(res.Payload, payload)
				Expect(err).NotTo(HaveOccurred())
				Expect(payload.PackageId).To(Equal("package-id"))

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
					PackageID: "awesome_package",
					Label:     "awesome_package_label",
					References: map[string][]*chaincode.Metadata{
						"test-channel": {
							&chaincode.Metadata{
								Name:    "cc0",
								Version: "cc0-version",
							},
						},
					},
				}, nil)
			})

			It("passes the arguments to and returns the results from the backing scc function implementation", func() {
				res := scc.Invoke(fakeStub)
				Expect(res.Status).To(Equal(int32(200)))
				payload := &lb.QueryInstalledChaincodeResult{}
				err := proto.Unmarshal(res.Payload, payload)
				Expect(err).NotTo(HaveOccurred())
				Expect(payload.Label).To(Equal("awesome_package_label"))
				Expect(payload.PackageId).To(Equal("awesome_package"))
				Expect(payload.References).To(Equal(map[string]*lb.QueryInstalledChaincodeResult_References{
					"test-channel": {
						Chaincodes: []*lb.QueryInstalledChaincodeResult_Chaincode{
							{
								Name:    "cc0",
								Version: "cc0-version",
							},
						},
					},
				}))

				Expect(fakeSCCFuncs.QueryInstalledChaincodeCallCount()).To(Equal(1))
				name := fakeSCCFuncs.QueryInstalledChaincodeArgsForCall(0)
				Expect(name).To(Equal("awesome_package"))
			})

			Context("when the code package cannot be found", func() {
				BeforeEach(func() {
					fakeSCCFuncs.QueryInstalledChaincodeReturns(nil, persistence.CodePackageNotFoundErr{PackageID: "less_awesome_package"})
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

		Describe("GetInstalledChaincodePackage", func() {
			var (
				arg          *lb.GetInstalledChaincodePackageArgs
				marshaledArg []byte
			)

			BeforeEach(func() {
				arg = &lb.GetInstalledChaincodePackageArgs{
					PackageId: "package-id",
				}

				var err error
				marshaledArg, err = proto.Marshal(arg)
				Expect(err).NotTo(HaveOccurred())

				fakeStub.GetArgsReturns([][]byte{[]byte("GetInstalledChaincodePackage"), marshaledArg})

				fakeSCCFuncs.GetInstalledChaincodePackageReturns([]byte("chaincode-package"), nil)
			})

			It("passes the arguments to and returns the results from the backing scc function implementation", func() {
				res := scc.Invoke(fakeStub)
				Expect(res.Status).To(Equal(int32(200)))
				payload := &lb.GetInstalledChaincodePackageResult{}
				err := proto.Unmarshal(res.Payload, payload)
				Expect(err).NotTo(HaveOccurred())
				Expect(payload.ChaincodeInstallPackage).To(Equal([]byte("chaincode-package")))

				Expect(fakeSCCFuncs.GetInstalledChaincodePackageCallCount()).To(Equal(1))
				packageID := fakeSCCFuncs.GetInstalledChaincodePackageArgsForCall(0)
				Expect(packageID).To(Equal("package-id"))
			})

			Context("when the underlying function implementation fails", func() {
				BeforeEach(func() {
					fakeSCCFuncs.GetInstalledChaincodePackageReturns(nil, fmt.Errorf("underlying-error"))
				})

				It("wraps and returns the error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'GetInstalledChaincodePackage': underlying-error"))
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

				fakeSCCFuncs.QueryInstalledChaincodesReturns([]*chaincode.InstalledChaincode{
					{
						Label:     "cc0-label",
						PackageID: "cc0-package-id",
						References: map[string][]*chaincode.Metadata{
							"test-channel": {
								&chaincode.Metadata{
									Name:    "cc0",
									Version: "cc0-version",
								},
							},
						},
					},
					{
						Label:     "cc1-label",
						PackageID: "cc1-package-id",
					},
				})
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
				Expect(payload.InstalledChaincodes[0].References).To(Equal(map[string]*lb.QueryInstalledChaincodesResult_References{
					"test-channel": {
						Chaincodes: []*lb.QueryInstalledChaincodesResult_Chaincode{
							{
								Name:    "cc0",
								Version: "cc0-version",
							},
						},
					},
				}))

				Expect(payload.InstalledChaincodes[1].Label).To(Equal("cc1-label"))
				Expect(payload.InstalledChaincodes[1].PackageId).To(Equal("cc1-package-id"))

				Expect(fakeSCCFuncs.QueryInstalledChaincodesCallCount()).To(Equal(1))
			})
		})

		Describe("ApproveChaincodeDefinitionForMyOrg", func() {
			var (
				err         error
				collConfigs collectionConfigs
				fakeMsp     *mock.MSP

				arg          *lb.ApproveChaincodeDefinitionForMyOrgArgs
				marshaledArg []byte
			)

			BeforeEach(func() {
				// identity1 of type MSPRole
				mspPrincipal := &mspprotos.MSPRole{
					MspIdentifier: "test-member-role",
				}
				mspPrincipalBytes, err := proto.Marshal(mspPrincipal)
				Expect(err).NotTo(HaveOccurred())
				identity1 := &mspprotos.MSPPrincipal{
					PrincipalClassification: mspprotos.MSPPrincipal_ROLE,
					Principal:               mspPrincipalBytes,
				}

				// identity2 of type OU
				mspou := &mspprotos.OrganizationUnit{
					MspIdentifier: "test-member-ou",
				}
				mspouBytes, err := proto.Marshal(mspou)
				Expect(err).NotTo(HaveOccurred())
				identity2 := &mspprotos.MSPPrincipal{
					PrincipalClassification: mspprotos.MSPPrincipal_ORGANIZATION_UNIT,
					Principal:               mspouBytes,
				}

				// identity3 of type identity
				identity3 := &mspprotos.MSPPrincipal{
					PrincipalClassification: mspprotos.MSPPrincipal_IDENTITY,
					Principal:               []byte("test-member-identity"),
				}

				fakeIdentities := []*mspprotos.MSPPrincipal{
					identity1, identity2, identity3,
				}

				fakeMsp = &mock.MSP{}
				fakeMSPManager.GetMSPsReturns(
					map[string]msp.MSP{
						"test-member-role":     fakeMsp,
						"test-member-ou":       fakeMsp,
						"test-member-identity": fakeMsp,
					},
					nil,
				)

				collConfigs = []*collectionConfig{
					{
						Name:              "test-collection",
						Policy:            "OR('fakeOrg1.member', 'fakeOrg2.member', 'fakeOrg3.member')",
						RequiredPeerCount: 2,
						MaxPeerCount:      3,
						BlockToLive:       0,
						Identities:        fakeIdentities,
					},
				}

				arg = &lb.ApproveChaincodeDefinitionForMyOrgArgs{
					Sequence:            7,
					Name:                "cc_name",
					Version:             "version_1.0",
					EndorsementPlugin:   "endorsement-plugin",
					ValidationPlugin:    "validation-plugin",
					ValidationParameter: []byte("validation-parameter"),
					InitRequired:        true,
					Source: &lb.ChaincodeSource{
						Type: &lb.ChaincodeSource_LocalPackage{
							LocalPackage: &lb.ChaincodeSource_Local{
								PackageId: "hash",
							},
						},
					},
				}
			})

			JustBeforeEach(func() {
				arg.Collections = collConfigs.toProtoCollectionConfigPackage()
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
				Expect(cd.Sequence).To(Equal(int64(7)))
				Expect(cd.EndorsementInfo).To(Equal(&lb.ChaincodeEndorsementInfo{
					Version:           "version_1.0",
					EndorsementPlugin: "endorsement-plugin",
					InitRequired:      true,
				}))
				Expect(cd.ValidationInfo).To(Equal(&lb.ChaincodeValidationInfo{
					ValidationPlugin:    "validation-plugin",
					ValidationParameter: []byte("validation-parameter"),
				}))
				Expect(proto.Equal(
					cd.Collections,
					collConfigs.toProtoCollectionConfigPackage(),
				)).Should(BeTrue())

				Expect(packageID).To(Equal("hash"))
				Expect(pubState).To(Equal(fakeStub))
				Expect(privState).To(BeAssignableToTypeOf(&lifecycle.ChaincodePrivateLedgerShim{}))
				Expect(privState.(*lifecycle.ChaincodePrivateLedgerShim).Collection).To(Equal("_implicit_org_fake-mspid"))
			})

			Context("when the chaincode name contains invalid characters", func() {
				BeforeEach(func() {
					arg.Name = "!nvalid"
				})

				It("wraps and returns the error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'ApproveChaincodeDefinitionForMyOrg': error validating chaincode definition: invalid chaincode name '!nvalid'. Names can only consist of alphanumerics, '_', and '-' and can only begin with alphanumerics"))
				})
			})

			Context("when the chaincode version contains invalid characters", func() {
				BeforeEach(func() {
					arg.Version = "$money$"
				})

				It("wraps and returns the error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'ApproveChaincodeDefinitionForMyOrg': error validating chaincode definition: invalid chaincode version '$money$'. Versions can only consist of alphanumerics, '_', '-', '+', and '.'"))
				})
			})

			Context("when the chaincode name matches an existing system chaincode name", func() {
				BeforeEach(func() {
					arg.Name = "cscc"
				})

				It("wraps and returns the error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'ApproveChaincodeDefinitionForMyOrg': error validating chaincode definition: chaincode name 'cscc' is the name of a system chaincode"))
				})
			})

			Context("when a collection name contains invalid characters", func() {
				BeforeEach(func() {
					collConfigs[0].Name = "collection@test"
				})

				It("wraps and returns the error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'ApproveChaincodeDefinitionForMyOrg': error validating chaincode definition: invalid collection name 'collection@test'. Names can only consist of alphanumerics, '_', and '-' and cannot begin with '_'"))
				})
			})

			Context("when a collection name begins with an invalid character", func() {
				BeforeEach(func() {
					collConfigs[0].Name = "_collection"
				})

				It("wraps and returns the error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'ApproveChaincodeDefinitionForMyOrg': error validating chaincode definition: invalid collection name '_collection'. Names can only consist of alphanumerics, '_', and '-' and cannot begin with '_'"))
				})
			})

			Context("when collection member-org-policy is nil", func() {
				BeforeEach(func() {
					collConfigs[0].UseGivenMemberOrgPolicy = true
					collConfigs[0].MemberOrgPolicy = nil
				})

				It("wraps and returns error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'ApproveChaincodeDefinitionForMyOrg': error validating chaincode definition: collection member policy is not set for collection 'test-collection'"))
				})
			})

			Context("when collection member-org-policy signature policy is nil", func() {
				BeforeEach(func() {
					collConfigs[0].UseGivenMemberOrgPolicy = true
					collConfigs[0].MemberOrgPolicy = &pb.CollectionPolicyConfig{}
				})

				It("wraps and returns error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'ApproveChaincodeDefinitionForMyOrg': error validating chaincode definition: collection member org policy is empty for collection 'test-collection'"))
				})
			})

			Context("when collection member-org-policy signature policy is not an OR only policy", func() {
				BeforeEach(func() {
					collConfigs[0].Policy = "OR('fakeOrg1.member', AND('fakeOrg2.member', 'fakeOrg3.member'))"
				})

				It("wraps and returns error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'ApproveChaincodeDefinitionForMyOrg': error validating chaincode definition: collection-name: test-collection -- error in member org policy: signature policy is not an OR concatenation, NOutOf 2"))
				})
			})

			Context("when collection member-org-policy signature policy contains unmarshable MSPRole", func() {
				BeforeEach(func() {
					collConfigs[0].Identities[0] = &mspprotos.MSPPrincipal{
						PrincipalClassification: mspprotos.MSPPrincipal_ROLE,
						Principal:               []byte("unmarshable bytes"),
					}
				})

				It("wraps and returns error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).Should(ContainSubstring("failed to invoke backing implementation of 'ApproveChaincodeDefinitionForMyOrg': error validating chaincode definition: collection-name: test-collection -- cannot unmarshal identity bytes into MSPRole"))
				})
			})

			Context("when collection member-org-policy signature policy contains too few principals", func() {
				BeforeEach(func() {
					collConfigs[0].Identities = collConfigs[0].Identities[0:1]
				})

				It("wraps and returns error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).Should(ContainSubstring("failed to invoke backing implementation of 'ApproveChaincodeDefinitionForMyOrg': error validating chaincode definition: invalid member org policy for collection 'test-collection': identity index out of range, requested 1, but identities length is 1"))
				})
			})

			Context("when collection MSPRole in member-org-policy in not a channel member", func() {
				BeforeEach(func() {
					fakeMSPManager.GetMSPsReturns(
						map[string]msp.MSP{
							"test-member-ou":       fakeMsp,
							"test-member-identity": fakeMsp,
						},
						nil,
					)
				})

				It("wraps and returns error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).Should(ContainSubstring("failed to invoke backing implementation of 'ApproveChaincodeDefinitionForMyOrg': error validating chaincode definition: collection-name: test-collection -- collection member 'test-member-role' is not part of the channel"))
				})
			})

			Context("when collection member-org-policy signature policy contains unmarshable ORGANIZATION_UNIT", func() {
				BeforeEach(func() {
					collConfigs[0].Identities[0] = &mspprotos.MSPPrincipal{
						PrincipalClassification: mspprotos.MSPPrincipal_ORGANIZATION_UNIT,
						Principal:               []byte("unmarshable bytes"),
					}
				})

				It("wraps and returns error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).Should(ContainSubstring("failed to invoke backing implementation of 'ApproveChaincodeDefinitionForMyOrg': error validating chaincode definition: collection-name: test-collection -- cannot unmarshal identity bytes into OrganizationUnit"))
				})
			})

			Context("when collection MSPOU in member-org-policy in not a channel member", func() {
				BeforeEach(func() {
					fakeMSPManager.GetMSPsReturns(
						map[string]msp.MSP{
							"test-member-role":     fakeMsp,
							"test-member-identity": fakeMsp,
						},
						nil,
					)
				})

				It("wraps and returns error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'ApproveChaincodeDefinitionForMyOrg': error validating chaincode definition: collection-name: test-collection -- collection member 'test-member-ou' is not part of the channel"))
				})
			})

			Context("when collection MSP identity in member-org-policy in not a channel member", func() {
				BeforeEach(func() {
					fakeMSPManager.DeserializeIdentityReturns(nil, errors.New("Nope"))
				})

				It("wraps and returns error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'ApproveChaincodeDefinitionForMyOrg': error validating chaincode definition: collection-name: test-collection -- contains an identity that is not part of the channel"))
				})
			})

			Context("when collection member-org-policy signature policy contains unsupported principal type", func() {
				BeforeEach(func() {
					collConfigs[0].Identities[0] = &mspprotos.MSPPrincipal{
						PrincipalClassification: mspprotos.MSPPrincipal_ANONYMITY,
					}
				})

				It("wraps and returns error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'ApproveChaincodeDefinitionForMyOrg': error validating chaincode definition: collection-name: test-collection -- principal type ANONYMITY is not supported"))
				})
			})

			Context("when collection config contains duplicate collections", func() {
				BeforeEach(func() {
					collConfigs = append(collConfigs, collConfigs[0])
				})

				It("wraps and returns error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'ApproveChaincodeDefinitionForMyOrg': error validating chaincode definition: collection-name: test-collection -- found duplicate in collection configuration"))
				})
			})

			Context("when collection config contains requiredPeerCount < zero", func() {
				BeforeEach(func() {
					collConfigs[0].RequiredPeerCount = -2
				})

				It("wraps and returns error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'ApproveChaincodeDefinitionForMyOrg': error validating chaincode definition: collection-name: test-collection -- requiredPeerCount (-2) cannot be less than zero"))
				})
			})

			Context("when collection config contains requiredPeerCount > maxPeerCount", func() {
				BeforeEach(func() {
					collConfigs[0].MaxPeerCount = 10
					collConfigs[0].RequiredPeerCount = 20
				})

				It("wraps and returns error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'ApproveChaincodeDefinitionForMyOrg': error validating chaincode definition: collection-name: test-collection -- maximum peer count (10) cannot be less than the required peer count (20)"))
				})
			})

			Context("when committed definition and proposed definition both contains no collection config", func() {
				BeforeEach(func() {
					fakeDeployedCCInfoProvider.ChaincodeInfoReturns(&ledger.DeployedChaincodeInfo{}, nil)
					arg.Collections = nil
				})

				It("does not return error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(200)))
				})
			})

			Context("when committed definition and proposed definition both contains same collection config", func() {
				BeforeEach(func() {
					fakeDeployedCCInfoProvider.ChaincodeInfoReturns(
						&ledger.DeployedChaincodeInfo{
							ExplicitCollectionConfigPkg: collConfigs.toProtoCollectionConfigPackage(),
						},
						nil,
					)
				})

				It("does not return error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(200)))
				})
			})

			Context("when committed definition contains collection config and the proposed definition contains no collection config", func() {
				BeforeEach(func() {
					fakeDeployedCCInfoProvider.ChaincodeInfoReturns(
						&ledger.DeployedChaincodeInfo{
							ExplicitCollectionConfigPkg: collConfigs.toProtoCollectionConfigPackage(),
						},
						nil,
					)
					collConfigs = nil
				})

				It("wraps and returns error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'ApproveChaincodeDefinitionForMyOrg': error validating chaincode definition: the proposed collection config does not contain previously defined collections"))
				})
			})

			Context("when committed definition contains a collection that is not defined in the proposed definition", func() {
				BeforeEach(func() {
					committedCollConfigs := collConfigs.deepCopy()
					additionalCommittedConfigs := collConfigs.deepCopy()
					additionalCommittedConfigs[0].Name = "missing-collection"
					committedCollConfigs = append(committedCollConfigs, additionalCommittedConfigs...)
					fakeDeployedCCInfoProvider.ChaincodeInfoReturns(
						&ledger.DeployedChaincodeInfo{
							ExplicitCollectionConfigPkg: committedCollConfigs.toProtoCollectionConfigPackage(),
						},
						nil,
					)
				})

				It("wraps and returns error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'ApproveChaincodeDefinitionForMyOrg': error validating chaincode definition: existing collection [missing-collection] missing in the proposed collection configuration"))
				})
			})

			Context("when committed definition contains a collection that has different BTL than defined in the proposed definition", func() {
				var committedCollConfigs collectionConfigs
				BeforeEach(func() {
					committedCollConfigs = collConfigs.deepCopy()
					committedCollConfigs[0].BlockToLive = committedCollConfigs[0].BlockToLive + 1
					fakeDeployedCCInfoProvider.ChaincodeInfoReturns(
						&ledger.DeployedChaincodeInfo{
							ExplicitCollectionConfigPkg: committedCollConfigs.toProtoCollectionConfigPackage(),
						},
						nil,
					)
				})

				It("wraps and returns error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal(
						fmt.Sprintf(
							"failed to invoke backing implementation of 'ApproveChaincodeDefinitionForMyOrg': error validating chaincode definition: the BlockToLive in an existing collection [test-collection] modified. Existing value [%d]",
							committedCollConfigs[0].BlockToLive,
						),
					))
				})
			})

			Context("when not able to get MSPManager for evaluating collection config", func() {
				BeforeEach(func() {
					fakeChannelConfig.MSPManagerReturns(nil)
				})

				It("wraps and returns error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'ApproveChaincodeDefinitionForMyOrg': error validating chaincode definition: could not get MSP manager for channel 'test-channel'"))
				})
			})

			Context("when not able to get MSPs for evaluating collection config", func() {
				BeforeEach(func() {
					fakeMSPManager.GetMSPsReturns(nil, errors.New("No MSPs"))
				})

				It("wraps and returns error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'ApproveChaincodeDefinitionForMyOrg': error validating chaincode definition: could not get MSPs: No MSPs"))
				})
			})

			Context("when not able to get committed definition for evaluating collection config", func() {
				BeforeEach(func() {
					fakeDeployedCCInfoProvider.ChaincodeInfoReturns(nil, errors.New("could not fetch definition"))
				})

				It("wraps and returns error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'ApproveChaincodeDefinitionForMyOrg': error validating chaincode definition: could not retrieve committed definition for chaincode 'cc_name': could not fetch definition"))
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
					Collections: &pb.CollectionConfigPackage{
						Config: []*pb.CollectionConfig{
							{
								Payload: &pb.CollectionConfig_StaticCollectionConfig{
									StaticCollectionConfig: &pb.StaticCollectionConfig{
										Name: "test_collection",
										MemberOrgsPolicy: &pb.CollectionPolicyConfig{
											Payload: &pb.CollectionPolicyConfig_SignaturePolicy{
												SignaturePolicy: policydsl.SignedByMspMember("org0"),
											},
										},
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

				fakeSCCFuncs.CommitChaincodeDefinitionReturns(map[string]bool{
					"fake-mspid":  true,
					"other-mspid": true,
				}, nil)

				fakeMsp := &mock.MSP{}
				fakeMSPManager.GetMSPsReturns(
					map[string]msp.MSP{
						"org0": fakeMsp,
						"org1": fakeMsp,
					},
					nil,
				)
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
					Collections: &pb.CollectionConfigPackage{
						Config: []*pb.CollectionConfig{
							{
								Payload: &pb.CollectionConfig_StaticCollectionConfig{
									StaticCollectionConfig: &pb.StaticCollectionConfig{
										Name: "test_collection",
										MemberOrgsPolicy: &pb.CollectionPolicyConfig{
											Payload: &pb.CollectionPolicyConfig_SignaturePolicy{
												SignaturePolicy: policydsl.SignedByMspMember("org0"),
											},
										},
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
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'CommitChaincodeDefinition': error validating chaincode definition: invalid chaincode name '_invalid'. Names can only consist of alphanumerics, '_', and '-' and can only begin with alphanumerics"))
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
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'CommitChaincodeDefinition': error validating chaincode definition: invalid chaincode version '$money$'. Versions can only consist of alphanumerics, '_', '-', '+', and '.'"))
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
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'CommitChaincodeDefinition': error validating chaincode definition: chaincode name 'qscc' is the name of a system chaincode"))
				})
			})

			Context("when a collection name contains invalid characters", func() {
				BeforeEach(func() {
					arg.Collections = &pb.CollectionConfigPackage{
						Config: []*pb.CollectionConfig{
							{
								Payload: &pb.CollectionConfig_StaticCollectionConfig{
									StaticCollectionConfig: &pb.StaticCollectionConfig{
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
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'CommitChaincodeDefinition': error validating chaincode definition: invalid collection name 'collection(test'. Names can only consist of alphanumerics, '_', and '-' and cannot begin with '_'"))
				})
			})

			Context("when a collection name begins with an invalid character", func() {
				BeforeEach(func() {
					arg.Collections = &pb.CollectionConfigPackage{
						Config: []*pb.CollectionConfig{
							{
								Payload: &pb.CollectionConfig_StaticCollectionConfig{
									StaticCollectionConfig: &pb.StaticCollectionConfig{
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
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'CommitChaincodeDefinition': error validating chaincode definition: invalid collection name '&collection'. Names can only consist of alphanumerics, '_', and '-' and cannot begin with '_'"))
				})
			})

			Context("when there is no agreement from this peer's org", func() {
				BeforeEach(func() {
					fakeSCCFuncs.CommitChaincodeDefinitionReturns(map[string]bool{
						"fake-mspid":  false,
						"other-mspid": false,
					}, nil)
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

		Describe("CheckCommitReadiness", func() {
			var (
				err            error
				arg            *lb.CheckCommitReadinessArgs
				marshaledArg   []byte
				fakeOrgConfigs []*mock.ApplicationOrgConfig
			)

			BeforeEach(func() {
				arg = &lb.CheckCommitReadinessArgs{
					Sequence:            7,
					Name:                "name",
					Version:             "version",
					EndorsementPlugin:   "endorsement-plugin",
					ValidationPlugin:    "validation-plugin",
					ValidationParameter: []byte("validation-parameter"),
					Collections:         &pb.CollectionConfigPackage{},
					InitRequired:        true,
				}

				marshaledArg, err = proto.Marshal(arg)
				Expect(err).NotTo(HaveOccurred())

				fakeStub.GetArgsReturns([][]byte{[]byte("CheckCommitReadiness"), marshaledArg})

				fakeOrgConfigs = []*mock.ApplicationOrgConfig{{}, {}}
				fakeOrgConfigs[0].MSPIDReturns("fake-mspid")
				fakeOrgConfigs[1].MSPIDReturns("other-mspid")

				fakeApplicationConfig.OrganizationsReturns(map[string]channelconfig.ApplicationOrg{
					"org0": fakeOrgConfigs[0],
					"org1": fakeOrgConfigs[1],
				})

				fakeSCCFuncs.CheckCommitReadinessReturns(map[string]bool{
					"fake-mspid":  true,
					"other-mspid": true,
				}, nil)
			})

			It("passes the arguments to and returns the results from the backing scc function implementation", func() {
				res := scc.Invoke(fakeStub)
				Expect(res.Message).To(Equal(""))
				Expect(res.Status).To(Equal(int32(200)))
				payload := &lb.CheckCommitReadinessResult{}
				err = proto.Unmarshal(res.Payload, payload)
				Expect(err).NotTo(HaveOccurred())

				orgApprovals := payload.GetApprovals()
				Expect(orgApprovals).NotTo(BeNil())
				Expect(len(orgApprovals)).To(Equal(2))
				Expect(orgApprovals["fake-mspid"]).To(BeTrue())
				Expect(orgApprovals["other-mspid"]).To(BeTrue())

				Expect(fakeSCCFuncs.CheckCommitReadinessCallCount()).To(Equal(1))
				chname, ccname, cd, pubState, orgStates := fakeSCCFuncs.CheckCommitReadinessArgsForCall(0)
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
				Expect(orgStates).To(HaveLen(2))
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
						Expect(res.Message).To(Equal("failed to invoke backing implementation of 'CheckCommitReadiness': no application config for channel ''"))
					})
				})
			})

			Context("when the underlying function implementation fails", func() {
				BeforeEach(func() {
					fakeSCCFuncs.CheckCommitReadinessReturns(nil, fmt.Errorf("underlying-error"))
				})

				It("wraps and returns the error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'CheckCommitReadiness': underlying-error"))
				})
			})
		})

		Describe("QueryApprovedChaincodeDefinition", func() {
			var (
				arg          *lb.QueryApprovedChaincodeDefinitionArgs
				marshaledArg []byte
			)

			BeforeEach(func() {
				arg = &lb.QueryApprovedChaincodeDefinitionArgs{
					Sequence: 7,
					Name:     "cc_name",
				}

				var err error
				marshaledArg, err = proto.Marshal(arg)
				Expect(err).NotTo(HaveOccurred())

				fakeStub.GetArgsReturns([][]byte{[]byte("QueryApprovedChaincodeDefinition"), marshaledArg})
				fakeSCCFuncs.QueryApprovedChaincodeDefinitionReturns(
					&lifecycle.ApprovedChaincodeDefinition{
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
						Collections: &pb.CollectionConfigPackage{},
						Source: &lb.ChaincodeSource{
							Type: &lb.ChaincodeSource_LocalPackage{
								LocalPackage: &lb.ChaincodeSource_Local{
									PackageId: "hash",
								},
							},
						},
					},
					nil,
				)
			})

			It("passes the arguments to and returns the results from the backing scc function implementation", func() {
				res := scc.Invoke(fakeStub)
				Expect(res.Status).To(Equal(int32(200)))
				payload := &lb.QueryApprovedChaincodeDefinitionResult{}
				err := proto.Unmarshal(res.Payload, payload)
				Expect(err).NotTo(HaveOccurred())
				Expect(proto.Equal(payload, &lb.QueryApprovedChaincodeDefinitionResult{
					Sequence:            7,
					Version:             "version",
					EndorsementPlugin:   "endorsement-plugin",
					ValidationPlugin:    "validation-plugin",
					ValidationParameter: []byte("validation-parameter"),
					InitRequired:        true,
					Collections:         &pb.CollectionConfigPackage{},
					Source: &lb.ChaincodeSource{
						Type: &lb.ChaincodeSource_LocalPackage{
							LocalPackage: &lb.ChaincodeSource_Local{
								PackageId: "hash",
							},
						},
					},
				})).To(BeTrue())

				Expect(fakeSCCFuncs.QueryApprovedChaincodeDefinitionCallCount()).To(Equal(1))
				chname, ccname, sequence, pubState, privState := fakeSCCFuncs.QueryApprovedChaincodeDefinitionArgsForCall(0)

				Expect(chname).To(Equal("test-channel"))
				Expect(ccname).To(Equal("cc_name"))
				Expect(sequence).To(Equal(int64(7)))

				Expect(pubState).To(Equal(fakeStub))
				Expect(privState).To(BeAssignableToTypeOf(&lifecycle.ChaincodePrivateLedgerShim{}))
				Expect(privState.(*lifecycle.ChaincodePrivateLedgerShim).Collection).To(Equal("_implicit_org_fake-mspid"))
			})

			Context("when the underlying QueryApprovedChaincodeDefinition function implementation fails", func() {
				BeforeEach(func() {
					fakeSCCFuncs.QueryApprovedChaincodeDefinitionReturns(nil, fmt.Errorf("underlying-error"))
				})

				It("wraps and returns the error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'QueryApprovedChaincodeDefinition': underlying-error"))
				})
			})

			Context("when the underlying QueryApprovedChaincodeDefinition function implementation fails", func() {
				BeforeEach(func() {
					fakeSCCFuncs.QueryApprovedChaincodeDefinitionReturns(nil, fmt.Errorf("underlying-error"))
				})

				It("wraps and returns the error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'QueryApprovedChaincodeDefinition': underlying-error"))
				})
			})
		})

		Describe("QueryChaincodeDefinition", func() {
			var (
				arg            *lb.QueryChaincodeDefinitionArgs
				marshaledArg   []byte
				fakeOrgConfigs []*mock.ApplicationOrgConfig
			)

			BeforeEach(func() {
				arg = &lb.QueryChaincodeDefinitionArgs{
					Name: "cc-name",
				}

				var err error
				marshaledArg, err = proto.Marshal(arg)
				Expect(err).NotTo(HaveOccurred())

				fakeOrgConfigs = []*mock.ApplicationOrgConfig{{}, {}}
				fakeOrgConfigs[0].MSPIDReturns("fake-mspid")
				fakeOrgConfigs[1].MSPIDReturns("other-mspid")

				fakeApplicationConfig.OrganizationsReturns(map[string]channelconfig.ApplicationOrg{
					"org0": fakeOrgConfigs[0],
					"org1": fakeOrgConfigs[1],
				})

				fakeStub.GetArgsReturns([][]byte{[]byte("QueryChaincodeDefinition"), marshaledArg})
				fakeSCCFuncs.QueryChaincodeDefinitionReturns(
					&lifecycle.ChaincodeDefinition{
						Sequence: 2,
						EndorsementInfo: &lb.ChaincodeEndorsementInfo{
							Version:           "version",
							EndorsementPlugin: "endorsement-plugin",
						},
						ValidationInfo: &lb.ChaincodeValidationInfo{
							ValidationPlugin:    "validation-plugin",
							ValidationParameter: []byte("validation-parameter"),
						},
						Collections: &pb.CollectionConfigPackage{},
					},
					nil,
				)

				fakeSCCFuncs.QueryOrgApprovalsReturns(
					map[string]bool{
						"fake-mspid":  true,
						"other-mspid": true,
					},
					nil,
				)
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
					Collections:         &pb.CollectionConfigPackage{},
					Approvals: map[string]bool{
						"fake-mspid":  true,
						"other-mspid": true,
					},
				})).To(BeTrue())

				Expect(fakeSCCFuncs.QueryChaincodeDefinitionCallCount()).To(Equal(1))
				name, pubState := fakeSCCFuncs.QueryChaincodeDefinitionArgsForCall(0)
				Expect(name).To(Equal("cc-name"))
				Expect(pubState).To(Equal(fakeStub))
				name, _, orgStates := fakeSCCFuncs.QueryOrgApprovalsArgsForCall(0)
				Expect(name).To(Equal("cc-name"))
				Expect(orgStates).To(HaveLen(2))
				Expect(orgStates[0]).To(BeAssignableToTypeOf(&lifecycle.ChaincodePrivateLedgerShim{}))
				Expect(orgStates[1]).To(BeAssignableToTypeOf(&lifecycle.ChaincodePrivateLedgerShim{}))
				collection0 := orgStates[0].(*lifecycle.ChaincodePrivateLedgerShim).Collection
				collection1 := orgStates[1].(*lifecycle.ChaincodePrivateLedgerShim).Collection
				Expect([]string{collection0, collection1}).To(ConsistOf("_implicit_org_fake-mspid", "_implicit_org_other-mspid"))
			})

			Context("when the underlying QueryChaincodeDefinition function implementation fails", func() {
				BeforeEach(func() {
					fakeSCCFuncs.QueryChaincodeDefinitionReturns(nil, fmt.Errorf("underlying-error"))
				})

				It("wraps and returns the error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'QueryChaincodeDefinition': underlying-error"))
				})
			})

			Context("when the underlying QueryOrgApprovals function implementation fails", func() {
				BeforeEach(func() {
					fakeSCCFuncs.QueryOrgApprovalsReturns(nil, fmt.Errorf("underlying-error"))
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
						Expect(res.Message).To(Equal("failed to invoke backing implementation of 'QueryChaincodeDefinition': no application config for channel ''"))
					})
				})
			})
		})

		Describe("QueryChaincodeDefinitions", func() {
			var (
				arg          *lb.QueryChaincodeDefinitionsArgs
				marshaledArg []byte
			)

			BeforeEach(func() {
				arg = &lb.QueryChaincodeDefinitionsArgs{}

				var err error
				marshaledArg, err = proto.Marshal(arg)
				Expect(err).NotTo(HaveOccurred())

				fakeStub.GetArgsReturns([][]byte{[]byte("QueryChaincodeDefinitions"), marshaledArg})
				fakeSCCFuncs.QueryNamespaceDefinitionsReturns(map[string]string{
					"foo": "Chaincode",
					"bar": "Token",
					"woo": "Chaincode",
				}, nil)
				fakeSCCFuncs.QueryChaincodeDefinitionStub = func(name string, rs lifecycle.ReadableState) (*lifecycle.ChaincodeDefinition, error) {
					cd := &lifecycle.ChaincodeDefinition{
						Sequence: 2,
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

					if name == "woo" {
						cd.Sequence = 5
					}

					return cd, nil
				}
			})

			It("passes the arguments to and returns the results from the backing scc function implementation", func() {
				res := scc.Invoke(fakeStub)
				Expect(res.Status).To(Equal(int32(200)))
				payload := &lb.QueryChaincodeDefinitionsResult{}
				err := proto.Unmarshal(res.Payload, payload)
				Expect(err).NotTo(HaveOccurred())
				Expect(payload.GetChaincodeDefinitions()).To(ConsistOf(
					&lb.QueryChaincodeDefinitionsResult_ChaincodeDefinition{
						Name:                "foo",
						Sequence:            2,
						Version:             "version",
						EndorsementPlugin:   "endorsement-plugin",
						ValidationPlugin:    "validation-plugin",
						ValidationParameter: []byte("validation-parameter"),
						Collections:         &pb.CollectionConfigPackage{},
					},
					&lb.QueryChaincodeDefinitionsResult_ChaincodeDefinition{
						Name:                "woo",
						Sequence:            5,
						Version:             "version",
						EndorsementPlugin:   "endorsement-plugin",
						ValidationPlugin:    "validation-plugin",
						ValidationParameter: []byte("validation-parameter"),
						Collections:         &pb.CollectionConfigPackage{},
					},
				))
				Expect(fakeSCCFuncs.QueryNamespaceDefinitionsCallCount()).To(Equal(1))
				Expect(fakeSCCFuncs.QueryChaincodeDefinitionCallCount()).To(Equal(2))
			})

			Context("when the underlying QueryChaincodeDefinition function implementation fails", func() {
				BeforeEach(func() {
					fakeSCCFuncs.QueryChaincodeDefinitionReturns(nil, fmt.Errorf("underlying-error"))
				})

				It("wraps and returns the error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'QueryChaincodeDefinitions': underlying-error"))
				})
			})

			Context("when the underlying QueryNamespaceDefinitions function implementation fails", func() {
				BeforeEach(func() {
					fakeSCCFuncs.QueryNamespaceDefinitionsReturns(nil, fmt.Errorf("underlying-error"))
				})

				It("wraps and returns the error", func() {
					res := scc.Invoke(fakeStub)
					Expect(res.Status).To(Equal(int32(500)))
					Expect(res.Message).To(Equal("failed to invoke backing implementation of 'QueryChaincodeDefinitions': underlying-error"))
				})
			})
		})
	})
})

type collectionConfigs []*collectionConfig

func (ccs collectionConfigs) deepCopy() collectionConfigs {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	dec := gob.NewDecoder(&buf)

	err := enc.Encode(ccs)
	Expect(err).NotTo(HaveOccurred())
	var newCCs collectionConfigs
	err = dec.Decode(&newCCs)
	Expect(err).NotTo(HaveOccurred())
	return newCCs
}

func (ccs collectionConfigs) toProtoCollectionConfigPackage() *pb.CollectionConfigPackage {
	if len(ccs) == 0 {
		return nil
	}
	collConfigsProtos := make([]*pb.CollectionConfig, len(ccs))
	for i, c := range ccs {
		collConfigsProtos[i] = c.toCollectionConfigProto()
	}
	return &pb.CollectionConfigPackage{
		Config: collConfigsProtos,
	}
}

type collectionConfig struct {
	Name              string
	RequiredPeerCount int32
	MaxPeerCount      int32
	BlockToLive       uint64

	Policy                  string
	Identities              []*mspprotos.MSPPrincipal
	UseGivenMemberOrgPolicy bool
	MemberOrgPolicy         *pb.CollectionPolicyConfig
}

func (cc *collectionConfig) toCollectionConfigProto() *pb.CollectionConfig {
	memberOrgPolicy := cc.MemberOrgPolicy
	if !cc.UseGivenMemberOrgPolicy {
		spe, err := policydsl.FromString(cc.Policy)
		Expect(err).NotTo(HaveOccurred())
		spe.Identities = cc.Identities
		memberOrgPolicy = &pb.CollectionPolicyConfig{
			Payload: &pb.CollectionPolicyConfig_SignaturePolicy{
				SignaturePolicy: spe,
			},
		}
	}
	return &pb.CollectionConfig{
		Payload: &pb.CollectionConfig_StaticCollectionConfig{
			StaticCollectionConfig: &pb.StaticCollectionConfig{
				Name:              cc.Name,
				MaximumPeerCount:  cc.MaxPeerCount,
				RequiredPeerCount: cc.RequiredPeerCount,
				BlockToLive:       cc.BlockToLive,
				MemberOrgsPolicy:  memberOrgPolicy,
			},
		},
	}
}
