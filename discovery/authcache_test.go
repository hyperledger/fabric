/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package discovery

import (
	"encoding/asn1"
	"errors"
	"sync"
	"testing"

	"github.com/hyperledger/fabric/protos/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestSignedDataToKey(t *testing.T) {
	key1, err1 := signedDataToKey(common.SignedData{
		Data:      []byte{1, 2, 3, 4},
		Identity:  []byte{5, 6, 7},
		Signature: []byte{8, 9},
	})
	key2, err2 := signedDataToKey(common.SignedData{
		Data:      []byte{1, 2, 3},
		Identity:  []byte{4, 5, 6},
		Signature: []byte{7, 8, 9},
	})
	assert.NoError(t, err1)
	assert.NoError(t, err2)
	assert.NotEqual(t, key1, key2)
}

type mockAcSupport struct {
	mock.Mock
}

func (as *mockAcSupport) EligibleForService(channel string, data common.SignedData) error {
	return as.Called(channel, data).Error(0)
}

func (as *mockAcSupport) ConfigSequence(channel string) uint64 {
	return as.Called(channel).Get(0).(uint64)
}

func TestCacheDisabled(t *testing.T) {
	sd := common.SignedData{
		Data:      []byte{1, 2, 3},
		Identity:  []byte("authorizedIdentity"),
		Signature: []byte{1, 2, 3},
	}

	as := &mockAcSupport{}
	as.On("ConfigSequence", "foo").Return(uint64(0))
	as.On("EligibleForService", "foo", sd).Return(nil)
	cache := newAuthCache(as, authCacheConfig{maxCacheSize: 100, purgeRetentionRatio: 0.5})

	// Call the cache twice with the same argument and ensure the call isn't cached
	cache.EligibleForService("foo", sd)
	cache.EligibleForService("foo", sd)
	as.AssertNumberOfCalls(t, "EligibleForService", 2)
}

func TestCacheUsage(t *testing.T) {
	as := &mockAcSupport{}
	as.On("ConfigSequence", "foo").Return(uint64(0))
	as.On("ConfigSequence", "bar").Return(uint64(0))
	cache := newAuthCache(as, defaultConfig())

	sd1 := common.SignedData{
		Data:      []byte{1, 2, 3},
		Identity:  []byte("authorizedIdentity"),
		Signature: []byte{1, 2, 3},
	}

	sd2 := common.SignedData{
		Data:      []byte{1, 2, 3},
		Identity:  []byte("authorizedIdentity"),
		Signature: []byte{1, 2, 3},
	}

	sd3 := common.SignedData{
		Data:      []byte{1, 2, 3, 3},
		Identity:  []byte("unAuthorizedIdentity"),
		Signature: []byte{1, 2, 3},
	}

	testCases := []struct {
		channel     string
		expectedErr error
		sd          common.SignedData
	}{
		{
			sd:      sd1,
			channel: "foo",
		},
		{
			sd:      sd2,
			channel: "bar",
		},
		{
			channel:     "bar",
			sd:          sd3,
			expectedErr: errors.New("user revoked"),
		},
	}

	for _, tst := range testCases {
		// Scenario I: Invocation is not cached
		invoked := false
		as.On("EligibleForService", tst.channel, tst.sd).Return(tst.expectedErr).Once().Run(func(_ mock.Arguments) {
			invoked = true
		})
		t.Run("Not cached test", func(t *testing.T) {
			err := cache.EligibleForService(tst.channel, tst.sd)
			if tst.expectedErr == nil {
				assert.NoError(t, err)
			} else {
				assert.Equal(t, tst.expectedErr.Error(), err.Error())
			}
			assert.True(t, invoked)
			// Reset invoked to false for next test
			invoked = false
		})

		// Scenario II: Invocation is cached.
		// We don't define the mock invocation because it should be the same as last time.
		// If the cache isn't used, the test would fail because the mock wasn't defined
		t.Run("Cached test", func(t *testing.T) {
			err := cache.EligibleForService(tst.channel, tst.sd)
			if tst.expectedErr == nil {
				assert.NoError(t, err)
			} else {
				assert.Equal(t, tst.expectedErr.Error(), err.Error())
			}
			assert.False(t, invoked)
		})
	}
}

func TestCacheMarshalFailure(t *testing.T) {
	as := &mockAcSupport{}
	cache := newAuthCache(as, defaultConfig())
	asBytes = func(_ interface{}) ([]byte, error) {
		return nil, errors.New("failed marshaling ASN1")
	}
	defer func() {
		asBytes = asn1.Marshal
	}()
	err := cache.EligibleForService("mychannel", common.SignedData{})
	assert.Contains(t, err.Error(), "failed marshaling ASN1")
}

func TestCacheConfigChange(t *testing.T) {
	as := &mockAcSupport{}
	sd := common.SignedData{
		Data:      []byte{1, 2, 3},
		Identity:  []byte("identity"),
		Signature: []byte{1, 2, 3},
	}

	cache := newAuthCache(as, defaultConfig())

	// Scenario I: At first, the identity is authorized
	as.On("EligibleForService", "mychannel", sd).Return(nil).Once()
	as.On("ConfigSequence", "mychannel").Return(uint64(0)).Times(2)
	err := cache.EligibleForService("mychannel", sd)
	assert.NoError(t, err)

	// Scenario II: The identity is still authorized, and config hasn't changed yet.
	// Result should be cached
	as.On("ConfigSequence", "mychannel").Return(uint64(0)).Once()
	err = cache.EligibleForService("mychannel", sd)
	assert.NoError(t, err)

	// Scenario III: A config change occurred, cache should be disregarded
	as.On("ConfigSequence", "mychannel").Return(uint64(1)).Times(2)
	as.On("EligibleForService", "mychannel", sd).Return(errors.New("unauthorized")).Once()
	err = cache.EligibleForService("mychannel", sd)
	assert.Contains(t, err.Error(), "unauthorized")
}

func TestCachePurgeCache(t *testing.T) {
	as := &mockAcSupport{}
	cache := newAuthCache(as, authCacheConfig{maxCacheSize: 4, purgeRetentionRatio: 0.75, enabled: true})
	as.On("ConfigSequence", "mychannel").Return(uint64(0))

	// Warm up the cache - attempt to place 4 identities to fill it up
	for _, id := range []string{"identity1", "identity2", "identity3", "identity4"} {
		sd := common.SignedData{
			Data:      []byte{1, 2, 3},
			Identity:  []byte(id),
			Signature: []byte{1, 2, 3},
		}
		// At first, all identities are eligible of the service
		as.On("EligibleForService", "mychannel", sd).Return(nil).Once()
		err := cache.EligibleForService("mychannel", sd)
		assert.NoError(t, err)
	}

	// Now, ensure that at least 1 of the identities was evicted from the cache, but not all
	var evicted int
	for _, id := range []string{"identity5", "identity1", "identity2"} {
		sd := common.SignedData{
			Data:      []byte{1, 2, 3},
			Identity:  []byte(id),
			Signature: []byte{1, 2, 3},
		}
		as.On("EligibleForService", "mychannel", sd).Return(errors.New("unauthorized")).Once()
		err := cache.EligibleForService("mychannel", sd)
		if err != nil {
			evicted++
		}
	}
	assert.True(t, evicted > 0 && evicted < 4, "evicted: %d, but expected between 1 and 3 evictions", evicted)
}

func TestCacheConcurrentConfigUpdate(t *testing.T) {
	// Scenario: 2 requests for the same identity are made concurrently.
	// Both are not cached, and thus their computation results might both enter the cache.
	// The first request enters when the config sequence is 0, and a config update
	// that revokes the identity takes place in the same time the access control check of the first request is evaluated.
	// The first request's computation is stalled because of scheduling, and completes after the second,
	// which happens after the config update takes place.
	// The second request's computation result should not be overridden by the computation result
	// of the first request although the first request's computation completes after the second request.

	as := &mockAcSupport{}
	sd := common.SignedData{
		Data:      []byte{1, 2, 3},
		Identity:  []byte{1, 2, 3},
		Signature: []byte{1, 2, 3},
	}
	var firstRequestInvoked sync.WaitGroup
	firstRequestInvoked.Add(1)
	var firstRequestFinished sync.WaitGroup
	firstRequestFinished.Add(1)
	var secondRequestFinished sync.WaitGroup
	secondRequestFinished.Add(1)
	cache := newAuthCache(as, defaultConfig())
	// At first, the identity is eligible.
	as.On("EligibleForService", "mychannel", mock.Anything).Return(nil).Once().Run(func(_ mock.Arguments) {
		firstRequestInvoked.Done()
		secondRequestFinished.Wait()
	})
	// But after the config change, it is not
	as.On("EligibleForService", "mychannel", mock.Anything).Return(errors.New("unauthorized")).Once()
	// The config sequence the first request sees
	as.On("ConfigSequence", "mychannel").Return(uint64(0)).Once()
	as.On("ConfigSequence", "mychannel").Return(uint64(1)).Once()
	// The config sequence the second request sees
	as.On("ConfigSequence", "mychannel").Return(uint64(1)).Times(2)

	// First request returns OK
	go func() {
		defer firstRequestFinished.Done()
		firstResult := cache.EligibleForService("mychannel", sd)
		assert.NoError(t, firstResult)
	}()
	firstRequestInvoked.Wait()
	// Second request returns that the identity isn't authorized
	secondResult := cache.EligibleForService("mychannel", sd)
	// Mark second request as finished to signal first request to finish its computation
	secondRequestFinished.Done()
	// Wait for first request to return
	firstRequestFinished.Wait()
	assert.Contains(t, secondResult.Error(), "unauthorized")

	// Now make another request and ensure that the second request's result (an-authorized) was cached,
	// even though it finished before the first request.
	as.On("ConfigSequence", "mychannel").Return(uint64(1)).Once()
	cachedResult := cache.EligibleForService("mychannel", sd)
	assert.Contains(t, cachedResult.Error(), "unauthorized")
}

func defaultConfig() authCacheConfig {
	return authCacheConfig{maxCacheSize: defaultMaxCacheSize, purgeRetentionRatio: defaultRetentionRatio, enabled: true}
}
