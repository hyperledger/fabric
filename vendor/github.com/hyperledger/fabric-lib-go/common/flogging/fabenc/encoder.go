/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabenc

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"time"

	"github.com/go-logfmt/logfmt"
	"go.uber.org/zap/buffer"
	"go.uber.org/zap/zapcore"
)

// A FormatEncoder is a zapcore.Encoder that formats log records according to a
// go-logging based format specifier. Structured fields are appended to the
// formatted record in logfmt (key=value) form: context added via With is
// emitted first, sorted by key, followed by the entry's own fields in the order
// they were supplied. Fields are not de-duplicated, so a repeated key is
// emitted once per occurrence with its own value.
//
// The embedded MapObjectEncoder provides the zapcore.ObjectEncoder
// implementation used to accumulate the With context.
type FormatEncoder struct {
	*zapcore.MapObjectEncoder
	formatters []Formatter
	pool       buffer.Pool
}

// A Formatter is used to format and write data from a zap log entry.
type Formatter interface {
	Format(w io.Writer, entry zapcore.Entry, fields []zapcore.Field)
}

func NewFormatEncoder(formatters ...Formatter) *FormatEncoder {
	return &FormatEncoder{
		MapObjectEncoder: zapcore.NewMapObjectEncoder(),
		formatters:       formatters,
		pool:             buffer.NewPool(),
	}
}

// Clone creates a new instance of this encoder with the same configuration.
func (f *FormatEncoder) Clone() zapcore.Encoder {
	clone := zapcore.NewMapObjectEncoder()
	maps.Copy(clone.Fields, f.Fields)
	return &FormatEncoder{
		MapObjectEncoder: clone,
		formatters:       f.formatters,
		pool:             f.pool,
	}
}

// EncodeEntry formats a zap log record. The With context is appended sorted by
// key, followed by this entry's fields in insertion order, all in logfmt form.
// All entries are terminated by a newline.
func (f *FormatEncoder) EncodeEntry(entry zapcore.Entry, fields []zapcore.Field) (*buffer.Buffer, error) {
	line := f.pool.Get()
	for _, formatter := range f.formatters {
		formatter.Format(line, entry, fields)
	}

	type keyVal struct {
		key string
		val any
	}

	// Assemble the output: the With context first, sorted by key (map order is
	// unrecoverable), then this entry's fields in the order supplied.
	keyVals := make([]keyVal, 0, len(f.Fields)+len(fields))
	for _, key := range slices.Sorted(maps.Keys(f.Fields)) {
		keyVals = append(keyVals, keyVal{key: key, val: f.Fields[key]})
	}
	for _, field := range fields {
		// Fields are not de-duplicated, so a repeated key is preserved with its own value, as
		// the original logfmt encoder did. Thus, we decode each field on its own map encoder.
		mapEncoder := zapcore.NewMapObjectEncoder()
		field.AddTo(mapEncoder)
		// Some fields are skipped, so we need to check if it was actually added.
		if v, ok := mapEncoder.Fields[field.Key]; ok {
			keyVals = append(keyVals, keyVal{key: field.Key, val: v})
		}
	}

	// Separate the first field from the formatted prefix; logfmt inserts
	// the separators between subsequent fields itself.
	if len(keyVals) > 0 && line.Len() > 0 {
		line.AppendString(" ")
	}

	enc := logfmt.NewEncoder(line)
	for _, kv := range keyVals {
		if err := encodeKeyValue(enc, kv.key, kv.val); err != nil {
			return nil, err
		}
	}

	line.AppendString("\n")

	return line, nil
}

func encodeKeyValue(enc *logfmt.Encoder, key string, value any) error {
	if t, ok := value.(time.Time); ok {
		// Normalizes values that logfmt would otherwise render differently
		// than intended. Timestamps use a millisecond-precision layout instead of the
		// nanosecond-precision encoding.TextMarshaler output.
		value = t.Format("2006-01-02T15:04:05.999Z07:00")
	}

	err := enc.EncodeKeyval(key, value)
	if errors.Is(err, logfmt.ErrUnsupportedValueType) {
		// logfmt rejects composite types (structs, maps, slices, ...); fall
		// back to their fmt representation, matching what EncodeKeyvals() does.
		err = enc.EncodeKeyval(key, fmt.Sprint(value))
	}
	return err
}
