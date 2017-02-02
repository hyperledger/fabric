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
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwset"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb/stateleveldb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/protos/common"
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
	batch.Put("ns1", "key1", []byte("value1"), version.NewHeight(1, 1))
	batch.Put("ns1", "key2", []byte("value2"), version.NewHeight(1, 2))
	batch.Put("ns1", "key3", []byte("value3"), version.NewHeight(1, 3))
	batch.Put("ns1", "key4", []byte("value4"), version.NewHeight(1, 4))
	batch.Put("ns1", "key5", []byte("value5"), version.NewHeight(1, 5))
	db.ApplyUpdates(batch, version.NewHeight(1, 5))

	validator := NewValidator(db)

	//rwset1 should be valid
	rwset1 := rwset.NewRWSet()
	rwset1.AddToReadSet("ns1", "key1", version.NewHeight(1, 1))
	rwset1.AddToReadSet("ns2", "key2", nil)
	checkValidation(t, validator, []*rwset.RWSet{rwset1}, []int{})

	//rwset2 should not be valid
	rwset2 := rwset.NewRWSet()
	rwset2.AddToReadSet("ns1", "key1", version.NewHeight(1, 2))
	checkValidation(t, validator, []*rwset.RWSet{rwset2}, []int{0})

	//rwset3 should not be valid
	rwset3 := rwset.NewRWSet()
	rwset3.AddToReadSet("ns1", "key1", nil)
	checkValidation(t, validator, []*rwset.RWSet{rwset3}, []int{0})

	// rwset4 and rwset5 within same block - rwset4 should be valid and makes rwset5 as invalid
	rwset4 := rwset.NewRWSet()
	rwset4.AddToReadSet("ns1", "key1", version.NewHeight(1, 1))
	rwset4.AddToWriteSet("ns1", "key1", []byte("value1_new"))
	rwset5 := rwset.NewRWSet()
	rwset5.AddToReadSet("ns1", "key1", version.NewHeight(1, 1))
	checkValidation(t, validator, []*rwset.RWSet{rwset4, rwset5}, []int{1})
}

func TestPhantomValidation(t *testing.T) {
	testDBEnv := stateleveldb.NewTestVDBEnv(t)
	defer testDBEnv.Cleanup()

	db, err := testDBEnv.DBProvider.GetDBHandle("TestDB")
	testutil.AssertNoError(t, err, "")

	//populate db with initial data
	batch := statedb.NewUpdateBatch()
	batch.Put("ns1", "key1", []byte("value1"), version.NewHeight(1, 1))
	batch.Put("ns1", "key2", []byte("value2"), version.NewHeight(1, 2))
	batch.Put("ns1", "key3", []byte("value3"), version.NewHeight(1, 3))
	batch.Put("ns1", "key4", []byte("value4"), version.NewHeight(1, 4))
	batch.Put("ns1", "key5", []byte("value5"), version.NewHeight(1, 5))
	db.ApplyUpdates(batch, version.NewHeight(1, 5))

	validator := NewValidator(db)

	//rwset1 should be valid
	rwset1 := rwset.NewRWSet()
	rqi1 := &rwset.RangeQueryInfo{StartKey: "key2", EndKey: "key4", ItrExhausted: true}
	rqi1.AddResult(rwset.NewKVRead("key2", version.NewHeight(1, 2)))
	rqi1.AddResult(rwset.NewKVRead("key3", version.NewHeight(1, 3)))
	rwset1.AddToRangeQuerySet("ns1", rqi1)
	checkValidation(t, validator, []*rwset.RWSet{rwset1}, []int{})

	//rwset2 should not be valid - Version of key4 changed
	rwset2 := rwset.NewRWSet()
	rqi2 := &rwset.RangeQueryInfo{StartKey: "key2", EndKey: "key4", ItrExhausted: false}
	rqi2.AddResult(rwset.NewKVRead("key2", version.NewHeight(1, 2)))
	rqi2.AddResult(rwset.NewKVRead("key3", version.NewHeight(1, 3)))
	rqi2.AddResult(rwset.NewKVRead("key4", version.NewHeight(1, 3)))
	rwset2.AddToRangeQuerySet("ns1", rqi2)
	checkValidation(t, validator, []*rwset.RWSet{rwset2}, []int{1})

	//rwset3 should not be valid - simulate key3 got commited to db
	rwset3 := rwset.NewRWSet()
	rqi3 := &rwset.RangeQueryInfo{StartKey: "key2", EndKey: "key4", ItrExhausted: false}
	rqi3.AddResult(rwset.NewKVRead("key2", version.NewHeight(1, 2)))
	rqi3.AddResult(rwset.NewKVRead("key4", version.NewHeight(1, 4)))
	rwset3.AddToRangeQuerySet("ns1", rqi3)
	checkValidation(t, validator, []*rwset.RWSet{rwset3}, []int{1})

	// //Remove a key in rwset4 and rwset5 should become invalid
	rwset4 := rwset.NewRWSet()
	rwset4.AddToWriteSet("ns1", "key3", nil)
	rwset5 := rwset.NewRWSet()
	rqi5 := &rwset.RangeQueryInfo{StartKey: "key2", EndKey: "key4", ItrExhausted: false}
	rqi5.AddResult(rwset.NewKVRead("key2", version.NewHeight(1, 2)))
	rqi5.AddResult(rwset.NewKVRead("key3", version.NewHeight(1, 3)))
	rqi5.AddResult(rwset.NewKVRead("key4", version.NewHeight(1, 4)))
	rwset5.AddToRangeQuerySet("ns1", rqi5)
	checkValidation(t, validator, []*rwset.RWSet{rwset4, rwset5}, []int{1})

	//Add a key in rwset6 and rwset7 should become invalid
	rwset6 := rwset.NewRWSet()
	rwset6.AddToWriteSet("ns1", "key2_1", []byte("value2_1"))
	rwset7 := rwset.NewRWSet()
	rqi7 := &rwset.RangeQueryInfo{StartKey: "key2", EndKey: "key4", ItrExhausted: false}
	rqi7.AddResult(rwset.NewKVRead("key2", version.NewHeight(1, 2)))
	rqi7.AddResult(rwset.NewKVRead("key3", version.NewHeight(1, 3)))
	rqi7.AddResult(rwset.NewKVRead("key4", version.NewHeight(1, 4)))
	rwset7.AddToRangeQuerySet("ns1", rqi7)
	checkValidation(t, validator, []*rwset.RWSet{rwset6, rwset7}, []int{1})
}

func checkValidation(t *testing.T, validator *Validator, rwsets []*rwset.RWSet, invalidTxIndexes []int) {
	simulationResults := [][]byte{}
	for _, rwset := range rwsets {
		sr, err := rwset.GetTxReadWriteSet().Marshal()
		testutil.AssertNoError(t, err, "")
		simulationResults = append(simulationResults, sr)
	}
	block := testutil.ConstructBlock(t, simulationResults, false)
	_, err := validator.ValidateAndPrepareBatch(block, true)
	txsFltr := util.NewFilterBitArrayFromBytes(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])
	invalidTxNum := 0
	for i := 0; i < len(block.Data.Data); i++ {
		if txsFltr.IsSet(uint(i)) {
			invalidTxNum++
		}
	}

	testutil.AssertNoError(t, err, "")
	testutil.AssertEquals(t, invalidTxNum, len(invalidTxIndexes))
	//TODO Add the check for exact txnum that is marked invlid when bitarray is in place
}
