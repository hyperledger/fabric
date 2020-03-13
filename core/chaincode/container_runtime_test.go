/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode_test

import (
	"testing"

	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/hyperledger/fabric/core/chaincode/mock"
	"github.com/hyperledger/fabric/core/container"
	"github.com/hyperledger/fabric/core/container/ccintf"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestContainerRuntimeBuild(t *testing.T) {
	fakeRouter := &mock.ContainerRouter{}
	fakeRouter.ChaincodeServerInfoReturns(&ccintf.ChaincodeServerInfo{Address: "ccaddress:12345"}, nil)

	cr := &chaincode.ContainerRuntime{
		ContainerRouter: fakeRouter,
		BuildRegistry:   &container.BuildRegistry{},
	}

	ccinfo, err := cr.Build("chaincode-name:chaincode-version")
	assert.NoError(t, err)
	assert.Equal(t, &ccintf.ChaincodeServerInfo{Address: "ccaddress:12345"}, ccinfo)

	assert.Equal(t, 1, fakeRouter.BuildCallCount())
	packageID := fakeRouter.BuildArgsForCall(0)
	assert.Equal(t, "chaincode-name:chaincode-version", packageID)
}

func TestContainerRuntimeStart(t *testing.T) {
	fakeRouter := &mock.ContainerRouter{}

	cr := &chaincode.ContainerRuntime{
		ContainerRouter: fakeRouter,
		BuildRegistry:   &container.BuildRegistry{},
	}

	err := cr.Start("chaincode-name:chaincode-version", &ccintf.PeerConnection{Address: "peer-address"})
	assert.NoError(t, err)

	assert.Equal(t, 1, fakeRouter.StartCallCount())
	ccid, peerConnection := fakeRouter.StartArgsForCall(0)
	assert.Equal(t, "chaincode-name:chaincode-version", ccid)
	assert.Equal(t, "peer-address", peerConnection.Address)
	assert.Nil(t, peerConnection.TLSConfig)

	// Try starting a second time, to ensure build is not invoked again
	// as the BuildRegistry already holds it
	err = cr.Start("chaincode-name:chaincode-version", &ccintf.PeerConnection{Address: "fake-address"})
	assert.NoError(t, err)
	assert.Equal(t, 2, fakeRouter.StartCallCount())
}

func TestContainerRuntimeStartErrors(t *testing.T) {
	tests := []struct {
		chaincodeType string
		startErr      error
		errValue      string
	}{
		{pb.ChaincodeSpec_GOLANG.String(), errors.New("process-failed"), "error starting container: process-failed"},
	}

	for _, tc := range tests {
		fakeRouter := &mock.ContainerRouter{}
		fakeRouter.StartReturns(tc.startErr)

		cr := &chaincode.ContainerRuntime{
			ContainerRouter: fakeRouter,
			BuildRegistry:   &container.BuildRegistry{},
		}

		err := cr.Start("ccid", &ccintf.PeerConnection{Address: "fake-address"})
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
	assert.Equal(t, "chaincode-id-name:chaincode-version", ccid)
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
	assert.Equal(t, "chaincode-id-name:chaincode-version", fakeRouter.WaitArgsForCall(0))

	fakeRouter.WaitReturns(3, errors.New("moles-and-trolls"))
	code, err := cr.Wait("chaincode-id-name:chaincode-version")
	assert.EqualError(t, err, "moles-and-trolls")
	assert.Equal(t, code, 3)
}
