/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channel

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/peer/common"
	"github.com/hyperledger/fabric/peer/common/mock"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	putils "github.com/hyperledger/fabric/protos/utils"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
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

	tempDir, err := ioutil.TempDir("", "fetch-output")
	if err != nil {
		t.Fatalf("failed to create temporary directory")
	}
	defer os.RemoveAll(tempDir)

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
			assert.NoError(t, err, "fetch command expected to succeed")

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
		assert.Error(t, err, "fetch command expected to fail")
		assert.Regexp(t, err.Error(), fmt.Sprintf("fetch target illegal: %s", block))

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
	assert.Error(t, err, "fetch command expected to fail")
	assert.Contains(t, err.Error(), "fetch target required")

	// failure - too many args
	args := []string{"strawberry", "kiwi", "lemonade"}
	cmd.SetArgs(args)
	err = cmd.Execute()
	assert.Error(t, err, "fetch command expected to fail")
	assert.Contains(t, err.Error(), "trailing args detected")
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
	assert.Error(t, err, "fetch command expected to fail")
	assert.Contains(t, err.Error(), "deliver client failed to connect to")
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
	lc := &cb.LastConfig{Index: 0}
	lcBytes := putils.MarshalOrPanic(lc)
	metadata := &cb.Metadata{
		Value: lcBytes,
	}
	metadataBytes := putils.MarshalOrPanic(metadata)
	blockMetadata := make([][]byte, cb.BlockMetadataIndex_LAST_CONFIG+1)
	blockMetadata[cb.BlockMetadataIndex_LAST_CONFIG] = metadataBytes
	block := &cb.Block{
		Header: &cb.BlockHeader{
			Number: 0,
		},
		Metadata: &cb.BlockMetadata{
			Metadata: blockMetadata,
		},
	}

	return block
}
