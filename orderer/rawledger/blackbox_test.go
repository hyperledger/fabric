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

package rawledger_test

import (
	"bytes"
	"reflect"
	"testing"

	. "github.com/hyperledger/fabric/orderer/rawledger"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
)

type ledgerTestable interface {
	Initialize() (ledgerTestFactory, error)
	Name() string
}

type ledgerTestFactory interface {
	New() (Factory, ReadWriter)
	Destroy() error
	Persistent() bool
}

var testables []ledgerTestable

func getBlock(number uint64, li ReadWriter) *cb.Block {
	i, _ := li.Iterator(ab.SeekInfo_SPECIFIED, number)
	select {
	case <-i.ReadyChan():
		block, status := i.Next()
		if status != cb.Status_SUCCESS {
			return nil
		}
		return block
	default:
		return nil
	}
}

func allTest(t *testing.T, test func(ledgerTestFactory, *testing.T)) {
	for _, lt := range testables {

		t.Log("Running test for", lt.Name())

		func() {
			lf, err := lt.Initialize()
			if err != nil {
				t.Fatalf("Error initializing %s: %s", lt.Name(), err)
			}
			test(lf, t)
			err = lf.Destroy()
			if err != nil {
				t.Fatalf("Error destroying %s: %s", lt.Name(), err)
			}
		}()

		t.Log("Completed test successfully for", lt.Name())
	}
}

func TestInitialization(t *testing.T) {
	allTest(t, testInitialization)
}

func testInitialization(lf ledgerTestFactory, t *testing.T) {
	_, li := lf.New()
	if li.Height() != 1 {
		t.Fatalf("Block height should be 1")
	}
	block := getBlock(0, li)
	if block == nil {
		t.Fatalf("Error retrieving genesis block")
	}
}

func TestReinitialization(t *testing.T) {
	allTest(t, testReinitialization)
}

func testReinitialization(lf ledgerTestFactory, t *testing.T) {
	if !lf.Persistent() {
		t.Log("Skipping test as persistence is not available for this ledger type")
		return
	}
	_, oli := lf.New()
	aBlock := oli.Append([]*cb.Envelope{&cb.Envelope{Payload: []byte("My Data")}}, nil)
	_, li := lf.New()
	if li.Height() != 2 {
		t.Fatalf("Block height should be 2")
	}
	block := getBlock(1, li)
	if block == nil {
		t.Fatalf("Error retrieving block 1")
	}
	if !bytes.Equal(block.Header.Hash(), aBlock.Header.Hash()) {
		t.Fatalf("Block hashes did no match")
	}
}

func TestAddition(t *testing.T) {
	allTest(t, testAddition)
}

func testAddition(lf ledgerTestFactory, t *testing.T) {
	_, li := lf.New()
	genesis := getBlock(0, li)
	if genesis == nil {
		t.Fatalf("Could not retrieve genesis block")
	}
	prevHash := genesis.Header.Hash()

	li.Append([]*cb.Envelope{&cb.Envelope{Payload: []byte("My Data")}}, nil)
	if li.Height() != 2 {
		t.Fatalf("Block height should be 2")
	}
	block := getBlock(1, li)
	if block == nil {
		t.Fatalf("Error retrieving genesis block")
	}
	if !bytes.Equal(block.Header.PreviousHash, prevHash) {
		t.Fatalf("Block hashes did no match")
	}
}

func TestRetrieval(t *testing.T) {
	allTest(t, testRetrieval)
}

func testRetrieval(lf ledgerTestFactory, t *testing.T) {
	_, li := lf.New()
	li.Append([]*cb.Envelope{&cb.Envelope{Payload: []byte("My Data")}}, nil)
	it, num := li.Iterator(ab.SeekInfo_OLDEST, 99)
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
	allTest(t, testBlockedRetrieval)
}

func testBlockedRetrieval(lf ledgerTestFactory, t *testing.T) {
	_, li := lf.New()
	it, num := li.Iterator(ab.SeekInfo_SPECIFIED, 1)
	if num != 1 {
		t.Fatalf("Expected block iterator at 1, but got %d", num)
	}
	signal := it.ReadyChan()
	select {
	case <-signal:
		t.Fatalf("Should not be ready for block read")
	default:
	}
	li.Append([]*cb.Envelope{&cb.Envelope{Payload: []byte("My Data")}}, nil)
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

func TestMultichain(t *testing.T) {
	allTest(t, testMultichain)
}

func testMultichain(lf ledgerTestFactory, t *testing.T) {
	f, _ := lf.New()
	chain1 := "chain1"
	chain2 := "chain2"

	c1p1 := []byte("c1 payload1")
	c1p2 := []byte("c1 payload2")

	c2p1 := []byte("c2 payload1")

	c1, err := f.GetOrCreate(chain1)
	if err != nil {
		t.Fatalf("Error creating chain1: %s", err)
	}

	c1.Append([]*cb.Envelope{&cb.Envelope{Payload: c1p1}}, nil)
	c1b1 := c1.Append([]*cb.Envelope{&cb.Envelope{Payload: c1p2}}, nil)

	if c1.Height() != 2 {
		t.Fatalf("Block height for c1 should be 2")
	}

	c2, err := f.GetOrCreate(chain2)
	if err != nil {
		t.Fatalf("Error creating chain2: %s", err)
	}
	c2b0 := c2.Append([]*cb.Envelope{&cb.Envelope{Payload: c2p1}}, nil)

	if c2.Height() != 1 {
		t.Fatalf("Block height for c2 should be 1")
	}

	c1, err = f.GetOrCreate(chain1)
	if err != nil {
		t.Fatalf("Error retrieving chain1: %s", err)
	}

	if b := getBlock(1, c1); !reflect.DeepEqual(c1b1, b) {
		t.Fatalf("Did not properly store block 1 on chain 1:")
	}

	c2, err = f.GetOrCreate(chain1)
	if err != nil {
		t.Fatalf("Error retrieving chain2: %s", err)
	}

	if b := getBlock(0, c2); reflect.DeepEqual(c2b0, b) {
		t.Fatalf("Did not properly store block 1 on chain 1")
	}
}
