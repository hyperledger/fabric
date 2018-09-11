/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package flogging

import (
	"regexp"
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
	Global.ResetLevels()
	Global.Apply(Config{})
}

// GetModuleLevel gets the current logging level for the specified module.
func GetModuleLevel(module string) string {
	return strings.ToUpper(Global.Level(module).String())
}

// SetModuleLevels sets the logging level for the modules that match the
// supplied regular expression. Can be used to dynamically change the log level
// for the module.
func SetModuleLevels(moduleRegexp, level string) error {
	re, err := regexp.Compile(moduleRegexp)
	if err != nil {
		return err
	}

	Global.SetLevels(re, NameToLevel(level))
	return nil
}

// SetModuleLevel sets the logging level for a single module.
func SetModuleLevel(module string, level string) error {
	Global.SetLevel(module, NameToLevel(level))
	return nil
}

// MustGetLogger is used in place of `logging.MustGetLogger` to allow us to
// store a map of all modules and submodules that have loggers in the logging.
func MustGetLogger(module string) *FabricLogger {
	return Global.Logger(module)
}

// GetModuleLevels takes a snapshot of the global module level information and
// returns it as a map. The map can then be used to restore logging levels to
// values in the snapshot.
func GetModuleLevels() map[string]zapcore.Level {
	return Global.Levels()
}

// RestoreLevels sets the global module level information to the contents of the
// provided map.
func RestoreLevels(levels map[string]zapcore.Level) {
	Global.RestoreLevels(levels)
}
