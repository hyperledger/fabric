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

package jsonledger

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
}

func initialize(t *testing.T) (*testEnv, *jsonLedger) {
	name, err := ioutil.TempDir("", "hyperledger_fabric")
	if err != nil {
		t.Fatalf("Error creating temp dir: %s", err)
	}
	flf := New(name).(*jsonLedgerFactory)
	fl, err := flf.GetOrCreate(provisional.TestChainID)
	if err != nil {
		panic(err)
	}

	fl.Append(genesisBlock)
	return &testEnv{location: name, t: t}, fl.(*jsonLedger)
}

func (tev *testEnv) tearDown() {
	err := os.RemoveAll(tev.location)
	if err != nil {
		tev.t.Fatalf("Error tearing down env: %s", err)
	}
}

func TestInitialization(t *testing.T) {
	tev, fl := initialize(t)
	defer tev.tearDown()
	if fl.height != 1 {
		t.Fatalf("Block height should be 1")
	}
	block, found := fl.readBlock(0)
	if block == nil || !found {
		t.Fatalf("Error retrieving genesis block")
	}
	if !bytes.Equal(block.Header.Hash(), fl.lastHash) {
		t.Fatalf("Block hashes did no match")
	}
}

func TestReinitialization(t *testing.T) {
	tev, ofl := initialize(t)
	defer tev.tearDown()
	ofl.Append(ledger.CreateNextBlock(ofl, []*cb.Envelope{&cb.Envelope{Payload: []byte("My Data")}}))
	flf := New(tev.location)
	chains := flf.ChainIDs()
	if len(chains) != 1 {
		t.Fatalf("Should have recovered the chain")
	}

	tfl, err := flf.GetOrCreate(chains[0])
	if err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}

	fl := tfl.(*jsonLedger)
	if fl.height != 2 {
		t.Fatalf("Block height should be 2")
	}
	block, found := fl.readBlock(1)
	if block == nil || !found {
		t.Fatalf("Error retrieving block 1")
	}
	if !bytes.Equal(block.Header.Hash(), fl.lastHash) {
		t.Fatalf("Block hashes did no match")
	}
}

func TestMultiReinitialization(t *testing.T) {
	tev, _ := initialize(t)
	defer tev.tearDown()
	flf := New(tev.location)

	_, err := flf.GetOrCreate("foo")
	if err != nil {
		t.Fatalf("Error creating chain")
	}

	_, err = flf.GetOrCreate("bar")
	if err != nil {
		t.Fatalf("Error creating chain")
	}

	flf = New(tev.location)
	chains := flf.ChainIDs()
	if len(chains) != 3 {
		t.Fatalf("Should have recovered the chains")
	}
}

func TestAddition(t *testing.T) {
	tev, fl := initialize(t)
	defer tev.tearDown()
	prevHash := fl.lastHash
	fl.Append(ledger.CreateNextBlock(fl, []*cb.Envelope{&cb.Envelope{Payload: []byte("My Data")}}))
	if fl.height != 2 {
		t.Fatalf("Block height should be 2")
	}
	block, found := fl.readBlock(1)
	if block == nil || !found {
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
