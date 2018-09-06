/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package flogging

import (
	"io"
	"os"
	"sync"

	"github.com/hyperledger/fabric/common/flogging/fabenc"
	logging "github.com/op/go-logging"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Config is used to provide dependencies to a Logging instance.
type Config struct {
	// Format is the log record format specifier for the Logging instance. If the
	// spec is the string "json", log records will be formatted as JSON. Any
	// other string will be provided to the FormatEncoder. Please see
	// fabenc.ParseFormat for details on the supported verbs.
	//
	// If Format is not provided, a default format that provides basic information will
	// be used.
	Format string

	// LogSpec determines the log levels that are enabled for the logging system. The
	// spec must be in a format that can be processed by ActivateSpec.
	//
	// If LogSpec is not provided, modules will be enabled at the INFO level.
	LogSpec string

	// Writer is the sink for encoded and formatted log records.
	//
	// If a Writer is not provided, os.Stderr will be used as the log sink.
	Writer io.Writer
}

// Logging maintains the state associated with the fabric logging system. It is
// intended to bridge between the legacy logging infrastructure built around
// go-logging and the structured, level logging provided by zap.
type Logging struct {
	*ModuleLevels

	mutex          sync.RWMutex
	encoding       Encoding
	encoderConfig  zapcore.EncoderConfig
	multiFormatter *fabenc.MultiFormatter
	writer         zapcore.WriteSyncer
}

// New creates a new logging system and initializes it with the provided
// configuration.
func New(c Config) (*Logging, error) {
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.NameKey = "name"

	s := &Logging{
		ModuleLevels: &ModuleLevels{
			defaultLevel: defaultLevel,
		},
		encoderConfig:  encoderConfig,
		multiFormatter: fabenc.NewMultiFormatter(),
	}

	err := s.Apply(c)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// Apply applies the provided configuration to the logging system.
func (s *Logging) Apply(c Config) error {
	err := s.SetFormat(c.Format)
	if err != nil {
		return err
	}

	if c.LogSpec == "" {
		c.LogSpec = "INFO"
	}

	err = s.ModuleLevels.ActivateSpec(c.LogSpec)
	if err != nil {
		return err
	}

	if c.Writer == nil {
		c.Writer = os.Stderr
	}
	s.SetWriter(c.Writer)

	var formatter logging.Formatter
	if s.Encoding() == JSON {
		formatter = SetFormat(defaultFormat)
	} else {
		formatter = SetFormat(c.Format)
	}

	InitBackend(formatter, c.Writer)

	return nil
}

// SetFormat updates how log records are formatted and encoded. Log entries
// created after this method has completed will use the new format.
//
// An error is returned if the log format specification cannot be parsed.
func (s *Logging) SetFormat(format string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if format == "" {
		format = defaultFormat
	}

	if format == "json" {
		s.encoding = JSON
		return nil
	}

	formatters, err := fabenc.ParseFormat(format)
	if err != nil {
		return err
	}
	s.multiFormatter.SetFormatters(formatters)
	s.encoding = CONSOLE

	return nil
}

// SetWriter controls which writer formatted log records are written to.
// Writers, with the exception of an *os.File, need to be safe for concurrent
// use by multiple go routines.
func (s *Logging) SetWriter(w io.Writer) {
	var sw zapcore.WriteSyncer
	switch t := w.(type) {
	case *os.File:
		sw = zapcore.Lock(t)
	case zapcore.WriteSyncer:
		sw = t
	default:
		sw = zapcore.AddSync(w)
	}

	s.mutex.Lock()
	s.writer = sw
	s.mutex.Unlock()
}

// Write satisfies the io.Write contract. It delegates to the writer argument
// of SetWriter or the Writer field of Config. The Core uses this when encoding
// log records.
func (s *Logging) Write(b []byte) (int, error) {
	s.mutex.RLock()
	w := s.writer
	s.mutex.RUnlock()

	return w.Write(b)
}

// Sync satisfies the zapcore.WriteSyncer interface. It is used by the Core to
// flush log records before terminating the process.
func (s *Logging) Sync() error {
	s.mutex.RLock()
	w := s.writer
	s.mutex.RUnlock()

	return w.Sync()
}

// Encoding satisfies the Encoding interface. It determines whether the JSON or
// CONSOLE encoder should be used by the Core when log records are written.
func (s *Logging) Encoding() Encoding {
	s.mutex.RLock()
	e := s.encoding
	s.mutex.RUnlock()
	return e
}

// ZapLogger instantiates a new zap.Logger for the specified module. The module
// becomes the name of the logger and is used to determine which log levels are
// enabled.
func (s *Logging) ZapLogger(module string) *zap.Logger {
	s.mutex.RLock()
	level := s.ModuleLevels.Level(module)
	s.ModuleLevels.SetLevel(module, level)
	core := &Core{
		LevelEnabler: s.ModuleLevels.LevelEnabler(module),
		Encoders: map[Encoding]zapcore.Encoder{
			JSON:    zapcore.NewJSONEncoder(s.encoderConfig),
			CONSOLE: fabenc.NewFormatEncoder(s.multiFormatter),
		},
		Selector: s,
		Output:   s,
	}
	s.mutex.RUnlock()

	return NewZapLogger(core).Named(module)
}

// Logger instantiates a new FabricLogger for the specified module. The module
// becomes the name of the logger and is used to determine which log levels are
// enabled.
func (s *Logging) Logger(module string) *FabricLogger {
	zl := s.ZapLogger(module)
	return NewFabricLogger(zl)
}
