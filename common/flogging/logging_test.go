/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package flogging_test

import (
	"errors"
	"testing"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/flogging/mock"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zapcore"
)

func TestNew(t *testing.T) {
	logging, err := flogging.New(flogging.Config{})
	assert.NoError(t, err)
	assert.Equal(t, zapcore.InfoLevel, logging.DefaultLevel())

	_, err = flogging.New(flogging.Config{
		LogSpec: "::=borken=::",
	})
	assert.EqualError(t, err, "invalid logging specification '::=borken=::': bad segment '=borken='")
}

//go:generate counterfeiter -o mock/write_syncer.go -fake-name WriteSyncer . writeSyncer
type writeSyncer interface {
	zapcore.WriteSyncer
}

func TestLoggingSetWriter(t *testing.T) {
	ws := &mock.WriteSyncer{}

	logging, err := flogging.New(flogging.Config{})
	assert.NoError(t, err)

	logging.SetWriter(ws)
	logging.Write([]byte("hello"))
	assert.Equal(t, 1, ws.WriteCallCount())
	assert.Equal(t, []byte("hello"), ws.WriteArgsForCall(0))

	err = logging.Sync()
	assert.NoError(t, err)

	ws.SyncReturns(errors.New("welp"))
	err = logging.Sync()
	assert.EqualError(t, err, "welp")
}

func TestZapLoggerNameConversion(t *testing.T) {
	logging, err := flogging.New(flogging.Config{
		LogSpec: "fatal:test=debug",
	})
	assert.NoError(t, err)

	assert.Equal(t, zapcore.FatalLevel, logging.Level("test/module/name"))
	assert.Equal(t, zapcore.DebugLevel, logging.Level("test.module.name"))

	logger := logging.Logger("test/module/name")
	assert.True(t, logger.IsEnabledFor(zapcore.DebugLevel))
}
