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

	ab "github.com/hyperledger/fabric/orderer/atomicbroadcast"
	"github.com/hyperledger/fabric/orderer/common/bootstrap/static"
)

var genesisBlock *ab.Block

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
	name, err := ioutil.TempDir("", "hyperledger")
	if err != nil {
		t.Fatalf("Error creating temp dir: %s", err)
	}
	return &testEnv{location: name, t: t}, New(name, genesisBlock).(*fileLedger)
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
	if !bytes.Equal(block.Hash(), fl.lastHash) {
		t.Fatalf("Block hashes did no match")
	}
}

func TestReinitialization(t *testing.T) {
	tev, ofl := initialize(t)
	defer tev.tearDown()
	ofl.Append([]*ab.BroadcastMessage{&ab.BroadcastMessage{Data: []byte("My Data")}}, nil)
	fl := New(tev.location, genesisBlock).(*fileLedger)
	if fl.height != 2 {
		t.Fatalf("Block height should be 2")
	}
	block, found := fl.readBlock(1)
	if block == nil || !found {
		t.Fatalf("Error retrieving block 1")
	}
	if !bytes.Equal(block.Hash(), fl.lastHash) {
		t.Fatalf("Block hashes did no match")
	}
}

func TestAddition(t *testing.T) {
	tev, fl := initialize(t)
	defer tev.tearDown()
	prevHash := fl.lastHash
	fl.Append([]*ab.BroadcastMessage{&ab.BroadcastMessage{Data: []byte("My Data")}}, nil)
	if fl.height != 2 {
		t.Fatalf("Block height should be 2")
	}
	block, found := fl.readBlock(1)
	if block == nil || !found {
		t.Fatalf("Error retrieving genesis block")
	}
	if !bytes.Equal(block.PrevHash, prevHash) {
		t.Fatalf("Block hashes did no match")
	}
}

func TestRetrieval(t *testing.T) {
	tev, fl := initialize(t)
	defer tev.tearDown()
	fl.Append([]*ab.BroadcastMessage{&ab.BroadcastMessage{Data: []byte("My Data")}}, nil)
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
	if status != ab.Status_SUCCESS {
		t.Fatalf("Expected to successfully read the genesis block")
	}
	if block.Number != 0 {
		t.Fatalf("Expected to successfully retrieve the genesis block")
	}
	signal = it.ReadyChan()
	select {
	case <-signal:
	default:
		t.Fatalf("Should still be ready for block read")
	}
	block, status = it.Next()
	if status != ab.Status_SUCCESS {
		t.Fatalf("Expected to successfully read the second block")
	}
	if block.Number != 1 {
		t.Fatalf("Expected to successfully retrieve the second block but got block number %d", block.Number)
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
	fl.Append([]*ab.BroadcastMessage{&ab.BroadcastMessage{Data: []byte("My Data")}}, nil)
	select {
	case <-signal:
	default:
		t.Fatalf("Should now be ready for block read")
	}
	block, status := it.Next()
	if status != ab.Status_SUCCESS {
		t.Fatalf("Expected to successfully read the second block")
	}
	if block.Number != 1 {
		t.Fatalf("Expected to successfully retrieve the second block")
	}
}
