/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package util

import (
	"sync"

	"github.com/hyperledger/fabric/common/flogging"
	"go.uber.org/zap/zapcore"
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

var loggersByModules = make(map[string]Logger)
var lock = sync.Mutex{}
var testMode bool

// defaultTestSpec is the default logging level for gossip tests
var defaultTestSpec = "WARNING"

type Logger interface {
	Debug(args ...interface{})
	Debugf(format string, args ...interface{})
	Error(args ...interface{})
	Errorf(format string, args ...interface{})
	Fatal(args ...interface{})
	Fatalf(format string, args ...interface{})
	Info(args ...interface{})
	Infof(format string, args ...interface{})
	Panic(args ...interface{})
	Panicf(format string, args ...interface{})
	Warning(args ...interface{})
	Warningf(format string, args ...interface{})
	IsEnabledFor(l zapcore.Level) bool
}

// GetLogger returns a logger for given gossip module and peerID
func GetLogger(module string, peerID string) Logger {
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
