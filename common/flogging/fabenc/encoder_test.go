/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabenc_test

import (
	"errors"
	"testing"

	"github.com/hyperledger/fabric/common/flogging/fabenc"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"go.uber.org/zap/buffer"
	"go.uber.org/zap/zapcore"
)

func TestNewFormatEncoderBadFormat(t *testing.T) {
	_, err := fabenc.NewFormatEncoder("%{color:garbage}")
	assert.EqualError(t, err, "invalid color option: garbage")
}

func TestEncodeEntry(t *testing.T) {
	var tests = []struct {
		name     string
		spec     string
		fields   []zapcore.Field
		expected string
	}{
		{name: "empty spec and nil fields", spec: "", fields: nil, expected: "\n"},
		{name: "empty spec with fields", spec: "", fields: []zapcore.Field{zap.String("key", "value")}, expected: "{\"key\": \"value\"}\n"},
		{name: "simple spec and nil fields", spec: "simple-string", expected: "simple-string\n"},
		{name: "simple spec and empty fields", spec: "simple-string", fields: []zapcore.Field{}, expected: "simple-string\n"},
		{name: "simple spec with fields", spec: "simple-string", fields: []zapcore.Field{zap.String("key", "value")}, expected: "simple-string {\"key\": \"value\"}\n"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			enc, err := fabenc.NewFormatEncoder(tc.spec)
			assert.NoError(t, err)

			line, err := enc.EncodeEntry(zapcore.Entry{}, tc.fields)
			assert.NoError(t, err)
			assert.Equal(t, tc.expected, line.String())
		})
	}
}

type brokenEncoder struct{ zapcore.Encoder }

func (b *brokenEncoder) EncodeEntry(zapcore.Entry, []zapcore.Field) (*buffer.Buffer, error) {
	return nil, errors.New("broken encoder")
}

func TestEncodeFieldsFailed(t *testing.T) {
	enc, err := fabenc.NewFormatEncoder("spec")
	assert.NoError(t, err)
	enc.Encoder = &brokenEncoder{}

	_, err = enc.EncodeEntry(zapcore.Entry{}, nil)
	assert.EqualError(t, err, "broken encoder")
}

func TestFormatEncoderClone(t *testing.T) {
	enc, err := fabenc.NewFormatEncoder("spec")
	assert.NoError(t, err)
	cloned := enc.Clone()
	assert.Equal(t, enc, cloned)
}
