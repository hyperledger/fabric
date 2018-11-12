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

var loggers = make(map[string]Logger)
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

// SetupTestLogging sets the default log levels for gossip unit tests
func SetupTestLogging() {
	testMode = true
	flogging.InitFromSpec(defaultTestSpec)
}
