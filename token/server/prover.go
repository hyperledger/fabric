/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"context"
	"runtime/debug"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/protos/token"
	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("token.server")

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

func (s *Prover) ProcessCommand(ctx context.Context, sc *token.SignedCommand) (cr *token.SignedCommandResponse, err error) {
	defer func() {
		if r := recover(); r != nil {
			logger.Criticalf("ProcessCommand triggered panic: %s\n%s", r, debug.Stack())
			err = errors.Errorf("ProcessCommand triggered panic: %s", r)
		}
	}()

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
	case *token.Command_IssueRequest:
		payload, err = s.RequestIssue(ctx, command.Header, t.IssueRequest)
	case *token.Command_TransferRequest:
		payload, err = s.RequestTransfer(ctx, command.Header, t.TransferRequest)
	case *token.Command_RedeemRequest:
		payload, err = s.RequestRedeem(ctx, command.Header, t.RedeemRequest)
	case *token.Command_ListRequest:
		payload, err = s.ListUnspentTokens(ctx, command.Header, t.ListRequest)
	case *token.Command_TokenOperationRequest:
		payload, err = s.RequestTokenOperations(ctx, command.Header, t.TokenOperationRequest)
	default:
		err = errors.Errorf("command type not recognized: %T", t)
	}

	if err != nil {
		payload = &token.CommandResponse_Err{
			Err: &token.Error{Message: err.Error()},
		}
	}

	cr, err = s.Marshaler.MarshalCommandResponse(sc.Command, payload)

	return
}

func (s *Prover) RequestIssue(ctx context.Context, header *token.Header, requestImport *token.IssueRequest) (*token.CommandResponse_TokenTransaction, error) {
	issuer, err := s.TMSManager.GetIssuer(header.ChannelId, requestImport.Credential, header.Creator)
	if err != nil {
		return nil, err
	}

	tokenTransaction, err := issuer.RequestIssue(requestImport.TokensToIssue)
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

// RequestTokenOperation gets an issuer or transactor and creates a token transaction response
// for import, transfer or redemption.
func (s *Prover) RequestTokenOperations(ctx context.Context, header *token.Header, request *token.TokenOperationRequest) (*token.CommandResponse_TokenTransactions, error) {
	ops := request.GetOperations()
	if len(ops) == 0 {
		return nil, errors.New("no token operations requested")
	}

	tokenIds := request.TokenIds
	// get an issuer and a transactor based on payload type in the request
	var txts []*token.TokenTransaction
	for _, op := range ops {
		var tokenTransaction *token.TokenTransaction
		switch t := op.GetAction().GetPayload().(type) {
		case *token.TokenOperationAction_Issue:
			issuer, err := s.TMSManager.GetIssuer(header.ChannelId, request.Credential, header.Creator)
			if err != nil {
				return nil, err
			}
			tokenTransaction, err = issuer.RequestTokenOperation(op)
			if err != nil {
				return nil, err
			}
		case *token.TokenOperationAction_Transfer:
			var count int
			transactor, err := s.TMSManager.GetTransactor(header.ChannelId, request.Credential, header.Creator)
			if err != nil {
				return nil, err
			}
			defer transactor.Done()
			tokenTransaction, count, err = transactor.RequestTokenOperation(tokenIds, op)
			if err != nil {
				return nil, err
			}
			tokenIds = tokenIds[count:]
		default:
			return nil, errors.Errorf("operation payload type not recognized: %T", t)
		}
		txts = append(txts, tokenTransaction)
	}

	return &token.CommandResponse_TokenTransactions{TokenTransactions: &token.TokenTransactions{Txs: txts}}, nil
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
