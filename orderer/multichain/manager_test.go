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
	"reflect"
	"testing"
	"time"

	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/orderer/common/bootstrap/provisional"
	ordererledger "github.com/hyperledger/fabric/orderer/ledger"
	ramledger "github.com/hyperledger/fabric/orderer/ledger/ram"
	"github.com/hyperledger/fabric/orderer/localconfig"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"

	"github.com/hyperledger/fabric/msp"
	logging "github.com/op/go-logging"
	"github.com/stretchr/testify/assert"
)

var conf *config.TopLevel
var genesisBlock *cb.Block

func init() {
	conf = config.Load()
	logging.SetLevel(logging.DEBUG, "")
	genesisBlock = provisional.New(conf).GenesisBlock()
}

func NewRAMLedgerAndFactory(maxSize int) (ordererledger.Factory, ordererledger.ReadWriter) {
	rlf := ramledger.New(10)
	rl, err := rlf.GetOrCreate(provisional.TestChainID)
	if err != nil {
		panic(err)
	}
	err = rl.Append(genesisBlock)
	if err != nil {
		panic(err)
	}
	return rlf, rl
}

func NewRAMLedger(maxSize int) ordererledger.ReadWriter {
	_, rl := NewRAMLedgerAndFactory(maxSize)
	return rl
}

// Tests for a normal chain which contains 3 config transactions and other normal transactions to make sure the right one returned
func TestGetConfigTx(t *testing.T) {
	rl := NewRAMLedger(10)
	for i := 0; i < 5; i++ {
		rl.Append(ordererledger.CreateNextBlock(rl, []*cb.Envelope{makeNormalTx(provisional.TestChainID, i)}))
	}
	rl.Append(ordererledger.CreateNextBlock(rl, []*cb.Envelope{makeConfigTx(provisional.TestChainID, 5)}))
	ctx := makeConfigTx(provisional.TestChainID, 6)
	rl.Append(ordererledger.CreateNextBlock(rl, []*cb.Envelope{ctx}))

	block := ordererledger.CreateNextBlock(rl, []*cb.Envelope{makeNormalTx(provisional.TestChainID, 7)})
	block.Metadata.Metadata[cb.BlockMetadataIndex_LAST_CONFIGURATION] = utils.MarshalOrPanic(&cb.Metadata{Value: utils.MarshalOrPanic(&cb.LastConfiguration{Index: 7})})
	rl.Append(block)

	pctx := getConfigTx(rl)

	if !reflect.DeepEqual(ctx, pctx) {
		t.Fatalf("Did not select most recent config transaction")
	}
}

// Tests a chain which contains blocks with multi-transactions mixed with config txs, and a single tx which is not a config tx, none count as config blocks so nil should return
func TestGetConfigTxFailure(t *testing.T) {
	rl := NewRAMLedger(10)
	for i := 0; i < 10; i++ {
		rl.Append(ordererledger.CreateNextBlock(rl, []*cb.Envelope{
			makeNormalTx(provisional.TestChainID, i),
			makeConfigTx(provisional.TestChainID, i),
		}))
	}
	rl.Append(ordererledger.CreateNextBlock(rl, []*cb.Envelope{makeNormalTx(provisional.TestChainID, 11)}))
	defer func() {
		if recover() == nil {
			t.Fatalf("Should have panic-ed because there was no configuration tx")
		}
	}()
	getConfigTx(rl)

}

// This test essentially brings the entire system up and is ultimately what main.go will replicate
func TestNoSystemChain(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatalf("Should have panicked when starting without a system chain")
		}
	}()

	lf := ramledger.New(10)

	consenters := make(map[string]Consenter)
	consenters[conf.Genesis.OrdererType] = &mockConsenter{}

	NewManagerImpl(lf, consenters, &xxxCryptoHelper{})
}

// This test essentially brings the entire system up and is ultimately what main.go will replicate
func TestManagerImpl(t *testing.T) {
	lf, rl := NewRAMLedgerAndFactory(10)

	consenters := make(map[string]Consenter)
	consenters[conf.Genesis.OrdererType] = &mockConsenter{}

	manager := NewManagerImpl(lf, consenters, &xxxCryptoHelper{})

	_, ok := manager.GetChain("Fake")
	if ok {
		t.Errorf("Should not have found a chain that was not created")
	}

	chainSupport, ok := manager.GetChain(provisional.TestChainID)

	if !ok {
		t.Fatalf("Should have gotten chain which was initialized by ramledger")
	}

	messages := make([]*cb.Envelope, conf.Genesis.BatchSize.MaxMessageCount)
	for i := 0; i < int(conf.Genesis.BatchSize.MaxMessageCount); i++ {
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
		for i := 0; i < int(conf.Genesis.BatchSize.MaxMessageCount); i++ {
			if !reflect.DeepEqual(utils.ExtractEnvelopeOrPanic(block, i), messages[i]) {
				t.Errorf("Block contents wrong at index %d", i)
			}
		}
	case <-time.After(time.Second):
		t.Fatalf("Block 1 not produced after timeout")
	}
}

// This test makes sure that the signature filter works
func TestSignatureFilter(t *testing.T) {
	lf, rl := NewRAMLedgerAndFactory(10)

	consenters := make(map[string]Consenter)
	consenters[conf.Genesis.OrdererType] = &mockConsenter{}

	manager := NewManagerImpl(lf, consenters, &xxxCryptoHelper{})

	cs, ok := manager.GetChain(provisional.TestChainID)

	if !ok {
		t.Fatalf("Should have gotten chain which was initialized by ramledger")
	}

	messages := make([]*cb.Envelope, conf.Genesis.BatchSize.MaxMessageCount)
	for i := 0; i < int(conf.Genesis.BatchSize.MaxMessageCount); i++ {
		messages[i] = makeSignaturelessTx(provisional.TestChainID, i)
	}

	for _, message := range messages {
		cs.Enqueue(message)
	}

	// Causes the consenter thread to exit after it processes all messages
	close(cs.(*chainSupport).chain.(*mockChain).queue)

	it, _ := rl.Iterator(&ab.SeekPosition{Type: &ab.SeekPosition_Specified{Specified: &ab.SeekSpecified{Number: 1}}})
	select {
	case <-it.ReadyChan():
		// Will unblock if a block is created
		t.Fatalf("Block 1 should not have been created")
	case <-cs.(*chainSupport).chain.(*mockChain).done:
		// Will unblock once the consenter thread has exited
	}
}

// This test brings up the entire system, with the mock consenter, including the broadcasters etc. and creates a new chain
func TestNewChain(t *testing.T) {
	conf := config.Load()
	lf, rl := NewRAMLedgerAndFactory(10)

	consenters := make(map[string]Consenter)
	consenters[conf.Genesis.OrdererType] = &mockConsenter{}

	manager := NewManagerImpl(lf, consenters, &xxxCryptoHelper{})

	generator := provisional.New(conf)
	items := generator.TemplateItems()
	simpleTemplate := configtx.NewSimpleTemplate(items...)

	signer, err := msp.NewNoopMsp().GetDefaultSigningIdentity()
	assert.NoError(t, err)

	newChainID := "TestNewChain"
	newChainMessage, err := configtx.MakeChainCreationTransaction(provisional.AcceptAllPolicyKey, newChainID, signer, simpleTemplate)
	if err != nil {
		t.Fatalf("Error producing configuration transaction: %s", err)
	}

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

	messages := make([]*cb.Envelope, conf.Genesis.BatchSize.MaxMessageCount)
	for i := 0; i < int(conf.Genesis.BatchSize.MaxMessageCount); i++ {
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
		for i := 0; i < int(conf.Genesis.BatchSize.MaxMessageCount); i++ {
			if !reflect.DeepEqual(utils.ExtractEnvelopeOrPanic(block, i), messages[i]) {
				t.Errorf("Block contents wrong at index %d in new chain", i)
			}
		}
	case <-time.After(time.Second):
		t.Fatalf("Block 1 not produced after timeout on new chain")
	}
}
