/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package genesis

import (
	"testing"

	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/assert"
)

func TestBasicSanity(t *testing.T) {
	impl := NewFactoryImpl(protoutil.NewConfigGroup())
	impl.Block("testchainid")
}

func TestForTransactionID(t *testing.T) {
	impl := NewFactoryImpl(protoutil.NewConfigGroup())
	block := impl.Block("testchainid")
	configEnv, _ := protoutil.ExtractEnvelope(block, 0)
	configEnvPayload, _ := protoutil.ExtractPayload(configEnv)
	configEnvPayloadChannelHeader, _ := protoutil.UnmarshalChannelHeader(configEnvPayload.GetHeader().ChannelHeader)
	assert.NotEmpty(t, configEnvPayloadChannelHeader.TxId, "tx_id of configuration transaction should not be empty")
}
