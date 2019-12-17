/*
Copyright IBM Corp, SecureKey Technologies Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package decoration

import (
	"github.com/hyperledger/fabric-protos-go/peer"
)

// Decorator decorates a chaincode input
type Decorator interface {
	// Decorate decorates a chaincode input by changing it
	Decorate(proposal *peer.Proposal, input *peer.ChaincodeInput) *peer.ChaincodeInput
}

// Apply decorators in the order provided
func Apply(proposal *peer.Proposal, input *peer.ChaincodeInput,
	decorators ...Decorator) *peer.ChaincodeInput {
	for _, decorator := range decorators {
		input = decorator.Decorate(proposal, input)
	}

	return input
}
