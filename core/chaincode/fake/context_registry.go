// Code generated by counterfeiter. DO NOT EDIT.
package fake

import (
	"sync"

	"github.com/hyperledger/fabric/v3/core/chaincode"
	"github.com/hyperledger/fabric/v3/core/common/ccprovider"
)

type ContextRegistry struct {
	CloseStub        func()
	closeMutex       sync.RWMutex
	closeArgsForCall []struct {
	}
	CreateStub        func(*ccprovider.TransactionParams) (*chaincode.TransactionContext, error)
	createMutex       sync.RWMutex
	createArgsForCall []struct {
		arg1 *ccprovider.TransactionParams
	}
	createReturns struct {
		result1 *chaincode.TransactionContext
		result2 error
	}
	createReturnsOnCall map[int]struct {
		result1 *chaincode.TransactionContext
		result2 error
	}
	DeleteStub        func(string, string)
	deleteMutex       sync.RWMutex
	deleteArgsForCall []struct {
		arg1 string
		arg2 string
	}
	GetStub        func(string, string) *chaincode.TransactionContext
	getMutex       sync.RWMutex
	getArgsForCall []struct {
		arg1 string
		arg2 string
	}
	getReturns struct {
		result1 *chaincode.TransactionContext
	}
	getReturnsOnCall map[int]struct {
		result1 *chaincode.TransactionContext
	}
	invocations      map[string][][]interface{}
	invocationsMutex sync.RWMutex
}

func (fake *ContextRegistry) Close() {
	fake.closeMutex.Lock()
	fake.closeArgsForCall = append(fake.closeArgsForCall, struct {
	}{})
	fake.recordInvocation("Close", []interface{}{})
	fake.closeMutex.Unlock()
	if fake.CloseStub != nil {
		fake.CloseStub()
	}
}

func (fake *ContextRegistry) CloseCallCount() int {
	fake.closeMutex.RLock()
	defer fake.closeMutex.RUnlock()
	return len(fake.closeArgsForCall)
}

func (fake *ContextRegistry) CloseCalls(stub func()) {
	fake.closeMutex.Lock()
	defer fake.closeMutex.Unlock()
	fake.CloseStub = stub
}

func (fake *ContextRegistry) Create(arg1 *ccprovider.TransactionParams) (*chaincode.TransactionContext, error) {
	fake.createMutex.Lock()
	ret, specificReturn := fake.createReturnsOnCall[len(fake.createArgsForCall)]
	fake.createArgsForCall = append(fake.createArgsForCall, struct {
		arg1 *ccprovider.TransactionParams
	}{arg1})
	fake.recordInvocation("Create", []interface{}{arg1})
	fake.createMutex.Unlock()
	if fake.CreateStub != nil {
		return fake.CreateStub(arg1)
	}
	if specificReturn {
		return ret.result1, ret.result2
	}
	fakeReturns := fake.createReturns
	return fakeReturns.result1, fakeReturns.result2
}

func (fake *ContextRegistry) CreateCallCount() int {
	fake.createMutex.RLock()
	defer fake.createMutex.RUnlock()
	return len(fake.createArgsForCall)
}

func (fake *ContextRegistry) CreateCalls(stub func(*ccprovider.TransactionParams) (*chaincode.TransactionContext, error)) {
	fake.createMutex.Lock()
	defer fake.createMutex.Unlock()
	fake.CreateStub = stub
}

func (fake *ContextRegistry) CreateArgsForCall(i int) *ccprovider.TransactionParams {
	fake.createMutex.RLock()
	defer fake.createMutex.RUnlock()
	argsForCall := fake.createArgsForCall[i]
	return argsForCall.arg1
}

func (fake *ContextRegistry) CreateReturns(result1 *chaincode.TransactionContext, result2 error) {
	fake.createMutex.Lock()
	defer fake.createMutex.Unlock()
	fake.CreateStub = nil
	fake.createReturns = struct {
		result1 *chaincode.TransactionContext
		result2 error
	}{result1, result2}
}

func (fake *ContextRegistry) CreateReturnsOnCall(i int, result1 *chaincode.TransactionContext, result2 error) {
	fake.createMutex.Lock()
	defer fake.createMutex.Unlock()
	fake.CreateStub = nil
	if fake.createReturnsOnCall == nil {
		fake.createReturnsOnCall = make(map[int]struct {
			result1 *chaincode.TransactionContext
			result2 error
		})
	}
	fake.createReturnsOnCall[i] = struct {
		result1 *chaincode.TransactionContext
		result2 error
	}{result1, result2}
}

func (fake *ContextRegistry) Delete(arg1 string, arg2 string) {
	fake.deleteMutex.Lock()
	fake.deleteArgsForCall = append(fake.deleteArgsForCall, struct {
		arg1 string
		arg2 string
	}{arg1, arg2})
	fake.recordInvocation("Delete", []interface{}{arg1, arg2})
	fake.deleteMutex.Unlock()
	if fake.DeleteStub != nil {
		fake.DeleteStub(arg1, arg2)
	}
}

func (fake *ContextRegistry) DeleteCallCount() int {
	fake.deleteMutex.RLock()
	defer fake.deleteMutex.RUnlock()
	return len(fake.deleteArgsForCall)
}

func (fake *ContextRegistry) DeleteCalls(stub func(string, string)) {
	fake.deleteMutex.Lock()
	defer fake.deleteMutex.Unlock()
	fake.DeleteStub = stub
}

func (fake *ContextRegistry) DeleteArgsForCall(i int) (string, string) {
	fake.deleteMutex.RLock()
	defer fake.deleteMutex.RUnlock()
	argsForCall := fake.deleteArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2
}

func (fake *ContextRegistry) Get(arg1 string, arg2 string) *chaincode.TransactionContext {
	fake.getMutex.Lock()
	ret, specificReturn := fake.getReturnsOnCall[len(fake.getArgsForCall)]
	fake.getArgsForCall = append(fake.getArgsForCall, struct {
		arg1 string
		arg2 string
	}{arg1, arg2})
	fake.recordInvocation("Get", []interface{}{arg1, arg2})
	fake.getMutex.Unlock()
	if fake.GetStub != nil {
		return fake.GetStub(arg1, arg2)
	}
	if specificReturn {
		return ret.result1
	}
	fakeReturns := fake.getReturns
	return fakeReturns.result1
}

func (fake *ContextRegistry) GetCallCount() int {
	fake.getMutex.RLock()
	defer fake.getMutex.RUnlock()
	return len(fake.getArgsForCall)
}

func (fake *ContextRegistry) GetCalls(stub func(string, string) *chaincode.TransactionContext) {
	fake.getMutex.Lock()
	defer fake.getMutex.Unlock()
	fake.GetStub = stub
}

func (fake *ContextRegistry) GetArgsForCall(i int) (string, string) {
	fake.getMutex.RLock()
	defer fake.getMutex.RUnlock()
	argsForCall := fake.getArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2
}

func (fake *ContextRegistry) GetReturns(result1 *chaincode.TransactionContext) {
	fake.getMutex.Lock()
	defer fake.getMutex.Unlock()
	fake.GetStub = nil
	fake.getReturns = struct {
		result1 *chaincode.TransactionContext
	}{result1}
}

func (fake *ContextRegistry) GetReturnsOnCall(i int, result1 *chaincode.TransactionContext) {
	fake.getMutex.Lock()
	defer fake.getMutex.Unlock()
	fake.GetStub = nil
	if fake.getReturnsOnCall == nil {
		fake.getReturnsOnCall = make(map[int]struct {
			result1 *chaincode.TransactionContext
		})
	}
	fake.getReturnsOnCall[i] = struct {
		result1 *chaincode.TransactionContext
	}{result1}
}

func (fake *ContextRegistry) Invocations() map[string][][]interface{} {
	fake.invocationsMutex.RLock()
	defer fake.invocationsMutex.RUnlock()
	fake.closeMutex.RLock()
	defer fake.closeMutex.RUnlock()
	fake.createMutex.RLock()
	defer fake.createMutex.RUnlock()
	fake.deleteMutex.RLock()
	defer fake.deleteMutex.RUnlock()
	fake.getMutex.RLock()
	defer fake.getMutex.RUnlock()
	copiedInvocations := map[string][][]interface{}{}
	for key, value := range fake.invocations {
		copiedInvocations[key] = value
	}
	return copiedInvocations
}

func (fake *ContextRegistry) recordInvocation(key string, args []interface{}) {
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
