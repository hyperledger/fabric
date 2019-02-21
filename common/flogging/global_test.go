/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package flogging_test

import (
	"bytes"
	"testing"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/stretchr/testify/assert"
)

func TestGlobalReset(t *testing.T) {
	flogging.Reset()
	flogging.Global.SetFormat("json")
	err := flogging.Global.ActivateSpec("logger=debug")
	assert.NoError(t, err)

	system, err := flogging.New(flogging.Config{})
	assert.NoError(t, err)
	assert.NotEqual(t, flogging.Global.LoggerLevels, system.LoggerLevels)
	assert.NotEqual(t, flogging.Global.Encoding(), system.Encoding())

	flogging.Reset()
	assert.Equal(t, flogging.Global.LoggerLevels, system.LoggerLevels)
	assert.Equal(t, flogging.Global.Encoding(), system.Encoding())
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

	assert.Equal(t, "this is a message\n", buf.String())
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

	assert.Regexp(t, `{"level":"debug","ts":\d+.\d+,"name":"testlogger","caller":"flogging/global_test.go:\d+","msg":"this is a message"}\s+`, buf.String())
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

	assert.Regexp(t, `^ts=\d+.\d+ level=debug name=testlogger caller=flogging/global_test.go:\d+ msg="this is a message"`, buf.String())
}

func TestGlobalInitPanic(t *testing.T) {
	flogging.Reset()
	defer flogging.Reset()

	assert.Panics(t, func() {
		flogging.Init(flogging.Config{
			Format: "%{color:evil}",
		})
	})
}

func TestGlobalDefaultLevel(t *testing.T) {
	flogging.Reset()

	assert.Equal(t, "INFO", flogging.DefaultLevel())
}

func TestGlobalGetLoggerLevel(t *testing.T) {
	flogging.Reset()
	assert.Equal(t, "INFO", flogging.GetLoggerLevel("some.logger"))
}

func TestGlobalMustGetLogger(t *testing.T) {
	flogging.Reset()

	l := flogging.MustGetLogger("logger-name")
	assert.NotNil(t, l)
}

func TestFlogginInitPanic(t *testing.T) {
	defer flogging.Reset()

	assert.Panics(t, func() {
		flogging.Init(flogging.Config{
			Format: "%{color:broken}",
		})
	})
}

func TestActivateSpec(t *testing.T) {
	defer flogging.Reset()

	flogging.ActivateSpec("fatal")
	assert.Equal(t, "fatal", flogging.Global.Spec())
}

func TestActivateSpecPanic(t *testing.T) {
	defer flogging.Reset()

	assert.Panics(t, func() {
		flogging.ActivateSpec("busted")
	})
}
