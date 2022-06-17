/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fileledger

import (
	"errors"
	"fmt"
	"testing"

	cb "github.com/hyperledger/fabric-protos-go/common"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/flogging"
	cl "github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/common/ledger/blkstorage/blkstoragetest"
	"github.com/hyperledger/fabric/common/ledger/blockledger"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

var genesisBlock = protoutil.NewBlock(0, nil)

func init() {
	flogging.ActivateSpec("common.ledger.blockledger.file=DEBUG")
}

type testEnv struct {
	t        *testing.T
	location string
	flf      blockledger.Factory
}

func initialize(t *testing.T) (*testEnv, *FileLedger) {
	name := t.TempDir()

	p, err := New(name, &disabled.Provider{})
	require.NoError(t, err)
	flf := p.(*fileLedgerFactory)
	fl, err := flf.GetOrCreate("testchannelid")
	require.NoError(t, err, "Error GetOrCreate channel")
	fl.Append(genesisBlock)
	return &testEnv{location: name, t: t, flf: flf}, fl.(*FileLedger)
}

func (tev *testEnv) tearDown() {
	tev.shutDown()
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

type mockBlockStoreIterator struct {
	mock.Mock
}

func (m *mockBlockStoreIterator) Next() (cl.QueryResult, error) {
	args := m.Called()
	return args.Get(0), args.Error(1)
}

func (m *mockBlockStoreIterator) Close() {
	m.Called()
}

func TestInitialization(t *testing.T) {
	tev, fl := initialize(t)
	defer tev.tearDown()

	require.Equal(t, uint64(1), fl.Height(), "Block height should be 1")

	block := blockledger.GetBlock(fl, 0)
	require.NotNil(t, block, "Error retrieving genesis block")
	require.Equal(t, protoutil.BlockHeaderHash(genesisBlock.Header), protoutil.BlockHeaderHash(block.Header), "Block hashes did no match")
}

func TestReinitialization(t *testing.T) {
	tev, ledger1 := initialize(t)
	defer tev.tearDown()

	// create a block to add to the ledger
	envelope := getSampleEnvelopeWithSignatureHeader()
	b1 := blockledger.CreateNextBlock(ledger1, []*cb.Envelope{envelope})

	// add the block to the ledger
	ledger1.Append(b1)

	fl, err := tev.flf.GetOrCreate("testchannelid")
	ledger1, ok := fl.(*FileLedger)
	require.NoError(t, err, "Expected to successfully get test channel")
	require.Equal(t, 1, len(tev.flf.ChannelIDs()), "Exptected not new channel to be created")
	require.True(t, ok, "Exptected type assertion to succeed")
	require.Equal(t, uint64(2), ledger1.Height(), "Block height should be 2. Got %v", ledger1.Height())

	// shut down the ledger provider
	tev.shutDown()

	// re-initialize the ledger provider (not the test ledger itself!)
	provider2, err := New(tev.location, &disabled.Provider{})
	require.NoError(t, err)

	// assert expected ledgers exist
	channels := provider2.ChannelIDs()
	require.Equal(t, 1, len(channels), "Should have recovered the channel")

	// get the existing test channel ledger
	ledger2, err := provider2.GetOrCreate(channels[0])
	require.NoError(t, err, "Unexpected error: %s", err)

	fl = ledger2.(*FileLedger)
	require.Equal(t, uint64(2), fl.Height(), "Block height should be 2. Got %v", fl.Height())

	block := blockledger.GetBlock(fl, 1)
	require.NotNil(t, block, "Error retrieving block 1")
	require.Equal(t, protoutil.BlockHeaderHash(b1.Header), protoutil.BlockHeaderHash(block.Header), "Block hashes did no match")
}

func TestAddition(t *testing.T) {
	tev, fl := initialize(t)
	defer tev.tearDown()
	info, _ := fl.blockStore.GetBlockchainInfo()
	prevHash := info.CurrentBlockHash
	envelope := getSampleEnvelopeWithSignatureHeader()
	b1 := blockledger.CreateNextBlock(fl, []*cb.Envelope{envelope})
	fl.Append(b1)
	require.Equal(t, uint64(2), fl.Height(), "Block height should be 2")

	block := blockledger.GetBlock(fl, 1)
	require.NotNil(t, block, "Error retrieving genesis block")
	require.Equal(t, prevHash, block.Header.PreviousHash, "Block hashes did no match")
}

func TestRetrieval(t *testing.T) {
	tev, fl := initialize(t)
	defer tev.tearDown()
	envelope := getSampleEnvelopeWithSignatureHeader()
	b1 := blockledger.CreateNextBlock(fl, []*cb.Envelope{envelope})
	fl.Append(b1)
	it, num := fl.Iterator(&ab.SeekPosition{Type: &ab.SeekPosition_Oldest{}})
	defer it.Close()
	require.Zero(t, num, "Expected genesis block iterator, but got %d", num)

	block, status := it.Next()
	require.Equal(t, cb.Status_SUCCESS, status, "Expected to successfully read the genesis block")
	require.Zero(t, block.Header.Number, "Expected to successfully retrieve the genesis block")

	block, status = it.Next()
	require.Equal(t, cb.Status_SUCCESS, status, "Expected to successfully read the second block")
	require.Equal(
		t,
		uint64(1),
		block.Header.Number,
		"Expected to successfully retrieve the second block but got block number %d", block.Header.Number)
}

func TestBlockedRetrieval(t *testing.T) {
	tev, fl := initialize(t)
	defer tev.tearDown()
	it, num := fl.Iterator(&ab.SeekPosition{Type: &ab.SeekPosition_Specified{Specified: &ab.SeekSpecified{Number: 1}}})
	defer it.Close()
	if num != 1 {
		t.Fatalf("Expected block iterator at 1, but got %d", num)
	}
	require.Equal(t, uint64(1), num, "Expected block iterator at 1, but got %d", num)

	envelope := getSampleEnvelopeWithSignatureHeader()
	b1 := blockledger.CreateNextBlock(fl, []*cb.Envelope{envelope})
	fl.Append(b1)

	block, status := it.Next()
	require.Equal(t, cb.Status_SUCCESS, status, "Expected to successfully read the second block")
	require.Equal(
		t,
		uint64(1),
		block.Header.Number,
		"Expected to successfully retrieve the second block but got block number %d", block.Header.Number)

	b2 := blockledger.CreateNextBlock(fl, []*cb.Envelope{envelope})
	fl.Append(b2)

	block, status = it.Next()
	require.Equal(t, cb.Status_SUCCESS, status, "Expected to successfully read the third block")
	require.Equal(t, uint64(2), block.Header.Number, "Expected to successfully retrieve the third block")

	// verify NextCommit seek position
	it2, num2 := fl.Iterator(&ab.SeekPosition{Type: &ab.SeekPosition_NextCommit{NextCommit: &ab.SeekNextCommit{}}})
	require.Equal(t, uint64(3), num2)
	defer it2.Close()

	b3 := blockledger.CreateNextBlock(fl, []*cb.Envelope{envelope})
	fl.Append(b3)

	block, status = it.Next()
	require.Equal(t, cb.Status_SUCCESS, status)
	require.Equal(t, uint64(3), block.Header.Number)
}

func TestBlockRetrievalWithSnapshot(t *testing.T) {
	numBlocks := 5
	// create an extra block to add later for iterator.Next testing
	blocks := testutil.ConstructTestBlocks(t, numBlocks+1)

	blockStore, cleanup := blkstoragetest.BootstrapBlockstoreFromSnapshot(t, "blockretrievalwithsnapshot", blocks[:numBlocks])
	defer cleanup()

	fl := NewFileLedger(blockStore)

	// verify lastBlockInSnapshot, which should be numBlocks - 1
	bcInfo, err := fl.blockStore.GetBlockchainInfo()
	require.NoError(t, err)
	require.Equal(t, uint64(numBlocks-1), bcInfo.BootstrappingSnapshotInfo.LastBlockInSnapshot)

	// verify iterator startingNum for Newest, NextCommit, and Specified
	it, startingNum := fl.Iterator(&ab.SeekPosition{Type: &ab.SeekPosition_Newest{}})
	defer it.Close()
	require.Equal(t, uint64(numBlocks), startingNum)

	it2, startingNum2 := fl.Iterator(&ab.SeekPosition{Type: &ab.SeekPosition_NextCommit{}})
	defer it2.Close()
	require.Equal(t, uint64(numBlocks), startingNum2)

	it3, startingNum3 := fl.Iterator(&ab.SeekPosition{Type: &ab.SeekPosition_Specified{Specified: &ab.SeekSpecified{Number: uint64(numBlocks)}}})
	defer it3.Close()
	require.Equal(t, uint64(numBlocks), startingNum3)

	it4, _ := fl.Iterator(&ab.SeekPosition{Type: &ab.SeekPosition_Specified{Specified: &ab.SeekSpecified{Number: uint64(numBlocks - 1)}}})
	defer it4.Close()
	require.Equal(t, &blockledger.NotFoundErrorIterator{}, it4)

	it5, _ := fl.Iterator(&ab.SeekPosition{Type: &ab.SeekPosition_Specified{Specified: &ab.SeekSpecified{Number: uint64(numBlocks + 1)}}})
	defer it5.Close()
	require.Equal(t, &blockledger.NotFoundErrorIterator{}, it4)

	// add a block and verify iterator.Next
	nextBlk := blocks[numBlocks]
	err = fl.Append(nextBlk)
	require.NoError(t, err)

	blk, status := it.Next()
	require.Equal(t, cb.Status_SUCCESS, status)
	require.Equal(t, nextBlk, blk)

	blk, status = it2.Next()
	require.Equal(t, cb.Status_SUCCESS, status)
	require.Equal(t, nextBlk, blk)

	blk, status = it3.Next()
	require.Equal(t, cb.Status_SUCCESS, status)
	require.Equal(t, nextBlk, blk)
}

func TestBlockstoreError(t *testing.T) {
	// Since this test only ensures failed GetBlockchainInfo
	// is properly handled. We don't bother creating fully
	// legit ledgers here (without genesis block).
	{
		fl := &FileLedger{
			blockStore: &mockBlockStore{
				blockchainInfo:         nil,
				getBlockchainInfoError: fmt.Errorf("Error getting blockchain info"),
			},
			signal: make(chan struct{}),
		}
		require.Panics(
			t,
			func() {
				fl.Iterator(&ab.SeekPosition{Type: &ab.SeekPosition_Newest{}})
			},
			"Expected Iterator() to panic if blockstore operation fails")

		require.Panics(
			t,
			func() { fl.Height() },
			"Expected Height() to panic if blockstore operation fails ")
	}

	{
		fl := &FileLedger{
			blockStore: &mockBlockStore{
				blockchainInfo:             &cb.BlockchainInfo{Height: uint64(1)},
				getBlockchainInfoError:     nil,
				retrieveBlockByNumberError: fmt.Errorf("Error retrieving block by number"),
			},
			signal: make(chan struct{}),
		}
		it, _ := fl.Iterator(&ab.SeekPosition{Type: &ab.SeekPosition_Specified{Specified: &ab.SeekSpecified{Number: 42}}})
		defer it.Close()
		require.IsType(
			t,
			&blockledger.NotFoundErrorIterator{},
			it,
			"Expected Not Found Error if seek number is greater than ledger height")
	}

	{
		resultsIterator := &mockBlockStoreIterator{}
		resultsIterator.On("Next").Return(nil, errors.New("a mocked error"))
		resultsIterator.On("Close").Return()
		fl := &FileLedger{
			blockStore: &mockBlockStore{
				blockchainInfo:             &cb.BlockchainInfo{Height: uint64(1)},
				getBlockchainInfoError:     nil,
				retrieveBlockByNumberError: fmt.Errorf("Error retrieving block by number"),
				resultsIterator:            resultsIterator,
			},
			signal: make(chan struct{}),
		}
		it, _ := fl.Iterator(&ab.SeekPosition{Type: &ab.SeekPosition_Oldest{}})
		defer it.Close()
		_, status := it.Next()
		require.Equal(t, cb.Status_SERVICE_UNAVAILABLE, status, "Expected service unavailable error")
	}
}

func getSampleEnvelopeWithSignatureHeader() *cb.Envelope {
	nonce := protoutil.CreateNonceOrPanic()
	sighdr := &cb.SignatureHeader{Nonce: nonce}
	sighdrBytes := protoutil.MarshalOrPanic(sighdr)

	header := &cb.Header{SignatureHeader: sighdrBytes}
	payload := &cb.Payload{Header: header}
	payloadBytes := protoutil.MarshalOrPanic(payload)
	return &cb.Envelope{Payload: payloadBytes}
}
