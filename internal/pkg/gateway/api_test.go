/*
Copyright 2021 IBM All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gateway

import (
	"context"
	"fmt"
	"testing"

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
	"github.com/stretchr/testify/require"
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

//go:generate counterfeiter -o mocks/submitserver.go --fake-name SubmitServer . submitServer
type submitServer interface {
	pb.Gateway_SubmitServer
}

//go:generate counterfeiter -o mocks/orderer.go --fake-name Orderer . orderer
type orderer interface {
	ab.AtomicBroadcast_BroadcastClient
}

type endorsementPlan map[string][]string

type networkMember struct {
	id       string
	endpoint string
	mspid    string
}

type testDef struct {
	name                 string
	plan                 endorsementPlan
	setupDiscovery       func(dm *mocks.Discovery)
	setupRegistry        func(reg *registry)
	processProposalError error
	signedProposal       *peer.SignedProposal
	localResponse        string
	errString            string
}

type contextKey string

func TestGateway(t *testing.T) {
	const testChannel = "test_channel"
	const testChaincode = "test_chaincode"

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
		localEndorser.ProcessProposalReturns(createProposalResponse(t, localResponse), tt.processProposalError)

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

		server := CreateServer(localEndorser, disc, "localhost:7051")

		factory := &endpointFactory{
			t:                t,
			proposalResponse: "mock_response",
		}
		server.registry.endorserFactory = factory.mockEndorserFactory
		server.registry.ordererFactory = factory.mockOrdererFactory

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
				errString:      "failed to unpack channel header: a signed proposal is required",
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
				signedProposal:       validSignedProposal,
				processProposalError: fmt.Errorf("mumbo-jumbo"),
				errString:            "mumbo-jumbo",
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				server, localEndorser, ctx, disc := setup(t, &tt)

				result, err := server.Evaluate(ctx, &pb.ProposedTransaction{Proposal: tt.signedProposal})

				if tt.errString != "" {
					require.ErrorContains(t, err, tt.errString)
					require.Nil(t, result)
					return
				}

				// test the assertions

				require.NoError(t, err, "Failed to evaluate the proposal")
				// assert the result is the payload from the proposal response returned by the local endorser
				require.Equal(t, []byte("mock_response"), result.Value, "Incorrect result")

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
				signedProposal:       validSignedProposal,
				processProposalError: fmt.Errorf("mumbo-jumbo"),
				errString:            "mumbo-jumbo",
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				server, localEndorser, ctx, disc := setup(t, &tt)

				preparedTxn, err := server.Endorse(ctx, &pb.ProposedTransaction{Proposal: tt.signedProposal})

				if tt.errString != "" {
					require.ErrorContains(t, err, tt.errString)
					require.Nil(t, preparedTxn)
					return
				}

				// test the assertions
				require.NoError(t, err, "Failed to evaluate the proposal")
				// assert the preparedTxn is the payload from the proposal response
				require.Equal(t, []byte("mock_response"), preparedTxn.Response.Value, "Incorrect response")

				// check the local endorser (mock) was called with the right parameters
				require.Equal(t, 1, localEndorser.ProcessProposalCallCount())
				ectx, prop, _ := localEndorser.ProcessProposalArgsForCall(0)
				require.Equal(t, tt.signedProposal, prop)
				require.Same(t, ctx, ectx)

				require.Equal(t, testChannel, preparedTxn.ChannelId)
				// check the prepare transaction (Envelope) contains the right number of endorsements
				payload, err := protoutil.UnmarshalPayload(preparedTxn.Envelope.Payload)
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
				setupRegistry: func(reg *registry) {
					reg.ordererFactory = func(address string, tlsRootCerts [][]byte) (ab.AtomicBroadcast_BroadcastClient, error) {
						orderer := &mocks.Orderer{}
						orderer.SendReturns(fmt.Errorf("Orderer says no!"))
						return orderer, nil
					}
				},
				errString: "failed to send envelope to orderer: Orderer says no!",
			},
			{
				name: "receive from orderer fails",
				plan: endorsementPlan{
					"g1": {"localhost:7051"},
				},
				signedProposal: validSignedProposal,
				setupRegistry: func(reg *registry) {
					reg.ordererFactory = func(address string, tlsRootCerts [][]byte) (ab.AtomicBroadcast_BroadcastClient, error) {
						orderer := &mocks.Orderer{}
						orderer.RecvReturns(nil, fmt.Errorf("Orderer not happy!"))
						return orderer, nil
					}
				},
				errString: "Orderer not happy!",
			},
			{
				name: "orderer returns nil",
				plan: endorsementPlan{
					"g1": {"localhost:7051"},
				},
				signedProposal: validSignedProposal,
				setupRegistry: func(reg *registry) {
					reg.ordererFactory = func(address string, tlsRootCerts [][]byte) (ab.AtomicBroadcast_BroadcastClient, error) {
						orderer := &mocks.Orderer{}
						orderer.RecvReturns(nil, nil)
						return orderer, nil
					}
				},
				errString: "received nil response from orderer",
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				server, _, ctx, _ := setup(t, &tt)

				// first call endorse to prepare the tx
				preparedTx, err := server.Endorse(ctx, &pb.ProposedTransaction{Proposal: tt.signedProposal})
				require.NoError(t, err, "Failed to prepare the transaction")

				// sign the envelope
				preparedTx.Envelope.Signature = []byte("mysignature")

				cs := &mocks.SubmitServer{}
				// submit
				err = server.Submit(preparedTx, cs)

				if tt.errString != "" {
					require.ErrorContains(t, err, tt.errString)
					return
				}

				require.NoError(t, err, "Failed to commit the transaction")
				require.Equal(t, 1, cs.SendCallCount())
			})
		}
	})
}

func TestNilArgs(t *testing.T) {
	server := CreateServer(&mocks.EndorserClient{}, &mocks.Discovery{}, "localhost:7051")
	ctx := context.Background()

	_, err := server.Evaluate(ctx, nil)
	require.ErrorContains(t, err, "a proposed transaction is required")

	_, err = server.Endorse(ctx, nil)
	require.ErrorContains(t, err, "a proposed transaction is required")

	err = server.Submit(nil, nil)
	require.ErrorContains(t, err, "a signed prepared transaction is required")

	err = server.Submit(&pb.PreparedTransaction{}, nil)
	require.ErrorContains(t, err, "a submit server is required")
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

type endpointFactory struct {
	t                 *testing.T
	proposalResponse  string
	broadcastResponse string
}

func (ef *endpointFactory) mockEndorserFactory(address string, tlsRootCerts [][]byte) (peer.EndorserClient, error) {
	endorser := &mocks.EndorserClient{}
	endorser.ProcessProposalReturns(createProposalResponse(ef.t, ef.proposalResponse), nil)
	return endorser, nil
}

func (ef *endpointFactory) mockOrdererFactory(address string, tlsRootCerts [][]byte) (ab.AtomicBroadcast_BroadcastClient, error) {
	orderer := &mocks.Orderer{}
	orderer.RecvReturns(&ab.BroadcastResponse{
		Info:   ef.broadcastResponse,
		Status: 200,
	}, nil)
	return orderer, nil
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

func createProposalResponse(t *testing.T, value string) *peer.ProposalResponse {
	response := &peer.Response{
		Status:  200,
		Payload: []byte(value),
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
