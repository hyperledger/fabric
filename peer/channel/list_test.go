package channel

import (
	"testing"

	"github.com/golang/protobuf/proto"
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
}
