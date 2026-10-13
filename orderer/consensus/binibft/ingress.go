/*
Copyright BiniWorld. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package binibft

import (
	ab "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

// MessageReceiver is implemented by a Chain: it receives the payloads that the
// cluster service authenticated as coming from consenter `sender`.
type MessageReceiver interface {
	HandleMessage(sender uint64, payload []byte)
	HandleRequest(sender uint64, req []byte)
}

// ReceiverGetter looks up the receiver of a channel.
type ReceiverGetter interface {
	ReceiverByChain(channelID string) MessageReceiver
}

// WarningLogger logs warnings.
type WarningLogger interface {
	Warningf(template string, args ...any)
}

// Ingress dispatches cluster Consensus and Submit requests to the chain of the
// channel they target. It implements cluster.RequestHandler.
type Ingress struct {
	Logger        WarningLogger
	ChainSelector ReceiverGetter
}

// OnConsensus handles a ConsensusRequest from consenter `sender`.
func (in *Ingress) OnConsensus(channel string, sender uint64, request *ab.ConsensusRequest) error {
	receiver := in.ChainSelector.ReceiverByChain(channel)
	if receiver == nil {
		in.Logger.Warningf("Consensus request from %d for non-existing channel %s", sender, channel)
		return errors.Errorf("channel %s doesn't exist", channel)
	}
	receiver.HandleMessage(sender, request.Payload)
	return nil
}

// OnSubmit handles a SubmitRequest (a forwarded transaction) from `sender`.
func (in *Ingress) OnSubmit(channel string, sender uint64, request *ab.SubmitRequest) error {
	receiver := in.ChainSelector.ReceiverByChain(channel)
	if receiver == nil {
		in.Logger.Warningf("Submit request from %d for non-existing channel %s", sender, channel)
		return errors.Errorf("channel %s doesn't exist", channel)
	}
	receiver.HandleRequest(sender, protoutil.MarshalOrPanic(request.Payload))
	return nil
}
