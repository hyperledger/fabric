/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package flogging_test

import (
	"bytes"
	"testing"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/stretchr/testify/assert"
)

func TestGlobalReset(t *testing.T) {
	flogging.Reset()
	flogging.SetModuleLevel("module", "DEBUG")
	flogging.Global.SetFormat("json")

	system, err := flogging.New(flogging.Config{})
	assert.NoError(t, err)
	assert.NotEqual(t, flogging.Global.ModuleLevels, system.ModuleLevels)
	assert.NotEqual(t, flogging.Global.Encoding(), system.Encoding())

	flogging.Reset()
	assert.Equal(t, flogging.Global.ModuleLevels, system.ModuleLevels)
	assert.Equal(t, flogging.Global.Encoding(), system.Encoding())
}

func TestGlobalInitConsole(t *testing.T) {
	flogging.Reset()
	defer flogging.Reset()

	buf := &bytes.Buffer{}
	flogging.Init(flogging.Config{
		Format:  "%{message}",
		LogSpec: "DEBUG",
		Writer:  buf,
	})

	logger := flogging.MustGetLogger("testlogger")
	logger.Debug("this is a message")

	assert.Equal(t, "this is a message\n", buf.String())
}

func TestGlobalInitJSON(t *testing.T) {
	flogging.Reset()
	defer flogging.Reset()

	buf := &bytes.Buffer{}
	flogging.Init(flogging.Config{
		Format:  "json",
		LogSpec: "DEBUG",
		Writer:  buf,
	})

	logger := flogging.MustGetLogger("testlogger")
	logger.Debug("this is a message")

	assert.Regexp(t, `{"level":"debug","ts":\d+.\d+,"name":"testlogger","caller":"flogging/global_test.go:\d+","msg":"this is a message"}\s+`, buf.String())
}

func TestGlobalInitPanic(t *testing.T) {
	flogging.Reset()
	defer flogging.Reset()

	assert.Panics(t, func() {
		flogging.Init(flogging.Config{
			Format: "%{color:evil}",
		})
	})
}

func TestGlobalGetAndRestoreLevels(t *testing.T) {
	flogging.Reset()

	flogging.SetModuleLevel("test-1", "DEBUG")
	flogging.SetModuleLevel("test-2", "ERROR")
	flogging.SetModuleLevel("test-3", "WARN")
	levels := flogging.GetModuleLevels()

	assert.Equal(t, "DEBUG", flogging.GetModuleLevel("test-1"))
	assert.Equal(t, "ERROR", flogging.GetModuleLevel("test-2"))
	assert.Equal(t, "WARN", flogging.GetModuleLevel("test-3"))

	flogging.Reset()
	assert.Equal(t, "INFO", flogging.GetModuleLevel("test-1"))
	assert.Equal(t, "INFO", flogging.GetModuleLevel("test-2"))
	assert.Equal(t, "INFO", flogging.GetModuleLevel("test-3"))

	flogging.RestoreLevels(levels)
	assert.Equal(t, "DEBUG", flogging.GetModuleLevel("test-1"))
	assert.Equal(t, "ERROR", flogging.GetModuleLevel("test-2"))
	assert.Equal(t, "WARN", flogging.GetModuleLevel("test-3"))
}

func TestGlobalDefaultLevel(t *testing.T) {
	flogging.Reset()

	assert.Equal(t, "INFO", flogging.DefaultLevel())
}

func TestGlobalSetModuleLevels(t *testing.T) {
	flogging.Reset()

	flogging.SetModuleLevel("a-module", "DEBUG")
	flogging.SetModuleLevel("another-module", "DEBUG")
	assert.Equal(t, "DEBUG", flogging.GetModuleLevel("a-module"))
	assert.Equal(t, "DEBUG", flogging.GetModuleLevel("another-module"))

	flogging.SetModuleLevels("^a-", "INFO")
	assert.Equal(t, "INFO", flogging.GetModuleLevel("a-module"))
	assert.Equal(t, "DEBUG", flogging.GetModuleLevel("another-module"))

	flogging.SetModuleLevels("module", "WARN")
	assert.Equal(t, "WARN", flogging.GetModuleLevel("a-module"))
	assert.Equal(t, "WARN", flogging.GetModuleLevel("another-module"))
}

func TestGlobalSetModuleLevelsBadRegex(t *testing.T) {
	flogging.Reset()

	err := flogging.SetModuleLevels("((", "DEBUG")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error parsing regexp: ")
}

func TestGlobalMustGetLogger(t *testing.T) {
	flogging.Reset()

	l := flogging.MustGetLogger("module-name")
	assert.NotNil(t, l)
}

func TestFlogginInitPanic(t *testing.T) {
	defer flogging.Reset()

	assert.Panics(t, func() {
		flogging.Init(flogging.Config{
			Format: "%{color:broken}",
		})
	})
}

func TestGlobalInitFromSpec(t *testing.T) {
	defer flogging.Reset()

	tests := []struct {
		name           string
		spec           string
		expectedResult string
		expectedLevels map[string]string
	}{
		{
			name:           "SingleModuleLevel",
			spec:           "a=info",
			expectedResult: "INFO",
			expectedLevels: map[string]string{"a": "INFO"},
		},
		{
			name:           "MultipleModulesMultipleLevels",
			spec:           "a=info:b=debug",
			expectedResult: "INFO",
			expectedLevels: map[string]string{"a": "INFO", "b": "DEBUG"},
		},
		{
			name:           "MultipleModulesSameLevel",
			spec:           "a,b=warning",
			expectedResult: "INFO",
			expectedLevels: map[string]string{"a": "WARN", "b": "WARN"},
		},
		{
			name:           "DefaultAndModules",
			spec:           "ERROR:a=warning",
			expectedResult: "ERROR",
			expectedLevels: map[string]string{"a": "WARN"},
		},
		{
			name:           "ModuleAndDefault",
			spec:           "a=debug:info",
			expectedResult: "INFO",
			expectedLevels: map[string]string{"a": "DEBUG"},
		},
		{
			name:           "EmptyModuleEqualsLevel",
			spec:           "=info",
			expectedResult: "INFO",
			expectedLevels: map[string]string{},
		},
		{
			name:           "InvalidSyntax",
			spec:           "a=b=c",
			expectedResult: "INFO",
			expectedLevels: map[string]string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			flogging.Reset()

			l := flogging.InitFromSpec(tc.spec)
			assert.Equal(t, tc.expectedResult, l)

			for k, v := range tc.expectedLevels {
				assert.Equal(t, v, flogging.GetModuleLevel(k))
			}
		})
	}
}
