/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package peer

import (
	"fmt"

	"github.com/gogo/protobuf/proto"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protoutil"
)

const (
	channelConfigKey = "resourcesconfigtx.CHANNEL_CONFIG_KEY"
	peerNamespace    = ""
)

// ConfigTxProcessor implements the interface 'github.com/hyperledger/fabric/core/ledger/customtx/Processor'
type ConfigTxProcessor struct{}

// GenerateSimulationResults implements function in the interface 'github.com/hyperledger/fabric/core/ledger/customtx/Processor'
// This implemantation processes following two types of transactions.
// CONFIG  - simply stores the config in the statedb. Additionally, stores the resource config seed if the transaction is from the genesis block.
// PEER_RESOURCE_UPDATE - In a normal course, this validates the transaction against the current resource bundle,
// computes the full configuration, and stores the full configuration if the transaction is found valid.
// However, if 'initializingLedger' is true (i.e., either the ledger is being created from the genesis block
// or the ledger is synching the state with the blockchain, during start up), the full config is computed using
// the most recent configs from statedb
func (tp *ConfigTxProcessor) GenerateSimulationResults(txEnv *common.Envelope, simulator ledger.TxSimulator, initializingLedger bool) error {
	payload := protoutil.UnmarshalPayloadOrPanic(txEnv.Payload)
	channelHdr := protoutil.UnmarshalChannelHeaderOrPanic(payload.Header.ChannelHeader)
	txType := common.HeaderType(channelHdr.GetType())

	switch txType {
	case common.HeaderType_CONFIG:
		peerLogger.Debugf("Processing CONFIG")
		return processChannelConfigTx(txEnv, simulator)

	default:
		return fmt.Errorf("tx type [%s] is not expected", txType)
	}
}

func processChannelConfigTx(txEnv *common.Envelope, simulator ledger.TxSimulator) error {
	configEnvelope := &common.ConfigEnvelope{}
	if _, err := protoutil.UnmarshalEnvelopeOfType(txEnv, common.HeaderType_CONFIG, configEnvelope); err != nil {
		return err
	}
	channelConfig := configEnvelope.Config
	if channelConfig == nil {
		return fmt.Errorf("channel config found nil")
	}

	if err := persistConf(simulator, channelConfigKey, channelConfig); err != nil {
		return err
	}

	peerLogger.Debugf("channelConfig=%s", channelConfig)

	return nil
}

func persistConf(simulator ledger.TxSimulator, key string, config *common.Config) error {
	serializedConfig, err := proto.Marshal(config)
	if err != nil {
		return err
	}
	return simulator.SetState(peerNamespace, key, serializedConfig)
}

func retrievePersistedConf(queryExecuter ledger.QueryExecutor, key string) (*common.Config, error) {
	serializedConfig, err := queryExecuter.GetState(peerNamespace, key)
	if err != nil {
		return nil, err
	}
	if serializedConfig == nil {
		return nil, nil
	}
	return deserialize(serializedConfig)
}

func deserialize(serializedConf []byte) (*common.Config, error) {
	conf := &common.Config{}
	if err := proto.Unmarshal(serializedConf, conf); err != nil {
		return nil, err
	}
	return conf, nil
}
