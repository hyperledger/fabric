/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package floggingtest

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"go.uber.org/zap/zapcore"
)

func TestLoggerRecorder(t *testing.T) {
	gt := NewGomegaWithT(t)

	tl, recorder := NewTestLogger(t, Named("test-logging"), AtLevel(zapcore.InfoLevel))
	tl.Error("this", "is", "an", "error")

	gt.Expect(recorder.Entries()).To(HaveLen(1))
	gt.Expect(recorder.Entries()).To(ConsistOf("[test-logging] TestLoggerRecorder -> ERRO 0001 this is an error"))

	gt.Expect(recorder.Messages()).To(HaveLen(1))
	gt.Expect(recorder.Messages()).To(ConsistOf("this is an error"))

	gt.Expect(string(recorder.Buffer().Contents())).To(Equal("[test-logging] TestLoggerRecorder -> ERRO 0001 this is an error\n"))
	gt.Expect(recorder).NotTo(gbytes.Say("nothing good"))
	gt.Expect(recorder).To(gbytes.Say(`\Q[test-logging] TestLoggerRecorder -> ERRO 0001 this is an error\E`))
}

func TestLoggerRecorderRegex(t *testing.T) {
	gt := NewGomegaWithT(t)

	tl, recorder := NewTestLogger(t, Named("test-logging"))
	tl.Debug("message one")
	tl.Debug("message two")
	tl.Debug("message three")

	gt.Expect(recorder.EntriesContaining("message")).To(HaveLen(3))
	gt.Expect(recorder.EntriesMatching("test-logging.*message t")).To(HaveLen(2))
	gt.Expect(recorder.MessagesContaining("message")).To(HaveLen(3))
	gt.Expect(recorder.MessagesMatching("^message t")).To(HaveLen(2))

	gt.Expect(recorder.EntriesContaining("one")).To(HaveLen(1))
	gt.Expect(recorder.MessagesContaining("one")).To(HaveLen(1))

	gt.Expect(recorder.EntriesContaining("two")).To(HaveLen(1))
	gt.Expect(recorder.MessagesContaining("two")).To(HaveLen(1))

	gt.Expect(recorder.EntriesContaining("")).To(HaveLen(3))
	gt.Expect(recorder.MessagesContaining("")).To(HaveLen(3))

	gt.Expect(recorder.EntriesContaining("mismatch")).To(HaveLen(0))
	gt.Expect(recorder.MessagesContaining("mismatch")).To(HaveLen(0))
}

func TestRecorderReset(t *testing.T) {
	gt := NewGomegaWithT(t)

	tl, recorder := NewTestLogger(t, Named("test-logging"))
	tl.Debug("message one")
	tl.Debug("message two")
	tl.Debug("message three")

	gt.Expect(recorder.Entries()).To(HaveLen(3))
	gt.Expect(recorder.Messages()).To(HaveLen(3))
	gt.Expect(recorder.Buffer().Contents()).NotTo(BeEmpty())

	recorder.Reset()
	gt.Expect(recorder.Entries()).To(HaveLen(0))
	gt.Expect(recorder.Messages()).To(HaveLen(0))
	gt.Expect(recorder.Buffer().Contents()).To(BeEmpty())
}

func TestFatalAsPanic(t *testing.T) {
	gt := NewGomegaWithT(t)

	tl, _ := NewTestLogger(t)
	gt.Expect(func() { tl.Fatal("this", "is", "an", "error") }).To(Panic())
}

func TestRecordingCoreWith(t *testing.T) {
	gt := NewGomegaWithT(t)
	logger, recorder := NewTestLogger(t)
	logger = logger.With("key", "value")

	logger.Debug("message")
	gt.Expect(recorder).To(gbytes.Say(`message key=value`))
}
