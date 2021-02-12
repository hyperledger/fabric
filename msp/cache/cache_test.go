/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cache

import (
	"sync"
	"testing"

	msp2 "github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/msp/mocks"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestNewCacheMsp(t *testing.T) {
	i, err := New(nil)
	require.Error(t, err)
	require.Nil(t, i)
	require.Contains(t, err.Error(), "Invalid passed MSP. It must be different from nil.")

	i, err = New(&mocks.MockMSP{})
	require.NoError(t, err)
	require.NotNil(t, i)
}

func TestSetup(t *testing.T) {
	mockMSP := &mocks.MockMSP{}
	i, err := New(mockMSP)
	require.NoError(t, err)

	mockMSP.On("Setup", (*msp2.MSPConfig)(nil)).Return(nil)
	err = i.Setup(nil)
	require.NoError(t, err)
	mockMSP.AssertExpectations(t)
	require.Equal(t, 0, i.(*cachedMSP).deserializeIdentityCache.len())
	require.Equal(t, 0, i.(*cachedMSP).satisfiesPrincipalCache.len())
	require.Equal(t, 0, i.(*cachedMSP).validateIdentityCache.len())
}

func TestGetType(t *testing.T) {
	mockMSP := &mocks.MockMSP{}
	i, err := New(mockMSP)
	require.NoError(t, err)

	mockMSP.On("GetType").Return(msp.FABRIC)
	require.Equal(t, msp.FABRIC, i.GetType())
	mockMSP.AssertExpectations(t)
}

func TestGetIdentifier(t *testing.T) {
	mockMSP := &mocks.MockMSP{}
	i, err := New(mockMSP)
	require.NoError(t, err)

	mockMSP.On("GetIdentifier").Return("MSP", nil)
	id, err := i.GetIdentifier()
	require.NoError(t, err)
	require.Equal(t, "MSP", id)
	mockMSP.AssertExpectations(t)
}

func TestGetDefaultSigningIdentity(t *testing.T) {
	mockMSP := &mocks.MockMSP{}
	i, err := New(mockMSP)
	require.NoError(t, err)

	mockIdentity := &mocks.MockSigningIdentity{Mock: mock.Mock{}, MockIdentity: &mocks.MockIdentity{ID: "Alice"}}
	mockMSP.On("GetDefaultSigningIdentity").Return(mockIdentity, nil)
	id, err := i.GetDefaultSigningIdentity()
	require.NoError(t, err)
	require.Equal(t, mockIdentity, id)
	mockMSP.AssertExpectations(t)
}

func TestGetTLSRootCerts(t *testing.T) {
	mockMSP := &mocks.MockMSP{}
	i, err := New(mockMSP)
	require.NoError(t, err)

	expected := [][]byte{{1}, {2}}
	mockMSP.On("GetTLSRootCerts").Return(expected)
	certs := i.GetTLSRootCerts()
	require.Equal(t, expected, certs)
}

func TestGetTLSIntermediateCerts(t *testing.T) {
	mockMSP := &mocks.MockMSP{}
	i, err := New(mockMSP)
	require.NoError(t, err)

	expected := [][]byte{{1}, {2}}
	mockMSP.On("GetTLSIntermediateCerts").Return(expected)
	certs := i.GetTLSIntermediateCerts()
	require.Equal(t, expected, certs)
}

func TestDeserializeIdentity(t *testing.T) {
	mockMSP := &mocks.MockMSP{}
	wrappedMSP, err := New(mockMSP)
	require.NoError(t, err)

	// Check id is cached
	mockIdentity := &mocks.MockIdentity{ID: "Alice"}
	mockIdentity2 := &mocks.MockIdentity{ID: "Bob"}
	serializedIdentity := []byte{1, 2, 3}
	serializedIdentity2 := []byte{4, 5, 6}
	mockMSP.On("DeserializeIdentity", serializedIdentity).Return(mockIdentity, nil)
	mockMSP.On("DeserializeIdentity", serializedIdentity2).Return(mockIdentity2, nil)
	// Prime the cache
	wrappedMSP.DeserializeIdentity(serializedIdentity)
	wrappedMSP.DeserializeIdentity(serializedIdentity2)

	// Stress the cache and ensure concurrent operations
	// do not result in a failure
	var wg sync.WaitGroup
	wg.Add(100)
	for i := 0; i < 100; i++ {
		go func(m msp.MSP, i int) {
			sIdentity := serializedIdentity
			expectedIdentity := mockIdentity
			defer wg.Done()
			if i%2 == 0 {
				sIdentity = serializedIdentity2
				expectedIdentity = mockIdentity2
			}
			id, err := wrappedMSP.DeserializeIdentity(sIdentity)
			require.NoError(t, err)
			require.Equal(t, expectedIdentity, id.(*cachedIdentity).Identity)
		}(wrappedMSP, i)
	}
	wg.Wait()

	mockMSP.AssertExpectations(t)
	// Check the cache
	_, ok := wrappedMSP.(*cachedMSP).deserializeIdentityCache.get(string(serializedIdentity))
	require.True(t, ok)

	// Check the same object is returned
	id, err := wrappedMSP.DeserializeIdentity(serializedIdentity)
	require.NoError(t, err)
	require.True(t, mockIdentity == id.(*cachedIdentity).Identity)
	mockMSP.AssertExpectations(t)

	// Check id is not cached
	mockIdentity = &mocks.MockIdentity{ID: "Bob"}
	serializedIdentity = []byte{1, 2, 3, 4}
	mockMSP.On("DeserializeIdentity", serializedIdentity).Return(mockIdentity, errors.New("Invalid identity"))
	_, err = wrappedMSP.DeserializeIdentity(serializedIdentity)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid identity")
	mockMSP.AssertExpectations(t)

	_, ok = wrappedMSP.(*cachedMSP).deserializeIdentityCache.get(string(serializedIdentity))
	require.False(t, ok)
}

func TestValidate(t *testing.T) {
	mockMSP := &mocks.MockMSP{}
	i, err := New(mockMSP)
	require.NoError(t, err)

	// Check validation is cached
	mockIdentity := &mocks.MockIdentity{ID: "Alice"}
	mockIdentity.On("GetIdentifier").Return(&msp.IdentityIdentifier{Mspid: "MSP", Id: "Alice"})
	mockMSP.On("Validate", mockIdentity).Return(nil)
	err = i.Validate(mockIdentity)
	require.NoError(t, err)
	mockIdentity.AssertExpectations(t)
	mockMSP.AssertExpectations(t)
	// Check the cache
	identifier := mockIdentity.GetIdentifier()
	key := string(identifier.Mspid + ":" + identifier.Id)
	v, ok := i.(*cachedMSP).validateIdentityCache.get(string(key))
	require.True(t, ok)
	require.True(t, v.(bool))

	// Recheck
	err = i.Validate(mockIdentity)
	require.NoError(t, err)

	// Check validation is not cached
	mockIdentity = &mocks.MockIdentity{ID: "Bob"}
	mockIdentity.On("GetIdentifier").Return(&msp.IdentityIdentifier{Mspid: "MSP", Id: "Bob"})
	mockMSP.On("Validate", mockIdentity).Return(errors.New("Invalid identity"))
	err = i.Validate(mockIdentity)
	require.Error(t, err)
	mockIdentity.AssertExpectations(t)
	mockMSP.AssertExpectations(t)
	// Check the cache
	identifier = mockIdentity.GetIdentifier()
	key = string(identifier.Mspid + ":" + identifier.Id)
	_, ok = i.(*cachedMSP).validateIdentityCache.get(string(key))
	require.False(t, ok)
}

func TestSatisfiesValidateIndirectCall(t *testing.T) {
	mockMSP := &mocks.MockMSP{}

	mockIdentity := &mocks.MockIdentity{ID: "Alice"}
	mockIdentity.On("Validate").Run(func(_ mock.Arguments) {
		panic("shouldn't have invoked the identity method")
	})
	mockMSP.On("DeserializeIdentity", mock.Anything).Return(mockIdentity, nil).Once()
	mockIdentity.On("GetIdentifier").Return(&msp.IdentityIdentifier{Mspid: "MSP", Id: "Alice"})

	cache, err := New(mockMSP)
	require.NoError(t, err)

	mockMSP.On("Validate", mockIdentity).Return(nil)

	// Test that cache returns the correct value, and also use this to prime the cache
	err = cache.Validate(mockIdentity)
	mockMSP.AssertNumberOfCalls(t, "Validate", 1)
	require.NoError(t, err)
	// Get the identity we test the caching on
	identity, err := cache.DeserializeIdentity([]byte{1, 2, 3})
	require.NoError(t, err)
	// Ensure the identity returned answers what the cached MSP answers.
	err = identity.Validate()
	require.NoError(t, err)
	// Ensure that although a call to Validate was called, the calls weren't passed on to the backing MSP
	mockMSP.AssertNumberOfCalls(t, "Validate", 1)
}

func TestSatisfiesPrincipalIndirectCall(t *testing.T) {
	mockMSP := &mocks.MockMSP{}
	mockMSPPrincipal := &msp2.MSPPrincipal{PrincipalClassification: msp2.MSPPrincipal_IDENTITY, Principal: []byte{1, 2, 3}}

	mockIdentity := &mocks.MockIdentity{ID: "Alice"}
	mockIdentity.On("SatisfiesPrincipal", mockMSPPrincipal).Run(func(_ mock.Arguments) {
		panic("shouldn't have invoked the identity method")
	})
	mockMSP.On("DeserializeIdentity", mock.Anything).Return(mockIdentity, nil).Once()
	mockIdentity.On("GetIdentifier").Return(&msp.IdentityIdentifier{Mspid: "MSP", Id: "Alice"})

	cache, err := New(mockMSP)
	require.NoError(t, err)

	// First invocation of the SatisfiesPrincipal returns an error
	mockMSP.On("SatisfiesPrincipal", mockIdentity, mockMSPPrincipal).Return(errors.New("error: foo")).Once()
	// Second invocation returns nil
	mockMSP.On("SatisfiesPrincipal", mockIdentity, mockMSPPrincipal).Return(nil).Once()

	// Test that cache returns the correct value
	err = cache.SatisfiesPrincipal(mockIdentity, mockMSPPrincipal)
	require.Equal(t, "error: foo", err.Error())
	// Get the identity we test the caching on
	identity, err := cache.DeserializeIdentity([]byte{1, 2, 3})
	require.NoError(t, err)
	// Ensure the identity returned answers what the cached MSP answers.
	// If the invocation doesn't hit the cache, it will return nil instead of an error.
	err = identity.SatisfiesPrincipal(mockMSPPrincipal)
	require.Equal(t, "error: foo", err.Error())
}

func TestSatisfiesPrincipal(t *testing.T) {
	mockMSP := &mocks.MockMSP{}
	i, err := New(mockMSP)
	require.NoError(t, err)

	// Check validation is cached
	mockIdentity := &mocks.MockIdentity{ID: "Alice"}
	mockIdentity.On("GetIdentifier").Return(&msp.IdentityIdentifier{Mspid: "MSP", Id: "Alice"})
	mockMSPPrincipal := &msp2.MSPPrincipal{PrincipalClassification: msp2.MSPPrincipal_IDENTITY, Principal: []byte{1, 2, 3}}
	mockMSP.On("SatisfiesPrincipal", mockIdentity, mockMSPPrincipal).Return(nil)
	mockMSP.SatisfiesPrincipal(mockIdentity, mockMSPPrincipal)
	err = i.SatisfiesPrincipal(mockIdentity, mockMSPPrincipal)
	require.NoError(t, err)
	mockIdentity.AssertExpectations(t)
	mockMSP.AssertExpectations(t)
	// Check the cache
	identifier := mockIdentity.GetIdentifier()
	identityKey := string(identifier.Mspid + ":" + identifier.Id)
	principalKey := string(mockMSPPrincipal.PrincipalClassification) + string(mockMSPPrincipal.Principal)
	key := identityKey + principalKey
	v, ok := i.(*cachedMSP).satisfiesPrincipalCache.get(key)
	require.True(t, ok)
	require.Nil(t, v)

	// Recheck
	err = i.SatisfiesPrincipal(mockIdentity, mockMSPPrincipal)
	require.NoError(t, err)

	// Check validation is not cached
	mockIdentity = &mocks.MockIdentity{ID: "Bob"}
	mockIdentity.On("GetIdentifier").Return(&msp.IdentityIdentifier{Mspid: "MSP", Id: "Bob"})
	mockMSPPrincipal = &msp2.MSPPrincipal{PrincipalClassification: msp2.MSPPrincipal_IDENTITY, Principal: []byte{1, 2, 3, 4}}
	mockMSP.On("SatisfiesPrincipal", mockIdentity, mockMSPPrincipal).Return(errors.New("Invalid"))
	mockMSP.SatisfiesPrincipal(mockIdentity, mockMSPPrincipal)
	err = i.SatisfiesPrincipal(mockIdentity, mockMSPPrincipal)
	require.Error(t, err)
	mockIdentity.AssertExpectations(t)
	mockMSP.AssertExpectations(t)
	// Check the cache
	identifier = mockIdentity.GetIdentifier()
	identityKey = string(identifier.Mspid + ":" + identifier.Id)
	principalKey = string(mockMSPPrincipal.PrincipalClassification) + string(mockMSPPrincipal.Principal)
	key = identityKey + principalKey
	v, ok = i.(*cachedMSP).satisfiesPrincipalCache.get(key)
	require.True(t, ok)
	require.NotNil(t, v)
	require.Contains(t, "Invalid", v.(error).Error())
}
