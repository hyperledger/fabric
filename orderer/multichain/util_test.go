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
	"fmt"

	"github.com/hyperledger/fabric/orderer/common/blockcutter"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
)

type mockConsenter struct {
}

func (mc *mockConsenter) HandleChain(support ConsenterSupport, metadata *cb.Metadata) (Chain, error) {
	return &mockChain{
		queue:    make(chan *cb.Envelope),
		cutter:   support.BlockCutter(),
		support:  support,
		metadata: metadata,
		done:     make(chan struct{}),
	}, nil
}

type mockChain struct {
	queue    chan *cb.Envelope
	cutter   blockcutter.Receiver
	support  ConsenterSupport
	metadata *cb.Metadata
	done     chan struct{}
}

func (mch *mockChain) Enqueue(env *cb.Envelope) bool {
	mch.queue <- env
	return true
}

func (mch *mockChain) Start() {
	go func() {
		defer close(mch.done)
		for {
			msg, ok := <-mch.queue
			if !ok {
				return
			}
			batches, committers, _ := mch.cutter.Ordered(msg)
			for i, batch := range batches {
				block := mch.support.CreateNextBlock(batch)
				mch.support.WriteBlock(block, committers[i], nil)
			}
		}
	}()
}

func (mch *mockChain) Halt() {
	close(mch.queue)
}

type mockLedgerWriter struct {
}

func (mlw *mockLedgerWriter) Append(blockContents []*cb.Envelope, metadata [][]byte) *cb.Block {
	logger.Debugf("Committed block")
	return nil
}

func makeConfigTx(chainID string, i int) *cb.Envelope {
	return makeConfigTxWithItems(chainID, &cb.ConfigurationItem{
		Value: []byte(fmt.Sprintf("%d", i)),
	})
}

func makeConfigTxWithItems(chainID string, items ...*cb.ConfigurationItem) *cb.Envelope {
	signedItems := make([]*cb.SignedConfigurationItem, len(items))
	for i, item := range items {
		signedItems[i] = &cb.SignedConfigurationItem{
			ConfigurationItem: utils.MarshalOrPanic(item),
		}
	}

	payload := &cb.Payload{
		Header: &cb.Header{
			ChainHeader: &cb.ChainHeader{
				Type:    int32(cb.HeaderType_CONFIGURATION_TRANSACTION),
				ChainID: chainID,
			},
			SignatureHeader: &cb.SignatureHeader{},
		},
		Data: utils.MarshalOrPanic(&cb.ConfigurationEnvelope{
			Items: signedItems,
			Header: &cb.ChainHeader{
				Type:    int32(cb.HeaderType_CONFIGURATION_ITEM),
				ChainID: chainID,
			},
		}),
	}
	return &cb.Envelope{
		Payload: utils.MarshalOrPanic(payload),
	}
}

func makeNormalTx(chainID string, i int) *cb.Envelope {
	payload := &cb.Payload{
		Header: &cb.Header{
			ChainHeader: &cb.ChainHeader{
				Type:    int32(cb.HeaderType_ENDORSER_TRANSACTION),
				ChainID: chainID,
			},
			SignatureHeader: &cb.SignatureHeader{},
		},
		Data: []byte(fmt.Sprintf("%d", i)),
	}
	return &cb.Envelope{
		Payload: utils.MarshalOrPanic(payload),
	}
}

func makeSignaturelessTx(chainID string, i int) *cb.Envelope {
	payload := &cb.Payload{
		Header: &cb.Header{
			ChainHeader: &cb.ChainHeader{
				Type:    int32(cb.HeaderType_ENDORSER_TRANSACTION),
				ChainID: chainID,
			},
		},
		Data: []byte(fmt.Sprintf("%d", i)),
	}
	return &cb.Envelope{
		Payload: utils.MarshalOrPanic(payload),
	}
}
