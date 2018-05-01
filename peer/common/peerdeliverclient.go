/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"context"

	"github.com/hyperledger/fabric/peer/chaincode/api"
	pb "github.com/hyperledger/fabric/protos/peer"
	grpc "google.golang.org/grpc"
)

type DeliverClient struct {
	Client pb.DeliverClient
}

func (dc DeliverClient) Deliver(ctx context.Context, opts ...grpc.CallOption) (api.Deliver, error) {
	d, err := dc.Client.Deliver(ctx, opts...)
	return d, err
}

func (dc DeliverClient) DeliverFiltered(ctx context.Context, opts ...grpc.CallOption) (api.Deliver, error) {
	df, err := dc.Client.DeliverFiltered(ctx, opts...)
	return df, err
}
