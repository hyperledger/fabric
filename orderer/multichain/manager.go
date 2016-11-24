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
	"sync"

	"github.com/hyperledger/fabric/orderer/common/configtx"
	"github.com/hyperledger/fabric/orderer/common/policies"
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
}

type multiLedger struct {
	chains        map[string]*chainSupport
	consenters    map[string]Consenter
	ledgerFactory rawledger.Factory
	mutex         sync.Mutex
}

// getConfigTx, this should ultimately be done more intelligently, but for now, we search the whole chain for txs and pick the last config one
func getConfigTx(reader rawledger.Reader) *cb.Envelope {
	var lastConfigTx *cb.Envelope

	it, _ := reader.Iterator(ab.SeekInfo_OLDEST, 0)
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
			err = proto.Unmarshal(maybeConfigTx.Payload, payload)

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
		ml.chains[chainID] = newChainSupport(configManager, policyManager, backingLedger, sharedConfigManager, consenters)
	}

	for _, cs := range ml.chains {
		cs.start()
	}

	return ml
}

// GetChain retrieves the chain support for a chain (and whether it exists)
func (ml *multiLedger) GetChain(chainID string) (ChainSupport, bool) {
	cs, ok := ml.chains[chainID]
	return cs, ok
}

func (ml *multiLedger) newResources(configTx *cb.Envelope) (configtx.Manager, policies.Manager, rawledger.ReadWriter, sharedconfig.Manager) {
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

	configManager, err := configtx.NewConfigurationManager(configEnvelope, policyManager, configHandlerMap)
	if err != nil {
		logger.Fatalf("Error unpacking configuration transaction: %s", err)
	}

	chainID := configManager.ChainID()

	ledger, err := ml.ledgerFactory.GetOrCreate(chainID)
	if err != nil {
		logger.Fatalf("Error getting ledger for %s", chainID)
	}

	return configManager, policyManager, ledger, sharedConfigManager
}
