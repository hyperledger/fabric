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
	genesisconfig "github.com/hyperledger/fabric/common/configtx/tool/localconfig"
	"github.com/hyperledger/fabric/common/configtx/tool/provisional"
	mockcrypto "github.com/hyperledger/fabric/common/mocks/crypto"
	"github.com/hyperledger/fabric/orderer/ledger"
	ramledger "github.com/hyperledger/fabric/orderer/ledger/ram"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"

	"errors"

	logging "github.com/op/go-logging"
	"github.com/stretchr/testify/assert"
)

var conf *genesisconfig.Profile
var genesisBlock = cb.NewBlock(0, nil) // *cb.Block

func init() {
	conf = genesisconfig.Load(genesisconfig.SampleInsecureProfile)
	logging.SetLevel(logging.DEBUG, "")
	genesisBlock = provisional.New(conf).GenesisBlock()
}

func mockCrypto() *mockCryptoHelper {
	return &mockCryptoHelper{LocalSigner: mockcrypto.FakeLocalSigner}
}

type mockCryptoHelper struct {
	*mockcrypto.LocalSigner
}

func (mch mockCryptoHelper) VerifySignature(sd *cb.SignedData) error {
	return nil
}

func mockCryptoRejector() *mockCryptoRejectorHelper {
	return &mockCryptoRejectorHelper{LocalSigner: mockcrypto.FakeLocalSigner}
}

type mockCryptoRejectorHelper struct {
	*mockcrypto.LocalSigner
}

func (mch mockCryptoRejectorHelper) VerifySignature(sd *cb.SignedData) error {
	return errors.New("Nope")
}

func NewRAMLedgerAndFactory(maxSize int) (ledger.Factory, ledger.ReadWriter) {
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

func NewRAMLedger(maxSize int) ledger.ReadWriter {
	_, rl := NewRAMLedgerAndFactory(maxSize)
	return rl
}

// Tests for a normal chain which contains 3 config transactions and other normal transactions to make sure the right one returned
func TestGetConfigTx(t *testing.T) {
	rl := NewRAMLedger(10)
	for i := 0; i < 5; i++ {
		rl.Append(ledger.CreateNextBlock(rl, []*cb.Envelope{makeNormalTx(provisional.TestChainID, i)}))
	}
	rl.Append(ledger.CreateNextBlock(rl, []*cb.Envelope{makeConfigTx(provisional.TestChainID, 5)}))
	ctx := makeConfigTx(provisional.TestChainID, 6)
	rl.Append(ledger.CreateNextBlock(rl, []*cb.Envelope{ctx}))

	block := ledger.CreateNextBlock(rl, []*cb.Envelope{makeNormalTx(provisional.TestChainID, 7)})
	block.Metadata.Metadata[cb.BlockMetadataIndex_LAST_CONFIG] = utils.MarshalOrPanic(&cb.Metadata{Value: utils.MarshalOrPanic(&cb.LastConfig{Index: 7})})
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
		rl.Append(ledger.CreateNextBlock(rl, []*cb.Envelope{
			makeNormalTx(provisional.TestChainID, i),
			makeConfigTx(provisional.TestChainID, i),
		}))
	}
	rl.Append(ledger.CreateNextBlock(rl, []*cb.Envelope{makeNormalTx(provisional.TestChainID, 11)}))
	defer func() {
		if recover() == nil {
			t.Fatalf("Should have panic-ed because there was no config tx")
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
	consenters[conf.Orderer.OrdererType] = &mockConsenter{}

	NewManagerImpl(lf, consenters, mockCrypto())
}

// This test essentially brings the entire system up and is ultimately what main.go will replicate
func TestManagerImpl(t *testing.T) {
	lf, rl := NewRAMLedgerAndFactory(10)

	consenters := make(map[string]Consenter)
	consenters[conf.Orderer.OrdererType] = &mockConsenter{}

	manager := NewManagerImpl(lf, consenters, mockCrypto())

	_, ok := manager.GetChain("Fake")
	if ok {
		t.Errorf("Should not have found a chain that was not created")
	}

	chainSupport, ok := manager.GetChain(provisional.TestChainID)

	if !ok {
		t.Fatalf("Should have gotten chain which was initialized by ramledger")
	}

	messages := make([]*cb.Envelope, conf.Orderer.BatchSize.MaxMessageCount)
	for i := 0; i < int(conf.Orderer.BatchSize.MaxMessageCount); i++ {
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
		for i := 0; i < int(conf.Orderer.BatchSize.MaxMessageCount); i++ {
			if !reflect.DeepEqual(utils.ExtractEnvelopeOrPanic(block, i), messages[i]) {
				t.Errorf("Block contents wrong at index %d", i)
			}
		}
	case <-time.After(time.Second):
		t.Fatalf("Block 1 not produced after timeout")
	}
}

/*
// This test makes sure that the signature filter works
func TestSignatureFilter(t *testing.T) {
	lf, rl := NewRAMLedgerAndFactory(10)

	consenters := make(map[string]Consenter)
	consenters[conf.Orderer.OrdererType] = &mockConsenter{}

	manager := NewManagerImpl(lf, consenters, mockCryptoRejector())

	cs, ok := manager.GetChain(provisional.TestChainID)

	if !ok {
		t.Fatalf("Should have gotten chain which was initialized by ramledger")
	}

	messages := make([]*cb.Envelope, conf.Orderer.BatchSize.MaxMessageCount)
	for i := 0; i < int(conf.Orderer.BatchSize.MaxMessageCount); i++ {
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
*/

// This test brings up the entire system, with the mock consenter, including the broadcasters etc. and creates a new chain
func TestNewChain(t *testing.T) {
	lf, rl := NewRAMLedgerAndFactory(10)

	consenters := make(map[string]Consenter)
	consenters[conf.Orderer.OrdererType] = &mockConsenter{}

	manager := NewManagerImpl(lf, consenters, mockCrypto())

	newChainID := "TestNewChain"

	configEnv, err := configtx.NewChainCreationTemplate(provisional.AcceptAllPolicyKey, provisional.New(conf).ChannelTemplate()).Envelope(newChainID)
	if err != nil {
		t.Fatalf("Error constructing configtx")
	}
	ingressTx := makeConfigTxFromConfigUpdateEnvelope(newChainID, configEnv)
	wrapped := wrapConfigTx(ingressTx)

	chainSupport, ok := manager.GetChain(manager.SystemChannelID())
	assert.True(t, ok, "Could not find system channel")
	chainSupport.Enqueue(wrapped)

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

		assert.Equal(t, wrapped, utils.UnmarshalEnvelopeOrPanic(block.Data.Data[0]), "Orderer config block contains wrong transaction")
	case <-time.After(time.Second):
		t.Fatalf("Block 1 not produced after timeout in system chain")
	}

	chainSupport, ok = manager.GetChain(newChainID)

	if !ok {
		t.Fatalf("Should have gotten new chain which was created")
	}

	messages := make([]*cb.Envelope, conf.Orderer.BatchSize.MaxMessageCount)
	for i := 0; i < int(conf.Orderer.BatchSize.MaxMessageCount); i++ {
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

		assert.Equal(t, ingressTx, utils.UnmarshalEnvelopeOrPanic(block.Data.Data[0]), "Genesis block contains wrong transaction")
	case <-time.After(time.Second):
		t.Fatalf("Block 1 not produced after timeout in system chain")
	}

	select {
	case <-it.ReadyChan():
		block, status := it.Next()
		if status != cb.Status_SUCCESS {
			t.Fatalf("Could not retrieve block on new chain")
		}
		for i := 0; i < int(conf.Orderer.BatchSize.MaxMessageCount); i++ {
			if !reflect.DeepEqual(utils.ExtractEnvelopeOrPanic(block, i), messages[i]) {
				t.Errorf("Block contents wrong at index %d in new chain", i)
			}
		}
	case <-time.After(time.Second):
		t.Fatalf("Block 1 not produced after timeout on new chain")
	}
}
