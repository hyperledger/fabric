/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"bytes"
	"testing"

	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/peer/lifecycle"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

func TestPrintQueryApprovalStatusResponse(t *testing.T) {
	proposalResponse := &peer.ProposalResponse{
		Response: &peer.Response{
			Payload: []byte("barf"),
		},
	}

	err := printQueryApprovalStatusResponse(proposalResponse, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected EOF")

	proposalResponse = &peer.ProposalResponse{
		Response: &peer.Response{
			Payload: protoutil.MarshalOrPanic(&lifecycle.QueryApprovalStatusResults{
				Approved: map[string]bool{
					"org2": false,
					"org1": true,
				},
			}),
		},
	}

	bs := bytes.NewBufferString("")
	err = printQueryApprovalStatusResponse(proposalResponse, bs)
	assert.NoError(t, err)
	assert.Contains(t, bs.String(), "\"org1\": true")
	assert.Contains(t, bs.String(), "\"org2\": false")
}

type mockSigner struct {
	SignErr      error
	SerializeErr error
}

func (m *mockSigner) Sign(message []byte) ([]byte, error) {
	return []byte("signature"), m.SignErr
}

func (m *mockSigner) Serialize() ([]byte, error) {
	return []byte("serialised"), m.SerializeErr
}

type mockEndoreser struct {
	ProcessProposalRv    *peer.ProposalResponse
	ProcessProposalError error
}

func (m *mockEndoreser) ProcessProposal(ctx context.Context, in *peer.SignedProposal, opts ...grpc.CallOption) (*peer.ProposalResponse, error) {
	return m.ProcessProposalRv, m.ProcessProposalError
}

func TestQueryApprovalStatusCmd(t *testing.T) {
	me := &mockEndoreser{}

	channelID = ""
	chaincodeName = ""
	chaincodeVersion = ""
	sequence = 0

	cmd := queryApprovalStatusCmd(&QueryApprovalStatus{
		Signer:         &mockSigner{},
		EndorserClient: me,
	})
	assert.NotNil(t, cmd)
	err := cmd.RunE(&cobra.Command{
		Use: "barf",
	}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "The required parameter 'channelID' is empty. Rerun the command with -C flag")

	channelID = "my-channel"
	cmd = queryApprovalStatusCmd(&QueryApprovalStatus{
		Signer:         &mockSigner{},
		EndorserClient: me,
	})
	assert.NotNil(t, cmd)
	err = cmd.RunE(&cobra.Command{
		Use: "barf",
	}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "The required parameter 'name' is empty. Rerun the command with -n flag")

	chaincodeName = "my-chaincode"
	cmd = queryApprovalStatusCmd(&QueryApprovalStatus{
		Signer:         &mockSigner{},
		EndorserClient: me,
	})
	assert.NotNil(t, cmd)
	err = cmd.RunE(&cobra.Command{
		Use: "barf",
	}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "The required parameter 'version' is empty. Rerun the command with -v flag")

	chaincodeVersion = "my-version"
	cmd = queryApprovalStatusCmd(&QueryApprovalStatus{
		Signer:         &mockSigner{},
		EndorserClient: me,
	})
	assert.NotNil(t, cmd)
	err = cmd.RunE(&cobra.Command{
		Use: "barf",
	}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "The required parameter 'sequence' is empty. Rerun the command with --sequence flag")

	sequence = 35
	signaturePolicy = "MAD"
	cmd = queryApprovalStatusCmd(&QueryApprovalStatus{
		Signer:         &mockSigner{},
		EndorserClient: me,
	})
	assert.NotNil(t, cmd)
	err = cmd.RunE(&cobra.Command{
		Use: "barf",
	}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid signature policy: MAD")

	signaturePolicy = "AND('A.member', 'B.member')"
	cmd = queryApprovalStatusCmd(&QueryApprovalStatus{
		EndorserClient: me,
	})
	assert.NotNil(t, cmd)
	err = cmd.RunE(&cobra.Command{
		Use: "barf",
	}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error creating signed proposal: nil signer provided")

	signaturePolicy = "AND('A.member', 'B.member')"
	collectionsConfigFile = "not.there"
	cmd = queryApprovalStatusCmd(&QueryApprovalStatus{
		Signer:         &mockSigner{},
		EndorserClient: me,
	})
	assert.NotNil(t, cmd)
	err = cmd.RunE(&cobra.Command{
		Use: "barf",
	}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid collection configuration in file not.there: could not read file 'not.there': open not.there: no such file or directory")

	collectionsConfigFile = ""
	me.ProcessProposalError = errors.New("nopes")
	cmd = queryApprovalStatusCmd(&QueryApprovalStatus{
		Signer:         &mockSigner{},
		EndorserClient: me,
	})
	assert.NotNil(t, cmd)
	err = cmd.RunE(&cobra.Command{
		Use: "barf",
	}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error endorsing proposal: nopes")

	me.ProcessProposalError = nil
	err = cmd.RunE(&cobra.Command{
		Use: "barf",
	}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "received nil proposal response")

	me.ProcessProposalRv = &peer.ProposalResponse{}
	err = cmd.RunE(&cobra.Command{
		Use: "barf",
	}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "proposal response had nil response")

	me.ProcessProposalRv = &peer.ProposalResponse{
		Response: &peer.Response{
			Status:  35,
			Message: "ho-ho-ho",
		},
	}
	err = cmd.RunE(&cobra.Command{
		Use: "barf",
	}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "query failed with status: 35 - ho-ho-ho")

	me.ProcessProposalRv = &peer.ProposalResponse{
		Response: &peer.Response{
			Status: int32(common.Status_SUCCESS),
		},
	}
	cmd = queryApprovalStatusCmd(&QueryApprovalStatus{
		Signer: &mockSigner{
			SignErr: errors.New("I can't sign"),
		},
		EndorserClient: me,
	})
	assert.NotNil(t, cmd)
	err = cmd.RunE(&cobra.Command{
		Use: "barf",
	}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error creating signed proposal: error signing proposal: I can't sign")

	me.ProcessProposalRv = &peer.ProposalResponse{
		Response: &peer.Response{
			Status: int32(common.Status_SUCCESS),
		},
	}
	cmd = queryApprovalStatusCmd(&QueryApprovalStatus{
		Signer: &mockSigner{
			SerializeErr: errors.New("I can't serialize"),
		},
		EndorserClient: me,
	})
	assert.NotNil(t, cmd)
	err = cmd.RunE(&cobra.Command{
		Use: "barf",
	}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error creating signed proposal: error serializing identity: I can't serialize")

	me.ProcessProposalRv = &peer.ProposalResponse{
		Response: &peer.Response{
			Status: int32(common.Status_SUCCESS),
		},
	}
	cmd = queryApprovalStatusCmd(&QueryApprovalStatus{
		Signer:         &mockSigner{},
		EndorserClient: me,
	})
	assert.NotNil(t, cmd)
	err = cmd.RunE(&cobra.Command{
		Use: "barf",
	}, nil)
	assert.NoError(t, err)
}
