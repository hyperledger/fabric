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

package historyleveldb

import (
	"errors"
	"fmt"

	commonledger "github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/common/ledger/util"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/history/historydb"
	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
	"github.com/syndtr/goleveldb/leveldb/iterator"
)

// LevelHistoryDBQueryExecutor is a query executor against the LevelDB history DB
type LevelHistoryDBQueryExecutor struct {
	historyDB *historyDB
}

// GetHistoryForKey implements method in interface `ledger.HistoryQueryExecutor`
func (q *LevelHistoryDBQueryExecutor) GetHistoryForKey(namespace string, key string) (commonledger.ResultsIterator, error) {

	if ledgerconfig.IsHistoryDBEnabled() == false {
		return nil, errors.New("History tracking not enabled - historyDatabase is false")
	}

	var compositeStartKey []byte
	var compositeEndKey []byte
	compositeStartKey = historydb.ConstructPartialCompositeHistoryKey(namespace, key, false)
	compositeEndKey = historydb.ConstructPartialCompositeHistoryKey(namespace, key, true)

	// range scan to find any history records starting with namespace~key
	dbItr := q.historyDB.db.GetIterator(compositeStartKey, compositeEndKey)
	return newHistoryScanner(compositeStartKey, dbItr), nil
}

//historyScanner implements ResultsIterator for iterating through history results
type historyScanner struct {
	compositePartialKey []byte //compositePartialKey includes namespace~key
	dbItr               iterator.Iterator
}

func newHistoryScanner(compositePartialKey []byte, dbItr iterator.Iterator) *historyScanner {
	return &historyScanner{compositePartialKey, dbItr}
}

func (scanner *historyScanner) Next() (commonledger.QueryResult, error) {
	if !scanner.dbItr.Next() {
		return nil, nil
	}
	historyKey := scanner.dbItr.Key() // history key is in the form namespace~key~blocknum~trannum

	// SplitCompositeKey(namespace~key~blocknum~trannum, namespace~key~) will return the blocknum~trannum in second position
	_, blockNumTranNumBytes := historydb.SplitCompositeHistoryKey(historyKey, scanner.compositePartialKey)
	blockNum, bytesConsumed := util.DecodeOrderPreservingVarUint64(blockNumTranNumBytes[0:])
	tranNum, _ := util.DecodeOrderPreservingVarUint64(blockNumTranNumBytes[bytesConsumed:])

	blockNumTranNum := fmt.Sprintf("%v:%v", blockNum, tranNum)
	logger.Debugf("Got history record for key %s: %s\n", scanner.compositePartialKey, blockNumTranNum)

	// For initial test return the blockNumTranNum as TxID.
	// TODO query block storage to get and return the TxID and value
	return &ledger.KeyModification{TxID: blockNumTranNum}, nil
}

func (scanner *historyScanner) Close() {
	scanner.dbItr.Release()
}
