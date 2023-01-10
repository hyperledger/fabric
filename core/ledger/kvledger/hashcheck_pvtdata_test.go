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
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/stretchr/testify/require"
)

func TestExtractValidPvtData(t *testing.T) {
	conf := testConfig(t)

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
	freshConf := testConfig(t)

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

		blocksValidPvtData, err := extractValidPvtData(
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
	})

	t.Run("for-data-after-snapshot:hash-mismatch-is-dropped", func(t *testing.T) {
		lgr := bootstrappedLedger
		pvtdata := pvtdataCopy()

		pvtdata[1], _ = produceSamplePvtdata(t, 1,
			[][4]string{
				{"ns-2", "coll-2", "key-2", "randomValue"},
			},
		)

		blocksValidPvtData, err := extractValidPvtData(
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
	})

	t.Run("works-for-mixed-data-before-and-after-snapshot", func(t *testing.T) {
		lgr := bootstrappedLedger
		pvtdata := pvtdataCopy()

		blocksValidPvtData, err := extractValidPvtData(
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
	})

	t.Run("for-data-before-snapshot:excludes-hash-mismatch-and-partial-data-supplied", func(t *testing.T) {
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

		blocksValidPvtData, err := extractValidPvtData(
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
	})

	t.Run("for-data-before-snapshot:drops-collection-with-corrupted-writeset", func(t *testing.T) {
		lgr := bootstrappedLedger
		pvtdataWithBadData := pvtdataCopy()
		pvtdataWithBadData[0].WriteSet.NsPvtRwset[0].CollectionPvtRwset[0].Rwset = []byte("bad-data")

		blocksValidPvtData, err := extractValidPvtData(
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

		expectedOutput := pvtdataCopy()
		expectedOutput[0].WriteSet.NsPvtRwset[0].CollectionPvtRwset =
			expectedOutput[0].WriteSet.NsPvtRwset[0].CollectionPvtRwset[1:]

		verifyBlocksPvtdata(t,
			map[uint64][]*ledger.TxPvtData{
				2: {
					expectedOutput[0],
					expectedOutput[1],
				},
			},
			blocksValidPvtData,
		)
	})

	t.Run("for-data-before-snapshot:drops-collection-with-empty-writeset", func(t *testing.T) {
		lgr := bootstrappedLedger
		pvtdata := pvtdataCopy()
		pvtdataWithEmptyCollections := pvtdataCopy()
		pvtdataWithEmptyCollections[0].WriteSet.NsPvtRwset[0].CollectionPvtRwset[0].Rwset = nil

		expectedValidDataForTx0, _ := produceSamplePvtdata(t, 0,
			[][4]string{
				{"ns-1", "coll-2", "tx0-key-2", "tx0-val-2"},
			},
		)

		blocksValidPvtData, err := extractValidPvtData(
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
	})

	t.Run("for-both-before-and-after-snapshot-data:removes-purged-keys", func(t *testing.T) {
		lgr := bootstrappedLedger
		// purge all keys committed by tx1 and only one of the keys committed by tx2
		pvtData, simulationResults := purgeKeyTransaction(t,
			[][3]string{
				{"ns-1", "coll-1", "tx0-key-1"},
				{"ns-1", "coll-2", "tx0-key-2"},
				{"ns-2", "coll-2", "tx1-key-2"},
			},
		)

		blk4 := blocksGenerator.NextBlock([][]byte{simulationResults})
		require.NoError(t, lgr.commit(
			&ledger.BlockAndPvtData{
				Block: blk4,
				PvtData: map[uint64]*ledger.TxPvtData{
					0: pvtData,
				},
			},
			&ledger.CommitOptions{},
		))

		pvtdata := pvtdataCopy()
		blocksValidPvtData, err := extractValidPvtData(
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

		// All keys committed by tx1 should be purged and only empty collections should be returned
		expectedValidDataForTx0 := pvtdataCopy()[0]
		expectedValidDataForTx0.WriteSet.NsPvtRwset[0].CollectionPvtRwset[0].Rwset = nil
		expectedValidDataForTx0.WriteSet.NsPvtRwset[0].CollectionPvtRwset[1].Rwset = nil

		// Only one of the keys committed by tx2 should be removed from the trimmed collection writeset
		expectedValidDataForTx1, _ := produceSamplePvtdata(t, 1,
			[][4]string{
				{"ns-1", "coll-1", "tx1-key-1", "tx1-val-1"},
				{"ns-2", "coll-2", "tx1-key-3", "tx1-val-3"},
				{"ns-2", "coll-2", "tx1-key-4", "tx1-val-4"},
				{"ns-2", "coll-2", "tx1-key-5", "tx1-val-5"},
			},
		)
		verifyBlocksPvtdata(t,
			map[uint64][]*ledger.TxPvtData{
				2: {
					expectedValidDataForTx0,
					expectedValidDataForTx1,
				},
				3: {
					expectedValidDataForTx0,
					expectedValidDataForTx1,
				},
			},
			blocksValidPvtData,
		)
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

func purgeKeyTransaction(t *testing.T, nsCollKeys [][3]string) (*ledger.TxPvtData, []byte) {
	builder := rwsetutil.NewRWSetBuilder()
	for _, nsCollKey := range nsCollKeys {
		builder.AddToPvtAndHashedWriteSetForPurge(nsCollKey[0], nsCollKey[1], nsCollKey[2])
	}
	simRes, err := builder.GetTxSimulationResults()
	require.NoError(t, err)
	pubSimulationResultsBytes, err := proto.Marshal(simRes.PubSimulationResults)
	require.NoError(t, err)
	return &ledger.TxPvtData{SeqInBlock: 0, WriteSet: simRes.PvtSimulationResults}, pubSimulationResultsBytes
}
