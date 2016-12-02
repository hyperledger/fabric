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
	"reflect"
	"testing"
	"time"

	"github.com/hyperledger/fabric/orderer/common/bootstrap/static"
	"github.com/hyperledger/fabric/orderer/common/util"
	"github.com/hyperledger/fabric/orderer/rawledger/ramledger"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
)

var genesisBlock *cb.Block

func init() {
	var err error
	genesisBlock, err = static.New().GenesisBlock()
	if err != nil {
		panic(err)
	}
}

func makeNormalTx(chainID string, i int) *cb.Envelope {
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
		Payload: util.MarshalOrPanic(payload),
	}
}

func makeConfigTx(chainID string, i int) *cb.Envelope {
	payload := &cb.Payload{
		Header: &cb.Header{
			ChainHeader: &cb.ChainHeader{
				Type:    int32(cb.HeaderType_CONFIGURATION_TRANSACTION),
				ChainID: chainID,
			},
		},
		Data: util.MarshalOrPanic(&cb.ConfigurationEnvelope{
			Items: []*cb.SignedConfigurationItem{&cb.SignedConfigurationItem{
				ConfigurationItem: util.MarshalOrPanic(&cb.ConfigurationItem{
					Value: []byte(fmt.Sprintf("%d", i)),
				}),
			}},
		}),
	}
	return &cb.Envelope{
		Payload: util.MarshalOrPanic(payload),
	}
}

// Tests for a normal chain which contains 3 config transactions and other normal transactions to make sure the right one returned
func TestGetConfigTx(t *testing.T) {
	_, rl := ramledger.New(10, genesisBlock)
	for i := 0; i < 5; i++ {
		rl.Append([]*cb.Envelope{makeNormalTx(static.TestChainID, i)}, nil)
	}
	rl.Append([]*cb.Envelope{makeConfigTx(static.TestChainID, 5)}, nil)
	ctx := makeConfigTx(static.TestChainID, 6)
	rl.Append([]*cb.Envelope{ctx}, nil)
	rl.Append([]*cb.Envelope{makeNormalTx(static.TestChainID, 7)}, nil)

	pctx := getConfigTx(rl)

	if !reflect.DeepEqual(ctx, pctx) {
		t.Fatalf("Did not select most recent config transaction")
	}
}

// Tests a chain which contains blocks with multi-transactions mixed with config txs, and a single tx which is not a config tx, none count as config blocks so nil should return
func TestGetConfigTxFailure(t *testing.T) {
	_, rl := ramledger.New(10, genesisBlock)
	for i := 0; i < 10; i++ {
		rl.Append([]*cb.Envelope{
			makeNormalTx(static.TestChainID, i),
			makeConfigTx(static.TestChainID, i),
		}, nil)
	}
	rl.Append([]*cb.Envelope{makeNormalTx(static.TestChainID, 11)}, nil)
	pctx := getConfigTx(rl)

	if pctx != nil {
		t.Fatalf("Should not have found a configuration tx")
	}
}

// This test essentially brings the entire system up and is ultimately what main.go will replicate
func TestManagerImpl(t *testing.T) {
	lf, rl := ramledger.New(10, genesisBlock)

	consenters := make(map[string]Consenter)
	consenters["solo"] = &mockConsenter{}

	manager := NewManagerImpl(lf, consenters)

	_, ok := manager.GetChain("Fake")
	if ok {
		t.Errorf("Should not have found a chain that was not created")
	}

	chainSupport, ok := manager.GetChain(static.TestChainID)

	if !ok {
		t.Fatalf("Should have gotten chain which was initialized by ramledger")
	}

	messages := make([]*cb.Envelope, XXXBatchSize)
	for i := 0; i < XXXBatchSize; i++ {
		messages[i] = makeNormalTx(static.TestChainID, i)
	}

	for _, message := range messages {
		chainSupport.Chain().Enqueue(message)
	}

	it, _ := rl.Iterator(ab.SeekInfo_SPECIFIED, 1)
	select {
	case <-it.ReadyChan():
		block, status := it.Next()
		if status != cb.Status_SUCCESS {
			t.Fatalf("Could not retrieve block")
		}
		for i := 0; i < XXXBatchSize; i++ {
			if !reflect.DeepEqual(util.ExtractEnvelopeOrPanic(block, i), messages[i]) {
				t.Errorf("Block contents wrong at index %d", i)
			}
		}
	case <-time.After(time.Second):
		t.Fatalf("Block 1 not produced after timeout")
	}
}
