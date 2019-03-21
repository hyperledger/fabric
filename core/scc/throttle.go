/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scc

import (
	"context"

	"github.com/hyperledger/fabric/common/semaphore"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	pb "github.com/hyperledger/fabric/protos/peer"
)

func Throttle(limit int, systemCC SelfDescribingSysCC) *ThrottledSysCC {
	return &ThrottledSysCC{
		SelfDescribingSysCC: systemCC,
		semaphore:           semaphore.New(limit),
	}
}

type ThrottledSysCC struct {
	SelfDescribingSysCC
	semaphore semaphore.Semaphore
}

func (t *ThrottledSysCC) Chaincode() shim.Chaincode {
	return &ThrottledChaincode{
		chaincode: t.SelfDescribingSysCC.Chaincode(),
		semaphore: t.semaphore,
	}
}

type ThrottledChaincode struct {
	chaincode shim.Chaincode
	semaphore semaphore.Semaphore
}

func (t *ThrottledChaincode) Init(stub shim.ChaincodeStubInterface) pb.Response {
	return t.chaincode.Init(stub)
}

func (t *ThrottledChaincode) Invoke(stub shim.ChaincodeStubInterface) pb.Response {
	if err := t.semaphore.Acquire(context.Background()); err != nil {
		return shim.Error(err.Error())
	}
	defer t.semaphore.Release()

	return t.chaincode.Invoke(stub)
}
