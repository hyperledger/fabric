/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scc_test

import (
	"testing"

	"github.com/hyperledger/fabric/core/container/ccintf"
	"github.com/hyperledger/fabric/core/scc"
	"github.com/hyperledger/fabric/core/scc/mock"
	"github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
)

//go:generate counterfeiter -o mock/chaincode_stream_handler.go --fake-name ChaincodeStreamHandler . chaincodeStreamHandler
type chaincodeStreamHandler interface {
	scc.ChaincodeStreamHandler
}

func TestDeploy(t *testing.T) {
	gt := gomega.NewGomegaWithT(t)

	csh := &mock.ChaincodeStreamHandler{}
	doneC := make(chan struct{})
	close(doneC)
	csh.LaunchInProcReturns(doneC)
	scc.DeploySysCC(&scc.SysCCWrapper{SCC: &scc.SystemChaincode{Name: "test"}}, "latest", csh)
	gt.Expect(csh.LaunchInProcCallCount()).To(gomega.Equal(1))
	gt.Expect(csh.LaunchInProcArgsForCall(0)).To(gomega.Equal(ccintf.CCID("test:latest")))
	gt.Eventually(csh.HandleChaincodeStreamCallCount).Should(gomega.Equal(1))
}

func TestCreatePluginSysCCs(t *testing.T) {
	assert.NotPanics(t, func() { scc.CreatePluginSysCCs() }, "expected successful init")
}
