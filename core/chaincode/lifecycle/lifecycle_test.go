/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle_test

import (
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/util"
	lc "github.com/hyperledger/fabric/core/chaincode/lifecycle"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle/mock"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	pb "github.com/hyperledger/fabric/protos/peer"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

var _ = Describe("Lifecycle", func() {
	var (
		lifecycle *lc.Lifecycle
	)

	BeforeEach(func() {
	})

	Describe("GetChaincodeDeploymentSpec", func() {
		var (
			fakeInstantiatedCCStore *mock.InstantiatedChaincodeStore
			deploymentSpec          *pb.ChaincodeDeploymentSpec
		)

		BeforeEach(func() {
			chaincodeID := &pb.ChaincodeID{Name: "chaincode-name", Version: "chaincode-version"}
			deploymentSpec = &pb.ChaincodeDeploymentSpec{
				CodePackage:   []byte("code-package"),
				ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: chaincodeID},
			}

			fakeInstantiatedCCStore = &mock.InstantiatedChaincodeStore{}
			fakeInstantiatedCCStore.ChaincodeDeploymentSpecReturns(deploymentSpec, nil)

			lifecycle = &lc.Lifecycle{
				InstantiatedChaincodeStore: fakeInstantiatedCCStore,
			}
		})

		It("invokes lscc getdepspec with the correct args", func() {
			cds, err := lifecycle.GetChaincodeDeploymentSpec("chain-id", "chaincode-name")
			Expect(err).NotTo(HaveOccurred())
			Expect(proto.Equal(cds, deploymentSpec)).To(BeTrue())

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
				_, err := lifecycle.GetChaincodeDeploymentSpec("chain-id", "chaincode-id")
				Expect(err).To(MatchError("could not retrieve deployment spec for chain-id/chaincode-id: mango-tango"))
			})
		})
	})

	Describe("GetChaincodeDefinition", func() {
		var (
			chaincodeData *ccprovider.ChaincodeData

			fakeExecutor *mock.Executor
			signedProp   *pb.SignedProposal
			proposal     *pb.Proposal
		)

		BeforeEach(func() {
			fakeExecutor = &mock.Executor{}
			signedProp = &pb.SignedProposal{ProposalBytes: []byte("some-proposal-bytes")}
			proposal = &pb.Proposal{Payload: []byte("some-payload-bytes")}

			lifecycle = &lc.Lifecycle{
				Executor: fakeExecutor,
			}

			chaincodeData = &ccprovider.ChaincodeData{
				Name:    "george",
				Version: "old",
			}
			payload, err := proto.Marshal(chaincodeData)
			Expect(err).NotTo(HaveOccurred())

			response := &pb.Response{
				Status:  shim.OK,
				Payload: payload,
			}
			fakeExecutor.ExecuteReturns(response, nil, nil)
		})

		It("invokes lscc getccdata with the correct args", func() {
			cd, err := lifecycle.GetChaincodeDefinition(context.Background(), "tx-id", signedProp, proposal, "chain-id", "chaincode-id")
			Expect(err).NotTo(HaveOccurred())
			Expect(cd).To(Equal(chaincodeData))

			Expect(fakeExecutor.ExecuteCallCount()).To(Equal(1))
			ctx, cccid, cis := fakeExecutor.ExecuteArgsForCall(0)
			Expect(ctx).To(Equal(context.Background()))
			Expect(cccid).To(Equal(ccprovider.NewCCContext("chain-id", "lscc", "latest", "tx-id", true, signedProp, proposal)))
			Expect(cis).To(Equal(&pb.ChaincodeInvocationSpec{
				ChaincodeSpec: &pb.ChaincodeSpec{
					Type:        pb.ChaincodeSpec_GOLANG,
					ChaincodeId: &pb.ChaincodeID{Name: "lscc"},
					Input: &pb.ChaincodeInput{
						Args: util.ToChaincodeArgs("getccdata", "chain-id", "chaincode-id"),
					},
				},
			}))
		})

		Context("when the executor fails", func() {
			BeforeEach(func() {
				fakeExecutor.ExecuteReturns(nil, nil, errors.New("mango-tango"))
			})

			It("returns a wrapped error", func() {
				_, err := lifecycle.GetChaincodeDefinition(context.Background(), "tx-id", signedProp, proposal, "chain-id", "chaincode-id")
				Expect(err).To(MatchError("getccdata chain-id/chaincode-id failed: mango-tango"))
			})
		})

		Context("when the executor returns an error response", func() {
			BeforeEach(func() {
				response := &pb.Response{
					Status:  shim.ERROR,
					Message: "danger-danger",
				}
				fakeExecutor.ExecuteReturns(response, nil, nil)
			})

			It("returns a wrapped error", func() {
				_, err := lifecycle.GetChaincodeDefinition(context.Background(), "tx-id", signedProp, proposal, "chain-id", "chaincode-id")
				Expect(err).To(MatchError("getccdata chain-id/chaincode-id responded with error: danger-danger"))
			})
		})

		Context("when unmarshaling the response fails", func() {
			BeforeEach(func() {
				response := &pb.Response{
					Status:  shim.OK,
					Payload: []byte("totally-bogus-payload"),
				}
				fakeExecutor.ExecuteReturns(response, nil, nil)
			})

			It("returns a wrapped error", func() {
				_, err := lifecycle.GetChaincodeDefinition(context.Background(), "tx-id", signedProp, proposal, "chain-id", "chaincode-id")
				Expect(err).To(MatchError(HavePrefix("failed to unmarshal chaincode definition: proto: ")))
			})
		})
	})
})
