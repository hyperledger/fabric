/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle_test

import (
	"github.com/hyperledger/fabric-chaincode-go/shim"
	"github.com/hyperledger/fabric-protos-go/ledger/queryresult"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	lb "github.com/hyperledger/fabric-protos-go/peer/lifecycle"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle/mock"
	"github.com/hyperledger/fabric/core/dispatcher"
	"github.com/hyperledger/fabric/protoutil"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/golang/protobuf/proto"
)

var _ = Describe("Integration", func() {
	var (
		resources *lifecycle.Resources
		ef        *lifecycle.ExternalFunctions
		scc       *lifecycle.SCC

		fakeChannelConfigSource    *mock.ChannelConfigSource
		fakeChannelConfig          *mock.ChannelConfig
		fakeApplicationConfig      *mock.ApplicationConfig
		fakeCapabilities           *mock.ApplicationCapabilities
		fakeOrgConfig              *mock.ApplicationOrgConfig
		fakeStub                   *mock.ChaincodeStub
		fakeACLProvider            *mock.ACLProvider
		fakeMSPManager             *mock.MSPManager
		fakeQueryExecutorProvider  *mock.QueryExecutorProvider
		fakeQueryExecutor          *mock.SimpleQueryExecutor
		fakeDeployedCCInfoProvider *mock.LegacyDeployedCCInfoProvider

		fakeOrgKVStore    map[string][]byte
		fakePublicKVStore map[string][]byte
	)

	BeforeEach(func() {
		resources = &lifecycle.Resources{
			Serializer: &lifecycle.Serializer{},
		}

		ef = &lifecycle.ExternalFunctions{
			Resources: resources,
		}

		fakeChannelConfigSource = &mock.ChannelConfigSource{}
		fakeChannelConfig = &mock.ChannelConfig{}
		fakeChannelConfigSource.GetStableChannelConfigReturns(fakeChannelConfig)
		fakeApplicationConfig = &mock.ApplicationConfig{}
		fakeChannelConfig.ApplicationConfigReturns(fakeApplicationConfig, true)
		fakeCapabilities = &mock.ApplicationCapabilities{}
		fakeCapabilities.LifecycleV20Returns(true)
		fakeApplicationConfig.CapabilitiesReturns(fakeCapabilities)
		fakeACLProvider = &mock.ACLProvider{}

		fakeOrgConfig = &mock.ApplicationOrgConfig{}
		fakeOrgConfig.MSPIDReturns("fake-mspid")

		fakeMSPManager = &mock.MSPManager{}
		fakeChannelConfig.MSPManagerReturns(fakeMSPManager)
		fakeQueryExecutorProvider = &mock.QueryExecutorProvider{}
		fakeQueryExecutor = &mock.SimpleQueryExecutor{}
		fakeQueryExecutorProvider.TxQueryExecutorReturns(fakeQueryExecutor)
		fakeDeployedCCInfoProvider = &mock.LegacyDeployedCCInfoProvider{}

		fakeApplicationConfig.OrganizationsReturns(map[string]channelconfig.ApplicationOrg{
			"fakeOrg": fakeOrgConfig,
		})

		scc = &lifecycle.SCC{
			Dispatcher: &dispatcher.Dispatcher{
				Protobuf: &dispatcher.ProtobufImpl{},
			},
			Functions:              ef,
			OrgMSPID:               "fake-mspid",
			ChannelConfigSource:    fakeChannelConfigSource,
			ACLProvider:            fakeACLProvider,
			QueryExecutorProvider:  fakeQueryExecutorProvider,
			DeployedCCInfoProvider: fakeDeployedCCInfoProvider,
		}

		fakePublicKVStore = map[string][]byte{}
		fakeOrgKVStore = map[string][]byte{}

		fakeStub = &mock.ChaincodeStub{}

		fakeStub.GetChannelIDReturns("test-channel")

		fakeStub.GetStateStub = func(key string) ([]byte, error) {
			return fakePublicKVStore[key], nil
		}
		fakeStub.PutStateStub = func(key string, value []byte) error {
			fakePublicKVStore[key] = value
			return nil
		}
		fakeStub.GetStateByRangeStub = func(begin, end string) (shim.StateQueryIteratorInterface, error) {
			fakeIterator := &mock.StateIterator{}
			i := 0
			for key, value := range fakePublicKVStore {
				if key >= begin && key < end {
					fakeIterator.HasNextReturnsOnCall(i, true)
					fakeIterator.NextReturnsOnCall(i, &queryresult.KV{
						Key:   key,
						Value: value,
					}, nil)
					i++
				}
			}
			return fakeIterator, nil
		}

		fakeStub.PutPrivateDataStub = func(collection, key string, value []byte) error {
			fakeOrgKVStore[key] = value
			return nil
		}

		fakeStub.GetPrivateDataStub = func(collection, key string) ([]byte, error) {
			return fakeOrgKVStore[key], nil
		}

		fakeStub.GetPrivateDataHashStub = func(collection, key string) ([]byte, error) {
			return util.ComputeSHA256(fakeOrgKVStore[key]), nil
		}
	})

	Describe("Instantiation", func() {
		It("defines the chaincode for the org, defines it for the channel, queries all namespaces, and queries the chaincode", func() {
			// Define for the org
			fakeStub.GetArgsReturns([][]byte{
				[]byte("ApproveChaincodeDefinitionForMyOrg"),
				protoutil.MarshalOrPanic(&lb.ApproveChaincodeDefinitionForMyOrgArgs{
					Name:                "cc-name",
					Version:             "1.0",
					Sequence:            1,
					EndorsementPlugin:   "builtin",
					ValidationPlugin:    "builtin",
					ValidationParameter: []byte("validation-parameter"),
					Source: &lb.ChaincodeSource{
						Type: &lb.ChaincodeSource_LocalPackage{
							LocalPackage: &lb.ChaincodeSource_Local{
								PackageId: "hash-value",
							},
						},
					},
				}),
			})
			response := scc.Invoke(fakeStub)
			Expect(response.Status).To(Equal(int32(200)))

			// Define for the channel
			fakeStub.GetArgsReturns([][]byte{
				[]byte("CommitChaincodeDefinition"),
				protoutil.MarshalOrPanic(&lb.CommitChaincodeDefinitionArgs{
					Name:                "cc-name",
					Version:             "1.0",
					Sequence:            1,
					EndorsementPlugin:   "builtin",
					ValidationPlugin:    "builtin",
					ValidationParameter: []byte("validation-parameter"),
				}),
			})
			response = scc.Invoke(fakeStub)
			Expect(response.Message).To(Equal(""))
			Expect(response.Status).To(Equal(int32(200)))

			// Get channel definitions
			fakeStub.GetArgsReturns([][]byte{
				[]byte("QueryChaincodeDefinitions"),
				protoutil.MarshalOrPanic(&lb.QueryChaincodeDefinitionsArgs{}),
			})
			response = scc.Invoke(fakeStub)
			Expect(response.Status).To(Equal(int32(200)))
			definitionsResult := &lb.QueryChaincodeDefinitionsResult{}
			err := proto.Unmarshal(response.Payload, definitionsResult)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(definitionsResult.ChaincodeDefinitions)).To(Equal(1))
			Expect(definitionsResult.ChaincodeDefinitions[0].Name).To(Equal("cc-name"))

			// Get chaincode definition details
			fakeStub.GetArgsReturns([][]byte{
				[]byte("QueryChaincodeDefinition"),
				protoutil.MarshalOrPanic(&lb.QueryChaincodeDefinitionArgs{
					Name: "cc-name",
				}),
			})
			response = scc.Invoke(fakeStub)
			Expect(response.Status).To(Equal(int32(200)))
			chaincodeResult := &lb.QueryChaincodeDefinitionResult{}
			err = proto.Unmarshal(response.Payload, chaincodeResult)
			Expect(err).NotTo(HaveOccurred())
			Expect(proto.Equal(chaincodeResult, &lb.QueryChaincodeDefinitionResult{
				Sequence:            1,
				Version:             "1.0",
				EndorsementPlugin:   "builtin",
				ValidationPlugin:    "builtin",
				ValidationParameter: []byte("validation-parameter"),
				Collections:         &pb.CollectionConfigPackage{},
				Approvals: map[string]bool{
					"fake-mspid": true,
				},
			})).To(BeTrue())
		})
	})
})
