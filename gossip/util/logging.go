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
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"sync"

	"github.com/op/go-logging"
	"google.golang.org/grpc/grpclog"
)

const (
	// LoggingMessageBuffModule defines the module for logging message buffer
	LoggingMessageBuffModule = "mbuff"
	// LoggingEmitterModule defines the module for logging emitter
	LoggingEmitterModule = "emitter"
	// LoggingGossipModule defines the module for logging gossip
	LoggingGossipModule = "gossip"
	// LoggingDiscoveryModule defines the module for logging discovery
	LoggingDiscoveryModule = "discovery"
	// LoggingCommModule defines the module for logging communication
	LoggingCommModule = "comm"
)

var loggersByModules = make(map[string]*Logger)
var defaultLevel = logging.WARNING
var lock = sync.Mutex{}

var format = logging.MustStringFormatter(
	`%{color} %{level} %{longfunc}():%{color:reset}(%{module})%{message}`,
)

func init() {
	logging.SetFormatter(format)
	grpclog.SetLogger(log.New(ioutil.Discard, "", 0))
}

// Logger defines a logger for gossip
type Logger struct {
	logging.Logger
	module string
}

// SetDefaultFormat sets the formatter for the gossip logger
func SetDefaultFormat(formatStr string) {
	format = logging.MustStringFormatter(formatStr)
}

// SetDefaultLoggingLevel sets the default logging level for the gossip logger
func SetDefaultLoggingLevel(level logging.Level) {
	defaultLevel = level
}

// SetLevel sets the level for the logger
func (l *Logger) SetLevel(lvl logging.Level) {
	logging.SetLevel(lvl, l.module)
}

// GetLogger returns a logger for given gossip module and peerID
func GetLogger(module string, peerID string) *Logger {
	module = module + "-" + peerID
	lock.Lock()
	defer lock.Unlock()

	if lgr, ok := loggersByModules[module]; ok {
		return lgr
	}

	// Logger doesn't exist, create a new one

	lvl, err := logging.LogLevel(defaultLevel.String())
	// Shouldn't happen, since setting default logging level validity
	// is checked in compile-time
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid default logging level: %v\n", err)
		return nil
	}
	logging.SetLevel(lvl, module)
	lgr := &Logger{}
	lgr.Logger = *logging.MustGetLogger(module)
	lgr.module = module
	loggersByModules[module] = lgr
	return lgr
}
