/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/hyperledger/fabric/peer/common"
	"github.com/hyperledger/fabric/peer/common/api"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestApproverForMyOrg(t *testing.T) {
	assert := assert.New(t)

	t.Run("success", func(t *testing.T) {
		a := newApproverForMyOrgForTest(t, nil, nil)
		a.Input = &ApproveForMyOrgInput{
			Name:      "testcc",
			Version:   "testversion",
			Hash:      []byte("hash"),
			Sequence:  1,
			ChannelID: "testchannel",
		}

		err := a.Approve()
		assert.NoError(err)
	})

	t.Run("failure - validation fails due to no name provided", func(t *testing.T) {
		a := newApproverForMyOrgForTest(t, nil, nil)
		a.Input = &ApproveForMyOrgInput{
			Version:   "testversion",
			Hash:      []byte("hash"),
			Sequence:  1,
			ChannelID: "testchannel",
		}

		err := a.Approve()
		assert.EqualError(err, "The required parameter 'name' is empty. Rerun the command with -n flag")
	})

	t.Run("failure - creating signed proposal fails", func(t *testing.T) {
		a := newApproverForMyOrgForTest(t, nil, nil)
		a.Input = &ApproveForMyOrgInput{
			Name:      "testcc",
			Version:   "testversion",
			Hash:      []byte("hash"),
			Sequence:  1,
			ChannelID: "testchannel",
		}
		a.Signer = nil

		err := a.Approve()
		assert.EqualError(err, "error creating signed proposal: nil signer provided")
	})

	t.Run("endorser client returns error", func(t *testing.T) {
		ec := common.GetMockEndorserClient(nil, errors.New("badbadnotgood"))
		a := newApproverForMyOrgForTest(t, nil, ec)
		a.Input = &ApproveForMyOrgInput{
			Name:      "testcc",
			Version:   "testversion",
			Hash:      []byte("hash"),
			Sequence:  1,
			ChannelID: "testchannel",
		}

		err := a.Approve()
		assert.EqualError(err, "error endorsing proposal: badbadnotgood")
	})

	t.Run("endorser client returns a proposal response with nil response", func(t *testing.T) {
		mockResponse := &pb.ProposalResponse{
			Response: nil,
		}
		ec := common.GetMockEndorserClient(mockResponse, nil)
		a := newApproverForMyOrgForTest(t, nil, ec)
		a.Input = &ApproveForMyOrgInput{
			Name:      "testcc",
			Version:   "testversion",
			Hash:      []byte("hash"),
			Sequence:  1,
			ChannelID: "testchannel",
		}

		err := a.Approve()
		assert.EqualError(err, "proposal response had nil response")
	})

	t.Run("endorser client returns a failure status code", func(t *testing.T) {
		mockResponse := &pb.ProposalResponse{
			Response: &pb.Response{
				Status:  500,
				Message: "rutroh",
			},
		}
		ec := common.GetMockEndorserClient(mockResponse, nil)
		a := newApproverForMyOrgForTest(t, nil, ec)
		a.Input = &ApproveForMyOrgInput{
			Name:      "testcc",
			Version:   "testversion",
			Hash:      []byte("hash"),
			Sequence:  1,
			ChannelID: "testchannel",
		}

		err := a.Approve()
		assert.EqualError(err, "bad response: 500 - rutroh")
	})

	t.Run("wait for event success", func(t *testing.T) {
		// success - one deliver client first receives block without txid and
		// then one with txid
		a := newApproverForMyOrgForTest(t, nil, nil)
		filteredBlocks := []*pb.FilteredBlock{
			createFilteredBlock("theseare", "notthetxidsyouarelookingfor"),
			createFilteredBlock("txid0"),
		}
		mockDCTwoBlocks := getMockDeliverClientRespondsWithFilteredBlocks(filteredBlocks)
		mockDC := getMockDeliverClientResponseWithTxID("txid0")
		a.DeliverClients = []api.PeerDeliverClient{mockDCTwoBlocks, mockDC}
		a.Input = &ApproveForMyOrgInput{
			Name:                "testcc",
			Version:             "testversion",
			Hash:                []byte("hash"),
			Sequence:            1,
			ChannelID:           "testchannel",
			PeerAddresses:       []string{"peer0", "peer1"},
			WaitForEvent:        true,
			WaitForEventTimeout: 30 * time.Second,
			TxID:                "txid0",
		}

		err := a.Approve()
		assert.NoError(err)
	})

	t.Run("wait for event failure - one deliver client returns error", func(t *testing.T) {
		a := newApproverForMyOrgForTest(t, nil, nil)
		mockDCErr := getMockDeliverClientWithErr("moist")
		mockDC := getMockDeliverClientResponseWithTxID("txid0")
		a.DeliverClients = []api.PeerDeliverClient{mockDCErr, mockDC}
		a.Input = &ApproveForMyOrgInput{
			Name:                "testcc",
			Version:             "testversion",
			Hash:                []byte("hash"),
			Sequence:            1,
			ChannelID:           "testchannel",
			PeerAddresses:       []string{"peer0", "peer1"},
			WaitForEvent:        true,
			WaitForEventTimeout: 30 * time.Second,
			TxID:                "txid0",
		}

		err := a.Approve()
		assert.Error(err)
		assert.Contains(err.Error(), "moist")
	})

	t.Run("wait for event failure - both deliver clients don't return an event with the expected txid before timeout", func(t *testing.T) {
		a := newApproverForMyOrgForTest(t, nil, nil)
		delayChan := make(chan struct{})
		mockDCDelay := getMockDeliverClientRespondAfterDelay(delayChan, "txid0")
		a.DeliverClients = []api.PeerDeliverClient{mockDCDelay, mockDCDelay}
		a.Input = &ApproveForMyOrgInput{
			Name:                "testcc",
			Version:             "testversion",
			Hash:                []byte("hash"),
			Sequence:            1,
			ChannelID:           "testchannel",
			PeerAddresses:       []string{"peer0", "peer1"},
			WaitForEvent:        true,
			WaitForEventTimeout: 10 * time.Millisecond,
			TxID:                "txid0",
		}

		err := a.Approve()
		assert.Error(err)
		assert.Contains(err.Error(), "timed out")
		close(delayChan)
	})
}

func TestApproveForMyOrgCmd(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		resetFlags()
		a := newApproverForMyOrgForTest(t, nil, nil)
		cmd := approveForMyOrgCmd(nil, a)
		a.Command = cmd
		args := []string{
			"-C", "testchannel",
			"-n", "testcc",
			"-v", "1.0",
			"--hash", hex.EncodeToString([]byte("hash")),
			"--sequence", "1",
			"-P", `AND ('Org1MSP.member','Org2MSP.member')`,
		}
		cmd.SetArgs(args)

		err := cmd.Execute()
		assert.NoError(t, err)
	})

	t.Run("failure - invalid signature policy", func(t *testing.T) {
		resetFlags()
		a := newApproverForMyOrgForTest(t, nil, nil)
		cmd := approveForMyOrgCmd(nil, a)
		a.Command = cmd
		args := []string{
			"-C", "testchannel",
			"-n", "testcc",
			"-v", "1.0",
			"--hash", hex.EncodeToString([]byte("hash")),
			"--sequence", "1",
			"-P", "notapolicy",
		}
		cmd.SetArgs(args)

		err := cmd.Execute()
		assert.EqualError(t, err, "invalid signature policy: notapolicy")
	})

	t.Run("failure - invalid collection config file", func(t *testing.T) {
		resetFlags()
		a := newApproverForMyOrgForTest(t, nil, nil)
		cmd := approveForMyOrgCmd(nil, a)
		a.Command = cmd
		args := []string{
			"-C", "testchannel",
			"-n", "testcc",
			"-v", "1.0",
			"--hash", hex.EncodeToString([]byte("hash")),
			"--sequence", "1",
			"--collections-config", "idontexist.json",
		}
		cmd.SetArgs(args)

		err := cmd.Execute()
		assert.EqualError(t, err, "invalid collection configuration in file idontexist.json: could not read file 'idontexist.json': open idontexist.json: no such file or directory")
	})
}

func TestValidateApproveForMyOrgInput(t *testing.T) {
	defer resetFlags()
	assert := assert.New(t)

	t.Run("success - all required parameters provided", func(t *testing.T) {
		input := &ApproveForMyOrgInput{
			Name:      "testcc",
			Version:   "testversion",
			Hash:      []byte("hash"),
			Sequence:  1,
			ChannelID: "testchannel",
		}
		err := input.Validate()
		assert.NoError(err)
	})

	t.Run("failure - nil input", func(t *testing.T) {
		var input *ApproveForMyOrgInput
		err := input.Validate()
		assert.EqualError(err, "nil input")
	})

	t.Run("failure - name not set", func(t *testing.T) {
		input := &ApproveForMyOrgInput{
			Version:   "testversion",
			Hash:      []byte("hash"),
			Sequence:  1,
			ChannelID: "testchannel",
		}
		err := input.Validate()
		assert.EqualError(err, "The required parameter 'name' is empty. Rerun the command with -n flag")
	})

	t.Run("failure - version not set", func(t *testing.T) {
		input := &ApproveForMyOrgInput{
			Name:      "testcc",
			Hash:      []byte("hash"),
			Sequence:  1,
			ChannelID: "testchannel",
		}
		err := input.Validate()
		assert.EqualError(err, "The required parameter 'version' is empty. Rerun the command with -v flag")
	})

	t.Run("failure - hash not set", func(t *testing.T) {
		input := &ApproveForMyOrgInput{
			Name:      "testcc",
			Version:   "testversion",
			Sequence:  1,
			ChannelID: "testchannel",
		}
		err := input.Validate()
		assert.EqualError(err, "The required parameter 'hash' is empty. Rerun the command with --hash flag")
	})

	t.Run("failure - sequence not set", func(t *testing.T) {
		input := &ApproveForMyOrgInput{
			Name:      "testcc",
			Version:   "testversion",
			Hash:      []byte("hash"),
			ChannelID: "testchannel",
		}
		err := input.Validate()
		assert.EqualError(err, "The required parameter 'sequence' is empty. Rerun the command with --sequence flag")
	})

	t.Run("failure - channelID not set", func(t *testing.T) {
		input := &ApproveForMyOrgInput{
			Name:     "testcc",
			Version:  "testversion",
			Hash:     []byte("hash"),
			Sequence: 1,
		}
		err := input.Validate()
		assert.EqualError(err, "The required parameter 'channelID' is empty. Rerun the command with -C flag")
	})
}

func initApproveForMyOrgTest(t *testing.T, ec pb.EndorserClient, mockResponse *pb.ProposalResponse) (*cobra.Command, *ChaincodeCmdFactory) {
	signer, err := common.GetDefaultSigner()
	if err != nil {
		t.Fatalf("Get default signer error: %v", err)
	}

	if mockResponse == nil {
		mockResponse = &pb.ProposalResponse{
			Response:    &pb.Response{Status: 200},
			Endorsement: &pb.Endorsement{},
		}
	}
	if ec == nil {
		ec = common.GetMockEndorserClient(mockResponse, nil)
	}

	mockBroadcastClient := common.GetMockBroadcastClient(nil)
	mockDC := getMockDeliverClientResponseWithTxID("txid0")
	mockDeliverClients := []api.PeerDeliverClient{mockDC, mockDC}
	mockCF := &ChaincodeCmdFactory{
		Signer:          signer,
		EndorserClients: []pb.EndorserClient{ec},
		BroadcastClient: mockBroadcastClient,
		DeliverClients:  mockDeliverClients,
	}

	cmd := approveForMyOrgCmd(mockCF, nil)
	addFlags(cmd)

	return cmd, mockCF
}

func newApproverForMyOrgForTest(t *testing.T, r Reader, ec pb.EndorserClient) *ApproverForMyOrg {
	_, mockCF := initApproveForMyOrgTest(t, ec, nil)

	return &ApproverForMyOrg{
		BroadcastClient: mockCF.BroadcastClient,
		DeliverClients:  mockCF.DeliverClients,
		EndorserClients: mockCF.EndorserClients,
		Signer:          mockCF.Signer,
	}
}
