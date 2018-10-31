/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/protos/token"
	"github.com/pkg/errors"
)

// UnmarshalCommand unmarshal token.Command messages
func UnmarshalCommand(raw []byte) (*token.Command, error) {
	command := &token.Command{}
	err := proto.Unmarshal(raw, command)
	if err != nil {
		return nil, err
	}

	return command, nil
}

type TimeFunc func() time.Time

// ResponseMarshaler produces token.SignedCommandResponse
type ResponseMarshaler struct {
	Signer  Signer
	Creator []byte
	Time    TimeFunc
}

func NewResponseMarshaler(signerID SignerIdentity) (*ResponseMarshaler, error) {
	creator, err := signerID.Serialize()
	if err != nil {
		return nil, err
	}

	return &ResponseMarshaler{
		Signer:  signerID,
		Creator: creator,
		Time:    time.Now,
	}, nil
}

func (s *ResponseMarshaler) MarshalCommandResponse(command []byte, responsePayload interface{}) (*token.SignedCommandResponse, error) {
	cr, err := commandResponseFromPayload(responsePayload)
	if err != nil {
		return nil, err
	}

	ts, err := ptypes.TimestampProto(s.Time())
	if err != nil {
		return nil, err
	}

	cr.Header = &token.CommandResponseHeader{
		Creator:     s.Creator,
		CommandHash: util.ComputeSHA256(command),
		Timestamp:   ts,
	}

	return s.createSignedCommandResponse(cr)
}

func (s *ResponseMarshaler) createSignedCommandResponse(cr *token.CommandResponse) (*token.SignedCommandResponse, error) {
	raw, err := proto.Marshal(cr)
	if err != nil {
		return nil, err
	}

	signature, err := s.Signer.Sign(raw)
	if err != nil {
		return nil, err
	}

	return &token.SignedCommandResponse{
		Response:  raw,
		Signature: signature,
	}, nil
}

func commandResponseFromPayload(payload interface{}) (*token.CommandResponse, error) {
	switch t := payload.(type) {
	case *token.CommandResponse_TokenTransaction:
		return &token.CommandResponse{Payload: t}, nil
	case *token.CommandResponse_Err:
		return &token.CommandResponse{Payload: t}, nil
	case *token.CommandResponse_UnspentTokens:
		return &token.CommandResponse{Payload: t}, nil
	default:
		return nil, errors.Errorf("command type not recognized: %T", t)
	}
}
