// Code generated by counterfeiter. DO NOT EDIT.
package mocks

import (
	"github.com/hyperledger/fabric/orderer/common/types"
	"sync"

	"github.com/hyperledger/fabric/orderer/common/channelparticipation"
)

type ChannelManagement struct {
	ListAllChannelsStub        func() ([]types.ChannelInfoShort, string)
	listAllChannelsMutex       sync.RWMutex
	listAllChannelsArgsForCall []struct {
	}
	listAllChannelsReturns struct {
		result1 []types.ChannelInfoShort
		result2 string
	}
	listAllChannelsReturnsOnCall map[int]struct {
		result1 []types.ChannelInfoShort
		result2 string
	}
	ListChannelStub        func(string) (*types.ChannelInfo, error)
	listChannelMutex       sync.RWMutex
	listChannelArgsForCall []struct {
		arg1 string
	}
	listChannelReturns struct {
		result1 *types.ChannelInfo
		result2 error
	}
	listChannelReturnsOnCall map[int]struct {
		result1 *types.ChannelInfo
		result2 error
	}
	invocations      map[string][][]interface{}
	invocationsMutex sync.RWMutex
}

func (fake *ChannelManagement) ListAllChannels() ([]types.ChannelInfoShort, string) {
	fake.listAllChannelsMutex.Lock()
	ret, specificReturn := fake.listAllChannelsReturnsOnCall[len(fake.listAllChannelsArgsForCall)]
	fake.listAllChannelsArgsForCall = append(fake.listAllChannelsArgsForCall, struct {
	}{})
	fake.recordInvocation("ChannelList", []interface{}{})
	fake.listAllChannelsMutex.Unlock()
	if fake.ListAllChannelsStub != nil {
		return fake.ListAllChannelsStub()
	}
	if specificReturn {
		return ret.result1, ret.result2
	}
	fakeReturns := fake.listAllChannelsReturns
	return fakeReturns.result1, fakeReturns.result2
}

func (fake *ChannelManagement) ListAllChannelsCallCount() int {
	fake.listAllChannelsMutex.RLock()
	defer fake.listAllChannelsMutex.RUnlock()
	return len(fake.listAllChannelsArgsForCall)
}

func (fake *ChannelManagement) ListAllChannelsCalls(stub func() ([]types.ChannelInfoShort, string)) {
	fake.listAllChannelsMutex.Lock()
	defer fake.listAllChannelsMutex.Unlock()
	fake.ListAllChannelsStub = stub
}

func (fake *ChannelManagement) ListAllChannelsReturns(result1 []types.ChannelInfoShort, result2 string) {
	fake.listAllChannelsMutex.Lock()
	defer fake.listAllChannelsMutex.Unlock()
	fake.ListAllChannelsStub = nil
	fake.listAllChannelsReturns = struct {
		result1 []types.ChannelInfoShort
		result2 string
	}{result1, result2}
}

func (fake *ChannelManagement) ListAllChannelsReturnsOnCall(i int, result1 []types.ChannelInfoShort, result2 string) {
	fake.listAllChannelsMutex.Lock()
	defer fake.listAllChannelsMutex.Unlock()
	fake.ListAllChannelsStub = nil
	if fake.listAllChannelsReturnsOnCall == nil {
		fake.listAllChannelsReturnsOnCall = make(map[int]struct {
			result1 []types.ChannelInfoShort
			result2 string
		})
	}
	fake.listAllChannelsReturnsOnCall[i] = struct {
		result1 []types.ChannelInfoShort
		result2 string
	}{result1, result2}
}

func (fake *ChannelManagement) ListChannel(arg1 string) (*types.ChannelInfo, error) {
	fake.listChannelMutex.Lock()
	ret, specificReturn := fake.listChannelReturnsOnCall[len(fake.listChannelArgsForCall)]
	fake.listChannelArgsForCall = append(fake.listChannelArgsForCall, struct {
		arg1 string
	}{arg1})
	fake.recordInvocation("ListChannel", []interface{}{arg1})
	fake.listChannelMutex.Unlock()
	if fake.ListChannelStub != nil {
		return fake.ListChannelStub(arg1)
	}
	if specificReturn {
		return ret.result1, ret.result2
	}
	fakeReturns := fake.listChannelReturns
	return fakeReturns.result1, fakeReturns.result2
}

func (fake *ChannelManagement) ListChannelCallCount() int {
	fake.listChannelMutex.RLock()
	defer fake.listChannelMutex.RUnlock()
	return len(fake.listChannelArgsForCall)
}

func (fake *ChannelManagement) ListChannelCalls(stub func(string) (*types.ChannelInfo, error)) {
	fake.listChannelMutex.Lock()
	defer fake.listChannelMutex.Unlock()
	fake.ListChannelStub = stub
}

func (fake *ChannelManagement) ListChannelArgsForCall(i int) string {
	fake.listChannelMutex.RLock()
	defer fake.listChannelMutex.RUnlock()
	argsForCall := fake.listChannelArgsForCall[i]
	return argsForCall.arg1
}

func (fake *ChannelManagement) ListChannelReturns(result1 *types.ChannelInfo, result2 error) {
	fake.listChannelMutex.Lock()
	defer fake.listChannelMutex.Unlock()
	fake.ListChannelStub = nil
	fake.listChannelReturns = struct {
		result1 *types.ChannelInfo
		result2 error
	}{result1, result2}
}

func (fake *ChannelManagement) ListChannelReturnsOnCall(i int, result1 *types.ChannelInfo, result2 error) {
	fake.listChannelMutex.Lock()
	defer fake.listChannelMutex.Unlock()
	fake.ListChannelStub = nil
	if fake.listChannelReturnsOnCall == nil {
		fake.listChannelReturnsOnCall = make(map[int]struct {
			result1 *types.ChannelInfo
			result2 error
		})
	}
	fake.listChannelReturnsOnCall[i] = struct {
		result1 *types.ChannelInfo
		result2 error
	}{result1, result2}
}

func (fake *ChannelManagement) Invocations() map[string][][]interface{} {
	fake.invocationsMutex.RLock()
	defer fake.invocationsMutex.RUnlock()
	fake.listAllChannelsMutex.RLock()
	defer fake.listAllChannelsMutex.RUnlock()
	fake.listChannelMutex.RLock()
	defer fake.listChannelMutex.RUnlock()
	copiedInvocations := map[string][][]interface{}{}
	for key, value := range fake.invocations {
		copiedInvocations[key] = value
	}
	return copiedInvocations
}

func (fake *ChannelManagement) recordInvocation(key string, args []interface{}) {
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

var _ channelparticipation.ChannelManagement = new(ChannelManagement)
