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

package flogging_test

import (
	"os"
	"testing"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/op/go-logging"
	"github.com/spf13/viper"
)

func TestLevelDefault(t *testing.T) {
	viper.Reset()

	flogging.InitFromViper("")

	assertDefaultLevel(t, flogging.DefaultLevel())
}

func TestLevelOtherThanDefault(t *testing.T) {
	viper.Reset()
	viper.Set("logging_level", "warning")

	flogging.InitFromViper("")

	assertDefaultLevel(t, logging.WARNING)
}

func TestLevelForSpecificModule(t *testing.T) {
	viper.Reset()
	viper.Set("logging_level", "core=info")

	flogging.InitFromViper("")

	assertModuleLevel(t, "core", logging.INFO)
}

func TestLeveltForMultipleModules(t *testing.T) {
	viper.Reset()
	viper.Set("logging_level", "core=warning:test=debug")

	flogging.InitFromViper("")

	assertModuleLevel(t, "core", logging.WARNING)
	assertModuleLevel(t, "test", logging.DEBUG)
}

func TestLevelForMultipleModulesAtSameLevel(t *testing.T) {
	viper.Reset()
	viper.Set("logging_level", "core,test=warning")

	flogging.InitFromViper("")

	assertModuleLevel(t, "core", logging.WARNING)
	assertModuleLevel(t, "test", logging.WARNING)
}

func TestLevelForModuleWithDefault(t *testing.T) {
	viper.Reset()
	viper.Set("logging_level", "info:test=warning")

	flogging.InitFromViper("")

	assertDefaultLevel(t, logging.INFO)
	assertModuleLevel(t, "test", logging.WARNING)
}

func TestLevelForModuleWithDefaultAtEnd(t *testing.T) {
	viper.Reset()
	viper.Set("logging_level", "test=warning:info")

	flogging.InitFromViper("")

	assertDefaultLevel(t, logging.INFO)
	assertModuleLevel(t, "test", logging.WARNING)
}

func TestLevelForSpecificCommand(t *testing.T) {
	viper.Reset()
	viper.Set("logging.node", "error")

	flogging.InitFromViper("node")

	assertDefaultLevel(t, logging.ERROR)
}

func TestLevelForUnknownCommandGoesToDefault(t *testing.T) {
	viper.Reset()

	flogging.InitFromViper("unknown command")

	assertDefaultLevel(t, flogging.DefaultLevel())
}

func TestLevelInvalid(t *testing.T) {
	viper.Reset()
	viper.Set("logging_level", "invalidlevel")

	flogging.InitFromViper("")

	assertDefaultLevel(t, flogging.DefaultLevel())
}

func TestLevelInvalidModules(t *testing.T) {
	viper.Reset()
	viper.Set("logging_level", "core=invalid")

	flogging.InitFromViper("")

	assertDefaultLevel(t, flogging.DefaultLevel())
}

func TestLevelInvalidEmptyModule(t *testing.T) {
	viper.Reset()
	viper.Set("logging_level", "=warning")

	flogging.InitFromViper("")

	assertDefaultLevel(t, flogging.DefaultLevel())
}

func TestLevelInvalidModuleSyntax(t *testing.T) {
	viper.Reset()
	viper.Set("logging_level", "type=warn=again")

	flogging.InitFromViper("")

	assertDefaultLevel(t, flogging.DefaultLevel())
}

func TestGetModuleLevelDefault(t *testing.T) {
	level, _ := flogging.GetModuleLevel("peer")

	// peer should be using the default log level at this point
	if level != "INFO" {
		t.FailNow()
	}
}

func TestGetModuleLevelDebug(t *testing.T) {
	flogging.SetModuleLevel("peer", "DEBUG")
	level, _ := flogging.GetModuleLevel("peer")

	// ensure that the log level has changed to debug
	if level != "DEBUG" {
		t.FailNow()
	}
}

func TestGetModuleLevelInvalid(t *testing.T) {
	flogging.SetModuleLevel("peer", "invalid")
	level, _ := flogging.GetModuleLevel("peer")

	// ensure that the log level didn't change after invalid log level specified
	if level != "DEBUG" {
		t.FailNow()
	}
}

func TestSetModuleLevel(t *testing.T) {
	flogging.SetModuleLevel("peer", "WARNING")

	// ensure that the log level has changed to warning
	assertModuleLevel(t, "peer", logging.WARNING)
}

func TestSetModuleLevelInvalid(t *testing.T) {
	flogging.SetModuleLevel("peer", "invalid")

	// ensure that the log level didn't change after invalid log level specified
	assertModuleLevel(t, "peer", logging.WARNING)
}

func ExampleSetLoggingFormat() {
	// initializes logging backend for testing and sets
	// time to 1970-01-01 00:00:00.000 UTC
	logging.InitForTesting(flogging.DefaultLevel())

	logFormat := "%{time:2006-01-02 15:04:05.000 MST} [%{module}] %{shortfunc} -> %{level:.4s} %{id:03x} %{message}"
	flogging.SetLoggingFormat(logFormat, os.Stdout)

	logger := logging.MustGetLogger("floggingTest")
	logger.Infof("test")

	// Output:
	// 1970-01-01 00:00:00.000 UTC [floggingTest] ExampleSetLoggingFormat -> INFO 001 test
}

func ExampleSetLoggingFormat_second() {
	// initializes logging backend for testing and sets
	// time to 1970-01-01 00:00:00.000 UTC
	logging.InitForTesting(flogging.DefaultLevel())

	logFormat := "%{time:15:04:05.000} [%{module}] %{shortfunc} -> %{level:.4s} %{id:03x} %{message}"
	flogging.SetLoggingFormat(logFormat, os.Stdout)

	logger := logging.MustGetLogger("floggingTest")
	logger.Infof("test")

	// Output:
	// 00:00:00.000 [floggingTest] ExampleSetLoggingFormat_second -> INFO 001 test
}

func assertDefaultLevel(t *testing.T, expectedLevel logging.Level) {
	assertModuleLevel(t, "", expectedLevel)
}

func assertModuleLevel(t *testing.T, module string, expectedLevel logging.Level) {
	assertEquals(t, expectedLevel, logging.GetLevel(module))
}

func assertEquals(t *testing.T, expected interface{}, actual interface{}) {
	if expected != actual {
		t.Errorf("Expected: %v, Got: %v", expected, actual)
	}
}
