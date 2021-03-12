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
	"github.com/hyperledger/fabric/internal/pkg/gateway/mocks"
	idmocks "github.com/hyperledger/fabric/internal/pkg/identity/mocks"
	"github.com/hyperledger/fabric/protoutil"
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
	ab.AtomicBroadcast_BroadcastClient
}

type endorsementPlan map[string][]string

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
	ordererSendError        error
	ordererRecvError        error
}

var defaultEndpointDef = &endpointDef{
	proposalResponseValue:  "mock_response",
	proposalResponseStatus: 200,
	ordererResponse:        "mock_orderer_response",
	ordererStatus:          200,
}

type testDef struct {
	name               string
	plan               endorsementPlan
	setupDiscovery     func(dm *mocks.Discovery)
	setupRegistry      func(reg *registry)
	signedProposal     *peer.SignedProposal
	localResponse      string
	errString          string
	errDetails         []*pb.EndpointError
	endpointDefinition *endpointDef
}

type contextKey string

func TestGateway(t *testing.T) {
	const testChannel = "test_channel"
	const testChaincode = "test_chaincode"
	const endorsementTimeout = -1 * time.Second

	mockSigner := &idmocks.SignerSerializer{}
	mockSigner.SignReturns([]byte("my_signature"), nil)

	validProposal := createProposal(t, testChannel, testChaincode)
	validSignedProposal, err := protoutil.GetSignedProposal(validProposal, mockSigner)
	require.NoError(t, err)

	setup := func(t *testing.T, tt *testDef) (*Server, *mocks.EndorserClient, context.Context, *mocks.Discovery) {
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

		ca, err := tlsgen.NewCA()
		require.NoError(t, err)
		config := &dp.ConfigResult{
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

		disc := mockDiscovery(t, tt.plan, members, config)
		if tt.setupDiscovery != nil {
			tt.setupDiscovery(disc)
		}

		options := Options{
			Enabled:            true,
			EndorsementTimeout: endorsementTimeout,
		}

		server := CreateServer(localEndorser, disc, "localhost:7051", "msp1", options)

		server.registry.endpointFactory = createEndpointFactory(t, epDef)

		require.NoError(t, err, "Failed to sign the proposal")
		ctx := context.WithValue(context.Background(), contextKey("orange"), "apples")

		if tt.setupRegistry != nil {
			tt.setupRegistry(server.registry)
		}

		return server, localEndorser, ctx, disc
	}

	t.Run("TestEvaluate", func(t *testing.T) {
		tests := []testDef{
			{
				name: "single endorser",
				plan: endorsementPlan{
					"g1": {"localhost:7051"},
				},
				signedProposal: validSignedProposal,
			},
			{
				name:           "no endorsers",
				plan:           endorsementPlan{},
				errString:      "no endorsing peers",
				signedProposal: validSignedProposal,
			},
			{
				name:           "missing signed proposal",
				plan:           endorsementPlan{},
				errString:      "failed to unpack transaction proposal: a signed proposal is required",
				signedProposal: nil,
			},
			{
				name: "discovery fails",
				plan: endorsementPlan{
					"g1": {"localhost:7051"},
				},
				signedProposal: validSignedProposal,
				setupDiscovery: func(d *mocks.Discovery) {
					d.PeersForEndorsementReturns(nil, fmt.Errorf("mango-tango"))
				},
				errString: "mango-tango",
			},
			{
				name: "process proposal fails",
				plan: endorsementPlan{
					"g1": {"localhost:7051"},
				},
				signedProposal: validSignedProposal,
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
					"g1": {"peer1:8051"},
				},
				endpointDefinition: &endpointDef{
					proposalResponseStatus:  400,
					proposalResponseMessage: "Mock chaincode error",
				},
				signedProposal: validSignedProposal,
				errString:      "rpc error: code = Aborted desc = transaction evaluation error",
				errDetails: []*pb.EndpointError{{
					Address: "peer1:8051",
					MspId:   "msp1",
					Message: "error 400, Mock chaincode error",
				}},
			},
			{
				name: "dialing endorser endpoint fails",
				plan: endorsementPlan{
					"g1": {"peer2:9051"},
				},
				signedProposal: validSignedProposal,
				setupRegistry: func(reg *registry) {
					reg.endpointFactory.dialer = func(_ context.Context, target string, _ ...grpc.DialOption) (*grpc.ClientConn, error) {
						if target == "peer2:9051" {
							return nil, fmt.Errorf("endorser not answering")
						}
						return nil, nil
					}
				},
				errString: "failed to create new connection: endorser not answering",
			},
			{
				name: "dialing orderer endpoint fails",
				plan: endorsementPlan{
					"g1": {"peer2:9051"},
				},
				signedProposal: validSignedProposal,
				setupRegistry: func(reg *registry) {
					reg.endpointFactory.dialer = func(_ context.Context, target string, _ ...grpc.DialOption) (*grpc.ClientConn, error) {
						if target == "orderer:7050" {
							return nil, fmt.Errorf("orderer not answering")
						}
						return nil, nil
					}
				},
				errString: "failed to create new connection: orderer not answering",
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				server, localEndorser, ctx, disc := setup(t, &tt)

				response, err := server.Evaluate(ctx, &pb.EvaluateRequest{ProposedTransaction: tt.signedProposal})

				if tt.errString != "" {
					require.ErrorContains(t, err, tt.errString)
					s, ok := status.FromError(err)
					require.True(t, ok, "Expected a gRPC status error")
					require.Len(t, s.Details(), len(tt.errDetails))
					for i, detail := range tt.errDetails {
						require.Equal(t, detail.Message, s.Details()[i].(*pb.EndpointError).Message)
						require.Equal(t, detail.MspId, s.Details()[i].(*pb.EndpointError).MspId)
						require.Equal(t, detail.Address, s.Details()[i].(*pb.EndpointError).Address)
					}
					require.Nil(t, response)
					return
				}

				// test the assertions

				require.NoError(t, err)
				// assert the result is the payload from the proposal response returned by the local endorser
				require.Equal(t, []byte("mock_response"), response.Result.Payload, "Incorrect result")

				// check the local endorser (mock) was called with the right parameters
				require.Equal(t, 1, localEndorser.ProcessProposalCallCount())
				ectx, prop, _ := localEndorser.ProcessProposalArgsForCall(0)
				require.Equal(t, tt.signedProposal, prop)
				require.Same(t, ctx, ectx)

				// check the discovery service (mock) was invoked as expected
				require.Equal(t, 1, disc.PeersForEndorsementCallCount())
				channel, interest := disc.PeersForEndorsementArgsForCall(0)
				expectedChannel := common.ChannelID(testChannel)
				expectedInterest := &dp.ChaincodeInterest{
					Chaincodes: []*dp.ChaincodeCall{{
						Name: testChaincode,
					}},
				}
				require.Equal(t, expectedChannel, channel)
				require.Equal(t, expectedInterest, interest)

				require.Equal(t, 1, disc.PeersOfChannelCallCount())
				channel = disc.PeersOfChannelArgsForCall(0)
				require.Equal(t, expectedChannel, channel)

				require.Equal(t, 1, disc.IdentityInfoCallCount())
			})
		}
	})

	t.Run("TestEndorse", func(t *testing.T) {
		tests := []testDef{
			{
				name: "two endorsers",
				plan: endorsementPlan{
					"g1": {"localhost:7051"},
					"g2": {"peer1:8051"},
				},
				signedProposal: validSignedProposal,
			},
			{
				name: "three endorsers, two groups",
				plan: endorsementPlan{
					"g1": {"localhost:7051"},
					"g2": {"peer1:8051", "peer2:9051"},
				},
				signedProposal: validSignedProposal,
			},
			{
				name:           "no endorsers",
				plan:           endorsementPlan{},
				errString:      "failed to assemble transaction: at least one proposal response is required",
				signedProposal: validSignedProposal,
			},
			{
				name: "non-matching responses",
				plan: endorsementPlan{
					"g1": {"localhost:7051"},
					"g2": {"peer1:8051"},
				},
				signedProposal: validSignedProposal,
				localResponse:  "different_response",
				errString:      "failed to assemble transaction: ProposalResponsePayloads do not match",
			},
			{
				name:           "missing signed proposal",
				plan:           endorsementPlan{},
				errString:      "the proposed transaction must contain a signed proposal",
				signedProposal: nil,
			},
			{
				name: "discovery fails",
				plan: endorsementPlan{
					"g1": {"localhost:7051"},
				},
				signedProposal: validSignedProposal,
				setupDiscovery: func(d *mocks.Discovery) {
					d.PeersForEndorsementReturns(nil, fmt.Errorf("mango-tango"))
				},
				errString: "mango-tango",
			},
			{
				name: "process proposal fails",
				plan: endorsementPlan{
					"g1": {"localhost:7051"},
				},
				signedProposal: validSignedProposal,
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
					"g1": {"peer1:8051"},
				},
				endpointDefinition: &endpointDef{
					proposalResponseStatus:  400,
					proposalResponseMessage: "Mock chaincode error",
				},
				signedProposal: validSignedProposal,
				errString:      "rpc error: code = Aborted desc = failed to endorse transaction",
				errDetails: []*pb.EndpointError{{
					Address: "peer1:8051",
					MspId:   "msp1",
					Message: "error 400, Mock chaincode error",
				}},
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				server, localEndorser, ctx, disc := setup(t, &tt)

				response, err := server.Endorse(ctx, &pb.EndorseRequest{ProposedTransaction: tt.signedProposal})

				if tt.errString != "" {
					require.ErrorContains(t, err, tt.errString)
					s, ok := status.FromError(err)
					require.True(t, ok, "Expected a gRPC status error")
					require.Len(t, s.Details(), len(tt.errDetails))
					for i, detail := range tt.errDetails {
						require.Equal(t, detail.Message, s.Details()[i].(*pb.EndpointError).Message)
						require.Equal(t, detail.MspId, s.Details()[i].(*pb.EndpointError).MspId)
						require.Equal(t, detail.Address, s.Details()[i].(*pb.EndpointError).Address)
					}
					require.Nil(t, response)
					return
				}

				// test the assertions
				require.NoError(t, err)
				// assert the preparedTxn is the payload from the proposal response
				require.Equal(t, []byte("mock_response"), response.Result.Payload, "Incorrect response")

				// check the local endorser (mock) was called with the right parameters
				require.Equal(t, 1, localEndorser.ProcessProposalCallCount())
				ectx, prop, _ := localEndorser.ProcessProposalArgsForCall(0)
				require.Equal(t, tt.signedProposal, prop)
				require.Equal(t, "apples", ectx.Value(contextKey("orange")))
				// context timeout was set to -1s, so deadline should be in the past
				deadline, ok := ectx.Deadline()
				require.True(t, ok)
				require.Negative(t, time.Until(deadline))

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
				require.Equal(t, 1, disc.PeersForEndorsementCallCount())
				channel, interest := disc.PeersForEndorsementArgsForCall(0)
				expectedChannel := common.ChannelID(testChannel)
				expectedInterest := &dp.ChaincodeInterest{
					Chaincodes: []*dp.ChaincodeCall{{
						Name: testChaincode,
					}},
				}
				require.Equal(t, expectedChannel, channel)
				require.Equal(t, expectedInterest, interest)

				require.Equal(t, 1, disc.PeersOfChannelCallCount())
				channel = disc.PeersOfChannelArgsForCall(0)
				require.Equal(t, expectedChannel, channel)

				require.Equal(t, 1, disc.IdentityInfoCallCount())
			})
		}
	})

	t.Run("TestSubmit", func(t *testing.T) {
		tests := []testDef{
			{
				name: "two endorsers",
				plan: endorsementPlan{
					"g1": {"localhost:7051"},
					"g2": {"peer1:8051"},
				},
				signedProposal: validSignedProposal,
			},
			{
				name: "discovery fails",
				plan: endorsementPlan{
					"g1": {"localhost:7051"},
				},
				signedProposal: validSignedProposal,
				setupDiscovery: func(d *mocks.Discovery) {
					d.ConfigReturnsOnCall(1, nil, fmt.Errorf("mango-tango"))
				},
				errString: "mango-tango",
			},
			{
				name: "no orderers",
				plan: endorsementPlan{
					"g1": {"localhost:7051"},
				},
				signedProposal: validSignedProposal,
				setupDiscovery: func(d *mocks.Discovery) {
					d.ConfigReturns(&dp.ConfigResult{
						Orderers: map[string]*dp.Endpoints{},
						Msps:     map[string]*msp.FabricMSPConfig{},
					}, nil)
				},
				errString: "no broadcastClients discovered",
			},
			{
				name: "send to orderer fails",
				plan: endorsementPlan{
					"g1": {"localhost:7051"},
				},
				signedProposal: validSignedProposal,
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
					"g1": {"localhost:7051"},
				},
				signedProposal: validSignedProposal,
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
				name: "orderer returns nil",
				plan: endorsementPlan{
					"g1": {"localhost:7051"},
				},
				signedProposal: validSignedProposal,
				setupRegistry: func(reg *registry) {
					reg.endpointFactory.connectOrderer = func(_ *grpc.ClientConn) (ab.AtomicBroadcast_BroadcastClient, error) {
						abc := &mocks.ABClient{}
						abc.RecvReturns(nil, nil)
						return abc, nil
					}
				},
				errString: "received nil response from orderer",
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				server, _, ctx, _ := setup(t, &tt)

				// first call endorse to prepare the tx
				endorseResponse, err := server.Endorse(ctx, &pb.EndorseRequest{ProposedTransaction: tt.signedProposal})
				require.NoError(t, err)

				preparedTx := endorseResponse.GetPreparedTransaction()

				// sign the envelope
				preparedTx.Signature = []byte("mysignature")

				// submit
				submitResponse, err := server.Submit(ctx, &pb.SubmitRequest{PreparedTransaction: preparedTx})

				if tt.errString != "" {
					require.ErrorContains(t, err, tt.errString)
					s, ok := status.FromError(err)
					require.True(t, ok, "Expected a gRPC status error")
					require.Len(t, s.Details(), len(tt.errDetails))
					for i, detail := range tt.errDetails {
						require.Equal(t, detail.Message, s.Details()[i].(*pb.EndpointError).Message)
						require.Equal(t, detail.MspId, s.Details()[i].(*pb.EndpointError).MspId)
						require.Equal(t, detail.Address, s.Details()[i].(*pb.EndpointError).Address)
					}
					return
				}

				require.NoError(t, err)
				require.True(t, proto.Equal(&pb.SubmitResponse{}, submitResponse), "Incorrect response")
			})
		}
	})
}

func TestNilArgs(t *testing.T) {
	server := CreateServer(&mocks.EndorserClient{}, &mocks.Discovery{}, "localhost:7051", "msp1", GetOptions(viper.New()))
	ctx := context.Background()

	_, err := server.Evaluate(ctx, nil)
	require.ErrorIs(t, err, status.Error(codes.InvalidArgument, "an evaluate request is required"))

	_, err = server.Endorse(ctx, nil)
	require.ErrorIs(t, err, status.Error(codes.InvalidArgument, "an endorse request is required"))

	_, err = server.Submit(ctx, nil)
	require.ErrorIs(t, err, status.Error(codes.InvalidArgument, "a submit request is required"))

	_, err = server.CommitStatus(ctx, nil)
	require.ErrorIs(t, err, status.Error(codes.InvalidArgument, "a commit status request is required"))
}

func TestRpcErrorWithBadDetails(t *testing.T) {
	err := rpcError(codes.InvalidArgument, "terrible error", nil)
	require.ErrorIs(t, err, status.Error(codes.InvalidArgument, "terrible error"))
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

func createMockEndorsementDescriptor(t *testing.T, plan map[string][]string) *dp.EndorsementDescriptor {
	quantitiesByGroup := map[string]uint32{}
	endorsersByGroups := map[string]*dp.Peers{}
	for group, names := range plan {
		quantitiesByGroup[group] = 1 // for now
		var peers []*dp.Peer
		for _, name := range names {
			peers = append(peers, createMockPeer(t, name))
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

func createMockPeer(t *testing.T, name string) *dp.Peer {
	msg := &gossip.GossipMessage{
		Content: &gossip.GossipMessage_AliveMsg{
			AliveMsg: &gossip.AliveMessage{
				Membership: &gossip.Member{Endpoint: name},
			},
		},
	}

	msgBytes, err := proto.Marshal(msg)
	require.NoError(t, err, "Failed to create mock peer")

	return &dp.Peer{
		StateInfo: nil,
		MembershipInfo: &gossip.Envelope{
			Payload: msgBytes,
		},
		Identity: []byte(name),
	}
}

func createEndpointFactory(t *testing.T, definition *endpointDef) *endpointFactory {
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
		connectOrderer: func(_ *grpc.ClientConn) (ab.AtomicBroadcast_BroadcastClient, error) {
			abc := &mocks.ABClient{}
			abc.SendReturns(definition.ordererSendError)
			abc.RecvReturns(&ab.BroadcastResponse{
				Info:   definition.ordererResponse,
				Status: cp.Status(definition.ordererStatus),
			}, definition.ordererRecvError)
			return abc, nil
		},
		dialer: func(_ context.Context, _ string, _ ...grpc.DialOption) (*grpc.ClientConn, error) {
			return nil, nil
		},
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
