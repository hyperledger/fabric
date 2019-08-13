/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode_test

import (
	"testing"

	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/hyperledger/fabric/core/chaincode/mock"
	"github.com/hyperledger/fabric/core/container/ccintf"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestContainerRuntimeStart(t *testing.T) {
	fakeRouter := &mock.ContainerRouter{}

	cr := &chaincode.ContainerRuntime{
		ContainerRouter: fakeRouter,
		PeerAddress:     "peer-address",
	}

	err := cr.Start("chaincode-name:chaincode-version")
	assert.NoError(t, err)

	assert.Equal(t, 1, fakeRouter.BuildCallCount())
	packageID := fakeRouter.BuildArgsForCall(0)
	assert.Equal(t, ccintf.CCID("chaincode-name:chaincode-version"), packageID)

	assert.Equal(t, 1, fakeRouter.StartCallCount())
	ccid, peerConnection := fakeRouter.StartArgsForCall(0)
	assert.Equal(t, ccintf.CCID("chaincode-name:chaincode-version"), ccid)
	assert.Equal(t, "peer-address", peerConnection.Address)
	assert.Nil(t, peerConnection.TLSConfig)
}

func TestContainerRuntimeStartErrors(t *testing.T) {
	tests := []struct {
		chaincodeType string
		buildErr      error
		startErr      error
		errValue      string
	}{
		{pb.ChaincodeSpec_GOLANG.String(), nil, errors.New("process-failed"), "error starting container: process-failed"},
		{pb.ChaincodeSpec_GOLANG.String(), errors.New("build-failed"), nil, "error building image: build-failed"},
		{pb.ChaincodeSpec_GOLANG.String(), errors.New("build-failed"), nil, "error building image: build-failed"},
	}

	for _, tc := range tests {
		fakeRouter := &mock.ContainerRouter{}
		fakeRouter.BuildReturns(tc.buildErr)
		fakeRouter.StartReturns(tc.startErr)

		cr := &chaincode.ContainerRuntime{
			ContainerRouter: fakeRouter,
		}

		err := cr.Start("ccid")
		assert.EqualError(t, err, tc.errValue)
	}
}

func TestContainerRuntimeStop(t *testing.T) {
	fakeRouter := &mock.ContainerRouter{}

	cr := &chaincode.ContainerRuntime{
		ContainerRouter: fakeRouter,
	}

	err := cr.Stop("chaincode-id-name:chaincode-version")
	assert.NoError(t, err)

	assert.Equal(t, 1, fakeRouter.StopCallCount())
	ccid := fakeRouter.StopArgsForCall(0)
	assert.Equal(t, ccintf.CCID("chaincode-id-name:chaincode-version"), ccid)
}

func TestContainerRuntimeStopErrors(t *testing.T) {
	tests := []struct {
		processErr error
		errValue   string
	}{
		{errors.New("process-failed"), "error stopping container: process-failed"},
	}

	for _, tc := range tests {
		fakeRouter := &mock.ContainerRouter{}
		fakeRouter.StopReturns(tc.processErr)

		cr := &chaincode.ContainerRuntime{
			ContainerRouter: fakeRouter,
		}

		assert.EqualError(t, cr.Stop("ccid"), tc.errValue)
	}
}

func TestContainerRuntimeWait(t *testing.T) {
	fakeRouter := &mock.ContainerRouter{}

	cr := &chaincode.ContainerRuntime{
		ContainerRouter: fakeRouter,
	}

	exitCode, err := cr.Wait("chaincode-id-name:chaincode-version")
	assert.NoError(t, err)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, 1, fakeRouter.WaitCallCount())
	assert.Equal(t, ccintf.CCID("chaincode-id-name:chaincode-version"), fakeRouter.WaitArgsForCall(0))

	fakeRouter.WaitReturns(3, errors.New("moles-and-trolls"))
	code, err := cr.Wait("chaincode-id-name:chaincode-version")
	assert.EqualError(t, err, "moles-and-trolls")
	assert.Equal(t, code, 3)
}
