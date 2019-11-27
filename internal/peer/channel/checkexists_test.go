/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channel

import (
	"errors"
	"testing"

	cb "github.com/hyperledger/fabric-protos-go/common"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/internal/peer/common"
	"github.com/hyperledger/fabric/internal/peer/common/mock"
	"github.com/stretchr/testify/assert"
)

func TestChannelExistsMissingChannelID(t *testing.T) {

	// Save current os.Exit function and restore at the end:
	oldOsExit := osExit
	defer func() { osExit = oldOsExit }()

	exitCode = -1
	mockExit := func(code int) {
		t.Logf("os.Exit is called with exit code %d", code)
		exitCode = code
	}
	osExit = mockExit

	InitMSP()

	signer, err := common.GetDefaultSigner()
	assert.NoError(t, err)

	mockCF := &ChannelCmdFactory{
		BroadcastFactory: mockBroadcastClientFactory,
		DeliverClient:    getMockDeliverClient(mockChannel),
		Signer:           signer,
	}

	cmd := checkexistsCmd(mockCF)
	AddFlags(cmd)

	err = cmd.Execute()
	t.Logf("Error: %s", err)

	assert.Error(t, err, "Command should return error")
	assert.Contains(t, err.Error(), "must supply channel ID")
}

func TestChannelExists(t *testing.T) {
	defer resetFlags()

	// Save current os.Exit function and restore at the end:
	oldOsExit := osExit
	defer func() { osExit = oldOsExit }()

	exitCode = -1
	mockExit := func(code int) {
		t.Logf("os.Exit is called with exit code %d", code)
		exitCode = code
	}
	osExit = mockExit

	InitMSP()

	signer, err := common.GetDefaultSigner()
	assert.NoError(t, err)

	mockCF := &ChannelCmdFactory{
		BroadcastFactory: mockBroadcastClientFactory,
		DeliverClient:    getMockDeliverClient(mockChannel),
		Signer:           signer,
	}

	cmd := checkexistsCmd(mockCF)
	AddFlags(cmd)

	args := []string{"-c", "mockChannel"}
	cmd.SetArgs(args)

	err = cmd.Execute()

	assert.NoError(t, err, "Command returned error")
	assert.NotEqual(t, -1, exitCode, "os.Exit is not called")
	assert.Equal(t, 0, exitCode, "os.Exit code is not 0")
}

func TestChannelDoesNotExist(t *testing.T) {
	defer resetFlags()

	// Save current os.Exit function and restore at the end:
	oldOsExit := osExit
	defer func() { osExit = oldOsExit }()

	exitCode = -1
	mockExit := func(code int) {
		t.Logf("os.Exit is called with exit code %d", code)
		exitCode = code
	}
	osExit = mockExit

	InitMSP()

	signer, err := common.GetDefaultSigner()
	assert.NoError(t, err)

	mockCF := &ChannelCmdFactory{
		BroadcastFactory: mockBroadcastClientFactory,
		DeliverClient:    getMockDeliverClientWithStatusNotFound(),
		Signer:           signer,
	}

	cmd := checkexistsCmd(mockCF)
	AddFlags(cmd)

	args := []string{"-c", "mockChannel"}
	cmd.SetArgs(args)

	err = cmd.Execute()

	assert.NoError(t, err, "Command returned error")
	assert.NotEqual(t, -1, exitCode, "os.Exit is not called")
	assert.Equal(t, 99, exitCode, "os.Exit code is not 99")
}

func getMockDeliverClientWithStatusNotFound() *common.DeliverClient {
	mockD := &mock.DeliverService{}
	statusResponse := &ab.DeliverResponse{
		Type: &ab.DeliverResponse_Status{Status: cb.Status_NOT_FOUND},
	}
	mockD.RecvStub = func() (*ab.DeliverResponse, error) {
		return statusResponse, errors.New("Error: can't read the block: &{NOT_FOUND}")
	}
	mockD.CloseSendReturns(nil)

	p := &common.DeliverClient{
		Service: mockD,
	}
	return p
}
