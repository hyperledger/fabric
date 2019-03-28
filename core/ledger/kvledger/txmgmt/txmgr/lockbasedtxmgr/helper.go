/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package lockbasedtxmgr

import (
	"fmt"

	commonledger "github.com/hyperledger/fabric/common/ledger"
	ledger "github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/storageutil"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/txmgr"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/protos/ledger/queryresult"
	"github.com/hyperledger/fabric/protos/ledger/rwset/kvrwset"
	"github.com/pkg/errors"
)

type queryHelper struct {
	txmgr             *LockBasedTxMgr
	collNameValidator *collNameValidator
	rwsetBuilder      *rwsetutil.RWSetBuilder
	itrs              []*resultsItr
	err               error
	doneInvoked       bool
}

func newQueryHelper(txmgr *LockBasedTxMgr, rwsetBuilder *rwsetutil.RWSetBuilder) *queryHelper {
	helper := &queryHelper{txmgr: txmgr, rwsetBuilder: rwsetBuilder}
	validator := newCollNameValidator(txmgr.ccInfoProvider, &lockBasedQueryExecutor{helper: helper})
	helper.collNameValidator = validator
	return helper
}

func (h *queryHelper) getState(ns string, key string) ([]byte, []byte, error) {
	if err := h.checkDone(); err != nil {
		return nil, nil, err
	}
	versionedValue, err := h.txmgr.db.GetState(ns, key)
	if err != nil {
		return nil, nil, err
	}
	val, metadata, ver := decomposeVersionedValue(versionedValue)
	if h.rwsetBuilder != nil {
		h.rwsetBuilder.AddToReadSet(ns, key, ver)
	}
	return val, metadata, nil
}

func (h *queryHelper) getStateMultipleKeys(namespace string, keys []string) ([][]byte, error) {
	if err := h.checkDone(); err != nil {
		return nil, err
	}
	versionedValues, err := h.txmgr.db.GetStateMultipleKeys(namespace, keys)
	if err != nil {
		return nil, nil
	}
	values := make([][]byte, len(versionedValues))
	for i, versionedValue := range versionedValues {
		val, _, ver := decomposeVersionedValue(versionedValue)
		if h.rwsetBuilder != nil {
			h.rwsetBuilder.AddToReadSet(namespace, keys[i], ver)
		}
		values[i] = val
	}
	return values, nil
}

func (h *queryHelper) getStateRangeScanIterator(namespace string, startKey string, endKey string) (ledger.QueryResultsIterator, error) {
	if err := h.checkDone(); err != nil {
		return nil, err
	}
	itr, err := newResultsItr(namespace, startKey, endKey, nil, h.txmgr.db, h.rwsetBuilder,
		ledgerconfig.IsQueryReadsHashingEnabled(), ledgerconfig.GetMaxDegreeQueryReadsHashing())
	if err != nil {
		return nil, err
	}
	h.itrs = append(h.itrs, itr)
	return itr, nil
}

func (h *queryHelper) getStateRangeScanIteratorWithMetadata(namespace string, startKey string, endKey string, metadata map[string]interface{}) (ledger.QueryResultsIterator, error) {
	if err := h.checkDone(); err != nil {
		return nil, err
	}
	itr, err := newResultsItr(namespace, startKey, endKey, metadata, h.txmgr.db, h.rwsetBuilder,
		ledgerconfig.IsQueryReadsHashingEnabled(), ledgerconfig.GetMaxDegreeQueryReadsHashing())
	if err != nil {
		return nil, err
	}
	h.itrs = append(h.itrs, itr)
	return itr, nil
}

func (h *queryHelper) executeQuery(namespace, query string) (commonledger.ResultsIterator, error) {
	if err := h.checkDone(); err != nil {
		return nil, err
	}
	dbItr, err := h.txmgr.db.ExecuteQuery(namespace, query)
	if err != nil {
		return nil, err
	}
	return &queryResultsItr{DBItr: dbItr, RWSetBuilder: h.rwsetBuilder}, nil
}

func (h *queryHelper) executeQueryWithMetadata(namespace, query string, metadata map[string]interface{}) (ledger.QueryResultsIterator, error) {
	if err := h.checkDone(); err != nil {
		return nil, err
	}
	dbItr, err := h.txmgr.db.ExecuteQueryWithMetadata(namespace, query, metadata)
	if err != nil {
		return nil, err
	}
	return &queryResultsItr{DBItr: dbItr, RWSetBuilder: h.rwsetBuilder}, nil
}

func (h *queryHelper) getPrivateData(ns, coll, key string) ([]byte, error) {
	if err := h.validateCollName(ns, coll); err != nil {
		return nil, err
	}
	if err := h.checkDone(); err != nil {
		return nil, err
	}

	var err error
	var hashVersion *version.Height
	var versionedValue *statedb.VersionedValue

	if versionedValue, err = h.txmgr.db.GetPrivateData(ns, coll, key); err != nil {
		return nil, err
	}

	// metadata is always nil for private data - because, the metadata is part of the hashed key (instead of raw key)
	val, _, ver := decomposeVersionedValue(versionedValue)

	keyHash := util.ComputeStringHash(key)
	if hashVersion, err = h.txmgr.db.GetKeyHashVersion(ns, coll, keyHash); err != nil {
		return nil, err
	}
	if !version.AreSame(hashVersion, ver) {
		return nil, &txmgr.ErrPvtdataNotAvailable{Msg: fmt.Sprintf(
			"private data matching public hash version is not available. Public hash version = %s, Private data version = %s",
			hashVersion, ver)}
	}
	if h.rwsetBuilder != nil {
		h.rwsetBuilder.AddToHashedReadSet(ns, coll, key, ver)
	}
	return val, nil
}

func (h *queryHelper) getPrivateDataValueHash(ns, coll, key string) (valueHash, metadataBytes []byte, err error) {
	if err := h.validateCollName(ns, coll); err != nil {
		return nil, nil, err
	}
	if err := h.checkDone(); err != nil {
		return nil, nil, err
	}
	var versionedValue *statedb.VersionedValue

	keyHash := util.ComputeStringHash(key)
	if versionedValue, err = h.txmgr.db.GetValueHash(ns, coll, keyHash); err != nil {
		return nil, nil, err
	}
	valHash, metadata, ver := decomposeVersionedValue(versionedValue)
	if h.rwsetBuilder != nil {
		h.rwsetBuilder.AddToHashedReadSet(ns, coll, key, ver)
	}
	return valHash, metadata, nil
}

func (h *queryHelper) getPrivateDataMultipleKeys(ns, coll string, keys []string) ([][]byte, error) {
	if err := h.validateCollName(ns, coll); err != nil {
		return nil, err
	}
	if err := h.checkDone(); err != nil {
		return nil, err
	}
	versionedValues, err := h.txmgr.db.GetPrivateDataMultipleKeys(ns, coll, keys)
	if err != nil {
		return nil, nil
	}
	values := make([][]byte, len(versionedValues))
	for i, versionedValue := range versionedValues {
		val, _, ver := decomposeVersionedValue(versionedValue)
		if h.rwsetBuilder != nil {
			h.rwsetBuilder.AddToHashedReadSet(ns, coll, keys[i], ver)
		}
		values[i] = val
	}
	return values, nil
}

func (h *queryHelper) getPrivateDataRangeScanIterator(namespace, collection, startKey, endKey string) (commonledger.ResultsIterator, error) {
	if err := h.validateCollName(namespace, collection); err != nil {
		return nil, err
	}
	if err := h.checkDone(); err != nil {
		return nil, err
	}
	dbItr, err := h.txmgr.db.GetPrivateDataRangeScanIterator(namespace, collection, startKey, endKey)
	if err != nil {
		return nil, err
	}
	return &pvtdataResultsItr{namespace, collection, dbItr}, nil
}

func (h *queryHelper) executeQueryOnPrivateData(namespace, collection, query string) (commonledger.ResultsIterator, error) {
	if err := h.validateCollName(namespace, collection); err != nil {
		return nil, err
	}
	if err := h.checkDone(); err != nil {
		return nil, err
	}
	dbItr, err := h.txmgr.db.ExecuteQueryOnPrivateData(namespace, collection, query)
	if err != nil {
		return nil, err
	}
	return &pvtdataResultsItr{namespace, collection, dbItr}, nil
}

func (h *queryHelper) getStateMetadata(ns string, key string) (map[string][]byte, error) {
	if err := h.checkDone(); err != nil {
		return nil, err
	}
	var metadataBytes []byte
	var err error
	if h.rwsetBuilder == nil {
		// reads versions are not getting recorded, retrieve metadata value via optimized path
		if metadataBytes, err = h.txmgr.db.GetStateMetadata(ns, key); err != nil {
			return nil, err
		}
	} else {
		if _, metadataBytes, err = h.getState(ns, key); err != nil {
			return nil, err
		}
	}
	return storageutil.DeserializeMetadata(metadataBytes)
}

func (h *queryHelper) getPrivateDataMetadata(ns, coll, key string) (map[string][]byte, error) {
	if h.rwsetBuilder == nil {
		// reads versions are not getting recorded, retrieve metadata value via optimized path
		return h.getPrivateDataMetadataByHash(ns, coll, util.ComputeStringHash(key))
	}
	if err := h.validateCollName(ns, coll); err != nil {
		return nil, err
	}
	if err := h.checkDone(); err != nil {
		return nil, err
	}
	_, metadataBytes, err := h.getPrivateDataValueHash(ns, coll, key)
	if err != nil {
		return nil, err
	}
	return storageutil.DeserializeMetadata(metadataBytes)
}

func (h *queryHelper) getPrivateDataMetadataByHash(ns, coll string, keyhash []byte) (map[string][]byte, error) {
	if err := h.validateCollName(ns, coll); err != nil {
		return nil, err
	}
	if err := h.checkDone(); err != nil {
		return nil, err
	}
	if h.rwsetBuilder != nil {
		// this requires to improve rwset builder to accept a keyhash
		return nil, errors.New("retrieving private data metadata by keyhash is not supported in simulation. This function is only available for query as yet")
	}
	metadataBytes, err := h.txmgr.db.GetPrivateDataMetadataByHash(ns, coll, keyhash)
	if err != nil {
		return nil, err
	}
	return storageutil.DeserializeMetadata(metadataBytes)
}

func (h *queryHelper) done() {
	if h.doneInvoked {
		return
	}

	defer func() {
		h.txmgr.commitRWLock.RUnlock()
		h.doneInvoked = true
		for _, itr := range h.itrs {
			itr.Close()
		}
	}()
}

func (h *queryHelper) addRangeQueryInfo() {
	for _, itr := range h.itrs {
		if h.rwsetBuilder != nil {
			results, hash, err := itr.rangeQueryResultsHelper.Done()
			if err != nil {
				h.err = err
				return
			}
			if results != nil {
				itr.rangeQueryInfo.SetRawReads(results)
			}
			if hash != nil {
				itr.rangeQueryInfo.SetMerkelSummary(hash)
			}
			h.rwsetBuilder.AddToRangeQuerySet(itr.ns, itr.rangeQueryInfo)
		}
	}
}

func (h *queryHelper) checkDone() error {
	if h.doneInvoked {
		return errors.New("this instance should not be used after calling Done()")
	}
	return nil
}

func (h *queryHelper) validateCollName(ns, coll string) error {
	return h.collNameValidator.validateCollName(ns, coll)
}

// resultsItr implements interface ledger.ResultsIterator
// this wraps the actual db iterator and intercept the calls
// to build rangeQueryInfo in the ReadWriteSet that is used
// for performing phantom read validation during commit
type resultsItr struct {
	ns                      string
	endKey                  string
	dbItr                   statedb.ResultsIterator
	rwSetBuilder            *rwsetutil.RWSetBuilder
	rangeQueryInfo          *kvrwset.RangeQueryInfo
	rangeQueryResultsHelper *rwsetutil.RangeQueryResultsHelper
}

func newResultsItr(ns string, startKey string, endKey string, metadata map[string]interface{},
	db statedb.VersionedDB, rwsetBuilder *rwsetutil.RWSetBuilder, enableHashing bool, maxDegree uint32) (*resultsItr, error) {
	var err error
	var dbItr statedb.ResultsIterator
	if metadata == nil {
		dbItr, err = db.GetStateRangeScanIterator(ns, startKey, endKey)
	} else {
		dbItr, err = db.GetStateRangeScanIteratorWithMetadata(ns, startKey, endKey, metadata)
	}
	if err != nil {
		return nil, err
	}
	itr := &resultsItr{ns: ns, dbItr: dbItr}
	// it's a simulation request so, enable capture of range query info
	if rwsetBuilder != nil {
		itr.rwSetBuilder = rwsetBuilder
		itr.endKey = endKey
		// just set the StartKey... set the EndKey later below in the Next() method.
		itr.rangeQueryInfo = &kvrwset.RangeQueryInfo{StartKey: startKey}
		resultsHelper, err := rwsetutil.NewRangeQueryResultsHelper(enableHashing, maxDegree)
		if err != nil {
			return nil, err
		}
		itr.rangeQueryResultsHelper = resultsHelper
	}
	return itr, nil
}

// Next implements method in interface ledger.ResultsIterator
// Before returning the next result, update the EndKey and ItrExhausted in rangeQueryInfo
// If we set the EndKey in the constructor (as we do for the StartKey) to what is
// supplied in the original query, we may be capturing the unnecessary longer range if the
// caller decides to stop iterating at some intermediate point. Alternatively, we could have
// set the EndKey and ItrExhausted in the Close() function but it may not be desirable to change
// transactional behaviour based on whether the Close() was invoked or not
func (itr *resultsItr) Next() (commonledger.QueryResult, error) {
	queryResult, err := itr.dbItr.Next()
	if err != nil {
		return nil, err
	}
	itr.updateRangeQueryInfo(queryResult)
	if queryResult == nil {
		return nil, nil
	}
	versionedKV := queryResult.(*statedb.VersionedKV)
	return &queryresult.KV{Namespace: versionedKV.Namespace, Key: versionedKV.Key, Value: versionedKV.Value}, nil
}

// GetBookmarkAndClose implements method in interface ledger.ResultsIterator
func (itr *resultsItr) GetBookmarkAndClose() string {
	returnBookmark := ""
	if queryResultIterator, ok := itr.dbItr.(statedb.QueryResultsIterator); ok {
		returnBookmark = queryResultIterator.GetBookmarkAndClose()
	}
	return returnBookmark
}

// updateRangeQueryInfo updates two attributes of the rangeQueryInfo
// 1) The EndKey - set to either a) latest key that is to be returned to the caller (if the iterator is not exhausted)
//                                  because, we do not know if the caller is again going to invoke Next() or not.
//                            or b) the last key that was supplied in the original query (if the iterator is exhausted)
// 2) The ItrExhausted - set to true if the iterator is going to return nil as a result of the Next() call
func (itr *resultsItr) updateRangeQueryInfo(queryResult statedb.QueryResult) {
	if itr.rwSetBuilder == nil {
		return
	}

	if queryResult == nil {
		// caller scanned till the iterator got exhausted.
		// So, set the endKey to the actual endKey supplied in the query
		itr.rangeQueryInfo.ItrExhausted = true
		itr.rangeQueryInfo.EndKey = itr.endKey
		return
	}
	versionedKV := queryResult.(*statedb.VersionedKV)
	itr.rangeQueryResultsHelper.AddResult(rwsetutil.NewKVRead(versionedKV.Key, versionedKV.Version))
	// Set the end key to the latest key retrieved by the caller.
	// Because, the caller may actually not invoke the Next() function again
	itr.rangeQueryInfo.EndKey = versionedKV.Key
}

// Close implements method in interface ledger.ResultsIterator
func (itr *resultsItr) Close() {
	itr.dbItr.Close()
}

type queryResultsItr struct {
	DBItr        statedb.ResultsIterator
	RWSetBuilder *rwsetutil.RWSetBuilder
}

// Next implements method in interface ledger.ResultsIterator
func (itr *queryResultsItr) Next() (commonledger.QueryResult, error) {

	queryResult, err := itr.DBItr.Next()
	if err != nil {
		return nil, err
	}
	if queryResult == nil {
		return nil, nil
	}
	versionedQueryRecord := queryResult.(*statedb.VersionedKV)
	logger.Debugf("queryResultsItr.Next() returned a record:%s", string(versionedQueryRecord.Value))

	if itr.RWSetBuilder != nil {
		itr.RWSetBuilder.AddToReadSet(versionedQueryRecord.Namespace, versionedQueryRecord.Key, versionedQueryRecord.Version)
	}
	return &queryresult.KV{Namespace: versionedQueryRecord.Namespace, Key: versionedQueryRecord.Key, Value: versionedQueryRecord.Value}, nil
}

// Close implements method in interface ledger.ResultsIterator
func (itr *queryResultsItr) Close() {
	itr.DBItr.Close()
}

func (itr *queryResultsItr) GetBookmarkAndClose() string {
	returnBookmark := ""
	if queryResultIterator, ok := itr.DBItr.(statedb.QueryResultsIterator); ok {
		returnBookmark = queryResultIterator.GetBookmarkAndClose()
	}
	return returnBookmark
}

func decomposeVersionedValue(versionedValue *statedb.VersionedValue) ([]byte, []byte, *version.Height) {
	var value []byte
	var metadata []byte
	var ver *version.Height
	if versionedValue != nil {
		value = versionedValue.Value
		ver = versionedValue.Version
		metadata = versionedValue.Metadata
	}
	return value, metadata, ver
}

// pvtdataResultsItr iterates over results of a query on pvt data
type pvtdataResultsItr struct {
	ns    string
	coll  string
	dbItr statedb.ResultsIterator
}

// Next implements method in interface ledger.ResultsIterator
func (itr *pvtdataResultsItr) Next() (commonledger.QueryResult, error) {
	queryResult, err := itr.dbItr.Next()
	if err != nil {
		return nil, err
	}
	if queryResult == nil {
		return nil, nil
	}
	versionedQueryRecord := queryResult.(*statedb.VersionedKV)
	return &queryresult.KV{
		Namespace: itr.ns,
		Key:       versionedQueryRecord.Key,
		Value:     versionedQueryRecord.Value,
	}, nil
}

// Close implements method in interface ledger.ResultsIterator
func (itr *pvtdataResultsItr) Close() {
	itr.dbItr.Close()
}
