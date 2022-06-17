/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channel

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	cb "github.com/hyperledger/fabric-protos-go/common"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/internal/peer/common"
	"github.com/hyperledger/fabric/internal/peer/common/mock"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestFetch(t *testing.T) {
	defer resetFlags()
	InitMSP()
	resetFlags()
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()

	mockchain := "mockchain"

	signer, err := common.GetDefaultSigner()
	if err != nil {
		t.Fatalf("Get default signer error: %v", err)
	}

	mockCF := &ChannelCmdFactory{
		BroadcastFactory: mockBroadcastClientFactory,
		Signer:           signer,
		DeliverClient:    getMockDeliverClient(mockchain),
	}

	tempDir := t.TempDir()

	cmd := fetchCmd(mockCF)
	AddFlags(cmd)

	// success cases - block and outputBlockPath
	blocksToFetch := []string{"oldest", "newest", "config", "1"}
	for _, bestEffort := range []bool{false, true} {
		for _, block := range blocksToFetch {
			outputBlockPath := filepath.Join(tempDir, block+".block")
			args := []string{"-c", mockchain, block, outputBlockPath}
			if bestEffort {
				args = append(args, "--bestEffort")
			}
			cmd.SetArgs(args)

			err = cmd.Execute()
			require.NoError(t, err, "fetch command expected to succeed")

			if _, err := os.Stat(outputBlockPath); os.IsNotExist(err) {
				// path/to/whatever does not exist
				t.Error("expected configuration block to be fetched")
				t.Fail()
			}
		}
	}

	// failure case
	blocksToFetchBad := []string{"banana"}
	for _, block := range blocksToFetchBad {
		outputBlockPath := filepath.Join(tempDir, block+".block")
		args := []string{"-c", mockchain, block, outputBlockPath}
		cmd.SetArgs(args)

		err = cmd.Execute()
		require.Error(t, err, "fetch command expected to fail")
		require.Regexp(t, err.Error(), fmt.Sprintf("fetch target illegal: %s", block))

		if fileInfo, _ := os.Stat(outputBlockPath); fileInfo != nil {
			// path/to/whatever does exist
			t.Error("expected configuration block to not be fetched")
			t.Fail()
		}
	}
}

func TestFetchArgs(t *testing.T) {
	// failure - no args
	cmd := fetchCmd(nil)
	AddFlags(cmd)
	err := cmd.Execute()
	require.Error(t, err, "fetch command expected to fail")
	require.Contains(t, err.Error(), "fetch target required")

	// failure - too many args
	args := []string{"strawberry", "kiwi", "lemonade"}
	cmd.SetArgs(args)
	err = cmd.Execute()
	require.Error(t, err, "fetch command expected to fail")
	require.Contains(t, err.Error(), "trailing args detected")
}

func TestFetchNilCF(t *testing.T) {
	defer viper.Reset()
	defer resetFlags()

	InitMSP()
	resetFlags()
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()

	mockchain := "mockchain"
	viper.Set("peer.client.connTimeout", 10*time.Millisecond)
	cmd := fetchCmd(nil)
	AddFlags(cmd)
	args := []string{"-c", mockchain, "oldest"}
	cmd.SetArgs(args)
	err := cmd.Execute()
	require.Error(t, err, "fetch command expected to fail")
	require.Contains(t, err.Error(), "deliver client failed to connect to")
}

func getMockDeliverClient(channelID string) *common.DeliverClient {
	p := getMockDeliverClientWithBlock(channelID, createTestBlock())
	return p
}

func getMockDeliverClientWithBlock(channelID string, block *cb.Block) *common.DeliverClient {
	p := &common.DeliverClient{
		Service:     getMockDeliverService(block),
		ChannelID:   channelID,
		TLSCertHash: []byte("tlscerthash"),
	}
	return p
}

func getMockDeliverService(block *cb.Block) *mock.DeliverService {
	mockD := &mock.DeliverService{}
	blockResponse := &ab.DeliverResponse{
		Type: &ab.DeliverResponse_Block{
			Block: block,
		},
	}
	statusResponse := &ab.DeliverResponse{
		Type: &ab.DeliverResponse_Status{Status: cb.Status_SUCCESS},
	}
	mockD.RecvStub = func() (*ab.DeliverResponse, error) {
		// alternate returning block and status
		if mockD.RecvCallCount()%2 == 1 {
			return blockResponse, nil
		}
		return statusResponse, nil
	}
	mockD.CloseSendReturns(nil)
	return mockD
}

func createTestBlock() *cb.Block {
	metadataBytes := protoutil.MarshalOrPanic(&cb.Metadata{
		Value: protoutil.MarshalOrPanic(&cb.OrdererBlockMetadata{
			LastConfig: &cb.LastConfig{Index: 0},
		}),
	})

	block := &cb.Block{
		Header: &cb.BlockHeader{
			Number: 0,
		},
		Metadata: &cb.BlockMetadata{
			Metadata: [][]byte{metadataBytes},
		},
	}

	return block
}
