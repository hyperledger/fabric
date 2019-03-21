/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scc

import (
	"testing"

	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/scc/mock"
	pb "github.com/hyperledger/fabric/protos/peer"
	. "github.com/onsi/gomega"
)

//go:generate counterfeiter -o mock/chaincode.go --fake-name Chaincode . chaincode
//go:generate counterfeiter -o mock/selfdescribingsyscc.go --fake-name SelfDescribingSysCC . selfDescribingSysCC

type chaincode interface{ shim.Chaincode }
type selfDescribingSysCC interface{ SelfDescribingSysCC } // prevent package cycle

func TestThrottle(t *testing.T) {
	gt := NewGomegaWithT(t)

	runningCh := make(chan struct{}, 1)
	doneCh := make(chan struct{})
	chaincode := &mock.Chaincode{}
	chaincode.InvokeStub = func(shim.ChaincodeStubInterface) pb.Response {
		gt.Eventually(runningCh).Should(BeSent(struct{}{}))
		<-doneCh
		return pb.Response{}
	}

	sysCC := &mock.SelfDescribingSysCC{}
	sysCC.ChaincodeReturns(chaincode)

	// run invokes concurrently
	throttled := Throttle(5, sysCC).Chaincode()
	for i := 0; i < 5; i++ {
		go throttled.Invoke(nil)
		gt.Eventually(runningCh).Should(Receive())
	}
	// invoke and ensure requet is delayed
	go throttled.Invoke(nil)
	gt.Consistently(runningCh).ShouldNot(Receive())

	// release one of the pending invokes and see that
	// the delayed request is started
	gt.Eventually(doneCh).Should(BeSent(struct{}{}))
	gt.Eventually(runningCh).Should(Receive())

	// cleanup
	close(doneCh)
}

func TestThrottledChaincode(t *testing.T) {
	gt := NewGomegaWithT(t)

	chaincode := &mock.Chaincode{}
	chaincode.InitReturns(pb.Response{Message: "init-returns"})
	chaincode.InvokeReturns(pb.Response{Message: "invoke-returns"})

	sysCC := &mock.SelfDescribingSysCC{}
	sysCC.ChaincodeReturns(chaincode)
	throttled := Throttle(5, sysCC).Chaincode()

	initResp := throttled.Init(nil)
	gt.Expect(initResp.Message).To(Equal("init-returns"))
	invokeResp := throttled.Invoke(nil)
	gt.Expect(invokeResp.Message).To(Equal("invoke-returns"))
}
