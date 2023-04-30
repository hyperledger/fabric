/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package consensus

import (
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/internal/pkg/identity"
	"github.com/hyperledger/fabric/orderer/common/blockcutter"
	"github.com/hyperledger/fabric/orderer/common/msgprocessor"
	"github.com/hyperledger/fabric/protoutil"
)

// Consenter defines the backing ordering mechanism.
type Consenter interface {
	// HandleChain should create and return a reference to a Chain for the given set of resources.
	// It will only be invoked for a given chain once per process.  In general, errors will be treated
	// as irrecoverable and cause system shutdown.  See the description of Chain for more details
	// The second argument to HandleChain is a pointer to the metadata stored on the `ORDERER` slot of
	// the last block committed to the ledger of this Chain. For a new chain, or one which is migrated,
	// this metadata will be nil (or contain a zero-length Value), as there is no prior metadata to report.
	HandleChain(support ConsenterSupport, metadata *cb.Metadata) (Chain, error)
}

// ClusterConsenter defines methods implemented by cluster-type consenters.
type ClusterConsenter interface {
	// IsChannelMember inspects the join block and detects whether it implies that this orderer is a member of the
	// channel. It returns true if the orderer is a member of the consenters set, and false if it is not. The method
	// also inspects the consensus type metadata for validity. It returns an error if membership cannot be determined
	// due to errors processing the block.
	IsChannelMember(joinBlock *cb.Block) (bool, error)
}

// MetadataValidator performs the validation of updates to ConsensusMetadata during config updates to the channel.
// NOTE: We expect the MetadataValidator interface to be optionally implemented by the Consenter implementation.
//
//	If a Consenter does not implement MetadataValidator, we default to using a no-op MetadataValidator.
type MetadataValidator interface {
	// ValidateConsensusMetadata determines the validity of a ConsensusMetadata update during config
	// updates on the channel.
	// Since the ConsensusMetadata is specific to the consensus implementation (independent of the particular
	// chain) this validation also needs to be implemented by the specific consensus implementation.
	ValidateConsensusMetadata(oldOrdererConfig, newOrdererConfig channelconfig.Orderer, newChannel bool) error
}

// Chain defines a way to inject messages for ordering.
// Note, that in order to allow flexibility in the implementation, it is the responsibility of the implementer
// to take the ordered messages, send them through the blockcutter.Receiver supplied via HandleChain to cut blocks,
// and ultimately write the ledger also supplied via HandleChain.  This design allows for two primary flows
//  1. Messages are ordered into a stream, the stream is cut into blocks, the blocks are committed (deprecated, orderer
//     no longer supports solo & kafka)
//  2. Messages are cut into blocks, the blocks are ordered, then the blocks are committed (etcdraft)
type Chain interface {
	// Order accepts a message which has been processed at a given configSeq.
	// If the configSeq advances, it is the responsibility of the consenter
	// to revalidate and potentially discard the message
	// The consenter may return an error, indicating the message was not accepted
	Order(env *cb.Envelope, configSeq uint64) error

	// Configure accepts a message which reconfigures the channel and will
	// trigger an update to the configSeq if committed.  The configuration must have
	// been triggered by a ConfigUpdate message. If the config sequence advances,
	// it is the responsibility of the consenter to recompute the resulting config,
	// discarding the message if the reconfiguration is no longer valid.
	// The consenter may return an error, indicating the message was not accepted
	Configure(config *cb.Envelope, configSeq uint64) error

	// WaitReady blocks waiting for consenter to be ready for accepting new messages.
	// This is useful when consenter needs to temporarily block ingress messages so
	// that in-flight messages can be consumed. It could return error if consenter is
	// in erroneous states. If this blocking behavior is not desired, consenter could
	// simply return nil.
	WaitReady() error

	// Errored returns a channel which will close when an error has occurred.
	// This is especially useful for the Deliver client, who must terminate waiting
	// clients when the consenter is not up to date.
	Errored() <-chan struct{}

	// Start should allocate whatever resources are needed for staying up to date with the chain.
	// Typically, this involves creating a thread which reads from the ordering source, passes those
	// messages to a block cutter, and writes the resulting blocks to the ledger.
	Start()

	// Halt frees the resources which were allocated for this Chain.
	Halt()
}

//go:generate counterfeiter -o mocks/mock_consenter_support.go . ConsenterSupport

// ConsenterSupport provides the resources available to a Consenter implementation.
type ConsenterSupport interface {
	identity.SignerSerializer
	msgprocessor.Processor

	// SignatureVerifier verifies a signature of a block.
	SignatureVerifier() protoutil.BlockVerifierFunc

	// BlockCutter returns the block cutting helper for this channel.
	BlockCutter() blockcutter.Receiver

	// SharedConfig provides the shared config from the channel's current config block.
	SharedConfig() channelconfig.Orderer

	// ChannelConfig provides the channel config from the channel's current config block.
	ChannelConfig() channelconfig.Channel

	// CreateNextBlock takes a list of messages and creates the next block based on the block with highest block number committed to the ledger
	// Note that either WriteBlock or WriteConfigBlock must be called before invoking this method a second time.
	CreateNextBlock(messages []*cb.Envelope) *cb.Block

	// Block returns a block with the given number,
	// or nil if such a block doesn't exist.
	Block(number uint64) *cb.Block

	// WriteBlock commits a block to the ledger.
	WriteBlock(block *cb.Block, encodedMetadataValue []byte)

	// WriteConfigBlock commits a block to the ledger, and applies the config update inside.
	WriteConfigBlock(block *cb.Block, encodedMetadataValue []byte)

	// Sequence returns the current config sequence.
	Sequence() uint64

	// ChannelID returns the channel ID this support is associated with.
	ChannelID() string

	// Height returns the number of blocks in the chain this channel is associated with.
	Height() uint64

	// Append appends a new block to the ledger in its raw form,
	// unlike WriteBlock that also mutates its metadata.
	Append(block *cb.Block) error
}

// NoOpMetadataValidator implements a MetadataValidator that always returns nil error irrespecttive of the inputs.
type NoOpMetadataValidator struct{}

// ValidateConsensusMetadata determines the validity of a ConsensusMetadata update during config updates
// on the channel, and it always returns nil error for the NoOpMetadataValidator implementation.
func (n NoOpMetadataValidator) ValidateConsensusMetadata(oldChannelConfig, newChannelConfig channelconfig.Orderer, newChannel bool) error {
	return nil
}
