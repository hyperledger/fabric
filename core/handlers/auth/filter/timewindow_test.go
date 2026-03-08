/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package filter

import (
	"context"
	"testing"
	"time"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func createSignedProposalForCheckTimeWindow(t *testing.T, tt time.Time) *peer.SignedProposal {
	sHdr := protoutil.MakeSignatureHeader(createX509Identity(t, "notExpiredCert.pem"), nil)
	hdr := protoutil.MakePayloadHeader(&common.ChannelHeader{
		Timestamp: timestamppb.New(tt),
	}, sHdr)
	hdrBytes, err := proto.Marshal(hdr)
	require.NoError(t, err)
	prop := &peer.Proposal{
		Header: hdrBytes,
	}
	propBytes, err := proto.Marshal(prop)
	require.NoError(t, err)
	return &peer.SignedProposal{
		ProposalBytes: propBytes,
	}
}

func TestTimeWindowCheckFilter(t *testing.T) {
	nextEndorser := &mockEndorserServer{}
	auth := NewTimeWindowCheckFilter(time.Minute * 15)
	auth.Init(nextEndorser)

	now := time.Now()

	// Scenario I: Not expired timestamp
	sp := createSignedProposalForCheckTimeWindow(t, now)
	_, err := auth.ProcessProposal(context.Background(), sp)
	require.NoError(t, err)
	require.True(t, nextEndorser.invoked)
	nextEndorser.invoked = false

	// Scenario II: Expired timestamp before
	sp = createSignedProposalForCheckTimeWindow(t, now.Add(-time.Minute*30))
	_, err = auth.ProcessProposal(context.Background(), sp)
	require.Contains(t, err.Error(), "request unauthorized due to incorrect timestamp")
	require.False(t, nextEndorser.invoked)

	// Scenario III: Expired timestamp after
	sp = createSignedProposalForCheckTimeWindow(t, now.Add(time.Minute*30))
	_, err = auth.ProcessProposal(context.Background(), sp)
	require.Contains(t, err.Error(), "request unauthorized due to incorrect timestamp")
	require.False(t, nextEndorser.invoked)
}
