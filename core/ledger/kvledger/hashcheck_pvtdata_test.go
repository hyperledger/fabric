/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	"fmt"
	"strings"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/hyperledger/fabric/protos/ledger/rwset"
	"github.com/stretchr/testify/assert"
)

func TestConstructValidInvalidBlocksPvtData(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()
	provider := testutilNewProvider(t)
	defer provider.Close()

	_, gb := testutil.NewBlockGenerator(t, "testLedger", false)
	gbHash := gb.Header.Hash()
	lg, _ := provider.Create(gb)
	defer lg.Close()

	// construct pvtData and pubRwSet (i.e., hashed rw set)
	v0 := []byte{0}
	pvtDataBlk1Tx0, pubSimResBytesBlk1Tx0 := produceSamplePvtdata(t, 0, []string{"ns-1:coll-1", "ns-1:coll-2"}, [][]byte{v0, v0})
	v1 := []byte{1}
	pvtDataBlk1Tx1, pubSimResBytesBlk1Tx1 := produceSamplePvtdata(t, 1, []string{"ns-1:coll-1", "ns-1:coll-2"}, [][]byte{v1, v1})
	v2 := []byte{2}
	pvtDataBlk1Tx2, pubSimResBytesBlk1Tx2 := produceSamplePvtdata(t, 2, []string{"ns-1:coll-1", "ns-2:coll-2"}, [][]byte{v2, v2})
	v3 := []byte{3}
	pvtDataBlk1Tx3, pubSimResBytesBlk1Tx3 := produceSamplePvtdata(t, 3, []string{"ns-1:coll-1", "ns-1:coll-2"}, [][]byte{v3, v3})
	v4 := []byte{4}
	pvtDataBlk1Tx4, pubSimResBytesBlk1Tx4 := produceSamplePvtdata(t, 4, []string{"ns-1:coll-1", "ns-4:coll-2"}, [][]byte{v4, v4})
	v5 := []byte{5}
	pvtDataBlk1Tx5, pubSimResBytesBlk1Tx5 := produceSamplePvtdata(t, 5, []string{"ns-1:coll-1", "ns-1:coll-2"}, [][]byte{v5, v5})
	v6 := []byte{6}
	pvtDataBlk1Tx6, pubSimResBytesBlk1Tx6 := produceSamplePvtdata(t, 6, []string{"ns-6:coll-2"}, [][]byte{v6})
	v7 := []byte{7}
	_, pubSimResBytesBlk1Tx7 := produceSamplePvtdata(t, 7, []string{"ns-1:coll-2"}, [][]byte{v7})
	wrongPvtDataBlk1Tx7, _ := produceSamplePvtdata(t, 7, []string{"ns-6:coll-2"}, [][]byte{v6})

	pubSimulationResults := &rwset.TxReadWriteSet{}
	err := proto.Unmarshal(pubSimResBytesBlk1Tx7, pubSimulationResults)
	assert.NoError(t, err)
	tx7PvtdataHash := pubSimulationResults.NsRwset[0].CollectionHashedRwset[0].PvtRwsetHash

	// construct block1
	simulationResultsBlk1 := [][]byte{pubSimResBytesBlk1Tx0, pubSimResBytesBlk1Tx1, pubSimResBytesBlk1Tx2,
		pubSimResBytesBlk1Tx3, pubSimResBytesBlk1Tx4, pubSimResBytesBlk1Tx5,
		pubSimResBytesBlk1Tx6, pubSimResBytesBlk1Tx7}
	blk1 := testutil.ConstructBlock(t, 1, gbHash, simulationResultsBlk1, false)

	// construct a pvtData list for block1
	pvtDataBlk1 := map[uint64]*ledger.TxPvtData{
		0: pvtDataBlk1Tx0,
		1: pvtDataBlk1Tx1,
		2: pvtDataBlk1Tx2,
		4: pvtDataBlk1Tx4,
		5: pvtDataBlk1Tx5,
	}

	// construct a missingData list for block1
	missingData := make(ledger.TxMissingPvtDataMap)
	missingData.Add(3, "ns-1", "coll-1", true)
	missingData.Add(3, "ns-1", "coll-2", true)
	missingData.Add(6, "ns-6", "coll-2", true)
	missingData.Add(7, "ns-1", "coll-2", true)

	// commit block1
	blockAndPvtData1 := &ledger.BlockAndPvtData{
		Block:          blk1,
		PvtData:        pvtDataBlk1,
		MissingPvtData: missingData}
	assert.NoError(t, lg.(*kvLedger).blockStore.CommitWithPvtData(blockAndPvtData1))

	// construct pvtData from missing data in tx3, tx6, and tx7
	blocksPvtData := []*ledger.BlockPvtData{
		{
			BlockNum: 1,
			WriteSets: map[uint64]*ledger.TxPvtData{
				3: pvtDataBlk1Tx3,
				6: pvtDataBlk1Tx6,
				7: wrongPvtDataBlk1Tx7,
				// ns-6:coll-2 does not present in tx7
			},
		},
	}

	expectedValidBlocksPvtData := map[uint64][]*ledger.TxPvtData{
		1: {
			pvtDataBlk1Tx3,
			pvtDataBlk1Tx6,
		},
	}

	blocksValidPvtData, hashMismatched, err := constructValidAndInvalidPvtData(blocksPvtData, lg.(*kvLedger).blockStore)
	assert.NoError(t, err)
	assert.Equal(t, len(expectedValidBlocksPvtData), len(blocksValidPvtData))
	assert.ElementsMatch(t, expectedValidBlocksPvtData[1], blocksValidPvtData[1])
	// should not include the pvtData passed for the tx7 even in hashmismatched as ns-6:coll-2 does not exist in tx7
	assert.Len(t, hashMismatched, 0)

	// construct pvtData from missing data in tx7 with wrong pvtData
	wrongPvtDataBlk1Tx7, pubSimResBytesBlk1Tx7 = produceSamplePvtdata(t, 7, []string{"ns-1:coll-2"}, [][]byte{v6})
	blocksPvtData = []*ledger.BlockPvtData{
		{
			BlockNum: 1,
			WriteSets: map[uint64]*ledger.TxPvtData{
				7: wrongPvtDataBlk1Tx7,
				// ns-1:coll-1 exists in tx7 but the passed pvtData is incorrect
			},
		},
	}

	expectedHashMismatches := []*ledger.PvtdataHashMismatch{
		{
			BlockNum:     1,
			TxNum:        7,
			Namespace:    "ns-1",
			Collection:   "coll-2",
			ExpectedHash: tx7PvtdataHash,
		},
	}

	blocksValidPvtData, hashMismatches, err := constructValidAndInvalidPvtData(blocksPvtData, lg.(*kvLedger).blockStore)
	assert.NoError(t, err)
	assert.Len(t, blocksValidPvtData, 0)

	assert.ElementsMatch(t, expectedHashMismatches, hashMismatches)
}

func produceSamplePvtdata(t *testing.T, txNum uint64, nsColls []string, values [][]byte) (*ledger.TxPvtData, []byte) {
	builder := rwsetutil.NewRWSetBuilder()
	for index, nsColl := range nsColls {
		nsCollSplit := strings.Split(nsColl, ":")
		ns := nsCollSplit[0]
		coll := nsCollSplit[1]
		key := fmt.Sprintf("key-%s-%s", ns, coll)
		value := values[index]
		builder.AddToPvtAndHashedWriteSet(ns, coll, key, value)
	}
	simRes, err := builder.GetTxSimulationResults()
	assert.NoError(t, err)
	pubSimulationResultsBytes, err := proto.Marshal(simRes.PubSimulationResults)
	assert.NoError(t, err)
	return &ledger.TxPvtData{SeqInBlock: txNum, WriteSet: simRes.PvtSimulationResults}, pubSimulationResultsBytes
}
