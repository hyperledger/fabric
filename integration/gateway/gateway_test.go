/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gateway

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/gateway"
	"github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric-protos-go/peer/lifecycle"
	"github.com/hyperledger/fabric/integration/channelparticipation"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/protoutil"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/tedsuo/ifrit"
	ginkgomon "github.com/tedsuo/ifrit/ginkgomon_v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func NewProposedTransaction(signingIdentity *nwo.SigningIdentity, channelName, chaincodeName, transactionName string, transientData map[string][]byte, args ...[]byte) (*peer.SignedProposal, string) {
	proposal, transactionID := newProposalProto(signingIdentity, channelName, chaincodeName, transactionName, transientData, args...)
	signedProposal, err := protoutil.GetSignedProposal(proposal, signingIdentity)
	Expect(err).NotTo(HaveOccurred())

	return signedProposal, transactionID
}

func newProposalProto(signingIdentity *nwo.SigningIdentity, channelName, chaincodeName, transactionName string, transientData map[string][]byte, args ...[]byte) (*peer.Proposal, string) {
	creator, err := signingIdentity.Serialize()
	Expect(err).NotTo(HaveOccurred())

	invocationSpec := &peer.ChaincodeInvocationSpec{
		ChaincodeSpec: &peer.ChaincodeSpec{
			Type:        peer.ChaincodeSpec_NODE,
			ChaincodeId: &peer.ChaincodeID{Name: chaincodeName},
			Input:       &peer.ChaincodeInput{Args: chaincodeArgs(transactionName, args...)},
		},
	}

	result, transactionID, err := protoutil.CreateChaincodeProposalWithTransient(
		common.HeaderType_ENDORSER_TRANSACTION,
		channelName,
		invocationSpec,
		creator,
		transientData,
	)
	Expect(err).NotTo(HaveOccurred())

	return result, transactionID
}

func chaincodeArgs(transactionName string, args ...[]byte) [][]byte {
	result := make([][]byte, len(args)+1)

	result[0] = []byte(transactionName)
	copy(result[1:], args)

	return result
}

var _ = Describe("GatewayService basic", func() {
	var (
		testDir                     string
		network                     *nwo.Network
		org1Peer0                   *nwo.Peer
		ordererRunner               *ginkgomon.Runner
		ordererProcess, peerProcess ifrit.Process
		conn                        *grpc.ClientConn
		gatewayClient               gateway.GatewayClient
		ctx                         context.Context
		cancel                      context.CancelFunc
		signingIdentity             *nwo.SigningIdentity
	)

	BeforeEach(func() {
		var err error
		testDir, err = ioutil.TempDir("", "gateway")
		Expect(err).NotTo(HaveOccurred())

		client, err := docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())

		config := nwo.BasicEtcdRaftNoSysChan()
		network = nwo.New(config, testDir, client, StartPort(), components)

		network.GenerateConfigTree()
		network.Bootstrap()

		// Start all the fabric processes
		ordererRunner, ordererProcess, peerProcess = network.StartSingleOrdererNetwork("orderer")

		By("setting up the channel")
		orderer := network.Orderer("orderer")
		channelparticipation.JoinOrdererJoinPeersAppChannel(network, "testchannel", orderer, ordererRunner)

		network.VerifyMembership(
			network.PeersWithChannel("testchannel"),
			"testchannel",
		)
		nwo.EnableCapabilities(
			network,
			"testchannel",
			"Application", "V2_0",
			orderer,
			network.PeersWithChannel("testchannel")...,
		)

		chaincode := nwo.Chaincode{
			Name:            "gatewaycc",
			Version:         "0.0",
			Path:            components.Build("github.com/hyperledger/fabric/integration/chaincode/simple/cmd"),
			Lang:            "binary",
			PackageFile:     filepath.Join(testDir, "gatewaycc.tar.gz"),
			Ctor:            `{"Args":[]}`,
			SignaturePolicy: `AND ('Org1MSP.peer')`,
			Sequence:        "1",
			InitRequired:    false,
			Label:           "gatewaycc_label",
		}

		nwo.DeployChaincode(network, "testchannel", orderer, chaincode)

		org1Peer0 = network.Peer("Org1", "peer0")

		conn = network.PeerClientConn(org1Peer0)
		gatewayClient = gateway.NewGatewayClient(conn)
		ctx, cancel = context.WithTimeout(context.Background(), network.EventuallyTimeout)

		signingIdentity = network.PeerUserSigner(org1Peer0, "User1")
	})

	AfterEach(func() {
		conn.Close()
		cancel()

		if ordererProcess != nil {
			ordererProcess.Signal(syscall.SIGTERM)
			Eventually(ordererProcess.Wait(), network.EventuallyTimeout).Should(Receive())
		}

		if peerProcess != nil {
			peerProcess.Signal(syscall.SIGTERM)
			Eventually(peerProcess.Wait(), network.EventuallyTimeout).Should(Receive())
		}

		if network != nil {
			network.Cleanup()
		}
		os.RemoveAll(testDir)
	})

	submitTransaction := func(transactionName string, args ...[]byte) (*peer.Response, string) {
		proposedTransaction, transactionID := NewProposedTransaction(signingIdentity, "testchannel", "gatewaycc", transactionName, nil, args...)

		endorseRequest := &gateway.EndorseRequest{
			TransactionId:       transactionID,
			ChannelId:           "testchannel",
			ProposedTransaction: proposedTransaction,
		}

		endorseResponse, err := gatewayClient.Endorse(ctx, endorseRequest)
		Expect(err).NotTo(HaveOccurred())

		preparedTransaction := endorseResponse.GetPreparedTransaction()
		preparedTransaction.Signature, err = signingIdentity.Sign(preparedTransaction.Payload)
		Expect(err).NotTo(HaveOccurred())

		submitRequest := &gateway.SubmitRequest{
			TransactionId:       transactionID,
			ChannelId:           "testchannel",
			PreparedTransaction: preparedTransaction,
		}
		_, err = gatewayClient.Submit(ctx, submitRequest)
		Expect(err).NotTo(HaveOccurred())

		chaincodeAction, err := protoutil.GetActionFromEnvelopeMsg(endorseResponse.GetPreparedTransaction())
		Expect(err).NotTo(HaveOccurred())

		return chaincodeAction.GetResponse(), transactionID
	}

	commitStatus := func(transactionID string, identity func() ([]byte, error), sign func(msg []byte) ([]byte, error)) (*gateway.CommitStatusResponse, error) {
		idBytes, err := identity()
		Expect(err).NotTo(HaveOccurred())

		statusRequest := &gateway.CommitStatusRequest{
			ChannelId:     "testchannel",
			Identity:      idBytes,
			TransactionId: transactionID,
		}
		statusRequestBytes, err := proto.Marshal(statusRequest)
		Expect(err).NotTo(HaveOccurred())

		signature, err := sign(statusRequestBytes)
		Expect(err).NotTo(HaveOccurred())

		signedStatusRequest := &gateway.SignedCommitStatusRequest{
			Request:   statusRequestBytes,
			Signature: signature,
		}

		return gatewayClient.CommitStatus(ctx, signedStatusRequest)
	}

	chaincodeEvents := func(
		ctx context.Context,
		startPosition *orderer.SeekPosition,
		afterTxID string,
		identity func() ([]byte, error),
		sign func(msg []byte) ([]byte, error),
	) (gateway.Gateway_ChaincodeEventsClient, error) {
		identityBytes, err := identity()
		Expect(err).NotTo(HaveOccurred())

		request := &gateway.ChaincodeEventsRequest{
			ChannelId:   "testchannel",
			ChaincodeId: "gatewaycc",
			Identity:    identityBytes,
		}
		if startPosition != nil {
			request.StartPosition = startPosition
		}
		if len(afterTxID) > 0 {
			request.AfterTransactionId = afterTxID
		}

		requestBytes, err := proto.Marshal(request)
		Expect(err).NotTo(HaveOccurred())

		signature, err := sign(requestBytes)
		Expect(err).NotTo(HaveOccurred())

		signedRequest := &gateway.SignedChaincodeEventsRequest{
			Request:   requestBytes,
			Signature: signature,
		}

		return gatewayClient.ChaincodeEvents(ctx, signedRequest)
	}

	Describe("Evaluate", func() {
		It("should respond with the expected result", func() {
			proposedTransaction, transactionID := NewProposedTransaction(signingIdentity, "testchannel", "gatewaycc", "respond", nil, []byte("200"), []byte("conga message"), []byte("conga payload"))

			request := &gateway.EvaluateRequest{
				TransactionId:       transactionID,
				ChannelId:           "testchannel",
				ProposedTransaction: proposedTransaction,
			}

			response, err := gatewayClient.Evaluate(ctx, request)
			Expect(err).NotTo(HaveOccurred())
			expectedResponse := &gateway.EvaluateResponse{
				Result: &peer.Response{
					Status:  200,
					Message: "conga message",
					Payload: []byte("conga payload"),
				},
			}
			Expect(response.Result.Payload).To(Equal(expectedResponse.Result.Payload))
			Expect(proto.Equal(response, expectedResponse)).To(BeTrue(), "Expected\n\t%#v\nto proto.Equal\n\t%#v", response, expectedResponse)
		})

		It("should respond with system chaincode result", func() {
			proposedTransaction, transactionID := NewProposedTransaction(signingIdentity, "testchannel", "qscc", "GetChainInfo", nil, []byte("testchannel"))

			request := &gateway.EvaluateRequest{
				TransactionId:       transactionID,
				ChannelId:           "testchannel",
				ProposedTransaction: proposedTransaction,
			}

			response, err := gatewayClient.Evaluate(ctx, request)
			Expect(err).NotTo(HaveOccurred())

			status := common.Status(response.GetResult().GetStatus())
			Expect(status).To(Equal(common.Status_SUCCESS))

			blockchainInfo := new(common.BlockchainInfo)
			Expect(proto.Unmarshal(response.GetResult().GetPayload(), blockchainInfo)).NotTo(HaveOccurred())
		})
	})

	Describe("Submit", func() {
		It("should respond with the expected result", func() {
			result, _ := submitTransaction("respond", []byte("200"), []byte("conga message"), []byte("conga payload"))
			expectedResult := &peer.Response{
				Status:  200,
				Message: "conga message",
				Payload: []byte("conga payload"),
			}
			Expect(result.Payload).To(Equal(expectedResult.Payload))
			Expect(proto.Equal(result, expectedResult)).To(BeTrue(), "Expected\n\t%#v\nto proto.Equal\n\t%#v", result, expectedResult)
		})

		It("should endorse a system chaincode transaction", func() {
			arg, err := proto.Marshal(&lifecycle.QueryInstalledChaincodesArgs{})
			Expect(err).NotTo(HaveOccurred())
			adminSigner := network.PeerUserSigner(org1Peer0, "Admin")
			proposedTransaction, transactionID := NewProposedTransaction(adminSigner, "testchannel", "_lifecycle", "QueryInstalledChaincodes", nil, arg)

			request := &gateway.EndorseRequest{
				TransactionId:          transactionID,
				ChannelId:              "testchannel",
				ProposedTransaction:    proposedTransaction,
				EndorsingOrganizations: []string{adminSigner.MSPID}, // Only use peers for our admin ID org
			}

			response, err := gatewayClient.Endorse(ctx, request)
			Expect(err).NotTo(HaveOccurred())

			chaincodeAction, err := protoutil.GetActionFromEnvelopeMsg(response.GetPreparedTransaction())
			Expect(err).NotTo(HaveOccurred())

			queryResult := new(lifecycle.QueryInstalledChaincodesResult)
			Expect(proto.Unmarshal(chaincodeAction.GetResponse().GetPayload(), queryResult)).NotTo(HaveOccurred())
		})
	})

	Describe("CommitStatus", func() {
		It("should respond with status of submitted transaction", func() {
			_, transactionID := submitTransaction("respond", []byte("200"), []byte("conga message"), []byte("conga payload"))
			statusResult, err := commitStatus(transactionID, signingIdentity.Serialize, signingIdentity.Sign)
			Expect(err).NotTo(HaveOccurred())

			Expect(statusResult.Result).To(Equal(peer.TxValidationCode_VALID))
		})

		It("should respond with block number", func() {
			_, transactionID := submitTransaction("respond", []byte("200"), []byte("conga message"), []byte("conga payload"))
			firstStatus, err := commitStatus(transactionID, signingIdentity.Serialize, signingIdentity.Sign)
			Expect(err).NotTo(HaveOccurred())

			_, transactionID = submitTransaction("respond", []byte("200"), []byte("conga message"), []byte("conga payload"))
			nextStatus, err := commitStatus(transactionID, signingIdentity.Serialize, signingIdentity.Sign)
			Expect(err).NotTo(HaveOccurred())

			Expect(nextStatus.BlockNumber).To(Equal(firstStatus.BlockNumber + 1))
		})

		It("should fail on unauthorized identity", func() {
			_, transactionID := submitTransaction("respond", []byte("200"), []byte("conga message"), []byte("conga payload"))
			badIdentity := network.OrdererUserSigner(network.Orderer("orderer"), "Admin")
			_, err := commitStatus(transactionID, badIdentity.Serialize, signingIdentity.Sign)
			Expect(err).To(HaveOccurred())

			grpcErr, _ := status.FromError(err)
			Expect(grpcErr.Code()).To(Equal(codes.PermissionDenied))
		})

		It("should fail on bad signature", func() {
			_, transactionID := submitTransaction("respond", []byte("200"), []byte("conga message"), []byte("conga payload"))
			badSign := func(digest []byte) ([]byte, error) {
				return signingIdentity.Sign([]byte("WRONG"))
			}
			_, err := commitStatus(transactionID, signingIdentity.Serialize, badSign)
			Expect(err).To(HaveOccurred())

			grpcErr, _ := status.FromError(err)
			Expect(grpcErr.Code()).To(Equal(codes.PermissionDenied))
		})
	})

	Describe("ChaincodeEvents", func() {
		It("should respond with emitted chaincode events", func() {
			eventCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()

			startPosition := &orderer.SeekPosition{
				Type: &orderer.SeekPosition_NextCommit{
					NextCommit: &orderer.SeekNextCommit{},
				},
			}

			eventsClient, err := chaincodeEvents(eventCtx, startPosition, "", signingIdentity.Serialize, signingIdentity.Sign)
			Expect(err).NotTo(HaveOccurred())

			_, transactionID := submitTransaction("event", []byte("EVENT_NAME"), []byte("EVENT_PAYLOAD"))

			event, err := eventsClient.Recv()
			Expect(err).NotTo(HaveOccurred())

			Expect(event.Events).To(HaveLen(1), "number of events")
			expectedEvent := &peer.ChaincodeEvent{
				ChaincodeId: "gatewaycc",
				TxId:        transactionID,
				EventName:   "EVENT_NAME",
				Payload:     []byte("EVENT_PAYLOAD"),
			}
			Expect(proto.Equal(event.Events[0], expectedEvent)).To(BeTrue(), "Expected\n\t%#v\nto proto.Equal\n\t%#v", event.Events[0], expectedEvent)
		})

		It("should respond with replayed chaincode events", func() {
			_, transactionID := submitTransaction("event", []byte("EVENT_NAME"), []byte("EVENT_PAYLOAD"))
			statusResult, err := commitStatus(transactionID, signingIdentity.Serialize, signingIdentity.Sign)
			Expect(err).NotTo(HaveOccurred())

			eventCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()

			startPosition := &orderer.SeekPosition{
				Type: &orderer.SeekPosition_Specified{
					Specified: &orderer.SeekSpecified{
						Number: statusResult.BlockNumber,
					},
				},
			}

			eventsClient, err := chaincodeEvents(eventCtx, startPosition, "", signingIdentity.Serialize, signingIdentity.Sign)
			Expect(err).NotTo(HaveOccurred())

			event, err := eventsClient.Recv()
			Expect(err).NotTo(HaveOccurred())

			Expect(event.BlockNumber).To(Equal(statusResult.BlockNumber), "block number")
			Expect(event.Events).To(HaveLen(1), "number of events")
			expectedEvent := &peer.ChaincodeEvent{
				ChaincodeId: "gatewaycc",
				TxId:        transactionID,
				EventName:   "EVENT_NAME",
				Payload:     []byte("EVENT_PAYLOAD"),
			}
			Expect(proto.Equal(event.Events[0], expectedEvent)).To(BeTrue(), "Expected\n\t%#v\nto proto.Equal\n\t%#v", event.Events[0], expectedEvent)
		})

		It("should respond with replayed chaincode events after specified transaction ID", func() {
			_, afterTransactionID := submitTransaction("event", []byte("WRONG_EVENT_NAME"), []byte("WRONG_EVENT_PAYLOAD"))
			_, nextTransactionID := submitTransaction("event", []byte("CORRECT_EVENT_NAME"), []byte("CORRECT_EVENT_PAYLOAD"))

			statusResult, err := commitStatus(afterTransactionID, signingIdentity.Serialize, signingIdentity.Sign)
			Expect(err).NotTo(HaveOccurred())

			_, err = commitStatus(nextTransactionID, signingIdentity.Serialize, signingIdentity.Sign)
			Expect(err).NotTo(HaveOccurred())

			eventCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()

			startPosition := &orderer.SeekPosition{
				Type: &orderer.SeekPosition_Specified{
					Specified: &orderer.SeekSpecified{
						Number: statusResult.BlockNumber,
					},
				},
			}

			eventsClient, err := chaincodeEvents(eventCtx, startPosition, afterTransactionID, signingIdentity.Serialize, signingIdentity.Sign)
			Expect(err).NotTo(HaveOccurred())

			event, err := eventsClient.Recv()
			Expect(err).NotTo(HaveOccurred())

			Expect(event.Events).To(HaveLen(1), "number of events")
			expectedEvent := &peer.ChaincodeEvent{
				ChaincodeId: "gatewaycc",
				TxId:        nextTransactionID,
				EventName:   "CORRECT_EVENT_NAME",
				Payload:     []byte("CORRECT_EVENT_PAYLOAD"),
			}
			Expect(proto.Equal(event.Events[0], expectedEvent)).To(BeTrue(), "Expected\n\t%#v\nto proto.Equal\n\t%#v", event.Events[0], expectedEvent)
		})

		It("should default to next commit if start position not specified", func() {
			eventCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()

			var startPosition *orderer.SeekPosition

			eventsClient, err := chaincodeEvents(eventCtx, startPosition, "", signingIdentity.Serialize, signingIdentity.Sign)
			Expect(err).NotTo(HaveOccurred())

			_, transactionID := submitTransaction("event", []byte("EVENT_NAME"), []byte("EVENT_PAYLOAD"))

			event, err := eventsClient.Recv()
			Expect(err).NotTo(HaveOccurred())

			Expect(event.Events).To(HaveLen(1), "number of events")
			expectedEvent := &peer.ChaincodeEvent{
				ChaincodeId: "gatewaycc",
				TxId:        transactionID,
				EventName:   "EVENT_NAME",
				Payload:     []byte("EVENT_PAYLOAD"),
			}
			Expect(proto.Equal(event.Events[0], expectedEvent)).To(BeTrue(), "Expected\n\t%#v\nto proto.Equal\n\t%#v", event.Events[0], expectedEvent)
		})

		It("should fail on unauthorized identity", func() {
			badIdentity := network.OrdererUserSigner(network.Orderer("orderer"), "Admin")

			eventCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()

			startPosition := &orderer.SeekPosition{
				Type: &orderer.SeekPosition_NextCommit{
					NextCommit: &orderer.SeekNextCommit{},
				},
			}

			eventsClient, err := chaincodeEvents(eventCtx, startPosition, "", badIdentity.Serialize, signingIdentity.Sign)
			Expect(err).NotTo(HaveOccurred())

			event, err := eventsClient.Recv()
			Expect(err).To(HaveOccurred(), "expected error but got event: %v", event)

			grpcErr, _ := status.FromError(err)
			Expect(grpcErr.Code()).To(Equal(codes.PermissionDenied))
		})

		It("should fail on bad signature", func() {
			badSign := func(digest []byte) ([]byte, error) {
				return signingIdentity.Sign([]byte("WRONG"))
			}

			eventCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()

			startPosition := &orderer.SeekPosition{
				Type: &orderer.SeekPosition_NextCommit{
					NextCommit: &orderer.SeekNextCommit{},
				},
			}

			eventsClient, err := chaincodeEvents(eventCtx, startPosition, "", signingIdentity.Serialize, badSign)
			Expect(err).NotTo(HaveOccurred())

			event, err := eventsClient.Recv()
			Expect(err).To(HaveOccurred(), "expected error but got event: %v", event)

			grpcErr, _ := status.FromError(err)
			Expect(grpcErr.Code()).To(Equal(codes.PermissionDenied))
		})
	})
})
