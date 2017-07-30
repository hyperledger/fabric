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

	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/orderer/common/blockcutter"
	"github.com/hyperledger/fabric/orderer/common/configtxfilter"
	"github.com/hyperledger/fabric/orderer/common/filter"
	"github.com/hyperledger/fabric/orderer/common/ledger"
	"github.com/hyperledger/fabric/orderer/common/msgprocessor"
	"github.com/hyperledger/fabric/orderer/common/sigfilter"
	"github.com/hyperledger/fabric/orderer/common/sizefilter"
	"github.com/hyperledger/fabric/orderer/consensus"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
)

// ChainSupport holds the resources for a particular channel.
type ChainSupport struct {
	*ledgerResources
	chain         consensus.Chain
	cutter        blockcutter.Receiver
	filters       *filter.RuleSet
	signer        crypto.LocalSigner
	lastConfig    uint64
	lastConfigSeq uint64
}

func newChainSupport(
	filters *filter.RuleSet,
	ledgerResources *ledgerResources,
	consenters map[string]consensus.Consenter,
	signer crypto.LocalSigner,
) *ChainSupport {

	cutter := blockcutter.NewReceiverImpl(ledgerResources.SharedConfig())
	consenterType := ledgerResources.SharedConfig().ConsensusType()
	consenter, ok := consenters[consenterType]
	if !ok {
		logger.Fatalf("Error retrieving consenter of type: %s", consenterType)
	}

	cs := &ChainSupport{
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
func createSystemChainFilters(r *Registrar, ledgerResources *ledgerResources) *filter.RuleSet {
	return filter.NewRuleSet([]filter.Rule{
		filter.EmptyRejectRule,
		sizefilter.MaxBytesRule(ledgerResources.SharedConfig()),
		sigfilter.New(policies.ChannelWriters, ledgerResources.PolicyManager()),
		newSystemChainFilter(ledgerResources, r),
		configtxfilter.NewFilter(ledgerResources),
		filter.AcceptRule,
	})
}

func (cs *ChainSupport) start() {
	cs.chain.Start()
}

// NewSignatureHeader passes through to the signer NewSignatureHeader method.
func (cs *ChainSupport) NewSignatureHeader() (*cb.SignatureHeader, error) {
	return cs.signer.NewSignatureHeader()
}

// Sign passes through to the signer Sign method.
func (cs *ChainSupport) Sign(message []byte) ([]byte, error) {
	return cs.signer.Sign(message)
}

// Filters returns the set of filters created for this channel.
func (cs *ChainSupport) Filters() *filter.RuleSet {
	return cs.filters
}

// BlockCutter returns the blockcutter.Receiver instance for this channel.
func (cs *ChainSupport) BlockCutter() blockcutter.Receiver {
	return cs.cutter
}

// ClassifyMsg inspects the message to determine which type of processing is necessary
func (cs *ChainSupport) ClassifyMsg(env *cb.Envelope) (msgprocessor.Classification, error) {
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
		return msgprocessor.ConfigUpdateMsg, nil
	case int32(cb.HeaderType_ORDERER_TRANSACTION):
		return msgprocessor.ConfigUpdateMsg, nil
		// XXX Eventually, these types cannot be allowed to be submitted directly
		// return 0, fmt.Errorf("Transactions of type ORDERER_TRANSACTION cannot be Broadcast")
	case int32(cb.HeaderType_CONFIG):
		return msgprocessor.ConfigUpdateMsg, nil
		// XXX Eventually, these types cannot be allowed to be submitted directly
		// return 0, fmt.Errorf("Transactions of type CONFIG cannot be Broadcast")
	default:
		return msgprocessor.NormalMsg, nil
	}
}

// ProcessNormalMsg will check the validity of a message based on the current configuration.  It returns the current
// configuration sequence number and nil on success, or an error if the message is not valid
func (cs *ChainSupport) ProcessNormalMsg(env *cb.Envelope) (configSeq uint64, err error) {
	configSeq = cs.Sequence()
	_, err = cs.filters.Apply(env)
	return
}

// ProcessConfigUpdateMsg will attempt to apply the config update msg to the current configuration, and if successful
// return the resulting config message and the configSeq the config was computed from.  If the config update message
// is invalid, an error is returned.
func (cs *ChainSupport) ProcessConfigUpdateMsg(env *cb.Envelope) (config *cb.Envelope, configSeq uint64, err error) {
	return nil, cs.Sequence(), fmt.Errorf("Config update message not yet implemented")
}

// Reader returns a reader for the underlying ledger.
func (cs *ChainSupport) Reader() ledger.Reader {
	return cs.ledger
}

// Order passes through to the Consenter implementation.
func (cs *ChainSupport) Order(env *cb.Envelope, configSeq uint64) error {
	return cs.chain.Order(env, configSeq)
}

// Configure passes through to the Consenter implementation.
func (cs *ChainSupport) Configure(configUpdate *cb.Envelope, config *cb.Envelope, configSeq uint64) error {
	return cs.chain.Configure(configUpdate, config, configSeq)
}

// Errored returns whether the backing consenter has errored
func (cs *ChainSupport) Errored() <-chan struct{} {
	return cs.chain.Errored()
}

// CreateNextBlock creates a new block with the next block number, and the given contents.
func (cs *ChainSupport) CreateNextBlock(messages []*cb.Envelope) *cb.Block {
	return ledger.CreateNextBlock(cs.ledger, messages)
}

func (cs *ChainSupport) addBlockSignature(block *cb.Block) {
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

func (cs *ChainSupport) addLastConfigSignature(block *cb.Block) {
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

// WriteConfigBlock should be invoked for blocks which contain a config transaction.
func (cs *ChainSupport) WriteConfigBlock(block *cb.Block, encodedMetadataValue []byte) *cb.Block {
	// XXX This hacky path is temporary and will be removed by the end of this change series
	// The panics here are just fine
	committer, err := cs.filters.Apply(utils.UnmarshalEnvelopeOrPanic(block.Data.Data[0]))
	if err != nil {
		logger.Panicf("Config should have already been validated")
	}
	committer.Commit()

	return cs.WriteBlock(block, encodedMetadataValue)
}

// WriteBlock should be invoked for blocks which contain normal transactions.
func (cs *ChainSupport) WriteBlock(block *cb.Block, encodedMetadataValue []byte) *cb.Block {
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

// Height passes through to the underlying ledger's Height.
func (cs *ChainSupport) Height() uint64 {
	return cs.Reader().Height()
}
