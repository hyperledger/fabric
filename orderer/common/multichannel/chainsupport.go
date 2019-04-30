/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package multichannel

import (
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/common/ledger/blockledger"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/orderer/common/blockcutter"
	"github.com/hyperledger/fabric/orderer/common/msgprocessor"
	"github.com/hyperledger/fabric/orderer/consensus"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
)

// ChainSupport holds the resources for a particular channel.
type ChainSupport struct {
	*ledgerResources
	msgprocessor.Processor
	*BlockWriter
	consensus.Chain
	cutter blockcutter.Receiver
	crypto.LocalSigner
}

func newChainSupport(
	registrar *Registrar,
	ledgerResources *ledgerResources,
	consenters map[string]consensus.Consenter,
	signer crypto.LocalSigner,
	blockcutterMetrics *blockcutter.Metrics,
) *ChainSupport {
	// Read in the last block and metadata for the channel
	lastBlock := blockledger.GetBlock(ledgerResources, ledgerResources.Height()-1)
	metadata, err := utils.GetMetadataFromBlock(lastBlock, cb.BlockMetadataIndex_ORDERER)
	// Assuming a block created with cb.NewBlock(), this should not
	// error even if the orderer metadata is an empty byte slice
	if err != nil {
		logger.Fatalf("[channel: %s] Error extracting orderer metadata: %s", ledgerResources.ConfigtxValidator().ChainID(), err)
	}

	// Construct limited support needed as a parameter for additional support
	cs := &ChainSupport{
		ledgerResources: ledgerResources,
		LocalSigner:     signer,
		cutter: blockcutter.NewReceiverImpl(
			ledgerResources.ConfigtxValidator().ChainID(),
			ledgerResources,
			blockcutterMetrics,
		),
	}

	// Set up the msgprocessor
	cs.Processor = msgprocessor.NewStandardChannel(cs, msgprocessor.CreateStandardChannelFilters(cs))

	// Set up the block writer
	cs.BlockWriter = newBlockWriter(lastBlock, registrar, cs)

	// Set up the consenter
	consenterType := ledgerResources.SharedConfig().ConsensusType()
	consenter, ok := consenters[consenterType]
	if !ok {
		logger.Panicf("Error retrieving consenter of type: %s", consenterType)
	}

	cs.Chain, err = consenter.HandleChain(cs, metadata)
	if err != nil {
		logger.Panicf("[channel: %s] Error creating consenter: %s", cs.ChainID(), err)
	}

	logger.Debugf("[channel: %s] Done creating channel support resources", cs.ChainID())

	return cs
}

// DetectConsensusMigration identifies restart after consensus-type migration.
// Restart after migration is detected by:
// 1. The last block and the block before it carry a config-tx
// 2. In both blocks we have ConsensusType.State == MAINTENANCE
// 3. The ConsensusType.Type is different in these two blocks
// 4. The last block metadata is empty
func (cs *ChainSupport) DetectConsensusMigration() bool {
	if !cs.ledgerResources.SharedConfig().Capabilities().ConsensusTypeMigration() {
		logger.Debugf("[channel: %s] Orderer capability ConsensusTypeMigration is disabled", cs.ChainID())
		return false
	}

	if cs.Height() <= 1 {
		return false
	}

	lastConfigIndex, err := utils.GetLastConfigIndexFromBlock(cs.lastBlock)
	if err != nil {
		logger.Panicf("[channel: %s] Chain did not have appropriately encoded last config in its latest block: %s",
			cs.ChainID(), err)
	}
	logger.Debugf("[channel: %s] lastBlockNumber=%d, lastConfigIndex=%d",
		cs.support.ChainID(), cs.lastBlock.Header.Number, lastConfigIndex)
	if lastConfigIndex != cs.lastBlock.Header.Number {
		return false
	}

	currentState := cs.support.SharedConfig().ConsensusState()
	logger.Debugf("[channel: %s] last block ConsensusState=%s", cs.ChainID(), currentState)
	if currentState != orderer.ConsensusType_STATE_MAINTENANCE {
		return false
	}

	metadata, err := utils.GetMetadataFromBlock(cs.lastBlock, cb.BlockMetadataIndex_ORDERER)
	if err != nil {
		logger.Panicf("[channel: %s] Error extracting orderer metadata: %s", cs.ChainID(), err)
	}

	metaLen := len(metadata.Value)
	logger.Debugf("[channel: %s] metadata.Value length=%d", cs.ChainID(), metaLen)
	if metaLen > 0 {
		return false
	}

	prevBlock := blockledger.GetBlock(cs.Reader(), cs.lastBlock.Header.Number-1)
	prevConfigIndex, err := utils.GetLastConfigIndexFromBlock(prevBlock)
	if err != nil {
		logger.Panicf("Chain did not have appropriately encoded last config in block %d: %s",
			prevBlock.Header.Number, err)
	}
	if prevConfigIndex != prevBlock.Header.Number {
		return false
	}

	prevPayload, prevChanHdr := cs.extractPayloadHeaderOrPanic(prevBlock)

	switch cb.HeaderType(prevChanHdr.Type) {
	case cb.HeaderType_ORDERER_TRANSACTION:
		return false

	case cb.HeaderType_CONFIG:
		return cs.isConsensusTypeChange(prevPayload, prevChanHdr.ChannelId, prevBlock.Header.Number)

	default:
		logger.Panicf("[channel: %s] config block with unknown header type in block %d: %v",
			cs.ChainID(), prevBlock.Header.Number, prevChanHdr.Type)
		return false
	}
}

func (cs *ChainSupport) extractPayloadHeaderOrPanic(prevBlock *cb.Block) (*cb.Payload, *cb.ChannelHeader) {
	configTx, err := utils.ExtractEnvelope(prevBlock, 0)
	if err != nil {
		logger.Panicf("[channel: %s] Error extracting configtx from block %d: %s",
			cs.ChainID(), prevBlock.Header.Number, err)
	}
	payload, err := utils.UnmarshalPayload(configTx.Payload)
	if err != nil {
		logger.Panicf("[channel: %s] configtx payload is invalid in block %d: %s",
			cs.ChainID(), prevBlock.Header.Number, err)
	}
	if payload.Header == nil {
		logger.Panicf("[channel: %s] configtx payload header is missing in block %d",
			cs.ChainID(), prevBlock.Header.Number)
	}
	chdr, err := utils.UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		logger.Panicf("[channel: %s] invalid channel header in block %d: %s",
			cs.ChainID(), prevBlock.Header.Number, err)
	}
	return payload, chdr
}

func (cs *ChainSupport) isConsensusTypeChange(payload *cb.Payload, channelId string, headerNumber uint64) bool {
	configEnvelope, err := configtx.UnmarshalConfigEnvelope(payload.Data)
	if err != nil {
		logger.Panicf("[channel: %s] Error extracting config envelope in block %d: %s",
			cs.ChainID(), headerNumber, err)
	}

	bundle, err := cs.CreateBundle(channelId, configEnvelope.Config)
	if err != nil {
		logger.Panicf("[channel: %s] Error converting config to a bundle in block %d: %s",
			cs.ChainID(), headerNumber, err)
	}

	oc, ok := bundle.OrdererConfig()
	if !ok {
		logger.Panicf("[channel: %s] OrdererConfig missing from bundle in block %d",
			cs.ChainID(), headerNumber)
	}

	prevState := oc.ConsensusState()
	logger.Debugf("[channel: %s] previous block ConsensusState=%s", cs.ChainID(), prevState)
	if prevState != orderer.ConsensusType_STATE_MAINTENANCE {
		return false
	}

	currentType := cs.SharedConfig().ConsensusType()
	prevType := oc.ConsensusType()
	logger.Debugf("[channel: %s] block ConsensusType: previous=%s, current=%s", cs.ChainID(), prevType, currentType)
	if currentType == prevType {
		return false
	}

	logger.Infof("[channel: %s] Consensus-type migration detected, ConsensusState=%s, ConsensusType changed from %s to %s",
		cs.ChainID(), cs.support.SharedConfig().ConsensusState(), prevType, currentType)

	return true
}

// Block returns a block with the following number,
// or nil if such a block doesn't exist.
func (cs *ChainSupport) Block(number uint64) *cb.Block {
	if cs.Height() <= number {
		return nil
	}
	return blockledger.GetBlock(cs.Reader(), number)
}

func (cs *ChainSupport) Reader() blockledger.Reader {
	return cs
}

// Signer returns the crypto.Localsigner for this channel.
func (cs *ChainSupport) Signer() crypto.LocalSigner {
	return cs
}

func (cs *ChainSupport) start() {
	cs.Chain.Start()
}

// BlockCutter returns the blockcutter.Receiver instance for this channel.
func (cs *ChainSupport) BlockCutter() blockcutter.Receiver {
	return cs.cutter
}

// Validate passes through to the underlying configtx.Validator
func (cs *ChainSupport) Validate(configEnv *cb.ConfigEnvelope) error {
	return cs.ConfigtxValidator().Validate(configEnv)
}

// ProposeConfigUpdate passes through to the underlying configtx.Validator
func (cs *ChainSupport) ProposeConfigUpdate(configtx *cb.Envelope) (*cb.ConfigEnvelope, error) {
	env, err := cs.ConfigtxValidator().ProposeConfigUpdate(configtx)
	if err != nil {
		return nil, err
	}

	bundle, err := cs.CreateBundle(cs.ChainID(), env.Config)
	if err != nil {
		return nil, err
	}

	if err = checkResources(bundle); err != nil {
		return nil, errors.Wrap(err, "config update is not compatible")
	}

	return env, cs.ValidateNew(bundle)
}

// ChainID passes through to the underlying configtx.Validator
func (cs *ChainSupport) ChainID() string {
	return cs.ConfigtxValidator().ChainID()
}

// ConfigProto passes through to the underlying configtx.Validator
func (cs *ChainSupport) ConfigProto() *cb.Config {
	return cs.ConfigtxValidator().ConfigProto()
}

// Sequence passes through to the underlying configtx.Validator
func (cs *ChainSupport) Sequence() uint64 {
	return cs.ConfigtxValidator().Sequence()
}

// Append appends a new block to the ledger in its raw form,
// unlike WriteBlock that also mutates its metadata.
func (cs *ChainSupport) Append(block *cb.Block) error {
	return cs.ledgerResources.ReadWriter.Append(block)
}

// VerifyBlockSignature verifies a signature of a block.
// It has an optional argument of a configuration envelope
// which would make the block verification to use validation rules
// based on the given configuration in the ConfigEnvelope.
// If the config envelope passed is nil, then the validation rules used
// are the ones that were applied at commit of previous blocks.
func (cs *ChainSupport) VerifyBlockSignature(sd []*cb.SignedData, envelope *cb.ConfigEnvelope) error {
	policyMgr := cs.PolicyManager()
	// If the envelope passed isn't nil, we should use a different policy manager.
	if envelope != nil {
		bundle, err := channelconfig.NewBundle(cs.ChainID(), envelope.Config)
		if err != nil {
			return err
		}
		policyMgr = bundle.PolicyManager()
	}
	policy, exists := policyMgr.GetPolicy(policies.BlockValidation)
	if !exists {
		return errors.Errorf("policy %s wasn't found", policies.BlockValidation)
	}
	err := policy.Evaluate(sd)
	if err != nil {
		return errors.Wrap(err, "block verification failed")
	}
	return nil
}
