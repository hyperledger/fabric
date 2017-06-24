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
	"bytes"
	"io/ioutil"
	"os"
	"testing"

	"github.com/hyperledger/fabric/common/configtx/tool/provisional"
	"github.com/hyperledger/fabric/orderer/ledger"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"

	logging "github.com/op/go-logging"
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
	if err != nil {
		t.Fatalf("Error creating temp dir: %s", err)
	}
	flf := New(name).(*fileLedgerFactory)
	fl, err := flf.GetOrCreate(provisional.TestChainID)
	if err != nil {
		panic(err)
	}
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

func TestInitialization(t *testing.T) {
	tev, fl := initialize(t)
	defer tev.tearDown()

	if fl.Height() != 1 {
		t.Fatalf("Block height should be 1")
	}
	block := ledger.GetBlock(fl, 0)
	if block == nil {
		t.Fatalf("Error retrieving genesis block")
	}
	if !bytes.Equal(block.Header.Hash(), genesisBlock.Header.Hash()) {
		t.Fatalf("Block hashes did no match")
	}
}

func TestReinitialization(t *testing.T) {
	// initialize ledger provider and a ledger for the test chain
	tev, leger1 := initialize(t)

	// make sure we cleanup at the end (delete all traces of ledgers)
	defer tev.tearDown()

	// create a block to add to the ledger
	b1 := ledger.CreateNextBlock(leger1, []*cb.Envelope{&cb.Envelope{Payload: []byte("My Data")}})

	// add the block to the ledger
	leger1.Append(b1)

	// shutdown the ledger
	leger1.blockStore.Shutdown()

	// shut down the ledger provider
	tev.shutDown()

	// re-initialize the ledger provider (not the test ledger itself!)
	provider2 := New(tev.location)

	// assert expected ledgers exist
	chains := provider2.ChainIDs()
	if len(chains) != 1 {
		t.Fatalf("Should have recovered the chain")
	}

	// get the existing test chain ledger
	ledger2, err := provider2.GetOrCreate(chains[0])
	if err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}

	fl := ledger2.(*fileLedger)
	if fl.Height() != 2 {
		t.Fatalf("Block height should be 2. Got %v", fl.Height())
	}
	block := ledger.GetBlock(fl, 1)
	if block == nil {
		t.Fatalf("Error retrieving block 1")
	}
	if !bytes.Equal(block.Header.Hash(), b1.Header.Hash()) {
		t.Fatalf("Block hashes did no match")
	}
}

func TestMultiReinitialization(t *testing.T) {
	tev, _ := initialize(t)
	defer tev.tearDown()
	tev.shutDown()
	flf := New(tev.location)

	_, err := flf.GetOrCreate("foo")
	if err != nil {
		t.Fatalf("Error creating chain")
	}

	_, err = flf.GetOrCreate("bar")
	if err != nil {
		t.Fatalf("Error creating chain")
	}
	flf.Close()
	flf = New(tev.location)
	chains := flf.ChainIDs()
	if len(chains) != 3 {
		t.Fatalf("Should have recovered the chains")
	}
}

func TestAddition(t *testing.T) {
	tev, fl := initialize(t)
	defer tev.tearDown()
	info, _ := fl.blockStore.GetBlockchainInfo()
	prevHash := info.CurrentBlockHash
	fl.Append(ledger.CreateNextBlock(fl, []*cb.Envelope{&cb.Envelope{Payload: []byte("My Data")}}))
	if fl.Height() != 2 {
		t.Fatalf("Block height should be 2")
	}
	block := ledger.GetBlock(fl, 1)
	if block == nil {
		t.Fatalf("Error retrieving genesis block")
	}
	if !bytes.Equal(block.Header.PreviousHash, prevHash) {
		t.Fatalf("Block hashes did no match")
	}
}

func TestRetrieval(t *testing.T) {
	tev, fl := initialize(t)
	defer tev.tearDown()
	fl.Append(ledger.CreateNextBlock(fl, []*cb.Envelope{&cb.Envelope{Payload: []byte("My Data")}}))
	it, num := fl.Iterator(&ab.SeekPosition{Type: &ab.SeekPosition_Oldest{}})
	if num != 0 {
		t.Fatalf("Expected genesis block iterator, but got %d", num)
	}
	signal := it.ReadyChan()
	select {
	case <-signal:
	default:
		t.Fatalf("Should be ready for block read")
	}
	block, status := it.Next()
	if status != cb.Status_SUCCESS {
		t.Fatalf("Expected to successfully read the genesis block")
	}
	if block.Header.Number != 0 {
		t.Fatalf("Expected to successfully retrieve the genesis block")
	}
	signal = it.ReadyChan()
	select {
	case <-signal:
	default:
		t.Fatalf("Should still be ready for block read")
	}
	block, status = it.Next()
	if status != cb.Status_SUCCESS {
		t.Fatalf("Expected to successfully read the second block")
	}
	if block.Header.Number != 1 {
		t.Fatalf("Expected to successfully retrieve the second block but got block number %d", block.Header.Number)
	}
}

func TestBlockedRetrieval(t *testing.T) {
	tev, fl := initialize(t)
	defer tev.tearDown()
	it, num := fl.Iterator(&ab.SeekPosition{Type: &ab.SeekPosition_Specified{Specified: &ab.SeekSpecified{Number: 1}}})
	if num != 1 {
		t.Fatalf("Expected block iterator at 1, but got %d", num)
	}
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
	if status != cb.Status_SUCCESS {
		t.Fatalf("Expected to successfully read the second block")
	}
	if block.Header.Number != 1 {
		t.Fatalf("Expected to successfully retrieve the second block")
	}
}
