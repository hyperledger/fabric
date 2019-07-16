/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package container_test

import (
	"testing"

	"github.com/hyperledger/fabric/core/container"
	"github.com/hyperledger/fabric/core/container/ccintf"
	"github.com/hyperledger/fabric/core/container/mock"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
)

func TestWaitContainerReq(t *testing.T) {
	gt := NewGomegaWithT(t)

	exited := &mock.ExitedFunc{}
	done := make(chan struct{})
	exited.Stub = func(int, error) { close(done) }

	req := container.WaitContainerReq{
		CCID:   ccintf.CCID("the-name:the-version"),
		Exited: exited.Spy,
	}
	gt.Expect(req.GetCCID()).To(Equal(ccintf.CCID("the-name:the-version")))

	fakeVM := &mock.VM{}
	fakeVM.WaitReturns(99, errors.New("boing-boing"))

	err := req.Do(fakeVM)
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Eventually(done).Should(BeClosed())

	gt.Expect(fakeVM.WaitCallCount()).To(Equal(1))
	ccid := fakeVM.WaitArgsForCall(0)
	gt.Expect(ccid).To(Equal(ccintf.CCID("the-name:the-version")))

	ec, exitErr := exited.ArgsForCall(0)
	gt.Expect(ec).To(Equal(99))
	gt.Expect(exitErr).To(MatchError("boing-boing"))
}
