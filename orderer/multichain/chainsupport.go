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
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/orderer/common/blockcutter"
	"github.com/hyperledger/fabric/orderer/common/broadcast"
	"github.com/hyperledger/fabric/orderer/common/filter"
	"github.com/hyperledger/fabric/orderer/common/sharedconfig"
	"github.com/hyperledger/fabric/orderer/common/sigfilter"
	"github.com/hyperledger/fabric/orderer/common/sizefilter"
	ordererledger "github.com/hyperledger/fabric/orderer/ledger"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
)

// Consenter defines the backing ordering mechanism
type Consenter interface {
	// HandleChain should create a return a reference to a Chain for the given set of resources
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
	// Enqueue accepts a message and returns true on acceptance, or false on shutdown
	Enqueue(env *cb.Envelope) bool

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
	SharedConfig() sharedconfig.Manager
	CreateNextBlock(messages []*cb.Envelope) *cb.Block
	WriteBlock(block *cb.Block, committers []filter.Committer, encodedMetadataValue []byte) *cb.Block
	ChainID() string // ChainID returns the chain ID this specific consenter instance is associated with
}

// ChainSupport provides a wrapper for the resources backing a chain
type ChainSupport interface {
	// This interface is actually the union with the deliver.Support but because of a golang
	// limitation https://github.com/golang/go/issues/6977 the methods must be explicitly declared

	// PolicyManager returns the current policy manager as specified by the chain configuration
	PolicyManager() policies.Manager

	// Reader returns the chain Reader for the chain
	Reader() ordererledger.Reader

	broadcast.Support
	ConsenterSupport
}

type chainSupport struct {
	*ledgerResources
	chain             Chain
	cutter            blockcutter.Receiver
	filters           *filter.RuleSet
	signer            crypto.LocalSigner
	lastConfiguration uint64
	lastConfigSeq     uint64
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

	var err error

	lastBlock := ordererledger.GetBlock(cs.Reader(), cs.Reader().Height()-1)
	metadata, err := utils.GetMetadataFromBlock(lastBlock, cb.BlockMetadataIndex_ORDERER)
	// Assuming a block created with cb.NewBlock(), this should not
	// error even if the orderer metadata is an empty byte slice
	if err != nil {
		logger.Fatalf("Error extracting orderer metadata for chain %x: %s", cs.ChainID(), err)
	}
	logger.Debugf("Retrieved metadata for tip of chain (block #%d): %+v", cs.Reader().Height()-1, metadata)

	cs.chain, err = consenter.HandleChain(cs, metadata)
	if err != nil {
		logger.Fatalf("Error creating consenter for chain %x: %s", ledgerResources.ChainID(), err)
	}

	return cs
}

// createStandardFilters creates the set of filters for a normal (non-system) chain
func createStandardFilters(ledgerResources *ledgerResources) *filter.RuleSet {
	return filter.NewRuleSet([]filter.Rule{
		filter.EmptyRejectRule,
		sizefilter.MaxBytesRule(ledgerResources.SharedConfig().BatchSize().AbsoluteMaxBytes),
		sigfilter.New(ledgerResources.SharedConfig().IngressPolicyNames, ledgerResources.PolicyManager()),
		configtx.NewFilter(ledgerResources),
		filter.AcceptRule,
	})

}

// createSystemChainFilters creates the set of filters for the ordering system chain
func createSystemChainFilters(ml *multiLedger, ledgerResources *ledgerResources) *filter.RuleSet {
	return filter.NewRuleSet([]filter.Rule{
		filter.EmptyRejectRule,
		sizefilter.MaxBytesRule(ledgerResources.SharedConfig().BatchSize().AbsoluteMaxBytes),
		sigfilter.New(ledgerResources.SharedConfig().IngressPolicyNames, ledgerResources.PolicyManager()),
		newSystemChainFilter(ml),
		configtx.NewFilter(ledgerResources),
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

func (cs *chainSupport) Reader() ordererledger.Reader {
	return cs.ledger
}

func (cs *chainSupport) Enqueue(env *cb.Envelope) bool {
	return cs.chain.Enqueue(env)
}

func (cs *chainSupport) CreateNextBlock(messages []*cb.Envelope) *cb.Block {
	return ordererledger.CreateNextBlock(cs.ledger, messages)
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
		cs.lastConfiguration = block.Header.Number
		cs.lastConfigSeq = configSeq
	}

	lastConfigSignature := &cb.MetadataSignature{
		SignatureHeader: utils.MarshalOrPanic(utils.NewSignatureHeaderOrPanic(cs.signer)),
	}

	lastConfigValue := utils.MarshalOrPanic(&cb.LastConfiguration{Index: cs.lastConfiguration})

	lastConfigSignature.Signature = utils.SignOrPanic(cs.signer, util.ConcatenateBytes(lastConfigValue, lastConfigSignature.SignatureHeader, block.Header.Bytes()))

	block.Metadata.Metadata[cb.BlockMetadataIndex_LAST_CONFIGURATION] = utils.MarshalOrPanic(&cb.Metadata{
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
		logger.Panicf("Could not append block: %s", err)
	}
	return block
}
