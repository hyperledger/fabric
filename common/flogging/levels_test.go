/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package flogging_test

import (
	"testing"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zapcore"
)

func TestNameToLevel(t *testing.T) {
	var tests = []struct {
		names []string
		level zapcore.Level
	}{
		{names: []string{"DEBUG", "debug"}, level: zapcore.DebugLevel},
		{names: []string{"INFO", "info"}, level: zapcore.InfoLevel},
		{names: []string{"WARNING", "warning", "WARN", "warn"}, level: zapcore.WarnLevel},
		{names: []string{"ERROR", "error"}, level: zapcore.ErrorLevel},
		{names: []string{"DPANIC", "dpanic"}, level: zapcore.DPanicLevel},
		{names: []string{"PANIC", "panic"}, level: zapcore.PanicLevel},
		{names: []string{"FATAL", "fatal"}, level: zapcore.FatalLevel},
		{names: []string{"NOTICE", "notice"}, level: zapcore.InfoLevel},
		{names: []string{"CRITICAL", "critical"}, level: zapcore.ErrorLevel},
		{names: []string{"unexpected", "invalid"}, level: zapcore.InfoLevel},
	}

	for _, tc := range tests {
		for _, name := range tc.names {
			t.Run(name, func(t *testing.T) {
				assert.Equal(t, tc.level, flogging.NameToLevel(name))
			})
		}
	}
}

func TestIsValidLevel(t *testing.T) {
	validNames := []string{
		"DEBUG", "debug",
		"INFO", "info",
		"WARNING", "warning",
		"WARN", "warn",
		"ERROR", "error",
		"DPANIC", "dpanic",
		"PANIC", "panic",
		"FATAL", "fatal",
		"NOTICE", "notice",
		"CRITICAL", "critical",
	}
	for _, name := range validNames {
		t.Run(name, func(t *testing.T) {
			assert.True(t, flogging.IsValidLevel(name))
		})
	}

	invalidNames := []string{
		"george", "bob",
		"warnings", "inf",
	}
	for _, name := range invalidNames {
		t.Run(name, func(t *testing.T) {
			assert.False(t, flogging.IsValidLevel(name))
		})
	}
}
