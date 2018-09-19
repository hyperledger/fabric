/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package filter

import (
	"context"
	"time"

	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/core/handlers/auth"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
)

// NewExpirationCheckFilter creates a new Filter that checks identity expiration
func NewExpirationCheckFilter() auth.Filter {
	return &expirationCheckFilter{}
}

type expirationCheckFilter struct {
	next peer.EndorserServer
}

// Init initializes the Filter with the next EndorserServer
func (f *expirationCheckFilter) Init(next peer.EndorserServer) {
	f.next = next
}

func validateProposal(signedProp *peer.SignedProposal) error {
	prop, err := utils.GetProposal(signedProp.ProposalBytes)
	if err != nil {
		return errors.Wrap(err, "failed parsing proposal")
	}

	hdr, err := utils.GetHeader(prop.Header)
	if err != nil {
		return errors.Wrap(err, "failed parsing header")
	}

	sh, err := utils.GetSignatureHeader(hdr.SignatureHeader)
	if err != nil {
		return errors.Wrap(err, "failed parsing signature header")
	}
	expirationTime := crypto.ExpiresAt(sh.Creator)
	if !expirationTime.IsZero() && time.Now().After(expirationTime) {
		return errors.New("identity expired")
	}
	return nil
}

// ProcessProposal processes a signed proposal
func (f *expirationCheckFilter) ProcessProposal(ctx context.Context, signedProp *peer.SignedProposal) (*peer.ProposalResponse, error) {
	if err := validateProposal(signedProp); err != nil {
		return nil, err
	}
	return f.next.ProcessProposal(ctx, signedProp)
}
