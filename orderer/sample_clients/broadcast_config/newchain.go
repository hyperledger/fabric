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

package main

import (
	"github.com/hyperledger/fabric/orderer/common/bootstrap/provisional"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
)

func newChainRequest(consensusType, creationPolicy, newChainID string) *cb.Envelope {
	conf.General.OrdererType = consensusType
	genesisBlock := provisional.New(conf).GenesisBlock()
	oldGenesisTx := utils.ExtractEnvelopeOrPanic(genesisBlock, 0)
	oldGenesisTxPayload := utils.ExtractPayloadOrPanic(oldGenesisTx)
	oldConfigEnv := utils.UnmarshalConfigurationEnvelopeOrPanic(oldGenesisTxPayload.Data)

	return utils.ChainCreationConfigurationTransaction(provisional.AcceptAllPolicyKey, newChainID, oldConfigEnv)
}
