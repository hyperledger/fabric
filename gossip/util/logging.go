/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
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
	LoggingPrivModule      = "gossip/privdata"
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
