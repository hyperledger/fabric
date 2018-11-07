/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package admin

import (
	"context"
	"fmt"
	"strings"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var logger = flogging.MustGetLogger("server")

type requestValidator interface {
	validate(ctx context.Context, env *common.Envelope) (*pb.AdminOperation, error)
}

// AccessControlEvaluator evaluates whether the creator of the given SignedData
// is eligible of using the admin service
type AccessControlEvaluator interface {
	// Evaluate evaluates the eligibility of the creator of the given SignedData
	// for being serviced by the admin service
	Evaluate(signatureSet []*common.SignedData) error
}

// NewAdminServer creates and returns a Admin service instance.
func NewAdminServer(ace AccessControlEvaluator) *ServerAdmin {
	s := &ServerAdmin{
		v: &validator{
			ace: ace,
		},
		specAtStartup: flogging.Global.Spec(),
	}
	return s
}

// ServerAdmin implementation of the Admin service for the Peer
type ServerAdmin struct {
	v requestValidator

	specAtStartup string
}

func (s *ServerAdmin) GetStatus(ctx context.Context, env *common.Envelope) (*pb.ServerStatus, error) {
	if _, err := s.v.validate(ctx, env); err != nil {
		return nil, err
	}
	status := &pb.ServerStatus{Status: pb.ServerStatus_STARTED}
	logger.Debugf("returning status: %s", status)
	return status, nil
}

func (s *ServerAdmin) StartServer(ctx context.Context, env *common.Envelope) (*pb.ServerStatus, error) {
	if _, err := s.v.validate(ctx, env); err != nil {
		return nil, err
	}
	status := &pb.ServerStatus{Status: pb.ServerStatus_STARTED}
	logger.Debugf("returning status: %s", status)
	return status, nil
}

func (s *ServerAdmin) GetModuleLogLevel(ctx context.Context, env *common.Envelope) (*pb.LogLevelResponse, error) {
	op, err := s.v.validate(ctx, env)
	if err != nil {
		return nil, err
	}
	request := op.GetLogReq()
	if request == nil {
		return nil, errors.New("request is nil")
	}
	logLevelString := flogging.GetLoggerLevel(request.LogModule)
	logResponse := &pb.LogLevelResponse{LogModule: request.LogModule, LogLevel: logLevelString}
	return logResponse, nil
}

func (s *ServerAdmin) SetModuleLogLevel(ctx context.Context, env *common.Envelope) (*pb.LogLevelResponse, error) {
	op, err := s.v.validate(ctx, env)
	if err != nil {
		return nil, err
	}
	request := op.GetLogReq()
	if request == nil {
		return nil, errors.New("request is nil")
	}

	spec := fmt.Sprintf("%s:%s=%s", flogging.Global.Spec(), request.LogModule, request.LogLevel)
	err = flogging.Global.ActivateSpec(spec)
	if err != nil {
		err = status.Errorf(codes.InvalidArgument, "error setting log spec to '%s': %s", spec, err.Error())
		return nil, err
	}

	logResponse := &pb.LogLevelResponse{LogModule: request.LogModule, LogLevel: strings.ToUpper(request.LogLevel)}
	return logResponse, nil
}

func (s *ServerAdmin) RevertLogLevels(ctx context.Context, env *common.Envelope) (*empty.Empty, error) {
	if _, err := s.v.validate(ctx, env); err != nil {
		return nil, err
	}
	flogging.ActivateSpec(s.specAtStartup)
	return &empty.Empty{}, nil
}

func (s *ServerAdmin) GetLogSpec(ctx context.Context, env *common.Envelope) (*pb.LogSpecResponse, error) {
	if _, err := s.v.validate(ctx, env); err != nil {
		return nil, err
	}
	logSpec := flogging.Global.Spec()
	logResponse := &pb.LogSpecResponse{LogSpec: logSpec}
	return logResponse, nil
}

func (s *ServerAdmin) SetLogSpec(ctx context.Context, env *common.Envelope) (*pb.LogSpecResponse, error) {
	op, err := s.v.validate(ctx, env)
	if err != nil {
		return nil, err
	}
	request := op.GetLogSpecReq()
	if request == nil {
		return nil, errors.New("request is nil")
	}
	err = flogging.Global.ActivateSpec(request.LogSpec)
	logResponse := &pb.LogSpecResponse{
		LogSpec: request.LogSpec,
	}
	if err != nil {
		logResponse.Error = err.Error()
	}
	return logResponse, nil
}
