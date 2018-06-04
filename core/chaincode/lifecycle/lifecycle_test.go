/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle_test

import (
	lc "github.com/hyperledger/fabric/core/chaincode/lifecycle"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle/mock"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	pb "github.com/hyperledger/fabric/protos/peer"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
)

var _ = Describe("Lifecycle", func() {
	var (
		lifecycle               *lc.Lifecycle
		fakeInstantiatedCCStore *mock.InstantiatedChaincodeStore
	)

	BeforeEach(func() {
		fakeInstantiatedCCStore = &mock.InstantiatedChaincodeStore{}

		lifecycle = &lc.Lifecycle{
			InstantiatedChaincodeStore: fakeInstantiatedCCStore,
		}
	})

	Describe("ChaincodeContainerInfo", func() {
		var (
			deploymentSpec *pb.ChaincodeDeploymentSpec
		)

		BeforeEach(func() {
			deploymentSpec = &pb.ChaincodeDeploymentSpec{
				ChaincodeSpec: &pb.ChaincodeSpec{
					ChaincodeId: &pb.ChaincodeID{
						Name:    "chaincode-name",
						Version: "chaincode-version",
						Path:    "chaincode-path",
					},
					Type: pb.ChaincodeSpec_GOLANG,
				},
				ExecEnv: pb.ChaincodeDeploymentSpec_SYSTEM,
			}

			fakeInstantiatedCCStore.ChaincodeDeploymentSpecReturns(deploymentSpec, nil)
		})

		It("invokes lscc getdepspec with the correct args", func() {
			ccci, err := lifecycle.ChaincodeContainerInfo("chain-id", "chaincode-name")
			Expect(err).NotTo(HaveOccurred())
			Expect(ccci.Name).To(Equal("chaincode-name"))
			Expect(ccci.Version).To(Equal("chaincode-version"))
			Expect(ccci.Path).To(Equal("chaincode-path"))
			Expect(ccci.Type).To(Equal("GOLANG"))
			Expect(ccci.ContainerType).To(Equal("SYSTEM"))

			Expect(fakeInstantiatedCCStore.ChaincodeDeploymentSpecCallCount()).To(Equal(1))
			channelID, chaincodeName := fakeInstantiatedCCStore.ChaincodeDeploymentSpecArgsForCall(0)
			Expect(channelID).To(Equal("chain-id"))
			Expect(chaincodeName).To(Equal("chaincode-name"))
		})

		Context("when the instantiated chaincode store fails", func() {
			BeforeEach(func() {
				fakeInstantiatedCCStore.ChaincodeDeploymentSpecReturns(nil, errors.New("mango-tango"))
			})

			It("returns a wrapped error", func() {
				_, err := lifecycle.ChaincodeContainerInfo("chain-id", "chaincode-id")
				Expect(err).To(MatchError("could not retrieve deployment spec for chain-id/chaincode-id: mango-tango"))
			})
		})
	})

	Describe("GetChaincodeDefinition", func() {
		var (
			chaincodeData *ccprovider.ChaincodeData
		)

		BeforeEach(func() {
			chaincodeData = &ccprovider.ChaincodeData{
				Name: "chaincode-data-name",
			}

			fakeInstantiatedCCStore.ChaincodeDefinitionReturns(chaincodeData, errors.New("fake-error"))
		})

		It("passes through to the underlying implementation", func() {
			chaincodeDefinition, err := lifecycle.GetChaincodeDefinition("foo", nil)
			Expect(chaincodeDefinition.(*ccprovider.ChaincodeData)).To(Equal(chaincodeData))
			Expect(err).To(MatchError("fake-error"))
			Expect(fakeInstantiatedCCStore.ChaincodeDefinitionCallCount()).To(Equal(1))
			ccNameArg, txSimArg := fakeInstantiatedCCStore.ChaincodeDefinitionArgsForCall(0)
			Expect(ccNameArg).To(Equal("foo"))
			Expect(txSimArg).To(BeNil())
		})
	})
})
