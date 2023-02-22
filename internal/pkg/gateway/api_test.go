/*
Copyright 2021 IBM All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gateway

import (
	"context"
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
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/common"
	gdiscovery "github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/hyperledger/fabric/internal/pkg/gateway/commit"
	"github.com/hyperledger/fabric/internal/pkg/gateway/config"
	ledgermocks "github.com/hyperledger/fabric/internal/pkg/gateway/ledger/mocks"
	"github.com/hyperledger/fabric/internal/pkg/gateway/mocks"
	idmocks "github.com/hyperledger/fabric/internal/pkg/identity/mocks"
	"github.com/hyperledger/fabric/internal/pkg/peer/orderers"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// The following private interfaces are here purely to prevent counterfeiter creating an import cycle in the unit test
//
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

//go:generate counterfeiter -o mocks/chaincodeeventsserver.go --fake-name ChaincodeEventsServer github.com/hyperledger/fabric-protos-go/gateway.Gateway_ChaincodeEventsServer

//go:generate counterfeiter -o mocks/aclchecker.go --fake-name ACLChecker . aclChecker
type aclChecker interface {
	ACLChecker
}

//go:generate counterfeiter -o mocks/resultsiterator.go --fake-name ResultsIterator . mockResultsIterator
type mockResultsIterator interface {
	ledger.ResultsIterator
}

//go:generate counterfeiter -o mocks/resources.go --fake-name Resources . ccResources

type ccResources interface {
	channelconfig.Resources
}

//go:generate counterfeiter -o mocks/orderer.go --fake-name Orderer . ccOrderer

type ccOrderer interface {
	channelconfig.Orderer
}

//go:generate counterfeiter -o mocks/channel.go --fake-name Channel . ccChannel

type ccChannel interface {
	channelconfig.Channel
}

//go:generate counterfeiter -o mocks/channel_capabilities.go --fake-name ChannelCapabilities . ccChannelCapabilities

type ccChannelCapabilities interface {
	channelconfig.ChannelCapabilities
}

type (
	endorsementPlan   map[string][]endorserState
	endorsementLayout map[string]uint32
)

type networkMember struct {
	id       string
	endpoint string
	mspid    string
	height   uint64
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
	broadcastTimeout   = 100 * time.Millisecond
)

type testDef struct {
	name                     string
	plan                     endorsementPlan
	layouts                  []endorsementLayout
	members                  []networkMember
	config                   *dp.ConfigResult
	identity                 []byte
	localResponse            string
	errString                string
	errCode                  codes.Code
	errDetails               []*pb.ErrorDetail
	endpointDefinition       *endpointDef
	endorsingOrgs            []string
	postSetup                func(t *testing.T, def *preparedTest)
	postTest                 func(t *testing.T, def *preparedTest)
	expectedEndorsers        []string
	finderStatus             *commit.Status
	finderErr                error
	eventErr                 error
	policyErr                error
	expectedResponse         proto.Message
	expectedResponses        []proto.Message
	transientData            map[string][]byte
	interest                 *peer.ChaincodeInterest
	blocks                   []*cp.Block
	startPosition            *ab.SeekPosition
	afterTxID                string
	ordererEndpointOverrides map[string]*orderers.Endpoint
	isBFT                    bool
	localLedgerHeight        uint64
}

type preparedTest struct {
	server         *Server
	ctx            context.Context
	cancel         context.CancelFunc
	signedProposal *peer.SignedProposal
	localEndorser  *mocks.EndorserClient
	discovery      *mocks.Discovery
	dialer         *mocks.Dialer
	finder         *mocks.CommitFinder
	eventsServer   *mocks.ChaincodeEventsServer
	policy         *mocks.ACLChecker
	ledgerProvider *ledgermocks.Provider
	ledger         *ledgermocks.Ledger
	blockIterator  *mocks.ResultsIterator
	logLevel       string
	logFields      []string
}

type contextKey string

var (
	localhostMock    = &endorser{endpointConfig: &endpointConfig{pkiid: []byte("id1"), address: "localhost:7051", mspid: "msp1"}} // Must have PKI ID of "id1" to match hard-coded local endorser value in tests
	peer1Mock        = &endorser{endpointConfig: &endpointConfig{pkiid: []byte("1"), address: "peer1:8051", mspid: "msp1"}}
	peer2Mock        = &endorser{endpointConfig: &endpointConfig{pkiid: []byte("2"), address: "peer2:9051", mspid: "msp2"}}
	peer3Mock        = &endorser{endpointConfig: &endpointConfig{pkiid: []byte("3"), address: "peer3:10051", mspid: "msp2"}}
	peer4Mock        = &endorser{endpointConfig: &endpointConfig{pkiid: []byte("4"), address: "peer4:11051", mspid: "msp3"}}
	unavailable1Mock = &endorser{endpointConfig: &endpointConfig{pkiid: []byte("5"), address: "unavailable1:12051", mspid: "msp1"}}
	unavailable2Mock = &endorser{endpointConfig: &endpointConfig{pkiid: []byte("6"), address: "unavailable2:13051", mspid: "msp1"}}
	unavailable3Mock = &endorser{endpointConfig: &endpointConfig{pkiid: []byte("7"), address: "unavailable3:14051", mspid: "msp1"}}
	endorsers        = map[string]*endorser{
		localhostMock.address: localhostMock,
		peer1Mock.address:     peer1Mock,
		peer2Mock.address:     peer2Mock,
		peer3Mock.address:     peer3Mock,
		peer4Mock.address:     peer4Mock,
	}
	orderer1Mock = &orderer{endpointConfig: &endpointConfig{pkiid: []byte("o1"), address: "orderer1:7050", mspid: "msp1"}}
	orderer2Mock = &orderer{endpointConfig: &endpointConfig{pkiid: []byte("o2"), address: "orderer2:7050", mspid: "msp1"}}
	orderer3Mock = &orderer{endpointConfig: &endpointConfig{pkiid: []byte("o3"), address: "orderer3:7050", mspid: "msp1"}}
	orderer4Mock = &orderer{endpointConfig: &endpointConfig{pkiid: []byte("o4"), address: "orderer4:7050", mspid: "msp1"}}
	orderer5Mock = &orderer{endpointConfig: &endpointConfig{pkiid: []byte("o5"), address: "orderer5:7050", mspid: "msp1"}}
	orderer6Mock = &orderer{endpointConfig: &endpointConfig{pkiid: []byte("o6"), address: "orderer6:7050", mspid: "msp1"}}
	orderer7Mock = &orderer{endpointConfig: &endpointConfig{pkiid: []byte("o7"), address: "orderer7:7050", mspid: "msp1"}}
	ordererMocks = map[string]*orderer{
		orderer1Mock.address: orderer1Mock,
		orderer2Mock.address: orderer2Mock,
		orderer3Mock.address: orderer3Mock,
		orderer4Mock.address: orderer4Mock,
		orderer5Mock.address: orderer5Mock,
		orderer6Mock.address: orderer6Mock,
		orderer7Mock.address: orderer7Mock,
	}
)

func channelToSlice(ch chan string) []string {
	slice := []string{}
	for {
		select {
		case value, ok := <-ch:
			if ok {
				slice = append(slice, value)
			} else {
				return slice
			}
		default:
			return slice
		}
	}
}

func TestNilArgs(t *testing.T) {
	server := newServer(
		&mocks.EndorserClient{},
		&mocks.Discovery{},
		&mocks.CommitFinder{},
		&mocks.ACLChecker{},
		&ledgermocks.Provider{},
		gdiscovery.NetworkMember{
			PKIid:    common.PKIidType("id1"),
			Endpoint: "localhost:7051",
		},
		"msp1",
		&comm.SecureOptions{},
		config.GetOptions(viper.New()),
		nil,
		nil,
		nil,
	)
	ctx := context.Background()

	_, err := server.Evaluate(ctx, nil)
	require.ErrorIs(t, err, status.Error(codes.InvalidArgument, "an evaluate request is required"))

	_, err = server.Evaluate(ctx, &pb.EvaluateRequest{ProposedTransaction: nil})
	require.ErrorIs(t, err, status.Error(codes.InvalidArgument, "failed to unpack transaction proposal: a signed proposal is required"))

	_, err = server.Evaluate(ctx, &pb.EvaluateRequest{ProposedTransaction: &peer.SignedProposal{}})
	require.ErrorIs(t, err, status.Error(codes.InvalidArgument, "failed to unpack transaction proposal: a signed proposal is required"))

	_, err = server.Evaluate(ctx, &pb.EvaluateRequest{ProposedTransaction: &peer.SignedProposal{ProposalBytes: []byte("jibberish")}})
	require.ErrorContains(t, err, "failed to unpack transaction proposal: error unmarshalling Proposal")

	request := &pb.EvaluateRequest{ProposedTransaction: &peer.SignedProposal{
		ProposalBytes: protoutil.MarshalOrPanic(&peer.Proposal{
			Header: protoutil.MarshalOrPanic(&cp.Header{}),
			Payload: protoutil.MarshalOrPanic(&peer.ChaincodeActionPayload{
				ChaincodeProposalPayload: protoutil.MarshalOrPanic(&peer.ChaincodeProposalPayload{
					Input: protoutil.MarshalOrPanic(&peer.ChaincodeInvocationSpec{
						ChaincodeSpec: &peer.ChaincodeSpec{
							ChaincodeId: &peer.ChaincodeID{
								Name: "testChaincode",
							},
						},
					}),
				}),
			}),
		}),
	}}
	require.True(t, len(request.GetProposedTransaction().GetProposalBytes()) != 0)
	_, err = server.Evaluate(ctx, request)
	require.ErrorIs(t, err, status.Error(codes.InvalidArgument, "failed to unpack transaction proposal: no channel id provided"))

	_, err = server.Evaluate(ctx, &pb.EvaluateRequest{ProposedTransaction: &peer.SignedProposal{
		ProposalBytes: protoutil.MarshalOrPanic(&peer.Proposal{
			Header: protoutil.MarshalOrPanic(&cp.Header{
				ChannelHeader: protoutil.MarshalOrPanic(&cp.ChannelHeader{
					ChannelId: "test",
				}),
			}),
			Payload: protoutil.MarshalOrPanic(&peer.ChaincodeActionPayload{
				ChaincodeProposalPayload: protoutil.MarshalOrPanic(&peer.ChaincodeProposalPayload{
					Input: nil,
				}),
			}),
		}),
	}})
	require.ErrorIs(t, err, status.Error(codes.InvalidArgument, "failed to unpack transaction proposal: no chaincode spec is provided, channel id [test]"))

	_, err = server.Evaluate(ctx, &pb.EvaluateRequest{ProposedTransaction: &peer.SignedProposal{
		ProposalBytes: protoutil.MarshalOrPanic(&peer.Proposal{
			Header: protoutil.MarshalOrPanic(&cp.Header{
				ChannelHeader: protoutil.MarshalOrPanic(&cp.ChannelHeader{
					ChannelId: "test",
				}),
			}),
			Payload: protoutil.MarshalOrPanic(&peer.ChaincodeActionPayload{
				ChaincodeProposalPayload: protoutil.MarshalOrPanic(&peer.ChaincodeProposalPayload{
					Input: protoutil.MarshalOrPanic(&peer.ChaincodeSpec{
						ChaincodeId: &peer.ChaincodeID{
							Name: "",
						},
					}),
				}),
			}),
		}),
	}})
	require.ErrorIs(t, err, status.Error(codes.InvalidArgument, "failed to unpack transaction proposal: no chaincode name is provided, channel id [test]"))

	_, err = server.Endorse(ctx, nil)
	require.ErrorIs(t, err, status.Error(codes.InvalidArgument, "an endorse request is required"))

	_, err = server.Endorse(ctx, &pb.EndorseRequest{ProposedTransaction: nil})
	require.ErrorIs(t, err, status.Error(codes.InvalidArgument, "the proposed transaction must contain a signed proposal"))

	_, err = server.Endorse(ctx, &pb.EndorseRequest{ProposedTransaction: &peer.SignedProposal{}})
	require.ErrorIs(t, err, status.Error(codes.InvalidArgument, "the proposed transaction must contain a signed proposal"))

	_, err = server.Endorse(ctx, &pb.EndorseRequest{ProposedTransaction: &peer.SignedProposal{ProposalBytes: []byte("jibberish")}})
	require.ErrorContains(t, err, "rpc error: code = InvalidArgument desc = error unmarshalling Proposal")

	_, err = server.Submit(ctx, nil)
	require.ErrorIs(t, err, status.Error(codes.InvalidArgument, "a submit request is required"))

	_, err = server.Submit(ctx, &pb.SubmitRequest{})
	require.ErrorIs(t, err, status.Error(codes.InvalidArgument, "a prepared transaction is required"))

	_, err = server.CommitStatus(ctx, nil)
	require.ErrorIs(t, err, status.Error(codes.InvalidArgument, "a commit status request is required"))

	_, err = server.CommitStatus(ctx, &pb.SignedCommitStatusRequest{})
	require.ErrorIs(t, err, status.Error(codes.InvalidArgument, "a commit status request is required"))

	err = server.ChaincodeEvents(nil, &mocks.ChaincodeEventsServer{})
	require.ErrorIs(t, err, status.Error(codes.InvalidArgument, "a chaincode events request is required"))

	err = server.ChaincodeEvents(&pb.SignedChaincodeEventsRequest{}, &mocks.ChaincodeEventsServer{})
	require.ErrorIs(t, err, status.Error(codes.InvalidArgument, "a chaincode events request is required"))
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
		localEndorser.ProcessProposalReturns(createErrorResponse(t, 500, epDef.proposalError.Error(), nil), nil)
	} else {
		localEndorser.ProcessProposalReturns(createProposalResponseWithInterest(t, localhostMock.address, localResponse, epDef.proposalResponseStatus, epDef.proposalResponseMessage, tt.interest), nil)
	}

	for _, e := range endorsers {
		e.client = &mocks.EndorserClient{}
		if epDef.proposalError != nil {
			e.client.(*mocks.EndorserClient).ProcessProposalReturns(createErrorResponse(t, 500, epDef.proposalError.Error(), nil), nil)
		} else {
			e.client.(*mocks.EndorserClient).ProcessProposalReturns(createProposalResponseWithInterest(t, e.address, epDef.proposalResponseValue, epDef.proposalResponseStatus, epDef.proposalResponseMessage, tt.interest), nil)
		}
	}

	for _, o := range ordererMocks {
		o.client = &mocks.ABClient{}
		if epDef.ordererBroadcastError != nil {
			o.client.(*mocks.ABClient).BroadcastReturns(nil, epDef.ordererBroadcastError)
			continue
		}
		abbc := &mocks.ABBClient{}
		abbc.SendReturns(epDef.ordererSendError)
		abbc.RecvReturns(&ab.BroadcastResponse{
			Info:   epDef.ordererResponse,
			Status: cp.Status(epDef.ordererStatus),
		}, epDef.ordererRecvError)
		o.client.(*mocks.ABClient).BroadcastReturns(abbc, nil)
	}

	mockSigner := &idmocks.SignerSerializer{}
	mockSigner.SignReturns([]byte("my_signature"), nil)

	mockFinder := &mocks.CommitFinder{}
	mockFinder.TransactionStatusReturns(tt.finderStatus, tt.finderErr)

	mockPolicy := &mocks.ACLChecker{}
	mockPolicy.CheckACLReturns(tt.policyErr)

	mockBlockIterator := &mocks.ResultsIterator{}
	blockChannel := make(chan *cp.Block, len(tt.blocks))
	for _, block := range tt.blocks {
		blockChannel <- block
	}
	close(blockChannel)
	mockBlockIterator.NextCalls(func() (ledger.QueryResult, error) {
		if tt.eventErr != nil {
			return nil, tt.eventErr
		}

		block := <-blockChannel
		if block == nil {
			return nil, errors.New("NO_MORE_BLOCKS")
		}

		return block, nil
	})

	mockLedger := &ledgermocks.Ledger{}
	ledgerInfo := &cp.BlockchainInfo{
		Height: tt.localLedgerHeight,
	}
	mockLedger.GetBlockchainInfoReturns(ledgerInfo, nil)
	mockLedger.GetBlocksIteratorReturns(mockBlockIterator, nil)

	mockLedgerProvider := &ledgermocks.Provider{}
	mockLedgerProvider.LedgerReturns(mockLedger, nil)

	validProposal := createProposal(t, testChannel, testChaincode, tt.transientData)
	validSignedProposal, err := protoutil.GetSignedProposal(validProposal, mockSigner)
	require.NoError(t, err)

	ca, err := tlsgen.NewCA()
	require.NoError(t, err)
	configResult := &dp.ConfigResult{
		Orderers: map[string]*dp.Endpoints{
			"msp1": {
				Endpoint: []*dp.Endpoint{
					{Host: "orderer1", Port: 7050},
				},
			},
		},
		Msps: map[string]*msp.FabricMSPConfig{
			"msp1": {
				TlsRootCerts: [][]byte{ca.CertBytes()},
			},
		},
	}

	if tt.config != nil {
		configResult = tt.config
	}

	members := []networkMember{
		{"id1", "localhost:7051", "msp1", 0},
		{"id2", "peer1:8051", "msp1", 0},
		{"id3", "peer2:9051", "msp2", 0},
		{"id4", "peer3:10051", "msp2", 0},
		{"id5", "peer4:11051", "msp3", 0},
	}

	if tt.members != nil {
		members = tt.members
	}

	disc := mockDiscovery(t, tt.plan, tt.layouts, members, configResult)

	options := config.Options{
		Enabled:            true,
		EndorsementTimeout: endorsementTimeout,
		BroadcastTimeout:   broadcastTimeout,
	}

	member := gdiscovery.NetworkMember{
		PKIid:    common.PKIidType("id1"),
		Endpoint: "localhost:7051",
	}

	getChannelConfig := func(channel string) channelconfig.Resources {
		cap := &mocks.ChannelCapabilities{}
		cap.ConsensusTypeBFTReturns(tt.isBFT)
		c := &mocks.Channel{}
		c.CapabilitiesReturns(cap)
		res := &mocks.Resources{}
		res.ChannelConfigReturns(c)
		return res
	}

	server := newServer(localEndorser, disc, mockFinder, mockPolicy, mockLedgerProvider, member, "msp1", &comm.SecureOptions{}, options, nil, tt.ordererEndpointOverrides, getChannelConfig)

	dialer := &mocks.Dialer{}
	dialer.Returns(nil, nil)
	server.registry.endpointFactory = createEndpointFactory(t, epDef, dialer.Spy, tt.ordererEndpointOverrides)

	ctx := context.WithValue(context.Background(), contextKey("orange"), "apples")

	pt := &preparedTest{
		server:         server,
		ctx:            ctx,
		signedProposal: validSignedProposal,
		localEndorser:  localEndorser,
		discovery:      disc,
		dialer:         dialer,
		finder:         mockFinder,
		eventsServer:   &mocks.ChaincodeEventsServer{},
		policy:         mockPolicy,
		ledgerProvider: mockLedgerProvider,
		ledger:         mockLedger,
		blockIterator:  mockBlockIterator,
	}
	if tt.postSetup != nil {
		tt.postSetup(t, pt)
	}
	return pt
}

func checkError(t *testing.T, tt *testDef, err error) (checked bool) {
	stringCheck := tt.errString != ""
	codeCheck := tt.errCode != codes.OK
	detailsCheck := len(tt.errDetails) > 0

	checked = stringCheck || codeCheck || detailsCheck
	if !checked {
		return
	}

	require.NotNil(t, err, "error")

	if stringCheck {
		require.ErrorContains(t, err, tt.errString, "error string")
	}

	s, ok := status.FromError(err)
	if !ok {
		s = status.FromContextError(err)
	}

	if codeCheck {
		require.Equal(t, tt.errCode.String(), s.Code().String(), "error status code")
	}

	if detailsCheck {
		require.Len(t, s.Details(), len(tt.errDetails))
		for _, detail := range s.Details() {
			require.Contains(t, tt.errDetails, detail, "error details, expected: %v", tt.errDetails)
		}
	}

	return
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
		require.Equal(t, 1, ec.ProcessProposalCallCount(), "Expected ProcessProposal() to be invoked on %s", e)
		ectx, prop, _ := ec.ProcessProposalArgsForCall(0)
		require.Equal(t, test.signedProposal, prop)
		require.Equal(t, "apples", ectx.Value(contextKey("orange")))
		// context timeout was set to -1s, so deadline should be in the past
		deadline, ok := ectx.Deadline()
		require.True(t, ok)
		require.Negative(t, time.Until(deadline))
	}
}

func mockDiscovery(t *testing.T, plan endorsementPlan, layouts []endorsementLayout, members []networkMember, config *dp.ConfigResult) *mocks.Discovery {
	discovery := &mocks.Discovery{}

	var peers []gdiscovery.NetworkMember
	var infoset []api.PeerIdentityInfo
	for _, member := range members {
		peers = append(peers, gdiscovery.NetworkMember{
			Endpoint:   member.endpoint,
			PKIid:      []byte(member.id),
			Properties: &gossip.Properties{Chaincodes: []*gossip.Chaincode{{Name: testChaincode}}, LedgerHeight: member.height},
		})
		infoset = append(infoset, api.PeerIdentityInfo{Organization: []byte(member.mspid), PKIId: []byte(member.id)})
	}
	ed := createMockEndorsementDescriptor(t, plan, layouts)
	discovery.PeersForEndorsementReturns(ed, nil)
	discovery.PeersOfChannelReturns(peers)
	discovery.IdentityInfoReturns(infoset)
	discovery.ConfigReturns(config, nil)
	return discovery
}

func createMockEndorsementDescriptor(t *testing.T, plan endorsementPlan, layouts []endorsementLayout) *dp.EndorsementDescriptor {
	quantitiesByGroup := map[string]uint32{}
	endorsersByGroups := map[string]*dp.Peers{}
	for group, endorsers := range plan {
		quantitiesByGroup[group] = 1 // for now
		var peers []*dp.Peer
		for _, endorser := range endorsers {
			peers = append(peers, createMockPeer(t, &endorser))
		}
		endorsersByGroups[group] = &dp.Peers{Peers: peers}
	}
	var layoutDef []*dp.Layout
	if layouts != nil {
		for _, layout := range layouts {
			layoutDef = append(layoutDef, &dp.Layout{QuantitiesByGroup: layout})
		}
	} else {
		// default single layout - one from each group
		layoutDef = []*dp.Layout{{QuantitiesByGroup: quantitiesByGroup}}
	}
	descriptor := &dp.EndorsementDescriptor{
		Chaincode:         "my_channel",
		Layouts:           layoutDef,
		EndorsersByGroups: endorsersByGroups,
	}
	return descriptor
}

func createMockPeer(t *testing.T, endorser *endorserState) *dp.Peer {
	aliveMsgBytes, err := proto.Marshal(
		&gossip.GossipMessage{
			Content: &gossip.GossipMessage_AliveMsg{
				AliveMsg: &gossip.AliveMessage{
					Membership: &gossip.Member{Endpoint: endorser.endorser.address},
				},
			},
		})

	require.NoError(t, err)

	stateInfoBytes, err := proto.Marshal(
		&gossip.GossipMessage{
			Content: &gossip.GossipMessage_StateInfo{
				StateInfo: &gossip.StateInfo{
					Properties: &gossip.Properties{
						LedgerHeight: endorser.height,
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
		Identity: marshal(&msp.SerializedIdentity{
			IdBytes: []byte(endorser.endorser.address),
			Mspid:   endorser.endorser.mspid,
		}, t),
	}
}

func createEndpointFactory(t *testing.T, definition *endpointDef, dialer dialer, ordererEndpointOverrides map[string]*orderers.Endpoint) *endpointFactory {
	var endpoint string
	ca, err := tlsgen.NewCA()
	require.NoError(t, err, "failed to create CA")
	pair, err := ca.NewClientCertKeyPair()
	require.NoError(t, err, "failed to create client key pair")
	return &endpointFactory{
		timeout: 5 * time.Second,
		connectEndorser: func(conn *grpc.ClientConn) peer.EndorserClient {
			if ep, ok := endorsers[endpoint]; ok && ep.client != nil {
				return ep.client
			}
			return nil
		},
		connectOrderer: func(conn *grpc.ClientConn) ab.AtomicBroadcastClient {
			if ep, ok := ordererMocks[endpoint]; ok && ep.client != nil {
				return ep.client
			}
			return nil
		},
		dialer: func(ctx context.Context, target string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
			endpoint = target
			for from, to := range ordererEndpointOverrides {
				if to.Address == target {
					endpoint = from
				}
			}
			return dialer(ctx, target, opts...)
		},
		clientKey:                pair.Key,
		clientCert:               pair.Cert,
		ordererEndpointOverrides: ordererEndpointOverrides,
	}
}

func createProposal(t *testing.T, channel string, chaincode string, transient map[string][]byte, args ...[]byte) *peer.Proposal {
	invocationSpec := &peer.ChaincodeInvocationSpec{
		ChaincodeSpec: &peer.ChaincodeSpec{
			Type:        peer.ChaincodeSpec_NODE,
			ChaincodeId: &peer.ChaincodeID{Name: chaincode},
			Input:       &peer.ChaincodeInput{Args: args},
		},
	}

	proposal, _, err := protoutil.CreateChaincodeProposalWithTransient(
		cp.HeaderType_ENDORSER_TRANSACTION,
		channel,
		invocationSpec,
		[]byte{},
		transient,
	)

	require.NoError(t, err, "Failed to create the proposal")

	return proposal
}

func createProposalResponse(t *testing.T, endorser, value string, status int32, errMessage string) *peer.ProposalResponse {
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
	endorsement := &peer.Endorsement{
		Endorser: []byte(endorser),
	}

	return &peer.ProposalResponse{
		Payload:     marshal(payload, t),
		Response:    response,
		Endorsement: endorsement,
	}
}

func createProposalResponseWithInterest(t *testing.T, endorser, value string, status int32, errMessage string, interest *peer.ChaincodeInterest) *peer.ProposalResponse {
	response := createProposalResponse(t, endorser, value, status, errMessage)
	if interest != nil {
		response.Interest = interest
	}
	return response
}

func createErrorResponse(t *testing.T, status int32, errMessage string, payload []byte) *peer.ProposalResponse {
	return &peer.ProposalResponse{
		Response: &peer.Response{
			Status:  status,
			Payload: payload,
			Message: errMessage,
		},
	}
}

func marshal(msg proto.Message, t *testing.T) []byte {
	buf, err := proto.Marshal(msg)
	require.NoError(t, err, "Failed to marshal message")
	return buf
}

func TestRpcErrorWithBadDetails(t *testing.T) {
	err := newRpcError(codes.InvalidArgument, "terrible error", nil)
	require.ErrorIs(t, err, status.Error(codes.InvalidArgument, "terrible error"))
}
