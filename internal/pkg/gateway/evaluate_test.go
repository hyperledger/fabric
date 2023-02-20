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

	pb "github.com/hyperledger/fabric-protos-go/gateway"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/gossip/common"
	gdiscovery "github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/internal/pkg/gateway/mocks"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestEvaluate(t *testing.T) {
	tests := []testDef{
		{
			name: "single endorser",
			members: []networkMember{
				{"id1", "localhost:7051", "msp1", 5},
			},
			localLedgerHeight: 5,
			expectedEndorsers: []string{"localhost:7051"},
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
			localLedgerHeight: 5,
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
			localLedgerHeight: 5,
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
			localLedgerHeight: 5,
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
			localLedgerHeight: 5,
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
			localLedgerHeight: 5,
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
			localLedgerHeight: 4,
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
			localLedgerHeight: 11,
			transientData:     map[string][]byte{"transient-key": []byte("transient-value")},
			endorsingOrgs:     []string{"msp2", "msp3"},
			expectedEndorsers: []string{"peer3:10051"},
		},
		{
			name: "process proposal fails",
			members: []networkMember{
				{"id1", "localhost:7051", "msp1", 5},
			},
			localLedgerHeight: 5,
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
			localLedgerHeight: 4,
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
			localLedgerHeight: 4,
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
			localLedgerHeight: 4,
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
			localLedgerHeight: 4,
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
			localLedgerHeight: 3,
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
		{
			name: "uses local host ledger height",
			members: []networkMember{
				{"id2", "peer1:8051", "msp1", 6},
				{"id1", "localhost:7051", "msp1", 5},
			},
			localLedgerHeight: 7,
			expectedEndorsers: []string{"localhost:7051"},
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
