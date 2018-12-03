/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"context"

	"github.com/hyperledger/fabric/protos/token"
	"github.com/pkg/errors"
)

//go:generate counterfeiter -o mock/access_control.go -fake-name PolicyChecker . PolicyChecker

// A PolicyChecker is responsible for performing policy based access control
// checks related to token commands.
type PolicyChecker interface {
	Check(sc *token.SignedCommand, c *token.Command) error
}

//go:generate counterfeiter -o mock/marshaler.go -fake-name Marshaler . Marshaler

// A Marshaler is responsible for marshaling and signging command responses.
type Marshaler interface {
	MarshalCommandResponse(command []byte, responsePayload interface{}) (*token.SignedCommandResponse, error)
}

// A Provider is responslble for processing token commands.
type Prover struct {
	CapabilityChecker CapabilityChecker
	Marshaler         Marshaler
	PolicyChecker     PolicyChecker
	TMSManager        TMSManager
}

// NewProver creates a Prover
func NewProver(policyChecker PolicyChecker, signingIdentity SignerIdentity) (*Prover, error) {
	responseMarshaler, err := NewResponseMarshaler(signingIdentity)
	if err != nil {
		return nil, err
	}

	return &Prover{
		Marshaler:     responseMarshaler,
		PolicyChecker: policyChecker,
		TMSManager: &Manager{
			LedgerManager: &PeerLedgerManager{},
		},
	}, nil
}

func (s *Prover) ProcessCommand(ctx context.Context, sc *token.SignedCommand) (*token.SignedCommandResponse, error) {
	command, err := UnmarshalCommand(sc.Command)
	if err != nil {
		return s.MarshalErrorResponse(sc.Command, err)
	}

	err = s.ValidateHeader(command.Header)
	if err != nil {
		return s.MarshalErrorResponse(sc.Command, err)
	}

	// check if FabToken capability is enabled
	channelId := command.Header.ChannelId
	enabled, err := s.CapabilityChecker.FabToken(channelId)
	if err != nil {
		return s.MarshalErrorResponse(sc.Command, err)
	}
	if !enabled {
		return s.MarshalErrorResponse(sc.Command, errors.Errorf("FabToken capability not enabled for channel %s", channelId))
	}

	err = s.PolicyChecker.Check(sc, command)
	if err != nil {
		return s.MarshalErrorResponse(sc.Command, err)
	}

	var payload interface{}
	switch t := command.GetPayload().(type) {
	case *token.Command_ImportRequest:
		payload, err = s.RequestImport(ctx, command.Header, t.ImportRequest)
	case *token.Command_TransferRequest:
		payload, err = s.RequestTransfer(ctx, command.Header, t.TransferRequest)
	case *token.Command_RedeemRequest:
		payload, err = s.RequestRedeem(ctx, command.Header, t.RedeemRequest)
	case *token.Command_ListRequest:
		payload, err = s.ListUnspentTokens(ctx, command.Header, t.ListRequest)
	case *token.Command_ApproveRequest:
		payload, err = s.RequestApprove(ctx, command.Header, t.ApproveRequest)
	case *token.Command_TransferFromRequest:
		payload, err = s.RequestTransferFrom(ctx, command.Header, t.TransferFromRequest)
	case *token.Command_ExpectationRequest:
		payload, err = s.RequestExpectation(ctx, command.Header, t.ExpectationRequest)
	default:
		err = errors.Errorf("command type not recognized: %T", t)
	}

	if err != nil {
		payload = &token.CommandResponse_Err{
			Err: &token.Error{Message: err.Error()},
		}
	}

	return s.Marshaler.MarshalCommandResponse(sc.Command, payload)
}

func (s *Prover) RequestImport(ctx context.Context, header *token.Header, requestImport *token.ImportRequest) (*token.CommandResponse_TokenTransaction, error) {
	issuer, err := s.TMSManager.GetIssuer(header.ChannelId, requestImport.Credential, header.Creator)
	if err != nil {
		return nil, err
	}

	tokenTransaction, err := issuer.RequestImport(requestImport.TokensToIssue)
	if err != nil {
		return nil, err
	}

	return &token.CommandResponse_TokenTransaction{TokenTransaction: tokenTransaction}, nil
}

func (s *Prover) RequestTransfer(ctx context.Context, header *token.Header, request *token.TransferRequest) (*token.CommandResponse_TokenTransaction, error) {
	transactor, err := s.TMSManager.GetTransactor(header.ChannelId, request.Credential, header.Creator)
	if err != nil {
		return nil, err
	}
	defer transactor.Done()

	tokenTransaction, err := transactor.RequestTransfer(request)
	if err != nil {
		return nil, err
	}

	return &token.CommandResponse_TokenTransaction{TokenTransaction: tokenTransaction}, nil
}

func (s *Prover) RequestRedeem(ctx context.Context, header *token.Header, request *token.RedeemRequest) (*token.CommandResponse_TokenTransaction, error) {
	transactor, err := s.TMSManager.GetTransactor(header.ChannelId, request.Credential, header.Creator)
	if err != nil {
		return nil, err
	}
	defer transactor.Done()

	tokenTransaction, err := transactor.RequestRedeem(request)
	if err != nil {
		return nil, err
	}

	return &token.CommandResponse_TokenTransaction{TokenTransaction: tokenTransaction}, nil
}

func (s *Prover) ListUnspentTokens(ctxt context.Context, header *token.Header, listRequest *token.ListRequest) (*token.CommandResponse_UnspentTokens, error) {
	transactor, err := s.TMSManager.GetTransactor(header.ChannelId, listRequest.Credential, header.Creator)
	if err != nil {
		return nil, err
	}
	defer transactor.Done()

	tokens, err := transactor.ListTokens()
	if err != nil {
		return nil, err
	}

	return &token.CommandResponse_UnspentTokens{UnspentTokens: tokens}, nil
}

func (s *Prover) RequestApprove(ctx context.Context, header *token.Header, request *token.ApproveRequest) (*token.CommandResponse_TokenTransaction, error) {
	transactor, err := s.TMSManager.GetTransactor(header.ChannelId, request.Credential, header.Creator)
	if err != nil {
		return nil, err
	}
	defer transactor.Done()

	tokenTransaction, err := transactor.RequestApprove(request)
	if err != nil {
		return nil, err
	}

	return &token.CommandResponse_TokenTransaction{TokenTransaction: tokenTransaction}, nil
}

func (s *Prover) RequestTransferFrom(ctx context.Context, header *token.Header, request *token.TransferRequest) (*token.CommandResponse_TokenTransaction, error) {
	transactor, err := s.TMSManager.GetTransactor(header.ChannelId, request.Credential, header.Creator)
	if err != nil {
		return nil, err
	}
	defer transactor.Done()

	tokenTransaction, err := transactor.RequestTransferFrom(request)
	if err != nil {
		return nil, err
	}

	return &token.CommandResponse_TokenTransaction{TokenTransaction: tokenTransaction}, nil
}

// RequestExpectation gets an issuer or transactor and creates a token transaction response
// for import, transfer or redemption.
func (s *Prover) RequestExpectation(ctx context.Context, header *token.Header, request *token.ExpectationRequest) (*token.CommandResponse_TokenTransaction, error) {
	if request.GetExpectation() == nil {
		return nil, errors.New("ExpectationRequest has nil Expectation")
	}
	plainExpectation := request.GetExpectation().GetPlainExpectation()
	if plainExpectation == nil {
		return nil, errors.New("ExpectationRequest has nil PlainExpectation")
	}

	// get either issuer or transactor based on payload type in the request
	var tokenTransaction *token.TokenTransaction
	switch t := plainExpectation.GetPayload().(type) {
	case *token.PlainExpectation_ImportExpectation:
		issuer, err := s.TMSManager.GetIssuer(header.ChannelId, request.Credential, header.Creator)
		if err != nil {
			return nil, err
		}
		tokenTransaction, err = issuer.RequestExpectation(request)
		if err != nil {
			return nil, err
		}
	case *token.PlainExpectation_TransferExpectation:
		transactor, err := s.TMSManager.GetTransactor(header.ChannelId, request.Credential, header.Creator)
		if err != nil {
			return nil, err
		}
		defer transactor.Done()

		tokenTransaction, err = transactor.RequestExpectation(request)
		if err != nil {
			return nil, err
		}
	default:
		return nil, errors.Errorf("expectation payload type not recognized: %T", t)
	}

	return &token.CommandResponse_TokenTransaction{TokenTransaction: tokenTransaction}, nil
}

func (s *Prover) ValidateHeader(header *token.Header) error {
	if header == nil {
		return errors.New("command header is required")
	}

	if header.ChannelId == "" {
		return errors.New("channel ID is required in header")
	}

	if len(header.Nonce) == 0 {
		return errors.New("nonce is required in header")
	}

	if len(header.Creator) == 0 {
		return errors.New("creator is required in header")
	}

	return nil
}

func (s *Prover) MarshalErrorResponse(command []byte, e error) (*token.SignedCommandResponse, error) {
	return s.Marshaler.MarshalCommandResponse(
		command,
		&token.CommandResponse_Err{
			Err: &token.Error{Message: e.Error()},
		})
}
