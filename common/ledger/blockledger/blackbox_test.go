/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blockledger_test

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/hyperledger/fabric/common/ledger/blockledger"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"
)

type ledgerTestable interface {
	Initialize() (ledgerTestFactory, error)
	Name() string
}

type ledgerTestFactory interface {
	New() (blockledger.Factory, blockledger.ReadWriter)
	Destroy() error
	Persistent() bool
}

var testables []ledgerTestable

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
	block := blockledger.GetBlock(li, 0)
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
	olf, oli := lf.New()
	envelope := getSampleEnvelopeWithSignatureHeader()
	aBlock := blockledger.CreateNextBlock(oli, []*cb.Envelope{envelope})
	err := oli.Append(aBlock)
	if err != nil {
		t.Fatalf("Error appending block: %s", err)
	}
	olf.Close()

	_, li := lf.New()
	if li.Height() != 2 {
		t.Fatalf("Block height should be 2")
	}
	block := blockledger.GetBlock(li, 1)
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
	genesis := blockledger.GetBlock(li, 0)
	if genesis == nil {
		t.Fatalf("Could not retrieve genesis block")
	}
	prevHash := genesis.Header.Hash()

	envelope := getSampleEnvelopeWithSignatureHeader()
	li.Append(blockledger.CreateNextBlock(li, []*cb.Envelope{envelope}))
	if li.Height() != 2 {
		t.Fatalf("Block height should be 2")
	}
	block := blockledger.GetBlock(li, 1)
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
	envelope := getSampleEnvelopeWithSignatureHeader()
	li.Append(blockledger.CreateNextBlock(li, []*cb.Envelope{envelope}))
	it, num := li.Iterator(&ab.SeekPosition{Type: &ab.SeekPosition_Oldest{}})
	defer it.Close()
	if num != 0 {
		t.Fatalf("Expected genesis block iterator, but got %d", num)
	}

	block, status := it.Next()
	if status != cb.Status_SUCCESS {
		t.Fatalf("Expected to successfully read the genesis block")
	}
	if block.Header.Number != 0 {
		t.Fatalf("Expected to successfully retrieve the genesis block")
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
	it, num := li.Iterator(&ab.SeekPosition{Type: &ab.SeekPosition_Specified{Specified: &ab.SeekSpecified{Number: 1}}})
	defer it.Close()
	if num != 1 {
		t.Fatalf("Expected block iterator at 1, but got %d", num)
	}

	envelope := getSampleEnvelopeWithSignatureHeader()
	li.Append(blockledger.CreateNextBlock(li, []*cb.Envelope{envelope}))

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

	c1, err := f.GetOrCreate(chain1)
	if err != nil {
		t.Fatalf("Error creating chain1: %s", err)
	}

	envelope1 := getSampleEnvelopeWithSignatureHeader()
	c1.Append(blockledger.CreateNextBlock(c1, []*cb.Envelope{envelope1}))
	envelope2 := getSampleEnvelopeWithSignatureHeader()
	c1b1 := blockledger.CreateNextBlock(c1, []*cb.Envelope{envelope2})
	c1.Append(c1b1)

	if c1.Height() != 2 {
		t.Fatalf("Block height for c1 should be 2")
	}

	c2, err := f.GetOrCreate(chain2)
	if err != nil {
		t.Fatalf("Error creating chain2: %s", err)
	}
	envelope3 := getSampleEnvelopeWithSignatureHeader()
	c2b0 := c2.Append(blockledger.CreateNextBlock(c2, []*cb.Envelope{envelope3}))

	if c2.Height() != 1 {
		t.Fatalf("Block height for c2 should be 1")
	}

	c1, err = f.GetOrCreate(chain1)
	if err != nil {
		t.Fatalf("Error retrieving chain1: %s", err)
	}

	if b := blockledger.GetBlock(c1, 1); !reflect.DeepEqual(c1b1, b) {
		t.Fatalf("Did not properly store block 1 on chain 1:")
	}

	c2, err = f.GetOrCreate(chain1)
	if err != nil {
		t.Fatalf("Error retrieving chain2: %s", err)
	}

	if b := blockledger.GetBlock(c2, 0); reflect.DeepEqual(c2b0, b) {
		t.Fatalf("Did not properly store block 1 on chain 1")
	}
}

func getSampleEnvelopeWithSignatureHeader() *cb.Envelope {
	nonce := utils.CreateNonceOrPanic()
	sighdr := &cb.SignatureHeader{Nonce: nonce}
	sighdrBytes := utils.MarshalOrPanic(sighdr)

	header := &cb.Header{SignatureHeader: sighdrBytes}
	payload := &cb.Payload{Header: header}
	payloadBytes := utils.MarshalOrPanic(payload)
	return &cb.Envelope{Payload: payloadBytes}
}
