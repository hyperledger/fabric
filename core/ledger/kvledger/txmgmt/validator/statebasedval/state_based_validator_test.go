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

package statebasedval

import (
	"os"
	"testing"

	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb/stateleveldb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/ledger/rwset/kvrwset"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/spf13/viper"
)

func TestMain(m *testing.M) {
	viper.Set("peer.fileSystemPath", "/tmp/fabric/ledgertests/kvledger/txmgmt/validator/statebasedval")
	os.Exit(m.Run())
}

func TestValidator(t *testing.T) {
	testDBEnv := stateleveldb.NewTestVDBEnv(t)
	defer testDBEnv.Cleanup()

	db, err := testDBEnv.DBProvider.GetDBHandle("TestDB")
	testutil.AssertNoError(t, err, "")

	//populate db with initial data
	batch := statedb.NewUpdateBatch()
	batch.Put("ns1", "key1", []byte("value1"), version.NewHeight(1, 0))
	batch.Put("ns1", "key2", []byte("value2"), version.NewHeight(1, 1))
	batch.Put("ns1", "key3", []byte("value3"), version.NewHeight(1, 2))
	batch.Put("ns1", "key4", []byte("value4"), version.NewHeight(1, 3))
	batch.Put("ns1", "key5", []byte("value5"), version.NewHeight(1, 4))
	db.ApplyUpdates(batch, version.NewHeight(1, 4))

	validator := NewValidator(db)

	//rwset1 should be valid
	rwsetBuilder1 := rwsetutil.NewRWSetBuilder()
	rwsetBuilder1.AddToReadSet("ns1", "key1", version.NewHeight(1, 0))
	rwsetBuilder1.AddToReadSet("ns2", "key2", nil)
	checkValidation(t, validator, []*rwsetutil.TxRwSet{rwsetBuilder1.GetTxReadWriteSet()}, nil, []int{})

	//rwset2 should not be valid
	rwsetBuilder2 := rwsetutil.NewRWSetBuilder()
	rwsetBuilder2.AddToReadSet("ns1", "key1", version.NewHeight(1, 1))
	checkValidation(t, validator, []*rwsetutil.TxRwSet{rwsetBuilder2.GetTxReadWriteSet()}, nil, []int{0})

	//rwset3 should not be valid
	rwsetBuilder3 := rwsetutil.NewRWSetBuilder()
	rwsetBuilder3.AddToReadSet("ns1", "key1", nil)
	checkValidation(t, validator, []*rwsetutil.TxRwSet{rwsetBuilder3.GetTxReadWriteSet()}, nil, []int{0})

	// rwset4 and rwset5 within same block - rwset4 should be valid and makes rwset5 as invalid
	rwsetBuilder4 := rwsetutil.NewRWSetBuilder()
	rwsetBuilder4.AddToReadSet("ns1", "key1", version.NewHeight(1, 0))
	rwsetBuilder4.AddToWriteSet("ns1", "key1", []byte("value1_new"))

	rwsetBuilder5 := rwsetutil.NewRWSetBuilder()
	rwsetBuilder5.AddToReadSet("ns1", "key1", version.NewHeight(1, 0))
	checkValidation(t, validator,
		[]*rwsetutil.TxRwSet{rwsetBuilder4.GetTxReadWriteSet(), rwsetBuilder5.GetTxReadWriteSet()}, nil, []int{1})
}

func TestValidatorSkipInvalidTxs(t *testing.T) {
	testDBEnv := stateleveldb.NewTestVDBEnv(t)
	defer testDBEnv.Cleanup()
	db, err := testDBEnv.DBProvider.GetDBHandle("TestDB")
	testutil.AssertNoError(t, err, "")
	validator := NewValidator(db)

	rwsetBuilder1 := rwsetutil.NewRWSetBuilder()
	rwsetBuilder1.AddToWriteSet("ns1", "key1", []byte("value1"))
	rwsetBuilder2 := rwsetutil.NewRWSetBuilder()
	rwsetBuilder2.AddToWriteSet("ns1", "key2", []byte("value2"))
	rwsetBuilder3 := rwsetutil.NewRWSetBuilder()
	rwsetBuilder3.AddToWriteSet("ns1", "key3", []byte("value3"))
	flags := util.NewTxValidationFlags(4)
	flags.SetFlag(1, peer.TxValidationCode_BAD_CREATOR_SIGNATURE)
	checkValidation(t, validator,
		[]*rwsetutil.TxRwSet{rwsetBuilder1.GetTxReadWriteSet(), rwsetBuilder2.GetTxReadWriteSet(), rwsetBuilder3.GetTxReadWriteSet()},
		flags, []int{1})
}

func TestPhantomValidation(t *testing.T) {
	testDBEnv := stateleveldb.NewTestVDBEnv(t)
	defer testDBEnv.Cleanup()

	db, err := testDBEnv.DBProvider.GetDBHandle("TestDB")
	testutil.AssertNoError(t, err, "")

	//populate db with initial data
	batch := statedb.NewUpdateBatch()
	batch.Put("ns1", "key1", []byte("value1"), version.NewHeight(1, 0))
	batch.Put("ns1", "key2", []byte("value2"), version.NewHeight(1, 1))
	batch.Put("ns1", "key3", []byte("value3"), version.NewHeight(1, 2))
	batch.Put("ns1", "key4", []byte("value4"), version.NewHeight(1, 3))
	batch.Put("ns1", "key5", []byte("value5"), version.NewHeight(1, 4))
	db.ApplyUpdates(batch, version.NewHeight(1, 4))

	validator := NewValidator(db)

	//rwset1 should be valid
	rwsetBuilder1 := rwsetutil.NewRWSetBuilder()
	rqi1 := &kvrwset.RangeQueryInfo{StartKey: "key2", EndKey: "key4", ItrExhausted: true}
	rqi1.SetRawReads([]*kvrwset.KVRead{
		rwsetutil.NewKVRead("key2", version.NewHeight(1, 1)),
		rwsetutil.NewKVRead("key3", version.NewHeight(1, 2))})
	rwsetBuilder1.AddToRangeQuerySet("ns1", rqi1)
	checkValidation(t, validator, []*rwsetutil.TxRwSet{rwsetBuilder1.GetTxReadWriteSet()}, nil, []int{})

	//rwset2 should not be valid - Version of key4 changed
	rwsetBuilder2 := rwsetutil.NewRWSetBuilder()
	rqi2 := &kvrwset.RangeQueryInfo{StartKey: "key2", EndKey: "key4", ItrExhausted: false}
	rqi2.SetRawReads([]*kvrwset.KVRead{
		rwsetutil.NewKVRead("key2", version.NewHeight(1, 1)),
		rwsetutil.NewKVRead("key3", version.NewHeight(1, 2)),
		rwsetutil.NewKVRead("key4", version.NewHeight(1, 2))})
	rwsetBuilder2.AddToRangeQuerySet("ns1", rqi2)
	checkValidation(t, validator, []*rwsetutil.TxRwSet{rwsetBuilder2.GetTxReadWriteSet()}, nil, []int{0})

	//rwset3 should not be valid - simulate key3 got committed to db
	rwsetBuilder3 := rwsetutil.NewRWSetBuilder()
	rqi3 := &kvrwset.RangeQueryInfo{StartKey: "key2", EndKey: "key4", ItrExhausted: false}
	rqi3.SetRawReads([]*kvrwset.KVRead{
		rwsetutil.NewKVRead("key2", version.NewHeight(1, 1)),
		rwsetutil.NewKVRead("key4", version.NewHeight(1, 3))})
	rwsetBuilder3.AddToRangeQuerySet("ns1", rqi3)
	checkValidation(t, validator, []*rwsetutil.TxRwSet{rwsetBuilder3.GetTxReadWriteSet()}, nil, []int{0})

	// //Remove a key in rwset4 and rwset5 should become invalid
	rwsetBuilder4 := rwsetutil.NewRWSetBuilder()
	rwsetBuilder4.AddToWriteSet("ns1", "key3", nil)
	rwsetBuilder5 := rwsetutil.NewRWSetBuilder()
	rqi5 := &kvrwset.RangeQueryInfo{StartKey: "key2", EndKey: "key4", ItrExhausted: false}
	rqi5.SetRawReads([]*kvrwset.KVRead{
		rwsetutil.NewKVRead("key2", version.NewHeight(1, 1)),
		rwsetutil.NewKVRead("key3", version.NewHeight(1, 2)),
		rwsetutil.NewKVRead("key4", version.NewHeight(1, 3))})
	rwsetBuilder5.AddToRangeQuerySet("ns1", rqi5)
	checkValidation(t, validator, []*rwsetutil.TxRwSet{
		rwsetBuilder4.GetTxReadWriteSet(), rwsetBuilder5.GetTxReadWriteSet()}, nil, []int{1})

	//Add a key in rwset6 and rwset7 should become invalid
	rwsetBuilder6 := rwsetutil.NewRWSetBuilder()
	rwsetBuilder6.AddToWriteSet("ns1", "key2_1", []byte("value2_1"))

	rwsetBuilder7 := rwsetutil.NewRWSetBuilder()
	rqi7 := &kvrwset.RangeQueryInfo{StartKey: "key2", EndKey: "key4", ItrExhausted: false}
	rqi7.SetRawReads([]*kvrwset.KVRead{
		rwsetutil.NewKVRead("key2", version.NewHeight(1, 1)),
		rwsetutil.NewKVRead("key3", version.NewHeight(1, 2)),
		rwsetutil.NewKVRead("key4", version.NewHeight(1, 3))})
	rwsetBuilder7.AddToRangeQuerySet("ns1", rqi7)
	checkValidation(t, validator, []*rwsetutil.TxRwSet{
		rwsetBuilder6.GetTxReadWriteSet(), rwsetBuilder7.GetTxReadWriteSet()}, nil, []int{1})
}

func TestPhantomHashBasedValidation(t *testing.T) {
	testDBEnv := stateleveldb.NewTestVDBEnv(t)
	defer testDBEnv.Cleanup()

	db, err := testDBEnv.DBProvider.GetDBHandle("TestDB")
	testutil.AssertNoError(t, err, "")

	//populate db with initial data
	batch := statedb.NewUpdateBatch()
	batch.Put("ns1", "key1", []byte("value1"), version.NewHeight(1, 0))
	batch.Put("ns1", "key2", []byte("value2"), version.NewHeight(1, 1))
	batch.Put("ns1", "key3", []byte("value3"), version.NewHeight(1, 2))
	batch.Put("ns1", "key4", []byte("value4"), version.NewHeight(1, 3))
	batch.Put("ns1", "key5", []byte("value5"), version.NewHeight(1, 4))
	batch.Put("ns1", "key6", []byte("value6"), version.NewHeight(1, 5))
	batch.Put("ns1", "key7", []byte("value7"), version.NewHeight(1, 6))
	batch.Put("ns1", "key8", []byte("value8"), version.NewHeight(1, 7))
	batch.Put("ns1", "key9", []byte("value9"), version.NewHeight(1, 8))
	db.ApplyUpdates(batch, version.NewHeight(1, 8))

	validator := NewValidator(db)

	rwsetBuilder1 := rwsetutil.NewRWSetBuilder()
	rqi1 := &kvrwset.RangeQueryInfo{StartKey: "key2", EndKey: "key9", ItrExhausted: true}
	kvReadsDuringSimulation1 := []*kvrwset.KVRead{
		rwsetutil.NewKVRead("key2", version.NewHeight(1, 1)),
		rwsetutil.NewKVRead("key3", version.NewHeight(1, 2)),
		rwsetutil.NewKVRead("key4", version.NewHeight(1, 3)),
		rwsetutil.NewKVRead("key5", version.NewHeight(1, 4)),
		rwsetutil.NewKVRead("key6", version.NewHeight(1, 5)),
		rwsetutil.NewKVRead("key7", version.NewHeight(1, 6)),
		rwsetutil.NewKVRead("key8", version.NewHeight(1, 7)),
	}
	rqi1.SetMerkelSummary(buildTestHashResults(t, 2, kvReadsDuringSimulation1))
	rwsetBuilder1.AddToRangeQuerySet("ns1", rqi1)
	checkValidation(t, validator, []*rwsetutil.TxRwSet{rwsetBuilder1.GetTxReadWriteSet()}, nil, []int{})

	rwsetBuilder2 := rwsetutil.NewRWSetBuilder()
	rqi2 := &kvrwset.RangeQueryInfo{StartKey: "key1", EndKey: "key9", ItrExhausted: false}
	kvReadsDuringSimulation2 := []*kvrwset.KVRead{
		rwsetutil.NewKVRead("key1", version.NewHeight(1, 0)),
		rwsetutil.NewKVRead("key2", version.NewHeight(1, 1)),
		rwsetutil.NewKVRead("key3", version.NewHeight(1, 1)),
		rwsetutil.NewKVRead("key4", version.NewHeight(1, 3)),
		rwsetutil.NewKVRead("key5", version.NewHeight(1, 4)),
		rwsetutil.NewKVRead("key6", version.NewHeight(1, 5)),
		rwsetutil.NewKVRead("key7", version.NewHeight(1, 6)),
		rwsetutil.NewKVRead("key8", version.NewHeight(1, 7)),
		rwsetutil.NewKVRead("key9", version.NewHeight(1, 8)),
	}
	rqi2.SetMerkelSummary(buildTestHashResults(t, 2, kvReadsDuringSimulation2))
	rwsetBuilder2.AddToRangeQuerySet("ns1", rqi2)
	checkValidation(t, validator, []*rwsetutil.TxRwSet{rwsetBuilder2.GetTxReadWriteSet()}, nil, []int{0})
}

func checkValidation(t *testing.T, validator *Validator, rwsets []*rwsetutil.TxRwSet,
	alreadyMarkedFlags util.TxValidationFlags, expectedInvalidTxIndexes []int) {
	simulationResults := [][]byte{}
	for _, txRWS := range rwsets {
		sr, err := txRWS.ToProtoBytes()
		testutil.AssertNoError(t, err, "")
		simulationResults = append(simulationResults, sr)
	}
	block := testutil.ConstructBlock(t, 1, []byte("dummyPreviousHash"), simulationResults, false)
	block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] = alreadyMarkedFlags
	_, err := validator.ValidateAndPrepareBatch(block, true)
	txsFltr := util.TxValidationFlags(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])
	invalidTxs := make([]int, 0)
	for i := 0; i < len(block.Data.Data); i++ {
		if txsFltr.IsInvalid(i) {
			invalidTxs = append(invalidTxs, i)
		}
	}
	testutil.AssertNoError(t, err, "")
	testutil.AssertEquals(t, len(invalidTxs), len(expectedInvalidTxIndexes))
	testutil.AssertContainsAll(t, invalidTxs, expectedInvalidTxIndexes)
}

func buildTestHashResults(t *testing.T, maxDegree int, kvReads []*kvrwset.KVRead) *kvrwset.QueryReadsMerkleSummary {
	if len(kvReads) <= maxDegree {
		t.Fatal("This method should be called with number of KVReads more than maxDegree; Else, hashing won't be performedrwset")
	}
	helper, _ := rwsetutil.NewRangeQueryResultsHelper(true, uint32(maxDegree))
	for _, kvRead := range kvReads {
		helper.AddResult(kvRead)
	}
	_, h, err := helper.Done()
	testutil.AssertNoError(t, err, "")
	testutil.AssertNotNil(t, h)
	return h
}
