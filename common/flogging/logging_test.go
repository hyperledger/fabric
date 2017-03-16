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
	assertEquals(t, logging.INFO.String(), level)

}

func TestGetModuleLevelDebug(t *testing.T) {
	flogging.SetModuleLevel("peer", "DEBUG")
	level, _ := flogging.GetModuleLevel("peer")

	// ensure that the log level has changed to debug
	assertEquals(t, logging.DEBUG.String(), level)
}

func TestGetModuleLevelInvalid(t *testing.T) {
	flogging.SetModuleLevel("peer", "invalid")
	level, _ := flogging.GetModuleLevel("peer")

	// ensure that the log level didn't change after invalid log level specified
	assertEquals(t, logging.DEBUG.String(), level)
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

// TestSetModuleLevel_moduleWithSubmodule and the following tests use the new
// flogging.MustGetLogger to initialize their loggers. flogging.MustGetLogger
// adds a modules map to track which module names have been defined and their
// current log level
func TestSetModuleLevel_moduleWithSubmodule(t *testing.T) {
	// enable setting the level by regular expressions
	// TODO - remove this once all modules have been updated to use
	// flogging.MustGetLogger
	flogging.IsSetLevelByRegExpEnabled = true

	flogging.MustGetLogger("module")
	flogging.MustGetLogger("module/subcomponent")
	flogging.MustGetLogger("sub")

	// ensure that the log level is the default level, INFO
	assertModuleLevel(t, "module", logging.INFO)
	assertModuleLevel(t, "module/subcomponent", logging.INFO)
	assertModuleLevel(t, "sub", logging.INFO)

	// set level for modules that contain 'module'
	flogging.SetModuleLevel("module", "warning")

	// ensure that the log level has changed to WARNING for the modules
	// that match the regular expression
	assertModuleLevel(t, "module", logging.WARNING)
	assertModuleLevel(t, "module/subcomponent", logging.WARNING)
	assertModuleLevel(t, "sub", logging.INFO)

	flogging.IsSetLevelByRegExpEnabled = false
}

func TestSetModuleLevel_regExpOr(t *testing.T) {
	flogging.IsSetLevelByRegExpEnabled = true

	flogging.MustGetLogger("module2")
	flogging.MustGetLogger("module2/subcomponent")
	flogging.MustGetLogger("sub2")
	flogging.MustGetLogger("randomLogger2")

	// ensure that the log level is the default level, INFO
	assertModuleLevel(t, "module2", logging.INFO)
	assertModuleLevel(t, "module2/subcomponent", logging.INFO)
	assertModuleLevel(t, "sub2", logging.INFO)
	assertModuleLevel(t, "randomLogger2", logging.INFO)

	// set level for modules that contain 'mod' OR 'sub'
	flogging.SetModuleLevel("mod|sub", "DEBUG")

	// ensure that the log level has changed to DEBUG for the modules that match
	// the regular expression
	assertModuleLevel(t, "module2", logging.DEBUG)
	assertModuleLevel(t, "module2/subcomponent", logging.DEBUG)
	assertModuleLevel(t, "sub2", logging.DEBUG)
	assertModuleLevel(t, "randomLogger2", logging.INFO)

	flogging.IsSetLevelByRegExpEnabled = false
}

func TestSetModuleLevel_regExpSuffix(t *testing.T) {
	flogging.IsSetLevelByRegExpEnabled = true

	flogging.MustGetLogger("module3")
	flogging.MustGetLogger("module3/subcomponent")
	flogging.MustGetLogger("sub3")
	flogging.MustGetLogger("sub3/subcomponent")
	flogging.MustGetLogger("randomLogger3")

	// ensure that the log level is the default level, INFO
	assertModuleLevel(t, "module3", logging.INFO)
	assertModuleLevel(t, "module3/subcomponent", logging.INFO)
	assertModuleLevel(t, "sub3", logging.INFO)
	assertModuleLevel(t, "sub3/subcomponent", logging.INFO)
	assertModuleLevel(t, "randomLogger3", logging.INFO)

	// set level for modules that contain component
	flogging.SetModuleLevel("component$", "ERROR")

	// ensure that the log level has changed to ERROR for the modules that match
	// the regular expression
	assertModuleLevel(t, "module3", logging.INFO)
	assertModuleLevel(t, "module3/subcomponent", logging.ERROR)
	assertModuleLevel(t, "sub3", logging.INFO)
	assertModuleLevel(t, "sub3/subcomponent", logging.ERROR)
	assertModuleLevel(t, "randomLogger3", logging.INFO)

	flogging.IsSetLevelByRegExpEnabled = false
}

func TestSetModuleLevel_regExpComplex(t *testing.T) {
	flogging.IsSetLevelByRegExpEnabled = true

	flogging.MustGetLogger("gossip/util")
	flogging.MustGetLogger("orderer/util")
	flogging.MustGetLogger("gossip/gossip#0.0.0.0:7051")
	flogging.MustGetLogger("gossip/conn#-1")
	flogging.MustGetLogger("orderer/conn#0.0.0.0:7051")

	// ensure that the log level is the default level, INFO
	assertModuleLevel(t, "gossip/util", logging.INFO)
	assertModuleLevel(t, "orderer/util", logging.INFO)
	assertModuleLevel(t, "gossip/gossip#0.0.0.0:7051", logging.INFO)
	assertModuleLevel(t, "gossip/conn#-1", logging.INFO)
	assertModuleLevel(t, "orderer/conn#0.0.0.0:7051", logging.INFO)

	// set level for modules that match the regular rexpression
	flogging.SetModuleLevel("^[a-z]+\\/[a-z]+#.+$", "WARNING")

	// ensure that the log level has changed to WARNING for the modules that match
	// the regular expression
	assertModuleLevel(t, "gossip/util", logging.INFO)
	assertModuleLevel(t, "orderer/util", logging.INFO)
	assertModuleLevel(t, "gossip/gossip#0.0.0.0:7051", logging.WARNING)
	assertModuleLevel(t, "gossip/conn#-1", logging.WARNING)
	assertModuleLevel(t, "orderer/conn#0.0.0.0:7051", logging.WARNING)

	flogging.IsSetLevelByRegExpEnabled = false
}

func TestSetModuleLevel_invalidRegExp(t *testing.T) {
	flogging.IsSetLevelByRegExpEnabled = true

	flogging.MustGetLogger("(module")

	// ensure that the log level is the default level, INFO
	assertModuleLevel(t, "(module", logging.INFO)

	level, err := flogging.SetModuleLevel("(", "info")

	assertEquals(t, "", level)
	if err == nil {
		t.FailNow()
	}

	// ensure that the log level hasn't changed
	assertModuleLevel(t, "(module", logging.INFO)

	flogging.IsSetLevelByRegExpEnabled = false
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
