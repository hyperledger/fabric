/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cluster

import (
	"context"
	"sync"

	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

//go:generate mockery -dir . -name SubmitClient -case underscore -output ./mocks/

// SubmitClient is the Submit gRPC stream
type SubmitClient interface {
	Send(request *orderer.SubmitRequest) error
	Recv() (*orderer.SubmitResponse, error)
	grpc.ClientStream
}

//go:generate mockery -dir . -name Client -case underscore -output ./mocks/

// Client is the definition of operations that the Cluster gRPC service
// exposes to cluster nodes.
type Client interface {
	// Submit submits transactions to a cluster member
	Submit(ctx context.Context, opts ...grpc.CallOption) (orderer.Cluster_SubmitClient, error)
	// Step passes an implementation-specific message to another cluster member.
	Step(ctx context.Context, in *orderer.StepRequest, opts ...grpc.CallOption) (*orderer.StepResponse, error)
}

// RPC performs remote procedure calls to remote cluster nodes.
type RPC struct {
	Channel             string
	Comm                Communicator
	lock                sync.RWMutex
	DestinationToStream map[uint64]orderer.Cluster_SubmitClient
}

// Step sends a StepRequest to the given destination node and returns the response
func (s *RPC) Step(destination uint64, msg *orderer.StepRequest) (*orderer.StepResponse, error) {
	stub, err := s.Comm.Remote(s.Channel, destination)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return stub.Step(msg)
}

// SendSubmit sends a SubmitRequest to the given destination node
func (s *RPC) SendSubmit(destination uint64, request *orderer.SubmitRequest) error {
	stream, err := s.getProposeStream(destination)
	if err != nil {
		return err
	}
	err = stream.Send(request)
	if err != nil {
		s.unMapStream(destination)
	}
	return err
}

// ReceiveSubmitResponse receives a SubmitResponse from the given destination node
func (s *RPC) ReceiveSubmitResponse(destination uint64) (*orderer.SubmitResponse, error) {
	stream, err := s.getProposeStream(destination)
	if err != nil {
		return nil, err
	}
	msg, err := stream.Recv()
	if err != nil {
		s.unMapStream(destination)
	}
	return msg, err
}

// getProposeStream obtains a Submit stream for the given destination node
func (s *RPC) getProposeStream(destination uint64) (orderer.Cluster_SubmitClient, error) {
	stream := s.getStream(destination)
	if stream != nil {
		return stream, nil
	}
	stub, err := s.Comm.Remote(s.Channel, destination)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	stream, err = stub.SubmitStream()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	s.mapStream(destination, stream)
	return stream, nil
}

func (s *RPC) getStream(destination uint64) orderer.Cluster_SubmitClient {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return s.DestinationToStream[destination]
}

func (s *RPC) mapStream(destination uint64, stream orderer.Cluster_SubmitClient) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.DestinationToStream[destination] = stream
}

func (s *RPC) unMapStream(destination uint64) {
	s.lock.Lock()
	defer s.lock.Unlock()
	delete(s.DestinationToStream, destination)
}
