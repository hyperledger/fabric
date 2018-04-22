/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package peer

import (
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/protos/common"
)

// computeFullConfig computes the full resource configuration given the current resource bundle and the transaction (that contains the delta)
func computeFullConfig(currentConfigBundle *channelconfig.Bundle, channelConfTx *common.Envelope) (*common.Config, error) {
	fullChannelConfigEnv, err := currentConfigBundle.ConfigtxValidator().ProposeConfigUpdate(channelConfTx)
	if err != nil {
		return nil, err
	}
	return fullChannelConfigEnv.Config, nil
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

// retrievePersistedChannelConfig retrieves the persisted channel config from statedb
func retrievePersistedChannelConfig(ledger ledger.PeerLedger) (*common.Config, error) {
	qe, err := ledger.NewQueryExecutor()
	if err != nil {
		return nil, err
	}
	defer qe.Done()
	return retrievePersistedConf(qe, channelConfigKey)
}
