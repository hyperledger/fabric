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
	"fmt"

	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/orderer/common/sharedconfig"
	"github.com/hyperledger/fabric/orderer/rawledger"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/op/go-logging"

	"github.com/golang/protobuf/proto"
)

var logger = logging.MustGetLogger("orderer/multichain")

// XXX This crypto helper is a stand in until we have a real crypto handler
// it considers all signatures to be valid
type xxxCryptoHelper struct{}

func (xxx xxxCryptoHelper) VerifySignature(msg []byte, ids []byte, sigs []byte) bool {
	return true
}

func init() {
	logging.SetLevel(logging.DEBUG, "")
}

// Manager coordinates the creation and access of chains
type Manager interface {
	// GetChain retrieves the chain support for a chain (and whether it exists)
	GetChain(chainID string) (ChainSupport, bool)

	// ProposeChain accepts a configuration transaction for a chain which does not already exists
	// The status returned is whether the proposal is accepted for consideration, only after consensus
	// occurs will the proposal be committed or rejected
	ProposeChain(env *cb.Envelope) cb.Status
}

type multiLedger struct {
	chains        map[string]*chainSupport
	consenters    map[string]Consenter
	ledgerFactory rawledger.Factory
	sysChain      *systemChain
}

// getConfigTx, this should ultimately be done more intelligently, but for now, we search the whole chain for txs and pick the last config one
func getConfigTx(reader rawledger.Reader) *cb.Envelope {
	var lastConfigTx *cb.Envelope

	it, _ := reader.Iterator(&ab.SeekPosition{Type: &ab.SeekPosition_Oldest{}})
	// Iterate over the blockchain, looking for config transactions, track the most recent one encountered
	// this will be the transaction which is returned
	for {
		select {
		case <-it.ReadyChan():
			block, status := it.Next()
			if status != cb.Status_SUCCESS {
				logger.Fatalf("Error parsing blockchain at startup: %v", status)
			}
			// ConfigTxs should always be by themselves
			if len(block.Data.Data) != 1 {
				continue
			}

			maybeConfigTx := &cb.Envelope{}

			err := proto.Unmarshal(block.Data.Data[0], maybeConfigTx)

			if err != nil {
				logger.Fatalf("Found data which was not an envelope: %s", err)
			}

			payload := &cb.Payload{}
			if err = proto.Unmarshal(maybeConfigTx.Payload, payload); err != nil {
				logger.Fatalf("Unable to unmarshal transaction payload: %s", err)
			}

			if payload.Header.ChainHeader.Type != int32(cb.HeaderType_CONFIGURATION_TRANSACTION) {
				continue
			}

			logger.Debugf("Found configuration transaction for chain %x at block %d", payload.Header.ChainHeader.ChainID, block.Header.Number)
			lastConfigTx = maybeConfigTx
		default:
			return lastConfigTx
		}
	}
}

// NewManagerImpl produces an instance of a Manager
func NewManagerImpl(ledgerFactory rawledger.Factory, consenters map[string]Consenter) Manager {
	ml := &multiLedger{
		chains:        make(map[string]*chainSupport),
		ledgerFactory: ledgerFactory,
		consenters:    consenters,
	}

	existingChains := ledgerFactory.ChainIDs()
	for _, chainID := range existingChains {
		rl, err := ledgerFactory.GetOrCreate(chainID)
		if err != nil {
			logger.Fatalf("Ledger factory reported chainID %s but could not retrieve it: %s", chainID, err)
		}
		configTx := getConfigTx(rl)
		if configTx == nil {
			logger.Fatalf("Could not find configuration transaction for chain %s", chainID)
		}
		configManager, policyManager, backingLedger, sharedConfigManager := ml.newResources(configTx)
		chainID := configManager.ChainID()

		if sharedConfigManager.ChainCreators() != nil {
			if ml.sysChain != nil {
				logger.Fatalf("There appear to be two system chains %x and %x", ml.sysChain.support.ChainID(), chainID)
			}
			logger.Debugf("Starting with system chain: %x", chainID)
			chain := newChainSupport(createSystemChainFilters(ml, configManager), configManager, policyManager, backingLedger, sharedConfigManager, consenters)
			ml.chains[string(chainID)] = chain
			ml.sysChain = newSystemChain(chain)
			// We delay starting this chain, as it might try to copy and replace the chains map via newChain before the map is fully built
			defer chain.start()
		} else {
			logger.Debugf("Starting chain: %x", chainID)
			chain := newChainSupport(createStandardFilters(configManager), configManager, policyManager, backingLedger, sharedConfigManager, consenters)
			ml.chains[string(chainID)] = chain
			chain.start()
		}

	}

	return ml
}

// ProposeChain accepts a configuration transaction for a chain which does not already exists
// The status returned is whether the proposal is accepted for consideration, only after consensus
// occurs will the proposal be committed or rejected
func (ml *multiLedger) ProposeChain(env *cb.Envelope) cb.Status {
	return ml.sysChain.proposeChain(env)
}

// GetChain retrieves the chain support for a chain (and whether it exists)
func (ml *multiLedger) GetChain(chainID string) (ChainSupport, bool) {
	cs, ok := ml.chains[chainID]
	return cs, ok
}

func newConfigTxManagerAndHandlers(configEnvelope *cb.ConfigurationEnvelope) (configtx.Manager, policies.Manager, sharedconfig.Manager, error) {
	policyManager := policies.NewManagerImpl(xxxCryptoHelper{})
	sharedConfigManager := sharedconfig.NewManagerImpl()
	configHandlerMap := make(map[cb.ConfigurationItem_ConfigurationType]configtx.Handler)
	for ctype := range cb.ConfigurationItem_ConfigurationType_name {
		rtype := cb.ConfigurationItem_ConfigurationType(ctype)
		switch rtype {
		case cb.ConfigurationItem_Policy:
			configHandlerMap[rtype] = policyManager
		case cb.ConfigurationItem_Orderer:
			configHandlerMap[rtype] = sharedConfigManager
		default:
			configHandlerMap[rtype] = configtx.NewBytesHandler()
		}
	}

	configManager, err := configtx.NewConfigurationManager(configEnvelope, policyManager, configHandlerMap)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("Error unpacking configuration transaction: %s", err)
	}

	return configManager, policyManager, sharedConfigManager, nil
}

func (ml *multiLedger) newResources(configTx *cb.Envelope) (configtx.Manager, policies.Manager, rawledger.ReadWriter, sharedconfig.Manager) {
	payload := &cb.Payload{}
	err := proto.Unmarshal(configTx.Payload, payload)
	if err != nil {
		logger.Fatalf("Error unmarshaling a config transaction payload: %s", err)
	}

	configEnvelope := &cb.ConfigurationEnvelope{}
	err = proto.Unmarshal(payload.Data, configEnvelope)
	if err != nil {
		logger.Fatalf("Error unmarshaling a config transaction to config envelope: %s", err)
	}

	configManager, policyManager, sharedConfigManager, err := newConfigTxManagerAndHandlers(configEnvelope)

	if err != nil {
		logger.Fatalf("Error creating configtx manager and handlers: %s", err)
	}

	chainID := configManager.ChainID()

	ledger, err := ml.ledgerFactory.GetOrCreate(chainID)
	if err != nil {
		logger.Fatalf("Error getting ledger for %s", chainID)
	}

	return configManager, policyManager, ledger, sharedConfigManager
}

func (ml *multiLedger) systemChain() *systemChain {
	return ml.sysChain
}

func (ml *multiLedger) newChain(configtx *cb.Envelope) {
	configManager, policyManager, backingLedger, sharedConfig := ml.newResources(configtx)
	backingLedger.Append([]*cb.Envelope{configtx}, nil)

	// Copy the map to allow concurrent reads from broadcast/deliver while the new chainSupport is
	newChains := make(map[string]*chainSupport)
	for key, value := range ml.chains {
		newChains[key] = value
	}

	cs := newChainSupport(createStandardFilters(configManager), configManager, policyManager, backingLedger, sharedConfig, ml.consenters)
	chainID := configManager.ChainID()

	logger.Debugf("Created and starting new chain %s", chainID)

	newChains[string(chainID)] = cs
	cs.start()

	ml.chains = newChains
}
