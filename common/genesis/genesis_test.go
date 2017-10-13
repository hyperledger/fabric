/*
Copyright IBM Corp. 2017 All Rights Reserved.

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

package genesis

import (
	"testing"

	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/stretchr/testify/assert"
)

func TestBasicSanity(t *testing.T) {
	impl := NewFactoryImpl(cb.NewConfigGroup())
	_, err := impl.Block("testchainid")
	assert.NoError(t, err, "Basic sanity fails")
}

func TestForTransactionID(t *testing.T) {
	impl := NewFactoryImpl(cb.NewConfigGroup())
	block, _ := impl.Block("testchainid")
	configEnv, _ := utils.ExtractEnvelope(block, 0)
	configEnvPayload, _ := utils.ExtractPayload(configEnv)
	configEnvPayloadChannelHeader, _ := utils.UnmarshalChannelHeader(configEnvPayload.GetHeader().ChannelHeader)
	assert.NotEmpty(t, configEnvPayloadChannelHeader.TxId, "tx_id of configuration transaction should not be empty")
}
