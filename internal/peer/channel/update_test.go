/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channel

import (
	"path/filepath"
	"testing"

	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/internal/peer/common"
	"github.com/stretchr/testify/require"
)

const mockChannel = "mockChannel"

func TestUpdateChannel(t *testing.T) {
	InitMSP()
	resetFlags()

	dir := t.TempDir()

	configtxFile := filepath.Join(dir, mockChannel)
	if _, err := createTxFile(configtxFile, cb.HeaderType_CONFIG_UPDATE, mockChannel); err != nil {
		t.Fatalf("couldn't create tx file")
	}

	signer, err := common.GetDefaultSigner()
	if err != nil {
		t.Fatalf("Get default signer error: %v", err)
	}

	mockCF := &ChannelCmdFactory{
		BroadcastFactory: mockBroadcastClientFactory,
		Signer:           signer,
		DeliverClient:    &mockDeliverClient{},
	}

	cmd := updateCmd(mockCF)

	AddFlags(cmd)

	args := []string{"-c", mockChannel, "-f", configtxFile, "-o", "localhost:7050"}
	cmd.SetArgs(args)

	require.NoError(t, cmd.Execute())
}

func TestUpdateChannelMissingConfigTxFlag(t *testing.T) {
	InitMSP()
	resetFlags()

	signer, err := common.GetDefaultSigner()
	if err != nil {
		t.Fatalf("Get default signer error: %v", err)
	}

	mockCF := &ChannelCmdFactory{
		BroadcastFactory: mockBroadcastClientFactory,
		Signer:           signer,
		DeliverClient:    &mockDeliverClient{},
	}

	cmd := updateCmd(mockCF)

	AddFlags(cmd)

	args := []string{"-c", mockChannel, "-o", "localhost:7050"}
	cmd.SetArgs(args)

	require.Error(t, cmd.Execute())
}

func TestUpdateChannelMissingConfigTxFile(t *testing.T) {
	InitMSP()
	resetFlags()

	signer, err := common.GetDefaultSigner()
	if err != nil {
		t.Fatalf("Get default signer error: %v", err)
	}

	mockCF := &ChannelCmdFactory{
		BroadcastFactory: mockBroadcastClientFactory,
		Signer:           signer,
		DeliverClient:    &mockDeliverClient{},
	}

	cmd := updateCmd(mockCF)

	AddFlags(cmd)

	args := []string{"-c", mockChannel, "-f", "Non-existent", "-o", "localhost:7050"}
	cmd.SetArgs(args)

	require.Error(t, cmd.Execute())
}

func TestUpdateChannelMissingChannelID(t *testing.T) {
	InitMSP()
	resetFlags()

	dir := t.TempDir()

	configtxFile := filepath.Join(dir, mockChannel)
	if _, err := createTxFile(configtxFile, cb.HeaderType_CONFIG_UPDATE, mockChannel); err != nil {
		t.Fatalf("couldn't create tx file")
	}

	signer, err := common.GetDefaultSigner()
	if err != nil {
		t.Fatalf("Get default signer error: %v", err)
	}

	mockCF := &ChannelCmdFactory{
		BroadcastFactory: mockBroadcastClientFactory,
		Signer:           signer,
		DeliverClient:    &mockDeliverClient{},
	}

	cmd := updateCmd(mockCF)

	AddFlags(cmd)

	args := []string{"-f", configtxFile, "-o", "localhost:7050"}
	cmd.SetArgs(args)

	require.Error(t, cmd.Execute())
}
