/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package flogging_test

import (
	"bytes"
	"testing"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/flogging/mock"
	"github.com/stretchr/testify/require"
)

func TestGlobalReset(t *testing.T) {
	flogging.Reset()
	err := flogging.Global.SetFormat("json")
	require.NoError(t, err)
	err = flogging.Global.ActivateSpec("logger=debug")
	require.NoError(t, err)

	system, err := flogging.New(flogging.Config{})
	require.NoError(t, err)
	require.NotEqual(t, flogging.Global.LoggerLevels, system.LoggerLevels)
	require.NotEqual(t, flogging.Global.Encoding(), system.Encoding())

	flogging.Reset()
	require.Equal(t, flogging.Global.LoggerLevels, system.LoggerLevels)
	require.Equal(t, flogging.Global.Encoding(), system.Encoding())
}

func TestGlobalInitConsole(t *testing.T) {
	flogging.Reset()
	defer flogging.Reset()

	buf := &bytes.Buffer{}
	flogging.Init(flogging.Config{
		Format:  "%{message}",
		LogSpec: "DEBUG",
		Writer:  buf,
	})

	logger := flogging.MustGetLogger("testlogger")
	logger.Debug("this is a message")

	require.Equal(t, "this is a message\n", buf.String())
}

func TestGlobalInitJSON(t *testing.T) {
	flogging.Reset()
	defer flogging.Reset()

	buf := &bytes.Buffer{}
	flogging.Init(flogging.Config{
		Format:  "json",
		LogSpec: "DEBUG",
		Writer:  buf,
	})

	logger := flogging.MustGetLogger("testlogger")
	logger.Debug("this is a message")

	require.Regexp(t, `{"level":"debug","ts":\d+.\d+,"name":"testlogger","caller":"flogging/global_test.go:\d+","msg":"this is a message"}\s+`, buf.String())
}

func TestGlobalInitLogfmt(t *testing.T) {
	flogging.Reset()
	defer flogging.Reset()

	buf := &bytes.Buffer{}
	flogging.Init(flogging.Config{
		Format:  "logfmt",
		LogSpec: "DEBUG",
		Writer:  buf,
	})

	logger := flogging.MustGetLogger("testlogger")
	logger.Debug("this is a message")

	require.Regexp(t, `^ts=\d+.\d+ level=debug name=testlogger caller=flogging/global_test.go:\d+ msg="this is a message"`, buf.String())
}

func TestGlobalInitPanic(t *testing.T) {
	flogging.Reset()
	defer flogging.Reset()

	require.Panics(t, func() {
		flogging.Init(flogging.Config{
			Format: "%{color:evil}",
		})
	})
}

func TestGlobalDefaultLevel(t *testing.T) {
	flogging.Reset()

	require.Equal(t, "info", flogging.DefaultLevel())
}

func TestGlobalLoggerLevel(t *testing.T) {
	flogging.Reset()
	require.Equal(t, "info", flogging.LoggerLevel("some.logger"))
}

func TestGlobalMustGetLogger(t *testing.T) {
	flogging.Reset()

	l := flogging.MustGetLogger("logger-name")
	require.NotNil(t, l)
}

func TestFlogginInitPanic(t *testing.T) {
	defer flogging.Reset()

	require.Panics(t, func() {
		flogging.Init(flogging.Config{
			Format: "%{color:broken}",
		})
	})
}

func TestActivateSpec(t *testing.T) {
	defer flogging.Reset()

	flogging.ActivateSpec("fatal")
	require.Equal(t, "fatal", flogging.Global.Spec())
}

func TestActivateSpecPanic(t *testing.T) {
	defer flogging.Reset()

	require.Panics(t, func() {
		flogging.ActivateSpec("busted")
	})
}

func TestGlobalSetObserver(t *testing.T) {
	flogging.Reset()
	defer flogging.Reset()

	observer := &mock.Observer{}

	flogging.Global.SetObserver(observer)
	o := flogging.Global.SetObserver(nil)
	require.Exactly(t, observer, o)
}

func TestGlobalSetWriter(t *testing.T) {
	flogging.Reset()
	defer flogging.Reset()

	w := &bytes.Buffer{}

	old := flogging.Global.SetWriter(w)
	flogging.Global.SetWriter(old)
	original := flogging.Global.SetWriter(nil)

	require.Exactly(t, old, original)
}
