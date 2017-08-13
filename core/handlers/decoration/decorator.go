/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package decoration

import "github.com/hyperledger/fabric/protos/peer"

// Decorator decorates a chaincode input
type Decorator interface {
	// Decorate decorates a chaincode input by changing it
	Decorate(proposal *peer.Proposal, input *peer.ChaincodeInput) *peer.ChaincodeInput
}

// NewDecorator creates a new decorator
func NewDecorator() Decorator {
	return &decorator{}
}

type decorator struct {
}

// Decorate decorates a chaincode input by changing it
func (d *decorator) Decorate(proposal *peer.Proposal, input *peer.ChaincodeInput) *peer.ChaincodeInput {
	return input
}
