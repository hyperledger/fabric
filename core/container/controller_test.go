/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package container_test

import (
	"testing"

	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/chaincode/platforms"
	"github.com/hyperledger/fabric/core/chaincode/platforms/golang"
	"github.com/hyperledger/fabric/core/container"
	"github.com/hyperledger/fabric/core/container/ccintf"
	"github.com/hyperledger/fabric/core/container/mock"
	pb "github.com/hyperledger/fabric/protos/peer"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestVM_GetChaincodePackageBytes(t *testing.T) {
	_, err := container.GetChaincodePackageBytes(nil, nil)
	assert.Error(t, err,
		"GetChaincodePackageBytes did not return error when chaincode spec is nil")
	spec := &pb.ChaincodeSpec{ChaincodeId: nil}
	_, err = container.GetChaincodePackageBytes(nil, spec)
	assert.Error(t, err, "Error expected when GetChaincodePackageBytes is called with nil chaincode ID")
	assert.Contains(t, err.Error(), "invalid chaincode spec")
	spec = &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_GOLANG,
		ChaincodeId: nil,
		Input:       &pb.ChaincodeInput{Args: util.ToChaincodeArgs("f")}}
	_, err = container.GetChaincodePackageBytes(platforms.NewRegistry(&golang.Platform{}), spec)
	assert.Error(t, err,
		"GetChaincodePackageBytes did not return error when chaincode ID is nil")
}

func TestWaitContainerReq(t *testing.T) {
	gt := NewGomegaWithT(t)

	exited := &mock.ExitedFunc{}
	done := make(chan struct{})
	exited.Stub = func(int, error) { close(done) }

	req := container.WaitContainerReq{
		CCID:   ccintf.CCID{Name: "the-name", Version: "the-version"},
		Exited: exited.Spy,
	}
	gt.Expect(req.GetCCID()).To(Equal(ccintf.CCID{Name: "the-name", Version: "the-version"}))

	fakeVM := &mock.VM{}
	fakeVM.WaitReturns(99, errors.New("boing-boing"))

	err := req.Do(fakeVM)
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Eventually(done).Should(BeClosed())

	gt.Expect(fakeVM.WaitCallCount()).To(Equal(1))
	ccid := fakeVM.WaitArgsForCall(0)
	gt.Expect(ccid).To(Equal(ccintf.CCID{Name: "the-name", Version: "the-version"}))

	ec, exitErr := exited.ArgsForCall(0)
	gt.Expect(ec).To(Equal(99))
	gt.Expect(exitErr).To(MatchError("boing-boing"))
}
