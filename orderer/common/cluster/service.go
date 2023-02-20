/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cluster

import (
	"context"
	"io"
	"time"

	"github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/util"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

//go:generate mockery --dir . --name Dispatcher --case underscore --output ./mocks/

// Dispatcher dispatches requests
type Dispatcher interface {
	DispatchSubmit(ctx context.Context, request *orderer.SubmitRequest) error
	DispatchConsensus(ctx context.Context, request *orderer.ConsensusRequest) error
}

//go:generate mockery --dir . --name StepStream --case underscore --output ./mocks/

// StepStream defines the gRPC stream for sending
// transactions, and receiving corresponding responses
type StepStream interface {
	Send(response *orderer.StepResponse) error
	Recv() (*orderer.StepRequest, error)
	grpc.ServerStream
}

// Service defines the raft Service
type Service struct {
	StreamCountReporter              *StreamCountReporter
	Dispatcher                       Dispatcher
	Logger                           *flogging.FabricLogger
	StepLogger                       *flogging.FabricLogger
	MinimumExpirationWarningInterval time.Duration
	CertExpWarningThreshold          time.Duration
}

// Step passes an implementation-specific message to another cluster member.
func (s *Service) Step(stream orderer.Cluster_StepServer) error {
	s.StreamCountReporter.Increment()
	defer s.StreamCountReporter.Decrement()

	addr := util.ExtractRemoteAddress(stream.Context())
	commonName := commonNameFromContext(stream.Context())
	exp := s.initializeExpirationCheck(stream, addr, commonName)
	s.Logger.Debugf("Connection from %s(%s)", commonName, addr)
	defer s.Logger.Debugf("Closing connection from %s(%s)", commonName, addr)
	for {
		err := s.handleMessage(stream, addr, exp)
		if err == io.EOF {
			s.Logger.Debugf("%s(%s) disconnected", commonName, addr)
			return nil
		}
		if err != nil {
			return err
		}
		// Else, no error occurred, so we continue to the next iteration
	}
}

func (s *Service) handleMessage(stream StepStream, addr string, exp *certificateExpirationCheck) error {
	request, err := stream.Recv()
	if err == io.EOF {
		return err
	}
	if err != nil {
		s.Logger.Warningf("Stream read from %s failed: %v", addr, err)
		return err
	}

	exp.checkExpiration(time.Now(), extractChannel(request))

	if s.StepLogger.IsEnabledFor(zap.DebugLevel) {
		nodeName := commonNameFromContext(stream.Context())
		s.StepLogger.Debugf("Received message from %s(%s): %v", nodeName, addr, requestAsString(request))
	}

	if submitReq := request.GetSubmitRequest(); submitReq != nil {
		nodeName := commonNameFromContext(stream.Context())
		s.Logger.Debugf("Received message from %s(%s): %v", nodeName, addr, requestAsString(request))
		return s.handleSubmit(submitReq, stream, addr)
	} else if consensusReq := request.GetConsensusRequest(); consensusReq != nil {
		return s.Dispatcher.DispatchConsensus(stream.Context(), request.GetConsensusRequest())
	}

	return errors.Errorf("message is neither a Submit nor a Consensus request")
}

func (s *Service) handleSubmit(request *orderer.SubmitRequest, stream StepStream, addr string) error {
	err := s.Dispatcher.DispatchSubmit(stream.Context(), request)
	if err != nil {
		s.Logger.Warningf("Handling of Submit() from %s failed: %v", addr, err)
		return err
	}
	return err
}

func (s *Service) initializeExpirationCheck(stream orderer.Cluster_StepServer, endpoint, nodeName string) *certificateExpirationCheck {
	return &certificateExpirationCheck{
		minimumExpirationWarningInterval: s.MinimumExpirationWarningInterval,
		expirationWarningThreshold:       s.CertExpWarningThreshold,
		expiresAt:                        expiresAt(stream),
		endpoint:                         endpoint,
		nodeName:                         nodeName,
		alert: func(template string, args ...interface{}) {
			s.Logger.Warningf(template, args...)
		},
	}
}

func expiresAt(stream orderer.Cluster_StepServer) time.Time {
	cert := util.ExtractCertificateFromContext(stream.Context())
	if cert == nil {
		return time.Time{}
	}
	return cert.NotAfter
}

func extractChannel(msg *orderer.StepRequest) string {
	if consReq := msg.GetConsensusRequest(); consReq != nil {
		return consReq.Channel
	}

	if submitReq := msg.GetSubmitRequest(); submitReq != nil {
		return submitReq.Channel
	}

	return ""
}
