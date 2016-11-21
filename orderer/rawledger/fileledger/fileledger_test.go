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

	"github.com/hyperledger/fabric/orderer/common/bootstrap/static"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
)

var genesisBlock *cb.Block

func init() {
	bootstrapper := static.New()
	var err error
	genesisBlock, err = bootstrapper.GenesisBlock()
	if err != nil {
		panic("Error intializing static bootstrap genesis block")
	}
}

type testEnv struct {
	t        *testing.T
	location string
}

func initialize(t *testing.T) (*testEnv, *fileLedger) {
	name, err := ioutil.TempDir("", "hyperledger_fabric")
	if err != nil {
		t.Fatalf("Error creating temp dir: %s", err)
	}
	_, fl := New(name, genesisBlock)
	return &testEnv{location: name, t: t}, fl.(*fileLedger)
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
	ofl.Append([]*cb.Envelope{&cb.Envelope{Payload: []byte("My Data")}}, nil)
	_, flt := New(tev.location, genesisBlock)
	fl := flt.(*fileLedger)
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

func TestAddition(t *testing.T) {
	tev, fl := initialize(t)
	defer tev.tearDown()
	prevHash := fl.lastHash
	fl.Append([]*cb.Envelope{&cb.Envelope{Payload: []byte("My Data")}}, nil)
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
	fl.Append([]*cb.Envelope{&cb.Envelope{Payload: []byte("My Data")}}, nil)
	it, num := fl.Iterator(ab.SeekInfo_OLDEST, 99)
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
	it, num := fl.Iterator(ab.SeekInfo_SPECIFIED, 1)
	if num != 1 {
		t.Fatalf("Expected block iterator at 1, but got %d", num)
	}
	signal := it.ReadyChan()
	select {
	case <-signal:
		t.Fatalf("Should not be ready for block read")
	default:
	}
	fl.Append([]*cb.Envelope{&cb.Envelope{Payload: []byte("My Data")}}, nil)
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
