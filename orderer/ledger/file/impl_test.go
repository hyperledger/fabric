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

package fileledger

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/hyperledger/fabric/common/configtx/tool/provisional"
	cl "github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/orderer/ledger"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/peer"
	logging "github.com/op/go-logging"
	"github.com/stretchr/testify/assert"
)

var genesisBlock = cb.NewBlock(0, nil)

func init() {
	logging.SetLevel(logging.DEBUG, "")
}

type testEnv struct {
	t        *testing.T
	location string
	flf      ledger.Factory
}

func initialize(t *testing.T) (*testEnv, *fileLedger) {
	name, err := ioutil.TempDir("", "hyperledger_fabric")
	assert.NoError(t, err, "Error creating temp dir: %s", err)

	flf := New(name).(*fileLedgerFactory)
	fl, err := flf.GetOrCreate(provisional.TestChainID)
	assert.NoError(t, err, "Error GetOrCreate chain")

	fl.Append(genesisBlock)
	return &testEnv{location: name, t: t, flf: flf}, fl.(*fileLedger)
}

func (tev *testEnv) tearDown() {
	tev.shutDown()
	err := os.RemoveAll(tev.location)
	if err != nil {
		tev.t.Fatalf("Error tearing down env: %s", err)
	}
}

func (tev *testEnv) shutDown() {
	tev.flf.Close()
}

type mockBlockStore struct {
	blockchainInfo             *cb.BlockchainInfo
	resultsIterator            cl.ResultsIterator
	block                      *cb.Block
	envelope                   *cb.Envelope
	txValidationCode           peer.TxValidationCode
	defaultError               error
	getBlockchainInfoError     error
	retrieveBlockByNumberError error
}

func (mbs *mockBlockStore) AddBlock(block *cb.Block) error {
	return mbs.defaultError
}

func (mbs *mockBlockStore) GetBlockchainInfo() (*cb.BlockchainInfo, error) {
	return mbs.blockchainInfo, mbs.getBlockchainInfoError
}

func (mbs *mockBlockStore) RetrieveBlocks(startNum uint64) (cl.ResultsIterator, error) {
	return mbs.resultsIterator, mbs.defaultError
}

func (mbs *mockBlockStore) RetrieveBlockByHash(blockHash []byte) (*cb.Block, error) {
	return mbs.block, mbs.defaultError
}

func (mbs *mockBlockStore) RetrieveBlockByNumber(blockNum uint64) (*cb.Block, error) {
	return mbs.block, mbs.retrieveBlockByNumberError
}

func (mbs *mockBlockStore) RetrieveTxByID(txID string) (*cb.Envelope, error) {
	return mbs.envelope, mbs.defaultError
}

func (mbs *mockBlockStore) RetrieveTxByBlockNumTranNum(blockNum uint64, tranNum uint64) (*cb.Envelope, error) {
	return mbs.envelope, mbs.defaultError
}

func (mbs *mockBlockStore) RetrieveBlockByTxID(txID string) (*cb.Block, error) {
	return mbs.block, mbs.defaultError
}

func (mbs *mockBlockStore) RetrieveTxValidationCodeByTxID(txID string) (peer.TxValidationCode, error) {
	return mbs.txValidationCode, mbs.defaultError
}

func (*mockBlockStore) Shutdown() {
}

func TestInitialization(t *testing.T) {
	tev, fl := initialize(t)
	defer tev.tearDown()

	assert.Equal(t, uint64(1), fl.Height(), "Block height should be 1")

	block := ledger.GetBlock(fl, 0)
	assert.NotNil(t, block, "Error retrieving genesis block")
	assert.Equal(t, genesisBlock.Header.Hash(), block.Header.Hash(), "Block hashes did no match")
}

func TestReinitialization(t *testing.T) {
	tev, ledger1 := initialize(t)
	defer tev.tearDown()

	// create a block to add to the ledger
	b1 := ledger.CreateNextBlock(ledger1, []*cb.Envelope{&cb.Envelope{Payload: []byte("My Data")}})

	// add the block to the ledger
	ledger1.Append(b1)

	fl, err := tev.flf.GetOrCreate(provisional.TestChainID)
	ledger1, ok := fl.(*fileLedger)
	assert.NoError(t, err, "Expected to sucessfully get test chain")
	assert.Equal(t, 1, len(tev.flf.ChainIDs()), "Exptected not new chain to be created")
	assert.True(t, ok, "Exptected type assertion to succeed")

	// shutdown the ledger
	ledger1.blockStore.Shutdown()

	// shut down the ledger provider
	tev.shutDown()

	// re-initialize the ledger provider (not the test ledger itself!)
	provider2 := New(tev.location)

	// assert expected ledgers exist
	chains := provider2.ChainIDs()
	assert.Equal(t, 1, len(chains), "Should have recovered the chain")

	// get the existing test chain ledger
	ledger2, err := provider2.GetOrCreate(chains[0])
	assert.NoError(t, err, "Unexpected error: %s", err)

	fl = ledger2.(*fileLedger)
	assert.Equal(t, uint64(2), fl.Height(), "Block height should be 2. Got %v", fl.Height())

	block := ledger.GetBlock(fl, 1)
	assert.NotNil(t, block, "Error retrieving block 1")
	assert.Equal(t, b1.Header.Hash(), block.Header.Hash(), "Block hashes did no match")
}

func TestAddition(t *testing.T) {
	tev, fl := initialize(t)
	defer tev.tearDown()
	info, _ := fl.blockStore.GetBlockchainInfo()
	prevHash := info.CurrentBlockHash
	fl.Append(ledger.CreateNextBlock(fl, []*cb.Envelope{&cb.Envelope{Payload: []byte("My Data")}}))
	assert.Equal(t, uint64(2), fl.Height(), "Block height should be 2")

	block := ledger.GetBlock(fl, 1)
	assert.NotNil(t, block, "Error retrieving genesis block")
	assert.Equal(t, prevHash, block.Header.PreviousHash, "Block hashes did no match")
}

func TestRetrieval(t *testing.T) {
	tev, fl := initialize(t)
	defer tev.tearDown()
	fl.Append(ledger.CreateNextBlock(fl, []*cb.Envelope{&cb.Envelope{Payload: []byte("My Data")}}))
	it, num := fl.Iterator(&ab.SeekPosition{Type: &ab.SeekPosition_Oldest{}})
	assert.Zero(t, num, "Expected genesis block iterator, but got %d", num)

	signal := it.ReadyChan()
	select {
	case <-signal:
	default:
		t.Fatalf("Should be ready for block read")
	}

	block, status := it.Next()
	assert.Equal(t, cb.Status_SUCCESS, status, "Expected to successfully read the genesis block")
	assert.Zero(t, block.Header.Number, "Expected to successfully retrieve the genesis block")

	signal = it.ReadyChan()
	select {
	case <-signal:
	default:
		t.Fatalf("Should still be ready for block read")
	}

	block, status = it.Next()
	assert.Equal(t, cb.Status_SUCCESS, status, "Expected to successfully read the second block")
	assert.Equal(
		t,
		uint64(1),
		block.Header.Number,
		"Expected to successfully retrieve the second block but got block number %d", block.Header.Number)
}

func TestBlockedRetrieval(t *testing.T) {
	tev, fl := initialize(t)
	defer tev.tearDown()
	it, num := fl.Iterator(&ab.SeekPosition{Type: &ab.SeekPosition_Specified{Specified: &ab.SeekSpecified{Number: 1}}})
	if num != 1 {
		t.Fatalf("Expected block iterator at 1, but got %d", num)
	}
	assert.Equal(t, uint64(1), num, "Expected block iterator at 1, but got %d", num)

	signal := it.ReadyChan()
	select {
	case <-signal:
		t.Fatalf("Should not be ready for block read")
	default:
	}

	fl.Append(ledger.CreateNextBlock(fl, []*cb.Envelope{&cb.Envelope{Payload: []byte("My Data")}}))
	select {
	case <-signal:
	default:
		t.Fatalf("Should now be ready for block read")
	}

	block, status := it.Next()
	assert.Equal(t, cb.Status_SUCCESS, status, "Expected to successfully read the second block")
	assert.Equal(
		t,
		uint64(1),
		block.Header.Number,
		"Expected to successfully retrieve the second block but got block number %d", block.Header.Number)

	go func() {
		fl.Append(ledger.CreateNextBlock(fl, []*cb.Envelope{&cb.Envelope{Payload: []byte("My Data")}}))
	}()
	select {
	case <-it.ReadyChan():
		t.Fatalf("Should not be ready for block read")
	default:
		block, status = it.Next()
		assert.Equal(t, cb.Status_SUCCESS, status, "Expected to successfully read the third block")
		assert.Equal(t, uint64(2), block.Header.Number, "Expected to successfully retrieve the third block")
	}
}

func TestBlockstoreError(t *testing.T) {
	// Since this test only ensures failed GetBlockchainInfo
	// is properly handled. We don't bother creating fully
	// legit ledgers here (without genesis block).
	{
		fl := &fileLedger{
			blockStore: &mockBlockStore{
				blockchainInfo:         nil,
				getBlockchainInfoError: fmt.Errorf("Error getting blockchain info"),
			},
			signal: make(chan struct{}),
		}
		assert.Panics(
			t,
			func() {
				fl.Iterator(&ab.SeekPosition{Type: &ab.SeekPosition_Newest{}})
			},
			"Expected Iterator() to panic if blockstore operation fails")

		assert.Panics(
			t,
			func() { fl.Height() },
			"Expected Height() to panic if blockstore operation fails ")
	}

	{
		fl := &fileLedger{
			blockStore: &mockBlockStore{
				blockchainInfo:             &cb.BlockchainInfo{Height: uint64(1)},
				getBlockchainInfoError:     nil,
				retrieveBlockByNumberError: fmt.Errorf("Error retrieving block by number"),
			},
			signal: make(chan struct{}),
		}
		it, _ := fl.Iterator(&ab.SeekPosition{Type: &ab.SeekPosition_Specified{Specified: &ab.SeekSpecified{Number: 42}}})
		assert.IsType(
			t,
			&ledger.NotFoundErrorIterator{},
			it,
			"Expected Not Found Error if seek number is greater than ledger height")

		it, _ = fl.Iterator(&ab.SeekPosition{Type: &ab.SeekPosition_Oldest{}})
		_, status := it.Next()
		assert.Equal(t, cb.Status_SERVICE_UNAVAILABLE, status, "Expected service unavailable error")
	}
}
