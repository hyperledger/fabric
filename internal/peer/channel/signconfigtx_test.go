/*
Copyright IBM Corp. All Rights Reserved.

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

func TestSignConfigtx(t *testing.T) {
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
		Signer: signer,
	}

	cmd := signconfigtxCmd(mockCF)

	AddFlags(cmd)

	args := []string{"-f", configtxFile}
	cmd.SetArgs(args)

	require.NoError(t, cmd.Execute())
}

func TestSignConfigtxMissingConfigTxFlag(t *testing.T) {
	InitMSP()
	resetFlags()

	signer, err := common.GetDefaultSigner()
	if err != nil {
		t.Fatalf("Get default signer error: %v", err)
	}

	mockCF := &ChannelCmdFactory{
		Signer: signer,
	}

	cmd := signconfigtxCmd(mockCF)

	AddFlags(cmd)

	cmd.SetArgs([]string{})

	require.Error(t, cmd.Execute())
}

func TestSignConfigtxChannelMissingConfigTxFile(t *testing.T) {
	InitMSP()
	resetFlags()

	signer, err := common.GetDefaultSigner()
	if err != nil {
		t.Fatalf("Get default signer error: %v", err)
	}

	mockCF := &ChannelCmdFactory{
		Signer: signer,
	}

	cmd := signconfigtxCmd(mockCF)

	AddFlags(cmd)

	args := []string{"-f", "Non-existent"}
	cmd.SetArgs(args)

	require.Error(t, cmd.Execute())
}
