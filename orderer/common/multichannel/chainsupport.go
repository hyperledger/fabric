/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

                 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package multichannel

import (
	"fmt"

	"github.com/hyperledger/fabric/common/config"
	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/orderer/common/blockcutter"
	"github.com/hyperledger/fabric/orderer/common/broadcast"
	"github.com/hyperledger/fabric/orderer/common/configtxfilter"
	"github.com/hyperledger/fabric/orderer/common/deliver"
	"github.com/hyperledger/fabric/orderer/common/filter"
	"github.com/hyperledger/fabric/orderer/common/ledger"
	"github.com/hyperledger/fabric/orderer/common/sigfilter"
	"github.com/hyperledger/fabric/orderer/common/sizefilter"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
)

// MsgClassification represents the types of possible messages.
type MsgClassification int

const (
	// NormalMsg is the class of standard (endorser or otherwise non-config) messages.
	// Messages of this type should be processed by ProcessNormalMsg.
	NormalMsg MsgClassification = iota

	// ConfigUpdateMsg is the class of configuration related messages.
	// Messages of this type should be processed by ProcessConfigUpdateMsg.
	ConfigUpdateMsg
)

// Consenter defines the backing ordering mechanism
type Consenter interface {
	// HandleChain should create and return a reference to a Chain for the given set of resources
	// It will only be invoked for a given chain once per process.  In general, errors will be treated
	// as irrecoverable and cause system shutdown.  See the description of Chain for more details
	// The second argument to HandleChain is a pointer to the metadata stored on the `ORDERER` slot of
	// the last block committed to the ledger of this Chain.  For a new chain, this metadata will be
	// nil, as this field is not set on the genesis block
	HandleChain(support ConsenterSupport, metadata *cb.Metadata) (Chain, error)
}

// Chain defines a way to inject messages for ordering
// Note, that in order to allow flexibility in the implementation, it is the responsibility of the implementer
// to take the ordered messages, send them through the blockcutter.Receiver supplied via HandleChain to cut blocks,
// and ultimately write the ledger also supplied via HandleChain.  This flow allows for two primary flows
// 1. Messages are ordered into a stream, the stream is cut into blocks, the blocks are committed (solo, kafka)
// 2. Messages are cut into blocks, the blocks are ordered, then the blocks are committed (sbft)
type Chain interface {
	// Enqueue accepts a message and returns true on acceptance, or false on failure
	Enqueue(env *cb.Envelope) bool

	// Errored returns a channel which will close when an error has occurred
	// This is especially useful for the Deliver client, who must terminate waiting
	// clients when the consenter is not up to date
	Errored() <-chan struct{}

	// Start should allocate whatever resources are needed for staying up to date with the chain
	// Typically, this involves creating a thread which reads from the ordering source, passes those
	// messages to a block cutter, and writes the resulting blocks to the ledger
	Start()

	// Halt frees the resources which were allocated for this Chain
	Halt()
}

// ConsenterSupport provides the resources available to a Consenter implementation
type ConsenterSupport interface {
	crypto.LocalSigner
	MsgProcessor
	BlockCutter() blockcutter.Receiver
	SharedConfig() config.Orderer
	CreateNextBlock(messages []*cb.Envelope) *cb.Block
	WriteBlock(block *cb.Block, encodedMetadataValue []byte) *cb.Block
	WriteConfigBlock(block *cb.Block, encodedMetadataValue []byte) *cb.Block
	ChainID() string // ChainID returns the chain ID this specific consenter instance is associated with
	Height() uint64  // Returns the number of blocks on the chain this specific consenter instance is associated with
}

// MsgProcessor defines the methods necessary to interact with Broadcast messages.
type MsgProcessor interface {
	// ClassifyMsg inspects the message to determine which type of processing is necessary.
	ClassifyMsg(env *cb.Envelope) (MsgClassification, error)

	// ProcessNormalMsg will check the validity of a message based on the current configuration.  It returns the current
	// configuration sequence number and nil on success, or an error if the message is not valid
	ProcessNormalMsg(env *cb.Envelope) (configSeq uint64, err error)

	// ProcessConfigUpdateMsg will attempt to apply the config impetus msg to the current configuration, and if successful
	// return the resulting config message and the configSeq the config was computed from.  If the config impetus message
	// is invalid, an error is returned.
	ProcessConfigUpdateMsg(env *cb.Envelope) (config *cb.Envelope, configSeq uint64, err error)
}

// ChainSupport provides a wrapper for the resources backing a chain
type ChainSupport interface {
	broadcast.Support
	deliver.Support
	ConsenterSupport

	// ProposeConfigUpdate applies a CONFIG_UPDATE to an existing config to produce a *cb.ConfigEnvelope
	ProposeConfigUpdate(env *cb.Envelope) (*cb.ConfigEnvelope, error)
}

type chainSupport struct {
	*ledgerResources
	chain         Chain
	cutter        blockcutter.Receiver
	filters       *filter.RuleSet
	signer        crypto.LocalSigner
	lastConfig    uint64
	lastConfigSeq uint64
}

func newChainSupport(
	filters *filter.RuleSet,
	ledgerResources *ledgerResources,
	consenters map[string]Consenter,
	signer crypto.LocalSigner,
) *chainSupport {

	cutter := blockcutter.NewReceiverImpl(ledgerResources.SharedConfig(), filters)
	consenterType := ledgerResources.SharedConfig().ConsensusType()
	consenter, ok := consenters[consenterType]
	if !ok {
		logger.Fatalf("Error retrieving consenter of type: %s", consenterType)
	}

	cs := &chainSupport{
		ledgerResources: ledgerResources,
		cutter:          cutter,
		filters:         filters,
		signer:          signer,
	}

	cs.lastConfigSeq = cs.Sequence()

	var err error

	lastBlock := ledger.GetBlock(cs.Reader(), cs.Reader().Height()-1)

	// If this is the genesis block, the lastconfig field may be empty, and, the last config is necessary 0
	// so no need to initialize lastConfig
	if lastBlock.Header.Number != 0 {
		cs.lastConfig, err = utils.GetLastConfigIndexFromBlock(lastBlock)
		if err != nil {
			logger.Fatalf("[channel: %s] Error extracting last config block from block metadata: %s", cs.ChainID(), err)
		}
	}

	metadata, err := utils.GetMetadataFromBlock(lastBlock, cb.BlockMetadataIndex_ORDERER)
	// Assuming a block created with cb.NewBlock(), this should not
	// error even if the orderer metadata is an empty byte slice
	if err != nil {
		logger.Fatalf("[channel: %s] Error extracting orderer metadata: %s", cs.ChainID(), err)
	}
	logger.Debugf("[channel: %s] Retrieved metadata for tip of chain (blockNumber=%d, lastConfig=%d, lastConfigSeq=%d): %+v", cs.ChainID(), lastBlock.Header.Number, cs.lastConfig, cs.lastConfigSeq, metadata)

	cs.chain, err = consenter.HandleChain(cs, metadata)
	if err != nil {
		logger.Fatalf("[channel: %s] Error creating consenter: %s", cs.ChainID(), err)
	}

	return cs
}

// createStandardFilters creates the set of filters for a normal (non-system) chain
func createStandardFilters(ledgerResources *ledgerResources) *filter.RuleSet {
	return filter.NewRuleSet([]filter.Rule{
		filter.EmptyRejectRule,
		sizefilter.MaxBytesRule(ledgerResources.SharedConfig()),
		sigfilter.New(policies.ChannelWriters, ledgerResources.PolicyManager()),
		configtxfilter.NewFilter(ledgerResources),
		filter.AcceptRule,
	})

}

// createSystemChainFilters creates the set of filters for the ordering system chain
func createSystemChainFilters(ml *multiLedger, ledgerResources *ledgerResources) *filter.RuleSet {
	return filter.NewRuleSet([]filter.Rule{
		filter.EmptyRejectRule,
		sizefilter.MaxBytesRule(ledgerResources.SharedConfig()),
		sigfilter.New(policies.ChannelWriters, ledgerResources.PolicyManager()),
		newSystemChainFilter(ledgerResources, ml),
		configtxfilter.NewFilter(ledgerResources),
		filter.AcceptRule,
	})
}

func (cs *chainSupport) start() {
	cs.chain.Start()
}

func (cs *chainSupport) NewSignatureHeader() (*cb.SignatureHeader, error) {
	return cs.signer.NewSignatureHeader()
}

func (cs *chainSupport) Sign(message []byte) ([]byte, error) {
	return cs.signer.Sign(message)
}

func (cs *chainSupport) Filters() *filter.RuleSet {
	return cs.filters
}

func (cs *chainSupport) BlockCutter() blockcutter.Receiver {
	return cs.cutter
}

// ClassifyMsg inspects the message to determine which type of processing is necessary
func (cs *chainSupport) ClassifyMsg(env *cb.Envelope) (MsgClassification, error) {
	payload, err := utils.UnmarshalPayload(env.Payload)
	if err != nil {
		return 0, fmt.Errorf("bad payload: %s", err)
	}

	if payload.Header == nil {
		return 0, fmt.Errorf("bad payload: missing header")
	}

	chdr, err := utils.UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		return 0, fmt.Errorf("bad channelheader: %s", err)
	}

	switch chdr.Type {
	case int32(cb.HeaderType_CONFIG_UPDATE):
		return ConfigUpdateMsg, nil
	case int32(cb.HeaderType_ORDERER_TRANSACTION):
		return ConfigUpdateMsg, nil
		// XXX Eventually, these types cannot be allowed to be submitted directly
		// return 0, fmt.Errorf("Transactions of type ORDERER_TRANSACTION cannot be Broadcast")
	case int32(cb.HeaderType_CONFIG):
		return ConfigUpdateMsg, nil
		// XXX Eventually, these types cannot be allowed to be submitted directly
		// return 0, fmt.Errorf("Transactions of type CONFIG cannot be Broadcast")
	default:
		return NormalMsg, nil
	}
}

// ProcessNormalMsg will check the validity of a message based on the current configuration.  It returns the current
// configuration sequence number and nil on success, or an error if the message is not valid
func (cs *chainSupport) ProcessNormalMsg(env *cb.Envelope) (configSeq uint64, err error) {
	configSeq = cs.Sequence()
	_, err = cs.filters.Apply(env)
	return
}

// ProcessConfigUpdateMsg will attempt to apply the config update msg to the current configuration, and if successful
// return the resulting config message and the configSeq the config was computed from.  If the config update message
// is invalid, an error is returned.
func (cs *chainSupport) ProcessConfigUpdateMsg(env *cb.Envelope) (config *cb.Envelope, configSeq uint64, err error) {
	return nil, cs.Sequence(), fmt.Errorf("Config update message not yet implemented")
}

func (cs *chainSupport) Reader() ledger.Reader {
	return cs.ledger
}

func (cs *chainSupport) Enqueue(env *cb.Envelope) bool {
	return cs.chain.Enqueue(env)
}

func (cs *chainSupport) Errored() <-chan struct{} {
	return cs.chain.Errored()
}

func (cs *chainSupport) CreateNextBlock(messages []*cb.Envelope) *cb.Block {
	return ledger.CreateNextBlock(cs.ledger, messages)
}

func (cs *chainSupport) addBlockSignature(block *cb.Block) {
	logger.Debugf("%+v", cs)
	logger.Debugf("%+v", cs.signer)

	blockSignature := &cb.MetadataSignature{
		SignatureHeader: utils.MarshalOrPanic(utils.NewSignatureHeaderOrPanic(cs.signer)),
	}

	// Note, this value is intentionally nil, as this metadata is only about the signature, there is no additional metadata
	// information required beyond the fact that the metadata item is signed.
	blockSignatureValue := []byte(nil)

	blockSignature.Signature = utils.SignOrPanic(cs.signer, util.ConcatenateBytes(blockSignatureValue, blockSignature.SignatureHeader, block.Header.Bytes()))

	block.Metadata.Metadata[cb.BlockMetadataIndex_SIGNATURES] = utils.MarshalOrPanic(&cb.Metadata{
		Value: blockSignatureValue,
		Signatures: []*cb.MetadataSignature{
			blockSignature,
		},
	})
}

func (cs *chainSupport) addLastConfigSignature(block *cb.Block) {
	configSeq := cs.Sequence()
	if configSeq > cs.lastConfigSeq {
		logger.Debugf("[channel: %s] Detected lastConfigSeq transitioning from %d to %d, setting lastConfig from %d to %d", cs.ChainID(), cs.lastConfigSeq, configSeq, cs.lastConfig, block.Header.Number)
		cs.lastConfig = block.Header.Number
		cs.lastConfigSeq = configSeq
	}

	lastConfigSignature := &cb.MetadataSignature{
		SignatureHeader: utils.MarshalOrPanic(utils.NewSignatureHeaderOrPanic(cs.signer)),
	}

	lastConfigValue := utils.MarshalOrPanic(&cb.LastConfig{Index: cs.lastConfig})
	logger.Debugf("[channel: %s] About to write block, setting its LAST_CONFIG to %d", cs.ChainID(), cs.lastConfig)

	lastConfigSignature.Signature = utils.SignOrPanic(cs.signer, util.ConcatenateBytes(lastConfigValue, lastConfigSignature.SignatureHeader, block.Header.Bytes()))

	block.Metadata.Metadata[cb.BlockMetadataIndex_LAST_CONFIG] = utils.MarshalOrPanic(&cb.Metadata{
		Value: lastConfigValue,
		Signatures: []*cb.MetadataSignature{
			lastConfigSignature,
		},
	})
}

func (cs *chainSupport) WriteConfigBlock(block *cb.Block, encodedMetadataValue []byte) *cb.Block {
	// XXX This hacky path is temporary and will be removed by the end of this change series
	// The panics here are just fine
	committer, err := cs.filters.Apply(utils.UnmarshalEnvelopeOrPanic(block.Data.Data[0]))
	if err != nil {
		logger.Panicf("Config should have already been validated")
	}
	committer.Commit()

	return cs.WriteBlock(block, encodedMetadataValue)
}

func (cs *chainSupport) WriteBlock(block *cb.Block, encodedMetadataValue []byte) *cb.Block {
	// Set the orderer-related metadata field
	if encodedMetadataValue != nil {
		block.Metadata.Metadata[cb.BlockMetadataIndex_ORDERER] = utils.MarshalOrPanic(&cb.Metadata{Value: encodedMetadataValue})
	}
	cs.addBlockSignature(block)
	cs.addLastConfigSignature(block)

	err := cs.ledger.Append(block)
	if err != nil {
		logger.Panicf("[channel: %s] Could not append block: %s", cs.ChainID(), err)
	}
	logger.Debugf("[channel: %s] Wrote block %d", cs.ChainID(), block.GetHeader().Number)

	return block
}

func (cs *chainSupport) Height() uint64 {
	return cs.Reader().Height()
}
