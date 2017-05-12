/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package channel

import (
	"errors"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/peer/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/stretchr/testify/assert"
)

func TestListChannels(t *testing.T) {
	InitMSP()

	mockChannelResponse := &pb.ChannelQueryResponse{
		Channels: []*pb.ChannelInfo{{
			ChannelId: "TEST_LIST_CHANNELS",
		}},
	}

	mockPayload, err := proto.Marshal(mockChannelResponse)
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

	cmd := listCmd(mockCF)
	AddFlags(cmd)
	if err := cmd.Execute(); err != nil {
		t.Fail()
		t.Error(err)
	}

	testListChannelsEmptyCF(t, mockCF)
}

func testListChannelsEmptyCF(t *testing.T, mockCF *ChannelCmdFactory) {
	cmd := listCmd(nil)
	AddFlags(cmd)

	// Error case 1: no orderer endpoints
	getEndorserClient := common.GetEndorserClientFnc
	getBroadcastClient := common.GetBroadcastClientFnc
	getDefaultSigner := common.GetDefaultSignerFnc
	defer func() {
		common.GetEndorserClientFnc = getEndorserClient
		common.GetBroadcastClientFnc = getBroadcastClient
		common.GetDefaultSignerFnc = getDefaultSigner
	}()
	common.GetDefaultSignerFnc = func() (msp.SigningIdentity, error) {
		return nil, errors.New("error")
	}
	common.GetEndorserClientFnc = func() (pb.EndorserClient, error) {
		return mockCF.EndorserClient, nil
	}
	common.GetBroadcastClientFnc = func(orderingEndpoint string, tlsEnabled bool, caFile string) (common.BroadcastClient, error) {
		broadcastClient := common.GetMockBroadcastClient(nil)
		return broadcastClient, nil
	}

	err := cmd.Execute()
	assert.Error(t, err, "Error expected because GetDefaultSignerFnc returns an error")

	common.GetDefaultSignerFnc = getDefaultSigner
	common.GetEndorserClientFnc = func() (pb.EndorserClient, error) {
		return nil, errors.New("error")
	}
	err = cmd.Execute()
	assert.Error(t, err, "Error expected because GetEndorserClientFnc returns an error")

	common.GetEndorserClientFnc = func() (pb.EndorserClient, error) {
		return mockCF.EndorserClient, nil
	}
	err = cmd.Execute()
	assert.NoError(t, err, "Error occurred while executing list command")
}
