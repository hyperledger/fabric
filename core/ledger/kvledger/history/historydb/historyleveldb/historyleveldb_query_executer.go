/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package historyleveldb

import (
	commonledger "github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/common/ledger/blkstorage"
	"github.com/hyperledger/fabric/common/ledger/util"
	"github.com/hyperledger/fabric/core/ledger/kvledger/history/historydb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/ledger/queryresult"
	putils "github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
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
		return nil, errors.New("history database not enabled")
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

// Next iterates to the next key from history scanner, decodes blockNumTranNumBytes to get blockNum and tranNum,
// loads the block:tran from block storage, finds the key and returns the result.
// The history keys are in the format of <namespace, key, blockNum, tranNum>, separated by nil byte "\x00".
// There could be clashes in keys if some keys were to contain nil bytes. For example, the historydb keys for
// both <ns, key, 16777473, 1> and <ns, key\x00\x04\x01, 1, 1> are same.
// When two keys have clashes, the previous historydb entry will be overwritten and hence missing.
// However, in practice, we do not expect this to be a common scenario.
// In a rare scenario, the decoded blockNum:tranNum may have a key in the write-set while the entry in the historydb
// was actually added for some other <ns, key, blockNum, tranNum>. It would cause this iterator to
// return a history query result out of the order.
func (scanner *historyScanner) Next() (commonledger.QueryResult, error) {
	for {
		if !scanner.dbItr.Next() {
			return nil, nil
		}
		historyKey := scanner.dbItr.Key() // history key is in the form namespace~key~blocknum~trannum

		// SplitCompositeKey(namespace~key~blocknum~trannum, namespace~key~) will return the blocknum~trannum in second position
		_, blockNumTranNumBytes := historydb.SplitCompositeHistoryKey(historyKey, scanner.compositePartialKey)

		//
		// FAB-15450
		// There may be false keys because a key may have nil byte(s).
		// Take an example of two keys "key" and "key\x00" in a namespace ns. The entries for these keys will be
		// of type "ns-\x00-key-\x00-blockNumTranNumBytes" and ns-\x00-key-\x00-\x00-blockNumTranNumBytes respectively.
		// "-" in above examples are just for readability. Further, when scanning the range
		// {ns-\x00-key-\x00 - ns-\x00-key-xff} for getting the history for <ns, key>, the entries for "key\x00" also
		// fall in the range and will be returned in range query.
		//
		// Meanwhile a valid blockNumTranNumBytes may also contain nil bytes. Therefore, we use the following approach
		// to verify and skip false keys.
		// If blockNumTranNumBytes cannot be decoded, it means that it is a false key and will be skipped.
		// If blockNumTranNumBytes can be decoded, we further verify that the block:tran can be found and
		// the key is present in the write set. If not, it is a false key.
		//
		// Note: in some scenarios, this can map to a block:tran in the block storage that contains the key
		// but is out of order of iteration and hence the results are not guaranteed to be in order.
		blockNum, tranNum, err := decodeBlockNumTranNum(blockNumTranNumBytes)
		if err != nil {
			logger.Warnf("Some other key [%#v] found in the range while scanning history for key [%#v]. Skipping (decoding error: %s)",
				historyKey, scanner.key, err)
			continue
		}

		logger.Debugf("Found history record for namespace:%s key:%s at blockNumTranNum %v:%v\n",
			scanner.namespace, scanner.key, blockNum, tranNum)

		// Get the transaction from block storage that is associated with this history record
		tranEnvelope, err := scanner.blockStore.RetrieveTxByBlockNumTranNum(blockNum, tranNum)
		if err == blkstorage.ErrNotFoundInIndex {
			logger.Warnf("Some other clashing key [%#v] found in the range while scanning history for key [%#v]. Skipping (cannot find block:tx)",
				historyKey, scanner.key)
			continue
		}
		if err != nil {
			return nil, err
		}

		// Get the txid, key write value, timestamp, and delete indicator associated with this transaction
		queryResult, err := getKeyModificationFromTran(tranEnvelope, scanner.namespace, scanner.key)
		if err != nil {
			return nil, err
		}
		if queryResult == nil {
			// no namespace or key is found, so it is a false key.
			// This may happen if a false key "ns, key\x00..., <otherBlockNum>, <otherTranNum>" is returned
			// in range query for "key" and its '...\x00<otherBlockNum><otherTranNum>' portion can be decoded to
			// valid blockNum:tranNum; however, the decoded blockNum:tranNum does not have the desired namespace/key.
			logger.Warnf("Some other key [%#v] found in the range while scanning history for key [%#v]. Skipping (namespace or key not found)",
				historyKey, scanner.key)
			continue
		}
		logger.Debugf("Found historic key value for namespace:%s key:%s from transaction %s",
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
			logger.Debugf("key [%s] not found in namespace [%s]'s writeset", key, namespace)
			return nil, nil
		} // end if
	} //end namespaces loop
	logger.Debugf("namespace [%s] not found in transaction's ReadWriteSets", namespace)
	return nil, nil
}

// decodeBlockNumTranNum decodes blockNumTranNumBytes to get blockNum and tranNum.
func decodeBlockNumTranNum(blockNumTranNumBytes []byte) (uint64, uint64, error) {
	blockNum, blockBytesConsumed, err := util.DecodeOrderPreservingVarUint64(blockNumTranNumBytes)
	if err != nil {
		return 0, 0, err
	}

	tranNum, tranBytesConsumed, err := util.DecodeOrderPreservingVarUint64(blockNumTranNumBytes[blockBytesConsumed:])
	if err != nil {
		return 0, 0, err
	}

	// blockBytesConsumed + tranBytesConsumed should be equal to blockNumTranNumBytes for a real key match
	if blockBytesConsumed+tranBytesConsumed != len(blockNumTranNumBytes) {
		return 0, 0, errors.Errorf("number of decoded bytes (%d) is not equal to the length of blockNumTranNumBytes (%d)",
			blockBytesConsumed+tranBytesConsumed, len(blockNumTranNumBytes))
	}

	return blockNum, tranNum, nil
}
