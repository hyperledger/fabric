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

package multichain

import (
	"github.com/hyperledger/fabric/common/config"
	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/orderer/common/blockcutter"
	"github.com/hyperledger/fabric/orderer/common/broadcast"
	"github.com/hyperledger/fabric/orderer/common/configtxfilter"
	"github.com/hyperledger/fabric/orderer/common/filter"
	"github.com/hyperledger/fabric/orderer/common/sigfilter"
	"github.com/hyperledger/fabric/orderer/common/sizefilter"
	"github.com/hyperledger/fabric/orderer/ledger"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
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
	BlockCutter() blockcutter.Receiver
	SharedConfig() config.Orderer
	CreateNextBlock(messages []*cb.Envelope) *cb.Block
	WriteBlock(block *cb.Block, committers []filter.Committer, encodedMetadataValue []byte) *cb.Block
	ChainID() string // ChainID returns the chain ID this specific consenter instance is associated with
	Height() uint64  // Returns the number of blocks on the chain this specific consenter instance is associated with
}

// ChainSupport provides a wrapper for the resources backing a chain
type ChainSupport interface {
	// This interface is actually the union with the deliver.Support but because of a golang
	// limitation https://github.com/golang/go/issues/6977 the methods must be explicitly declared

	// PolicyManager returns the current policy manager as specified by the chain config
	PolicyManager() policies.Manager

	// Reader returns the chain Reader for the chain
	Reader() ledger.Reader

	// Errored returns whether the backing consenter has errored
	Errored() <-chan struct{}

	broadcast.Support
	ConsenterSupport

	// Sequence returns the current config sequence number
	Sequence() uint64

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

func (cs *chainSupport) WriteBlock(block *cb.Block, committers []filter.Committer, encodedMetadataValue []byte) *cb.Block {
	for _, committer := range committers {
		committer.Commit()
	}
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
