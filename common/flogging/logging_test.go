/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package flogging_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/flogging/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
)

func TestNew(t *testing.T) {
	logging, err := flogging.New(flogging.Config{})
	require.NoError(t, err)
	require.Equal(t, zapcore.InfoLevel, logging.DefaultLevel())

	_, err = flogging.New(flogging.Config{
		LogSpec: "::=borken=::",
	})
	require.EqualError(t, err, "invalid logging specification '::=borken=::': bad segment '=borken='")
}

func TestNewWithEnvironment(t *testing.T) {
	oldSpec, set := os.LookupEnv("FABRIC_LOGGING_SPEC")
	if set {
		defer os.Setenv("FABRIC_LOGGING_SPEC", oldSpec)
	}

	os.Setenv("FABRIC_LOGGING_SPEC", "fatal")
	logging, err := flogging.New(flogging.Config{})
	require.NoError(t, err)
	require.Equal(t, zapcore.FatalLevel, logging.DefaultLevel())

	os.Unsetenv("FABRIC_LOGGING_SPEC")
	logging, err = flogging.New(flogging.Config{})
	require.NoError(t, err)
	require.Equal(t, zapcore.InfoLevel, logging.DefaultLevel())
}

//go:generate counterfeiter -o mock/write_syncer.go -fake-name WriteSyncer . writeSyncer
type writeSyncer interface {
	zapcore.WriteSyncer
}

func TestLoggingSetWriter(t *testing.T) {
	ws := &mock.WriteSyncer{}

	w := &bytes.Buffer{}
	logging, err := flogging.New(flogging.Config{
		Writer: w,
	})
	require.NoError(t, err)

	old := logging.SetWriter(ws)
	logging.SetWriter(w)
	original := logging.SetWriter(ws)

	require.Exactly(t, old, original)
	_, err = logging.Write([]byte("hello"))
	require.NoError(t, err)
	require.Equal(t, 1, ws.WriteCallCount())
	require.Equal(t, []byte("hello"), ws.WriteArgsForCall(0))

	err = logging.Sync()
	require.NoError(t, err)

	ws.SyncReturns(errors.New("welp"))
	err = logging.Sync()
	require.EqualError(t, err, "welp")
}

func TestNamedLogger(t *testing.T) {
	defer flogging.Reset()
	buf := &bytes.Buffer{}
	flogging.Global.SetWriter(buf)

	t.Run("logger and named (child) logger with different levels", func(t *testing.T) {
		defer buf.Reset()
		logger := flogging.MustGetLogger("eugene")
		logger2 := logger.Named("george")
		flogging.ActivateSpec("eugene=info:eugene.george=error")

		logger.Info("from eugene")
		logger2.Info("from george")
		require.Contains(t, buf.String(), "from eugene")
		require.NotContains(t, buf.String(), "from george")
	})

	t.Run("named logger where parent logger isn't enabled", func(t *testing.T) {
		logger := flogging.MustGetLogger("foo")
		logger2 := logger.Named("bar")
		flogging.ActivateSpec("foo=fatal:foo.bar=error")
		logger.Error("from foo")
		logger2.Error("from bar")
		require.NotContains(t, buf.String(), "from foo")
		require.Contains(t, buf.String(), "from bar")
	})
}

func TestInvalidLoggerName(t *testing.T) {
	names := []string{"test*", ".test", "test.", ".", ""}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			msg := fmt.Sprintf("invalid logger name: %s", name)
			require.PanicsWithValue(t, msg, func() { flogging.MustGetLogger(name) })
		})
	}
}

func TestCheck(t *testing.T) {
	l := &flogging.Logging{}
	observer := &mock.Observer{}
	e := zapcore.Entry{}

	// set observer
	l.SetObserver(observer)
	l.Check(e, nil)
	require.Equal(t, 1, observer.CheckCallCount())
	e, ce := observer.CheckArgsForCall(0)
	require.Equal(t, e, zapcore.Entry{})
	require.Nil(t, ce)

	l.WriteEntry(e, nil)
	require.Equal(t, 1, observer.WriteEntryCallCount())
	e, f := observer.WriteEntryArgsForCall(0)
	require.Equal(t, e, zapcore.Entry{})
	require.Nil(t, f)

	//	remove observer
	l.SetObserver(nil)
	l.Check(zapcore.Entry{}, nil)
	require.Equal(t, 1, observer.CheckCallCount())
}

func TestLoggerCoreCheck(t *testing.T) {
	logging, err := flogging.New(flogging.Config{})
	require.NoError(t, err)

	logger := logging.ZapLogger("foo")

	err = logging.ActivateSpec("info")
	require.NoError(t, err)
	require.False(t, logger.Core().Enabled(zapcore.DebugLevel), "debug should not be enabled at info level")

	err = logging.ActivateSpec("debug")
	require.NoError(t, err)
	require.True(t, logger.Core().Enabled(zapcore.DebugLevel), "debug should now be enabled at debug level")
}
