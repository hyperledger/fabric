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

package static

import (
	"fmt"

	"github.com/hyperledger/fabric/core/crypto/primitives"
	"github.com/hyperledger/fabric/orderer/common/bootstrap"
	"github.com/hyperledger/fabric/orderer/common/cauthdsl"
	"github.com/hyperledger/fabric/orderer/common/configtx"
	"github.com/hyperledger/fabric/orderer/common/util"
	cb "github.com/hyperledger/fabric/protos/common"
)

const msgVersion = int32(1)

type bootstrapper struct {
	chainID []byte
}

// New returns a new static bootstrap helper.
func New() bootstrap.Helper {
	chainID, err := primitives.GetRandomBytes(16)
	if err != nil {
		panic(fmt.Errorf("Cannot generate random chain ID: %s", err))
	}
	return &bootstrapper{chainID}
}

// GenesisBlock returns the genesis block to be used for bootstrapping
func (b *bootstrapper) GenesisBlock() (*cb.Block, error) {
	// Lock down the default modification policy to prevent any further policy modifications
	configItemKey := configtx.DefaultModificationPolicyID
	configItemValue := util.MarshalOrPanic(util.MakePolicyOrPanic(cauthdsl.RejectAllPolicy))
	modPolicy := configtx.DefaultModificationPolicyID

	lastModified := uint64(0)
	epoch := uint64(0)
	configItemChainHeader := util.MakeChainHeader(cb.HeaderType_CONFIGURATION_ITEM, msgVersion, b.chainID, epoch)
	configItem := util.MakeConfigurationItem(configItemChainHeader, cb.ConfigurationItem_Policy, lastModified, modPolicy, configItemKey, configItemValue)
	signedConfigItem := &cb.SignedConfigurationItem{ConfigurationItem: util.MarshalOrPanic(configItem), Signatures: nil}

	configEnvelope := util.MakeConfigurationEnvelope(signedConfigItem)
	payloadChainHeader := util.MakeChainHeader(cb.HeaderType_CONFIGURATION_TRANSACTION, configItemChainHeader.Version, b.chainID, epoch)
	payloadSignatureHeader := util.MakeSignatureHeader(nil, util.CreateNonceOrPanic())
	payloadHeader := util.MakePayloadHeader(payloadChainHeader, payloadSignatureHeader)
	payload := &cb.Payload{Header: payloadHeader, Data: util.MarshalOrPanic(configEnvelope)}
	envelope := &cb.Envelope{Payload: util.MarshalOrPanic(payload), Signature: nil}

	blockData := &cb.BlockData{Data: [][]byte{util.MarshalOrPanic(envelope)}}

	return &cb.Block{
		Header: &cb.BlockHeader{
			Number:       0,
			PreviousHash: nil,
			DataHash:     blockData.Hash(),
		},
		Data:     blockData,
		Metadata: nil,
	}, nil
}
