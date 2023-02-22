/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cluster

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
)

//go:generate mockery --dir . --name StepClient --case underscore --output ./mocks/

// StepClient defines a client that sends and receives Step requests and responses.
type StepClient interface {
	Send(*orderer.StepRequest) error
	Recv() (*orderer.StepResponse, error)
	grpc.ClientStream
}

//go:generate mockery --dir . --name ClusterClient --case underscore --output ./mocks/

// ClusterClient creates streams that point to a remote cluster member.
type ClusterClient interface {
	Step(ctx context.Context, opts ...grpc.CallOption) (orderer.Cluster_StepClient, error)
}

// RPC performs remote procedure calls to remote cluster nodes.
type RPC struct {
	consensusLock sync.Mutex
	submitLock    sync.Mutex
	Logger        *flogging.FabricLogger
	Timeout       time.Duration
	Channel       string
	Comm          Communicator
	lock          sync.RWMutex
	StreamsByType map[OperationType]map[uint64]*Stream
}

// NewStreamsByType returns a mapping of operation type to
// a mapping of destination to stream.
func NewStreamsByType() map[OperationType]map[uint64]*Stream {
	m := make(map[OperationType]map[uint64]*Stream)
	m[ConsensusOperation] = make(map[uint64]*Stream)
	m[SubmitOperation] = make(map[uint64]*Stream)
	return m
}

// OperationType denotes a type of operation that the RPC can perform
// such as sending a transaction, or a consensus related message.
type OperationType int

const (
	ConsensusOperation OperationType = iota
	SubmitOperation
)

func (ot OperationType) String() string {
	if ot == SubmitOperation {
		return "transaction"
	}

	return "consensus"
}

// SendConsensus passes the given ConsensusRequest message to the raft.Node instance.
func (s *RPC) SendConsensus(destination uint64, msg *orderer.ConsensusRequest) error {
	if s.Logger.IsEnabledFor(zapcore.DebugLevel) {
		defer s.consensusSent(time.Now(), destination, msg)
	}

	stream, err := s.getOrCreateStream(destination, ConsensusOperation)
	if err != nil {
		return err
	}

	req := &orderer.StepRequest{
		Payload: &orderer.StepRequest_ConsensusRequest{
			ConsensusRequest: msg,
		},
	}

	s.consensusLock.Lock()
	defer s.consensusLock.Unlock()

	err = stream.Send(req)
	if err != nil {
		s.unMapStream(destination, ConsensusOperation, stream.ID)
	}

	return err
}

// SendSubmit sends a SubmitRequest to the given destination node.
func (s *RPC) SendSubmit(destination uint64, request *orderer.SubmitRequest, report func(error)) error {
	if s.Logger.IsEnabledFor(zapcore.DebugLevel) {
		defer s.submitSent(time.Now(), destination, request)
	}

	stream, err := s.getOrCreateStream(destination, SubmitOperation)
	if err != nil {
		return err
	}

	req := &orderer.StepRequest{
		Payload: &orderer.StepRequest_SubmitRequest{
			SubmitRequest: request,
		},
	}

	unmapOnFailure := func(err error) {
		if err != nil && err.Error() == io.EOF.Error() {
			s.Logger.Infof("Un-mapping transaction stream to %d because encountered a stale stream", destination)
			s.unMapStream(destination, SubmitOperation, stream.ID)
		}
		report(err)
	}

	s.submitLock.Lock()
	defer s.submitLock.Unlock()

	err = stream.SendWithReport(req, unmapOnFailure)
	if err != nil {
		s.unMapStream(destination, SubmitOperation, stream.ID)
	}
	return err
}

func (s *RPC) submitSent(start time.Time, to uint64, msg *orderer.SubmitRequest) {
	s.Logger.Debugf("Sending msg of %d bytes to %d on channel %s took %v", submitMsgLength(msg), to, s.Channel, time.Since(start))
}

func (s *RPC) consensusSent(start time.Time, to uint64, msg *orderer.ConsensusRequest) {
	s.Logger.Debugf("Sending msg of %d bytes to %d on channel %s took %v", len(msg.Payload), to, s.Channel, time.Since(start))
}

// getOrCreateStream obtains a Submit stream for the given destination node
func (s *RPC) getOrCreateStream(destination uint64, operationType OperationType) (*Stream, error) {
	stream := s.getStream(destination, operationType)
	if stream != nil {
		return stream, nil
	}
	stub, err := s.Comm.Remote(s.Channel, destination)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	stream, err = stub.NewStream(s.Timeout)
	if err != nil {
		return nil, err
	}
	s.mapStream(destination, stream, operationType)
	return stream, nil
}

func (s *RPC) getStream(destination uint64, operationType OperationType) *Stream {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return s.StreamsByType[operationType][destination]
}

func (s *RPC) mapStream(destination uint64, stream *Stream, operationType OperationType) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.StreamsByType[operationType][destination] = stream
	s.cleanCanceledStreams(operationType)
}

func (s *RPC) unMapStream(destination uint64, operationType OperationType, streamIDToUnmap uint64) {
	s.lock.Lock()
	defer s.lock.Unlock()

	stream, exists := s.StreamsByType[operationType][destination]
	if !exists {
		s.Logger.Debugf("No %s stream to %d found, nothing to unmap", operationType, destination)
		return
	}

	if stream.ID != streamIDToUnmap {
		s.Logger.Debugf("Stream for %s to %d has an ID of %d, not %d", operationType, destination, stream.ID, streamIDToUnmap)
		return
	}

	delete(s.StreamsByType[operationType], destination)
}

func (s *RPC) cleanCanceledStreams(operationType OperationType) {
	for destination, stream := range s.StreamsByType[operationType] {
		if !stream.Canceled() {
			continue
		}
		s.Logger.Infof("Removing stream %d to %d for channel %s because it is canceled", stream.ID, destination, s.Channel)
		delete(s.StreamsByType[operationType], destination)
	}
}

func submitMsgLength(request *orderer.SubmitRequest) int {
	if request.Payload == nil {
		return 0
	}
	return len(request.Payload.Payload)
}
