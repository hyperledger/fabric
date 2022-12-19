/*
Copyright IBM Corp. 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/pkg/errors"
)

type BroadcastClient interface {
	// Send data to orderer
	Send(env *cb.Envelope) error
	Close() error
}

type BroadcastGRPCClient struct {
	Client BroadcastOrdererClient
}

// GetBroadcastClient creates a simple instance of the BroadcastClient interface
func GetBroadcastClient() (BroadcastClient, error) {
	oc, err := NewOrdererClientFromEnv()
	if err != nil {
		return nil, err
	}
	bc, err := oc.Broadcast()
	if err != nil {
		return nil, err
	}

	return &BroadcastGRPCClient{Client: bc}, nil
}

func (s *BroadcastGRPCClient) getAck() error {
	msg, err := s.Client.Recv()
	if err != nil {
		return err
	}
	if msg.Status != cb.Status_SUCCESS {
		return errors.Errorf("got unexpected status: %v -- %s", msg.Status, msg.Info)
	}
	return nil
}

// Send data to orderer
func (s *BroadcastGRPCClient) Send(env *cb.Envelope) error {
	if err := s.Client.Send(env); err != nil {
		return errors.WithMessage(err, "could not send to orderer node")
	}

	err := s.getAck()

	return err
}

func (s *BroadcastGRPCClient) Close() error {
	return s.Client.CloseSend()
}
