/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package flogging

import (
	"go.uber.org/zap/zapcore"
)

type Encoding int8

const (
	CONSOLE = iota
	JSON
)

// EncodingSelector is used to determine whether log records are
// encoded as JSON or in a human readable CONSOLE format.
type EncodingSelector interface {
	Encoding() Encoding
}

// Core is a custom implementation of a zapcore.Core. It's a terrible hack that
// only exists to work around the intersection of state associated with
// encoders, implementation hiding in zapcore, and implicit, ad-hoc logger
// initialization within fabric.
//
// In addition to encoding log entries and fields to a buffer, zap Encoder
// implementations also need to maintain field state. When zapcore.Core.With is
// used, the associated encoder is cloned and the fields are added to the
// encoder. This means that encoder instances cannot be shared across cores.
//
// In terms of implementation hiding, it's difficult for our FormatEncoder to
// cleanly wrap the JSON and console implementations from zap as all methods
// from the zapcore.ObjectEncoder would need to be implemented to delegate to
// the correct backend.
//
// This implementation works by associating multiple encoders with a core. When
// fields are added to the core, the fields are added to all of the encoder
// implementations. The core also references the logging configuration to
// determine the proper encoding to use, the writer to delegate to, and the
// enabled levels.
type Core struct {
	zapcore.LevelEnabler
	Levels   *LoggerLevels
	Encoders map[Encoding]zapcore.Encoder
	Selector EncodingSelector
	Output   zapcore.WriteSyncer
}

func (c *Core) With(fields []zapcore.Field) zapcore.Core {
	clones := map[Encoding]zapcore.Encoder{}
	for name, enc := range c.Encoders {
		clone := enc.Clone()
		addFields(clone, fields)
		clones[name] = clone
	}

	return &Core{
		LevelEnabler: c.LevelEnabler,
		Levels:       c.Levels,
		Encoders:     clones,
		Selector:     c.Selector,
		Output:       c.Output,
	}
}

func (c *Core) Check(e zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(e.Level) && c.Levels.Level(e.LoggerName).Enabled(e.Level) {
		return ce.AddCore(e, c)
	}
	return ce
}

func (c *Core) Write(e zapcore.Entry, fields []zapcore.Field) error {
	encoding := c.Selector.Encoding()
	enc := c.Encoders[encoding]

	buf, err := enc.EncodeEntry(e, fields)
	if err != nil {
		return err
	}
	_, err = c.Output.Write(buf.Bytes())
	buf.Free()
	if err != nil {
		return err
	}

	if e.Level >= zapcore.PanicLevel {
		c.Sync()
	}

	return nil
}

func (c *Core) Sync() error {
	return c.Output.Sync()
}

func addFields(enc zapcore.ObjectEncoder, fields []zapcore.Field) {
	for i := range fields {
		fields[i].AddTo(enc)
	}
}
