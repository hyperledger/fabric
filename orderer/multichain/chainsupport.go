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
	"github.com/hyperledger/fabric/orderer/common/blockcutter"
	"github.com/hyperledger/fabric/orderer/common/broadcast"
	"github.com/hyperledger/fabric/orderer/common/broadcastfilter"
	"github.com/hyperledger/fabric/orderer/common/configtx"
	"github.com/hyperledger/fabric/orderer/common/deliver"
	"github.com/hyperledger/fabric/orderer/common/policies"
	"github.com/hyperledger/fabric/orderer/common/sharedconfig"
	"github.com/hyperledger/fabric/orderer/rawledger"
	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/golang/protobuf/proto"
)

// Consenter defines the backing ordering mechanism
type Consenter interface {
	// HandleChain should create a return a reference to a Chain for the given set of resources
	// It will only be invoked for a given chain once per process.  In general, errors will be treated
	// as irrecoverable and cause system shutdown.  See the description of Chain for more details
	HandleChain(support ConsenterSupport) (Chain, error)
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
	BlockCutter() blockcutter.Receiver
	SharedConfig() sharedconfig.Manager
	Writer() rawledger.Writer
}

// ChainSupport provides a wrapper for the resources backing a chain
type ChainSupport interface {
	broadcast.Support
	deliver.Support
	ConsenterSupport
}

type chainSupport struct {
	chain               Chain
	cutter              blockcutter.Receiver
	configManager       configtx.Manager
	policyManager       policies.Manager
	sharedConfigManager sharedconfig.Manager
	reader              rawledger.Reader
	writer              rawledger.Writer
	filters             *broadcastfilter.RuleSet
}

func newChainSupport(configManager configtx.Manager, policyManager policies.Manager, backing rawledger.ReadWriter, sharedConfigManager sharedconfig.Manager, consenters map[string]Consenter) *chainSupport {
	batchSize := sharedConfigManager.BatchSize()
	filters := createBroadcastRuleset(configManager)
	cutter := blockcutter.NewReceiverImpl(batchSize, filters, configManager)
	consenterType := sharedConfigManager.ConsensusType()
	consenter, ok := consenters[consenterType]
	if !ok {
		logger.Fatalf("Error retrieving consenter of type: %s", consenterType)
	}

	cs := &chainSupport{
		configManager:       configManager,
		policyManager:       policyManager,
		sharedConfigManager: sharedConfigManager,
		cutter:              cutter,
		filters:             filters,
		reader:              backing,
		writer:              newWriteInterceptor(configManager, backing),
	}

	var err error
	cs.chain, err = consenter.HandleChain(cs)
	if err != nil {
		logger.Fatalf("Error creating consenter for chain %x: %s", configManager.ChainID(), err)
	}

	return cs
}

func createBroadcastRuleset(configManager configtx.Manager) *broadcastfilter.RuleSet {
	return broadcastfilter.NewRuleSet([]broadcastfilter.Rule{
		broadcastfilter.EmptyRejectRule,
		configtx.NewFilter(configManager),
		broadcastfilter.AcceptRule,
	})
}

func (cs *chainSupport) start() {
	cs.chain.Start()
}

func (cs *chainSupport) SharedConfig() sharedconfig.Manager {
	return cs.sharedConfigManager
}

func (cs *chainSupport) ConfigManager() configtx.Manager {
	return cs.configManager
}

func (cs *chainSupport) PolicyManager() policies.Manager {
	return cs.policyManager
}

func (cs *chainSupport) Filters() *broadcastfilter.RuleSet {
	return cs.filters
}

func (cs *chainSupport) BlockCutter() blockcutter.Receiver {
	return cs.cutter
}

func (cs *chainSupport) Reader() rawledger.Reader {
	return cs.reader
}

func (cs *chainSupport) Writer() rawledger.Writer {
	return cs.writer
}

func (cs *chainSupport) Enqueue(env *cb.Envelope) bool {
	return cs.chain.Enqueue(env)
}

// writeInterceptor performs 'execution/processing' of blockContents before committing them to the normal passive ledger
// This is intended to support reconfiguration transactions, and ultimately chain creation
type writeInterceptor struct {
	configtxManager configtx.Manager
	backing         rawledger.Writer
}

func newWriteInterceptor(configtxManager configtx.Manager, backing rawledger.Writer) *writeInterceptor {
	return &writeInterceptor{
		backing:         backing,
		configtxManager: configtxManager,
	}
}

func (wi *writeInterceptor) Append(blockContents []*cb.Envelope, metadata [][]byte) *cb.Block {
	// Note that in general any errors encountered in this path are fatal.
	// The previous layers (broadcastfilters, blockcutter) should have scrubbed any invalid
	// 'executable' transactions like config before committing via Append

	if len(blockContents) == 1 {
		payload := &cb.Payload{}
		err := proto.Unmarshal(blockContents[0].Payload, payload)
		if err != nil {
			logger.Fatalf("Asked to write a malformed envelope to the chain: %s", err)
		}

		if payload.Header.ChainHeader.Type == int32(cb.HeaderType_CONFIGURATION_TRANSACTION) {
			configEnvelope := &cb.ConfigurationEnvelope{}
			err = proto.Unmarshal(payload.Data, configEnvelope)
			if err != nil {
				logger.Fatalf("Configuration envelope was malformed: %s", err)
			}

			err = wi.configtxManager.Apply(configEnvelope)
			if err != nil {
				logger.Fatalf("Error applying configuration transaction which was already validated: %s", err)
			}
		}
	}
	return wi.backing.Append(blockContents, metadata)
}
