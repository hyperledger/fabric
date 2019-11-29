/*
Copyright IBM Corp. 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"context"

	cb "github.com/hyperledger/fabric-protos-go/common"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	grpc "google.golang.org/grpc"
)

// GetMockEndorserClient return a endorser client return specified ProposalResponse and err(nil or error)
func GetMockEndorserClient(response *pb.ProposalResponse, err error) pb.EndorserClient {
	return &mockEndorserClient{
		response: response,
		err:      err,
	}
}

type mockEndorserClient struct {
	response *pb.ProposalResponse
	err      error
}

func (m *mockEndorserClient) ProcessProposal(ctx context.Context, in *pb.SignedProposal, opts ...grpc.CallOption) (*pb.ProposalResponse, error) {
	return m.response, m.err
}

func GetMockBroadcastClient(err error) BroadcastClient {
	return &mockBroadcastClient{err: err}
}

// mockBroadcastClient return success immediately
type mockBroadcastClient struct {
	err error
}

func (m *mockBroadcastClient) Send(env *cb.Envelope) error {
	return m.err
}

func (m *mockBroadcastClient) Close() error {
	return nil
}
