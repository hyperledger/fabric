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
	"bytes"
	"errors"

	commonledger "github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/common/ledger/blkstorage"
	"github.com/hyperledger/fabric/common/ledger/util"
	"github.com/hyperledger/fabric/core/ledger/kvledger/history/historydb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/ledger/queryresult"
	putils "github.com/hyperledger/fabric/protos/utils"
	"github.com/syndtr/goleveldb/leveldb/iterator"
)

// LevelHistoryDBQueryExecutor is a query executor against the LevelDB history DB
type LevelHistoryDBQueryExecutor struct {
	historyDB  *historyDB
	blockStore blkstorage.BlockStore
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
	return newHistoryScanner(compositeStartKey, namespace, key, dbItr, q.blockStore), nil
}

//historyScanner implements ResultsIterator for iterating through history results
type historyScanner struct {
	compositePartialKey []byte //compositePartialKey includes namespace~key
	namespace           string
	key                 string
	dbItr               iterator.Iterator
	blockStore          blkstorage.BlockStore
}

func newHistoryScanner(compositePartialKey []byte, namespace string, key string,
	dbItr iterator.Iterator, blockStore blkstorage.BlockStore) *historyScanner {
	return &historyScanner{compositePartialKey, namespace, key, dbItr, blockStore}
}

func (scanner *historyScanner) Next() (commonledger.QueryResult, error) {
	for {
		if !scanner.dbItr.Next() {
			return nil, nil
		}
		historyKey := scanner.dbItr.Key() // history key is in the form namespace~key~blocknum~trannum

		// SplitCompositeKey(namespace~key~blocknum~trannum, namespace~key~) will return the blocknum~trannum in second position
		_, blockNumTranNumBytes := historydb.SplitCompositeHistoryKey(historyKey, scanner.compositePartialKey)

		// check that blockNumTranNumBytes does not contain a nil byte (FAB-11244) - except the last byte.
		// if this contains a nil byte that indicate that its a different key other than the one we are
		// scanning the history for. However, the last byte can be nil even for the valid key (indicating the transaction numer being zero)
		// This is because, if 'blockNumTranNumBytes' really is the suffix of the desired key - only possibility of this containing a nil byte
		// is the last byte when the transaction number in blockNumTranNumBytes is zero).
		// On the other hand, if 'blockNumTranNumBytes' really is NOT the suffix of the desired key, then this has to be a prefix
		// of some other key (other than the desired key) and in this case, there has to be at least one nil byte (other than the last byte),
		// for the 'last' CompositeKeySep in the composite key
		// Take an example of two keys "key" and "key\x00" in a namespace ns. The entries for these keys will be
		// of type "ns-\x00-key-\x00-blkNumTranNumBytes" and ns-\x00-key-\x00-\x00-blkNumTranNumBytes respectively.
		// "-" in above examples are just for readability. Further, when scanning the range
		// {ns-\x00-key-\x00 - ns-\x00-key-xff} for getting the history for <ns, key>, the entry for the other key
		// falls in the range and needs to be ignored
		if bytes.Contains(blockNumTranNumBytes[:len(blockNumTranNumBytes)-1], historydb.CompositeKeySep) {
			logger.Debugf("Some other key [%#v] found in the range while scanning history for key [%#v]. Skipping...",
				historyKey, scanner.key)
			continue
		}
		blockNum, bytesConsumed := util.DecodeOrderPreservingVarUint64(blockNumTranNumBytes[0:])
		tranNum, _ := util.DecodeOrderPreservingVarUint64(blockNumTranNumBytes[bytesConsumed:])
		logger.Debugf("Found history record for namespace:%s key:%s at blockNumTranNum %v:%v\n",
			scanner.namespace, scanner.key, blockNum, tranNum)

		// Get the transaction from block storage that is associated with this history record
		tranEnvelope, err := scanner.blockStore.RetrieveTxByBlockNumTranNum(blockNum, tranNum)
		if err != nil {
			return nil, err
		}

		// Get the txid, key write value, timestamp, and delete indicator associated with this transaction
		queryResult, err := getKeyModificationFromTran(tranEnvelope, scanner.namespace, scanner.key)
		if err != nil {
			return nil, err
		}
		logger.Debugf("Found historic key value for namespace:%s key:%s from transaction %s\n",
			scanner.namespace, scanner.key, queryResult.(*queryresult.KeyModification).TxId)
		return queryResult, nil
	}
}

func (scanner *historyScanner) Close() {
	scanner.dbItr.Release()
}

// getTxIDandKeyWriteValueFromTran inspects a transaction for writes to a given key
func getKeyModificationFromTran(tranEnvelope *common.Envelope, namespace string, key string) (commonledger.QueryResult, error) {
	logger.Debugf("Entering getKeyModificationFromTran()\n", namespace, key)

	// extract action from the envelope
	payload, err := putils.GetPayload(tranEnvelope)
	if err != nil {
		return nil, err
	}

	tx, err := putils.GetTransaction(payload.Data)
	if err != nil {
		return nil, err
	}

	_, respPayload, err := putils.GetPayloads(tx.Actions[0])
	if err != nil {
		return nil, err
	}

	chdr, err := putils.UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		return nil, err
	}

	txID := chdr.TxId
	timestamp := chdr.Timestamp

	txRWSet := &rwsetutil.TxRwSet{}

	// Get the Result from the Action and then Unmarshal
	// it into a TxReadWriteSet using custom unmarshalling
	if err = txRWSet.FromProtoBytes(respPayload.Results); err != nil {
		return nil, err
	}

	// look for the namespace and key by looping through the transaction's ReadWriteSets
	for _, nsRWSet := range txRWSet.NsRwSets {
		if nsRWSet.NameSpace == namespace {
			// got the correct namespace, now find the key write
			for _, kvWrite := range nsRWSet.KvRwSet.Writes {
				if kvWrite.Key == key {
					return &queryresult.KeyModification{TxId: txID, Value: kvWrite.Value,
						Timestamp: timestamp, IsDelete: kvWrite.IsDelete}, nil
				}
			} // end keys loop
			return nil, errors.New("Key not found in namespace's writeset")
		} // end if
	} //end namespaces loop
	return nil, errors.New("Namespace not found in transaction's ReadWriteSets")

}
