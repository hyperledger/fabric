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
	Marshaler     Marshaler
	PolicyChecker PolicyChecker
	TMSManager    TMSManager
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

	err = s.PolicyChecker.Check(sc, command)
	if err != nil {
		return s.MarshalErrorResponse(sc.Command, err)
	}

	var payload interface{}
	switch t := command.GetPayload().(type) {
	case *token.Command_ImportRequest:
		payload, err = s.RequestImport(ctx, command.Header, t.ImportRequest)
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
