/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package flogging

import (
	"regexp"
	"strings"
	"sync"

	"github.com/pkg/errors"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// ModuleLevels tracks the logging level of logging modules.
type ModuleLevels struct {
	defaultLevel zapcore.Level

	mutex  sync.RWMutex
	levels map[string]zapcore.Level
}

// SetDefaultLevel sets the default logging level for modules that do not have
// an explicit level set.
func (m *ModuleLevels) SetDefaultLevel(l zapcore.Level) {
	m.mutex.Lock()
	m.defaultLevel = l
	m.mutex.Unlock()
}

// DefaultLevel returns the default logging level for modules that do not have
// an explicit level set.
func (m *ModuleLevels) DefaultLevel() zapcore.Level {
	m.mutex.RLock()
	l := m.defaultLevel
	m.mutex.RUnlock()
	return l
}

// ResetLevels discards level information about all modules and restores the default
// logging level to zapcore.InfoLevel.
func (m *ModuleLevels) ResetLevels() {
	m.mutex.Lock()
	m.levels = nil
	m.defaultLevel = zapcore.InfoLevel
	m.mutex.Unlock()
}

// ActivateSpec is used to modify module logging levels.
//
// The logging specification has the following form:
//   [<module>[,<module>...]=]<level>[:[<module>[,<module>...]=]<level>...]
func (m *ModuleLevels) ActivateSpec(spec string) error {
	var levelAll *zapcore.Level
	updates := map[string]zapcore.Level{}

	fields := strings.Split(spec, ":")
	for _, field := range fields {
		split := strings.Split(field, "=")
		switch len(split) {
		case 1: // level
			l := NameToLevel(field)
			levelAll = &l
		case 2: // <module>[,<module>...]=<level>
			level := NameToLevel(split[1])
			if split[0] == "" {
				return errors.Errorf("invalid logging specification '%s': no module specified in segment '%s'", spec, field)
			}

			modules := strings.Split(split[0], ",")
			for _, module := range modules {
				updates[module] = level
			}

		default:
			return errors.Errorf("invalid logging specification '%s': bad segment '%s'", spec, field)
		}
	}

	// Update existing modules iff an unqualified level is set.
	if levelAll != nil {
		l := *levelAll
		m.SetDefaultLevel(l)
		for module := range m.Levels() {
			m.SetLevel(module, l)
		}
	}

	for module, level := range updates {
		m.SetLevel(module, level)
	}

	return nil
}

// SetLevel sets the logging level for a single logging module.
func (m *ModuleLevels) SetLevel(module string, l zapcore.Level) {
	m.mutex.Lock()
	if m.levels == nil {
		m.levels = map[string]zapcore.Level{}
	}
	m.levels[module] = l
	m.mutex.Unlock()
}

// SetLevels sets the logging level for all logging modules that match the
// provided regular expression.
func (m *ModuleLevels) SetLevels(re *regexp.Regexp, l zapcore.Level) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	for module := range m.levels {
		if re.MatchString(module) {
			m.levels[module] = l
		}
	}
}

// Level returns the effective logging level for a module. If a level has not
// been explicitly set for the module, the default logging level will be
// returned.
func (m *ModuleLevels) Level(module string) zapcore.Level {
	m.mutex.RLock()
	l, ok := m.levels[module]
	if !ok {
		l = m.defaultLevel
	}
	m.mutex.RUnlock()

	return l
}

// Levels returns a copy of current log levels.
func (m *ModuleLevels) Levels() map[string]zapcore.Level {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	levels := make(map[string]zapcore.Level, len(m.levels))
	for k, v := range m.levels {
		levels[k] = v
	}
	return levels
}

// RestoreLevels replaces the log levels with values previously acquired from
// Levels.
func (m *ModuleLevels) RestoreLevels(levels map[string]zapcore.Level) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	m.levels = map[string]zapcore.Level{}
	for k, v := range levels {
		m.levels[k] = v
	}
}

// LevelEnabler adapts ModuleLevels for use with zap as a zapcore.LevelEnabler.
func (m *ModuleLevels) LevelEnabler(module string) zapcore.LevelEnabler {
	return zap.LevelEnablerFunc(func(l zapcore.Level) bool {
		return m.Level(module).Enabled(l)
	})
}
