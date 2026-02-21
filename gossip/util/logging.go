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

// Logger names for logger initialization.
const (
	ChannelLogger     = "gossip.channel"
	CommLogger        = "gossip.comm"
	DiscoveryLogger   = "gossip.discovery"
	ElectionLogger    = "gossip.election"
	GossipLogger      = "gossip.gossip"
	CommMockLogger    = "gossip.comm.mock"
	PullLogger        = "gossip.pull"
	ServiceLogger     = "gossip.service"
	StateLogger       = "gossip.state"
	PrivateDataLogger = "gossip.privdata"
)

var (
	loggers  = make(map[string]Logger)
	lock     = sync.Mutex{}
	testMode bool
)

// defaultTestSpec is the default logging level for gossip tests
var defaultTestSpec = "WARNING"

type Logger interface {
	Debug(args ...any)
	Debugf(format string, args ...any)
	Error(args ...any)
	Errorf(format string, args ...any)
	Fatal(args ...any)
	Fatalf(format string, args ...any)
	Info(args ...any)
	Infof(format string, args ...any)
	Panic(args ...any)
	Panicf(format string, args ...any)
	Warning(args ...any)
	Warningf(format string, args ...any)
	IsEnabledFor(l zapcore.Level) bool
	With(args ...any) *flogging.FabricLogger
}

// GetLogger returns a logger for given gossip logger name and peerID
func GetLogger(name string, peerID string) Logger {
	if peerID != "" && testMode {
		name = name + "#" + peerID
	}

	lock.Lock()
	defer lock.Unlock()

	if lgr, ok := loggers[name]; ok {
		return lgr
	}

	// Logger doesn't exist, create a new one
	lgr := flogging.MustGetLogger(name)
	loggers[name] = lgr
	return lgr
}

// SetupTestLogging sets the default log levels for gossip unit tests to defaultTestSpec
func SetupTestLogging() {
	SetupTestLoggingWithLevel(defaultTestSpec)
}

// SetupTestLoggingWithLevel sets the default log levels for gossip unit tests to level
func SetupTestLoggingWithLevel(level string) {
	testMode = true
	flogging.ActivateSpec(level)
}
