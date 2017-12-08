/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package common

import (
	"github.com/golang/protobuf/ptypes/empty"
	cb "github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	context "golang.org/x/net/context"
	grpc "google.golang.org/grpc"
)

type MockResponse struct {
	Response *pb.ProposalResponse
	Error    error
}

// GetMockEndorserClient return a endorser client return specified ProposalResponse and err(nil or error)
func GetMockMultiEndorserClient(responses ...MockResponse) pb.EndorserClient {
	return &mockEndorserClient{
		responses: responses,
	}
}

// GetMockEndorserClient return a endorser client return specified ProposalResponse and err(nil or error)
func GetMockEndorserClient(resp *pb.ProposalResponse, err error) pb.EndorserClient {
	return GetMockMultiEndorserClient(MockResponse{resp, err})
}

type mockEndorserClient struct {
	i         int
	responses []MockResponse
}

func (m *mockEndorserClient) ProcessProposal(ctx context.Context, in *pb.SignedProposal, opts ...grpc.CallOption) (*pb.ProposalResponse, error) {
	if len(m.responses) == 1 {
		// Constant response
		return m.responses[0].Response, m.responses[0].Error
	}
	m.i++
	return m.responses[m.i-1].Response, m.responses[m.i-1].Error
}

func GetMockBroadcastClient(err error) *MockBroadcastClient {
	return &MockBroadcastClient{err: err}
}

// MockBroadcastClient return success immediately
type MockBroadcastClient struct {
	Envelope *cb.Envelope
	err      error
}

func (m *MockBroadcastClient) Send(env *cb.Envelope) error {
	m.Envelope = env
	return m.err
}

func (m *MockBroadcastClient) Close() error {
	return nil
}

func GetMockAdminClient(err error) pb.AdminClient {
	return &mockAdminClient{err: err}
}

type mockAdminClient struct {
	status *pb.ServerStatus
	err    error
}

func (m *mockAdminClient) GetStatus(ctx context.Context, in *empty.Empty, opts ...grpc.CallOption) (*pb.ServerStatus, error) {
	return m.status, m.err
}

func (m *mockAdminClient) StartServer(ctx context.Context, in *empty.Empty, opts ...grpc.CallOption) (*pb.ServerStatus, error) {
	m.status = &pb.ServerStatus{Status: pb.ServerStatus_STARTED}
	return m.status, m.err
}

func (m *mockAdminClient) StopServer(ctx context.Context, in *empty.Empty, opts ...grpc.CallOption) (*pb.ServerStatus, error) {
	m.status = &pb.ServerStatus{Status: pb.ServerStatus_STOPPED}
	return m.status, m.err
}

func (m *mockAdminClient) GetModuleLogLevel(ctx context.Context, in *pb.LogLevelRequest, opts ...grpc.CallOption) (*pb.LogLevelResponse, error) {
	response := &pb.LogLevelResponse{LogModule: in.LogModule, LogLevel: "INFO"}
	return response, m.err
}

func (m *mockAdminClient) SetModuleLogLevel(ctx context.Context, in *pb.LogLevelRequest, opts ...grpc.CallOption) (*pb.LogLevelResponse, error) {
	response := &pb.LogLevelResponse{LogModule: in.LogModule, LogLevel: in.LogLevel}
	return response, m.err
}

func (m *mockAdminClient) RevertLogLevels(ctx context.Context, in *empty.Empty, opts ...grpc.CallOption) (*empty.Empty, error) {
	return &empty.Empty{}, m.err
}
