/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package flogging

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/pkg/errors"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// ModuleLevels tracks the logging level of logging modules.
type ModuleLevels struct {
	defaultLevel zapcore.Level

	mutex      sync.RWMutex
	levelCache map[string]zapcore.Level
	specs      map[string]zapcore.Level
}

// DefaultLevel returns the default logging level for modules that do not have
// an explicit level set.
func (m *ModuleLevels) DefaultLevel() zapcore.Level {
	m.mutex.RLock()
	l := m.defaultLevel
	m.mutex.RUnlock()
	return l
}

// ActivateSpec is used to modify logging levels.
//
// The logging specification has the following form:
//   [<logger>[,<logger>...]=]<level>[:[<logger>[,<logger>...]=]<level>...]
func (m *ModuleLevels) ActivateSpec(spec string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	defaultLevel := zapcore.InfoLevel
	specs := map[string]zapcore.Level{}
	for _, field := range strings.Split(spec, ":") {
		split := strings.Split(field, "=")
		switch len(split) {
		case 1: // level
			if field != "" && !IsValidLevel(field) {
				return errors.Errorf("invalid logging specification '%s': bad segment '%s'", spec, field)
			}
			defaultLevel = NameToLevel(field)

		case 2: // <logger>[,<logger>...]=<level>
			if split[0] == "" {
				return errors.Errorf("invalid logging specification '%s': no logger specified in segment '%s'", spec, field)
			}
			if field != "" && !IsValidLevel(split[1]) {
				return errors.Errorf("invalid logging specification '%s': bad segment '%s'", spec, field)
			}

			level := NameToLevel(split[1])
			loggers := strings.Split(split[0], ",")
			for _, loggers := range loggers {
				specs[loggers] = level
			}

		default:
			return errors.Errorf("invalid logging specification '%s': bad segment '%s'", spec, field)
		}
	}

	m.defaultLevel = defaultLevel
	m.specs = specs
	m.levelCache = map[string]zapcore.Level{}

	return nil
}

// Level returns the effective logging level for a logger. If a level has not
// been explicitly set for the logger, the default logging level will be
// returned.
func (m *ModuleLevels) Level(loggerName string) zapcore.Level {
	if level, ok := m.cachedLevel(loggerName); ok {
		return level
	}

	m.mutex.Lock()
	level := m.calculateLevel(loggerName)
	m.levelCache[loggerName] = level
	m.mutex.Unlock()

	return level
}

// calculateLevel walks the logger name back to find the appropriate
// log level from the current spec.
func (m *ModuleLevels) calculateLevel(loggerName string) zapcore.Level {
	candidate := loggerName + "."
	for {
		if lvl, ok := m.specs[candidate]; ok {
			return lvl
		}

		idx := strings.LastIndex(candidate, ".")
		if idx <= 0 {
			return m.defaultLevel
		}
		candidate = candidate[:idx]
	}
}

// cachedLevel attempts to retrieve the effective log level for a logger from the
// cache. If the logger is not found, ok will be false.
func (m *ModuleLevels) cachedLevel(loggerName string) (lvl zapcore.Level, ok bool) {
	m.mutex.RLock()
	level, ok := m.levelCache[loggerName]
	m.mutex.RUnlock()
	return level, ok
}

// LevelEnabler adapts ModuleLevels for use with zap as a zapcore.LevelEnabler.
func (m *ModuleLevels) LevelEnabler(module string) zapcore.LevelEnabler {
	return zap.LevelEnablerFunc(func(l zapcore.Level) bool {
		return m.Level(module).Enabled(l)
	})
}

// Spec returns a normalized version of the active logging spec.
func (m *ModuleLevels) Spec() string {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	var fields []string
	for k, v := range m.specs {
		fields = append(fields, fmt.Sprintf("%s=%s", k, v))
	}

	sort.Strings(fields)
	fields = append(fields, m.defaultLevel.String())

	return strings.Join(fields, ":")
}
