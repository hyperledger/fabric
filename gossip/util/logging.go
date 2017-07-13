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

package util

import (
	"sync"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/op/go-logging"
)

// Module names for logger initialization.
const (
	LoggingChannelModule   = "gossip/channel"
	LoggingCommModule      = "gossip/comm"
	LoggingDiscoveryModule = "gossip/discovery"
	LoggingElectionModule  = "gossip/election"
	LoggingGossipModule    = "gossip/gossip"
	LoggingMockModule      = "gossip/comm/mock"
	LoggingPullModule      = "gossip/pull"
	LoggingServiceModule   = "gossip/service"
	LoggingStateModule     = "gossip/state"
)

var loggersByModules = make(map[string]*logging.Logger)
var lock = sync.Mutex{}
var testMode bool

// defaultTestSpec is the default logging level for gossip tests
var defaultTestSpec = "WARNING"

// GetLogger returns a logger for given gossip module and peerID
func GetLogger(module string, peerID string) *logging.Logger {
	if peerID != "" && testMode {
		module = module + "#" + peerID
	}

	lock.Lock()
	defer lock.Unlock()

	if lgr, ok := loggersByModules[module]; ok {
		return lgr
	}

	// Logger doesn't exist, create a new one
	lgr := flogging.MustGetLogger(module)
	loggersByModules[module] = lgr
	return lgr
}

// SetupTestLogging sets the default log levels for gossip unit tests
func SetupTestLogging() {
	testMode = true
	flogging.InitFromSpec(defaultTestSpec)
}
