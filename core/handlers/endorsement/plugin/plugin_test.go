/*
Copyright Hitachi, Ltd. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main_test

import (
	"errors"
	"testing"

	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/core/endorser/mocks"
	mocks2 "github.com/hyperledger/fabric/core/handlers/endorsement/builtin/mocks"
	plgn "github.com/hyperledger/fabric/core/handlers/endorsement/plugin"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestEndorsementPlugin(t *testing.T) {
	factory := plgn.NewPluginFactory()
	plugin := factory.New()
	dependency := &struct{}{}
	err := plugin.Init(dependency)
	require.EqualError(t, err, "could not find SigningIdentityFetcher in dependencies")

	sif := &mocks.SigningIdentityFetcher{}
	err1 := plugin.Init(sif)
	require.NoError(t, err1)

	// For each test, mock methods are called only once. Check it for them.
	// SigningIdentity fails
	sif.On("SigningIdentityForRequest", mock.Anything).Return(nil, errors.New("signingIdentityForRequestReturnsError")).Once()
	endorsement2, prepBytes2, err2 := plugin.Endorse(nil, nil)
	require.Nil(t, endorsement2)
	require.Nil(t, prepBytes2)
	require.EqualError(t, err2, "failed fetching signing identity: signingIdentityForRequestReturnsError")

	// Serializing the identity fails
	sid := &mocks2.SigningIdentity{}
	sid.On("Serialize").Return(nil, errors.New("serializeReturnsError")).Once()
	sif.On("SigningIdentityForRequest", mock.Anything).Return(sid, nil).Once()
	endorsement3, prepBytes3, err3 := plugin.Endorse(nil, nil)
	require.Nil(t, endorsement3)
	require.Nil(t, prepBytes3)
	require.EqualError(t, err3, "could not serialize the signing identity: serializeReturnsError")

	// Signing fails
	sid.On("Serialize").Return([]byte("Endorser4"), nil).Once()
	sid.On("Sign", mock.Anything).Return(nil, errors.New("signReturnsError")).Once()
	sif.On("SigningIdentityForRequest", mock.Anything).Return(sid, nil).Once()
	endorsement4, prepBytes4, err4 := plugin.Endorse([]byte("prpBytes4"), nil)
	require.Nil(t, endorsement4)
	require.Nil(t, prepBytes4)
	require.EqualError(t, err4, "could not sign the proposal response payload: signReturnsError")

	// Success
	sid.On("Serialize").Return([]byte("Endorser5"), nil).Once()
	sid.On("Sign", mock.Anything).Return([]byte("Signature5"), nil).Once()
	sif.On("SigningIdentityForRequest", mock.Anything).Return(sid, nil).Once()
	endorsement5, prpBytes5, err5 := plugin.Endorse([]byte("prpBytes5"), nil)
	expected5 := &peer.Endorsement{Signature: []byte("Signature5"), Endorser: []byte("Endorser5")}
	require.NoError(t, err5)
	require.Equal(t, expected5, endorsement5)
	require.Equal(t, []byte("prpBytes5"), prpBytes5)
}
