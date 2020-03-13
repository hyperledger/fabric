// Package zaplogfmt provides a zap encoder that formats log entries in
// "logfmt" format.
package zaplogfmt

import (
	"bytes"
	"encoding"
	"encoding/base64"
	"fmt"
	"math"
	"reflect"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"go.uber.org/zap/buffer"
	"go.uber.org/zap/zapcore"
)

var (
	logfmtPool = sync.Pool{
		New: func() interface{} { return &logfmtEncoder{} },
	}
	bufferpool = buffer.NewPool()
)

func getEncoder() *logfmtEncoder {
	return logfmtPool.Get().(*logfmtEncoder)
}

func putEncoder(enc *logfmtEncoder) {
	enc.EncoderConfig = nil
	enc.buf = nil
	enc.namespaces = nil
	enc.arrayLiteral = false
	logfmtPool.Put(enc)
}

type logfmtEncoder struct {
	*zapcore.EncoderConfig
	buf          *buffer.Buffer
	namespaces   []string
	arrayLiteral bool
}

// NewEncoder creates an encoder writes logfmt formatted log entries.
func NewEncoder(cfg zapcore.EncoderConfig) zapcore.Encoder {
	return &logfmtEncoder{
		EncoderConfig: &cfg,
		buf:           bufferpool.Get(),
	}
}

func (enc *logfmtEncoder) AddArray(key string, arr zapcore.ArrayMarshaler) error {
	enc.addKey(key)
	return enc.AppendArray(arr)
}

func (enc *logfmtEncoder) AddBinary(key string, value []byte) {
	enc.AddString(key, base64.StdEncoding.EncodeToString(value))
}

func (enc *logfmtEncoder) AddBool(key string, value bool) {
	enc.addKey(key)
	enc.AppendBool(value)
}

func (enc *logfmtEncoder) AddByteString(key string, value []byte) {
	enc.addKey(key)
	enc.AppendByteString(value)
}

func (enc *logfmtEncoder) AddComplex64(k string, v complex64) { enc.AddComplex128(k, complex128(v)) }
func (enc *logfmtEncoder) AddComplex128(key string, value complex128) {
	enc.addKey(key)
	enc.AppendComplex128(value)
}

func (enc *logfmtEncoder) AddDuration(key string, value time.Duration) {
	enc.addKey(key)
	enc.AppendDuration(value)
}

func (enc *logfmtEncoder) AddFloat32(key string, value float32) {
	enc.addKey(key)
	enc.AppendFloat32(value)
}

func (enc *logfmtEncoder) AddFloat64(key string, value float64) {
	enc.addKey(key)
	enc.AppendFloat64(value)
}

func (enc *logfmtEncoder) AddInt(k string, v int)     { enc.AddInt64(k, int64(v)) }
func (enc *logfmtEncoder) AddInt8(k string, v int8)   { enc.AddInt64(k, int64(v)) }
func (enc *logfmtEncoder) AddInt32(k string, v int32) { enc.AddInt64(k, int64(v)) }
func (enc *logfmtEncoder) AddInt16(k string, v int16) { enc.AddInt64(k, int64(v)) }
func (enc *logfmtEncoder) AddInt64(key string, value int64) {
	enc.addKey(key)
	enc.AppendInt64(value)
}

func (enc *logfmtEncoder) AddObject(key string, obj zapcore.ObjectMarshaler) error {
	enc.addKey(key)
	return enc.AppendObject(obj)
}

func (enc *logfmtEncoder) AddReflected(key string, value interface{}) error {
	enc.addKey(key)
	return enc.AppendReflected(value)
}

func (enc *logfmtEncoder) AddString(key, value string) {
	enc.addKey(key)
	enc.AppendString(value)
}

func (enc *logfmtEncoder) AddTime(key string, value time.Time) {
	enc.addKey(key)
	enc.AppendTime(value)
}

func (enc *logfmtEncoder) AddUint(k string, v uint)       { enc.AddUint64(k, uint64(v)) }
func (enc *logfmtEncoder) AddUint8(k string, v uint8)     { enc.AddUint64(k, uint64(v)) }
func (enc *logfmtEncoder) AddUint32(k string, v uint32)   { enc.AddUint64(k, uint64(v)) }
func (enc *logfmtEncoder) AddUint16(k string, v uint16)   { enc.AddUint64(k, uint64(v)) }
func (enc *logfmtEncoder) AddUintptr(k string, v uintptr) { enc.AddUint64(k, uint64(v)) }
func (enc *logfmtEncoder) AddUint64(key string, value uint64) {
	enc.addKey(key)
	enc.AppendUint64(value)
}

func (enc *logfmtEncoder) AppendArray(arr zapcore.ArrayMarshaler) error {
	marshaler := enc.clone()
	marshaler.namespaces = nil
	marshaler.arrayLiteral = true

	marshaler.buf.AppendByte('[')
	err := arr.MarshalLogArray(marshaler)
	if err == nil {
		marshaler.buf.AppendByte(']')
		enc.AppendByteString(marshaler.buf.Bytes())
	} else {
		enc.AppendByteString(nil)
	}
	marshaler.buf.Free()
	putEncoder(marshaler)
	return err
}

func (enc *logfmtEncoder) AppendBool(value bool) {
	if value {
		enc.AppendString("true")
	} else {
		enc.AppendString("false")
	}
}

func (enc *logfmtEncoder) AppendByteString(value []byte) {
	enc.addSeparator()

	needsQuotes := bytes.IndexFunc(value, needsQuotedValueRune) != -1
	if needsQuotes {
		enc.buf.AppendByte('"')
	}
	enc.safeAddByteString(value)
	if needsQuotes {
		enc.buf.AppendByte('"')
	}
}

func (enc *logfmtEncoder) AppendComplex64(v complex64) { enc.AppendComplex128(complex128(v)) }
func (enc *logfmtEncoder) AppendComplex128(value complex128) {
	enc.addSeparator()

	// Cast to a platform-independent, fixed-size type.
	r, i := float64(real(value)), float64(imag(value))
	enc.buf.AppendFloat(r, 64)
	enc.buf.AppendByte('+')
	enc.buf.AppendFloat(i, 64)
	enc.buf.AppendByte('i')
}

func (enc *logfmtEncoder) AppendDuration(value time.Duration) {
	cur := enc.buf.Len()
	if enc.EncodeDuration != nil {
		enc.EncodeDuration(value, enc)
	}
	if cur == enc.buf.Len() {
		enc.AppendInt64(int64(value))
	}
}

func (enc *logfmtEncoder) AppendFloat32(v float32) { enc.appendFloat(float64(v), 32) }
func (enc *logfmtEncoder) AppendFloat64(v float64) { enc.appendFloat(v, 64) }
func (enc *logfmtEncoder) appendFloat(val float64, bitSize int) {
	enc.addSeparator()

	switch {
	case math.IsNaN(val):
		enc.buf.AppendString(`NaN`)
	case math.IsInf(val, 1):
		enc.buf.AppendString(`+Inf`)
	case math.IsInf(val, -1):
		enc.buf.AppendString(`-Inf`)
	default:
		enc.buf.AppendFloat(val, bitSize)
	}
}

func (enc *logfmtEncoder) AppendInt(v int)     { enc.AppendInt64(int64(v)) }
func (enc *logfmtEncoder) AppendInt8(v int8)   { enc.AppendInt64(int64(v)) }
func (enc *logfmtEncoder) AppendInt16(v int16) { enc.AppendInt64(int64(v)) }
func (enc *logfmtEncoder) AppendInt32(v int32) { enc.AppendInt64(int64(v)) }
func (enc *logfmtEncoder) AppendInt64(value int64) {
	enc.addSeparator()
	enc.buf.AppendInt(value)
}

func (enc *logfmtEncoder) AppendObject(obj zapcore.ObjectMarshaler) error {
	marshaler := enc.clone()
	marshaler.namespaces = nil

	err := obj.MarshalLogObject(marshaler)
	if err == nil {
		enc.AppendByteString(marshaler.buf.Bytes())
	} else {
		enc.AppendByteString(nil)
	}
	marshaler.buf.Free()
	putEncoder(marshaler)
	return err
}

func (enc *logfmtEncoder) AppendReflected(value interface{}) error {
	switch v := value.(type) {
	case nil:
		enc.AppendString("null")
	case error:
		enc.AppendString(v.Error())
	case []byte:
		enc.AppendByteString(v)
	case fmt.Stringer:
		enc.AppendString(v.String())
	case encoding.TextMarshaler:
		b, err := v.MarshalText()
		if err != nil {
			return err
		}
		enc.AppendString(string(b))
	default:
		rvalue := reflect.ValueOf(value)
		switch rvalue.Kind() {
		case reflect.Bool:
			enc.AppendBool(rvalue.Bool())
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			enc.AppendInt64(rvalue.Int())
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			enc.AppendUint64(rvalue.Uint())
		case reflect.Float32:
			enc.appendFloat(rvalue.Float(), 32)
		case reflect.Float64:
			enc.AppendFloat64(rvalue.Float())
		case reflect.String:
			enc.AppendString(rvalue.String())
		case reflect.Complex64, reflect.Complex128:
			enc.AppendComplex128(rvalue.Complex())
		case reflect.Chan, reflect.Func:
			enc.AppendString(fmt.Sprintf("%T(%p)", value, value))
		case reflect.Map, reflect.Struct:
			enc.AppendString(fmt.Sprint(value))
		case reflect.Array, reflect.Slice:
			enc.AppendArray(zapcore.ArrayMarshalerFunc(func(ae zapcore.ArrayEncoder) error {
				for i := 0; i < rvalue.Len(); i++ {
					ae.AppendReflected(rvalue.Index(i).Interface())
				}
				return nil
			}))
		case reflect.Interface, reflect.Ptr:
			return enc.AppendReflected(rvalue.Elem().Interface())
		}
	}
	return nil
}

func (enc *logfmtEncoder) AppendString(value string) {
	enc.addSeparator()

	needsQuotes := strings.IndexFunc(value, needsQuotedValueRune) != -1
	if needsQuotes {
		enc.buf.AppendByte('"')
	}
	enc.safeAddString(value)
	if needsQuotes {
		enc.buf.AppendByte('"')
	}
}

func (enc *logfmtEncoder) AppendTime(value time.Time) {
	cur := enc.buf.Len()
	if enc.EncodeTime != nil {
		enc.EncodeTime(value, enc)
	}
	if cur == enc.buf.Len() {
		enc.AppendInt64(value.UnixNano())
	}
}

func (enc *logfmtEncoder) AppendUint(v uint)       { enc.AppendUint64(uint64(v)) }
func (enc *logfmtEncoder) AppendUint8(v uint8)     { enc.AppendUint64(uint64(v)) }
func (enc *logfmtEncoder) AppendUint16(v uint16)   { enc.AppendUint64(uint64(v)) }
func (enc *logfmtEncoder) AppendUint32(v uint32)   { enc.AppendUint64(uint64(v)) }
func (enc *logfmtEncoder) AppendUintptr(v uintptr) { enc.AppendUint64(uint64(v)) }
func (enc *logfmtEncoder) AppendUint64(value uint64) {
	enc.addSeparator()
	enc.buf.AppendUint(value)
}

func (enc *logfmtEncoder) Clone() zapcore.Encoder {
	clone := enc.clone()
	clone.buf.Write(enc.buf.Bytes())
	return clone
}

func (enc *logfmtEncoder) clone() *logfmtEncoder {
	clone := getEncoder()
	clone.EncoderConfig = enc.EncoderConfig
	clone.buf = bufferpool.Get()
	clone.namespaces = enc.namespaces
	return clone
}

func (enc *logfmtEncoder) OpenNamespace(key string) {
	key = strings.Map(keyRuneFilter, key)
	enc.namespaces = append(enc.namespaces, key)
}

func (enc *logfmtEncoder) EncodeEntry(ent zapcore.Entry, fields []zapcore.Field) (*buffer.Buffer, error) {
	final := enc.clone()
	if final.TimeKey != "" {
		final.AddTime(final.TimeKey, ent.Time)
	}
	if final.LevelKey != "" {
		final.addKey(final.LevelKey)
		cur := final.buf.Len()
		if final.EncodeLevel != nil {
			final.EncodeLevel(ent.Level, final)
		}
		if cur == final.buf.Len() {
			// User-supplied EncodeLevel was a no-op. Fall back to strings to keep
			// output valid.
			final.AppendString(ent.Level.String())
		}
	}
	if ent.LoggerName != "" && final.NameKey != "" {
		final.addKey(final.NameKey)
		cur := final.buf.Len()
		if final.EncodeName != nil {
			final.EncodeName(ent.LoggerName, final)
		}
		if cur == final.buf.Len() {
			// User-supplied EncodeName was a no-op. Fall back to strings to
			// keep output valid.
			final.AppendString(ent.LoggerName)
		}
	}
	if ent.Caller.Defined && final.CallerKey != "" {
		final.addKey(final.CallerKey)
		cur := final.buf.Len()
		if final.EncodeCaller != nil {
			final.EncodeCaller(ent.Caller, final)
		}
		if cur == final.buf.Len() {
			// User-supplied EncodeCaller was a no-op. Fall back to strings to
			// keep output valid.
			final.AppendString(ent.Caller.String())
		}
	}
	if final.MessageKey != "" {
		final.addKey(enc.MessageKey)
		final.AppendString(ent.Message)
	}
	if enc.buf.Len() > 0 {
		if final.buf.Len() > 0 {
			final.buf.AppendByte(' ')
		}
		final.buf.Write(enc.buf.Bytes())
	}
	addFields(final, fields)
	if ent.Stack != "" && final.StacktraceKey != "" {
		final.AddString(final.StacktraceKey, ent.Stack)
	}
	if final.LineEnding != "" {
		final.buf.AppendString(final.LineEnding)
	} else {
		final.buf.AppendString(zapcore.DefaultLineEnding)
	}

	ret := final.buf
	putEncoder(final)
	return ret, nil
}

func (enc *logfmtEncoder) addSeparator() {
	if !enc.arrayLiteral {
		return
	}

	last := enc.buf.Len() - 1
	if last >= 0 && enc.buf.Bytes()[last] != '[' {
		enc.buf.AppendByte(',')
	}
}

func (enc *logfmtEncoder) addKey(key string) {
	key = strings.Map(keyRuneFilter, key)
	if enc.buf.Len() > 0 {
		enc.buf.AppendByte(' ')
	}
	for _, ns := range enc.namespaces {
		enc.safeAddString(ns)
		enc.buf.AppendByte('.')
	}
	enc.safeAddString(key)
	enc.buf.AppendByte('=')
}

// safeAddString JSON-escapes a string and appends it to the internal buffer.
// Unlike the standard library's encoder, it doesn't attempt to protect the
// user from browser vulnerabilities or JSONP-related problems.
func (enc *logfmtEncoder) safeAddString(s string) {
	for i := 0; i < len(s); {
		if enc.tryAddRuneSelf(s[i]) {
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if enc.tryAddRuneError(r, size) {
			i++
			continue
		}
		enc.buf.AppendString(s[i : i+size])
		i += size
	}
}

// safeAddByteString is no-alloc equivalent of safeAddString(string(s)) for s []byte.
func (enc *logfmtEncoder) safeAddByteString(s []byte) {
	for i := 0; i < len(s); {
		if enc.tryAddRuneSelf(s[i]) {
			i++
			continue
		}
		r, size := utf8.DecodeRune(s[i:])
		if enc.tryAddRuneError(r, size) {
			i++
			continue
		}
		enc.buf.Write(s[i : i+size])
		i += size
	}
}

// tryAddRuneSelf appends b if it is valid UTF-8 character represented in a single byte.
func (enc *logfmtEncoder) tryAddRuneSelf(b byte) bool {
	if b >= utf8.RuneSelf {
		return false
	}
	if 0x20 <= b && b != '\\' && b != '"' {
		enc.buf.AppendByte(b)
		return true
	}
	switch b {
	case '\\', '"':
		enc.buf.AppendByte('\\')
		enc.buf.AppendByte(b)
	case '\n':
		enc.buf.AppendByte('\\')
		enc.buf.AppendByte('n')
	case '\r':
		enc.buf.AppendByte('\\')
		enc.buf.AppendByte('r')
	case '\t':
		enc.buf.AppendByte('\\')
		enc.buf.AppendByte('t')
	default:
		// Encode bytes < 0x20, except for the escape sequences above.
		const _hex = "0123456789abcdef"
		enc.buf.AppendString(`\u00`)
		enc.buf.AppendByte(_hex[b>>4])
		enc.buf.AppendByte(_hex[b&0xF])
	}
	return true
}

func (enc *logfmtEncoder) tryAddRuneError(r rune, size int) bool {
	if r == utf8.RuneError && size == 1 {
		enc.buf.AppendString(`\ufffd`)
		return true
	}
	return false
}

func needsQuotedValueRune(r rune) bool {
	return r <= ' ' || r == '=' || r == '"' || r == utf8.RuneError
}

func keyRuneFilter(r rune) rune {
	if needsQuotedValueRune(r) {
		return -1
	}
	return r
}

func addFields(enc zapcore.ObjectEncoder, fields []zapcore.Field) {
	for i := range fields {
		fields[i].AddTo(enc)
	}
}
