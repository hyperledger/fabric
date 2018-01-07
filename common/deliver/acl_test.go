/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package deliver

import (
	"testing"
	"time"

	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockACSupport struct {
	mock.Mock
}

func (s *mockACSupport) ExpiresAt(identityBytes []byte) time.Time {
	return s.Called().Get(0).(time.Time)
}

func (s *mockACSupport) Sequence() uint64 {
	return s.Called().Get(0).(uint64)
}

func createEnvelope() *common.Envelope {
	chHdr := utils.MakeChannelHeader(common.HeaderType_DELIVER_SEEK_INFO, 0, "mychannel", 0)
	siHdr := utils.MakeSignatureHeader(nil, nil)
	paylBytes := utils.MarshalOrPanic(&common.Payload{
		Header: utils.MakePayloadHeader(chHdr, siHdr),
	})

	return &common.Envelope{Payload: paylBytes}
}

type oneTimeInvoke struct {
	f       func(*common.Envelope, string) error
	invoked bool
}

func (oti *oneTimeInvoke) invokeOnce() func(*common.Envelope, string) error {
	return func(env *common.Envelope, s string) error {
		if oti.invoked {
			panic("already invoked!")
		}
		oti.invoked = true
		return oti.f(env, s)
	}
}

func oneTimeFunction(f func(*common.Envelope, string) error) func(*common.Envelope, string) error {
	oti := &oneTimeInvoke{f: f}
	return oti.invokeOnce()
}

func TestOneTimeFunction(t *testing.T) {
	acceptPolicyChecker := func(envelope *common.Envelope, channelID string) error {
		return nil
	}
	f := oneTimeFunction(acceptPolicyChecker)
	// First time no panic
	assert.NotPanics(t, func() {
		f(nil, "")
	})

	// Second time we panic
	assert.Panics(t, func() {
		f(nil, "")
	})
}

func TestAC(t *testing.T) {
	acceptPolicyChecker := func(envelope *common.Envelope, channelID string) error {
		return nil
	}

	denyPolicyChecker := func(envelope *common.Envelope, channelID string) error {
		return errors.New("forbidden")
	}

	sup := &mockACSupport{}
	// Scenario I: create empty header
	ac, err := newSessionAC(sup, &common.Envelope{}, nil, "mychannel", sup.ExpiresAt)
	assert.Nil(t, ac)
	assert.Contains(t, err.Error(), "Missing Header")

	// Scenario II: Identity has expired.
	sup = &mockACSupport{}
	sup.On("ExpiresAt").Return(time.Now().Add(-1 * time.Second)).Once()
	ac, err = newSessionAC(sup, createEnvelope(), oneTimeFunction(acceptPolicyChecker), "mychannel", sup.ExpiresAt)
	assert.NotNil(t, ac)
	assert.NoError(t, err)
	err = ac.evaluate()
	assert.Contains(t, err.Error(), "expired")

	// Scenario III: Identity hasn't expired, but is forbidden
	sup = &mockACSupport{}
	sup.On("ExpiresAt").Return(time.Now().Add(time.Second)).Once()
	sup.On("Sequence").Return(uint64(0)).Once()
	ac, err = newSessionAC(sup, createEnvelope(), oneTimeFunction(denyPolicyChecker), "mychannel", sup.ExpiresAt)
	assert.NoError(t, err)
	err = ac.evaluate()
	assert.Contains(t, err.Error(), "forbidden")

	// Scenario IV: Identity hasn't expired, and is allowed
	// We actually check 2 sub-cases, the first one is if the identity can expire,
	// and the second one is if the identity can't expire (i.e an idemix identity currently can't expire)
	for _, expirationTime := range []time.Time{time.Now().Add(time.Second), {}} {
		sup = &mockACSupport{}
		sup.On("ExpiresAt").Return(expirationTime).Once()
		sup.On("Sequence").Return(uint64(0)).Once()
		ac, err = newSessionAC(sup, createEnvelope(), oneTimeFunction(acceptPolicyChecker), "mychannel", sup.ExpiresAt)
		assert.NoError(t, err)
		err = ac.evaluate()
		assert.NoError(t, err)
		// Execute again. We should not evaluate the policy again.
		// If we do, the test fails with panic because the function can be invoked only once
		sup.On("Sequence").Return(uint64(0)).Once()
		err = ac.evaluate()
		assert.NoError(t, err)
	}

	// Scenario V: Identity hasn't expired, and is allowed at first, but afterwards there
	// is a config change and afterwards it isn't allowed
	sup = &mockACSupport{}
	sup.On("ExpiresAt").Return(time.Now().Add(time.Second)).Once()
	sup.On("Sequence").Return(uint64(0)).Once()
	sup.On("Sequence").Return(uint64(1)).Once()

	firstInvoke := true
	policyChecker := func(envelope *common.Envelope, channelID string) error {
		if firstInvoke {
			firstInvoke = false
			return nil
		}
		return errors.New("forbidden")
	}

	ac, err = newSessionAC(sup, createEnvelope(), policyChecker, "mychannel", sup.ExpiresAt)
	assert.NoError(t, err)
	err = ac.evaluate() // first time
	assert.NoError(t, err)
	err = ac.evaluate() // second time
	assert.Contains(t, err.Error(), "forbidden")

	// Scenario VI: Identity hasn't expired at first, but expires at a later time,
	// and then it shouldn't be allowed to be serviced
	sup = &mockACSupport{}
	sup.On("ExpiresAt").Return(time.Now().Add(time.Millisecond * 500)).Once()
	sup.On("Sequence").Return(uint64(0)).Times(3)
	ac, err = newSessionAC(sup, createEnvelope(), oneTimeFunction(acceptPolicyChecker), "mychannel", sup.ExpiresAt)
	assert.NoError(t, err)
	err = ac.evaluate()
	assert.NoError(t, err)
	err = ac.evaluate()
	assert.NoError(t, err)
	time.Sleep(time.Second)
	err = ac.evaluate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expired")
}
