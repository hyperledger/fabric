/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package api

import (
	"context"

	pcommon "github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"google.golang.org/grpc"
)

//go:generate counterfeiter -o ../mock/deliverclient.go -fake-name DeliverClient . DeliverClient

// DeliverClient defines the interface for a peer's deliver service
type DeliverClient interface {
	Deliver(ctx context.Context, opts ...grpc.CallOption) (Deliver, error)
	DeliverFiltered(ctx context.Context, opts ...grpc.CallOption) (Deliver, error)
}

//go:generate counterfeiter -o ../mock/deliver.go -fake-name Deliver . Deliver

// Deliver defines the interface for delivering blocks
type Deliver interface {
	Send(*pcommon.Envelope) error
	Recv() (*pb.DeliverResponse, error)
	CloseSend() error
}
