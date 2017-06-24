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

package ramledger

import (
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

func newTestChain(maxSize int) *ramLedger {
	rlf := New(maxSize)
	chain, err := rlf.GetOrCreate(provisional.TestChainID)
	if err != nil {
		panic(err)
	}
	chain.Append(genesisBlock)
	return chain.(*ramLedger)
}

// TestAppend ensures that appending blocks stores only the maxSize most recent blocks
// Note that 'only' is applicable because the genesis block will be discarded
func TestAppend(t *testing.T) {
	maxSize := 3
	rl := newTestChain(maxSize)
	var blocks []*cb.Block
	for i := 0; i < 3; i++ {
		blocks = append(blocks, &cb.Block{Header: &cb.BlockHeader{Number: uint64(i + 1)}})
		rl.appendBlock(blocks[i])
	}
	item := rl.oldest
	for i := 0; i < 3; i++ {
		if item.block == nil {
			t.Fatalf("Block for item %d should not be nil", i)
		}
		if item.block.Header.Number != blocks[i].Header.Number {
			t.Errorf("Expected block %d to be %d but got %d", i, blocks[i].Header.Number, item.block.Header.Number)
		}
		if i != 2 && item.next == nil {
			t.Fatalf("Next item should not be nil")
		} else {
			item = item.next
		}
	}
}

// TestSignal checks if the signal channel closes when an item is appended
func TestSignal(t *testing.T) {
	maxSize := 3
	rl := newTestChain(maxSize)
	item := rl.newest
	select {
	case <-item.signal:
		t.Fatalf("There is no successor, there should be no signal to continue")
	default:
	}
	rl.appendBlock(&cb.Block{Header: &cb.BlockHeader{Number: 1}})
	select {
	case <-item.signal:
	default:
		t.Fatalf("There is a successor, there should be a signal to continue")
	}
}

// TestTruncatingSafety is intended to simulate a reader who fetches a reference to the oldest list item
// which is then pushed off the history by appending greater than the history size (here, 10 appends with
// a maxSize of 3).  We let the go garbage collector ensure the references still exist
func TestTruncationSafety(t *testing.T) {
	maxSize := 3
	newBlocks := 10
	rl := newTestChain(maxSize)
	item := rl.newest
	for i := 0; i < newBlocks; i++ {
		rl.appendBlock(&cb.Block{Header: &cb.BlockHeader{Number: uint64(i + 1)}})
	}
	count := 0
	for item.next != nil {
		item = item.next
		count++
	}

	if count != newBlocks {
		t.Fatalf("The iterator should have found %d new blocks but found %d", newBlocks, count)
	}
}

func TestRetrieval(t *testing.T) {
	rl := newTestChain(3)
	rl.Append(ledger.CreateNextBlock(rl, []*cb.Envelope{&cb.Envelope{Payload: []byte("My Data")}}))
	it, num := rl.Iterator(&ab.SeekPosition{Type: &ab.SeekPosition_Oldest{}})
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
	rl := newTestChain(3)
	it, num := rl.Iterator(&ab.SeekPosition{Type: &ab.SeekPosition_Specified{Specified: &ab.SeekSpecified{Number: 1}}})
	if num != 1 {
		t.Fatalf("Expected block iterator at 1, but got %d", num)
	}
	signal := it.ReadyChan()
	select {
	case <-signal:
		t.Fatalf("Should not be ready for block read")
	default:
	}
	rl.Append(ledger.CreateNextBlock(rl, []*cb.Envelope{&cb.Envelope{Payload: []byte("My Data")}}))
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

func TestIteratorPastEnd(t *testing.T) {
	rl := newTestChain(3)
	it, _ := rl.Iterator(&ab.SeekPosition{Type: &ab.SeekPosition_Specified{Specified: &ab.SeekSpecified{Number: 2}}})
	if _, status := it.Next(); status != cb.Status_NOT_FOUND {
		t.Fatalf("Expected block with status NOT_FOUND, but got %d", status)
	}
}

func TestIteratorOldest(t *testing.T) {
	rl := newTestChain(3)
	// add enough block to roll off the genesis block
	rl.Append(ledger.CreateNextBlock(rl, []*cb.Envelope{&cb.Envelope{Payload: []byte("My Data")}}))
	rl.Append(ledger.CreateNextBlock(rl, []*cb.Envelope{&cb.Envelope{Payload: []byte("My Data")}}))
	rl.Append(ledger.CreateNextBlock(rl, []*cb.Envelope{&cb.Envelope{Payload: []byte("My Data")}}))
	_, num := rl.Iterator(&ab.SeekPosition{Type: &ab.SeekPosition_Specified{Specified: &ab.SeekSpecified{Number: 1}}})
	if num != 1 {
		t.Fatalf("Expected block iterator at 1, but got %d", num)
	}
}

func TestAppendBadBLock(t *testing.T) {
	rl := newTestChain(3)
	t.Run("BadBlockNumber", func(t *testing.T) {
		nextBlock := ledger.CreateNextBlock(rl, []*cb.Envelope{&cb.Envelope{Payload: []byte("My Data")}})
		nextBlock.Header.Number = nextBlock.Header.Number + 1
		if err := rl.Append(nextBlock); err == nil {
			t.Fatalf("Expected Append to fail.")
		}
	})
	t.Run("BadPreviousHash", func(t *testing.T) {
		nextBlock := ledger.CreateNextBlock(rl, []*cb.Envelope{&cb.Envelope{Payload: []byte("My Data")}})
		nextBlock.Header.PreviousHash = []byte("bad hash")
		if err := rl.Append(nextBlock); err == nil {
			t.Fatalf("Expected Append to fail.")
		}
	})
}
