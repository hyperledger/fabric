/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabenc

import (
	"fmt"
	"io"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"go.uber.org/zap/zapcore"
)

// formatRegexp is broken into three groups:
//  1. the format verb
//  2. an optional colon that is ungrouped with '?:'
//  3. an optional, non-greedy format directive
//
// The grouping simplifies the verb proccssing during spec parsing.
var formatRegexp = regexp.MustCompile(`%{(color|id|level|message|module|shortfunc|time)(?::(.*?))?}`)

// ParseFormat parses a log format spec and returns a slice of formatters
// that should be iterated over to build a formatted log record.
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
func ParseFormat(spec string) ([]Formatter, error) {
	cursor := 0
	formatters := []Formatter{}

	// iterate over the regex groups and convert to formatters
	matches := formatRegexp.FindAllStringSubmatchIndex(spec, -1)
	for _, m := range matches {
		start, end := m[0], m[1]
		verbStart, verbEnd := m[2], m[3]
		formatStart, formatEnd := m[4], m[5]

		if start > cursor {
			formatters = append(formatters, StringFormatter{Value: spec[cursor:start]})
		}

		var format string
		if formatStart >= 0 {
			format = spec[formatStart:formatEnd]
		}

		formatter, err := NewFormatter(spec[verbStart:verbEnd], format)
		if err != nil {
			return nil, err
		}

		formatters = append(formatters, formatter)
		cursor = end
	}

	// handle any trailing suffix
	if cursor != len(spec) {
		formatters = append(formatters, StringFormatter{Value: spec[cursor:]})
	}

	return formatters, nil
}

// A MultiFormatter presents multiple formatters as a single Formatter. It can
// be used to change the set of formatters associated with an encoder at
// runtime.
type MultiFormatter struct {
	mutex      sync.RWMutex
	formatters []Formatter
}

// NewMultiFormatter creates a new MultiFormatter that delegates to the
// provided formatters. The formatters are used in the order they are
// presented.
func NewMultiFormatter(formatters ...Formatter) *MultiFormatter {
	return &MultiFormatter{
		formatters: formatters,
	}
}

// Format iterates over its delegates to format a log record to the provided
// buffer.
func (m *MultiFormatter) Format(w io.Writer, entry zapcore.Entry, fields []zapcore.Field) {
	m.mutex.RLock()
	for i := range m.formatters {
		m.formatters[i].Format(w, entry, fields)
	}
	m.mutex.RUnlock()
}

// SetFormatters replaces the delegate formatters.
func (m *MultiFormatter) SetFormatters(formatters []Formatter) {
	m.mutex.Lock()
	m.formatters = formatters
	m.mutex.Unlock()
}

// A StringFormatter formats a fixed string.
type StringFormatter struct{ Value string }

// Format writes the formatter's fixed string to provided writer.
func (s StringFormatter) Format(w io.Writer, entry zapcore.Entry, fields []zapcore.Field) {
	fmt.Fprintf(w, "%s", s.Value)
}

// NewFormatter creates the formatter for the provided verb. When a format is
// not provided, the default format for the verb is used.
func NewFormatter(verb, format string) (Formatter, error) {
	switch verb {
	case "color":
		return newColorFormatter(format)
	case "id":
		return newSequenceFormatter(format), nil
	case "level":
		return newLevelFormatter(format), nil
	case "message":
		return newMessageFormatter(format), nil
	case "module":
		return newModuleFormatter(format), nil
	case "shortfunc":
		return newShortFuncFormatter(format), nil
	case "time":
		return newTimeFormatter(format), nil
	default:
		return nil, fmt.Errorf("unknown verb: %s", verb)
	}
}

// A ColorFormatter formats an SGR color code.
type ColorFormatter struct {
	Bold  bool // set the bold attribute
	Reset bool // reset colors and attributes
}

func newColorFormatter(f string) (ColorFormatter, error) {
	switch f {
	case "bold":
		return ColorFormatter{Bold: true}, nil
	case "reset":
		return ColorFormatter{Reset: true}, nil
	case "":
		return ColorFormatter{}, nil
	default:
		return ColorFormatter{}, fmt.Errorf("invalid color option: %s", f)
	}
}

// LevelColor returns the Color associated with a specific zap logging level.
func (c ColorFormatter) LevelColor(l zapcore.Level) Color {
	switch l {
	case zapcore.DebugLevel:
		return ColorCyan
	case zapcore.InfoLevel:
		return ColorBlue
	case zapcore.WarnLevel:
		return ColorYellow
	case zapcore.ErrorLevel:
		return ColorRed
	case zapcore.DPanicLevel, zapcore.PanicLevel:
		return ColorMagenta
	case zapcore.FatalLevel:
		return ColorMagenta
	default:
		return ColorNone
	}
}

// Format writes the SGR color code to the provided writer.
func (c ColorFormatter) Format(w io.Writer, entry zapcore.Entry, fields []zapcore.Field) {
	switch {
	case c.Reset:
		fmt.Fprint(w, ResetColor())
	case c.Bold:
		fmt.Fprint(w, c.LevelColor(entry.Level).Bold())
	default:
		fmt.Fprint(w, c.LevelColor(entry.Level).Normal())
	}
}

// LevelFormatter formats a log level.
type LevelFormatter struct{ FormatVerb string }

func newLevelFormatter(f string) LevelFormatter {
	return LevelFormatter{FormatVerb: "%" + stringOrDefault(f, "s")}
}

// Format writes the logging level to the provided writer.
func (l LevelFormatter) Format(w io.Writer, entry zapcore.Entry, fields []zapcore.Field) {
	fmt.Fprintf(w, l.FormatVerb, entry.Level.CapitalString())
}

// MessageFormatter formats a log message.
type MessageFormatter struct{ FormatVerb string }

func newMessageFormatter(f string) MessageFormatter {
	return MessageFormatter{FormatVerb: "%" + stringOrDefault(f, "s")}
}

// Format writes the log entry message to the provided writer.
func (m MessageFormatter) Format(w io.Writer, entry zapcore.Entry, fields []zapcore.Field) {
	fmt.Fprintf(w, m.FormatVerb, strings.TrimRight(entry.Message, "\n"))
}

// ModuleFormatter formats the zap logger name.
type ModuleFormatter struct{ FormatVerb string }

func newModuleFormatter(f string) ModuleFormatter {
	return ModuleFormatter{FormatVerb: "%" + stringOrDefault(f, "s")}
}

// Format writes the zap logger name to the specified writer.
func (m ModuleFormatter) Format(w io.Writer, entry zapcore.Entry, fields []zapcore.Field) {
	fmt.Fprintf(w, m.FormatVerb, entry.LoggerName)
}

// sequence maintains the global sequence number shared by all SequeneFormatter
// instances.
var sequence uint64

// SetSequence explicitly sets the global sequence number.
func SetSequence(s uint64) { atomic.StoreUint64(&sequence, s) }

// SequenceFormatter formats a global sequence number.
type SequenceFormatter struct{ FormatVerb string }

func newSequenceFormatter(f string) SequenceFormatter {
	return SequenceFormatter{FormatVerb: "%" + stringOrDefault(f, "d")}
}

// SequenceFormatter increments a global sequence number and writes it to the
// provided writer.
func (s SequenceFormatter) Format(w io.Writer, entry zapcore.Entry, fields []zapcore.Field) {
	fmt.Fprintf(w, s.FormatVerb, atomic.AddUint64(&sequence, 1))
}

// ShortFuncFormatter formats the name of the function creating the log record.
type ShortFuncFormatter struct{ FormatVerb string }

func newShortFuncFormatter(f string) ShortFuncFormatter {
	return ShortFuncFormatter{FormatVerb: "%" + stringOrDefault(f, "s")}
}

// Format writes the calling function name to the provided writer. The name is obtained from
// the runtime and the package and line numbers are discarded.
func (s ShortFuncFormatter) Format(w io.Writer, entry zapcore.Entry, fields []zapcore.Field) {
	f := runtime.FuncForPC(entry.Caller.PC)
	if f == nil {
		fmt.Fprintf(w, s.FormatVerb, "(unknown)")
		return
	}

	fname := f.Name()
	funcIdx := strings.LastIndex(fname, ".")
	fmt.Fprintf(w, s.FormatVerb, fname[funcIdx+1:])
}

// TimeFormatter formats the time from the zap log entry.
type TimeFormatter struct{ Layout string }

func newTimeFormatter(f string) TimeFormatter {
	return TimeFormatter{Layout: stringOrDefault(f, "2006-01-02T15:04:05.999Z07:00")}
}

// Format writes the log record time stamp to the provided writer.
func (t TimeFormatter) Format(w io.Writer, entry zapcore.Entry, fields []zapcore.Field) {
	fmt.Fprint(w, entry.Time.Format(t.Layout))
}

func stringOrDefault(str, dflt string) string {
	if str != "" {
		return str
	}
	return dflt
}
