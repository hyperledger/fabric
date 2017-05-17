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
	"context"
	"testing"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/testutil"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/stretchr/testify/assert"
)

var adminServer *ServerAdmin

func init() {
	adminServer = NewAdminServer()
	testutil.SetupTestConfig()
}

func TestGetStatus(t *testing.T) {
	response, err := adminServer.GetStatus(context.Background(), &empty.Empty{})
	assert.NotNil(t, response, "Response should have been set")
	assert.Nil(t, err, "Error should have been nil")
}

func TestStartServer(t *testing.T) {
	response, err := adminServer.StartServer(context.Background(), &empty.Empty{})
	assert.NotNil(t, response, "Response should have been set")
	assert.Nil(t, err, "Error should have been nil")
}

func TestLoggingCalls(t *testing.T) {
	flogging.MustGetLogger("test")
	flogging.SetPeerStartupModulesMap()

	logResponse, err := adminServer.GetModuleLogLevel(context.Background(), &pb.LogLevelRequest{LogModule: "test"})
	assert.NotNil(t, logResponse, "logResponse should have been set")
	assert.Equal(t, flogging.DefaultLevel(), logResponse.LogLevel, "log level should have been the default")
	assert.Nil(t, err, "Error should have been nil")

	logResponse, err = adminServer.SetModuleLogLevel(context.Background(), &pb.LogLevelRequest{LogModule: "test", LogLevel: "debug"})
	assert.NotNil(t, logResponse, "logResponse should have been set")
	assert.Equal(t, "DEBUG", logResponse.LogLevel, "log level should have been set to debug")
	assert.Nil(t, err, "Error should have been nil")

	_, err = adminServer.RevertLogLevels(context.Background(), &empty.Empty{})
	assert.Nil(t, err, "Error should have been nil")

	logResponse, err = adminServer.GetModuleLogLevel(context.Background(), &pb.LogLevelRequest{LogModule: "test"})
	assert.NotNil(t, logResponse, "logResponse should have been set")
	assert.Equal(t, flogging.DefaultLevel(), logResponse.LogLevel, "log level should have been the default")
	assert.Nil(t, err, "Error should have been nil")
}
