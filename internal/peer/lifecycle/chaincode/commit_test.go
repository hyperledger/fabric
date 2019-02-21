/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"errors"
	"testing"
	"time"

	"github.com/hyperledger/fabric/internal/peer/common"
	"github.com/hyperledger/fabric/internal/peer/common/api"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestCommitter(t *testing.T) {
	assert := assert.New(t)

	t.Run("success", func(t *testing.T) {
		c := newCommitterForTest(t, nil, nil)
		c.Input = &CommitInput{
			Name:      "testcc",
			Version:   "testversion",
			Sequence:  1,
			ChannelID: "testchannel",
		}

		err := c.Commit()
		assert.NoError(err)
	})

	t.Run("failure - validation fails due to no name provided", func(t *testing.T) {
		c := newCommitterForTest(t, nil, nil)
		c.Input = &CommitInput{
			Version:   "testversion",
			Sequence:  1,
			ChannelID: "testchannel",
		}

		err := c.Commit()
		assert.EqualError(err, "The required parameter 'name' is empty. Rerun the command with -n flag")
	})

	t.Run("failure - creating signed proposal fails", func(t *testing.T) {
		c := newCommitterForTest(t, nil, nil)
		c.Input = &CommitInput{
			Name:      "testcc",
			Version:   "testversion",
			Sequence:  1,
			ChannelID: "testchannel",
		}
		c.Signer = nil

		err := c.Commit()
		assert.EqualError(err, "error creating signed proposal: nil signer provided")
	})

	t.Run("endorser client returns error", func(t *testing.T) {
		ec := common.GetMockEndorserClient(nil, errors.New("badbadnotgood"))
		c := newCommitterForTest(t, nil, ec)
		c.Input = &CommitInput{
			Name:      "testcc",
			Version:   "testversion",
			Sequence:  1,
			ChannelID: "testchannel",
		}

		err := c.Commit()
		assert.EqualError(err, "error endorsing proposal: badbadnotgood")
	})

	t.Run("endorser client returns a nil proposal response", func(t *testing.T) {
		ec := common.GetMockEndorserClient(nil, nil)
		c := newCommitterForTest(t, nil, ec)
		c.Input = &CommitInput{
			Name:      "testcc",
			Version:   "testversion",
			Sequence:  1,
			ChannelID: "testchannel",
		}

		err := c.Commit()
		assert.EqualError(err, "received nil proposal response")
	})

	t.Run("endorser client returns a proposal response with nil response", func(t *testing.T) {
		mockResponse := &pb.ProposalResponse{
			Response: nil,
		}
		ec := common.GetMockEndorserClient(mockResponse, nil)
		c := newCommitterForTest(t, nil, ec)
		c.Input = &CommitInput{
			Name:      "testcc",
			Version:   "testversion",
			Sequence:  1,
			ChannelID: "testchannel",
		}

		err := c.Commit()
		assert.EqualError(err, "proposal response had nil response")
	})

	t.Run("broadcast client returns an error", func(t *testing.T) {
		c := newCommitterForTest(t, nil, nil)
		c.BroadcastClient = common.GetMockBroadcastClient(errors.New("jinkies"))

		c.Input = &CommitInput{
			Name:      "testcc",
			Version:   "testversion",
			Sequence:  1,
			ChannelID: "testchannel",
		}

		err := c.Commit()
		assert.EqualError(err, "error sending transaction for commit: jinkies")
	})

	t.Run("endorser client returns a failure status code", func(t *testing.T) {
		mockResponse := &pb.ProposalResponse{
			Response: &pb.Response{
				Status:  500,
				Message: "rutroh",
			},
		}
		ec := common.GetMockEndorserClient(mockResponse, nil)
		c := newCommitterForTest(t, nil, ec)
		c.Input = &CommitInput{
			Name:      "testcc",
			Version:   "testversion",
			Sequence:  1,
			ChannelID: "testchannel",
		}

		err := c.Commit()
		assert.EqualError(err, "bad response: 500 - rutroh")
	})

	t.Run("wait for event success", func(t *testing.T) {
		// success - one deliver client first receives block without txid and
		// then one with txid
		c := newCommitterForTest(t, nil, nil)
		filteredBlocks := []*pb.FilteredBlock{
			createFilteredBlock("theseare", "notthetxidsyouarelookingfor"),
			createFilteredBlock("txid0"),
		}
		mockDCTwoBlocks := getMockDeliverClientRespondsWithFilteredBlocks(filteredBlocks)
		mockDC := getMockDeliverClientResponseWithTxID("txid0")
		c.DeliverClients = []api.PeerDeliverClient{mockDCTwoBlocks, mockDC}
		c.Input = &CommitInput{
			Name:                "testcc",
			Version:             "testversion",
			Sequence:            1,
			ChannelID:           "testchannel",
			PeerAddresses:       []string{"peer0", "peer1"},
			WaitForEvent:        true,
			WaitForEventTimeout: 30 * time.Second,
			TxID:                "txid0",
		}

		err := c.Commit()
		assert.NoError(err)
	})

	t.Run("wait for event failure - one deliver client returns error", func(t *testing.T) {
		c := newCommitterForTest(t, nil, nil)
		mockDCErr := getMockDeliverClientWithErr("moist")
		mockDC := getMockDeliverClientResponseWithTxID("txid0")
		c.DeliverClients = []api.PeerDeliverClient{mockDCErr, mockDC}
		c.Input = &CommitInput{
			Name:                "testcc",
			Version:             "testversion",
			Sequence:            1,
			ChannelID:           "testchannel",
			PeerAddresses:       []string{"peer0", "peer1"},
			WaitForEvent:        true,
			WaitForEventTimeout: 30 * time.Second,
			TxID:                "txid0",
		}

		err := c.Commit()
		assert.Error(err)
		assert.Contains(err.Error(), "moist")
	})

	t.Run("wait for event failure - both deliver clients don't return an event with the expected txid", func(t *testing.T) {
		c := newCommitterForTest(t, nil, nil)
		delayChan := make(chan struct{})
		mockDCDelay := getMockDeliverClientRespondAfterDelay(delayChan, "txid0")
		c.DeliverClients = []api.PeerDeliverClient{mockDCDelay, mockDCDelay}
		c.Input = &CommitInput{
			Name:                "testcc",
			Version:             "testversion",
			Sequence:            1,
			ChannelID:           "testchannel",
			PeerAddresses:       []string{"peer0", "peer1"},
			WaitForEvent:        true,
			WaitForEventTimeout: 10 * time.Millisecond,
			TxID:                "txid0",
		}

		err := c.Commit()
		assert.Error(err)
		assert.Contains(err.Error(), "timed out")
	})
}

func TestCommitCmd(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		resetFlags()
		c := newCommitterForTest(t, nil, nil)
		cmd := commitCmd(nil, c)
		c.Command = cmd
		args := []string{
			"-C", "testchannel",
			"-n", "testcc",
			"-v", "1.0",
			"--sequence", "1",
			"-P", `AND ('Org1MSP.member','Org2MSP.member')`,
		}
		cmd.SetArgs(args)

		err := cmd.Execute()
		assert.NoError(t, err)
	})

	t.Run("failure - invalid signature policy", func(t *testing.T) {
		resetFlags()
		c := newCommitterForTest(t, nil, nil)
		cmd := commitCmd(nil, c)
		c.Command = cmd
		args := []string{
			"-C", "testchannel",
			"-n", "testcc",
			"-v", "1.0",
			"--sequence", "1",
			"-P", "notapolicy",
		}
		cmd.SetArgs(args)

		err := cmd.Execute()
		assert.EqualError(t, err, "invalid signature policy: notapolicy")
	})

	t.Run("failure - invalid collection config file", func(t *testing.T) {
		resetFlags()
		c := newCommitterForTest(t, nil, nil)
		cmd := commitCmd(nil, c)
		c.Command = cmd
		args := []string{
			"-C", "testchannel",
			"-n", "testcc",
			"-v", "1.0",
			"--sequence", "1",
			"--collections-config", "idontexist.json",
		}
		cmd.SetArgs(args)

		err := cmd.Execute()
		assert.EqualError(t, err, "invalid collection configuration in file idontexist.json: could not read file 'idontexist.json': open idontexist.json: no such file or directory")
	})
}

func TestValidateCommitInput(t *testing.T) {
	defer resetFlags()
	assert := assert.New(t)

	t.Run("success - all required parameters provided", func(t *testing.T) {
		input := &CommitInput{
			Name:      "testcc",
			Version:   "testversion",
			Sequence:  1,
			ChannelID: "testchannel",
		}
		err := input.Validate()
		assert.NoError(err)
	})

	t.Run("failure - nil input", func(t *testing.T) {
		var input *CommitInput
		err := input.Validate()
		assert.EqualError(err, "nil input")
	})

	t.Run("failure - name not set", func(t *testing.T) {
		input := &CommitInput{
			Version:   "testversion",
			Sequence:  1,
			ChannelID: "testchannel",
		}
		err := input.Validate()
		assert.EqualError(err, "The required parameter 'name' is empty. Rerun the command with -n flag")
	})

	t.Run("failure - version not set", func(t *testing.T) {
		input := &CommitInput{
			Name:      "testcc",
			Sequence:  1,
			ChannelID: "testchannel",
		}
		err := input.Validate()
		assert.EqualError(err, "The required parameter 'version' is empty. Rerun the command with -v flag")
	})

	t.Run("failure - sequence not set", func(t *testing.T) {
		input := &CommitInput{
			Name:      "testcc",
			Version:   "testversion",
			ChannelID: "testchannel",
		}
		err := input.Validate()
		assert.EqualError(err, "The required parameter 'sequence' is empty. Rerun the command with --sequence flag")
	})

	t.Run("failure - channelID not set", func(t *testing.T) {
		input := &CommitInput{
			Name:     "testcc",
			Version:  "testversion",
			Sequence: 1,
		}
		err := input.Validate()
		assert.EqualError(err, "The required parameter 'channelID' is empty. Rerun the command with -C flag")
	})
}

func initCommitterForTest(t *testing.T, ec pb.EndorserClient, mockResponse *pb.ProposalResponse) (*cobra.Command, *CmdFactory) {
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
	mockCF := &CmdFactory{
		Signer:          signer,
		EndorserClients: []pb.EndorserClient{ec},
		BroadcastClient: mockBroadcastClient,
		DeliverClients:  mockDeliverClients,
	}

	cmd := commitCmd(mockCF, nil)
	addFlags(cmd)

	return cmd, mockCF
}

func newCommitterForTest(t *testing.T, r Reader, ec pb.EndorserClient) *Committer {
	_, mockCF := initCommitterForTest(t, ec, nil)

	return &Committer{
		BroadcastClient: mockCF.BroadcastClient,
		DeliverClients:  mockCF.DeliverClients,
		EndorserClients: mockCF.EndorserClients,
		Signer:          mockCF.Signer,
	}
}
