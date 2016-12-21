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

	"github.com/hyperledger/fabric/orderer/common/bootstrap/provisional"
	"github.com/hyperledger/fabric/orderer/localconfig"
	"github.com/hyperledger/fabric/orderer/rawledger/ramledger"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"
)

var conf *config.TopLevel
var genesisBlock *cb.Block

func init() {
	conf = config.Load()
	genesisBlock = provisional.New(conf).GenesisBlock()
}

// TODO move to util
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
		Payload: utils.MarshalOrPanic(payload),
	}
}

// Tests for a normal chain which contains 3 config transactions and other normal transactions to make sure the right one returned
func TestGetConfigTx(t *testing.T) {
	_, rl := ramledger.New(10, genesisBlock)
	for i := 0; i < 5; i++ {
		rl.Append([]*cb.Envelope{makeNormalTx(provisional.TestChainID, i)}, nil)
	}
	rl.Append([]*cb.Envelope{makeConfigTx(provisional.TestChainID, 5)}, nil)
	ctx := makeConfigTx(provisional.TestChainID, 6)
	rl.Append([]*cb.Envelope{ctx}, nil)
	rl.Append([]*cb.Envelope{makeNormalTx(provisional.TestChainID, 7)}, nil)

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
			makeNormalTx(provisional.TestChainID, i),
			makeConfigTx(provisional.TestChainID, i),
		}, nil)
	}
	rl.Append([]*cb.Envelope{makeNormalTx(provisional.TestChainID, 11)}, nil)
	pctx := getConfigTx(rl)

	if pctx != nil {
		t.Fatalf("Should not have found a configuration tx")
	}
}

// This test essentially brings the entire system up and is ultimately what main.go will replicate
func TestManagerImpl(t *testing.T) {
	lf, rl := ramledger.New(10, genesisBlock)

	consenters := make(map[string]Consenter)
	consenters[conf.General.OrdererType] = &mockConsenter{}

	manager := NewManagerImpl(lf, consenters)

	_, ok := manager.GetChain("Fake")
	if ok {
		t.Errorf("Should not have found a chain that was not created")
	}

	chainSupport, ok := manager.GetChain(provisional.TestChainID)

	if !ok {
		t.Fatalf("Should have gotten chain which was initialized by ramledger")
	}

	messages := make([]*cb.Envelope, conf.General.BatchSize.MaxMessageCount)
	for i := 0; i < int(conf.General.BatchSize.MaxMessageCount); i++ {
		messages[i] = makeNormalTx(provisional.TestChainID, i)
	}

	for _, message := range messages {
		chainSupport.Enqueue(message)
	}

	it, _ := rl.Iterator(&ab.SeekPosition{Type: &ab.SeekPosition_Specified{Specified: &ab.SeekSpecified{Number: 1}}})
	select {
	case <-it.ReadyChan():
		block, status := it.Next()
		if status != cb.Status_SUCCESS {
			t.Fatalf("Could not retrieve block")
		}
		for i := 0; i < int(conf.General.BatchSize.MaxMessageCount); i++ {
			if !reflect.DeepEqual(utils.ExtractEnvelopeOrPanic(block, i), messages[i]) {
				t.Errorf("Block contents wrong at index %d", i)
			}
		}
	case <-time.After(time.Second):
		t.Fatalf("Block 1 not produced after timeout")
	}
}

// This test brings up the entire system, with the mock consenter, including the broadcasters etc. and creates a new chain
func TestNewChain(t *testing.T) {
	conf := config.Load()
	lf, rl := ramledger.New(10, genesisBlock)

	consenters := make(map[string]Consenter)
	consenters[conf.General.OrdererType] = &mockConsenter{}

	manager := NewManagerImpl(lf, consenters)

	oldGenesisTx := utils.ExtractEnvelopeOrPanic(genesisBlock, 0)
	oldGenesisTxPayload := utils.ExtractPayloadOrPanic(oldGenesisTx)
	oldConfigEnv := utils.UnmarshalConfigurationEnvelopeOrPanic(oldGenesisTxPayload.Data)

	newChainID := "TestNewChain"
	newChainMessage := utils.ChainCreationConfigurationTransaction(provisional.AcceptAllPolicyKey, newChainID, oldConfigEnv)

	status := manager.ProposeChain(newChainMessage)

	if status != cb.Status_SUCCESS {
		t.Fatalf("Error submitting chain creation request")
	}

	it, _ := rl.Iterator(&ab.SeekPosition{Type: &ab.SeekPosition_Specified{Specified: &ab.SeekSpecified{Number: 1}}})
	select {
	case <-it.ReadyChan():
		block, status := it.Next()
		if status != cb.Status_SUCCESS {
			t.Fatalf("Could not retrieve block")
		}
		if len(block.Data.Data) != 1 {
			t.Fatalf("Should have had only one message in the orderer transaction block")
		}
		genesisConfigTx := utils.UnmarshalEnvelopeOrPanic(utils.UnmarshalPayloadOrPanic(utils.ExtractEnvelopeOrPanic(block, 0).Payload).Data)
		if !reflect.DeepEqual(genesisConfigTx, newChainMessage) {
			t.Errorf("Orderer config block contains wrong transaction, expected %v got %v", genesisConfigTx, newChainMessage)
		}
	case <-time.After(time.Second):
		t.Fatalf("Block 1 not produced after timeout in system chain")
	}

	chainSupport, ok := manager.GetChain(newChainID)

	if !ok {
		t.Fatalf("Should have gotten new chain which was created")
	}

	messages := make([]*cb.Envelope, conf.General.BatchSize.MaxMessageCount)
	for i := 0; i < int(conf.General.BatchSize.MaxMessageCount); i++ {
		messages[i] = makeNormalTx(newChainID, i)
	}

	for _, message := range messages {
		chainSupport.Enqueue(message)
	}

	it, _ = chainSupport.Reader().Iterator(&ab.SeekPosition{Type: &ab.SeekPosition_Specified{Specified: &ab.SeekSpecified{Number: 0}}})
	select {
	case <-it.ReadyChan():
		block, status := it.Next()
		if status != cb.Status_SUCCESS {
			t.Fatalf("Could not retrieve new chain genesis block")
		}
		if len(block.Data.Data) != 1 {
			t.Fatalf("Should have had only one message in the new genesis block")
		}
		genesisConfigTx := utils.ExtractEnvelopeOrPanic(block, 0)
		if !reflect.DeepEqual(genesisConfigTx, newChainMessage) {
			t.Errorf("Genesis block contains wrong transaction, expected %v got %v", genesisConfigTx, newChainMessage)
		}
	case <-time.After(time.Second):
		t.Fatalf("Block 1 not produced after timeout in system chain")
	}

	select {
	case <-it.ReadyChan():
		block, status := it.Next()
		if status != cb.Status_SUCCESS {
			t.Fatalf("Could not retrieve block on new chain")
		}
		for i := 0; i < int(conf.General.BatchSize.MaxMessageCount); i++ {
			if !reflect.DeepEqual(utils.ExtractEnvelopeOrPanic(block, i), messages[i]) {
				t.Errorf("Block contents wrong at index %d in new chain", i)
			}
		}
	case <-time.After(time.Second):
		t.Fatalf("Block 1 not produced after timeout on new chain")
	}
}
