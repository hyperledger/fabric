/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package floggingtest

import (
	"bytes"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/flogging/fabenc"
	"github.com/onsi/gomega/gbytes"
	"go.uber.org/zap"
	"go.uber.org/zap/buffer"
	"go.uber.org/zap/zapcore"
)

// DefaultFormat is a log encoding format that is mostly compatible with the default
// log format but excludes colorization and time.
const DefaultFormat = "[%{module}] %{shortfunc} -> %{level:.4s} %{id:04x} %{message}"

type Recorder struct {
	mutex    sync.RWMutex
	entries  []string
	messages []string
	buffer   *gbytes.Buffer
}

func newRecorder() *Recorder {
	return &Recorder{
		buffer:   gbytes.NewBuffer(),
		entries:  []string{},
		messages: []string{},
	}
}

func (r *Recorder) addEntry(e zapcore.Entry, line *buffer.Buffer) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	r.buffer.Write(line.Bytes())
	r.entries = append(r.entries, strings.TrimRight(line.String(), "\n"))
	r.messages = append(r.messages, e.Message)
}

func (r *Recorder) Reset() {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.buffer = gbytes.NewBuffer()
	r.entries = []string{}
	r.messages = []string{}
}

func (r *Recorder) Buffer() *gbytes.Buffer {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return r.buffer
}

func (r *Recorder) Entries() []string {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	entries := make([]string, len(r.entries), cap(r.entries))
	copy(entries, r.entries)
	return entries
}

func (r *Recorder) EntriesContaining(sub string) []string {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	matches := []string{}
	for _, entry := range r.entries {
		if strings.Contains(entry, sub) {
			matches = append(matches, entry)
		}
	}
	return matches
}

func (r *Recorder) EntriesMatching(regex string) []string {
	re := regexp.MustCompile(regex)
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	matches := []string{}
	for _, entry := range r.entries {
		if re.MatchString(entry) {
			matches = append(matches, entry)
		}
	}
	return matches
}

func (r *Recorder) Messages() []string {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	messages := make([]string, len(r.messages), cap(r.messages))
	copy(messages, r.messages)
	return messages
}

func (r *Recorder) MessagesContaining(sub string) []string {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	matches := []string{}
	for _, msg := range r.messages {
		if strings.Contains(msg, sub) {
			matches = append(matches, msg)
		}
	}
	return matches
}

func (r *Recorder) MessagesMatching(regex string) []string {
	re := regexp.MustCompile(regex)
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	matches := []string{}
	for _, msg := range r.messages {
		if re.MatchString(msg) {
			matches = append(matches, msg)
		}
	}
	return matches
}

type RecordingCore struct {
	zapcore.LevelEnabler
	encoder  zapcore.Encoder
	recorder *Recorder
	writer   zapcore.WriteSyncer
}

func (r *RecordingCore) Write(e zapcore.Entry, fields []zapcore.Field) error {
	buf, err := r.encoder.EncodeEntry(e, fields)
	if err != nil {
		return err
	}

	r.writer.Write(buf.Bytes())
	r.recorder.addEntry(e, buf)

	buf.Free()

	return nil
}

func (r *RecordingCore) Check(e zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if r.Enabled(e.Level) {
		ce = ce.AddCore(e, r)
	}
	if ce != nil && e.Level == zapcore.FatalLevel {
		panic(e.Message)
	}
	return ce
}

func (r *RecordingCore) With(fields []zapcore.Field) zapcore.Core {
	clone := &RecordingCore{
		LevelEnabler: r.LevelEnabler,
		encoder:      r.encoder.Clone(),
		recorder:     r.recorder,
		writer:       r.writer,
	}

	for _, f := range fields {
		f.AddTo(clone.encoder)
	}

	return clone
}

func (r *RecordingCore) Sync() error {
	return r.writer.Sync()
}

type TestingWriter struct{ testing.TB }

func (t *TestingWriter) Write(buf []byte) (int, error) {
	t.Logf("%s", bytes.TrimRight(buf, "\n"))
	return len(buf), nil
}

func (t *TestingWriter) Sync() error { return nil }

type Option func(r *RecordingCore, l *zap.Logger) *zap.Logger

func Named(loggerName string) Option {
	return func(r *RecordingCore, l *zap.Logger) *zap.Logger {
		return l.Named(loggerName)
	}
}

func AtLevel(level zapcore.Level) Option {
	return func(r *RecordingCore, l *zap.Logger) *zap.Logger {
		r.LevelEnabler = zap.LevelEnablerFunc(func(l zapcore.Level) bool {
			return level.Enabled(l)
		})
		return l
	}
}

func NewTestLogger(tb testing.TB, options ...Option) (*flogging.FabricLogger, *Recorder) {
	enabler := zap.LevelEnablerFunc(func(l zapcore.Level) bool {
		return zapcore.DebugLevel.Enabled(l)
	})

	formatters, err := fabenc.ParseFormat(DefaultFormat)
	if err != nil {
		tb.Fatalf("failed to parse format %s: %s", DefaultFormat, err)
	}
	encoder := fabenc.NewFormatEncoder(formatters...)
	if err != nil {
		tb.Fatalf("failed to create format encoder: %s", err)
	}

	recorder := newRecorder()
	recordingCore := &RecordingCore{
		LevelEnabler: enabler,
		encoder:      encoder,
		recorder:     recorder,
		writer:       &TestingWriter{TB: tb},
	}

	zl := zap.New(recordingCore)
	for _, o := range options {
		zl = o(recordingCore, zl)
	}

	return flogging.NewFabricLogger(zl, zap.AddCaller()), recorder
}
