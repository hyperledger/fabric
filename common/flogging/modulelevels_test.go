/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package flogging_test

import (
	"regexp"
	"testing"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zapcore"
)

func TestModuleLevels(t *testing.T) {
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

	ml.Reset()
	ml.SetLevel("module-three", zapcore.ErrorLevel)
	assert.Equal(t, map[string]zapcore.Level{"module-three": zapcore.ErrorLevel}, ml.Levels())

	ml.RestoreLevels(levels)
	levels = ml.Levels()
	assert.Equal(t, map[string]zapcore.Level{"module-one": zapcore.DebugLevel, "module-two": zapcore.DebugLevel}, levels)
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
