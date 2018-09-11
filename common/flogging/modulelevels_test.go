/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package flogging_test

import (
	"errors"
	"regexp"
	"testing"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zapcore"
)

func TestModuleLevelsSetLevel(t *testing.T) {
	ml := &flogging.ModuleLevels{}

	lvl := ml.Level("module-name")
	assert.Equal(t, zapcore.InfoLevel, lvl)

	ml.SetDefaultLevel(zapcore.DebugLevel)
	lvl = ml.Level("module-name")
	assert.Equal(t, zapcore.DebugLevel, lvl)

	ml.SetLevel("module-name", zapcore.ErrorLevel)
	lvl = ml.Level("module-name")
	assert.Equal(t, zapcore.ErrorLevel, lvl)
}

func TestModuleLevelsDefaultLevel(t *testing.T) {
	ml := &flogging.ModuleLevels{}
	assert.Equal(t, zapcore.InfoLevel, ml.DefaultLevel())

	ml.SetDefaultLevel(zapcore.DebugLevel)
	assert.Equal(t, zapcore.DebugLevel, ml.DefaultLevel())
}

func TestModuleLevelsSetLevels(t *testing.T) {
	var tests = []struct {
		name           string
		regexp         *regexp.Regexp
		expectedLevels map[string]zapcore.Level
	}{
		{
			name:   "match-all",
			regexp: regexp.MustCompile("module"),
			expectedLevels: map[string]zapcore.Level{
				"module-one": zapcore.InfoLevel,
				"module-two": zapcore.InfoLevel,
			},
		},
		{
			name:   "match-none",
			regexp: regexp.MustCompile(regexp.QuoteMeta("....")),
			expectedLevels: map[string]zapcore.Level{
				"module-one": zapcore.DebugLevel,
				"module-two": zapcore.DebugLevel,
			},
		},
		{
			name:   "match-one",
			regexp: regexp.MustCompile("module-one"),
			expectedLevels: map[string]zapcore.Level{
				"module-one": zapcore.InfoLevel,
				"module-two": zapcore.DebugLevel,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ml := &flogging.ModuleLevels{}
			ml.SetLevel("module-one", zapcore.DebugLevel)
			ml.SetLevel("module-two", zapcore.DebugLevel)

			ml.SetLevels(tc.regexp, zapcore.InfoLevel)
			assert.Equal(t, tc.expectedLevels, ml.Levels())
		})
	}
}

func TestModuleLevelsRestoreLevels(t *testing.T) {
	ml := &flogging.ModuleLevels{}

	levels := ml.Levels()
	assert.NotNil(t, levels)
	assert.Empty(t, levels)

	ml.SetLevel("module-one", zapcore.DebugLevel)
	ml.SetLevel("module-two", zapcore.DebugLevel)

	levels = ml.Levels()
	assert.Equal(t, map[string]zapcore.Level{"module-one": zapcore.DebugLevel, "module-two": zapcore.DebugLevel}, levels)

	ml.ResetLevels()
	ml.SetLevel("module-three", zapcore.ErrorLevel)
	assert.Equal(t, map[string]zapcore.Level{"module-three": zapcore.ErrorLevel}, ml.Levels())

	ml.RestoreLevels(levels)
	levels = ml.Levels()
	assert.Equal(t, map[string]zapcore.Level{"module-one": zapcore.DebugLevel, "module-two": zapcore.DebugLevel}, levels)
}

func TestModuleLevelsActivateSpec(t *testing.T) {
	var tests = []struct {
		spec                 string
		err                  error
		initialLevels        map[string]zapcore.Level
		expectedLevels       map[string]zapcore.Level
		expectedDefaultLevel zapcore.Level
	}{
		{
			spec:                 "DEBUG",
			initialLevels:        map[string]zapcore.Level{},
			expectedLevels:       map[string]zapcore.Level{},
			expectedDefaultLevel: zapcore.DebugLevel,
		},
		{
			spec: "INFO",
			initialLevels: map[string]zapcore.Level{
				"module1": zapcore.DebugLevel,
				"module2": zapcore.WarnLevel,
			},
			expectedLevels: map[string]zapcore.Level{
				"module1": zapcore.InfoLevel,
				"module2": zapcore.InfoLevel,
			},
			expectedDefaultLevel: zapcore.InfoLevel,
		},
		{
			spec:          "module1=info:DEBUG",
			initialLevels: map[string]zapcore.Level{},
			expectedLevels: map[string]zapcore.Level{
				"module1": zapcore.InfoLevel,
			},
			expectedDefaultLevel: zapcore.DebugLevel,
		},
		{
			spec: "module1,module2=info:module3=WARN:DEBUG",
			initialLevels: map[string]zapcore.Level{
				"existing": zapcore.PanicLevel,
			},
			expectedLevels: map[string]zapcore.Level{
				"existing": zapcore.DebugLevel,
				"module1":  zapcore.InfoLevel,
				"module2":  zapcore.InfoLevel,
				"module3":  zapcore.WarnLevel,
			},
			expectedDefaultLevel: zapcore.DebugLevel,
		},
		{
			spec:                 "=INFO:DEBUG",
			err:                  errors.New("invalid logging specification '=INFO:DEBUG': no module specified in segment '=INFO'"),
			initialLevels:        map[string]zapcore.Level{},
			expectedLevels:       map[string]zapcore.Level{},
			expectedDefaultLevel: zapcore.DebugLevel,
		},
		{
			spec:                 "=INFO=:DEBUG",
			err:                  errors.New("invalid logging specification '=INFO=:DEBUG': bad segment '=INFO='"),
			initialLevels:        map[string]zapcore.Level{},
			expectedLevels:       map[string]zapcore.Level{},
			expectedDefaultLevel: zapcore.DebugLevel,
		},
		{
			spec: "error:existing,module1=debug:info",
			initialLevels: map[string]zapcore.Level{
				"existing": zapcore.PanicLevel,
			},
			expectedLevels: map[string]zapcore.Level{
				"existing": zapcore.DebugLevel,
				"module1":  zapcore.DebugLevel,
			},
		},
		{
			spec: "existing=debug:module1=panic",
			initialLevels: map[string]zapcore.Level{
				"existing": zapcore.PanicLevel,
			},
			expectedLevels: map[string]zapcore.Level{
				"existing": zapcore.DebugLevel,
				"module1":  zapcore.PanicLevel,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.spec, func(t *testing.T) {
			ml := &flogging.ModuleLevels{}
			ml.RestoreLevels(tc.initialLevels)

			err := ml.ActivateSpec(tc.spec)

			if tc.err == nil {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedDefaultLevel, ml.DefaultLevel())
				assert.Equal(t, tc.expectedLevels, ml.Levels())
			} else {
				assert.EqualError(t, err, tc.err.Error())
			}
		})
	}
}

func TestModuleLevelsEnabler(t *testing.T) {
	ml := &flogging.ModuleLevels{}
	ml.SetLevel("module-name", zapcore.ErrorLevel)

	enabler := ml.LevelEnabler("module-name")
	assert.False(t, enabler.Enabled(zapcore.DebugLevel))
	assert.False(t, enabler.Enabled(zapcore.InfoLevel))
	assert.False(t, enabler.Enabled(zapcore.WarnLevel))
	assert.True(t, enabler.Enabled(zapcore.ErrorLevel))
	assert.True(t, enabler.Enabled(zapcore.DPanicLevel))
	assert.True(t, enabler.Enabled(zapcore.PanicLevel))
	assert.True(t, enabler.Enabled(zapcore.FatalLevel))
}
