/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package peer

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/resourcesconfig"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/customtx"
	"github.com/hyperledger/fabric/protos/common"
	protospeer "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
)

// validateAndApplyResourceConfig validates that resourceConfigTx does not have any read-write conflict with the current
// resource configuration and prepares the full configuration by applying delta in the transaction to the current config.
// Finally, this sets the full configuration.
// TODO - Make sure that ProposeConfigUpdate is either deterministic in throwing error or it should throw
// explicit throw &customtx.InvalidTxError in the case of version conflict and general error for all other error conditions
func validateAndApplyResourceConfig(chainid string, resourceConfigTx *common.Envelope) (*common.Config, error) {
	chains.Lock()
	defer chains.Unlock()
	cs := chains.list[chainid].cs
	currentBundle := cs.bundleSource.StableBundle()
	fullResourceConfig, err := computeFullConfig(currentBundle, resourceConfigTx)
	if err != nil {
		peerLogger.Debugf("Failed to validate PEER_RESOURCE_UPDATE because: %s", err)
		return nil, &customtx.InvalidTxError{Msg: fmt.Sprintf("Error in validating the resource conf transaction: %s", err)}
	}
	peerLogger.Debugf("Successfully validated PEER_RESOURCE_UPDATE")
	rBundle, err := resourcesconfig.NewBundle(chainid, fullResourceConfig, currentBundle.ChannelConfig())
	if err != nil {
		return nil, err
	}
	cs.bundleSource.Update(rBundle)
	return fullResourceConfig, nil
}

func isResConfigCapabilityOn(chainid string, chanConfig *common.Config) (bool, error) {
	chanConfigBundle, err := channelconfig.NewBundle(chainid, chanConfig)
	if err != nil {
		return false, err
	}
	appConfig, exists := chanConfigBundle.ApplicationConfig()
	if !exists {
		return false, fmt.Errorf("Application config missing")
	}
	return appConfig.Capabilities().ResourcesTree(), nil
}

// extractFullConfigFromSeedTx pulls out the seed resource config tx from the config transaction in the genesis block
func extractFullConfigFromSeedTx(configEnvelope *common.ConfigEnvelope) (*common.Config, error) {
	configUpdateEnvelop := &common.ConfigUpdateEnvelope{}
	configUpdate := &common.ConfigUpdate{}
	if configEnvelope.LastUpdate == nil {
		return nil, nil
	}
	if _, err := utils.UnmarshalEnvelopeOfType(configEnvelope.LastUpdate, common.HeaderType_CONFIG_UPDATE, configUpdateEnvelop); err != nil {
		return nil, err
	}
	if err := proto.Unmarshal(configUpdateEnvelop.ConfigUpdate, configUpdate); err != nil {
		return nil, err
	}

	isolatedData := configUpdate.IsolatedData
	resourceConfigSeedDataBytes := isolatedData[protospeer.ResourceConfigSeedDataKey]
	if resourceConfigSeedDataBytes == nil {
		return nil, nil
	}
	resourceConfig := &common.Config{}
	if err := proto.Unmarshal(resourceConfigSeedDataBytes, resourceConfig); err != nil {
		return nil, err
	}
	return resourceConfig, nil
}

// computeFullConfig computes the full resource configuration given the current resource bundle and the transaction (that contains the delta)
func computeFullConfig(currentResourceBundle *resourcesconfig.Bundle, resConfTx *common.Envelope) (*common.Config, error) {
	fullResourceConfigEnv, err := currentResourceBundle.ConfigtxValidator().ProposeConfigUpdate(resConfTx)
	if err != nil {
		return nil, err
	}
	return fullResourceConfigEnv.Config, nil
}

// TODO preferably make the serialize/deserialize deterministic
func serialize(resConfig *common.Config) ([]byte, error) {
	return proto.Marshal(resConfig)
}

func deserialize(serializedConf []byte) (*common.Config, error) {
	conf := &common.Config{}
	if err := proto.Unmarshal(serializedConf, conf); err != nil {
		return nil, err
	}
	return conf, nil
}

// retrievePersistedResourceConfig retrieves the persisted resource config from statedb
func retrievePersistedResourceConfig(ledger ledger.PeerLedger) (*common.Config, error) {
	qe, err := ledger.NewQueryExecutor()
	if err != nil {
		return nil, err
	}
	defer qe.Done()
	return retrievePersistedConf(qe, resourcesConfigKey)
}

// retrievePersistedChannelConfig retrieves the persisted channel config from statedb
func retrievePersistedChannelConfig(ledger ledger.PeerLedger) (*common.Config, error) {
	qe, err := ledger.NewQueryExecutor()
	if err != nil {
		return nil, err
	}
	defer qe.Done()
	return retrievePersistedConf(qe, channelConfigKey)
}
