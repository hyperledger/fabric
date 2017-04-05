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
	"github.com/stretchr/testify/assert"
)

const logLevelCount = 6

type testCase struct {
	name           string
	args           []string
	expectedLevels []string
	modules        []string
	withRegEx      bool
	shouldErr      bool
}

func TestGetModuleLevelDefault(t *testing.T) {
	assert.Equal(t, flogging.DefaultLevel(), flogging.GetModuleLevel("a"))
}

func TestSetModuleLevel(t *testing.T) {
	defer flogging.Reset()

	var tc []testCase

	tc = append(tc,
		testCase{"Valid", []string{"a", "warning"}, []string{"WARNING"}, []string{"a"}, false, false},
		// Same as before
		testCase{"Invalid", []string{"a", "foo"}, []string{"WARNING"}, []string{"a"}, false, false},
		// Tests with regular expressions
		testCase{"RegexModuleWithSubmodule", []string{"foo", "warning"}, []string{"WARNING", "WARNING", flogging.DefaultLevel()},
			[]string{"foo", "foo/bar", "baz"}, true, false},
		// Set the level for modules that contain "foo" or "baz"
		testCase{"RegexOr", []string{"foo|baz", "debug"}, []string{"DEBUG", "DEBUG", "DEBUG", flogging.DefaultLevel()},
			[]string{"foo", "foo/bar", "baz", "random"}, true, false},
		// Set the level for modules that end with "bar"
		testCase{"RegexSuffix", []string{"bar$", "error"}, []string{"ERROR", flogging.DefaultLevel()},
			[]string{"foo/bar", "bar/baz"}, true, false},
		testCase{"RegexComplex", []string{"^[a-z]+\\/[a-z]+#.+$", "warning"}, []string{flogging.DefaultLevel(), flogging.DefaultLevel(), "WARNING", "WARNING", "WARNING"},
			[]string{"gossip/util", "orderer/util", "gossip/gossip#0.0.0.0:7051", "gossip/conn#-1", "orderer/conn#0.0.0.0:7051"}, true, false},
		testCase{"RegexInvalid", []string{"(", "warning"}, []string{flogging.DefaultLevel()},
			[]string{"foo"}, true, true},
	)

	assert := assert.New(t)

	for i := 0; i < len(tc); i++ {
		t.Run(tc[i].name, func(t *testing.T) {
			if tc[i].withRegEx {
				for j := 0; j < len(tc[i].modules); j++ {
					flogging.MustGetLogger(tc[i].modules[j])
				}
				flogging.IsSetLevelByRegExpEnabled = true // enable for call below
			}

			_, err := flogging.SetModuleLevel(tc[i].args[0], tc[i].args[1])
			if tc[i].shouldErr {
				assert.NotNil(err, "Should have returned an error")
			}
			for k := 0; k < len(tc[i].expectedLevels); k++ {
				assert.Equal(tc[i].expectedLevels[k], flogging.GetModuleLevel(tc[i].modules[k]))
			}

			if tc[i].withRegEx {
				// Force reset (a) in case the next test is non-regex, (b) so as
				// to reset the modules map and reuse module names.
				flogging.Reset()

			}
		})
	}

}

func TestInitFromSpec(t *testing.T) {
	var tc []testCase

	// GLOBAL

	// all allowed log levels
	for i := 0; i < logLevelCount; i++ {
		level := logging.Level(i).String()
		tc = append(tc, testCase{
			name:           "Global" + level,
			args:           []string{level},
			expectedLevels: []string{level},
			modules:        []string{""},
		})
	}
	// NIL INPUT
	tc = append(tc, testCase{
		name:           "Global" + "NIL",
		args:           []string{""},
		expectedLevels: []string{flogging.DefaultLevel()},
		modules:        []string{""},
	})

	// MODULES

	tc = append(tc,
		testCase{"SingleModuleLevel", []string{"a=info"}, []string{"INFO"}, []string{"a"}, false, false},
		testCase{"MultipleModulesMultipleLevels", []string{"a=info:b=debug"}, []string{"INFO", "DEBUG"}, []string{"a", "b"}, false, false},
		testCase{"MultipleModulesSameLevel", []string{"a,b=warning"}, []string{"WARNING", "WARNING"}, []string{"a", "b"}, false, false},
	)

	// MODULES + DEFAULT

	tc = append(tc,
		testCase{"GlobalDefaultAndSingleModuleLevel", []string{"info:a=warning"}, []string{"INFO", "WARNING"}, []string{"", "a"}, false, false},
		testCase{"SingleModuleLevelAndGlobalDefaultAtEnd", []string{"a=warning:info"}, []string{"WARNING", "INFO"}, []string{"a", ""}, false, false},
	)

	// INVALID INPUT

	tc = append(tc,
		testCase{"InvalidLevel", []string{"foo"}, []string{flogging.DefaultLevel()}, []string{""}, false, false},
		testCase{"InvalidLevelForSingleModule", []string{"a=foo"}, []string{flogging.DefaultLevel()}, []string{""}, false, false},
		testCase{"EmptyModuleEqualsLevel", []string{"=warning"}, []string{flogging.DefaultLevel()}, []string{""}, false, false},
		testCase{"InvalidModuleSyntax", []string{"a=b=c"}, []string{flogging.DefaultLevel()}, []string{""}, false, false},
	)

	assert := assert.New(t)

	for i := 0; i < len(tc); i++ {
		t.Run(tc[i].name, func(t *testing.T) {
			defer flogging.Reset()
			flogging.InitFromSpec(tc[i].args[0])
			for j := 0; j < len(tc[i].expectedLevels); j++ {
				assert.Equal(tc[i].expectedLevels[j], flogging.GetModuleLevel(tc[i].modules[j]))
			}
		})
	}

}

func ExampleInitBackend() {
	level, _ := logging.LogLevel(flogging.DefaultLevel())
	// initializes logging backend for testing and sets time to 1970-01-01 00:00:00.000 UTC
	logging.InitForTesting(level)

	formatSpec := "%{time:2006-01-02 15:04:05.000 MST} [%{module}] %{shortfunc} -> %{level:.4s} %{id:03x} %{message}"
	flogging.InitBackend(flogging.SetFormat(formatSpec), os.Stdout)

	logger := logging.MustGetLogger("testModule")
	logger.Info("test output")

	// Output:
	// 1970-01-01 00:00:00.000 UTC [testModule] ExampleInitBackend -> INFO 001 test output
}
