/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scc_test

import (
	"testing"

	"github.com/hyperledger/fabric/core/chaincode/lifecycle"
	"github.com/hyperledger/fabric/core/scc"
	"github.com/hyperledger/fabric/core/scc/mock"
	. "github.com/onsi/gomega"
)

//go:generate counterfeiter -o mock/chaincode_stream_handler.go --fake-name ChaincodeStreamHandler . chaincodeStreamHandler
type chaincodeStreamHandler interface {
	scc.ChaincodeStreamHandler
}

func TestDeploy(t *testing.T) {
	gt := NewGomegaWithT(t)

	doneC := make(chan struct{})
	csh := &mock.ChaincodeStreamHandler{}
	csh.LaunchInProcReturns(doneC)

	deployC := make(chan struct{})
	go func() { scc.DeploySysCC(&lifecycle.SCC{}, csh); close(deployC) }()

	// Consume the register message to enable cleanup
	gt.Eventually(csh.HandleChaincodeStreamCallCount).Should(Equal(1))
	stream := csh.HandleChaincodeStreamArgsForCall(0)
	stream.Recv()

	close(doneC)
	gt.Eventually(deployC).Should(BeClosed())

	gt.Expect(csh.LaunchInProcCallCount()).To(Equal(1))
	gt.Expect(csh.LaunchInProcArgsForCall(0)).To(Equal("_lifecycle.syscc"))
	gt.Eventually(csh.HandleChaincodeStreamCallCount).Should(Equal(1))
}
