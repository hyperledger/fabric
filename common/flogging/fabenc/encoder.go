/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabenc

import (
	"io"

	"go.uber.org/zap/buffer"
	"go.uber.org/zap/zapcore"
)

// A FormatEncoder is a zapcore.Encoder that formats log records according to a
// go-logging based format specifier.
type FormatEncoder struct {
	zapcore.Encoder
	formatters     []Formatter
	pool           buffer.Pool
	consoleEncoder zapcore.Encoder
}

// A Formatter is used to format and write data from a zap log entry.
type Formatter interface {
	Format(w io.Writer, entry zapcore.Entry, fields []zapcore.Field)
}

// NewFormatEncoder creates a zapcore.Encoder that supports a subset of the
// formats provided by go-logging.
//
// The op-loggng specifiers supported by this formatter are:
//   - %{color} - level specific SGR color escape or SGR reset
//   - %{id} - a unique log sequence number
//   - %{level} - the log level of the entry
//   - %{message} - the log message
//   - %{module} - the zap logger name
//   - %{shortfunc} - the name of the function creating the log record
//   - %{time} - the time the log entry was created
//
// Specifiers may include an optional format verb:
//   - color: reset|bold
//   - id: a fmt style numeric formatter without the leading %
//   - level: a fmt style string formatter without the leading %
//   - message: a fmt style string formatter without the leading %
//   - module: a fmt style string formatter without the leading %
//
func NewFormatEncoder(formatSpec string) (*FormatEncoder, error) {
	formatters, err := ParseFormat(formatSpec)
	if err != nil {
		return nil, err
	}

	return &FormatEncoder{
		Encoder:    zapcore.NewConsoleEncoder(zapcore.EncoderConfig{LineEnding: "\n"}),
		formatters: formatters,
		pool:       buffer.NewPool(),
	}, nil
}

// Clone creates a new instance of this encoder with the same configuration.
func (f *FormatEncoder) Clone() zapcore.Encoder {
	return &FormatEncoder{
		Encoder:    f.Encoder.Clone(),
		formatters: f.formatters,
		pool:       f.pool,
	}
}

// EncodeEntry formats a zap log record. The structured fields are formatted by a
// zapcore.ConsoleEncoder and are appended as JSON to the end of the formatted entry.
// All entries are terminated by a newline.
func (f *FormatEncoder) EncodeEntry(entry zapcore.Entry, fields []zapcore.Field) (*buffer.Buffer, error) {
	line := f.pool.Get()
	for _, f := range f.formatters {
		f.Format(line, entry, fields)
	}

	encodedFields, err := f.Encoder.EncodeEntry(entry, fields)
	if err != nil {
		return nil, err
	}
	if line.Len() > 0 && encodedFields.Len() != 1 {
		line.AppendString(" ")
	}
	line.AppendString(encodedFields.String())
	encodedFields.Free()

	return line, nil
}
