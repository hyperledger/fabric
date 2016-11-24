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
	"github.com/hyperledger/fabric/orderer/common/broadcastfilter"
	"github.com/hyperledger/fabric/orderer/common/configtx"
	"github.com/hyperledger/fabric/orderer/common/policies"
	"github.com/hyperledger/fabric/orderer/common/sharedconfig"
	"github.com/hyperledger/fabric/orderer/rawledger"
	cb "github.com/hyperledger/fabric/protos/common"
)

const XXXBatchSize = 10 // XXX

// Consenter defines the backing ordering mechanism
type Consenter interface {
	// HandleChain should create a return a reference to a Chain for the given set of resources
	// It will only be invoked for a given chain once per process.  See the description of Chain
	// for more details
	HandleChain(configManager configtx.Manager, cutter blockcutter.Receiver, rl rawledger.Writer, metadata []byte) Chain
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

// ChainSupport provides a wrapper for the resources backing a chain
type ChainSupport interface {
	// ConfigManager returns the current config for the chain
	ConfigManager() configtx.Manager

	// PolicyManager returns the current policy manager as specified by the chain configuration
	PolicyManager() policies.Manager

	// Filters returns the set of broadcast filters for this chain
	Filters() *broadcastfilter.RuleSet

	// Reader returns the chain Reader for the chain
	Reader() rawledger.Reader

	// Chain returns the consenter backed chain
	Chain() Chain
}

type chainSupport struct {
	chain         Chain
	configManager configtx.Manager
	policyManager policies.Manager
	reader        rawledger.Reader
	writer        rawledger.Writer
	filters       *broadcastfilter.RuleSet
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
		configManager: configManager,
		policyManager: policyManager,
		filters:       filters,
		reader:        backing,
		writer:        newWriteInterceptor(configManager, backing),
	}

	cs.chain = consenter.HandleChain(configManager, cutter, cs.writer, nil)

	return cs
}

func createBroadcastRuleset(configManager configtx.Manager) *broadcastfilter.RuleSet {
	return broadcastfilter.NewRuleSet([]broadcastfilter.Rule{
		broadcastfilter.EmptyRejectRule,
		// configfilter.New(configManager),
		broadcastfilter.AcceptRule,
	})
}

func (cs *chainSupport) start() {
	cs.chain.Start()
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

func (cs *chainSupport) Reader() rawledger.Reader {
	return cs.reader
}

func (cs *chainSupport) Chain() Chain {
	return cs.chain
}

type writeInterceptor struct {
	backing rawledger.Writer
}

// TODO ultimately set write interception policy by config
func newWriteInterceptor(configManager configtx.Manager, backing rawledger.Writer) *writeInterceptor {
	return &writeInterceptor{
		backing: backing,
	}
}

func (wi *writeInterceptor) Append(blockContents []*cb.Envelope, metadata [][]byte) *cb.Block {
	return wi.backing.Append(blockContents, metadata)
}
