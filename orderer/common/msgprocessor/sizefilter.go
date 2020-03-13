/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msgprocessor

import (
	"fmt"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/channelconfig"
)

// SizeFilterResources defines the subset of the channel resources required to create this filter
type SizeFilterResources interface {
	// OrdererConfig returns the config.Orderer for the channel and whether the Orderer config exists
	OrdererConfig() (channelconfig.Orderer, bool)
}

// NewSizeFilter creates a size filter which rejects messages larger than maxBytes
func NewSizeFilter(resources SizeFilterResources) *MaxBytesRule {
	return &MaxBytesRule{resources: resources}
}

// MaxBytesRule implements the Rule interface.
type MaxBytesRule struct {
	resources SizeFilterResources
}

// Apply returns an error if the message exceeds the configured absolute max batch size.
func (r *MaxBytesRule) Apply(message *common.Envelope) error {
	ordererConf, ok := r.resources.OrdererConfig()
	if !ok {
		logger.Panic("Programming error: orderer config not found")
	}

	maxBytes := ordererConf.BatchSize().AbsoluteMaxBytes
	if size := messageByteSize(message); size > maxBytes {
		return fmt.Errorf("message payload is %d bytes and exceeds maximum allowed %d bytes", size, maxBytes)
	}
	return nil
}

func messageByteSize(message *common.Envelope) uint32 {
	// XXX this is good approximation, but is going to be a few bytes short, because of the field specifiers in the proto marshaling
	// this should probably be padded to determine the true exact marshaled size
	return uint32(len(message.Payload) + len(message.Signature))
}
