/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabenc

import (
	"io"
	"time"

	"go.uber.org/zap/buffer"
	"go.uber.org/zap/zapcore"
)

// A FormatEncoder is a zapcore.Encoder that formats log records according to a
// go-logging based format specifier.
type FormatEncoder struct {
	zapcore.Encoder
	formatters []Formatter
	pool       buffer.Pool
}

// A Formatter is used to format and write data from a zap log entry.
type Formatter interface {
	Format(w io.Writer, entry zapcore.Entry, fields []zapcore.Field)
}

func NewFormatEncoder(formatters ...Formatter) *FormatEncoder {
	return &FormatEncoder{
		Encoder: zapcore.NewConsoleEncoder(zapcore.EncoderConfig{
			MessageKey:     "", // disable
			LevelKey:       "", // disable
			TimeKey:        "", // disable
			NameKey:        "", // disable
			CallerKey:      "", // disable
			StacktraceKey:  "", // disable
			LineEnding:     "\n",
			EncodeDuration: zapcore.StringDurationEncoder,
			EncodeTime: func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
				enc.AppendString(t.Format("2006-01-02T15:04:05.999Z07:00"))
			},
		}),
		formatters: formatters,
		pool:       buffer.NewPool(),
	}
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
