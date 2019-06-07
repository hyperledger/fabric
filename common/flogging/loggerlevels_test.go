/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package flogging_test

import (
	"errors"
	"strconv"
	"testing"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zapcore"
)

func TestLoggerLevelsActivateSpec(t *testing.T) {
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
			ll := &flogging.LoggerLevels{}

			err := ll.ActivateSpec(tc.spec)
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedDefaultLevel, ll.DefaultLevel())
			for name, lvl := range tc.expectedLevels {
				assert.Equal(t, lvl, ll.Level(name))
			}
		})
	}
}

func TestLoggerLevelsActivateSpecErrors(t *testing.T) {
	var tests = []struct {
		spec string
		err  error
	}{
		{spec: "=INFO:DEBUG", err: errors.New("invalid logging specification '=INFO:DEBUG': no logger specified in segment '=INFO'")},
		{spec: "=INFO=:DEBUG", err: errors.New("invalid logging specification '=INFO=:DEBUG': bad segment '=INFO='")},
		{spec: "bogus", err: errors.New("invalid logging specification 'bogus': bad segment 'bogus'")},
		{spec: "a.b=info:a=broken:c.b=info:c.=warn:debug", err: errors.New("invalid logging specification 'a.b=info:a=broken:c.b=info:c.=warn:debug': bad segment 'a=broken'")},
		{spec: "a*=info:debug", err: errors.New("invalid logging specification 'a*=info:debug': bad logger name 'a*'")},
		{spec: ".a=info:debug", err: errors.New("invalid logging specification '.a=info:debug': bad logger name '.a'")},
	}
	for _, tc := range tests {
		t.Run(tc.spec, func(t *testing.T) {
			ll := &flogging.LoggerLevels{}
			err := ll.ActivateSpec("fatal:a=warn")

			err = ll.ActivateSpec(tc.spec)
			assert.EqualError(t, err, tc.err.Error())

			assert.Equal(t, zapcore.FatalLevel, ll.DefaultLevel(), "default should not change")
			assert.Equal(t, zapcore.WarnLevel, ll.Level("a.b"), "log levels should not change")
		})
	}
}

func TestSpec(t *testing.T) {
	var tests = []struct {
		input  string
		output string
	}{
		{input: "", output: "info"},
		{input: "debug", output: "debug"},
		{input: "a.=info:warning", output: "a.=info:warn"},
		{input: "a-b=error", output: "a-b=error:info"},
		{input: "a#b=error", output: "a#b=error:info"},
		{input: "a_b=error", output: "a_b=error:info"},
		{input: "debug:a=info:b=warn", output: "a=info:b=warn:debug"},
		{input: "b=warn:a=error", output: "a=error:b=warn:info"},
	}

	for _, tc := range tests {
		ll := &flogging.LoggerLevels{}
		err := ll.ActivateSpec(tc.input)
		assert.NoError(t, err)

		assert.Equal(t, tc.output, ll.Spec())
	}
}

func TestEnabled(t *testing.T) {
	var tests = []struct {
		spec      string
		enabledAt zapcore.Level
	}{
		{spec: "payload", enabledAt: flogging.PayloadLevel},
		{spec: "debug", enabledAt: zapcore.DebugLevel},
		{spec: "info", enabledAt: zapcore.InfoLevel},
		{spec: "warn", enabledAt: zapcore.WarnLevel},
		{spec: "panic", enabledAt: zapcore.PanicLevel},
		{spec: "fatal", enabledAt: zapcore.FatalLevel},
		{spec: "fatal:a=debug", enabledAt: zapcore.DebugLevel},
		{spec: "a=fatal:b=warn", enabledAt: zapcore.InfoLevel},
		{spec: "a=warn", enabledAt: zapcore.InfoLevel},
		{spec: "a=debug", enabledAt: zapcore.DebugLevel},
	}

	for i, tc := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			ll := &flogging.LoggerLevels{}
			err := ll.ActivateSpec(tc.spec)
			assert.NoError(t, err)

			for i := flogging.PayloadLevel; i <= zapcore.FatalLevel; i++ {
				if tc.enabledAt <= i {
					assert.Truef(t, ll.Enabled(i), "expected level %s and spec %s to be enabled", zapcore.Level(i), tc.spec)
				} else {
					assert.False(t, ll.Enabled(i), "expected level %s and spec %s to be disabled", zapcore.Level(i), tc.spec)
				}
			}
		})
	}
}
