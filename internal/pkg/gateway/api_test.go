/*
Copyright 2021 IBM All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gateway

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/timestamp"
	cp "github.com/hyperledger/fabric-protos-go/common"
	dp "github.com/hyperledger/fabric-protos-go/discovery"
	pb "github.com/hyperledger/fabric-protos-go/gateway"
	"github.com/hyperledger/fabric-protos-go/gossip"
	"github.com/hyperledger/fabric-protos-go/msp"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/flogging/mock"
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
	"go.uber.org/zap/zapcore"
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
	localhostMock    = &endorser{endpointConfig: &endpointConfig{pkiid: []byte("0"), address: "localhost:7051", mspid: "msp1"}}
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
)

func TestEvaluate(t *testing.T) {
	tests := []testDef{
		{
			name: "single endorser",
			members: []networkMember{
				{"id1", "localhost:7051", "msp1", 5},
			},
		},
		{
			name:      "no endorsers",
			plan:      endorsementPlan{},
			members:   []networkMember{},
			errCode:   codes.FailedPrecondition,
			errString: "no peers available to evaluate chaincode test_chaincode in channel test_channel",
		},
		{
			name: "five endorsers, prefer local org",
			members: []networkMember{
				{"id1", "localhost:7051", "msp1", 5},
				{"id2", "peer1:8051", "msp1", 6},
				{"id3", "peer2:9051", "msp2", 6},
				{"id4", "peer3:10051", "msp2", 5},
				{"id5", "peer4:11051", "msp3", 6},
			},
			expectedEndorsers: []string{"peer1:8051"},
		},
		{
			name: "five endorsers, prefer host peer",
			members: []networkMember{
				{"id1", "localhost:7051", "msp1", 5},
				{"id2", "peer1:8051", "msp1", 5},
				{"id3", "peer2:9051", "msp2", 6},
				{"id4", "peer3:10051", "msp2", 5},
				{"id5", "peer4:11051", "msp3", 6},
			},
			expectedEndorsers: []string{"localhost:7051"},
		},
		{
			name: "five endorsers, prefer host peer despite no endpoint",
			members: []networkMember{
				{"id1", "", "msp1", 5},
				{"id2", "peer1:8051", "msp1", 5},
				{"id3", "peer2:9051", "msp2", 6},
				{"id4", "peer3:10051", "msp2", 5},
				{"id5", "peer4:11051", "msp3", 6},
			},
			expectedEndorsers: []string{"localhost:7051"},
		},
		{
			name: "evaluate with targetOrganizations, prefer local org despite block height",
			members: []networkMember{
				{"id1", "localhost:7051", "msp1", 5},
				{"id2", "peer1:8051", "msp1", 5},
				{"id3", "peer2:9051", "msp2", 6},
				{"id4", "peer3:10051", "msp2", 5},
				{"id5", "peer4:11051", "msp3", 6},
			},
			endorsingOrgs:     []string{"msp3", "msp1"},
			expectedEndorsers: []string{"localhost:7051"},
		},
		{
			name: "evaluate with targetOrganizations that doesn't include local org, prefer highest block height",
			members: []networkMember{
				{"id1", "localhost:7051", "msp1", 5},
				{"id2", "peer1:8051", "msp1", 5},
				{"id3", "peer2:9051", "msp2", 6},
				{"id4", "peer3:10051", "msp2", 5},
				{"id5", "peer4:11051", "msp3", 7},
			},
			endorsingOrgs:     []string{"msp2", "msp3"},
			expectedEndorsers: []string{"peer4:11051"},
		},
		{
			name: "evaluate with transient data should select local org, highest block height",
			members: []networkMember{
				{"id1", "localhost:7051", "msp1", 4},
				{"id2", "peer1:8051", "msp1", 5},
				{"id3", "peer2:9051", "msp2", 6},
				{"id4", "peer3:10051", "msp2", 5},
				{"id5", "peer4:11051", "msp3", 7},
			},
			transientData:     map[string][]byte{"transient-key": []byte("transient-value")},
			expectedEndorsers: []string{"peer1:8051"},
		},
		{
			name: "evaluate with transient data should fail if local org not available",
			members: []networkMember{
				{"id3", "peer2:9051", "msp2", 6},
				{"id4", "peer3:10051", "msp2", 5},
				{"id5", "peer4:11051", "msp3", 7},
			},
			transientData: map[string][]byte{"transient-key": []byte("transient-value")},
			errCode:       codes.FailedPrecondition,
			errString:     "no endorsers found in the gateway's organization; retry specifying target organization(s) to protect transient data: no peers available to evaluate chaincode test_chaincode in channel test_channel",
		},
		{
			name: "evaluate with transient data and target (non-local) orgs should select the highest block height peer",
			members: []networkMember{
				{"id1", "localhost:7051", "msp1", 11},
				{"id2", "peer1:8051", "msp1", 5},
				{"id3", "peer2:9051", "msp2", 6},
				{"id4", "peer3:10051", "msp2", 9},
				{"id5", "peer4:11051", "msp3", 7},
			},
			transientData:     map[string][]byte{"transient-key": []byte("transient-value")},
			endorsingOrgs:     []string{"msp2", "msp3"},
			expectedEndorsers: []string{"peer3:10051"},
		},
		{
			name: "process proposal fails",
			members: []networkMember{
				{"id1", "localhost:7051", "msp1", 5},
			},
			endpointDefinition: &endpointDef{
				proposalError: status.Error(codes.Aborted, "wibble"),
			},
			errCode:   codes.Aborted,
			errString: "failed to evaluate transaction, see attached details for more info",
			errDetails: []*pb.ErrorDetail{{
				Address: "localhost:7051",
				MspId:   "msp1",
				Message: "rpc error: code = Aborted desc = wibble",
			}},
		},
		{
			name: "process proposal chaincode error",
			members: []networkMember{
				{"id2", "peer1:8051", "msp1", 5},
			},
			endpointDefinition: &endpointDef{
				proposalResponseStatus:  400,
				proposalResponseMessage: "Mock chaincode error",
			},
			errCode:   codes.Unknown,
			errString: "evaluate call to endorser returned error: chaincode response 400, Mock chaincode error",
			errDetails: []*pb.ErrorDetail{{
				Address: "peer1:8051",
				MspId:   "msp1",
				Message: "chaincode response 400, Mock chaincode error",
			}},
		},
		{
			name: "evaluate on local org fails - retry in other org",
			members: []networkMember{
				{"id1", "localhost:7051", "msp1", 4},
				{"id2", "peer1:8051", "msp1", 4},
				{"id3", "peer2:9051", "msp2", 3},
				{"id4", "peer3:10051", "msp2", 4},
				{"id5", "peer4:11051", "msp3", 5},
			},
			plan: endorsementPlan{
				"g1": {{endorser: localhostMock, height: 4}, {endorser: peer1Mock, height: 4}}, // msp1
				"g2": {{endorser: peer2Mock, height: 3}, {endorser: peer3Mock, height: 4}},     // msp2
				"g3": {{endorser: peer4Mock, height: 5}},                                       // msp3
			},
			postSetup: func(t *testing.T, def *preparedTest) {
				def.localEndorser.ProcessProposalReturns(createErrorResponse(t, 500, "bad local endorser", nil), nil)
				peer1Mock.client.(*mocks.EndorserClient).ProcessProposalReturns(createErrorResponse(t, 500, "bad peer1 endorser", nil), nil)
			},
			expectedEndorsers: []string{"peer4:11051"},
		},
		{
			name: "restrict to local org peers - which all fail",
			members: []networkMember{
				{"id1", "localhost:7051", "msp1", 4},
				{"id2", "peer1:8051", "msp1", 4},
				{"id3", "peer2:9051", "msp2", 3},
				{"id4", "peer3:10051", "msp2", 4},
				{"id5", "peer4:11051", "msp3", 5},
			},
			plan: endorsementPlan{
				"g1": {{endorser: localhostMock, height: 4}, {endorser: peer1Mock, height: 4}}, // msp1
				"g2": {{endorser: peer2Mock, height: 3}, {endorser: peer3Mock, height: 4}},     // msp2
				"g3": {{endorser: peer4Mock, height: 5}},                                       // msp3
			},
			postSetup: func(t *testing.T, def *preparedTest) {
				def.localEndorser.ProcessProposalReturns(createErrorResponse(t, 500, "bad local endorser", nil), nil)
				peer1Mock.client.(*mocks.EndorserClient).ProcessProposalReturns(createErrorResponse(t, 500, "bad peer1 endorser", nil), nil)
			},
			endorsingOrgs: []string{"msp1"},
			errCode:       codes.Aborted,
			errString:     "failed to evaluate transaction, see attached details for more info",
			errDetails: []*pb.ErrorDetail{
				{
					Address: "localhost:7051",
					MspId:   "msp1",
					Message: "bad local endorser",
				},
				{
					Address: "peer1:8051",
					MspId:   "msp1",
					Message: "bad peer1 endorser",
				},
			},
		},
		{
			name: "fails due to invalid signature (pre-process check) - does not retry",
			members: []networkMember{
				{"id1", "localhost:7051", "msp1", 4},
				{"id2", "peer1:8051", "msp1", 4},
				{"id3", "peer2:9051", "msp2", 3},
				{"id4", "peer3:10051", "msp2", 4},
				{"id5", "peer4:11051", "msp3", 5},
			},
			plan: endorsementPlan{
				"g1": {{endorser: localhostMock, height: 4}, {endorser: peer1Mock, height: 4}}, // msp1
				"g2": {{endorser: peer2Mock, height: 3}, {endorser: peer3Mock, height: 4}},     // msp2
				"g3": {{endorser: peer4Mock, height: 5}},                                       // msp3
			},
			postSetup: func(t *testing.T, def *preparedTest) {
				def.localEndorser.ProcessProposalReturns(createErrorResponse(t, 500, "invalid signature", nil), fmt.Errorf("invalid signature"))
			},
			endorsingOrgs: []string{"msp1"},
			errCode:       codes.FailedPrecondition, // Code path could fail for reasons other than authentication
			errString:     "evaluate call to endorser returned error: invalid signature",
			errDetails: []*pb.ErrorDetail{
				{
					Address: "localhost:7051",
					MspId:   "msp1",
					Message: "invalid signature",
				},
			},
		},
		{
			name: "fails due to chaincode panic - retry on next peer",
			members: []networkMember{
				{"id1", "localhost:7051", "msp1", 4},
				{"id2", "peer1:8051", "msp1", 4},
				{"id3", "peer2:9051", "msp2", 3},
				{"id4", "peer3:10051", "msp2", 4},
				{"id5", "peer4:11051", "msp3", 5},
			},
			plan: endorsementPlan{
				"g1": {{endorser: localhostMock, height: 4}, {endorser: peer1Mock, height: 4}}, // msp1
				"g2": {{endorser: peer2Mock, height: 3}, {endorser: peer3Mock, height: 4}},     // msp2
				"g3": {{endorser: peer4Mock, height: 5}},                                       // msp3
			},
			postSetup: func(t *testing.T, def *preparedTest) {
				def.localEndorser.ProcessProposalReturns(createErrorResponse(t, 500, "error in simulation: chaincode stream terminated", nil), nil)
				peer1Mock.client.(*mocks.EndorserClient).ProcessProposalReturns(createErrorResponse(t, 500, "error in simulation: chaincode stream terminated", nil), nil)
			},
			endorsingOrgs: []string{"msp1"},
			errCode:       codes.Aborted,
			errString:     "failed to evaluate transaction, see attached details for more info",
			errDetails: []*pb.ErrorDetail{
				{
					Address: "localhost:7051",
					MspId:   "msp1",
					Message: "error in simulation: chaincode stream terminated",
				},
				{
					Address: "peer1:8051",
					MspId:   "msp1",
					Message: "error in simulation: chaincode stream terminated",
				},
			},
		},
		{
			name: "dialing endorser endpoint fails",
			members: []networkMember{
				{"id3", "peer2:9051", "msp2", 5},
			},
			postSetup: func(t *testing.T, def *preparedTest) {
				def.dialer.Calls(func(_ context.Context, target string, _ ...grpc.DialOption) (*grpc.ClientConn, error) {
					if target == "peer2:9051" {
						return nil, fmt.Errorf("endorser not answering")
					}
					return nil, nil
				})
			},
			errCode:   codes.Unavailable,
			errString: "failed to create new connection: endorser not answering",
		},
		{
			name: "discovery returns incomplete information - no Properties",
			postSetup: func(t *testing.T, def *preparedTest) {
				def.discovery.PeersOfChannelReturns([]gdiscovery.NetworkMember{{
					Endpoint: "localhost:7051",
					PKIid:    []byte("ill-defined"),
				}})
			},
			errCode:   codes.FailedPrecondition,
			errString: "no peers available to evaluate chaincode test_chaincode in channel test_channel",
		},
		{
			name: "context timeout during evaluate",
			plan: endorsementPlan{
				"g1": {{endorser: localhostMock, height: 3}}, // msp1
			},
			postSetup: func(t *testing.T, def *preparedTest) {
				def.ctx, def.cancel = context.WithTimeout(def.ctx, 100*time.Millisecond)

				def.localEndorser.ProcessProposalStub = func(ctx context.Context, proposal *peer.SignedProposal, option ...grpc.CallOption) (*peer.ProposalResponse, error) {
					time.Sleep(200 * time.Millisecond)
					return createProposalResponse(t, peer1Mock.address, "mock_response", 200, ""), nil
				}
			},
			postTest: func(t *testing.T, def *preparedTest) {
				def.cancel()
			},
			errCode:   codes.DeadlineExceeded,
			errString: "evaluate timeout expired",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			test := prepareTest(t, &tt)

			response, err := test.server.Evaluate(test.ctx, &pb.EvaluateRequest{ProposedTransaction: test.signedProposal, TargetOrganizations: tt.endorsingOrgs})

			if checkError(t, &tt, err) {
				require.Nil(t, response, "response on error")
				return
			}

			// test the assertions

			require.NoError(t, err)
			// assert the result is the payload from the proposal response returned by the local endorser
			require.Equal(t, []byte("mock_response"), response.Result.Payload, "Incorrect result")

			// check the correct endorsers (mock) were called with the right parameters
			checkEndorsers(t, tt.expectedEndorsers, test)

			// check the discovery service (mock) was invoked as expected
			expectedChannel := common.ChannelID(testChannel)
			require.Equal(t, 2, test.discovery.PeersOfChannelCallCount())
			channel := test.discovery.PeersOfChannelArgsForCall(0)
			require.Equal(t, expectedChannel, channel)
			channel = test.discovery.PeersOfChannelArgsForCall(1)
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
				"g1": {{endorser: localhostMock, height: 3}}, // msp1
				"g2": {{endorser: peer2Mock, height: 3}},     // msp2
			},
			expectedEndorsers: []string{"localhost:7051", "peer2:9051"},
		},
		{
			name: "three endorsers, two groups",
			plan: endorsementPlan{
				"g1": {{endorser: localhostMock, height: 4}},                               // msp1
				"g2": {{endorser: peer3Mock, height: 4}, {endorser: peer2Mock, height: 5}}, // msp2
			},
			expectedEndorsers: []string{"localhost:7051", "peer2:9051"},
		},
		{
			name: "multiple endorsers, two groups, prefer host peer",
			plan: endorsementPlan{
				"g1": {{endorser: peer1Mock, height: 4}, {endorser: localhostMock, height: 4}, {endorser: unavailable1Mock, height: 4}}, // msp1
				"g2": {{endorser: peer3Mock, height: 4}, {endorser: peer2Mock, height: 5}},                                              // msp2
			},
			expectedEndorsers: []string{"localhost:7051", "peer2:9051"},
		},
		{
			name:              "endorse with specified orgs, despite block height",
			endorsingOrgs:     []string{"msp1", "msp3"},
			expectedEndorsers: []string{"localhost:7051", "peer4:11051"},
		},
		{
			name:              "endorse with specified orgs, doesn't include local peer",
			endorsingOrgs:     []string{"msp2", "msp3"},
			expectedEndorsers: []string{"peer2:9051", "peer4:11051"},
		},
		{
			name:          "endorse with specified orgs, but fails to satisfy one org",
			endorsingOrgs: []string{"msp2", "msp4"},
			errCode:       codes.Unavailable,
			errString:     "failed to find any endorsing peers for org(s): msp4",
		},
		{
			name:          "endorse with specified orgs, but fails to satisfy two orgs",
			endorsingOrgs: []string{"msp2", "msp4", "msp5"},
			errCode:       codes.Unavailable,
			errString:     "failed to find any endorsing peers for org(s): msp4, msp5",
		},
		{
			name: "endorse retry - localhost and peer3 fail - retry on peer1 and peer2",
			plan: endorsementPlan{
				"g1": {{endorser: localhostMock, height: 4}, {endorser: peer1Mock, height: 4}}, // msp1
				"g2": {{endorser: peer2Mock, height: 3}, {endorser: peer3Mock, height: 4}},     // msp2
				"g3": {{endorser: peer4Mock, height: 5}},                                       // msp3
			},
			layouts: []endorsementLayout{
				{"g1": 1, "g2": 1},
			},
			postSetup: func(t *testing.T, def *preparedTest) {
				def.localEndorser.ProcessProposalReturns(nil, status.Error(codes.Aborted, "bad local endorser"))
				peer3Mock.client.(*mocks.EndorserClient).ProcessProposalReturns(nil, status.Error(codes.Aborted, "bad peer3 endorser"))
			},
			expectedEndorsers: []string{"peer1:8051", "peer2:9051"},
		},
		{
			name: "endorse retry - org3 fail - retry with layout 3",
			plan: endorsementPlan{
				"g1": {{endorser: localhostMock, height: 4}, {endorser: peer1Mock, height: 4}}, // msp1
				"g2": {{endorser: peer2Mock, height: 3}, {endorser: peer3Mock, height: 4}},     // msp2
				"g3": {{endorser: peer4Mock, height: 5}},                                       // msp3
			},
			layouts: []endorsementLayout{
				{"g1": 1, "g3": 1},
				{"g2": 1, "g3": 1},
				{"g1": 1, "g2": 1},
			},
			postSetup: func(t *testing.T, def *preparedTest) {
				peer2Mock.client.(*mocks.EndorserClient).ProcessProposalReturns(nil, status.Error(codes.Aborted, "bad peer2 endorser"))
				peer3Mock.client.(*mocks.EndorserClient).ProcessProposalReturns(createProposalResponse(t, peer3Mock.address, "mock_response", 200, ""), nil)
				peer4Mock.client.(*mocks.EndorserClient).ProcessProposalReturns(nil, status.Error(codes.Aborted, "bad peer4 endorser"))
			},
			expectedEndorsers: []string{"localhost:7051", "peer3:10051"},
		},
		{
			name: "endorse retry - org3 fail & 1 org2 peer fail - requires 2 from org1",
			plan: endorsementPlan{
				"g1": {{endorser: localhostMock, height: 4}, {endorser: peer1Mock, height: 4}}, // msp1
				"g2": {{endorser: peer2Mock, height: 5}, {endorser: peer3Mock, height: 4}},     // msp2
				"g3": {{endorser: peer4Mock, height: 5}},                                       // msp3
			},
			layouts: []endorsementLayout{
				{"g1": 1, "g2": 1, "g3": 1},
				{"g1": 1, "g2": 2},
				{"g1": 2, "g2": 1},
			},
			postSetup: func(t *testing.T, def *preparedTest) {
				peer2Mock.client.(*mocks.EndorserClient).ProcessProposalReturns(nil, status.Error(codes.Aborted, "bad peer2 endorser"))
				peer3Mock.client.(*mocks.EndorserClient).ProcessProposalReturns(createProposalResponse(t, peer3Mock.address, "mock_response", 200, ""), nil)
				peer4Mock.client.(*mocks.EndorserClient).ProcessProposalReturns(nil, status.Error(codes.Aborted, "bad peer4 endorser"))
			},
			expectedEndorsers: []string{"localhost:7051", "peer1:8051", "peer3:10051"},
		},
		{
			name: "endorse retry - org 2 & org3 fail - fails to endorse",
			plan: endorsementPlan{
				"g1": {{endorser: localhostMock, height: 4}, {endorser: peer1Mock, height: 4}}, // msp1
				"g2": {{endorser: peer2Mock, height: 3}, {endorser: peer3Mock, height: 4}},     // msp2
				"g3": {{endorser: peer4Mock, height: 5}},                                       // msp3
			},
			layouts: []endorsementLayout{
				{"g1": 1, "g3": 1},
				{"g2": 1, "g3": 1},
				{"g1": 1, "g2": 1},
			},
			postSetup: func(t *testing.T, def *preparedTest) {
				peer2Mock.client.(*mocks.EndorserClient).ProcessProposalReturns(nil, status.Error(codes.Aborted, "bad peer2 endorser"))
				peer3Mock.client.(*mocks.EndorserClient).ProcessProposalReturns(nil, status.Error(codes.Aborted, "bad peer3 endorser"))
				peer4Mock.client.(*mocks.EndorserClient).ProcessProposalReturns(nil, status.Error(codes.Aborted, "bad peer4 endorser"))
			},
			errCode:   codes.Aborted,
			errString: "failed to collect enough transaction endorsements, see attached details for more info",
			errDetails: []*pb.ErrorDetail{
				{Address: "peer2:9051", MspId: "msp2", Message: "rpc error: code = Aborted desc = bad peer2 endorser"},
				{Address: "peer3:10051", MspId: "msp2", Message: "rpc error: code = Aborted desc = bad peer3 endorser"},
				{Address: "peer4:11051", MspId: "msp3", Message: "rpc error: code = Aborted desc = bad peer4 endorser"},
			},
		},
		{
			name: "endorse with multiple layouts - non-availability forces second layout",
			plan: endorsementPlan{
				"g1": {{endorser: localhostMock, height: 4}, {endorser: peer1Mock, height: 4}},           // msp1
				"g2": {{endorser: unavailable1Mock, height: 3}, {endorser: unavailable2Mock, height: 4}}, // msp2
				"g3": {{endorser: peer4Mock, height: 5}},                                                 // msp3
			},
			layouts: []endorsementLayout{
				{"g1": 1, "g2": 1},
				{"g1": 1, "g3": 1},
				{"g2": 1, "g3": 1},
			},
			expectedEndorsers: []string{"localhost:7051", "peer4:11051"},
		},
		{
			name: "non-local endorsers",
			plan: endorsementPlan{
				"g1": {{endorser: peer2Mock, height: 3}, {endorser: peer3Mock, height: 4}}, // msp2
				"g2": {{endorser: peer4Mock, height: 5}},                                   // msp3
			},
			layouts: []endorsementLayout{
				{"g1": 1, "g2": 1},
			},
			members: []networkMember{
				{"id2", "peer2:9051", "msp2", 3},
				{"id3", "peer3:10051", "msp2", 4},
				{"id4", "peer4:11051", "msp3", 5},
			},
			expectedEndorsers: []string{"peer3:10051", "peer4:11051"},
		},
		{
			name: "local endorser is not in the endorsement plan",
			plan: endorsementPlan{
				"g1": {{endorser: peer2Mock, height: 3}, {endorser: peer3Mock, height: 4}}, // msp2
				"g2": {{endorser: peer4Mock, height: 5}},                                   // msp3
			},
			layouts: []endorsementLayout{
				{"g1": 1, "g2": 1},
			},
			members: []networkMember{
				{"id1", "localhost:7051", "msp1", 3},
				{"id2", "peer2:9051", "msp2", 3},
				{"id3", "peer3:10051", "msp2", 4},
				{"id4", "peer4:11051", "msp3", 5},
			},
			expectedEndorsers: []string{"peer3:10051", "peer4:11051"},
		},
		{
			name: "non-local endorsers with transient data will fail",
			plan: endorsementPlan{
				"g1": {{endorser: peer2Mock, height: 3}, {endorser: peer3Mock, height: 4}}, // msp2
				"g2": {{endorser: peer4Mock, height: 5}},                                   // msp3
			},
			members: []networkMember{
				{"id2", "peer2:9051", "msp2", 3},
				{"id3", "peer3:10051", "msp2", 4},
				{"id4", "peer4:11051", "msp3", 5},
			},
			transientData: map[string][]byte{"transient-key": []byte("transient-value")},
			errCode:       codes.FailedPrecondition,
			errString:     "no endorsers found in the gateway's organization; retry specifying endorsing organization(s) to protect transient data",
		},
		{
			name: "local and non-local endorsers with transient data will fail",
			plan: endorsementPlan{
				"g1": {{endorser: localhostMock, height: 3}}, // msp1
				"g2": {{endorser: peer3Mock, height: 4}},     // msp2
			},
			layouts: []endorsementLayout{
				{"g1": 1, "g2": 1},
			},
			members: []networkMember{
				{"id1", "localhost:7051", "msp1", 3},
				{"id3", "peer3:10051", "msp2", 4},
			},
			interest: &peer.ChaincodeInterest{
				Chaincodes: []*peer.ChaincodeCall{{
					Name:            testChaincode,
					CollectionNames: []string{"mycollection1", "mycollection2"},
					NoPrivateReads:  true,
				}},
			},
			transientData: map[string][]byte{"transient-key": []byte("transient-value")},
			postSetup: func(t *testing.T, def *preparedTest) {
				def.discovery.PeersForEndorsementReturnsOnCall(0, nil, errors.New("protect-transient"))
			},
			errCode:   codes.FailedPrecondition,
			errString: "requires endorsement from organisation(s) that are not in the distribution policy of the private data collection(s): [mycollection1 mycollection2]; retry specifying trusted endorsing organizations to protect transient data",
		},
		{
			name: "extra endorsers with transient data",
			plan: endorsementPlan{
				"g1": {{endorser: localhostMock, height: 4}, {endorser: peer1Mock, height: 4}}, // msp1
				"g2": {{endorser: peer4Mock, height: 5}},                                       // msp3
			},
			transientData:     map[string][]byte{"transient-key": []byte("transient-value")},
			expectedEndorsers: []string{"localhost:7051", "peer4:11051"},
		},
		{
			name: "non-local endorsers with transient data and set endorsing orgs",
			plan: endorsementPlan{
				"g1": {{endorser: peer2Mock, height: 3}, {endorser: peer3Mock, height: 4}}, // msp2
				"g2": {{endorser: peer4Mock, height: 5}},                                   // msp3
			},
			members: []networkMember{
				{"id2", "peer2:9051", "msp2", 3},
				{"id3", "peer3:10051", "msp2", 4},
				{"id4", "peer4:11051", "msp3", 5},
			},
			endorsingOrgs:     []string{"msp2", "msp3"},
			transientData:     map[string][]byte{"transient-key": []byte("transient-value")},
			expectedEndorsers: []string{"peer3:10051", "peer4:11051"},
		},
		{
			name: "endorse with multiple layouts - non-availability of peers fails on all layouts",
			plan: endorsementPlan{
				"g1": {{endorser: localhostMock, height: 4}, {endorser: peer1Mock, height: 4}},           // msp1
				"g2": {{endorser: unavailable1Mock, height: 3}, {endorser: unavailable2Mock, height: 4}}, // msp2
				"g3": {{endorser: unavailable3Mock, height: 5}},                                          // msp3
			},
			layouts: []endorsementLayout{
				{"g1": 1, "g2": 1},
				{"g1": 1, "g3": 1},
				{"g2": 1, "g3": 1},
			},
			errCode: codes.FailedPrecondition,
			// the following is a substring of the error message - the endpoints get listed in indeterminate order which would lead to flaky test
			errString: "failed to select a set of endorsers that satisfy the endorsement policy due to unavailability of peers",
		},
		{
			name: "non-matching responses",
			plan: endorsementPlan{
				"g1": {{endorser: localhostMock, height: 4}}, // msp1
				"g2": {{endorser: peer2Mock, height: 5}},     // msp2
			},
			localResponse: "different_response",
			errCode:       codes.Aborted,
			errString:     "failed to collect enough transaction endorsements",
			errDetails: []*pb.ErrorDetail{
				{
					Address: "peer2:9051",
					MspId:   "msp2",
					Message: "ProposalResponsePayloads do not match",
				},
			},
			postSetup: func(t *testing.T, def *preparedTest) {
				logObserver := &mock.Observer{}
				logObserver.WriteEntryStub = func(entry zapcore.Entry, fields []zapcore.Field) {
					if strings.HasPrefix(entry.Message, "Proposal response mismatch") {
						for _, field := range fields {
							def.logFields = append(def.logFields, field.String)
						}
					}
				}
				flogging.SetObserver(logObserver)
			},
			postTest: func(t *testing.T, def *preparedTest) {
				require.Equal(t, "chaincode response mismatch", def.logFields[0])
				require.Equal(t, "status: 200, message: , payload: different_response", def.logFields[1])
				require.Equal(t, "status: 200, message: , payload: mock_response", def.logFields[2])
				flogging.SetObserver(nil)
			},
		},
		{
			name: "non-matching response logging suppressed",
			plan: endorsementPlan{
				"g1": {{endorser: localhostMock, height: 4}}, // msp1
				"g2": {{endorser: peer2Mock, height: 5}},     // msp2
			},
			localResponse: "different_response",
			errCode:       codes.Aborted,
			errString:     "failed to collect enough transaction endorsements",
			errDetails: []*pb.ErrorDetail{
				{
					Address: "peer2:9051",
					MspId:   "msp2",
					Message: "ProposalResponsePayloads do not match",
				},
			},
			postSetup: func(t *testing.T, def *preparedTest) {
				def.logLevel = flogging.LoggerLevel("gateway.responsechecker")
				flogging.ActivateSpec("error")
				logObserver := &mock.Observer{}
				logObserver.WriteEntryStub = func(entry zapcore.Entry, fields []zapcore.Field) {
					if strings.HasPrefix(entry.Message, "Proposal response mismatch") {
						for _, field := range fields {
							def.logFields = append(def.logFields, field.String)
						}
					}
				}
				flogging.SetObserver(logObserver)
			},
			postTest: func(t *testing.T, def *preparedTest) {
				require.Empty(t, def.logFields)
				flogging.ActivateSpec(def.logLevel)
				flogging.SetObserver(nil)
			},
		},
		{
			name: "discovery fails",
			plan: endorsementPlan{
				"g1": {{endorser: localhostMock, height: 2}},
			},
			postSetup: func(t *testing.T, def *preparedTest) {
				def.discovery.PeersForEndorsementReturns(nil, fmt.Errorf("peach-melba"))
			},
			errCode:   codes.FailedPrecondition,
			errString: "no combination of peers can be derived which satisfy the endorsement policy: peach-melba",
		},
		{
			name: "discovery returns incomplete protos - nil layout",
			plan: endorsementPlan{
				"g1": {{endorser: localhostMock, height: 2}},
			},
			postSetup: func(t *testing.T, def *preparedTest) {
				ed := &dp.EndorsementDescriptor{
					Chaincode: "my_channel",
					Layouts:   []*dp.Layout{nil},
				}
				def.discovery.PeersForEndorsementReturns(ed, nil)
			},
			errCode:   codes.FailedPrecondition,
			errString: "failed to select a set of endorsers that satisfy the endorsement policy",
		},
		{
			name: "discovery returns incomplete protos - nil state info",
			plan: endorsementPlan{
				"g1": {{endorser: localhostMock, height: 2}},
			},
			postSetup: func(t *testing.T, def *preparedTest) {
				ed := &dp.EndorsementDescriptor{
					Chaincode:         "my_channel",
					Layouts:           []*dp.Layout{{QuantitiesByGroup: map[string]uint32{"g1": 1}}},
					EndorsersByGroups: map[string]*dp.Peers{"g1": {Peers: []*dp.Peer{{StateInfo: nil}}}},
				}
				def.discovery.PeersForEndorsementReturns(ed, nil)
			},
			errCode:   codes.FailedPrecondition,
			errString: "failed to select a set of endorsers that satisfy the endorsement policy",
		},
		{
			name: "process proposal fails",
			plan: endorsementPlan{
				"g1": {{endorser: localhostMock, height: 1}},
			},
			endpointDefinition: &endpointDef{
				proposalError: status.Error(codes.Aborted, "wibble"),
			},
			errCode:   codes.Aborted,
			errString: "failed to endorse transaction, see attached details for more info",
			errDetails: []*pb.ErrorDetail{
				{
					Address: "localhost:7051",
					MspId:   "msp1",
					Message: "rpc error: code = Aborted desc = wibble",
				},
				{
					Address: "peer1:8051",
					MspId:   "msp1",
					Message: "rpc error: code = Aborted desc = wibble",
				},
			},
		},
		{
			name: "local endorser succeeds, remote endorser fails",
			plan: endorsementPlan{
				"g1": {{endorser: localhostMock, height: 1}},
				"g2": {{endorser: peer4Mock, height: 1}},
			},
			endpointDefinition: &endpointDef{
				proposalError: status.Error(codes.Aborted, "remote-wobble"),
			},
			postSetup: func(t *testing.T, def *preparedTest) {
				def.localEndorser.ProcessProposalReturns(createProposalResponse(t, localhostMock.address, "all_good", 200, ""), nil)
			},
			errCode:   codes.Aborted,
			errString: "failed to collect enough transaction endorsements, see attached details for more info",
			errDetails: []*pb.ErrorDetail{{
				Address: "peer4:11051",
				MspId:   "msp3",
				Message: "rpc error: code = Aborted desc = remote-wobble",
			}},
		},
		{
			name: "process proposal chaincode error",
			plan: endorsementPlan{
				"g1": {{endorser: localhostMock, height: 2}},
			},
			endpointDefinition: &endpointDef{
				proposalResponseStatus:  400,
				proposalResponseMessage: "Mock chaincode error",
			},
			errCode:   codes.Aborted,
			errString: "failed to endorse transaction, see attached details for more info",
			errDetails: []*pb.ErrorDetail{
				{
					Address: "localhost:7051",
					MspId:   "msp1",
					Message: "chaincode response 400, Mock chaincode error",
				},
				{
					Address: "peer1:8051",
					MspId:   "msp1",
					Message: "chaincode response 400, Mock chaincode error",
				},
			},
		},
		{
			name: "local endorser succeeds, remote endorser chaincode error",
			plan: endorsementPlan{
				"g1": {{endorser: localhostMock, height: 1}},
				"g2": {{endorser: peer4Mock, height: 1}},
			},
			endpointDefinition: &endpointDef{
				proposalResponseStatus:  400,
				proposalResponseMessage: "Mock chaincode error",
			},
			postSetup: func(t *testing.T, def *preparedTest) {
				def.localEndorser.ProcessProposalReturns(createProposalResponse(t, localhostMock.address, "all_good", 200, ""), nil)
			},
			errCode:   codes.Aborted,
			errString: "failed to collect enough transaction endorsements, see attached details for more info",
			errDetails: []*pb.ErrorDetail{{
				Address: "peer4:11051",
				MspId:   "msp3",
				Message: "chaincode response 400, Mock chaincode error",
			}},
		},
		{
			name: "first endorser returns chaincode interest",
			plan: endorsementPlan{
				"g1": {{endorser: localhostMock, height: 3}},
				"g2": {{endorser: peer2Mock, height: 3}},
			},
			interest: &peer.ChaincodeInterest{
				Chaincodes: []*peer.ChaincodeCall{{
					Name:            testChaincode,
					CollectionNames: []string{"mycollection1", "mycollection2"},
					NoPrivateReads:  true,
				}},
			},
			expectedEndorsers: []string{"localhost:7051", "peer2:9051"},
		},
		{
			name: "context timeout during first endorsement",
			plan: endorsementPlan{
				"g1": {{endorser: localhostMock, height: 3}}, // msp1
				"g2": {{endorser: peer4Mock, height: 5}},     // msp3
			},
			postSetup: func(t *testing.T, def *preparedTest) {
				def.ctx, def.cancel = context.WithTimeout(def.ctx, 100*time.Millisecond)

				def.localEndorser.ProcessProposalStub = func(ctx context.Context, proposal *peer.SignedProposal, option ...grpc.CallOption) (*peer.ProposalResponse, error) {
					time.Sleep(200 * time.Millisecond)
					return createProposalResponse(t, peer1Mock.address, "mock_response", 200, ""), nil
				}
			},
			postTest: func(t *testing.T, def *preparedTest) {
				def.cancel()
			},
			errCode:   codes.DeadlineExceeded,
			errString: "endorsement timeout expired while collecting first endorsement",
		},
		{
			name: "context timeout collecting endorsements",
			plan: endorsementPlan{
				"g1": {{endorser: localhostMock, height: 3}}, // msp1
				"g2": {{endorser: peer4Mock, height: 5}},     // msp3
			},
			postSetup: func(t *testing.T, def *preparedTest) {
				def.ctx, def.cancel = context.WithTimeout(def.ctx, 100*time.Millisecond)

				peer4Mock.client.(*mocks.EndorserClient).ProcessProposalStub = func(ctx context.Context, proposal *peer.SignedProposal, option ...grpc.CallOption) (*peer.ProposalResponse, error) {
					time.Sleep(200 * time.Millisecond)
					return createProposalResponse(t, peer4Mock.address, "mock_response", 200, ""), nil
				}
			},
			postTest: func(t *testing.T, def *preparedTest) {
				def.cancel()
			},
			errCode:   codes.DeadlineExceeded,
			errString: "endorsement timeout expired while collecting endorsements",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			test := prepareTest(t, &tt)

			response, err := test.server.Endorse(test.ctx, &pb.EndorseRequest{ProposedTransaction: test.signedProposal, EndorsingOrganizations: tt.endorsingOrgs})

			if checkError(t, &tt, err) {
				require.Nil(t, response, "response on error")
				if tt.postTest != nil {
					tt.postTest(t, test)
				}
				return
			}

			// test the assertions
			require.NoError(t, err)

			// assert the preparedTxn is the payload from the proposal response
			chaincodeAction, err := protoutil.GetActionFromEnvelopeMsg(response.GetPreparedTransaction())
			require.NoError(t, err)
			require.Equal(t, []byte("mock_response"), chaincodeAction.GetResponse().GetPayload(), "Incorrect response")

			// check the generated transaction envelope contains the correct endorsements
			checkTransaction(t, tt.expectedEndorsers, response.GetPreparedTransaction())

			// check the correct endorsers (mocks) were called with the right parameters
			checkEndorsers(t, tt.expectedEndorsers, test)

			if tt.postTest != nil {
				tt.postTest(t, test)
			}
		})
	}
}

func TestSubmit(t *testing.T) {
	tests := []testDef{
		{
			name: "two endorsers",
			plan: endorsementPlan{
				"g1": {{endorser: localhostMock, height: 3}},
				"g2": {{endorser: peer1Mock, height: 3}},
			},
		},
		{
			name: "discovery fails",
			plan: endorsementPlan{
				"g1": {{endorser: localhostMock}},
			},
			postSetup: func(t *testing.T, def *preparedTest) {
				def.discovery.ConfigReturnsOnCall(1, nil, fmt.Errorf("jabberwocky"))
			},
			errCode:   codes.FailedPrecondition,
			errString: "failed to get config for channel [test_channel]: jabberwocky",
		},
		{
			name: "no orderers",
			plan: endorsementPlan{
				"g1": {{endorser: localhostMock}},
			},
			postSetup: func(t *testing.T, def *preparedTest) {
				def.discovery.ConfigReturns(&dp.ConfigResult{
					Orderers: map[string]*dp.Endpoints{},
					Msps:     map[string]*msp.FabricMSPConfig{},
				}, nil)
			},
			errCode:   codes.Unavailable,
			errString: "no orderer nodes available",
		},
		{
			name: "orderer broadcast fails",
			plan: endorsementPlan{
				"g1": {{endorser: localhostMock}},
			},
			endpointDefinition: &endpointDef{
				proposalResponseStatus: 200,
				ordererBroadcastError:  status.Error(codes.FailedPrecondition, "Orderer not listening!"),
			},
			errCode:   codes.Unavailable,
			errString: "no orderers could successfully process transaction",
			errDetails: []*pb.ErrorDetail{{
				Address: "orderer:7050",
				MspId:   "msp1",
				Message: "rpc error: code = FailedPrecondition desc = Orderer not listening!",
			}},
		},
		{
			name: "send to orderer fails",
			plan: endorsementPlan{
				"g1": {{endorser: localhostMock}},
			},
			endpointDefinition: &endpointDef{
				proposalResponseStatus: 200,
				ordererSendError:       status.Error(codes.Internal, "Orderer says no!"),
			},
			errCode:   codes.Unavailable,
			errString: "no orderers could successfully process transaction",
			errDetails: []*pb.ErrorDetail{{
				Address: "orderer:7050",
				MspId:   "msp1",
				Message: "rpc error: code = Internal desc = Orderer says no!",
			}},
		},
		{
			name: "receive from orderer fails",
			plan: endorsementPlan{
				"g1": {{endorser: localhostMock}},
			},
			endpointDefinition: &endpointDef{
				proposalResponseStatus: 200,
				ordererRecvError:       status.Error(codes.FailedPrecondition, "Orderer not happy!"),
			},
			errCode:   codes.Unavailable,
			errString: "no orderers could successfully process transaction",
			errDetails: []*pb.ErrorDetail{{
				Address: "orderer:7050",
				MspId:   "msp1",
				Message: "rpc error: code = FailedPrecondition desc = Orderer not happy!",
			}},
		},
		{
			name: "orderer Recv() returns nil",
			plan: endorsementPlan{
				"g1": {{endorser: localhostMock}},
			},
			postSetup: func(t *testing.T, def *preparedTest) {
				def.server.registry.endpointFactory.connectOrderer = func(_ *grpc.ClientConn) ab.AtomicBroadcastClient {
					abc := &mocks.ABClient{}
					abbc := &mocks.ABBClient{}
					abbc.RecvReturns(nil, nil)
					abc.BroadcastReturns(abbc, nil)
					return abc
				}
			},
			errCode:   codes.Unavailable,
			errString: "no orderers could successfully process transaction",
		},
		{
			name: "orderer returns unsuccessful response",
			plan: endorsementPlan{
				"g1": {{endorser: localhostMock}},
			},
			postSetup: func(t *testing.T, def *preparedTest) {
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
			errCode:   codes.Aborted,
			errString: "received unsuccessful response from orderer: " + cp.Status_name[int32(cp.Status_BAD_REQUEST)],
		},
		{
			name: "dialing orderer endpoint fails",
			plan: endorsementPlan{
				"g1": {{endorser: localhostMock}},
			},
			postSetup: func(t *testing.T, def *preparedTest) {
				def.dialer.Calls(func(_ context.Context, target string, _ ...grpc.DialOption) (*grpc.ClientConn, error) {
					if target == "orderer:7050" {
						return nil, fmt.Errorf("orderer not answering")
					}
					return nil, nil
				})
			},
			errCode:   codes.Unavailable,
			errString: "no orderer nodes available",
		},
		{
			name: "orderer retry",
			plan: endorsementPlan{
				"g1": {{endorser: localhostMock}},
			},
			config: &dp.ConfigResult{
				Orderers: map[string]*dp.Endpoints{
					"msp1": {
						Endpoint: []*dp.Endpoint{
							{Host: "orderer1", Port: 7050},
							{Host: "orderer2", Port: 7050},
							{Host: "orderer3", Port: 7050},
						},
					},
				},
				Msps: map[string]*msp.FabricMSPConfig{
					"msp1": {
						TlsRootCerts: [][]byte{},
					},
				},
			},
			postSetup: func(t *testing.T, def *preparedTest) {
				abc := &mocks.ABClient{}
				abbc := &mocks.ABBClient{}
				abbc.SendReturnsOnCall(0, status.Error(codes.Unavailable, "First orderer error"))
				abbc.SendReturnsOnCall(1, status.Error(codes.Unavailable, "Second orderer error"))
				abbc.SendReturnsOnCall(2, nil) // third time lucky
				abbc.RecvReturns(&ab.BroadcastResponse{
					Info:   "success",
					Status: cp.Status(200),
				}, nil)
				abc.BroadcastReturns(abbc, nil)
				def.server.registry.endpointFactory = &endpointFactory{
					timeout: 5 * time.Second,
					connectEndorser: func(conn *grpc.ClientConn) peer.EndorserClient {
						return &mocks.EndorserClient{}
					},
					connectOrderer: func(_ *grpc.ClientConn) ab.AtomicBroadcastClient {
						return abc
					},
					dialer: func(ctx context.Context, target string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
						return nil, nil
					},
				}
			},
		},
		{
			name: "orderer bad response retry",
			plan: endorsementPlan{
				"g1": {{endorser: localhostMock}},
			},
			config: &dp.ConfigResult{
				Orderers: map[string]*dp.Endpoints{
					"msp1": {
						Endpoint: []*dp.Endpoint{
							{Host: "orderer1", Port: 7050},
							{Host: "orderer2", Port: 7050},
							{Host: "orderer3", Port: 7050},
						},
					},
				},
				Msps: map[string]*msp.FabricMSPConfig{
					"msp1": {
						TlsRootCerts: [][]byte{},
					},
				},
			},
			postSetup: func(t *testing.T, def *preparedTest) {
				abc := &mocks.ABClient{}
				abbc := &mocks.ABBClient{}
				abbc.SendReturns(nil)
				abbc.RecvReturnsOnCall(0, &ab.BroadcastResponse{
					Info:   "internal error",
					Status: cp.Status(500),
				}, nil)
				abbc.RecvReturnsOnCall(1, &ab.BroadcastResponse{
					Info:   "success",
					Status: cp.Status(200),
				}, nil)
				abc.BroadcastReturns(abbc, nil)
				def.server.registry.endpointFactory = &endpointFactory{
					timeout: 5 * time.Second,
					connectEndorser: func(conn *grpc.ClientConn) peer.EndorserClient {
						return &mocks.EndorserClient{}
					},
					connectOrderer: func(_ *grpc.ClientConn) ab.AtomicBroadcastClient {
						return abc
					},
					dialer: func(ctx context.Context, target string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
						return nil, nil
					},
				}
			},
		},
		{
			name: "orderer timeout - retry",
			plan: endorsementPlan{
				"g1": {{endorser: localhostMock}},
			},
			config: &dp.ConfigResult{
				Orderers: map[string]*dp.Endpoints{
					"msp1": {
						Endpoint: []*dp.Endpoint{
							{Host: "orderer1", Port: 7050},
							{Host: "orderer2", Port: 7050},
							{Host: "orderer3", Port: 7050},
						},
					},
				},
			},
			postSetup: func(t *testing.T, def *preparedTest) {
				def.ctx, def.cancel = context.WithTimeout(def.ctx, 300*time.Millisecond)
				broadcastTime := 200 * time.Millisecond // first invocation exceeds BroadcastTimeout

				abc := &mocks.ABClient{}
				abbc := &mocks.ABBClient{}
				abbc.SendReturns(nil)
				abbc.RecvReturns(&ab.BroadcastResponse{
					Info:   "success",
					Status: cp.Status(200),
				}, nil)
				abc.BroadcastStub = func(ctx context.Context, co ...grpc.CallOption) (ab.AtomicBroadcast_BroadcastClient, error) {
					defer func() {
						broadcastTime = time.Millisecond // subsequent invocations will not timeout
					}()
					select {
					case <-time.After(broadcastTime):
						return abbc, nil
					case <-ctx.Done():
						return nil, ctx.Err()
					}
				}
				def.server.registry.endpointFactory = &endpointFactory{
					timeout: 5 * time.Second,
					connectEndorser: func(conn *grpc.ClientConn) peer.EndorserClient {
						return &mocks.EndorserClient{}
					},
					connectOrderer: func(_ *grpc.ClientConn) ab.AtomicBroadcastClient {
						return abc
					},
					dialer: func(ctx context.Context, target string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
						return nil, nil
					},
				}
			},
			postTest: func(t *testing.T, def *preparedTest) {
				def.cancel()
			},
		},
		{
			name: "submit timeout",
			plan: endorsementPlan{
				"g1": {{endorser: localhostMock}},
			},
			config: &dp.ConfigResult{
				Orderers: map[string]*dp.Endpoints{
					"msp1": {
						Endpoint: []*dp.Endpoint{
							{Host: "orderer1", Port: 7050},
							{Host: "orderer2", Port: 7050},
							{Host: "orderer3", Port: 7050},
						},
					},
				},
			},
			postSetup: func(t *testing.T, def *preparedTest) {
				def.ctx, def.cancel = context.WithTimeout(def.ctx, 50*time.Millisecond)
				broadcastTime := 200 * time.Millisecond // invocation exceeds BroadcastTimeout

				abc := &mocks.ABClient{}
				abbc := &mocks.ABBClient{}
				abbc.SendReturns(nil)
				abbc.RecvReturns(&ab.BroadcastResponse{
					Info:   "success",
					Status: cp.Status(200),
				}, nil)
				abc.BroadcastStub = func(ctx context.Context, co ...grpc.CallOption) (ab.AtomicBroadcast_BroadcastClient, error) {
					select {
					case <-time.After(broadcastTime):
						return abbc, nil
					case <-ctx.Done():
						return nil, ctx.Err()
					}
				}
				def.server.registry.endpointFactory = &endpointFactory{
					timeout: 5 * time.Second,
					connectEndorser: func(conn *grpc.ClientConn) peer.EndorserClient {
						return &mocks.EndorserClient{}
					},
					connectOrderer: func(_ *grpc.ClientConn) ab.AtomicBroadcastClient {
						return abc
					},
					dialer: func(ctx context.Context, target string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
						return nil, nil
					},
				}
			},
			postTest: func(t *testing.T, def *preparedTest) {
				def.cancel()
			},
			errCode:   codes.DeadlineExceeded,
			errString: "submit timeout expired while broadcasting to ordering service",
		},
		{
			name: "multiple orderers all fail",
			plan: endorsementPlan{
				"g1": {{endorser: localhostMock}},
			},
			config: &dp.ConfigResult{
				Orderers: map[string]*dp.Endpoints{
					"msp1": {
						Endpoint: []*dp.Endpoint{
							{Host: "orderer1", Port: 7050},
							{Host: "orderer2", Port: 7050},
							{Host: "orderer3", Port: 7050},
						},
					},
				},
				Msps: map[string]*msp.FabricMSPConfig{
					"msp1": {
						TlsRootCerts: [][]byte{},
					},
				},
			},
			endpointDefinition: &endpointDef{
				proposalResponseStatus: 200,
				ordererBroadcastError:  status.Error(codes.Unavailable, "Orderer not listening!"),
			},
			errCode:   codes.Unavailable,
			errString: "no orderers could successfully process transaction",
			errDetails: []*pb.ErrorDetail{
				{
					Address: "orderer1:7050",
					MspId:   "msp1",
					Message: "rpc error: code = Unavailable desc = Orderer not listening!",
				},
				{
					Address: "orderer2:7050",
					MspId:   "msp1",
					Message: "rpc error: code = Unavailable desc = Orderer not listening!",
				},
				{
					Address: "orderer3:7050",
					MspId:   "msp1",
					Message: "rpc error: code = Unavailable desc = Orderer not listening!",
				},
			},
		},
		{
			name: "orderer endpoint overrides",
			plan: endorsementPlan{
				"g1": {{endorser: localhostMock}},
			},
			ordererEndpointOverrides: map[string]*orderers.Endpoint{
				"orderer1:7050": {Address: "override1:1234"},
				"orderer3:7050": {Address: "override3:4321"},
			},
			config: &dp.ConfigResult{
				Orderers: map[string]*dp.Endpoints{
					"msp1": {
						Endpoint: []*dp.Endpoint{
							{Host: "orderer1", Port: 7050},
							{Host: "orderer2", Port: 7050},
							{Host: "orderer3", Port: 7050},
						},
					},
				},
				Msps: map[string]*msp.FabricMSPConfig{
					"msp1": {
						TlsRootCerts: [][]byte{},
					},
				},
			},
			endpointDefinition: &endpointDef{
				proposalResponseStatus: 200,
				ordererBroadcastError:  status.Error(codes.Unavailable, "Orderer not listening!"),
			},
			errCode:   codes.Unavailable,
			errString: "no orderers could successfully process transaction",
			errDetails: []*pb.ErrorDetail{
				{
					Address: "override1:1234 (mapped from orderer1:7050)",
					MspId:   "msp1",
					Message: "rpc error: code = Unavailable desc = Orderer not listening!",
				},
				{
					Address: "orderer2:7050",
					MspId:   "msp1",
					Message: "rpc error: code = Unavailable desc = Orderer not listening!",
				},
				{
					Address: "override3:4321 (mapped from orderer3:7050)",
					MspId:   "msp1",
					Message: "rpc error: code = Unavailable desc = Orderer not listening!",
				},
			},
			postTest: func(t *testing.T, def *preparedTest) {
				var addresses []string
				for i := 0; i < def.dialer.CallCount(); i++ {
					_, address, _ := def.dialer.ArgsForCall(i)
					addresses = append(addresses, address)
				}
				require.Contains(t, addresses, "override1:1234")
				require.NotContains(t, addresses, "orderer1:7050")
				require.Contains(t, addresses, "orderer2:7050")
				require.Contains(t, addresses, "override3:4321")
				require.NotContains(t, addresses, "orderer3:7050")
			},
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
			submitResponse, err := test.server.Submit(test.ctx, &pb.SubmitRequest{PreparedTransaction: preparedTx, ChannelId: testChannel})

			if checkError(t, &tt, err) {
				require.Nil(t, submitResponse, "response on error")
				if tt.postTest != nil {
					tt.postTest(t, test)
				}
				return
			}

			require.NoError(t, err)
			require.True(t, proto.Equal(&pb.SubmitResponse{}, submitResponse), "Incorrect response")

			if tt.postTest != nil {
				tt.postTest(t, test)
			}
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
			errCode:   codes.Aborted,
			errString: "FINDER_ERROR",
		},
		{
			name: "returns transaction status",
			finderStatus: &commit.Status{
				Code:        peer.TxValidationCode_MVCC_READ_CONFLICT,
				BlockNumber: 101,
			},
			expectedResponse: &pb.CommitStatusResponse{
				Result:      peer.TxValidationCode_MVCC_READ_CONFLICT,
				BlockNumber: 101,
			},
		},
		{
			name: "passes channel name to finder",
			postSetup: func(t *testing.T, test *preparedTest) {
				test.finder.TransactionStatusCalls(func(ctx context.Context, channelName string, transactionID string) (*commit.Status, error) {
					require.Equal(t, testChannel, channelName)
					status := &commit.Status{
						Code:        peer.TxValidationCode_MVCC_READ_CONFLICT,
						BlockNumber: 101,
					}
					return status, nil
				})
			},
		},
		{
			name: "passes transaction ID to finder",
			postSetup: func(t *testing.T, test *preparedTest) {
				test.finder.TransactionStatusCalls(func(ctx context.Context, channelName string, transactionID string) (*commit.Status, error) {
					require.Equal(t, "TX_ID", transactionID)
					status := &commit.Status{
						Code:        peer.TxValidationCode_MVCC_READ_CONFLICT,
						BlockNumber: 101,
					}
					return status, nil
				})
			},
		},
		{
			name:      "failed policy or signature check",
			policyErr: errors.New("POLICY_ERROR"),
			errCode:   codes.PermissionDenied,
			errString: "POLICY_ERROR",
		},
		{
			name: "passes channel name to policy checker",
			postSetup: func(t *testing.T, test *preparedTest) {
				test.policy.CheckACLCalls(func(policyName string, channelName string, data interface{}) error {
					require.Equal(t, testChannel, channelName)
					return nil
				})
			},
			finderStatus: &commit.Status{
				Code:        peer.TxValidationCode_MVCC_READ_CONFLICT,
				BlockNumber: 101,
			},
		},
		{
			name:     "passes identity to policy checker",
			identity: []byte("IDENTITY"),
			postSetup: func(t *testing.T, test *preparedTest) {
				test.policy.CheckACLCalls(func(policyName string, channelName string, data interface{}) error {
					require.IsType(t, &protoutil.SignedData{}, data)
					signedData := data.(*protoutil.SignedData)
					require.Equal(t, []byte("IDENTITY"), signedData.Identity)
					return nil
				})
			},
			finderStatus: &commit.Status{
				Code:        peer.TxValidationCode_MVCC_READ_CONFLICT,
				BlockNumber: 101,
			},
		},
		{
			name:      "context timeout",
			finderErr: context.DeadlineExceeded,
			errCode:   codes.DeadlineExceeded,
			errString: "context deadline exceeded",
		},
		{
			name:      "context canceled",
			finderErr: context.Canceled,
			errCode:   codes.Canceled,
			errString: "context canceled",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			test := prepareTest(t, &tt)

			request := &pb.CommitStatusRequest{
				ChannelId:     testChannel,
				Identity:      tt.identity,
				TransactionId: "TX_ID",
			}
			requestBytes, err := proto.Marshal(request)
			require.NoError(t, err)

			signedRequest := &pb.SignedCommitStatusRequest{
				Request:   requestBytes,
				Signature: []byte{},
			}

			response, err := test.server.CommitStatus(test.ctx, signedRequest)

			if checkError(t, &tt, err) {
				require.Nil(t, response, "response on error")
				return
			}

			require.NoError(t, err)
			if tt.expectedResponse != nil {
				require.True(t, proto.Equal(tt.expectedResponse, response), "incorrect response", response)
			}
		})
	}
}

func TestChaincodeEvents(t *testing.T) {
	now := time.Now()
	lastTransactionID := "LAST_TX_ID"

	newChaincodeEvent := func(chaincodeName string, transactionID string) *peer.ChaincodeEvent {
		return &peer.ChaincodeEvent{
			ChaincodeId: chaincodeName,
			TxId:        transactionID,
			EventName:   "EVENT_NAME",
			Payload:     []byte("PAYLOAD"),
		}
	}

	newTransactionHeader := func(transactionID string) *cp.Header {
		return &cp.Header{
			ChannelHeader: protoutil.MarshalOrPanic(&cp.ChannelHeader{
				Type: int32(cp.HeaderType_ENDORSER_TRANSACTION),
				Timestamp: &timestamp.Timestamp{
					Seconds: now.Unix(),
					Nanos:   int32(now.Nanosecond()),
				},
				TxId: transactionID,
			}),
		}
	}

	newTransactionEnvelope := func(event *peer.ChaincodeEvent) *cp.Envelope {
		return &cp.Envelope{
			Payload: protoutil.MarshalOrPanic(&cp.Payload{
				Header: newTransactionHeader(event.GetTxId()),
				Data: protoutil.MarshalOrPanic(&peer.Transaction{
					Actions: []*peer.TransactionAction{
						{
							Payload: protoutil.MarshalOrPanic(&peer.ChaincodeActionPayload{
								Action: &peer.ChaincodeEndorsedAction{
									ProposalResponsePayload: protoutil.MarshalOrPanic(&peer.ProposalResponsePayload{
										Extension: protoutil.MarshalOrPanic(&peer.ChaincodeAction{
											Events: protoutil.MarshalOrPanic(event),
										}),
									}),
								},
							}),
						},
					},
				}),
			}),
		}
	}

	newBlock := func(number uint64) *cp.Block {
		return &cp.Block{
			Header: &cp.BlockHeader{
				Number: number,
			},
			Metadata: &cp.BlockMetadata{
				Metadata: make([][]byte, 5),
			},
			Data: &cp.BlockData{
				Data: [][]byte{},
			},
		}
	}

	addTransaction := func(block *cp.Block, transaction *cp.Envelope, status peer.TxValidationCode) {
		metadata := block.GetMetadata().GetMetadata()
		metadata[cp.BlockMetadataIndex_TRANSACTIONS_FILTER] = append(metadata[cp.BlockMetadataIndex_TRANSACTIONS_FILTER], byte(status))

		blockData := block.GetData()
		blockData.Data = append(blockData.Data, protoutil.MarshalOrPanic(transaction))
	}

	matchEvent := newChaincodeEvent(testChaincode, "EXPECTED_TX_ID")
	wrongChaincodeEvent := newChaincodeEvent("WRONG_CHAINCODE", "WRONG__TX_ID")
	oldTransactionEvent := newChaincodeEvent(testChaincode, "OLD_TX_ID")
	lastTransactionEvent := newChaincodeEvent(testChaincode, lastTransactionID)
	lastTransactionWrongChaincodeEvent := newChaincodeEvent("WRONG_CHAINCODE", lastTransactionID)

	configTxEnvelope := &cp.Envelope{
		Payload: protoutil.MarshalOrPanic(&cp.Payload{
			Header: &cp.Header{
				ChannelHeader: protoutil.MarshalOrPanic(&cp.ChannelHeader{
					Type: int32(cp.HeaderType_CONFIG_UPDATE),
				}),
			},
		}),
	}

	noMatchingEventsBlock := newBlock(100)
	addTransaction(noMatchingEventsBlock, newTransactionEnvelope(wrongChaincodeEvent), peer.TxValidationCode_VALID)

	matchingEventBlock := newBlock(101)
	addTransaction(matchingEventBlock, configTxEnvelope, peer.TxValidationCode_VALID)
	addTransaction(matchingEventBlock, newTransactionEnvelope(wrongChaincodeEvent), peer.TxValidationCode_VALID)
	addTransaction(matchingEventBlock, newTransactionEnvelope(matchEvent), peer.TxValidationCode_VALID)

	partReadBlock := newBlock(200)
	addTransaction(partReadBlock, newTransactionEnvelope(oldTransactionEvent), peer.TxValidationCode_VALID)
	addTransaction(partReadBlock, newTransactionEnvelope(lastTransactionEvent), peer.TxValidationCode_VALID)
	addTransaction(partReadBlock, newTransactionEnvelope(matchEvent), peer.TxValidationCode_VALID)

	differentChaincodePartReadBlock := newBlock(300)
	addTransaction(differentChaincodePartReadBlock, newTransactionEnvelope(oldTransactionEvent), peer.TxValidationCode_VALID)
	addTransaction(differentChaincodePartReadBlock, newTransactionEnvelope(lastTransactionWrongChaincodeEvent), peer.TxValidationCode_VALID)
	addTransaction(differentChaincodePartReadBlock, newTransactionEnvelope(matchEvent), peer.TxValidationCode_VALID)

	tests := []testDef{
		{
			name:      "error reading events",
			eventErr:  errors.New("EVENT_ERROR"),
			errCode:   codes.Aborted,
			errString: "EVENT_ERROR",
		},
		{
			name: "returns chaincode events",
			blocks: []*cp.Block{
				matchingEventBlock,
			},
			expectedResponses: []proto.Message{
				&pb.ChaincodeEventsResponse{
					BlockNumber: matchingEventBlock.GetHeader().GetNumber(),
					Events: []*peer.ChaincodeEvent{
						{
							ChaincodeId: testChaincode,
							TxId:        matchEvent.GetTxId(),
							EventName:   matchEvent.GetEventName(),
							Payload:     matchEvent.GetPayload(),
						},
					},
				},
			},
		},
		{
			name: "skips blocks containing only non-matching chaincode events",
			blocks: []*cp.Block{
				noMatchingEventsBlock,
				matchingEventBlock,
			},
			expectedResponses: []proto.Message{
				&pb.ChaincodeEventsResponse{
					BlockNumber: matchingEventBlock.GetHeader().GetNumber(),
					Events: []*peer.ChaincodeEvent{
						{
							ChaincodeId: testChaincode,
							TxId:        matchEvent.GetTxId(),
							EventName:   matchEvent.GetEventName(),
							Payload:     matchEvent.GetPayload(),
						},
					},
				},
			},
		},
		{
			name: "skips previously seen transactions",
			blocks: []*cp.Block{
				partReadBlock,
			},
			afterTxID: lastTransactionID,
			expectedResponses: []proto.Message{
				&pb.ChaincodeEventsResponse{
					BlockNumber: partReadBlock.GetHeader().GetNumber(),
					Events: []*peer.ChaincodeEvent{
						{
							ChaincodeId: testChaincode,
							TxId:        matchEvent.GetTxId(),
							EventName:   matchEvent.GetEventName(),
							Payload:     matchEvent.GetPayload(),
						},
					},
				},
			},
		},
		{
			name: "identifies specified transaction if from different chaincode",
			blocks: []*cp.Block{
				differentChaincodePartReadBlock,
			},
			afterTxID: lastTransactionID,
			expectedResponses: []proto.Message{
				&pb.ChaincodeEventsResponse{
					BlockNumber: differentChaincodePartReadBlock.GetHeader().GetNumber(),
					Events: []*peer.ChaincodeEvent{
						{
							ChaincodeId: testChaincode,
							TxId:        matchEvent.GetTxId(),
							EventName:   matchEvent.GetEventName(),
							Payload:     matchEvent.GetPayload(),
						},
					},
				},
			},
		},
		{
			name: "identifies specified transaction if not in first read block",
			blocks: []*cp.Block{
				noMatchingEventsBlock,
				partReadBlock,
			},
			afterTxID: lastTransactionID,
			expectedResponses: []proto.Message{
				&pb.ChaincodeEventsResponse{
					BlockNumber: partReadBlock.GetHeader().GetNumber(),
					Events: []*peer.ChaincodeEvent{
						{
							ChaincodeId: testChaincode,
							TxId:        matchEvent.GetTxId(),
							EventName:   matchEvent.GetEventName(),
							Payload:     matchEvent.GetPayload(),
						},
					},
				},
			},
		},
		{
			name: "passes channel name to ledger provider",
			postTest: func(t *testing.T, test *preparedTest) {
				require.Equal(t, 1, test.ledgerProvider.LedgerCallCount())
				require.Equal(t, testChannel, test.ledgerProvider.LedgerArgsForCall(0))
			},
		},
		{
			name: "returns error obtaining ledger",
			blocks: []*cp.Block{
				matchingEventBlock,
			},
			errCode:   codes.NotFound,
			errString: "LEDGER_PROVIDER_ERROR",
			postSetup: func(t *testing.T, test *preparedTest) {
				test.ledgerProvider.LedgerReturns(nil, errors.New("LEDGER_PROVIDER_ERROR"))
			},
		},
		{
			name: "returns error obtaining ledger height",
			blocks: []*cp.Block{
				matchingEventBlock,
			},
			errCode:   codes.Aborted,
			errString: "LEDGER_INFO_ERROR",
			postSetup: func(t *testing.T, test *preparedTest) {
				test.ledger.GetBlockchainInfoReturns(nil, errors.New("LEDGER_INFO_ERROR"))
			},
		},
		{
			name: "uses block height as start block if next commit is specified as start position",
			blocks: []*cp.Block{
				matchingEventBlock,
			},
			postSetup: func(t *testing.T, test *preparedTest) {
				ledgerInfo := &cp.BlockchainInfo{
					Height: 101,
				}
				test.ledger.GetBlockchainInfoReturns(ledgerInfo, nil)
			},
			startPosition: &ab.SeekPosition{
				Type: &ab.SeekPosition_NextCommit{
					NextCommit: &ab.SeekNextCommit{},
				},
			},
			postTest: func(t *testing.T, test *preparedTest) {
				require.Equal(t, 1, test.ledger.GetBlocksIteratorCallCount())
				require.EqualValues(t, 101, test.ledger.GetBlocksIteratorArgsForCall(0))
			},
		},
		{
			name: "uses specified start block",
			blocks: []*cp.Block{
				matchingEventBlock,
			},
			postSetup: func(t *testing.T, test *preparedTest) {
				ledgerInfo := &cp.BlockchainInfo{
					Height: 101,
				}
				test.ledger.GetBlockchainInfoReturns(ledgerInfo, nil)
			},
			startPosition: &ab.SeekPosition{
				Type: &ab.SeekPosition_Specified{
					Specified: &ab.SeekSpecified{
						Number: 99,
					},
				},
			},
			postTest: func(t *testing.T, test *preparedTest) {
				require.Equal(t, 1, test.ledger.GetBlocksIteratorCallCount())
				require.EqualValues(t, 99, test.ledger.GetBlocksIteratorArgsForCall(0))
			},
		},
		{
			name: "defaults to next commit if start position not specified",
			blocks: []*cp.Block{
				matchingEventBlock,
			},
			postSetup: func(t *testing.T, test *preparedTest) {
				ledgerInfo := &cp.BlockchainInfo{
					Height: 101,
				}
				test.ledger.GetBlockchainInfoReturns(ledgerInfo, nil)
			},
			postTest: func(t *testing.T, test *preparedTest) {
				require.Equal(t, 1, test.ledger.GetBlocksIteratorCallCount())
				require.EqualValues(t, 101, test.ledger.GetBlocksIteratorArgsForCall(0))
			},
		},
		{
			name: "uses block containing specified transaction instead of start block",
			blocks: []*cp.Block{
				matchingEventBlock,
			},
			postSetup: func(t *testing.T, test *preparedTest) {
				ledgerInfo := &cp.BlockchainInfo{
					Height: 101,
				}
				test.ledger.GetBlockchainInfoReturns(ledgerInfo, nil)

				block := &cp.Block{
					Header: &cp.BlockHeader{
						Number: 99,
					},
				}
				test.ledger.GetBlockByTxIDReturns(block, nil)
			},
			afterTxID: "TX_ID",
			startPosition: &ab.SeekPosition{
				Type: &ab.SeekPosition_Specified{
					Specified: &ab.SeekSpecified{
						Number: 1,
					},
				},
			},
			postTest: func(t *testing.T, test *preparedTest) {
				require.Equal(t, 1, test.ledger.GetBlocksIteratorCallCount())
				require.EqualValues(t, 99, test.ledger.GetBlocksIteratorArgsForCall(0))
				require.Equal(t, 1, test.ledger.GetBlockByTxIDCallCount())
				require.Equal(t, "TX_ID", test.ledger.GetBlockByTxIDArgsForCall(0))
			},
		},
		{
			name: "uses start block if specified transaction not found",
			blocks: []*cp.Block{
				matchingEventBlock,
			},
			postSetup: func(t *testing.T, test *preparedTest) {
				ledgerInfo := &cp.BlockchainInfo{
					Height: 101,
				}
				test.ledger.GetBlockchainInfoReturns(ledgerInfo, nil)

				test.ledger.GetBlockByTxIDReturns(nil, errors.New("NOT_FOUND"))
			},
			afterTxID: "TX_ID",
			startPosition: &ab.SeekPosition{
				Type: &ab.SeekPosition_Specified{
					Specified: &ab.SeekSpecified{
						Number: 1,
					},
				},
			},
			postTest: func(t *testing.T, test *preparedTest) {
				require.Equal(t, 1, test.ledger.GetBlocksIteratorCallCount())
				require.EqualValues(t, 1, test.ledger.GetBlocksIteratorArgsForCall(0))
			},
		},
		{
			name: "returns error for unsupported start position type",
			blocks: []*cp.Block{
				matchingEventBlock,
			},
			startPosition: &ab.SeekPosition{
				Type: &ab.SeekPosition_Oldest{
					Oldest: &ab.SeekOldest{},
				},
			},
			errCode:   codes.InvalidArgument,
			errString: "invalid start position type: *orderer.SeekPosition_Oldest",
		},
		{
			name: "returns error obtaining ledger iterator",
			blocks: []*cp.Block{
				matchingEventBlock,
			},
			errCode:   codes.Aborted,
			errString: "LEDGER_ITERATOR_ERROR",
			postSetup: func(t *testing.T, test *preparedTest) {
				test.ledger.GetBlocksIteratorReturns(nil, errors.New("LEDGER_ITERATOR_ERROR"))
			},
		},
		{
			name: "returns canceled status error when client closes stream",
			blocks: []*cp.Block{
				matchingEventBlock,
			},
			errCode: codes.Canceled,
			postSetup: func(t *testing.T, test *preparedTest) {
				test.eventsServer.SendReturns(io.EOF)
			},
		},
		{
			name: "returns status error from send to client",
			blocks: []*cp.Block{
				matchingEventBlock,
			},
			errCode:   codes.Aborted,
			errString: "SEND_ERROR",
			postSetup: func(t *testing.T, test *preparedTest) {
				test.eventsServer.SendReturns(status.Error(codes.Aborted, "SEND_ERROR"))
			},
		},
		{
			name:      "failed policy or signature check",
			policyErr: errors.New("POLICY_ERROR"),
			errCode:   codes.PermissionDenied,
			errString: "POLICY_ERROR",
		},
		{
			name: "passes channel name to policy checker",
			postTest: func(t *testing.T, test *preparedTest) {
				require.Equal(t, 1, test.policy.CheckACLCallCount())
				_, channelName, _ := test.policy.CheckACLArgsForCall(0)
				require.Equal(t, testChannel, channelName)
			},
		},
		{
			name:     "passes identity to policy checker",
			identity: []byte("IDENTITY"),
			postTest: func(t *testing.T, test *preparedTest) {
				require.Equal(t, 1, test.policy.CheckACLCallCount())
				_, _, data := test.policy.CheckACLArgsForCall(0)
				require.IsType(t, &protoutil.SignedData{}, data)
				signedData := data.(*protoutil.SignedData)
				require.Equal(t, []byte("IDENTITY"), signedData.Identity)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			test := prepareTest(t, &tt)

			request := &pb.ChaincodeEventsRequest{
				ChannelId:   testChannel,
				Identity:    tt.identity,
				ChaincodeId: testChaincode,
			}
			if tt.startPosition != nil {
				request.StartPosition = tt.startPosition
			}
			if len(tt.afterTxID) > 0 {
				request.AfterTransactionId = tt.afterTxID
			}
			requestBytes, err := proto.Marshal(request)
			require.NoError(t, err)

			signedRequest := &pb.SignedChaincodeEventsRequest{
				Request:   requestBytes,
				Signature: []byte{},
			}

			err = test.server.ChaincodeEvents(signedRequest, test.eventsServer)

			if checkError(t, &tt, err) {
				return
			}

			for i, expectedResponse := range tt.expectedResponses {
				actualResponse := test.eventsServer.SendArgsForCall(i)
				require.True(t, proto.Equal(expectedResponse, actualResponse), "response[%d] mismatch: %v", i, actualResponse)
			}

			if tt.postTest != nil {
				tt.postTest(t, test)
			}
		})
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

func TestRpcErrorWithBadDetails(t *testing.T) {
	err := newRpcError(codes.InvalidArgument, "terrible error", nil)
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
		Height: 1,
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

	server := newServer(localEndorser, disc, mockFinder, mockPolicy, mockLedgerProvider, member, "msp1", &comm.SecureOptions{}, options, nil, tt.ordererEndpointOverrides)

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

func checkTransaction(t *testing.T, expectedEndorsers []string, transaction *cp.Envelope) {
	// check the prepared transaction contains the correct endorsements
	var actualEndorsers []string

	payload, err := protoutil.UnmarshalPayload(transaction.GetPayload())
	require.NoError(t, err)
	txn, err := protoutil.UnmarshalTransaction(payload.GetData())
	require.NoError(t, err)
	for _, action := range txn.GetActions() {
		cap, err := protoutil.UnmarshalChaincodeActionPayload(action.GetPayload())
		require.NoError(t, err)
		for _, endorsement := range cap.GetAction().GetEndorsements() {
			actualEndorsers = append(actualEndorsers, string(endorsement.GetEndorser()))
		}
	}

	require.ElementsMatch(t, expectedEndorsers, actualEndorsers)
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
		dialer: func(ctx context.Context, target string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
			endpoint = target
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
