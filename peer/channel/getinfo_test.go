/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channel

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/peer/common"
	cb "github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/stretchr/testify/assert"
)

func TestGetChannelInfo(t *testing.T) {
	InitMSP()
	resetFlags()

	mockBlockchainInfo := &cb.BlockchainInfo{
		Height:            1,
		CurrentBlockHash:  []byte("CurrentBlockHash"),
		PreviousBlockHash: []byte("PreviousBlockHash"),
	}
	mockPayload, err := proto.Marshal(mockBlockchainInfo)
	assert.NoError(t, err)

	mockResponse := &pb.ProposalResponse{
		Response: &pb.Response{
			Status:  200,
			Payload: mockPayload,
		},
		Endorsement: &pb.Endorsement{},
	}

	signer, err := common.GetDefaultSigner()
	assert.NoError(t, err)

	mockCF := &ChannelCmdFactory{
		EndorserClient:   common.GetMockEndorserClient(mockResponse, nil),
		BroadcastFactory: mockBroadcastClientFactory,
		Signer:           signer,
	}

	cmd := getinfoCmd(mockCF)
	AddFlags(cmd)

	args := []string{"-c", mockChannel}
	cmd.SetArgs(args)

	assert.NoError(t, cmd.Execute())
}

func TestGetChannelInfoMissingChannelID(t *testing.T) {
	InitMSP()
	resetFlags()

	signer, err := common.GetDefaultSigner()
	if err != nil {
		t.Fatalf("Get default signer error: %v", err)
	}

	mockCF := &ChannelCmdFactory{
		Signer: signer,
	}

	cmd := getinfoCmd(mockCF)

	AddFlags(cmd)

	cmd.SetArgs([]string{})

	assert.Error(t, cmd.Execute())
}
