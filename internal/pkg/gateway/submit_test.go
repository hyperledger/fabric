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
	"github.com/hyperledger/fabric-protos-go/msp"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/internal/pkg/gateway/mocks"
	"github.com/hyperledger/fabric/internal/pkg/peer/orderers"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

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
				Address: "orderer1:7050",
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
				Address: "orderer1:7050",
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
				Address: "orderer1:7050",
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
				abbc := &mocks.ABBClient{}
				abbc.RecvReturns(nil, nil)
				orderer1Mock.client.(*mocks.ABClient).BroadcastReturns(abbc, nil)
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
				abbc := &mocks.ABBClient{}
				response := &ab.BroadcastResponse{
					Status: cp.Status_BAD_REQUEST,
					Info:   "err-info",
				}
				abbc.RecvReturns(response, nil)
				orderer1Mock.client.(*mocks.ABClient).BroadcastReturns(abbc, nil)
			},
			errCode:   codes.Aborted,
			errString: fmt.Sprintf("received unsuccessful response from orderer: status=%s, info=err-info", cp.Status_name[int32(cp.Status_BAD_REQUEST)]),
		},
		{
			name: "dialing orderer endpoint fails",
			plan: endorsementPlan{
				"g1": {{endorser: localhostMock}},
			},
			postSetup: func(t *testing.T, def *preparedTest) {
				def.dialer.Calls(func(_ context.Context, target string, _ ...grpc.DialOption) (*grpc.ClientConn, error) {
					if target == "orderer1:7050" {
						return nil, fmt.Errorf("orderer not answering")
					}
					return nil, nil
				})
			},
			errCode:   codes.Unavailable,
			errString: "no orderer nodes available",
		},
		{
			name: "multiple non-BFT orderers - only one needs to succeed",
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
			postTest: func(t *testing.T, def *preparedTest) {
				// only one orderer is invoked
				var invoked int
				for _, o := range ordererMocks {
					invoked += o.client.(*mocks.ABClient).BroadcastCallCount()
				}
				require.Equal(t, invoked, 1)
			},
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
				abbc := &mocks.ABBClient{}
				abbc.SendReturnsOnCall(0, status.Error(codes.Unavailable, "First orderer error"))
				abbc.SendReturnsOnCall(1, status.Error(codes.Unavailable, "Second orderer error"))
				abbc.SendReturnsOnCall(2, nil) // third time lucky
				abbc.RecvReturns(&ab.BroadcastResponse{
					Info:   "success",
					Status: cp.Status(200),
				}, nil)
				orderer1Mock.client.(*mocks.ABClient).BroadcastReturns(abbc, nil)
				orderer2Mock.client.(*mocks.ABClient).BroadcastReturns(abbc, nil)
				orderer3Mock.client.(*mocks.ABClient).BroadcastReturns(abbc, nil)
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
				orderer1Mock.client.(*mocks.ABClient).BroadcastReturns(abbc, nil)
				orderer2Mock.client.(*mocks.ABClient).BroadcastReturns(abbc, nil)
				orderer3Mock.client.(*mocks.ABClient).BroadcastReturns(abbc, nil)
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

				abbc := &mocks.ABBClient{}
				abbc.SendReturns(nil)
				abbc.RecvReturns(&ab.BroadcastResponse{
					Info:   "success",
					Status: cp.Status(200),
				}, nil)
				bs := func(ctx context.Context, co ...grpc.CallOption) (ab.AtomicBroadcast_BroadcastClient, error) {
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
				orderer1Mock.client.(*mocks.ABClient).BroadcastStub = bs
				orderer2Mock.client.(*mocks.ABClient).BroadcastStub = bs
				orderer3Mock.client.(*mocks.ABClient).BroadcastStub = bs
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

				abbc := &mocks.ABBClient{}
				abbc.SendReturns(nil)
				abbc.RecvReturns(&ab.BroadcastResponse{
					Info:   "success",
					Status: cp.Status(200),
				}, nil)
				bs := func(ctx context.Context, co ...grpc.CallOption) (ab.AtomicBroadcast_BroadcastClient, error) {
					select {
					case <-time.After(broadcastTime):
						return abbc, nil
					case <-ctx.Done():
						return nil, ctx.Err()
					}
				}
				orderer1Mock.client.(*mocks.ABClient).BroadcastStub = bs
				orderer2Mock.client.(*mocks.ABClient).BroadcastStub = bs
				orderer3Mock.client.(*mocks.ABClient).BroadcastStub = bs
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
		{
			name:  "multiple BFT orderers are all invoked",
			isBFT: true,
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
							{Host: "orderer4", Port: 7050},
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
				ordererStatus:          200,
			},
			postTest: func(t *testing.T, def *preparedTest) {
				require.Eventually(t, func() bool {
					return orderer1Mock.client.(*mocks.ABClient).BroadcastCallCount() == 1 &&
						orderer2Mock.client.(*mocks.ABClient).BroadcastCallCount() == 1 &&
						orderer3Mock.client.(*mocks.ABClient).BroadcastCallCount() == 1 &&
						orderer4Mock.client.(*mocks.ABClient).BroadcastCallCount() == 1
				}, broadcastTimeout, 10*time.Millisecond)
			},
		},
		{
			name:  "multiple BFT orderers all fail",
			isBFT: true,
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
			errString: "insufficient number of orderers could successfully process transaction to satisfy quorum requirement",
		},
		{
			name:  "7 BFT orderers can tolerate 2 faults",
			isBFT: true,
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
							{Host: "orderer4", Port: 7050},
							{Host: "orderer5", Port: 7050},
							{Host: "orderer6", Port: 7050},
							{Host: "orderer7", Port: 7050},
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
				ordererStatus:          200,
			},
			postSetup: func(t *testing.T, def *preparedTest) {
				orderer1Mock.client.(*mocks.ABClient).BroadcastReturns(nil, errors.New("orderer1 fails"))
				orderer4Mock.client.(*mocks.ABClient).BroadcastReturns(nil, errors.New("orderer4 fails"))
			},
		},
		{
			name:  "7 BFT orderers cannot tolerate 3 faults",
			isBFT: true,
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
							{Host: "orderer4", Port: 7050},
							{Host: "orderer5", Port: 7050},
							{Host: "orderer6", Port: 7050},
							{Host: "orderer7", Port: 7050},
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
				ordererStatus:          200,
			},
			postSetup: func(t *testing.T, def *preparedTest) {
				abbc := &mocks.ABBClient{}
				response := &ab.BroadcastResponse{
					Status: cp.Status_BAD_REQUEST,
					Info:   "extra details",
				}
				abbc.RecvReturns(response, nil)
				orderer1Mock.client.(*mocks.ABClient).BroadcastReturns(nil, status.Error(codes.Unavailable, "orderer1 fails"))
				orderer4Mock.client.(*mocks.ABClient).BroadcastReturns(nil, status.Error(codes.Unavailable, "orderer4 fails"))
				orderer7Mock.client.(*mocks.ABClient).BroadcastReturns(abbc, nil)
			},
			errCode:   codes.Unavailable,
			errString: "insufficient number of orderers could successfully process transaction to satisfy quorum requirement",
			errDetails: []*pb.ErrorDetail{
				{
					Address: "orderer1:7050",
					MspId:   "msp1",
					Message: "rpc error: code = Unavailable desc = orderer1 fails",
				},
				{
					Address: "orderer4:7050",
					MspId:   "msp1",
					Message: "rpc error: code = Unavailable desc = orderer4 fails",
				},
				{
					Address: "orderer7:7050",
					MspId:   "msp1",
					Message: "received unsuccessful response from orderer: status=BAD_REQUEST, info=extra details",
				},
			},
		},
		{
			name:  "all BFT orderers exceed timeout",
			isBFT: true,
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
				ordererStatus:          200,
			},
			postSetup: func(t *testing.T, def *preparedTest) {
				def.ctx, def.cancel = context.WithTimeout(def.ctx, 50*time.Millisecond)
				abbc := &mocks.ABBClient{}
				abbc.SendReturns(nil)
				abbc.RecvStub = func() (*ab.BroadcastResponse, error) {
					time.Sleep(100 * time.Millisecond)
					return nil, nil
				}
				orderer1Mock.client.(*mocks.ABClient).BroadcastReturns(abbc, nil)
				orderer2Mock.client.(*mocks.ABClient).BroadcastReturns(abbc, nil)
				orderer3Mock.client.(*mocks.ABClient).BroadcastReturns(abbc, nil)
			},
			postTest: func(t *testing.T, def *preparedTest) {
				def.cancel()
				require.Equal(t, 1, orderer1Mock.client.(*mocks.ABClient).BroadcastCallCount())
				require.Equal(t, 1, orderer2Mock.client.(*mocks.ABClient).BroadcastCallCount())
				require.Equal(t, 1, orderer3Mock.client.(*mocks.ABClient).BroadcastCallCount())
			},
			errCode:   codes.DeadlineExceeded,
			errString: "submit timeout expired while broadcasting to ordering service",
		},
		{
			name:  "one BFT orderer exceeds timeout",
			isBFT: true,
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
							{Host: "orderer4", Port: 7050},
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
				ordererStatus:          200,
			},
			postSetup: func(t *testing.T, def *preparedTest) {
				def.ctx, def.cancel = context.WithTimeout(def.ctx, 50*time.Millisecond)
				abbc := &mocks.ABBClient{}
				abbc.SendReturns(nil)
				abbc.RecvStub = func() (*ab.BroadcastResponse, error) {
					time.Sleep(100 * time.Millisecond)
					return nil, nil
				}
				orderer2Mock.client.(*mocks.ABClient).BroadcastReturns(abbc, nil)
			},
			postTest: func(t *testing.T, def *preparedTest) {
				def.cancel()
				require.Eventually(t, func() bool {
					return orderer1Mock.client.(*mocks.ABClient).BroadcastCallCount() == 1 &&
						orderer2Mock.client.(*mocks.ABClient).BroadcastCallCount() == 1 &&
						orderer3Mock.client.(*mocks.ABClient).BroadcastCallCount() == 1 &&
						orderer4Mock.client.(*mocks.ABClient).BroadcastCallCount() == 1
				}, broadcastTimeout, 10*time.Millisecond)
			},
		},
		{
			name:  "one BFT orderer cannot connect - quorum satisfied",
			isBFT: true,
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
							{Host: "orderer4", Port: 7050},
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
				ordererStatus:          200,
			},
			postSetup: func(t *testing.T, def *preparedTest) {
				def.dialer.Calls(func(_ context.Context, target string, _ ...grpc.DialOption) (*grpc.ClientConn, error) {
					if target == "orderer2:7050" {
						return nil, fmt.Errorf("orderer not answering")
					}
					return nil, nil
				})
			},
			postTest: func(t *testing.T, def *preparedTest) {
				require.Equal(t, 1, orderer1Mock.client.(*mocks.ABClient).BroadcastCallCount())
				require.Equal(t, 0, orderer2Mock.client.(*mocks.ABClient).BroadcastCallCount())
				require.Equal(t, 1, orderer3Mock.client.(*mocks.ABClient).BroadcastCallCount())
				require.Equal(t, 1, orderer4Mock.client.(*mocks.ABClient).BroadcastCallCount())
			},
		},
		{
			name:  "two BFT orderers cannot connect - quorum not satisfied",
			isBFT: true,
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
							{Host: "orderer4", Port: 7050},
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
				ordererStatus:          200,
			},
			postSetup: func(t *testing.T, def *preparedTest) {
				def.dialer.Calls(func(_ context.Context, target string, _ ...grpc.DialOption) (*grpc.ClientConn, error) {
					if target == "orderer2:7050" || target == "orderer3:7050" {
						return nil, fmt.Errorf("orderer not answering")
					}
					return nil, nil
				})
			},
			postTest: func(t *testing.T, def *preparedTest) {
				require.Equal(t, 1, orderer1Mock.client.(*mocks.ABClient).BroadcastCallCount())
				require.Equal(t, 0, orderer2Mock.client.(*mocks.ABClient).BroadcastCallCount())
				require.Equal(t, 0, orderer3Mock.client.(*mocks.ABClient).BroadcastCallCount())
				require.Equal(t, 1, orderer4Mock.client.(*mocks.ABClient).BroadcastCallCount())
			},
			errCode:   codes.Unavailable,
			errString: "insufficient number of orderers could successfully process transaction to satisfy quorum requirement",
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

// copied from the smartbft library...
func TestComputeBFTQuorum(t *testing.T) {
	// Ensure that quorum size is as expected.

	type quorum struct {
		N uint64
		F int
		Q int
	}

	quorums := []quorum{
		{4, 1, 3},
		{5, 1, 4},
		{6, 1, 4},
		{7, 2, 5},
		{8, 2, 6},
		{9, 2, 6},
		{10, 3, 7},
		{11, 3, 8},
		{12, 3, 8},
	}

	for _, testCase := range quorums {
		t.Run(fmt.Sprintf("%d nodes", testCase.N), func(t *testing.T) {
			Q, F := computeBFTQuorum(testCase.N)
			require.Equal(t, testCase.Q, Q)
			require.Equal(t, testCase.F, F)
		})
	}
}
