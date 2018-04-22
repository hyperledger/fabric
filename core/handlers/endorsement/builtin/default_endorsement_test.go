/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package builtin_test

import (
	"testing"

	"github.com/hyperledger/fabric/core/endorser/mocks"
	"github.com/hyperledger/fabric/core/handlers/endorsement/builtin"
	mocks2 "github.com/hyperledger/fabric/core/handlers/endorsement/builtin/mocks"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestDefaultEndorsement(t *testing.T) {
	factory := &builtin.DefaultEndorsementFactory{}
	endorser := factory.New()

	// Scenario I: Don't pass any dependencies, and observe that the initialization fails
	err := endorser.Init()
	assert.Equal(t, "could not find SigningIdentityFetcher in dependencies", err.Error())

	// Scenario II: Pass into init a SigningIdentityFetcher
	sif := &mocks.SigningIdentityFetcher{}
	// Pass also another item just to ensure it is ignored
	err = endorser.Init("foo", sif)
	assert.NoError(t, err)

	// Scenario III: Obtaining a signing identity fails
	sif.On("SigningIdentityForRequest", mock.Anything).Return(nil, errors.New("foo")).Once()
	_, _, err = endorser.Endorse(nil, nil)
	assert.Contains(t, err.Error(), "foo")

	// Scenario IV: Obtaining a signing identity succeeds but serializing the identity fails
	sid := &mocks2.SigningIdentity{}
	sid.On("Serialize").Return(nil, errors.New("bar")).Once()
	sif.On("SigningIdentityForRequest", mock.Anything).Return(sid, nil)
	_, _, err = endorser.Endorse(nil, nil)
	assert.Contains(t, err.Error(), "bar")

	// Scenario V: Serializing the identity succeeds but signing fails
	sid.On("Serialize").Return([]byte{1, 2, 3}, nil)
	sid.On("Sign", mock.Anything).Return(nil, errors.New("baz")).Once()
	_, _, err = endorser.Endorse([]byte{1, 1, 1, 1, 1}, nil)
	assert.Contains(t, err.Error(), "baz")

	// Scenario VI: Signing succeeds
	sid.On("Serialize").Return([]byte{1, 2, 3}, nil)
	sid.On("Sign", mock.Anything).Return([]byte{10, 20, 30}, nil).Once()
	endorsement, resp, err := endorser.Endorse([]byte{1, 1, 1, 1, 1}, nil)
	assert.NoError(t, err)
	assert.Equal(t, resp, []byte{1, 1, 1, 1, 1})
	assert.Equal(t, &peer.Endorsement{
		Signature: []byte{10, 20, 30},
		Endorser:  []byte{1, 2, 3},
	}, endorsement)
}
