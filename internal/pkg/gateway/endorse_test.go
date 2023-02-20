/*
Copyright 2021 IBM All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gateway

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	cp "github.com/hyperledger/fabric-protos-go/common"
	dp "github.com/hyperledger/fabric-protos-go/discovery"
	pb "github.com/hyperledger/fabric-protos-go/gateway"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/flogging/mock"
	"github.com/hyperledger/fabric/internal/pkg/gateway/mocks"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

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
			name: "use highest block height local org peer",
			plan: endorsementPlan{
				"g1": {{endorser: peer1Mock, height: 5}, {endorser: localhostMock, height: 4}}, // msp1
			},
			members: []networkMember{
				{string(localhostMock.pkiid), localhostMock.address, localhostMock.mspid, 4},
				{string(peer1Mock.pkiid), peer1Mock.address, peer1Mock.mspid, 5},
			},
			localLedgerHeight: 4,
			expectedEndorsers: []string{peer1Mock.address},
		},
		{
			name: "use local host ledger height",
			plan: endorsementPlan{
				"g1": {{endorser: peer1Mock, height: 5}, {endorser: localhostMock, height: 4}}, // msp1
			},
			members: []networkMember{
				{string(localhostMock.pkiid), localhostMock.address, localhostMock.mspid, 4},
				{string(peer1Mock.pkiid), peer1Mock.address, peer1Mock.mspid, 5},
			},
			localLedgerHeight: 6,
			expectedEndorsers: []string{localhostMock.address},
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
