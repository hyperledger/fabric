// Code generated by counterfeiter. DO NOT EDIT.
package mock

import (
	"sync"

	"github.com/hyperledger/fabric/v3/common/chaincode"
	"github.com/hyperledger/fabric/v3/core/chaincode/lifecycle"
)

type InstalledChaincodesLister struct {
	GetInstalledChaincodeStub        func(string) (*chaincode.InstalledChaincode, error)
	getInstalledChaincodeMutex       sync.RWMutex
	getInstalledChaincodeArgsForCall []struct {
		arg1 string
	}
	getInstalledChaincodeReturns struct {
		result1 *chaincode.InstalledChaincode
		result2 error
	}
	getInstalledChaincodeReturnsOnCall map[int]struct {
		result1 *chaincode.InstalledChaincode
		result2 error
	}
	ListInstalledChaincodesStub        func() []*chaincode.InstalledChaincode
	listInstalledChaincodesMutex       sync.RWMutex
	listInstalledChaincodesArgsForCall []struct {
	}
	listInstalledChaincodesReturns struct {
		result1 []*chaincode.InstalledChaincode
	}
	listInstalledChaincodesReturnsOnCall map[int]struct {
		result1 []*chaincode.InstalledChaincode
	}
	invocations      map[string][][]interface{}
	invocationsMutex sync.RWMutex
}

func (fake *InstalledChaincodesLister) GetInstalledChaincode(arg1 string) (*chaincode.InstalledChaincode, error) {
	fake.getInstalledChaincodeMutex.Lock()
	ret, specificReturn := fake.getInstalledChaincodeReturnsOnCall[len(fake.getInstalledChaincodeArgsForCall)]
	fake.getInstalledChaincodeArgsForCall = append(fake.getInstalledChaincodeArgsForCall, struct {
		arg1 string
	}{arg1})
	fake.recordInvocation("GetInstalledChaincode", []interface{}{arg1})
	fake.getInstalledChaincodeMutex.Unlock()
	if fake.GetInstalledChaincodeStub != nil {
		return fake.GetInstalledChaincodeStub(arg1)
	}
	if specificReturn {
		return ret.result1, ret.result2
	}
	fakeReturns := fake.getInstalledChaincodeReturns
	return fakeReturns.result1, fakeReturns.result2
}

func (fake *InstalledChaincodesLister) GetInstalledChaincodeCallCount() int {
	fake.getInstalledChaincodeMutex.RLock()
	defer fake.getInstalledChaincodeMutex.RUnlock()
	return len(fake.getInstalledChaincodeArgsForCall)
}

func (fake *InstalledChaincodesLister) GetInstalledChaincodeCalls(stub func(string) (*chaincode.InstalledChaincode, error)) {
	fake.getInstalledChaincodeMutex.Lock()
	defer fake.getInstalledChaincodeMutex.Unlock()
	fake.GetInstalledChaincodeStub = stub
}

func (fake *InstalledChaincodesLister) GetInstalledChaincodeArgsForCall(i int) string {
	fake.getInstalledChaincodeMutex.RLock()
	defer fake.getInstalledChaincodeMutex.RUnlock()
	argsForCall := fake.getInstalledChaincodeArgsForCall[i]
	return argsForCall.arg1
}

func (fake *InstalledChaincodesLister) GetInstalledChaincodeReturns(result1 *chaincode.InstalledChaincode, result2 error) {
	fake.getInstalledChaincodeMutex.Lock()
	defer fake.getInstalledChaincodeMutex.Unlock()
	fake.GetInstalledChaincodeStub = nil
	fake.getInstalledChaincodeReturns = struct {
		result1 *chaincode.InstalledChaincode
		result2 error
	}{result1, result2}
}

func (fake *InstalledChaincodesLister) GetInstalledChaincodeReturnsOnCall(i int, result1 *chaincode.InstalledChaincode, result2 error) {
	fake.getInstalledChaincodeMutex.Lock()
	defer fake.getInstalledChaincodeMutex.Unlock()
	fake.GetInstalledChaincodeStub = nil
	if fake.getInstalledChaincodeReturnsOnCall == nil {
		fake.getInstalledChaincodeReturnsOnCall = make(map[int]struct {
			result1 *chaincode.InstalledChaincode
			result2 error
		})
	}
	fake.getInstalledChaincodeReturnsOnCall[i] = struct {
		result1 *chaincode.InstalledChaincode
		result2 error
	}{result1, result2}
}

func (fake *InstalledChaincodesLister) ListInstalledChaincodes() []*chaincode.InstalledChaincode {
	fake.listInstalledChaincodesMutex.Lock()
	ret, specificReturn := fake.listInstalledChaincodesReturnsOnCall[len(fake.listInstalledChaincodesArgsForCall)]
	fake.listInstalledChaincodesArgsForCall = append(fake.listInstalledChaincodesArgsForCall, struct {
	}{})
	fake.recordInvocation("ListInstalledChaincodes", []interface{}{})
	fake.listInstalledChaincodesMutex.Unlock()
	if fake.ListInstalledChaincodesStub != nil {
		return fake.ListInstalledChaincodesStub()
	}
	if specificReturn {
		return ret.result1
	}
	fakeReturns := fake.listInstalledChaincodesReturns
	return fakeReturns.result1
}

func (fake *InstalledChaincodesLister) ListInstalledChaincodesCallCount() int {
	fake.listInstalledChaincodesMutex.RLock()
	defer fake.listInstalledChaincodesMutex.RUnlock()
	return len(fake.listInstalledChaincodesArgsForCall)
}

func (fake *InstalledChaincodesLister) ListInstalledChaincodesCalls(stub func() []*chaincode.InstalledChaincode) {
	fake.listInstalledChaincodesMutex.Lock()
	defer fake.listInstalledChaincodesMutex.Unlock()
	fake.ListInstalledChaincodesStub = stub
}

func (fake *InstalledChaincodesLister) ListInstalledChaincodesReturns(result1 []*chaincode.InstalledChaincode) {
	fake.listInstalledChaincodesMutex.Lock()
	defer fake.listInstalledChaincodesMutex.Unlock()
	fake.ListInstalledChaincodesStub = nil
	fake.listInstalledChaincodesReturns = struct {
		result1 []*chaincode.InstalledChaincode
	}{result1}
}

func (fake *InstalledChaincodesLister) ListInstalledChaincodesReturnsOnCall(i int, result1 []*chaincode.InstalledChaincode) {
	fake.listInstalledChaincodesMutex.Lock()
	defer fake.listInstalledChaincodesMutex.Unlock()
	fake.ListInstalledChaincodesStub = nil
	if fake.listInstalledChaincodesReturnsOnCall == nil {
		fake.listInstalledChaincodesReturnsOnCall = make(map[int]struct {
			result1 []*chaincode.InstalledChaincode
		})
	}
	fake.listInstalledChaincodesReturnsOnCall[i] = struct {
		result1 []*chaincode.InstalledChaincode
	}{result1}
}

func (fake *InstalledChaincodesLister) Invocations() map[string][][]interface{} {
	fake.invocationsMutex.RLock()
	defer fake.invocationsMutex.RUnlock()
	fake.getInstalledChaincodeMutex.RLock()
	defer fake.getInstalledChaincodeMutex.RUnlock()
	fake.listInstalledChaincodesMutex.RLock()
	defer fake.listInstalledChaincodesMutex.RUnlock()
	copiedInvocations := map[string][][]interface{}{}
	for key, value := range fake.invocations {
		copiedInvocations[key] = value
	}
	return copiedInvocations
}

func (fake *InstalledChaincodesLister) recordInvocation(key string, args []interface{}) {
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

var _ lifecycle.InstalledChaincodesLister = new(InstalledChaincodesLister)
