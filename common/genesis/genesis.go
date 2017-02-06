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
	"github.com/hyperledger/fabric/common/configtx"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
)

const (
	msgVersion = int32(1)

	// These values are fixed for the genesis block.
	lastModified = 0
	epoch        = 0
)

type Factory interface {
	Block(chainID string) (*cb.Block, error)
}

type factory struct {
	template configtx.Template
}

func NewFactoryImpl(template configtx.Template) Factory {
	return &factory{template: template}
}

func (f *factory) Block(chainID string) (*cb.Block, error) {
	items, err := f.template.Items(chainID)
	if err != nil {
		return nil, err
	}

	payloadChainHeader := utils.MakeChainHeader(cb.HeaderType_CONFIGURATION_TRANSACTION, msgVersion, chainID, epoch)
	payloadSignatureHeader := utils.MakeSignatureHeader(nil, utils.CreateNonceOrPanic())
	payloadHeader := utils.MakePayloadHeader(payloadChainHeader, payloadSignatureHeader)
	payload := &cb.Payload{Header: payloadHeader, Data: utils.MarshalOrPanic(&cb.ConfigurationEnvelope{Header: &cb.ChainHeader{ChainID: chainID}, Items: items})}
	envelope := &cb.Envelope{Payload: utils.MarshalOrPanic(payload), Signature: nil}

	block := cb.NewBlock(0, nil)
	block.Data = &cb.BlockData{Data: [][]byte{utils.MarshalOrPanic(envelope)}}
	block.Header.DataHash = block.Data.Hash()
	block.Metadata.Metadata[cb.BlockMetadataIndex_LAST_CONFIGURATION] = utils.MarshalOrPanic(&cb.Metadata{
		Value: utils.MarshalOrPanic(&cb.LastConfiguration{Index: 0}),
	})
	return block, nil
}
