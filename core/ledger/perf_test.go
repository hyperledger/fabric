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

package ledger

import (
	"flag"
	"strconv"
	"testing"
	"time"

	"github.com/hyperledger/fabric/core/db"
	"github.com/hyperledger/fabric/core/ledger/perfstat"
	"github.com/hyperledger/fabric/core/ledger/testutil"
	"github.com/hyperledger/fabric/core/util"
	"github.com/hyperledger/fabric/protos"
	"github.com/op/go-logging"
	"github.com/tecbot/gorocksdb"
)

func BenchmarkDB(b *testing.B) {
	b.Logf("testParams:%q", testParams)
	flags := flag.NewFlagSet("testParams", flag.ExitOnError)
	kvSize := flags.Int("KVSize", 1000, "size of the key-value")
	toPopulateDB := flags.Bool("PopulateDB", false, "Run in populate DB mode")
	maxKeySuffix := flags.Int("MaxKeySuffix", 1, "the keys are appended with _1, _2,.. upto MaxKeySuffix")
	keyPrefix := flags.String("KeyPrefix", "Key_", "The generated workload will have keys such as KeyPrefix_1, KeyPrefix_2, and so on")
	flags.Parse(testParams)
	if *toPopulateDB {
		b.ResetTimer()
		populateDB(b, *kvSize, *maxKeySuffix, *keyPrefix)
		return
	}

	dbWrapper := db.NewTestDBWrapper()
	randNumGen := testutil.NewTestRandomNumberGenerator(*maxKeySuffix)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := []byte(*keyPrefix + strconv.Itoa(randNumGen.Next()))
		value := dbWrapper.GetFromDB(b, key)
		b.SetBytes(int64(len(value)))
	}
}

func BenchmarkLedgerSingleKeyTransaction(b *testing.B) {
	b.Logf("testParams:%q", testParams)
	flags := flag.NewFlagSet("testParams", flag.ExitOnError)
	key := flags.String("Key", "key", "key name")
	kvSize := flags.Int("KVSize", 1000, "size of the key-value")
	batchSize := flags.Int("BatchSize", 100, "size of the key-value")
	numBatches := flags.Int("NumBatches", 100, "number of batches")
	numWritesToLedger := flags.Int("NumWritesToLedger", 4, "size of the key-value")
	flags.Parse(testParams)

	b.Logf(`Running test with params: key=%s, kvSize=%d, batchSize=%d, numBatches=%d, NumWritesToLedger=%d`,
		*key, *kvSize, *batchSize, *numBatches, *numWritesToLedger)

	ledgerTestWrapper := createFreshDBAndTestLedgerWrapper(b)
	ledger := ledgerTestWrapper.ledger

	chaincode := "chaincodeId"
	value := testutil.ConstructRandomBytes(b, *kvSize-(len(chaincode)+len(*key)))
	tx := constructDummyTx(b)
	serializedBytes, _ := tx.Bytes()
	b.Logf("Size of serialized bytes for tx = %d", len(serializedBytes))
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		for i := 0; i < *numBatches; i++ {
			ledger.BeginTxBatch(1)
			// execute one batch
			var transactions []*protos.Transaction
			for j := 0; j < *batchSize; j++ {
				ledger.TxBegin("txUuid")
				_, err := ledger.GetState(chaincode, *key, true)
				if err != nil {
					b.Fatalf("Error in getting state: %s", err)
				}
				for l := 0; l < *numWritesToLedger; l++ {
					ledger.SetState(chaincode, *key, value)
				}
				ledger.TxFinished("txUuid", true)
				transactions = append(transactions, tx)
			}
			ledger.CommitTxBatch(1, transactions, nil, []byte("proof"))
		}
	}
	b.StopTimer()

	//verify value persisted
	value, _ = ledger.GetState(chaincode, *key, true)
	size := ledger.GetBlockchainSize()
	b.Logf("Value size=%d, Blockchain height=%d", len(value), size)
}

func BenchmarkLedgerPopulate(b *testing.B) {
	b.Logf("testParams:%q", testParams)
	disableLogging()
	flags := flag.NewFlagSet("testParams", flag.ExitOnError)
	kvSize := flags.Int("KVSize", 1000, "size of the key-value")
	maxKeySuffix := flags.Int("MaxKeySuffix", 1, "the keys are appended with _1, _2,.. upto MaxKeySuffix")
	keyPrefix := flags.String("KeyPrefix", "Key_", "The generated workload will have keys such as KeyPrefix_1, KeyPrefix_2, and so on")
	batchSize := flags.Int("BatchSize", 100, "size of the key-value")
	flags.Parse(testParams)

	b.Logf(`Running test with params: keyPrefix=%s, kvSize=%d, batchSize=%d, maxKeySuffix=%d`,
		*keyPrefix, *kvSize, *batchSize, *maxKeySuffix)

	ledgerTestWrapper := createFreshDBAndTestLedgerWrapper(b)
	ledger := ledgerTestWrapper.ledger

	chaincode := "chaincodeId"
	numBatches := *maxKeySuffix / *batchSize
	tx := constructDummyTx(b)
	value := testutil.ConstructRandomBytes(b, *kvSize-(len(chaincode)+len(*keyPrefix)))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for batchID := 0; batchID < numBatches; batchID++ {
			ledger.BeginTxBatch(1)
			// execute one batch
			var transactions []*protos.Transaction
			for j := 0; j < *batchSize; j++ {
				ledger.TxBegin("txUuid")
				keyNumber := batchID*(*batchSize) + j
				key := *keyPrefix + strconv.Itoa(keyNumber)
				ledger.SetState(chaincode, key, value)
				ledger.TxFinished("txUuid", true)
				transactions = append(transactions, tx)
			}
			ledger.CommitTxBatch(1, transactions, nil, []byte("proof"))
		}
	}
	b.StopTimer()
	b.Logf("DB stats afters populating: %s", testDBWrapper.GetEstimatedNumKeys(b))
}

func BenchmarkLedgerRandomTransactions(b *testing.B) {
	disableLogging()
	b.Logf("testParams:%q", testParams)
	flags := flag.NewFlagSet("testParams", flag.ExitOnError)
	keyPrefix := flags.String("KeyPrefix", "Key_", "The generated workload will have keys such as KeyPrefix_1, KeyPrefix_2, and so on")
	kvSize := flags.Int("KVSize", 1000, "size of the key-value")
	maxKeySuffix := flags.Int("MaxKeySuffix", 1, "the keys are appended with _1, _2,.. upto MaxKeySuffix")
	batchSize := flags.Int("BatchSize", 100, "size of the key-value")
	numBatches := flags.Int("NumBatches", 100, "number of batches")
	numReadsFromLedger := flags.Int("NumReadsFromLedger", 4, "Number of Key-Values to read")
	numWritesToLedger := flags.Int("NumWritesToLedger", 4, "Number of Key-Values to write")
	flags.Parse(testParams)

	b.Logf(`Running test with params: keyPrefix=%s, kvSize=%d, batchSize=%d, maxKeySuffix=%d, numBatches=%d, numReadsFromLedger=%d, numWritesToLedger=%d`,
		*keyPrefix, *kvSize, *batchSize, *maxKeySuffix, *numBatches, *numReadsFromLedger, *numWritesToLedger)

	ledger, err := GetNewLedger()
	testutil.AssertNoError(b, err, "Error while constructing ledger")

	chaincode := "chaincodeId"
	tx := constructDummyTx(b)
	value := testutil.ConstructRandomBytes(b, *kvSize-(len(chaincode)+len(*keyPrefix)))

	b.ResetTimer()
	startTime := time.Now()
	for i := 0; i < b.N; i++ {
		for batchID := 0; batchID < *numBatches; batchID++ {
			ledger.BeginTxBatch(1)
			// execute one batch
			var transactions []*protos.Transaction
			for j := 0; j < *batchSize; j++ {
				randomKeySuffixGen := testutil.NewTestRandomNumberGenerator(*maxKeySuffix)
				ledger.TxBegin("txUuid")
				for k := 0; k < *numReadsFromLedger; k++ {
					randomKey := *keyPrefix + strconv.Itoa(randomKeySuffixGen.Next())
					ledger.GetState(chaincode, randomKey, true)
				}
				for k := 0; k < *numWritesToLedger; k++ {
					randomKey := *keyPrefix + strconv.Itoa(randomKeySuffixGen.Next())
					ledger.SetState(chaincode, randomKey, value)
				}
				ledger.TxFinished("txUuid", true)
				transactions = append(transactions, tx)
			}
			ledger.CommitTxBatch(1, transactions, nil, []byte("proof"))
		}
	}
	b.StopTimer()
	perfstat.UpdateTimeStat("timeSpent", startTime)
	perfstat.PrintStats()
	b.Logf("DB stats afters populating: %s", testDBWrapper.GetEstimatedNumKeys(b))
}

func populateDB(tb testing.TB, kvSize int, totalKeys int, keyPrefix string) {
	dbWrapper := db.NewTestDBWrapper()
	dbWrapper.CleanDB(tb)
	batch := gorocksdb.NewWriteBatch()
	for i := 0; i < totalKeys; i++ {
		key := []byte(keyPrefix + strconv.Itoa(i))
		value := testutil.ConstructRandomBytes(tb, kvSize-len(key))
		batch.Put(key, value)
		if i%1000 == 0 {
			dbWrapper.WriteToDB(tb, batch)
			batch = gorocksdb.NewWriteBatch()
		}
	}
	dbWrapper.CloseDB(tb)
}

func constructDummyTx(tb testing.TB) *protos.Transaction {
	uuid := util.GenerateUUID()
	tx, err := protos.NewTransaction(protos.ChaincodeID{Path: "dummyChaincodeId"}, uuid, "dummyFunction", []string{"dummyParamValue1, dummyParamValue2"})
	testutil.AssertNil(tb, err)
	return tx
}

func disableLogging() {
	testutil.SetLogLevel(logging.ERROR, "indexes")
	testutil.SetLogLevel(logging.ERROR, "ledger")
	testutil.SetLogLevel(logging.INFO, "state")
	testutil.SetLogLevel(logging.ERROR, "statemgmt")
	testutil.SetLogLevel(logging.INFO, "buckettree")
	testutil.SetLogLevel(logging.INFO, "db")
}
