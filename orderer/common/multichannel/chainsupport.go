/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package multichannel

import (
	"github.com/hyperledger/fabric/common/channelconfig"
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
	// Needed for consensus-type migration: to execute the migration state machine correctly,
	// chains need to know if they are system or standard channel.
	systemChannel bool
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

	// When ConsortiumsConfig exists, it is the system channel
	_, cs.systemChannel = ledgerResources.ConsortiumsConfig()

	// Set up the msgprocessor
	cs.Processor = msgprocessor.NewStandardChannel(cs, msgprocessor.CreateStandardChannelFilters(cs))

	// Set up the block writer
	cs.BlockWriter = newBlockWriter(lastBlock, registrar, cs)

	// TODO Identify recovery after crash in the middle of consensus-type migration
	if cs.detectMigration(lastBlock) {
		// We do this because the last block after migration (COMMIT/CONTEXT) carries Kafka metadata.
		// This prevents the code down the line from unmarshaling it as Raft, and panicking.
		metadata.Value = nil
		logger.Debugf("[channel: %s] Consensus-type migration: restart on to Raft, resetting Kafka block metadata", cs.ChainID())
	}

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

// detectMigration identifies restart after consensus-type migration was committed (green path).
// Restart after migration is detected by:
// 1. The Kafka2RaftMigration capability in on
// 2. The last block carries a config-tx
// 3. In the config-tx, you have:
//    - (system-channel && state=COMMIT), OR
//    - (standard-channel && state=CONTEXT)
// This assumes that migration was successful (green path). When migration ends successfully,
// every channel will have a config block as the last block. On the system channel, containing state=COMMIT;
// on standard channels, containing state=CONTEXT.
func (cs *ChainSupport) detectMigration(lastBlock *cb.Block) bool {
	isMigration := false

	if !cs.ledgerResources.SharedConfig().Capabilities().Kafka2RaftMigration() {
		return isMigration
	}

	lastConfigIndex, err := utils.GetLastConfigIndexFromBlock(lastBlock)
	if err != nil {
		logger.Panicf("Chain did not have appropriately encoded last config in its latest block: %s", err)
	}

	logger.Debugf("[channel: %s], sysChan=%v, lastConfigIndex=%d, H=%d, mig-state: %s",
		cs.ChainID(), cs.systemChannel, lastConfigIndex, cs.ledgerResources.Height(),
		cs.ledgerResources.SharedConfig().ConsensusMigrationState())

	if lastConfigIndex == lastBlock.Header.Number { //The last block was a config-tx
		state := cs.ledgerResources.SharedConfig().ConsensusMigrationState()
		if cs.systemChannel {
			if state == orderer.ConsensusType_MIG_STATE_COMMIT {
				isMigration = true
			}
		} else {
			if state == orderer.ConsensusType_MIG_STATE_CONTEXT {
				isMigration = true
			}
		}

		if isMigration {
			logger.Infof("[channel: %s], Restarting after consensus-type migration. New consensus-type is: %s",
				cs.ChainID(), cs.ledgerResources.SharedConfig().ConsensusType())
		}
	}

	return isMigration
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

// IsSystemChannel returns true if this is the system channel.
func (cs *ChainSupport) IsSystemChannel() bool {
	return cs.systemChannel
}
