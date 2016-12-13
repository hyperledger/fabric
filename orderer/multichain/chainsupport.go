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
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/orderer/common/blockcutter"
	"github.com/hyperledger/fabric/orderer/common/broadcast"
	"github.com/hyperledger/fabric/orderer/common/deliver"
	"github.com/hyperledger/fabric/orderer/common/filter"
	"github.com/hyperledger/fabric/orderer/common/sharedconfig"
	"github.com/hyperledger/fabric/orderer/rawledger"
	cb "github.com/hyperledger/fabric/protos/common"
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
	WriteBlock(data []*cb.Envelope, metadata [][]byte, committers []filter.Committer)
	ChainID() string // ChainID returns the chain ID this specific consenter instance is associated with
}

// ChainSupport provides a wrapper for the resources backing a chain
type ChainSupport interface {
	broadcast.Support
	deliver.Support
	ConsenterSupport

	// ConfigTxManager returns the corresponding configtx.Manager for this chain
	ConfigTxManager() configtx.Manager
}

type chainSupport struct {
	chain               Chain
	cutter              blockcutter.Receiver
	configManager       configtx.Manager
	policyManager       policies.Manager
	sharedConfigManager sharedconfig.Manager
	ledger              rawledger.ReadWriter
	filters             *filter.RuleSet
}

func newChainSupport(
	filters *filter.RuleSet,
	configManager configtx.Manager,
	policyManager policies.Manager,
	backing rawledger.ReadWriter,
	sharedConfigManager sharedconfig.Manager,
	consenters map[string]Consenter,
) *chainSupport {

	cutter := blockcutter.NewReceiverImpl(sharedConfigManager, filters)
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
		ledger:              backing,
	}

	var err error
	cs.chain, err = consenter.HandleChain(cs)
	if err != nil {
		logger.Fatalf("Error creating consenter for chain %x: %s", configManager.ChainID(), err)
	}

	return cs
}

// createStandardFilters creates the set of filters for a normal (non-system) chain
func createStandardFilters(configManager configtx.Manager) *filter.RuleSet {
	return filter.NewRuleSet([]filter.Rule{
		filter.EmptyRejectRule,
		configtx.NewFilter(configManager),
		filter.AcceptRule,
	})

}

// createSystemChainFilters creates the set of filters for the ordering system chain
func createSystemChainFilters(ml *multiLedger, configManager configtx.Manager) *filter.RuleSet {
	return filter.NewRuleSet([]filter.Rule{
		filter.EmptyRejectRule,
		newSystemChainFilter(ml),
		configtx.NewFilter(configManager),
		filter.AcceptRule,
	})
}

func (cs *chainSupport) start() {
	cs.chain.Start()
}

func (cs *chainSupport) SharedConfig() sharedconfig.Manager {
	return cs.sharedConfigManager
}

func (cs *chainSupport) ConfigTxManager() configtx.Manager {
	return cs.configManager
}

func (cs *chainSupport) ChainID() string {
	return cs.configManager.ChainID()
}

func (cs *chainSupport) PolicyManager() policies.Manager {
	return cs.policyManager
}

func (cs *chainSupport) Filters() *filter.RuleSet {
	return cs.filters
}

func (cs *chainSupport) BlockCutter() blockcutter.Receiver {
	return cs.cutter
}

func (cs *chainSupport) Reader() rawledger.Reader {
	return cs.ledger
}

func (cs *chainSupport) Enqueue(env *cb.Envelope) bool {
	return cs.chain.Enqueue(env)
}

func (cs *chainSupport) WriteBlock(data []*cb.Envelope, metadata [][]byte, committers []filter.Committer) {
	for _, committer := range committers {
		committer.Commit()
	}

	cs.ledger.Append(data, metadata)
}
