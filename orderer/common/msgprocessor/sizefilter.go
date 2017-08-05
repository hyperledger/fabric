/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msgprocessor

import (
	"fmt"

	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
)

// Support defines the subset of the channel support required to create this filter
type Support interface {
	BatchSize() *ab.BatchSize
}

// New creates a size filter which rejects messages larger than maxBytes
func NewSizeFilter(support Support) *MaxBytesRule {
	return &MaxBytesRule{support: support}
}

// MaxBytesRule implements the Rule interface.
type MaxBytesRule struct {
	support Support
}

// Apply returns an error if the message exceeds the configured absolute max batch size.
func (r *MaxBytesRule) Apply(message *cb.Envelope) error {
	maxBytes := r.support.BatchSize().AbsoluteMaxBytes
	if size := messageByteSize(message); size > maxBytes {
		return fmt.Errorf("message payload is %d bytes and exceeds maximum allowed %d bytes", size, maxBytes)
	}
	return nil
}

func messageByteSize(message *cb.Envelope) uint32 {
	// XXX this is good approximation, but is going to be a few bytes short, because of the field specifiers in the proto marshaling
	// this should probably be padded to determine the true exact marshaled size
	return uint32(len(message.Payload) + len(message.Signature))
}
