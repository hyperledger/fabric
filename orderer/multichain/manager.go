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
	"github.com/hyperledger/fabric/orderer/common/sharedconfig"
	ordererledger "github.com/hyperledger/fabric/orderer/ledger"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/op/go-logging"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/crypto"
)

var logger = logging.MustGetLogger("orderer/multichain")

// XXX This crypto helper is a stand in until we have a real crypto handler
// it considers all signatures to be valid
type xxxCryptoHelper struct{}

func (xxx xxxCryptoHelper) VerifySignature(sd *cb.SignedData) error {
	return nil
}

func (xxx xxxCryptoHelper) NewSignatureHeader() (*cb.SignatureHeader, error) {
	return &cb.SignatureHeader{}, nil
}

func (xxx xxxCryptoHelper) Sign(message []byte) ([]byte, error) {
	return message, nil
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

type configResources struct {
	configtx.Manager
	sharedConfig sharedconfig.Manager
}

func (cr *configResources) SharedConfig() sharedconfig.Manager {
	return cr.sharedConfig
}

type ledgerResources struct {
	*configResources
	ledger ordererledger.ReadWriter
}

type multiLedger struct {
	chains        map[string]*chainSupport
	consenters    map[string]Consenter
	ledgerFactory ordererledger.Factory
	sysChain      *systemChain
	signer        crypto.LocalSigner
}

func getConfigTx(reader ordererledger.Reader) *cb.Envelope {
	lastBlock := ordererledger.GetBlock(reader, reader.Height()-1)
	index, err := utils.GetLastConfigurationIndexFromBlock(lastBlock)
	if err != nil {
		logger.Panicf("Chain did not have appropriately encoded last configuration in its latest block: %s", err)
	}
	configBlock := ordererledger.GetBlock(reader, index)
	if configBlock == nil {
		logger.Panicf("Configuration block does not exist")
	}

	return utils.ExtractEnvelopeOrPanic(configBlock, 0)
}

// NewManagerImpl produces an instance of a Manager
func NewManagerImpl(ledgerFactory ordererledger.Factory, consenters map[string]Consenter, signer crypto.LocalSigner) Manager {
	ml := &multiLedger{
		chains:        make(map[string]*chainSupport),
		ledgerFactory: ledgerFactory,
		consenters:    consenters,
		signer:        signer,
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
		ledgerResources := ml.newLedgerResources(configTx)
		chainID := ledgerResources.ChainID()

		if ledgerResources.SharedConfig().ChainCreationPolicyNames() != nil {
			if ml.sysChain != nil {
				logger.Fatalf("There appear to be two system chains %s and %s", ml.sysChain.support.ChainID(), chainID)
			}
			logger.Debugf("Starting with system chain: %x", chainID)
			chain := newChainSupport(createSystemChainFilters(ml, ledgerResources),
				ledgerResources,
				consenters,
				signer)
			ml.chains[string(chainID)] = chain
			ml.sysChain = newSystemChain(chain)
			// We delay starting this chain, as it might try to copy and replace the chains map via newChain before the map is fully built
			defer chain.start()
		} else {
			logger.Debugf("Starting chain: %x", chainID)
			chain := newChainSupport(createStandardFilters(ledgerResources),
				ledgerResources,
				consenters,
				signer)
			ml.chains[string(chainID)] = chain
			chain.start()
		}

	}

	if ml.sysChain == nil {
		logger.Panicf("No system chain found")
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

func newConfigResources(configEnvelope *cb.ConfigurationEnvelope) (*configResources, error) {
	sharedConfigManager := sharedconfig.NewManagerImpl()
	initializer := configtx.NewInitializer()
	initializer.Handlers()[cb.ConfigurationItem_Orderer] = sharedConfigManager

	configManager, err := configtx.NewManagerImpl(configEnvelope, initializer, nil)
	if err != nil {
		return nil, fmt.Errorf("Error unpacking configuration transaction: %s", err)
	}

	return &configResources{
		Manager:      configManager,
		sharedConfig: sharedConfigManager,
	}, nil
}

func (ml *multiLedger) newLedgerResources(configTx *cb.Envelope) *ledgerResources {
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

	configResources, err := newConfigResources(configEnvelope)

	if err != nil {
		logger.Fatalf("Error creating configtx manager and handlers: %s", err)
	}

	chainID := configResources.ChainID()

	ledger, err := ml.ledgerFactory.GetOrCreate(chainID)
	if err != nil {
		logger.Fatalf("Error getting ledger for %s", chainID)
	}

	return &ledgerResources{
		configResources: configResources,
		ledger:          ledger,
	}
}

func (ml *multiLedger) systemChain() *systemChain {
	return ml.sysChain
}

func (ml *multiLedger) newChain(configtx *cb.Envelope) {
	ledgerResources := ml.newLedgerResources(configtx)
	ledgerResources.ledger.Append(ordererledger.CreateNextBlock(ledgerResources.ledger, []*cb.Envelope{configtx}))

	// Copy the map to allow concurrent reads from broadcast/deliver while the new chainSupport is
	newChains := make(map[string]*chainSupport)
	for key, value := range ml.chains {
		newChains[key] = value
	}

	cs := newChainSupport(createStandardFilters(ledgerResources), ledgerResources, ml.consenters, ml.signer)
	chainID := ledgerResources.ChainID()

	logger.Debugf("Created and starting new chain %s", chainID)

	newChains[string(chainID)] = cs
	cs.start()

	ml.chains = newChains
}
