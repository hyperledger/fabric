// Code generated by counterfeiter. DO NOT EDIT.
package mocks

import (
	"sync"
	"time"

	"github.com/hyperledger/fabric-lib-go/bccsp"
	"github.com/hyperledger/fabric-lib-go/common/flogging"
	"github.com/hyperledger/fabric/internal/pkg/identity"
	"github.com/hyperledger/fabric/internal/pkg/peer/blocksprovider"
	"github.com/hyperledger/fabric/orderer/consensus/smartbft"
)

type BFTDelivererFactory struct {
	CreateBFTDelivererStub        func(string, blocksprovider.BlockHandler, blocksprovider.LedgerInfo, blocksprovider.UpdatableBlockVerifier, blocksprovider.Dialer, blocksprovider.OrdererConnectionSourceFactory, bccsp.BCCSP, chan struct{}, identity.SignerSerializer, blocksprovider.DeliverStreamer, blocksprovider.CensorshipDetectorFactory, *flogging.FabricLogger, time.Duration, time.Duration, time.Duration, time.Duration, blocksprovider.MaxRetryDurationExceededHandler) smartbft.BFTBlockDeliverer
	createBFTDelivererMutex       sync.RWMutex
	createBFTDelivererArgsForCall []struct {
		arg1  string
		arg2  blocksprovider.BlockHandler
		arg3  blocksprovider.LedgerInfo
		arg4  blocksprovider.UpdatableBlockVerifier
		arg5  blocksprovider.Dialer
		arg6  blocksprovider.OrdererConnectionSourceFactory
		arg7  bccsp.BCCSP
		arg8  chan struct{}
		arg9  identity.SignerSerializer
		arg10 blocksprovider.DeliverStreamer
		arg11 blocksprovider.CensorshipDetectorFactory
		arg12 *flogging.FabricLogger
		arg13 time.Duration
		arg14 time.Duration
		arg15 time.Duration
		arg16 time.Duration
		arg17 blocksprovider.MaxRetryDurationExceededHandler
	}
	createBFTDelivererReturns struct {
		result1 smartbft.BFTBlockDeliverer
	}
	createBFTDelivererReturnsOnCall map[int]struct {
		result1 smartbft.BFTBlockDeliverer
	}
	invocations      map[string][][]interface{}
	invocationsMutex sync.RWMutex
}

func (fake *BFTDelivererFactory) CreateBFTDeliverer(arg1 string, arg2 blocksprovider.BlockHandler, arg3 blocksprovider.LedgerInfo, arg4 blocksprovider.UpdatableBlockVerifier, arg5 blocksprovider.Dialer, arg6 blocksprovider.OrdererConnectionSourceFactory, arg7 bccsp.BCCSP, arg8 chan struct{}, arg9 identity.SignerSerializer, arg10 blocksprovider.DeliverStreamer, arg11 blocksprovider.CensorshipDetectorFactory, arg12 *flogging.FabricLogger, arg13 time.Duration, arg14 time.Duration, arg15 time.Duration, arg16 time.Duration, arg17 blocksprovider.MaxRetryDurationExceededHandler) smartbft.BFTBlockDeliverer {
	fake.createBFTDelivererMutex.Lock()
	ret, specificReturn := fake.createBFTDelivererReturnsOnCall[len(fake.createBFTDelivererArgsForCall)]
	fake.createBFTDelivererArgsForCall = append(fake.createBFTDelivererArgsForCall, struct {
		arg1  string
		arg2  blocksprovider.BlockHandler
		arg3  blocksprovider.LedgerInfo
		arg4  blocksprovider.UpdatableBlockVerifier
		arg5  blocksprovider.Dialer
		arg6  blocksprovider.OrdererConnectionSourceFactory
		arg7  bccsp.BCCSP
		arg8  chan struct{}
		arg9  identity.SignerSerializer
		arg10 blocksprovider.DeliverStreamer
		arg11 blocksprovider.CensorshipDetectorFactory
		arg12 *flogging.FabricLogger
		arg13 time.Duration
		arg14 time.Duration
		arg15 time.Duration
		arg16 time.Duration
		arg17 blocksprovider.MaxRetryDurationExceededHandler
	}{arg1, arg2, arg3, arg4, arg5, arg6, arg7, arg8, arg9, arg10, arg11, arg12, arg13, arg14, arg15, arg16, arg17})
	stub := fake.CreateBFTDelivererStub
	fakeReturns := fake.createBFTDelivererReturns
	fake.recordInvocation("CreateBFTDeliverer", []interface{}{arg1, arg2, arg3, arg4, arg5, arg6, arg7, arg8, arg9, arg10, arg11, arg12, arg13, arg14, arg15, arg16, arg17})
	fake.createBFTDelivererMutex.Unlock()
	if stub != nil {
		return stub(arg1, arg2, arg3, arg4, arg5, arg6, arg7, arg8, arg9, arg10, arg11, arg12, arg13, arg14, arg15, arg16, arg17)
	}
	if specificReturn {
		return ret.result1
	}
	return fakeReturns.result1
}

func (fake *BFTDelivererFactory) CreateBFTDelivererCallCount() int {
	fake.createBFTDelivererMutex.RLock()
	defer fake.createBFTDelivererMutex.RUnlock()
	return len(fake.createBFTDelivererArgsForCall)
}

func (fake *BFTDelivererFactory) CreateBFTDelivererCalls(stub func(string, blocksprovider.BlockHandler, blocksprovider.LedgerInfo, blocksprovider.UpdatableBlockVerifier, blocksprovider.Dialer, blocksprovider.OrdererConnectionSourceFactory, bccsp.BCCSP, chan struct{}, identity.SignerSerializer, blocksprovider.DeliverStreamer, blocksprovider.CensorshipDetectorFactory, *flogging.FabricLogger, time.Duration, time.Duration, time.Duration, time.Duration, blocksprovider.MaxRetryDurationExceededHandler) smartbft.BFTBlockDeliverer) {
	fake.createBFTDelivererMutex.Lock()
	defer fake.createBFTDelivererMutex.Unlock()
	fake.CreateBFTDelivererStub = stub
}

func (fake *BFTDelivererFactory) CreateBFTDelivererArgsForCall(i int) (string, blocksprovider.BlockHandler, blocksprovider.LedgerInfo, blocksprovider.UpdatableBlockVerifier, blocksprovider.Dialer, blocksprovider.OrdererConnectionSourceFactory, bccsp.BCCSP, chan struct{}, identity.SignerSerializer, blocksprovider.DeliverStreamer, blocksprovider.CensorshipDetectorFactory, *flogging.FabricLogger, time.Duration, time.Duration, time.Duration, time.Duration, blocksprovider.MaxRetryDurationExceededHandler) {
	fake.createBFTDelivererMutex.RLock()
	defer fake.createBFTDelivererMutex.RUnlock()
	argsForCall := fake.createBFTDelivererArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2, argsForCall.arg3, argsForCall.arg4, argsForCall.arg5, argsForCall.arg6, argsForCall.arg7, argsForCall.arg8, argsForCall.arg9, argsForCall.arg10, argsForCall.arg11, argsForCall.arg12, argsForCall.arg13, argsForCall.arg14, argsForCall.arg15, argsForCall.arg16, argsForCall.arg17
}

func (fake *BFTDelivererFactory) CreateBFTDelivererReturns(result1 smartbft.BFTBlockDeliverer) {
	fake.createBFTDelivererMutex.Lock()
	defer fake.createBFTDelivererMutex.Unlock()
	fake.CreateBFTDelivererStub = nil
	fake.createBFTDelivererReturns = struct {
		result1 smartbft.BFTBlockDeliverer
	}{result1}
}

func (fake *BFTDelivererFactory) CreateBFTDelivererReturnsOnCall(i int, result1 smartbft.BFTBlockDeliverer) {
	fake.createBFTDelivererMutex.Lock()
	defer fake.createBFTDelivererMutex.Unlock()
	fake.CreateBFTDelivererStub = nil
	if fake.createBFTDelivererReturnsOnCall == nil {
		fake.createBFTDelivererReturnsOnCall = make(map[int]struct {
			result1 smartbft.BFTBlockDeliverer
		})
	}
	fake.createBFTDelivererReturnsOnCall[i] = struct {
		result1 smartbft.BFTBlockDeliverer
	}{result1}
}

func (fake *BFTDelivererFactory) Invocations() map[string][][]interface{} {
	fake.invocationsMutex.RLock()
	defer fake.invocationsMutex.RUnlock()
	fake.createBFTDelivererMutex.RLock()
	defer fake.createBFTDelivererMutex.RUnlock()
	copiedInvocations := map[string][][]interface{}{}
	for key, value := range fake.invocations {
		copiedInvocations[key] = value
	}
	return copiedInvocations
}

func (fake *BFTDelivererFactory) recordInvocation(key string, args []interface{}) {
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

var _ smartbft.BFTDelivererFactory = new(BFTDelivererFactory)
