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
	"github.com/hyperledger/fabric/orderer/common/blockcutter"
	"github.com/hyperledger/fabric/orderer/common/configtx"
	"github.com/hyperledger/fabric/orderer/rawledger"
	cb "github.com/hyperledger/fabric/protos/common"
)

type mockConsenter struct {
}

func (mc *mockConsenter) HandleChain(configManager configtx.Manager, cutter blockcutter.Receiver, rl rawledger.Writer, metadata []byte) Chain {
	return &mockChain{
		queue:  make(chan *cb.Envelope),
		ledger: rl,
		cutter: cutter,
	}
}

type mockChain struct {
	queue  chan *cb.Envelope
	ledger rawledger.Writer
	cutter blockcutter.Receiver
}

func (mch *mockChain) Enqueue(env *cb.Envelope) bool {
	mch.queue <- env
	return true
}

func (mch *mockChain) Start() {
	go func() {
		for {
			msg, ok := <-mch.queue
			if !ok {
				return
			}
			batches, _ := mch.cutter.Ordered(msg)
			for _, batch := range batches {
				mch.ledger.Append(batch, nil)
			}
		}
	}()
}

func (mch *mockChain) Halt() {
	close(mch.queue)
}
