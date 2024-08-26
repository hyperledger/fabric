/*
Copyright IBM Corp, SecureKey Technologies Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package filter

import (
	"context"
	"time"

	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/hyperledger/fabric/core/handlers/auth"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

// NewTimeWindowCheckFilter creates a new Filter that checks timewindow expiration
func NewTimeWindowCheckFilter(timeWindow time.Duration) auth.Filter {
	return &timewindowCheckFilter{
		timeWindow: timeWindow,
	}
}

type timewindowCheckFilter struct {
	next       peer.EndorserServer
	timeWindow time.Duration
}

// Init initializes the Filter with the next EndorserServer
func (f *timewindowCheckFilter) Init(next peer.EndorserServer) {
	f.next = next
}

func validateTimewindowProposal(signedProp *peer.SignedProposal, timeWindow time.Duration) error {
	prop, err := protoutil.UnmarshalProposal(signedProp.ProposalBytes)
	if err != nil {
		return errors.Wrap(err, "failed parsing proposal")
	}

	hdr, err := protoutil.UnmarshalHeader(prop.Header)
	if err != nil {
		return errors.Wrap(err, "failed parsing header")
	}

	chdr, err := protoutil.UnmarshalChannelHeader(hdr.ChannelHeader)
	if err != nil {
		return errors.Wrap(err, "failed parsing channel header")
	}

	timeProposal := chdr.Timestamp.AsTime().UTC()
	now := time.Now().UTC()

	if timeProposal.Add(timeWindow).Before(now) || timeProposal.Add(-timeWindow).After(now) {
		return errors.Errorf("request unauthorized due to incorrect timestamp %s, peer time %s, peer.authentication.timewindow %s",
			timeProposal.Format(time.RFC3339), now.Format(time.RFC3339), timeWindow.String())
	}

	return nil
}

// ProcessProposal processes a signed proposal
func (f *timewindowCheckFilter) ProcessProposal(ctx context.Context, signedProp *peer.SignedProposal) (*peer.ProposalResponse, error) {
	if err := validateTimewindowProposal(signedProp, f.timeWindow); err != nil {
		return nil, err
	}
	return f.next.ProcessProposal(ctx, signedProp)
}
