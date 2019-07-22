/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode_test

import (
	"testing"

	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/hyperledger/fabric/core/chaincode/mock"
	persistence "github.com/hyperledger/fabric/core/chaincode/persistence/intf"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/container"
	"github.com/hyperledger/fabric/core/container/ccintf"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestContainerRuntimeStart(t *testing.T) {
	fakeVM := &mock.ContainerVM{}

	cr := &chaincode.ContainerRuntime{
		LockingVM: &container.LockingVM{
			Underlying:     fakeVM,
			ContainerLocks: container.NewContainerLocks(),
		},
	}

	ccci := &ccprovider.ChaincodeContainerInfo{
		Type:      "GOLANG",
		Path:      "chaincode-path",
		Name:      "chaincode-name",
		Version:   "chaincode-version",
		PackageID: "chaincode-name:chaincode-version",
	}

	err := cr.Start(ccci, []byte("code-package"))
	assert.NoError(t, err)

	assert.Equal(t, 1, fakeVM.BuildCallCount())
	ccci, codePackage := fakeVM.BuildArgsForCall(0)
	assert.Equal(t, ccci, &ccprovider.ChaincodeContainerInfo{
		PackageID: "chaincode-name:chaincode-version",
		Type:      "GOLANG",
		Path:      "chaincode-path",
		Name:      "chaincode-name",
		Version:   "chaincode-version",
	})
	assert.Equal(t, []byte("code-package"), codePackage)

	assert.Equal(t, 1, fakeVM.StartCallCount())
	ccid, ccType, tlsConfig := fakeVM.StartArgsForCall(0)
	assert.Equal(t, ccintf.CCID("chaincode-name:chaincode-version"), ccid)
	assert.Equal(t, "GOLANG", ccType)
	assert.Nil(t, tlsConfig)
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
		fakeVM := &mock.ContainerVM{}
		fakeVM.BuildReturns(tc.buildErr)
		fakeVM.StartReturns(tc.startErr)

		cr := &chaincode.ContainerRuntime{
			LockingVM: &container.LockingVM{
				Underlying:     fakeVM,
				ContainerLocks: container.NewContainerLocks(),
			},
		}

		ccci := &ccprovider.ChaincodeContainerInfo{
			Type:    tc.chaincodeType,
			Name:    "chaincode-id-name",
			Version: "chaincode-version",
		}

		err := cr.Start(ccci, nil)
		assert.EqualError(t, err, tc.errValue)
	}
}

func TestContainerRuntimeStop(t *testing.T) {
	fakeVM := &mock.ContainerVM{}

	cr := &chaincode.ContainerRuntime{
		LockingVM: &container.LockingVM{
			Underlying:     fakeVM,
			ContainerLocks: container.NewContainerLocks(),
		},
	}

	ccci := &ccprovider.ChaincodeContainerInfo{
		Type:      pb.ChaincodeSpec_GOLANG.String(),
		PackageID: "chaincode-id-name:chaincode-version",
	}

	err := cr.Stop(ccci)
	assert.NoError(t, err)

	assert.Equal(t, 1, fakeVM.StopCallCount())
	ccid := fakeVM.StopArgsForCall(0)
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
		fakeVM := &mock.ContainerVM{}
		fakeVM.StopReturns(tc.processErr)

		cr := &chaincode.ContainerRuntime{
			LockingVM: &container.LockingVM{
				Underlying:     fakeVM,
				ContainerLocks: container.NewContainerLocks(),
			},
		}

		ccci := &ccprovider.ChaincodeContainerInfo{
			Type:    pb.ChaincodeSpec_GOLANG.String(),
			Name:    "chaincode-id-name",
			Version: "chaincode-version",
		}

		assert.EqualError(t, cr.Stop(ccci), tc.errValue)
	}
}

func TestContainerRuntimeWait(t *testing.T) {
	fakeVM := &mock.ContainerVM{}

	cr := &chaincode.ContainerRuntime{
		LockingVM: &container.LockingVM{
			Underlying:     fakeVM,
			ContainerLocks: container.NewContainerLocks(),
		},
	}

	ccci := &ccprovider.ChaincodeContainerInfo{
		Type:      pb.ChaincodeSpec_GOLANG.String(),
		Name:      "chaincode-id-name",
		Version:   "chaincode-version",
		PackageID: persistence.PackageID("chaincode-id-name:chaincode-version"),
	}

	exitCode, err := cr.Wait(ccci)
	assert.NoError(t, err)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, 1, fakeVM.WaitCallCount())
	assert.Equal(t, ccintf.CCID("chaincode-id-name:chaincode-version"), fakeVM.WaitArgsForCall(0))

	fakeVM.WaitReturns(3, errors.New("moles-and-trolls"))
	code, err := cr.Wait(ccci)
	assert.EqualError(t, err, "moles-and-trolls")
	assert.Equal(t, code, 3)
}
