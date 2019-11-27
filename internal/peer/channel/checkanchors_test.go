/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channel

import (
	"testing"

	cb "github.com/hyperledger/fabric-protos-go/common"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/internal/peer/common"
	"github.com/hyperledger/fabric/protoutil"

	"github.com/stretchr/testify/assert"
)

func TestAnchorsMissingChannelID(t *testing.T) {
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

	cmd := checkanchorsCmd(mockCF)
	AddFlags(cmd)

	args := []string{"-g", "mockOrg"}
	cmd.SetArgs(args)

	err = cmd.Execute()
	t.Logf("Error: %s", err)

	assert.Error(t, err, "Command should return error")
	assert.Contains(t, err.Error(), "must supply channel ID")
}

func TestAnchorsMissingOrgID(t *testing.T) {
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

	cmd := checkanchorsCmd(mockCF)
	AddFlags(cmd)

	args := []string{"-c", "mockChannel"}
	cmd.SetArgs(args)

	err = cmd.Execute()
	t.Logf("Error: %s", err)

	assert.Error(t, err, "Command should return error")
	assert.Contains(t, err.Error(), "must supply organization ID")
}

func TestAnchors(t *testing.T) {
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

	anchorPeers := &pb.AnchorPeers{
		AnchorPeers: []*pb.AnchorPeer{{
			Host: "doesnt matter",
			Port: 42,
		}},
	}

	mockCF := &ChannelCmdFactory{
		BroadcastFactory: mockBroadcastClientFactory,
		DeliverClient:    anchorsMockDeliverClient(anchorPeers),
		Signer:           signer,
	}

	cmd := checkanchorsCmd(mockCF)
	AddFlags(cmd)

	args := []string{"-c", "mockChannel", "-g", "mockOrg"}
	cmd.SetArgs(args)

	err = cmd.Execute()

	assert.NoError(t, err, "Command returned error")
	assert.NotEqual(t, -1, exitCode, "os.Exit is not called")
	assert.Equal(t, 0, exitCode, "os.Exit code is not 0")
}

func TestNoAnchors(t *testing.T) {
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
		DeliverClient:    anchorsMockDeliverClient(nil),
		Signer:           signer,
	}

	cmd := checkanchorsCmd(mockCF)
	AddFlags(cmd)

	args := []string{"-c", "mockChannel", "-g", "mockOrg"}
	cmd.SetArgs(args)

	err = cmd.Execute()

	assert.NoError(t, err, "Command returned error")
	assert.NotEqual(t, -1, exitCode, "os.Exit is not called")
	assert.Equal(t, 99, exitCode, "os.Exit code is not 0")

}

func anchorsMockDeliverClient(anchorPeers *pb.AnchorPeers) *common.DeliverClient {
	return getMockDeliverClientWithBlock("channelID doesnt matter", anchorsTestBlock(anchorPeers))
}

func anchorsTestBlock(anchorPeers *pb.AnchorPeers) *cb.Block {
	var anchorPeersValue []byte = nil
	if anchorPeers != nil {
		anchorPeersValue = protoutil.MarshalOrPanic(anchorPeers)
	}

	payload := &cb.Payload{
		Header: protoutil.MakePayloadHeader(
			protoutil.MakeChannelHeader(cb.HeaderType_MESSAGE, int32(1), "test", 0),
			protoutil.MakeSignatureHeader([]byte("creator"), []byte("nonce"))),

		Data: protoutil.MarshalOrPanic(&cb.ConfigEnvelope{
			Config: &cb.Config{
				ChannelGroup: &cb.ConfigGroup{
					Groups: map[string]*cb.ConfigGroup{
						"Application": {
							Groups: map[string]*cb.ConfigGroup{
								"mockOrg": {
									Values: map[string]*cb.ConfigValue{
										"AnchorPeers": {
											Value: anchorPeersValue,
										},
									},
								},
							},
						},
					},
				},
			},
		}),
	}
	envelope := &cb.Envelope{Payload: protoutil.MarshalOrPanic(payload)}

	lc := &cb.LastConfig{Index: 0}
	lcBytes := protoutil.MarshalOrPanic(lc)
	metadata := &cb.Metadata{
		Value: lcBytes,
	}
	metadataBytes := protoutil.MarshalOrPanic(metadata)
	blockMetadata := make([][]byte, cb.BlockMetadataIndex_LAST_CONFIG+1)
	blockMetadata[cb.BlockMetadataIndex_LAST_CONFIG] = metadataBytes

	return &cb.Block{
		Header: &cb.BlockHeader{
			Number: 0,
		},
		Metadata: &cb.BlockMetadata{
			Metadata: blockMetadata,
		},
		Data: &cb.BlockData{
			Data: [][]byte{protoutil.MarshalOrPanic(envelope)},
		},
	}
}
