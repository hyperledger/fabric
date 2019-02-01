/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package genesis

import (
	"testing"

	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/stretchr/testify/assert"
)

func TestBasicSanity(t *testing.T) {
	impl := NewFactoryImpl(cb.NewConfigGroup())
	impl.Block("testchainid")
}

func TestForTransactionID(t *testing.T) {
	impl := NewFactoryImpl(cb.NewConfigGroup())
	block := impl.Block("testchainid")
	configEnv, _ := utils.ExtractEnvelope(block, 0)
	configEnvPayload, _ := utils.ExtractPayload(configEnv)
	configEnvPayloadChannelHeader, _ := utils.UnmarshalChannelHeader(configEnvPayload.GetHeader().ChannelHeader)
	assert.NotEmpty(t, configEnvPayloadChannelHeader.TxId, "tx_id of configuration transaction should not be empty")
}
