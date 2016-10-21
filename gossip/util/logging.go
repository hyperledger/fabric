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
	"github.com/op/go-logging"
	"os"
	"sync"
)

const (
	LOGGING_MESSAGE_BUFF_MODULE = "mbuff"
	LOGGING_EMITTER_MODULE      = "emitter"
	LOGGING_GOSMEMBER_MODULE    = "gossip"
	LOGGING_DISCOVERY_MODULE    = "discovery"
)

var loggersByModules = make(map[string]*Logger)
var defaultLevel = logging.WARNING
var lock = sync.Mutex{}

var format = logging.MustStringFormatter(
	`%{color}%{level} %{longfunc}():%{color:reset}(%{module})%{message}`,
)

func init() {
	logging.SetFormatter(format)
}

type Logger struct {
	logger logging.Logger
	module string
}

func SetDefaultFormat(formatStr string) {
	format = logging.MustStringFormatter(formatStr)
}

func SetDefaultLoggingLevel(level logging.Level) {
	defaultLevel = level
}

func (l *Logger) SetLevel(lvl logging.Level) {
	logging.SetLevel(lvl, l.module)
}

func GetLogger(module string, peerId string) *Logger {
	module = module + "-" + peerId
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
	lgr.logger = *logging.MustGetLogger(module)
	lgr.logger.ExtraCalldepth++
	lgr.module = module
	loggersByModules[module] = lgr
	return lgr
}

func (l *Logger) Fatal(args ...interface{}) {
	lock.Lock()
	defer lock.Unlock()
	l.logger.Fatal(args)
}

func (l *Logger) Fatalf(format string, args ...interface{}) {
	lock.Lock()
	defer lock.Unlock()
	l.logger.Fatalf(format, args)
}

func (l *Logger) Panic(args ...interface{}) {
	lock.Lock()
	defer lock.Unlock()
	l.logger.Panic(args)
}

func (l *Logger) Panicf(format string, args ...interface{}) {
	lock.Lock()
	defer lock.Unlock()
	l.logger.Panicf(format, args)
}

func (l *Logger) Critical(args ...interface{}) {
	lock.Lock()
	defer lock.Unlock()
	l.logger.Critical(args)
}

func (l *Logger) Criticalf(format string, args ...interface{}) {
	lock.Lock()
	defer lock.Unlock()
	l.logger.Criticalf(format, args)
}

func (l *Logger) Error(args ...interface{}) {
	lock.Lock()
	defer lock.Unlock()
	l.logger.Error(args)
}

func (l *Logger) Errorf(format string, args ...interface{}) {
	lock.Lock()
	defer lock.Unlock()
	l.logger.Errorf(format, args)
}

func (l *Logger) Warning(args ...interface{}) {
	lock.Lock()
	defer lock.Unlock()
	l.logger.Warning(args)
}

func (l *Logger) Warningf(format string, args ...interface{}) {
	lock.Lock()
	defer lock.Unlock()
	l.logger.Warningf(format, args)
}

func (l *Logger) Notice(args ...interface{}) {
	lock.Lock()
	defer lock.Unlock()
	l.logger.Notice(args)
}

func (l *Logger) Noticef(format string, args ...interface{}) {
	lock.Lock()
	defer lock.Unlock()
	l.logger.Noticef(format, args)
}

func (l *Logger) Info(args ...interface{}) {
	lock.Lock()
	defer lock.Unlock()
	l.logger.Info(args)
}

func (l *Logger) Infof(format string, args ...interface{}) {
	lock.Lock()
	defer lock.Unlock()
	l.logger.Infof(format, args)
}

func (l *Logger) Debug(args ...interface{}) {
	lock.Lock()
	defer lock.Unlock()
	l.logger.Debug(args)
}

func (l *Logger) Debugf(format string, args ...interface{}) {
	lock.Lock()
	defer lock.Unlock()
	l.logger.Debugf(format, args)
}
