/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package smartbft

import (
	protos "github.com/SmartBFT-Go/consensus/smartbftprotos"
	"github.com/golang/protobuf/proto"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

//go:generate mockery -dir . -name MessageReceiver -case underscore -output mocks

// MessageReceiver receives messages
type MessageReceiver interface {
	HandleMessage(sender uint64, m *protos.Message)
	HandleRequest(sender uint64, req []byte)
}

//go:generate mockery -dir . -name ReceiverGetter -case underscore -output mocks

// ReceiverGetter obtains instances of MessageReceiver given a channel ID
type ReceiverGetter interface {
	// ReceiverByChain returns the MessageReceiver if it exists, or nil if it doesn't
	ReceiverByChain(channelID string) MessageReceiver
}

type WarningLogger interface {
	Warningf(template string, args ...interface{})
}

// Ingress dispatches Submit and Step requests to the designated per chain instances
type Ingress struct {
	Logger        WarningLogger
	ChainSelector ReceiverGetter
}

// OnConsensus notifies the Ingress for a reception of a StepRequest from a given sender on a given channel
func (in *Ingress) OnConsensus(channel string, sender uint64, request *ab.ConsensusRequest) error {
	receiver := in.ChainSelector.ReceiverByChain(channel)
	if receiver == nil {
		in.Logger.Warningf("An attempt to send a consensus request to a non existing channel (%s) was made by %d", channel, sender)
		return errors.Errorf("channel %s doesn't exist", channel)
	}
	msg := &protos.Message{}
	if err := proto.Unmarshal(request.Payload, msg); err != nil {
		in.Logger.Warningf("Malformed message: %v", err)
		return errors.Wrap(err, "malformed message")
	}
	receiver.HandleMessage(sender, msg)
	return nil
}

// OnSubmit notifies the Ingress for a reception of a SubmitRequest from a given sender on a given channel
func (in *Ingress) OnSubmit(channel string, sender uint64, request *ab.SubmitRequest) error {
	receiver := in.ChainSelector.ReceiverByChain(channel)
	if receiver == nil {
		in.Logger.Warningf("An attempt to submit a transaction to a non existing channel (%s) was made by %d", channel, sender)
		return errors.Errorf("channel %s doesn't exist", channel)
	}
	receiver.HandleRequest(sender, protoutil.MarshalOrPanic(request.Payload))
	return nil
}
