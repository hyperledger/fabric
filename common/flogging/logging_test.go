/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package flogging_test

import (
	"bytes"
	"errors"
	"regexp"
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
	assert.Empty(t, logging.Levels())

	_, err = flogging.New(flogging.Config{
		LogSpec: "::=borken=::",
	})
	assert.EqualError(t, err, "invalid logging specification '::=borken=::': bad segment '=borken='")
}

func TestLoggingReset(t *testing.T) {
	logging, err := flogging.New(flogging.Config{})
	assert.NoError(t, err)

	var tests = []struct {
		desc string
		flogging.Config
		err            error
		expectedRegexp string
	}{
		{
			desc:           "implicit log spec",
			Config:         flogging.Config{Format: "%{message}"},
			expectedRegexp: regexp.QuoteMeta("this is a warning message\n"),
		},
		{
			desc:           "simple debug config",
			Config:         flogging.Config{LogSpec: "debug", Format: "%{message}"},
			expectedRegexp: regexp.QuoteMeta("this is a debug message\nthis is a warning message\n"),
		},
		{
			desc:           "module error config",
			Config:         flogging.Config{LogSpec: "test-module=error:info", Format: "%{message}"},
			expectedRegexp: "^$",
		},
		{
			desc:           "json",
			Config:         flogging.Config{LogSpec: "info", Format: "json"},
			expectedRegexp: `{"level":"warn","ts":\d+\.\d+,"name":"test-module","caller":"flogging/logging_test.go:\d+","msg":"this is a warning message"}`,
		},
		{
			desc:   "bad log spec",
			Config: flogging.Config{LogSpec: "::=borken=::", Format: "%{message}"},
			err:    errors.New("invalid logging specification '::=borken=::': bad segment '=borken='"),
		},
		{
			desc:   "bad format",
			Config: flogging.Config{LogSpec: "info", Format: "%{color:bad}"},
			err:    errors.New("invalid color option: bad"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			buf := &bytes.Buffer{}
			tc.Config.Writer = buf

			logging.ResetLevels()
			err := logging.Apply(tc.Config)
			if tc.err != nil {
				assert.EqualError(t, err, tc.err.Error())
				return
			}
			assert.NoError(t, err)

			logger := logging.Logger("test-module")
			logger.Debug("this is a debug message")
			logger.Warn("this is a warning message")

			assert.Regexp(t, tc.expectedRegexp, buf.String())
		})
	}
}

//go:generate counterfeiter -o mock/write_syncer.go -fake-name WriteSyncer . writeSyncer
type writeSyncer interface{ zapcore.WriteSyncer }

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
