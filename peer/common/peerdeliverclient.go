/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"context"

	ccapi "github.com/hyperledger/fabric/peer/chaincode/api"
	pb "github.com/hyperledger/fabric/protos/peer"
	grpc "google.golang.org/grpc"
)

// PeerDeliverClient holds the necessary information to connect a client
// to a peer deliver service
type PeerDeliverClient struct {
	Client pb.DeliverClient
}

// Deliver connects the client to the Deliver RPC
func (dc PeerDeliverClient) Deliver(ctx context.Context, opts ...grpc.CallOption) (ccapi.Deliver, error) {
	d, err := dc.Client.Deliver(ctx, opts...)
	return d, err
}

// DeliverFiltered connects the client to the DeliverFiltered RPC
func (dc PeerDeliverClient) DeliverFiltered(ctx context.Context, opts ...grpc.CallOption) (ccapi.Deliver, error) {
	df, err := dc.Client.DeliverFiltered(ctx, opts...)
	return df, err
}
