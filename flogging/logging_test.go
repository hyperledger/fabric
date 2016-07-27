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
	"testing"

	"github.com/hyperledger/fabric/flogging"
	"github.com/op/go-logging"
	"github.com/spf13/viper"
)

func TestLoggingLevelDefault(t *testing.T) {
	viper.Reset()

	flogging.LoggingInit("")

	assertDefaultLoggingLevel(t, flogging.DefaultLoggingLevel())
}

func TestLoggingLevelOtherThanDefault(t *testing.T) {
	viper.Reset()
	viper.Set("logging_level", "warning")

	flogging.LoggingInit("")

	assertDefaultLoggingLevel(t, logging.WARNING)
}

func TestLoggingLevelForSpecificModule(t *testing.T) {
	viper.Reset()
	viper.Set("logging_level", "core=info")

	flogging.LoggingInit("")

	assertModuleLoggingLevel(t, "core", logging.INFO)
}

func TestLoggingLeveltForMultipleModules(t *testing.T) {
	viper.Reset()
	viper.Set("logging_level", "core=warning:test=debug")

	flogging.LoggingInit("")

	assertModuleLoggingLevel(t, "core", logging.WARNING)
	assertModuleLoggingLevel(t, "test", logging.DEBUG)
}

func TestLoggingLevelForMultipleModulesAtSameLevel(t *testing.T) {
	viper.Reset()
	viper.Set("logging_level", "core,test=warning")

	flogging.LoggingInit("")

	assertModuleLoggingLevel(t, "core", logging.WARNING)
	assertModuleLoggingLevel(t, "test", logging.WARNING)
}

func TestLoggingLevelForModuleWithDefault(t *testing.T) {
	viper.Reset()
	viper.Set("logging_level", "info:test=warning")

	flogging.LoggingInit("")

	assertDefaultLoggingLevel(t, logging.INFO)
	assertModuleLoggingLevel(t, "test", logging.WARNING)
}

func TestLoggingLevelForModuleWithDefaultAtEnd(t *testing.T) {
	viper.Reset()
	viper.Set("logging_level", "test=warning:info")

	flogging.LoggingInit("")

	assertDefaultLoggingLevel(t, logging.INFO)
	assertModuleLoggingLevel(t, "test", logging.WARNING)
}

func TestLoggingLevelForSpecificCommand(t *testing.T) {
	viper.Reset()
	viper.Set("logging.node", "error")

	flogging.LoggingInit("node")

	assertDefaultLoggingLevel(t, logging.ERROR)
}

func TestLoggingLevelForUnknownCommandGoesToDefault(t *testing.T) {
	viper.Reset()

	flogging.LoggingInit("unknown command")

	assertDefaultLoggingLevel(t, flogging.DefaultLoggingLevel())
}

func TestLoggingLevelInvalid(t *testing.T) {
	viper.Reset()
	viper.Set("logging_level", "invalidlevel")

	flogging.LoggingInit("")

	assertDefaultLoggingLevel(t, flogging.DefaultLoggingLevel())
}

func TestLoggingLevelInvalidModules(t *testing.T) {
	viper.Reset()
	viper.Set("logging_level", "core=invalid")

	flogging.LoggingInit("")

	assertDefaultLoggingLevel(t, flogging.DefaultLoggingLevel())
}

func TestLoggingLevelInvalidEmptyModule(t *testing.T) {
	viper.Reset()
	viper.Set("logging_level", "=warning")

	flogging.LoggingInit("")

	assertDefaultLoggingLevel(t, flogging.DefaultLoggingLevel())
}

func TestLoggingLevelInvalidModuleSyntax(t *testing.T) {
	viper.Reset()
	viper.Set("logging_level", "type=warn=again")

	flogging.LoggingInit("")

	assertDefaultLoggingLevel(t, flogging.DefaultLoggingLevel())
}

func assertDefaultLoggingLevel(t *testing.T, expectedLevel logging.Level) {
	assertModuleLoggingLevel(t, "", expectedLevel)
}

func assertModuleLoggingLevel(t *testing.T, module string, expectedLevel logging.Level) {
	assertEquals(t, expectedLevel, logging.GetLevel(module))
}

func assertEquals(t *testing.T, expected interface{}, actual interface{}) {
	if expected != actual {
		t.Errorf("Expected: %v, Got: %v", expected, actual)
	}
}
