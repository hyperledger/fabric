/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package crypto

import (
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func bytesFromArgs(args mock.Arguments) ([]byte, error) {
	if args.Get(1) != nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]byte), nil
}

type mockSigner struct {
	mock.Mock
}

func (s *mockSigner) Sign(message []byte) ([]byte, error) {
	return bytesFromArgs(s.Called(message))
}

type mockIdentitySerializer struct {
	mock.Mock
}

func (is *mockIdentitySerializer) Serialize() ([]byte, error) {
	return bytesFromArgs(is.Called())
}

type signerSupport struct {
	Signer
	IdentitySerializer
}

func TestCLISignerNewSignatureHeader(t *testing.T) {
	tests := []struct {
		name           string
		signError      error
		serializeError error
	}{
		{
			name:           "SerializeFailure",
			serializeError: errors.New("failed1"),
		},
		{
			name:           "SignFailure",
			serializeError: errors.New("failed2"),
		},
		{
			name: "Success",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			s := &mockSigner{}
			s.On("Sign", mock.Anything).Return([]byte{1, 2, 3}, test.signError)
			is := &mockIdentitySerializer{}
			is.On("Serialize", mock.Anything).Return([]byte{1, 2, 3}, test.serializeError)
			signer := NewSignatureHeaderCreator(&signerSupport{
				Signer:             s,
				IdentitySerializer: is,
			})
			sh, err := signer.NewSignatureHeader()
			if test.serializeError == nil && test.signError == nil {
				assert.NoError(t, err)
				assert.NotNil(t, sh)
				return
			}
			assert.Error(t, err)
			assert.Nil(t, sh)
		})
	}
}
