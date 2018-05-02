/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode_test

import (
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/hyperledger/fabric/core/chaincode/mock"
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
		fakeExecutor *mock.Executor
		signedProp   *pb.SignedProposal
		proposal     *pb.Proposal

		lifecycle *chaincode.Lifecycle
	)

	BeforeEach(func() {
		fakeExecutor = &mock.Executor{}
		signedProp = &pb.SignedProposal{ProposalBytes: []byte("some-proposal-bytes")}
		proposal = &pb.Proposal{Payload: []byte("some-payload-bytes")}
		lifecycle = &chaincode.Lifecycle{
			Executor: fakeExecutor,
		}
	})

	Describe("GetChaincodeDeploymentSpec", func() {
		var deploymentSpec *pb.ChaincodeDeploymentSpec

		BeforeEach(func() {
			chaincodeID := &pb.ChaincodeID{Name: "chaincode-name", Version: "chaincode-version"}
			deploymentSpec = &pb.ChaincodeDeploymentSpec{
				CodePackage:   []byte("code-package"),
				ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: chaincodeID},
			}
			deploymentSpecPayload, err := proto.Marshal(deploymentSpec)
			Expect(err).NotTo(HaveOccurred())

			response := &pb.Response{Status: shim.OK, Payload: deploymentSpecPayload}
			fakeExecutor.ExecuteReturns(response, nil, nil)
		})

		It("invokes lscc getdepspec with the correct args", func() {
			cds, err := lifecycle.GetChaincodeDeploymentSpec(context.Background(), "tx-id", signedProp, proposal, "chain-id", "chaincode-id")
			Expect(err).NotTo(HaveOccurred())
			Expect(cds).To(Equal(deploymentSpec))

			Expect(fakeExecutor.ExecuteCallCount()).To(Equal(1))
			ctx, cccid, cis := fakeExecutor.ExecuteArgsForCall(0)
			Expect(ctx).To(Equal(context.Background()))
			Expect(cccid).To(Equal(ccprovider.NewCCContext("chain-id", "lscc", "latest", "tx-id", true, signedProp, proposal)))
			Expect(cis).To(Equal(&pb.ChaincodeInvocationSpec{
				ChaincodeSpec: &pb.ChaincodeSpec{
					Type:        pb.ChaincodeSpec_GOLANG,
					ChaincodeId: &pb.ChaincodeID{Name: "lscc"},
					Input: &pb.ChaincodeInput{
						Args: util.ToChaincodeArgs("getdepspec", "chain-id", "chaincode-id"),
					},
				},
			}))
		})

		Context("when the executor fails", func() {
			BeforeEach(func() {
				fakeExecutor.ExecuteReturns(nil, nil, errors.New("mango-tango"))
			})

			It("returns a wrapped error", func() {
				_, err := lifecycle.GetChaincodeDeploymentSpec(context.Background(), "tx-id", signedProp, proposal, "chain-id", "chaincode-id")
				Expect(err).To(MatchError("getdepspec chain-id/chaincode-id failed: mango-tango"))
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
				_, err := lifecycle.GetChaincodeDeploymentSpec(context.Background(), "tx-id", signedProp, proposal, "chain-id", "chaincode-id")
				Expect(err).To(MatchError("getdepspec chain-id/chaincode-id responded with error: danger-danger"))
			})
		})

		Context("when the response contains a nil payload", func() {
			BeforeEach(func() {
				response := &pb.Response{Status: shim.OK, Payload: nil}
				fakeExecutor.ExecuteReturns(response, nil, nil)
			})

			It("returns a wrapped error", func() {
				_, err := lifecycle.GetChaincodeDeploymentSpec(context.Background(), "tx-id", signedProp, proposal, "chain-id", "chaincode-id")
				Expect(err).To(MatchError("getdepspec chain-id/chaincode-id failed: payload is nil"))
			})
		})

		Context("when unmarshaling the payload fails", func() {
			BeforeEach(func() {
				response := &pb.Response{Status: shim.OK, Payload: []byte("bogus-payload")}
				fakeExecutor.ExecuteReturns(response, nil, nil)
			})

			It("returns a wrapped error", func() {
				_, err := lifecycle.GetChaincodeDeploymentSpec(context.Background(), "tx-id", signedProp, proposal, "chain-id", "chaincode-id")
				Expect(err).To(MatchError(HavePrefix("failed to unmarshal deployment spec payload for chain-id/chaincode-id")))
			})
		})
	})

	Describe("GetChaincodeDefinition", func() {
		var chaincodeData *ccprovider.ChaincodeData

		BeforeEach(func() {
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
