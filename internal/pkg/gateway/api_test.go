/*
Copyright 2021 IBM All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gateway

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	cp "github.com/hyperledger/fabric-protos-go/common"
	dp "github.com/hyperledger/fabric-protos-go/discovery"
	pb "github.com/hyperledger/fabric-protos-go/gateway"
	"github.com/hyperledger/fabric-protos-go/gossip"
	"github.com/hyperledger/fabric-protos-go/msp"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/common"
	gdiscovery "github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/internal/pkg/gateway/config"
	"github.com/hyperledger/fabric/internal/pkg/gateway/mocks"
	idmocks "github.com/hyperledger/fabric/internal/pkg/identity/mocks"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// The following private interfaces are here purely to prevent counterfeiter creating an import cycle in the unit test
//go:generate counterfeiter -o mocks/endorserclient.go --fake-name EndorserClient . endorserClient
type endorserClient interface {
	peer.EndorserClient
}

//go:generate counterfeiter -o mocks/discovery.go --fake-name Discovery . discovery
type discovery interface {
	Discovery
}

//go:generate counterfeiter -o mocks/abclient.go --fake-name ABClient . abClient
type abClient interface {
	ab.AtomicBroadcastClient
}

//go:generate counterfeiter -o mocks/abbclient.go --fake-name ABBClient . abbClient
type abbClient interface {
	ab.AtomicBroadcast_BroadcastClient
}

//go:generate counterfeiter -o mocks/commitfinder.go --fake-name CommitFinder . commitFinder
type commitFinder interface {
	CommitFinder
}

type endorsementPlan map[string][]endorserState

type networkMember struct {
	id       string
	endpoint string
	mspid    string
}

type endpointDef struct {
	proposalResponseValue   string
	proposalResponseStatus  int32
	proposalResponseMessage string
	proposalError           error
	ordererResponse         string
	ordererStatus           int32
	ordererBroadcastError   error
	ordererSendError        error
	ordererRecvError        error
}

var defaultEndpointDef = &endpointDef{
	proposalResponseValue:  "mock_response",
	proposalResponseStatus: 200,
	ordererResponse:        "mock_orderer_response",
	ordererStatus:          200,
}

const (
	testChannel        = "test_channel"
	testChaincode      = "test_chaincode"
	endorsementTimeout = -1 * time.Second
)

type testDef struct {
	name               string
	plan               endorsementPlan
	localResponse      string
	errString          string
	errDetails         []*pb.EndpointError
	endpointDefinition *endpointDef
	postSetup          func(def *preparedTest)
	expectedEndorsers  []string
	finderStatus       peer.TxValidationCode
	finderErr          error
	expectedResponse   proto.Message
}

type preparedTest struct {
	server         *Server
	ctx            context.Context
	signedProposal *peer.SignedProposal
	localEndorser  *mocks.EndorserClient
	discovery      *mocks.Discovery
	dialer         *mocks.Dialer
	finder         *mocks.CommitFinder
}

type contextKey string

func TestEvaluate(t *testing.T) {
	tests := []testDef{
		{
			name: "single endorser",
			plan: endorsementPlan{
				"g1": {{endpoint: "localhost:7051"}},
			},
		},
		{
			name:      "no endorsers",
			plan:      endorsementPlan{},
			errString: "no endorsing peers",
		},
		{
			name: "five endorsers, two groups, prefer largest ledger height",
			plan: endorsementPlan{
				"g1": {{endpoint: "peer3:10051", height: 4}, {endpoint: "localhost:7051", height: 4}, {endpoint: "peer4:11051", height: 4}},
				"g2": {{endpoint: "peer1:8051", height: 3}, {endpoint: "peer2:9051", height: 5}},
			},
			expectedEndorsers: []string{"peer2:9051"},
		},
		{
			name: "five endorsers, two groups, prefer host peer",
			plan: endorsementPlan{
				"g1": {{endpoint: "peer3:10051", height: 4}, {endpoint: "localhost:7051", height: 4}, {endpoint: "peer4:11051", height: 4}},
				"g2": {{endpoint: "peer1:8051", height: 3}, {endpoint: "peer2:9051", height: 4}},
			},
			expectedEndorsers: []string{"localhost:7051"},
		},
		{
			name: "discovery fails",
			plan: endorsementPlan{
				"g1": {{endpoint: "localhost:7051"}},
			},
			postSetup: func(def *preparedTest) {
				def.discovery.PeersForEndorsementReturns(nil, fmt.Errorf("mango-tango"))
			},
			errString: "mango-tango",
		},
		{
			name: "process proposal fails",
			plan: endorsementPlan{
				"g1": {{endpoint: "localhost:7051"}},
			},
			endpointDefinition: &endpointDef{
				proposalError: status.Error(codes.Aborted, "mumbo-jumbo"),
			},
			errString: "rpc error: code = Aborted desc = failed to evaluate transaction",
			errDetails: []*pb.EndpointError{{
				Address: "localhost:7051",
				MspId:   "msp1",
				Message: "rpc error: code = Aborted desc = mumbo-jumbo",
			}},
		},
		{
			name: "process proposal chaincode error",
			plan: endorsementPlan{
				"g1": {{endpoint: "peer1:8051"}},
			},
			endpointDefinition: &endpointDef{
				proposalResponseStatus:  400,
				proposalResponseMessage: "Mock chaincode error",
			},
			errString: "rpc error: code = Aborted desc = transaction evaluation error",
			errDetails: []*pb.EndpointError{{
				Address: "peer1:8051",
				MspId:   "msp1",
				Message: "error 400, Mock chaincode error",
			}},
		},
		{
			name: "dialing endorser endpoint fails",
			plan: endorsementPlan{
				"g1": {{endpoint: "peer2:9051"}},
			},
			postSetup: func(def *preparedTest) {
				def.dialer.Calls(func(_ context.Context, target string, _ ...grpc.DialOption) (*grpc.ClientConn, error) {
					if target == "peer2:9051" {
						return nil, fmt.Errorf("endorser not answering")
					}
					return nil, nil
				})
			},
			errString: "failed to create new connection: endorser not answering",
		},
		{
			name: "dialing orderer endpoint fails",
			plan: endorsementPlan{
				"g1": {{endpoint: "peer2:9051"}},
			},
			postSetup: func(def *preparedTest) {
				def.dialer.Calls(func(_ context.Context, target string, _ ...grpc.DialOption) (*grpc.ClientConn, error) {
					if target == "orderer:7050" {
						return nil, fmt.Errorf("orderer not answering")
					}
					return nil, nil
				})
			},
			errString: "failed to create new connection: orderer not answering",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			test := prepareTest(t, &tt)

			response, err := test.server.Evaluate(test.ctx, &pb.EvaluateRequest{ProposedTransaction: test.signedProposal})

			if tt.errString != "" {
				checkError(t, err, tt.errString, tt.errDetails)
				require.Nil(t, response)
				return
			}

			// test the assertions

			require.NoError(t, err)
			// assert the result is the payload from the proposal response returned by the local endorser
			require.Equal(t, []byte("mock_response"), response.Result.Payload, "Incorrect result")

			// check the correct endorsers (mock) were called with the right parameters
			checkEndorsers(t, tt.expectedEndorsers, test)

			// check the discovery service (mock) was invoked as expected
			require.Equal(t, 1, test.discovery.PeersForEndorsementCallCount())
			channel, interest := test.discovery.PeersForEndorsementArgsForCall(0)
			expectedChannel := common.ChannelID(testChannel)
			expectedInterest := &dp.ChaincodeInterest{
				Chaincodes: []*dp.ChaincodeCall{{
					Name: testChaincode,
				}},
			}
			require.Equal(t, expectedChannel, channel)
			require.Equal(t, expectedInterest, interest)

			require.Equal(t, 1, test.discovery.PeersOfChannelCallCount())
			channel = test.discovery.PeersOfChannelArgsForCall(0)
			require.Equal(t, expectedChannel, channel)

			require.Equal(t, 1, test.discovery.IdentityInfoCallCount())
		})
	}
}

func TestEndorse(t *testing.T) {
	tests := []testDef{
		{
			name: "two endorsers",
			plan: endorsementPlan{
				"g1": {{endpoint: "localhost:7051", height: 3}},
				"g2": {{endpoint: "peer1:8051", height: 3}},
			},
		},
		{
			name: "three endorsers, two groups",
			plan: endorsementPlan{
				"g1": {{endpoint: "localhost:7051", height: 4}},
				"g2": {{endpoint: "peer1:8051", height: 4}, {endpoint: "peer2:9051", height: 5}},
			},
			expectedEndorsers: []string{"localhost:7051", "peer2:9051"},
		},
		{
			name: "three endorsers, two groups, prefer host peer",
			plan: endorsementPlan{
				"g1": {{endpoint: "peer3:10051", height: 4}, {endpoint: "localhost:7051", height: 4}, {endpoint: "peer4:11051", height: 4}},
				"g2": {{endpoint: "peer1:8051", height: 4}, {endpoint: "peer2:9051", height: 5}},
			},
			expectedEndorsers: []string{"localhost:7051", "peer2:9051"},
		},
		{
			name:      "no endorsers",
			plan:      endorsementPlan{},
			errString: "failed to assemble transaction: at least one proposal response is required",
		},
		{
			name: "non-matching responses",
			plan: endorsementPlan{
				"g1": {{endpoint: "localhost:7051", height: 4}},
				"g2": {{endpoint: "peer1:8051", height: 5}},
			},
			localResponse: "different_response",
			errString:     "failed to assemble transaction: ProposalResponsePayloads do not match",
		},
		{
			name: "discovery fails",
			plan: endorsementPlan{
				"g1": {{endpoint: "localhost:7051", height: 2}},
			},
			postSetup: func(def *preparedTest) {
				def.discovery.PeersForEndorsementReturns(nil, fmt.Errorf("peach-melba"))
			},
			errString: "peach-melba",
		},
		{
			name: "process proposal fails",
			plan: endorsementPlan{
				"g1": {{endpoint: "localhost:7051", height: 1}},
			},
			endpointDefinition: &endpointDef{
				proposalError: status.Error(codes.Aborted, "wibble"),
			},
			errString: "failed to endorse transaction",
			errDetails: []*pb.EndpointError{{
				Address: "localhost:7051",
				MspId:   "msp1",
				Message: "rpc error: code = Aborted desc = wibble",
			}},
		},
		{
			name: "process proposal chaincode error",
			plan: endorsementPlan{
				"g1": {{endpoint: "peer1:8051", height: 2}},
			},
			endpointDefinition: &endpointDef{
				proposalResponseStatus:  400,
				proposalResponseMessage: "Mock chaincode error",
			},
			errString: "rpc error: code = Aborted desc = failed to endorse transaction",
			errDetails: []*pb.EndpointError{{
				Address: "peer1:8051",
				MspId:   "msp1",
				Message: "error 400, Mock chaincode error",
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			test := prepareTest(t, &tt)

			response, err := test.server.Endorse(test.ctx, &pb.EndorseRequest{ProposedTransaction: test.signedProposal})

			if tt.errString != "" {
				checkError(t, err, tt.errString, tt.errDetails)
				require.Nil(t, response)
				return
			}

			// test the assertions
			require.NoError(t, err)
			// assert the preparedTxn is the payload from the proposal response
			require.Equal(t, []byte("mock_response"), response.Result.Payload, "Incorrect response")

			// check the correct endorsers (mock) were called with the right parameters
			checkEndorsers(t, tt.expectedEndorsers, test)

			// check the prepare transaction (Envelope) contains the right number of endorsements
			payload, err := protoutil.UnmarshalPayload(response.PreparedTransaction.Payload)
			require.NoError(t, err)
			txn, err := protoutil.UnmarshalTransaction(payload.Data)
			require.NoError(t, err)
			cap, err := protoutil.UnmarshalChaincodeActionPayload(txn.Actions[0].Payload)
			require.NoError(t, err)
			endorsements := cap.Action.Endorsements
			require.Len(t, endorsements, len(tt.plan))

			// check the discovery service (mock) was invoked as expected
			require.Equal(t, 1, test.discovery.PeersForEndorsementCallCount())
			channel, interest := test.discovery.PeersForEndorsementArgsForCall(0)
			expectedChannel := common.ChannelID(testChannel)
			expectedInterest := &dp.ChaincodeInterest{
				Chaincodes: []*dp.ChaincodeCall{{
					Name: testChaincode,
				}},
			}
			require.Equal(t, expectedChannel, channel)
			require.Equal(t, expectedInterest, interest)

			require.Equal(t, 1, test.discovery.PeersOfChannelCallCount())
			channel = test.discovery.PeersOfChannelArgsForCall(0)
			require.Equal(t, expectedChannel, channel)

			require.Equal(t, 1, test.discovery.IdentityInfoCallCount())
		})
	}
}

func TestSubmit(t *testing.T) {
	tests := []testDef{
		{
			name: "two endorsers",
			plan: endorsementPlan{
				"g1": {{endpoint: "localhost:7051", height: 3}},
				"g2": {{endpoint: "peer1:8051", height: 3}},
			},
		},
		{
			name: "discovery fails",
			plan: endorsementPlan{
				"g1": {{endpoint: "localhost:7051"}},
			},
			postSetup: func(def *preparedTest) {
				def.discovery.ConfigReturnsOnCall(1, nil, fmt.Errorf("jabberwocky"))
			},
			errString: "jabberwocky",
		},
		{
			name: "no orderers",
			plan: endorsementPlan{
				"g1": {{endpoint: "localhost:7051"}},
			},
			postSetup: func(def *preparedTest) {
				def.discovery.ConfigReturns(&dp.ConfigResult{
					Orderers: map[string]*dp.Endpoints{},
					Msps:     map[string]*msp.FabricMSPConfig{},
				}, nil)
			},
			errString: "no broadcastClients discovered",
		},
		{
			name: "orderer broadcast fails",
			plan: endorsementPlan{
				"g1": {{endpoint: "localhost:7051"}},
			},
			endpointDefinition: &endpointDef{
				proposalResponseStatus: 200,
				ordererBroadcastError:  status.Error(codes.FailedPrecondition, "Orderer not listening!"),
			},
			errString: "rpc error: code = Aborted desc = failed to send transaction to orderer",
			errDetails: []*pb.EndpointError{{
				Address: "orderer:7050",
				MspId:   "msp1",
				Message: "rpc error: code = FailedPrecondition desc = Orderer not listening!",
			}},
		},
		{
			name: "send to orderer fails",
			plan: endorsementPlan{
				"g1": {{endpoint: "localhost:7051"}},
			},
			endpointDefinition: &endpointDef{
				proposalResponseStatus: 200,
				ordererSendError:       status.Error(codes.Internal, "Orderer says no!"),
			},
			errString: "rpc error: code = Aborted desc = failed to send transaction to orderer",
			errDetails: []*pb.EndpointError{{
				Address: "orderer:7050",
				MspId:   "msp1",
				Message: "rpc error: code = Internal desc = Orderer says no!",
			}},
		},
		{
			name: "receive from orderer fails",
			plan: endorsementPlan{
				"g1": {{endpoint: "localhost:7051"}},
			},
			endpointDefinition: &endpointDef{
				proposalResponseStatus: 200,
				ordererRecvError:       status.Error(codes.FailedPrecondition, "Orderer not happy!"),
			},
			errString: "rpc error: code = Aborted desc = failed to receive response from orderer",
			errDetails: []*pb.EndpointError{{
				Address: "orderer:7050",
				MspId:   "msp1",
				Message: "rpc error: code = FailedPrecondition desc = Orderer not happy!",
			}},
		},
		{
			name: "orderer Send() returns nil",
			plan: endorsementPlan{
				"g1": {{endpoint: "localhost:7051"}},
			},
			postSetup: func(def *preparedTest) {
				def.server.registry.endpointFactory.connectOrderer = func(_ *grpc.ClientConn) ab.AtomicBroadcastClient {
					abc := &mocks.ABClient{}
					abbc := &mocks.ABBClient{}
					abbc.RecvReturns(nil, nil)
					abc.BroadcastReturns(abbc, nil)
					return abc
				}
			},
			errString: "received nil response from orderer",
		},
		{
			name: "orderer returns unsuccessful response",
			plan: endorsementPlan{
				"g1": {{endpoint: "localhost:7051"}},
			},
			postSetup: func(def *preparedTest) {
				def.server.registry.endpointFactory.connectOrderer = func(_ *grpc.ClientConn) ab.AtomicBroadcastClient {
					abc := &mocks.ABClient{}
					abbc := &mocks.ABBClient{}
					response := &ab.BroadcastResponse{
						Status: cp.Status_BAD_REQUEST,
					}
					abbc.RecvReturns(response, nil)
					abc.BroadcastReturns(abbc, nil)
					return abc
				}
			},
			errString: cp.Status_name[int32(cp.Status_BAD_REQUEST)],
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			test := prepareTest(t, &tt)

			// first call endorse to prepare the tx
			endorseResponse, err := test.server.Endorse(test.ctx, &pb.EndorseRequest{ProposedTransaction: test.signedProposal})
			require.NoError(t, err)

			preparedTx := endorseResponse.GetPreparedTransaction()

			// sign the envelope
			preparedTx.Signature = []byte("mysignature")

			// submit
			submitResponse, err := test.server.Submit(test.ctx, &pb.SubmitRequest{PreparedTransaction: preparedTx})

			if tt.errString != "" {
				checkError(t, err, tt.errString, tt.errDetails)
				require.Nil(t, submitResponse)
				return
			}

			require.NoError(t, err)
			require.True(t, proto.Equal(&pb.SubmitResponse{}, submitResponse), "Incorrect response")
		})
	}
}

func TestSubmitUnsigned(t *testing.T) {
	server := &Server{}
	req := &pb.SubmitRequest{
		TransactionId:       "transaction-id",
		ChannelId:           "channel-id",
		PreparedTransaction: &cp.Envelope{},
	}
	_, err := server.Submit(context.Background(), req)
	require.Error(t, err)
	require.Equal(t, err, status.Error(codes.InvalidArgument, "prepared transaction must be signed"))
}

func TestCommitStatus(t *testing.T) {
	tests := []testDef{
		{
			name:      "error finding transaction status",
			finderErr: errors.New("FINDER_ERROR"),
			errString: "rpc error: code = FailedPrecondition desc = FINDER_ERROR",
		},
		{
			name:         "returns transaction status",
			finderStatus: peer.TxValidationCode_MVCC_READ_CONFLICT,
			expectedResponse: &pb.CommitStatusResponse{
				Result: peer.TxValidationCode_MVCC_READ_CONFLICT,
			},
		},
		{
			name: "passes channel name to finder",
			postSetup: func(test *preparedTest) {
				test.finder.TransactionStatusCalls(func(ctx context.Context, channelName string, transactionID string) (peer.TxValidationCode, error) {
					if channelName != testChannel {
						return 0, errors.Errorf("channel name: %s", channelName)
					}
					return peer.TxValidationCode_MVCC_READ_CONFLICT, nil
				})
			},
			expectedResponse: &pb.CommitStatusResponse{
				Result: peer.TxValidationCode_MVCC_READ_CONFLICT,
			},
		},
		{
			name: "passes transaction ID to finder",
			postSetup: func(test *preparedTest) {
				test.finder.TransactionStatusCalls(func(ctx context.Context, channelName string, transactionID string) (peer.TxValidationCode, error) {
					if transactionID != "TX_ID" {
						return 0, errors.Errorf("transaction ID: %s", transactionID)
					}
					return peer.TxValidationCode_MVCC_READ_CONFLICT, nil
				})
			},
			expectedResponse: &pb.CommitStatusResponse{
				Result: peer.TxValidationCode_MVCC_READ_CONFLICT,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			test := prepareTest(t, &tt)

			// skeleton test code - to be completed when CommitStatus is implemented
			response, err := test.server.CommitStatus(test.ctx, &pb.CommitStatusRequest{ChannelId: testChannel, TransactionId: "TX_ID"})

			if tt.errString != "" {
				checkError(t, err, tt.errString, tt.errDetails)
				require.Nil(t, response)
				return
			}

			require.NoError(t, err)
			if tt.expectedResponse != nil {
				require.True(t, proto.Equal(tt.expectedResponse, response), "incorrect response", response)
			}
		})
	}
}

func TestNilArgs(t *testing.T) {
	server := CreateServer(&mocks.EndorserClient{}, &mocks.Discovery{}, &mocks.CommitFinder{}, "localhost:7051", "msp1", config.GetOptions(viper.New()))
	ctx := context.Background()

	_, err := server.Evaluate(ctx, nil)
	require.ErrorIs(t, err, status.Error(codes.InvalidArgument, "an evaluate request is required"))

	_, err = server.Evaluate(ctx, &pb.EvaluateRequest{ProposedTransaction: nil})
	require.ErrorIs(t, err, status.Error(codes.InvalidArgument, "failed to unpack transaction proposal: a signed proposal is required"))

	_, err = server.Endorse(ctx, nil)
	require.ErrorIs(t, err, status.Error(codes.InvalidArgument, "an endorse request is required"))

	_, err = server.Endorse(ctx, &pb.EndorseRequest{ProposedTransaction: nil})
	require.ErrorIs(t, err, status.Error(codes.InvalidArgument, "the proposed transaction must contain a signed proposal"))

	_, err = server.Endorse(ctx, &pb.EndorseRequest{ProposedTransaction: &peer.SignedProposal{ProposalBytes: []byte("jibberish")}})
	require.ErrorIs(t, err, status.Error(codes.InvalidArgument, "failed to unpack transaction proposal: error unmarshaling Proposal: unexpected EOF"))

	_, err = server.Submit(ctx, nil)
	require.ErrorIs(t, err, status.Error(codes.InvalidArgument, "a submit request is required"))

	_, err = server.CommitStatus(ctx, nil)
	require.ErrorIs(t, err, status.Error(codes.InvalidArgument, "a commit status request is required"))
}

func TestRpcErrorWithBadDetails(t *testing.T) {
	err := rpcError(codes.InvalidArgument, "terrible error", nil)
	require.ErrorIs(t, err, status.Error(codes.InvalidArgument, "terrible error"))
}

func prepareTest(t *testing.T, tt *testDef) *preparedTest {
	localEndorser := &mocks.EndorserClient{}
	localResponse := tt.localResponse
	if localResponse == "" {
		localResponse = "mock_response"
	}
	epDef := tt.endpointDefinition
	if epDef == nil {
		epDef = defaultEndpointDef
	}
	if epDef.proposalError != nil {
		localEndorser.ProcessProposalReturns(nil, epDef.proposalError)
	} else {
		localEndorser.ProcessProposalReturns(createProposalResponse(t, localResponse, 200, ""), nil)
	}

	mockSigner := &idmocks.SignerSerializer{}
	mockSigner.SignReturns([]byte("my_signature"), nil)

	mockFinder := &mocks.CommitFinder{}
	mockFinder.TransactionStatusReturns(tt.finderStatus, tt.finderErr)

	validProposal := createProposal(t, testChannel, testChaincode)
	validSignedProposal, err := protoutil.GetSignedProposal(validProposal, mockSigner)
	require.NoError(t, err)

	ca, err := tlsgen.NewCA()
	require.NoError(t, err)
	configResult := &dp.ConfigResult{
		Orderers: map[string]*dp.Endpoints{
			"msp1": {
				Endpoint: []*dp.Endpoint{
					{Host: "orderer", Port: 7050},
				},
			},
		},
		Msps: map[string]*msp.FabricMSPConfig{
			"msp1": {
				TlsRootCerts: [][]byte{ca.CertBytes()},
			},
		},
	}

	members := []networkMember{
		{"id1", "localhost:7051", "msp1"},
		{"id2", "peer1:8051", "msp1"},
		{"id3", "peer2:9051", "msp1"},
	}

	disc := mockDiscovery(t, tt.plan, members, configResult)

	options := config.Options{
		Enabled:            true,
		EndorsementTimeout: endorsementTimeout,
	}

	server := CreateServer(localEndorser, disc, mockFinder, "localhost:7051", "msp1", options)

	dialer := &mocks.Dialer{}
	dialer.Returns(nil, nil)
	server.registry.endpointFactory = createEndpointFactory(t, epDef, dialer.Spy)

	require.NoError(t, err, "Failed to sign the proposal")
	ctx := context.WithValue(context.Background(), contextKey("orange"), "apples")

	pt := &preparedTest{
		server:         server,
		ctx:            ctx,
		signedProposal: validSignedProposal,
		localEndorser:  localEndorser,
		discovery:      disc,
		dialer:         dialer,
		finder:         mockFinder,
	}
	if tt.postSetup != nil {
		tt.postSetup(pt)
	}
	return pt
}

func checkError(t *testing.T, err error, errString string, details []*pb.EndpointError) {
	require.ErrorContains(t, err, errString)
	s, ok := status.FromError(err)
	require.True(t, ok, "Expected a gRPC status error")
	require.Len(t, s.Details(), len(details))
	for i, detail := range details {
		require.Equal(t, detail.Message, s.Details()[i].(*pb.EndpointError).Message)
		require.Equal(t, detail.MspId, s.Details()[i].(*pb.EndpointError).MspId)
		require.Equal(t, detail.Address, s.Details()[i].(*pb.EndpointError).Address)
	}
}

func checkEndorsers(t *testing.T, endorsers []string, test *preparedTest) {
	// check the correct endorsers (mock) were called with the right parameters
	if endorsers == nil {
		endorsers = []string{"localhost:7051"}
	}
	for _, e := range endorsers {
		var ec *mocks.EndorserClient
		if e == test.server.registry.localEndorser.address {
			ec = test.localEndorser
		} else {
			ec = test.server.registry.remoteEndorsers[e].client.(*mocks.EndorserClient)
		}
		require.Equal(t, 1, ec.ProcessProposalCallCount())
		ectx, prop, _ := ec.ProcessProposalArgsForCall(0)
		require.Equal(t, test.signedProposal, prop)
		require.Equal(t, "apples", ectx.Value(contextKey("orange")))
		// context timeout was set to -1s, so deadline should be in the past
		deadline, ok := ectx.Deadline()
		require.True(t, ok)
		require.Negative(t, time.Until(deadline))
	}
}

func mockDiscovery(t *testing.T, plan endorsementPlan, members []networkMember, config *dp.ConfigResult) *mocks.Discovery {
	discovery := &mocks.Discovery{}

	var peers []gdiscovery.NetworkMember
	var infoset []api.PeerIdentityInfo
	for _, member := range members {
		peers = append(peers, gdiscovery.NetworkMember{Endpoint: member.endpoint, PKIid: []byte(member.id)})
		infoset = append(infoset, api.PeerIdentityInfo{Organization: []byte(member.mspid), PKIId: []byte(member.id)})
	}
	ed := createMockEndorsementDescriptor(t, plan)
	discovery.PeersForEndorsementReturns(ed, nil)
	discovery.PeersOfChannelReturns(peers)
	discovery.IdentityInfoReturns(infoset)
	discovery.ConfigReturns(config, nil)
	return discovery
}

func createMockEndorsementDescriptor(t *testing.T, plan map[string][]endorserState) *dp.EndorsementDescriptor {
	quantitiesByGroup := map[string]uint32{}
	endorsersByGroups := map[string]*dp.Peers{}
	for group, endorsers := range plan {
		quantitiesByGroup[group] = 1 // for now
		var peers []*dp.Peer
		for _, endorser := range endorsers {
			peers = append(peers, createMockPeer(t, endorser.endpoint, endorser.height))
		}
		endorsersByGroups[group] = &dp.Peers{Peers: peers}
	}
	descriptor := &dp.EndorsementDescriptor{
		Chaincode: "my_channel",
		Layouts: []*dp.Layout{
			{
				QuantitiesByGroup: quantitiesByGroup,
			},
		},
		EndorsersByGroups: endorsersByGroups,
	}
	return descriptor
}

func createMockPeer(t *testing.T, name string, ledgerHeight uint64) *dp.Peer {
	aliveMsgBytes, err := proto.Marshal(
		&gossip.GossipMessage{
			Content: &gossip.GossipMessage_AliveMsg{
				AliveMsg: &gossip.AliveMessage{
					Membership: &gossip.Member{Endpoint: name},
				},
			},
		})

	require.NoError(t, err)

	stateInfoBytes, err := proto.Marshal(
		&gossip.GossipMessage{
			Content: &gossip.GossipMessage_StateInfo{
				StateInfo: &gossip.StateInfo{
					Properties: &gossip.Properties{
						LedgerHeight: ledgerHeight,
					},
				},
			},
		})

	require.NoError(t, err)

	return &dp.Peer{
		StateInfo: &gossip.Envelope{
			Payload: stateInfoBytes,
		},
		MembershipInfo: &gossip.Envelope{
			Payload: aliveMsgBytes,
		},
		Identity: []byte(name),
	}
}

func createEndpointFactory(t *testing.T, definition *endpointDef, dialer dialer) *endpointFactory {
	return &endpointFactory{
		timeout: 5 * time.Second,
		connectEndorser: func(_ *grpc.ClientConn) peer.EndorserClient {
			e := &mocks.EndorserClient{}
			if definition.proposalError != nil {
				e.ProcessProposalReturns(nil, definition.proposalError)
			} else {
				e.ProcessProposalReturns(createProposalResponse(t, definition.proposalResponseValue, definition.proposalResponseStatus, definition.proposalResponseMessage), nil)
			}
			return e
		},
		connectOrderer: func(_ *grpc.ClientConn) ab.AtomicBroadcastClient {
			abc := &mocks.ABClient{}
			if definition.ordererBroadcastError != nil {
				abc.BroadcastReturns(nil, definition.ordererBroadcastError)
				return abc
			}
			abbc := &mocks.ABBClient{}
			abbc.SendReturns(definition.ordererSendError)
			abbc.RecvReturns(&ab.BroadcastResponse{
				Info:   definition.ordererResponse,
				Status: cp.Status(definition.ordererStatus),
			}, definition.ordererRecvError)
			abc.BroadcastReturns(abbc, nil)
			return abc
		},
		dialer: dialer,
	}
}

func createProposal(t *testing.T, channel string, chaincode string, args ...[]byte) *peer.Proposal {
	invocationSpec := &peer.ChaincodeInvocationSpec{
		ChaincodeSpec: &peer.ChaincodeSpec{
			Type:        peer.ChaincodeSpec_NODE,
			ChaincodeId: &peer.ChaincodeID{Name: chaincode},
			Input:       &peer.ChaincodeInput{Args: args},
		},
	}

	proposal, _, err := protoutil.CreateChaincodeProposal(
		cp.HeaderType_ENDORSER_TRANSACTION,
		channel,
		invocationSpec,
		[]byte{},
	)

	require.NoError(t, err, "Failed to create the proposal")

	return proposal
}

func createProposalResponse(t *testing.T, value string, status int32, errMessage string) *peer.ProposalResponse {
	response := &peer.Response{
		Status:  status,
		Payload: []byte(value),
		Message: errMessage,
	}
	action := &peer.ChaincodeAction{
		Response: response,
	}
	payload := &peer.ProposalResponsePayload{
		ProposalHash: []byte{},
		Extension:    marshal(action, t),
	}
	endorsement := &peer.Endorsement{}

	return &peer.ProposalResponse{
		Payload:     marshal(payload, t),
		Response:    response,
		Endorsement: endorsement,
	}
}

func marshal(msg proto.Message, t *testing.T) []byte {
	buf, err := proto.Marshal(msg)
	require.NoError(t, err, "Failed to marshal message")
	return buf
}
