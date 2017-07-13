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

package core

import (
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/hyperledger/fabric/common/flogging"
	pb "github.com/hyperledger/fabric/protos/peer"
	"golang.org/x/net/context"
)

var log = flogging.MustGetLogger("server")

// NewAdminServer creates and returns a Admin service instance.
func NewAdminServer() *ServerAdmin {
	s := new(ServerAdmin)
	return s
}

// ServerAdmin implementation of the Admin service for the Peer
type ServerAdmin struct {
}

// GetStatus reports the status of the server
func (*ServerAdmin) GetStatus(context.Context, *empty.Empty) (*pb.ServerStatus, error) {
	status := &pb.ServerStatus{Status: pb.ServerStatus_STARTED}
	log.Debugf("returning status: %s", status)
	return status, nil
}

// StartServer starts the server
func (*ServerAdmin) StartServer(context.Context, *empty.Empty) (*pb.ServerStatus, error) {
	status := &pb.ServerStatus{Status: pb.ServerStatus_STARTED}
	log.Debugf("returning status: %s", status)
	return status, nil
}

// GetModuleLogLevel gets the current logging level for the specified module
// TODO Modify the signature so as to remove the error return - it's always been nil
func (*ServerAdmin) GetModuleLogLevel(ctx context.Context, request *pb.LogLevelRequest) (*pb.LogLevelResponse, error) {
	logLevelString := flogging.GetModuleLevel(request.LogModule)
	logResponse := &pb.LogLevelResponse{LogModule: request.LogModule, LogLevel: logLevelString}
	return logResponse, nil
}

// SetModuleLogLevel sets the logging level for the specified module
func (*ServerAdmin) SetModuleLogLevel(ctx context.Context, request *pb.LogLevelRequest) (*pb.LogLevelResponse, error) {
	logLevelString, err := flogging.SetModuleLevel(request.LogModule, request.LogLevel)
	logResponse := &pb.LogLevelResponse{LogModule: request.LogModule, LogLevel: logLevelString}
	return logResponse, err
}

// RevertLogLevels reverts the log levels for all modules to the level
// defined at the end of peer startup.
func (*ServerAdmin) RevertLogLevels(context.Context, *empty.Empty) (*empty.Empty, error) {
	err := flogging.RevertToPeerStartupLevels()

	return &empty.Empty{}, err
}
