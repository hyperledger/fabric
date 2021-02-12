/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	"fmt"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset/kvrwset"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/stretchr/testify/require"
)

func TestConstructValidInvalidBlocksPvtData(t *testing.T) {
	conf, cleanup := testConfig(t)
	defer cleanup()

	nsCollBtlConfs := []*nsCollBtlConfig{
		{
			namespace: "ns-1",
			btlConfig: map[string]uint64{
				"coll-1": 0,
				"coll-2": 0,
			},
		},
		{
			namespace: "ns-2",
			btlConfig: map[string]uint64{
				"coll-2": 0,
			},
		},
	}
	provider := testutilNewProviderWithCollectionConfig(
		t,
		nsCollBtlConfs,
		conf,
	)
	defer provider.Close()

	blocksGenerator, gb := testutil.NewBlockGenerator(t, "testLedger", false)
	lg, _ := provider.CreateFromGenesisBlock(gb)
	kvledger := lg.(*kvLedger)
	defer kvledger.Close()

	// block-1
	commitCollectionConfigsHistoryAndDummyBlock(t, provider, kvledger, nsCollBtlConfs, blocksGenerator)

	pvtDataTx0, pubSimResTx0 := produceSamplePvtdata(t, 0,
		[][4]string{
			{"ns-1", "coll-1", "tx0-key-1", "tx0-val-1"},
			{"ns-1", "coll-2", "tx0-key-2", "tx0-val-2"},
		},
	)

	pvtDataTx1, pubSimResTx1 := produceSamplePvtdata(t, 1,
		[][4]string{
			{"ns-1", "coll-1", "tx1-key-1", "tx1-val-1"},
			{"ns-2", "coll-2", "tx1-key-2", "tx1-val-2"},
			{"ns-2", "coll-2", "tx1-key-3", "tx1-val-3"},
			{"ns-2", "coll-2", "tx1-key-4", "tx1-val-4"},
			{"ns-2", "coll-2", "tx1-key-5", "tx1-val-5"},
		},
	)

	pvtData := map[uint64]*ledger.TxPvtData{
		0: pvtDataTx0,
		1: pvtDataTx1,
	}
	simulationResults := [][]byte{pubSimResTx0, pubSimResTx1}
	missingData := make(ledger.TxMissingPvtData)
	missingData.Add(0, "ns-1", "coll-1", true)
	missingData.Add(0, "ns-1", "coll-2", true)
	missingData.Add(1, "ns-1", "coll-1", true)
	missingData.Add(1, "ns-2", "coll-2", true)

	// commit block-2
	blk2 := blocksGenerator.NextBlock([][]byte{pubSimResTx0, pubSimResTx1})
	blockAndPvtData1 := &ledger.BlockAndPvtData{
		Block:          blk2,
		PvtData:        pvtData,
		MissingPvtData: missingData,
	}
	require.NoError(t, kvledger.commit(blockAndPvtData1, &ledger.CommitOptions{}))

	// generate snapshot at block-2
	require.NoError(t, kvledger.generateSnapshot())
	freshConf, cleanup := testConfig(t)
	defer cleanup()

	freshProvider := testutilNewProviderWithCollectionConfig(
		t,
		nsCollBtlConfs,
		freshConf,
	)
	defer freshProvider.Close()

	lgr, _, err := freshProvider.CreateFromSnapshot(SnapshotDirForLedgerBlockNum(conf.SnapshotsConfig.RootDir, "testLedger", 2))
	fmt.Printf("%+v", err)
	require.NoError(t, err)
	bootstrappedLedger := lgr.(*kvLedger)
	defer bootstrappedLedger.Close()

	// commit block-3
	blk3 := blocksGenerator.NextBlock(simulationResults)
	require.NoError(t, bootstrappedLedger.commit(
		&ledger.BlockAndPvtData{
			Block:   blk3,
			PvtData: pvtData,
		},
		&ledger.CommitOptions{},
	))

	pvtdataCopy := func() map[uint64]*ledger.TxPvtData {
		m := make(map[uint64]*ledger.TxPvtData, len(pvtData))
		for k, v := range pvtData {
			wsCopy := (proto.Clone(v.WriteSet)).(*rwset.TxPvtReadWriteSet)
			m[k] = &ledger.TxPvtData{
				SeqInBlock: v.SeqInBlock,
				WriteSet:   wsCopy,
			}
		}
		return m
	}

	t.Run("for-data-after-snapshot:extra-collection-is-ignored", func(t *testing.T) {
		lgr := bootstrappedLedger
		pvtdata := pvtdataCopy()
		pvtdata[1], _ = produceSamplePvtdata(t, 1,
			[][4]string{
				{"ns-1", "non-existing-collection", "randomValue", "randomValue"},
			},
		)

		blocksValidPvtData, hashMismatched, err := constructValidAndInvalidPvtData(
			[]*ledger.ReconciledPvtdata{
				{
					BlockNum:  3,
					WriteSets: pvtdata,
				},
			},
			lgr.blockStore,
			lgr.pvtdataStore,
			2,
		)
		require.NoError(t, err)
		verifyBlocksPvtdata(t,
			map[uint64][]*ledger.TxPvtData{
				3: {
					pvtdata[0],
				},
			},
			blocksValidPvtData,
		)
		// should not include the pvtData passed for the tx1 even in hashmismatched as the collection does not exist in tx1
		require.Len(t, hashMismatched, 0)
	})

	t.Run("for-data-after-snapshot:hash-mismatch-is-reported", func(t *testing.T) {
		lgr := bootstrappedLedger
		pvtdata := pvtdataCopy()

		pvtdata[1], _ = produceSamplePvtdata(t, 1,
			[][4]string{
				{"ns-2", "coll-2", "key-2", "randomValue"},
			},
		)

		blocksValidPvtData, hashMismatches, err := constructValidAndInvalidPvtData(
			[]*ledger.ReconciledPvtdata{
				{
					BlockNum:  3,
					WriteSets: pvtdata,
				},
			},
			lgr.blockStore,
			lgr.pvtdataStore,
			2,
		)
		require.NoError(t, err)
		verifyBlocksPvtdata(t,
			map[uint64][]*ledger.TxPvtData{
				3: {
					pvtdata[0],
				},
			},
			blocksValidPvtData,
		)
		require.Equal(
			t,
			[]*ledger.PvtdataHashMismatch{
				{
					BlockNum:   3,
					TxNum:      1,
					Namespace:  "ns-2",
					Collection: "coll-2",
				},
			},
			hashMismatches,
		)
	})

	t.Run("works-for-mixed-data-before-and-after-snapshot", func(t *testing.T) {
		lgr := bootstrappedLedger
		pvtdata := pvtdataCopy()

		blocksValidPvtData, hashMismatches, err := constructValidAndInvalidPvtData(
			[]*ledger.ReconciledPvtdata{
				{
					BlockNum:  2,
					WriteSets: pvtdata,
				},
				{
					BlockNum:  3,
					WriteSets: pvtdata,
				},
			},
			lgr.blockStore,
			lgr.pvtdataStore,
			2,
		)
		require.NoError(t, err)
		verifyBlocksPvtdata(t,
			map[uint64][]*ledger.TxPvtData{
				2: {
					pvtdata[0],
					pvtdata[1],
				},
				3: {
					pvtdata[0],
					pvtdata[1],
				},
			},
			blocksValidPvtData,
		)
		require.Len(t, hashMismatches, 0)
	})

	t.Run("for-data-before-snapshot:does-not-trim-the-extra-keys", func(t *testing.T) {
		lgr := bootstrappedLedger
		pvtdata := pvtdataCopy()
		pvtdataWithExtraKey := pvtdataCopy()
		pvtdataWithExtraKey[0], _ = produceSamplePvtdata(t, 0,
			[][4]string{
				{"ns-1", "coll-1", "tx0-key-1", "tx0-val-1"},
				{"ns-1", "coll-2", "tx0-key-2", "tx0-val-2"},
				{"ns-1", "coll-2", "extra-key", "extra-val"},
			},
		)
		pvtdataWithExtraKey[2], _ = produceSamplePvtdata(t, 2,
			[][4]string{
				{"ns-1", "coll-1", "tx2-key-1", "tx2-val-1"},
			},
		)

		blocksValidPvtData, hashMismatches, err := constructValidAndInvalidPvtData(
			[]*ledger.ReconciledPvtdata{
				{
					BlockNum:  2,
					WriteSets: pvtdataWithExtraKey,
				},
			},
			lgr.blockStore,
			lgr.pvtdataStore,
			2,
		)
		require.NoError(t, err)

		verifyBlocksPvtdata(t,
			map[uint64][]*ledger.TxPvtData{
				2: {
					pvtdataWithExtraKey[0],
					pvtdata[1],
				},
			},
			blocksValidPvtData,
		)
		require.Len(t, hashMismatches, 0)
	})

	t.Run("for-data-before-snapshot:reports-hash-mismatch-and-partial-data-supplied", func(t *testing.T) {
		lgr := bootstrappedLedger
		temptered := pvtdataCopy()
		temptered[0], _ = produceSamplePvtdata(t, 0,
			[][4]string{
				{"ns-1", "coll-1", "tx0-key-1", "tx0-val-1-tempered"},
			},
		)
		temptered[1], _ = produceSamplePvtdata(t, 1,
			[][4]string{
				{"ns-2", "coll-2", "tx1-key-2", "tx1-val-2-tempered"},
				{"ns-2", "coll-2", "tx1-key-3", "tx1-val-3-tempered"},
			},
		)

		blocksValidPvtData, hashMismatches, err := constructValidAndInvalidPvtData(
			[]*ledger.ReconciledPvtdata{
				{
					BlockNum:  2,
					WriteSets: temptered,
				},
			},
			lgr.blockStore,
			lgr.pvtdataStore,
			2,
		)
		require.NoError(t, err)
		require.Len(t, blocksValidPvtData, 0)
		require.ElementsMatch(t,
			[]*ledger.PvtdataHashMismatch{
				{
					BlockNum:   2,
					TxNum:      0,
					Namespace:  "ns-1",
					Collection: "coll-1",
				},
				{
					BlockNum:   2,
					TxNum:      1,
					Namespace:  "ns-2",
					Collection: "coll-2",
				},
			},
			hashMismatches,
		)
	})

	t.Run("for-data-before-snapshot:reports-repeated-key", func(t *testing.T) {
		lgr := bootstrappedLedger
		pvtdata := pvtdataCopy()
		repeatedKeyInTx0Ns1Coll1 := pvtdataCopy()
		repeatedKeyWS := &kvrwset.KVRWSet{
			Writes: []*kvrwset.KVWrite{
				{
					Key:   "tx0-key-1",
					Value: []byte("tx0-val-1"),
				},
				{
					Key:   "tx0-key-1",
					Value: []byte("tx0-val-1-tempered"),
				},
			},
		}

		repeatedKeyWSBytes, err := proto.Marshal(repeatedKeyWS)
		require.NoError(t, err)
		repeatedKeyInTx0Ns1Coll1[0].WriteSet.NsPvtRwset[0].CollectionPvtRwset[0].Rwset = repeatedKeyWSBytes

		expectedPartOfTx0, _ := produceSamplePvtdata(t, 0,
			[][4]string{
				{"ns-1", "coll-2", "tx0-key-2", "tx0-val-2"},
			},
		)

		blocksValidPvtData, hashMismatches, err := constructValidAndInvalidPvtData(
			[]*ledger.ReconciledPvtdata{
				{
					BlockNum:  2,
					WriteSets: repeatedKeyInTx0Ns1Coll1,
				},
			},
			lgr.blockStore,
			lgr.pvtdataStore,
			2,
		)
		require.NoError(t, err)

		verifyBlocksPvtdata(t,
			map[uint64][]*ledger.TxPvtData{
				2: {
					expectedPartOfTx0,
					pvtdata[1],
				},
			},
			blocksValidPvtData,
		)

		require.ElementsMatch(t,
			[]*ledger.PvtdataHashMismatch{
				{
					BlockNum:   2,
					TxNum:      0,
					Namespace:  "ns-1",
					Collection: "coll-1",
				},
			},
			hashMismatches,
		)
	})

	t.Run("for-data-before-snapshot:ignores-bad-data-corrupted-writeset", func(t *testing.T) {
		lgr := bootstrappedLedger
		pvtdata := pvtdataCopy()
		pvtdataWithBadData := pvtdataCopy()
		pvtdataWithBadData[0].WriteSet.NsPvtRwset[0].CollectionPvtRwset[0].Rwset = []byte("bad-data")

		blocksValidPvtData, hashMismatches, err := constructValidAndInvalidPvtData(
			[]*ledger.ReconciledPvtdata{
				{
					BlockNum:  2,
					WriteSets: pvtdataWithBadData,
				},
			},
			lgr.blockStore,
			lgr.pvtdataStore,
			2,
		)
		require.NoError(t, err)

		verifyBlocksPvtdata(t,
			map[uint64][]*ledger.TxPvtData{
				2: {
					pvtdata[1],
				},
			},
			blocksValidPvtData,
		)
		require.Len(t, hashMismatches, 0)
	})

	t.Run("for-data-before-snapshot:ignores-bad-data-empty-collections", func(t *testing.T) {
		lgr := bootstrappedLedger
		pvtdata := pvtdataCopy()
		pvtdataWithEmptyCollections := pvtdataCopy()
		pvtdataWithEmptyCollections[0].WriteSet.NsPvtRwset[0].CollectionPvtRwset[0].Rwset = nil

		expectedValidDataForTx0, _ := produceSamplePvtdata(t, 0,
			[][4]string{
				{"ns-1", "coll-2", "tx0-key-2", "tx0-val-2"},
			},
		)

		blocksValidPvtData, hashMismatches, err := constructValidAndInvalidPvtData(
			[]*ledger.ReconciledPvtdata{
				{
					BlockNum:  2,
					WriteSets: pvtdataWithEmptyCollections,
				},
			},
			lgr.blockStore,
			lgr.pvtdataStore,
			2,
		)
		require.NoError(t, err)

		verifyBlocksPvtdata(t,
			map[uint64][]*ledger.TxPvtData{
				2: {
					expectedValidDataForTx0,
					pvtdata[1],
				},
			},
			blocksValidPvtData,
		)
		require.Len(t, hashMismatches, 0)
	})
}

func verifyBlocksPvtdata(t *testing.T, expected, actual map[uint64][]*ledger.TxPvtData) {
	require.Len(t, actual, len(expected))
	for blkNum, expectedTx := range expected {
		actualTx := actual[blkNum]
		require.Len(t, actualTx, len(expectedTx))
		m := map[uint64]*rwset.TxPvtReadWriteSet{}
		for _, a := range actualTx {
			m[a.SeqInBlock] = a.WriteSet
		}

		for _, e := range expectedTx {
			require.NotNil(t, m[e.SeqInBlock])
			require.Equal(t, e.WriteSet, m[e.SeqInBlock])
		}
	}
}

func produceSamplePvtdata(t *testing.T, txNum uint64, data [][4]string) (*ledger.TxPvtData, []byte) {
	builder := rwsetutil.NewRWSetBuilder()
	for _, d := range data {
		builder.AddToPvtAndHashedWriteSet(d[0], d[1], d[2], []byte(d[3]))
	}
	simRes, err := builder.GetTxSimulationResults()
	require.NoError(t, err)
	pubSimulationResultsBytes, err := proto.Marshal(simRes.PubSimulationResults)
	require.NoError(t, err)
	return &ledger.TxPvtData{SeqInBlock: txNum, WriteSet: simRes.PvtSimulationResults}, pubSimulationResultsBytes
}

func commitCollectionConfigsHistoryAndDummyBlock(t *testing.T, provider *Provider, kvledger *kvLedger,
	nsCollBtlConfs []*nsCollBtlConfig, blocksGenerator *testutil.BlockGenerator,
) {
	blockAndPvtdata := prepareNextBlockForTest(t, kvledger, blocksGenerator, "SimulateForBlk1",
		map[string]string{
			"dummyKeyForCollectionConfig": "dummyValForCollectionConfig",
		},
		nil,
	)
	require.NoError(t, kvledger.commit(blockAndPvtdata, &ledger.CommitOptions{}))

	for _, nsBTLConf := range nsCollBtlConfs {
		namespace := nsBTLConf.namespace
		collConfig := []*peer.StaticCollectionConfig{}
		for coll, btl := range nsBTLConf.btlConfig {
			collConfig = append(collConfig,
				&peer.StaticCollectionConfig{
					Name:        coll,
					BlockToLive: btl,
				},
			)
		}
		addDummyEntryInCollectionConfigHistory(
			t,
			provider,
			kvledger.ledgerID,
			namespace,
			blockAndPvtdata.Block.Header.Number,
			collConfig,
		)
	}
}

func TestRemoveCollFromTxPvtReadWriteSet(t *testing.T) {
	txpvtrwset := testutilConstructSampleTxPvtRwset(
		[]*testNsColls{
			{ns: "ns-1", colls: []string{"coll-1", "coll-2"}},
			{ns: "ns-2", colls: []string{"coll-3", "coll-4"}},
		},
	)

	removeCollFromTxPvtReadWriteSet(txpvtrwset, "ns-1", "coll-1")
	require.Equal(
		t,
		testutilConstructSampleTxPvtRwset(
			[]*testNsColls{
				{ns: "ns-1", colls: []string{"coll-2"}},
				{ns: "ns-2", colls: []string{"coll-3", "coll-4"}},
			},
		),
		txpvtrwset,
	)

	removeCollFromTxPvtReadWriteSet(txpvtrwset, "ns-1", "coll-2")
	require.Equal(
		t,
		testutilConstructSampleTxPvtRwset(
			[]*testNsColls{
				{ns: "ns-2", colls: []string{"coll-3", "coll-4"}},
			},
		),
		txpvtrwset,
	)
}

func testutilConstructSampleTxPvtRwset(nsCollsList []*testNsColls) *rwset.TxPvtReadWriteSet {
	txPvtRwset := &rwset.TxPvtReadWriteSet{}
	for _, nsColls := range nsCollsList {
		ns := nsColls.ns
		nsdata := &rwset.NsPvtReadWriteSet{
			Namespace:          ns,
			CollectionPvtRwset: []*rwset.CollectionPvtReadWriteSet{},
		}
		txPvtRwset.NsPvtRwset = append(txPvtRwset.NsPvtRwset, nsdata)
		for _, coll := range nsColls.colls {
			nsdata.CollectionPvtRwset = append(nsdata.CollectionPvtRwset,
				&rwset.CollectionPvtReadWriteSet{
					CollectionName: coll,
					Rwset:          []byte(fmt.Sprintf("pvtrwset-for-%s-%s", ns, coll)),
				},
			)
		}
	}
	return txPvtRwset
}

type testNsColls struct {
	ns    string
	colls []string
}
