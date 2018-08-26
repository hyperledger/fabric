/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package etcdraft

import (
	"fmt"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/pkg/errors"
)

//go:generate mockery -dir . -name MessageReceiver -case underscore -output mocks

// MessageReceiver receives messages
type MessageReceiver interface {
	// Step passes the given StepRequest message to the MessageReceiver
	Step(req *orderer.StepRequest, sender uint64) error

	// Submit passes the given SubmitRequest message to the MessageReceiver
	Submit(req *orderer.SubmitRequest, sender uint64) error
}

//go:generate mockery -dir . -name ReceiverGetter -case underscore -output mocks

// ReceiverGetter obtains instances of MessageReceiver given a channel ID
type ReceiverGetter interface {
	// ReceiverByChain returns the MessageReceiver if it exists, or nil if it doesn't
	ReceiverByChain(channelID string) MessageReceiver
}

// Dispatcher dispatches Submit and Step requests to the designated per chain instances
type Dispatcher struct {
	Logger        *flogging.FabricLogger
	ChainSelector ReceiverGetter
}

// OnStep notifies the Dispatcher for a reception of a StepRequest from a given sender on a given channel
func (d *Dispatcher) OnStep(channel string, sender uint64, request *orderer.StepRequest) (*orderer.StepResponse, error) {
	receiver := d.ChainSelector.ReceiverByChain(channel)
	if receiver == nil {
		d.Logger.Warningf("An attempt to send a StepRequest to a non existing channel (%s) was made by %d", channel, sender)
		return nil, errors.Errorf("channel %s doesn't exist", channel)
	}
	return &orderer.StepResponse{}, receiver.Step(request, sender)
}

// OnSubmit notifies the Dispatcher for a reception of a SubmitRequest from a given sender on a given channel
func (d *Dispatcher) OnSubmit(channel string, sender uint64, request *orderer.SubmitRequest) (*orderer.SubmitResponse, error) {
	receiver := d.ChainSelector.ReceiverByChain(channel)
	if receiver == nil {
		d.Logger.Warningf("An attempt to submit a transaction to a non existing channel (%s) was made by %d", channel, sender)
		return &orderer.SubmitResponse{
			Info:   fmt.Sprintf("channel %s doesn't exist", channel),
			Status: common.Status_NOT_FOUND,
		}, nil
	}
	if err := receiver.Submit(request, sender); err != nil {
		d.Logger.Errorf("Failed handling transaction on channel %s from %d: %+v", channel, sender, err)
		return &orderer.SubmitResponse{
			Info:   err.Error(),
			Status: common.Status_INTERNAL_SERVER_ERROR,
		}, nil
	}
	return &orderer.SubmitResponse{
		Status: common.Status_SUCCESS,
	}, nil
}
