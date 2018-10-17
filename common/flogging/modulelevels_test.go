/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package flogging_test

import (
	"errors"
	"testing"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zapcore"
)

func TestModuleLevelsActivateSpec(t *testing.T) {
	var tests = []struct {
		spec                 string
		expectedLevels       map[string]zapcore.Level
		expectedDefaultLevel zapcore.Level
	}{
		{
			spec:                 "DEBUG",
			expectedLevels:       map[string]zapcore.Level{},
			expectedDefaultLevel: zapcore.DebugLevel,
		},
		{
			spec:                 "INFO",
			expectedLevels:       map[string]zapcore.Level{},
			expectedDefaultLevel: zapcore.InfoLevel,
		},
		{
			spec: "logger=info:DEBUG",
			expectedLevels: map[string]zapcore.Level{
				"logger":     zapcore.InfoLevel,
				"logger.a":   zapcore.InfoLevel,
				"logger.b":   zapcore.InfoLevel,
				"logger.a.b": zapcore.InfoLevel,
			},
			expectedDefaultLevel: zapcore.DebugLevel,
		},
		{
			spec: "logger=info:logger.=error:DEBUG",
			expectedLevels: map[string]zapcore.Level{
				"logger":     zapcore.ErrorLevel,
				"logger.a":   zapcore.InfoLevel,
				"logger.b":   zapcore.InfoLevel,
				"logger.a.b": zapcore.InfoLevel,
			},
			expectedDefaultLevel: zapcore.DebugLevel,
		},
		{
			spec: "logger.a,logger.b=info:logger.c=WARN:DEBUG",
			expectedLevels: map[string]zapcore.Level{
				"logger.a": zapcore.InfoLevel,
				"logger.b": zapcore.InfoLevel,
				"logger.c": zapcore.WarnLevel,
			},
			expectedDefaultLevel: zapcore.DebugLevel,
		},
		{
			spec: "a.b=info:a,z=error:c.b=info:c.=warn:debug",
			expectedLevels: map[string]zapcore.Level{
				"a":       zapcore.ErrorLevel,
				"z":       zapcore.ErrorLevel,
				"a.b":     zapcore.InfoLevel,
				"a.b.c":   zapcore.InfoLevel,
				"a.b.c.d": zapcore.InfoLevel,
				"a.c":     zapcore.ErrorLevel,
				"c":       zapcore.WarnLevel,
				"c.a":     zapcore.DebugLevel,
				"c.b":     zapcore.InfoLevel,
				"d":       zapcore.DebugLevel,
				"ab.c":    zapcore.DebugLevel,
				"c.b.a":   zapcore.InfoLevel,
				"c.b.a.b": zapcore.InfoLevel,
			},
			expectedDefaultLevel: zapcore.DebugLevel,
		},
		{
			spec: "info:warn",
			expectedLevels: map[string]zapcore.Level{
				"a":   zapcore.WarnLevel,
				"a.b": zapcore.WarnLevel,
				"b":   zapcore.WarnLevel,
				"c":   zapcore.WarnLevel,
				"d":   zapcore.WarnLevel,
			},
			expectedDefaultLevel: zapcore.WarnLevel,
		},
	}

	for _, tc := range tests {
		t.Run(tc.spec, func(t *testing.T) {
			ml := &flogging.ModuleLevels{}

			err := ml.ActivateSpec(tc.spec)
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedDefaultLevel, ml.DefaultLevel())
			for name, lvl := range tc.expectedLevels {
				assert.Equal(t, lvl, ml.Level(name))
			}
		})
	}
}

func TestModuleLevelsActivateSpecErrors(t *testing.T) {
	var tests = []struct {
		spec string
		err  error
	}{
		{spec: "=INFO:DEBUG", err: errors.New("invalid logging specification '=INFO:DEBUG': no logger specified in segment '=INFO'")},
		{spec: "=INFO=:DEBUG", err: errors.New("invalid logging specification '=INFO=:DEBUG': bad segment '=INFO='")},
		{spec: "bogus", err: errors.New("invalid logging specification 'bogus': bad segment 'bogus'")},
		{spec: "a.b=info:a=broken:c.b=info:c.=warn:debug", err: errors.New("invalid logging specification 'a.b=info:a=broken:c.b=info:c.=warn:debug': bad segment 'a=broken'")},
	}
	for _, tc := range tests {
		t.Run(tc.spec, func(t *testing.T) {
			ml := &flogging.ModuleLevels{}
			err := ml.ActivateSpec("fatal:a=warn")

			err = ml.ActivateSpec(tc.spec)
			assert.EqualError(t, err, tc.err.Error())

			assert.Equal(t, zapcore.FatalLevel, ml.DefaultLevel(), "default should not change")
			assert.Equal(t, zapcore.WarnLevel, ml.Level("a.b"), "log levels should not change")
		})
	}
}

func TestModuleLevelsEnabler(t *testing.T) {
	ml := &flogging.ModuleLevels{}
	err := ml.ActivateSpec("logger=error")
	assert.NoError(t, err)

	for _, name := range []string{"logger.one.two", "logger.one", "logger"} {
		enabler := ml.LevelEnabler(name)
		assert.False(t, enabler.Enabled(zapcore.DebugLevel))
		assert.False(t, enabler.Enabled(zapcore.InfoLevel))
		assert.False(t, enabler.Enabled(zapcore.WarnLevel))
		assert.True(t, enabler.Enabled(zapcore.ErrorLevel))
		assert.True(t, enabler.Enabled(zapcore.DPanicLevel))
		assert.True(t, enabler.Enabled(zapcore.PanicLevel))
		assert.True(t, enabler.Enabled(zapcore.FatalLevel))
	}
}

func TestModuleLevelSpec(t *testing.T) {
	var tests = []struct {
		input  string
		output string
	}{
		{input: "", output: "info"},
		{input: "debug", output: "debug"},
		{input: "debug:a=info:b=warn", output: "a=info:b=warn:debug"},
		{input: "b=warn:a=error", output: "a=error:b=warn:info"},
	}

	for _, tc := range tests {
		ml := &flogging.ModuleLevels{}
		err := ml.ActivateSpec(tc.input)
		assert.NoError(t, err)

		assert.Equal(t, tc.output, ml.Spec())
	}
}
