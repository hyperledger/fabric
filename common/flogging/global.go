/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package flogging

import (
	"strings"

	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc/grpclog"
)

const (
	defaultFormat = "%{color}%{time:2006-01-02 15:04:05.000 MST} [%{module}] %{shortfunc} -> %{level:.4s} %{id:03x}%{color:reset} %{message}"
	defaultLevel  = zapcore.InfoLevel
)

var Global *Logging
var logger *FabricLogger

func init() {
	logging, err := New(Config{})
	if err != nil {
		panic(err)
	}

	Global = logging
	logger = Global.Logger("flogging")
	grpcLogger := Global.ZapLogger("grpc")
	grpclog.SetLogger(NewGRPCLogger(grpcLogger))
}

// Init initializes logging with the provided config.
func Init(config Config) {
	err := Global.Apply(config)
	if err != nil {
		panic(err)
	}
}

// Reset sets logging to the defaults defined in this package.
//
// Used in tests and in the package init
func Reset() {
	Global.Apply(Config{})
}

// GetModuleLevel gets the current logging level for the specified module.
func GetModuleLevel(module string) string {
	return strings.ToUpper(Global.Level(module).String())
}

// MustGetLogger is used in place of `logging.MustGetLogger` to allow us to
// store a map of all modules and submodules that have loggers in the logging.
func MustGetLogger(module string) *FabricLogger {
	return Global.Logger(module)
}

// ActivateSpec is used to acivate a logging specification.
func ActivateSpec(spec string) {
	err := Global.ActivateSpec(spec)
	if err != nil {
		panic(err)
	}
}
