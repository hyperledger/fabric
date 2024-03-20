// Code generated by counterfeiter. DO NOT EDIT.
package mocks

import (
	"sync"

	"github.com/hyperledger/fabric/v3/protoutil"
)

type Verifier struct {
	VerifyByChannelStub        func(string, *protoutil.SignedData) error
	verifyByChannelMutex       sync.RWMutex
	verifyByChannelArgsForCall []struct {
		arg1 string
		arg2 *protoutil.SignedData
	}
	verifyByChannelReturns struct {
		result1 error
	}
	verifyByChannelReturnsOnCall map[int]struct {
		result1 error
	}
	invocations      map[string][][]interface{}
	invocationsMutex sync.RWMutex
}

func (fake *Verifier) VerifyByChannel(arg1 string, arg2 *protoutil.SignedData) error {
	fake.verifyByChannelMutex.Lock()
	ret, specificReturn := fake.verifyByChannelReturnsOnCall[len(fake.verifyByChannelArgsForCall)]
	fake.verifyByChannelArgsForCall = append(fake.verifyByChannelArgsForCall, struct {
		arg1 string
		arg2 *protoutil.SignedData
	}{arg1, arg2})
	fake.recordInvocation("VerifyByChannel", []interface{}{arg1, arg2})
	fake.verifyByChannelMutex.Unlock()
	if fake.VerifyByChannelStub != nil {
		return fake.VerifyByChannelStub(arg1, arg2)
	}
	if specificReturn {
		return ret.result1
	}
	fakeReturns := fake.verifyByChannelReturns
	return fakeReturns.result1
}

func (fake *Verifier) VerifyByChannelCallCount() int {
	fake.verifyByChannelMutex.RLock()
	defer fake.verifyByChannelMutex.RUnlock()
	return len(fake.verifyByChannelArgsForCall)
}

func (fake *Verifier) VerifyByChannelCalls(stub func(string, *protoutil.SignedData) error) {
	fake.verifyByChannelMutex.Lock()
	defer fake.verifyByChannelMutex.Unlock()
	fake.VerifyByChannelStub = stub
}

func (fake *Verifier) VerifyByChannelArgsForCall(i int) (string, *protoutil.SignedData) {
	fake.verifyByChannelMutex.RLock()
	defer fake.verifyByChannelMutex.RUnlock()
	argsForCall := fake.verifyByChannelArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2
}

func (fake *Verifier) VerifyByChannelReturns(result1 error) {
	fake.verifyByChannelMutex.Lock()
	defer fake.verifyByChannelMutex.Unlock()
	fake.VerifyByChannelStub = nil
	fake.verifyByChannelReturns = struct {
		result1 error
	}{result1}
}

func (fake *Verifier) VerifyByChannelReturnsOnCall(i int, result1 error) {
	fake.verifyByChannelMutex.Lock()
	defer fake.verifyByChannelMutex.Unlock()
	fake.VerifyByChannelStub = nil
	if fake.verifyByChannelReturnsOnCall == nil {
		fake.verifyByChannelReturnsOnCall = make(map[int]struct {
			result1 error
		})
	}
	fake.verifyByChannelReturnsOnCall[i] = struct {
		result1 error
	}{result1}
}

func (fake *Verifier) Invocations() map[string][][]interface{} {
	fake.invocationsMutex.RLock()
	defer fake.invocationsMutex.RUnlock()
	fake.verifyByChannelMutex.RLock()
	defer fake.verifyByChannelMutex.RUnlock()
	copiedInvocations := map[string][][]interface{}{}
	for key, value := range fake.invocations {
		copiedInvocations[key] = value
	}
	return copiedInvocations
}

func (fake *Verifier) recordInvocation(key string, args []interface{}) {
	fake.invocationsMutex.Lock()
	defer fake.invocationsMutex.Unlock()
	if fake.invocations == nil {
		fake.invocations = map[string][][]interface{}{}
	}
	if fake.invocations[key] == nil {
		fake.invocations[key] = [][]interface{}{}
	}
	fake.invocations[key] = append(fake.invocations[key], args)
}
