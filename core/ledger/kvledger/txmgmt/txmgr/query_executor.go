/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package txmgr

import (
	"github.com/hyperledger/fabric-protos-go/ledger/queryresult"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset/kvrwset"
	commonledger "github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/internal/version"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statemetadata"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/pkg/errors"
)

const (
	queryReadsHashingEnabled   = true
	maxDegreeQueryReadsHashing = uint32(50)
)

// queryExecutor is a query executor used in `LockBasedTxMgr`
type queryExecutor struct {
	txmgr             *LockBasedTxMgr
	collNameValidator *collNameValidator
	collectReadset    bool
	rwsetBuilder      *rwsetutil.RWSetBuilder
	itrs              []*resultsItr
	err               error
	doneInvoked       bool
	hasher            rwsetutil.HashFunc
	txid              string
	privateReads      *ledger.PrivateReads
}

func newQueryExecutor(txmgr *LockBasedTxMgr,
	txid string,
	rwsetBuilder *rwsetutil.RWSetBuilder,
	performCollCheck bool,
	hashFunc rwsetutil.HashFunc) *queryExecutor {
	logger.Debugf("constructing new query executor txid = [%s]", txid)
	qe := &queryExecutor{}
	qe.txid = txid
	qe.txmgr = txmgr
	if rwsetBuilder != nil {
		qe.collectReadset = true
		qe.rwsetBuilder = rwsetBuilder
	}
	qe.hasher = hashFunc
	validator := newCollNameValidator(txmgr.ledgerid, txmgr.ccInfoProvider, qe, !performCollCheck)
	qe.collNameValidator = validator
	qe.privateReads = &ledger.PrivateReads{}
	return qe
}

// GetState implements method in interface `ledger.QueryExecutor`
func (q *queryExecutor) GetState(ns, key string) ([]byte, error) {
	val, _, err := q.getState(ns, key)
	return val, err
}

func (q *queryExecutor) getState(ns, key string) ([]byte, []byte, error) {
	if err := q.checkDone(); err != nil {
		return nil, nil, err
	}
	versionedValue, err := q.txmgr.db.GetState(ns, key)
	if err != nil {
		return nil, nil, err
	}
	val, metadata, ver := decomposeVersionedValue(versionedValue)
	if q.collectReadset {
		q.rwsetBuilder.AddToReadSet(ns, key, ver)
	}
	return val, metadata, nil
}

// GetStateMetadata implements method in interface `ledger.QueryExecutor`
func (q *queryExecutor) GetStateMetadata(ns, key string) (map[string][]byte, error) {
	if err := q.checkDone(); err != nil {
		return nil, err
	}
	var metadata []byte
	var err error
	if !q.collectReadset {
		if metadata, err = q.txmgr.db.GetStateMetadata(ns, key); err != nil {
			return nil, err
		}
	} else {
		if _, metadata, err = q.getState(ns, key); err != nil {
			return nil, err
		}
	}
	return statemetadata.Deserialize(metadata)
}

// GetStateMultipleKeys implements method in interface `ledger.QueryExecutor`
func (q *queryExecutor) GetStateMultipleKeys(ns string, keys []string) ([][]byte, error) {
	if err := q.checkDone(); err != nil {
		return nil, err
	}
	versionedValues, err := q.txmgr.db.GetStateMultipleKeys(ns, keys)
	if err != nil {
		return nil, nil
	}
	values := make([][]byte, len(versionedValues))
	for i, versionedValue := range versionedValues {
		val, _, ver := decomposeVersionedValue(versionedValue)
		if q.collectReadset {
			q.rwsetBuilder.AddToReadSet(ns, keys[i], ver)
		}
		values[i] = val
	}
	return values, nil
}

// GetStateRangeScanIterator implements method in interface `ledger.QueryExecutor`
// startKey is included in the results and endKey is excluded. An empty startKey refers to the first available key
// and an empty endKey refers to the last available key. For scanning all the keys, both the startKey and the endKey
// can be supplied as empty strings. However, a full scan should be used judiciously for performance reasons.
func (q *queryExecutor) GetStateRangeScanIterator(namespace string, startKey string, endKey string) (commonledger.ResultsIterator, error) {
	if err := q.checkDone(); err != nil {
		return nil, err
	}
	itr, err := newResultsItr(
		namespace,
		startKey,
		endKey,
		0,
		q.txmgr.db,
		q.rwsetBuilder,
		queryReadsHashingEnabled,
		maxDegreeQueryReadsHashing,
		q.hasher,
	)
	if err != nil {
		return nil, err
	}
	q.itrs = append(q.itrs, itr)
	return itr, nil
}

func (q *queryExecutor) GetStateRangeScanIteratorWithPagination(namespace string, startKey string, endKey string, pageSize int32) (ledger.QueryResultsIterator, error) {
	if err := q.checkDone(); err != nil {
		return nil, err
	}
	itr, err := newResultsItr(
		namespace,
		startKey,
		endKey,
		pageSize,
		q.txmgr.db,
		q.rwsetBuilder,
		queryReadsHashingEnabled,
		maxDegreeQueryReadsHashing,
		q.hasher,
	)
	if err != nil {
		return nil, err
	}
	q.itrs = append(q.itrs, itr)
	return itr, nil
}

// ExecuteQuery implements method in interface `ledger.QueryExecutor`
func (q *queryExecutor) ExecuteQuery(namespace, query string) (commonledger.ResultsIterator, error) {
	if err := q.checkDone(); err != nil {
		return nil, err
	}
	dbItr, err := q.txmgr.db.ExecuteQuery(namespace, query)
	if err != nil {
		return nil, err
	}
	return &queryResultsItr{DBItr: dbItr, RWSetBuilder: q.rwsetBuilder}, nil
}

func (q *queryExecutor) ExecuteQueryWithPagination(namespace, query, bookmark string, pageSize int32) (ledger.QueryResultsIterator, error) {
	if err := q.checkDone(); err != nil {
		return nil, err
	}
	dbItr, err := q.txmgr.db.ExecuteQueryWithPagination(namespace, query, bookmark, pageSize)
	if err != nil {
		return nil, err
	}
	return &queryResultsItr{DBItr: dbItr, RWSetBuilder: q.rwsetBuilder}, nil
}

// GetPrivateData implements method in interface `ledger.QueryExecutor`
func (q *queryExecutor) GetPrivateData(ns, coll, key string) ([]byte, error) {
	if err := q.validateCollName(ns, coll); err != nil {
		return nil, err
	}
	if err := q.checkDone(); err != nil {
		return nil, err
	}

	var err error
	var hashVersion *version.Height
	var versionedValue *statedb.VersionedValue

	if versionedValue, err = q.txmgr.db.GetPrivateData(ns, coll, key); err != nil {
		return nil, err
	}

	// metadata is always nil for private data - because, the metadata is part of the hashed key (instead of raw key)
	val, _, ver := decomposeVersionedValue(versionedValue)

	keyHash := util.ComputeStringHash(key)
	if hashVersion, err = q.txmgr.db.GetKeyHashVersion(ns, coll, keyHash); err != nil {
		return nil, err
	}
	if !version.AreSame(hashVersion, ver) {
		return nil, errors.Errorf(
			"private data matching public hash version is not available. Public hash version = %s, Private data version = %s",
			hashVersion, ver,
		)
	}
	if q.collectReadset {
		q.rwsetBuilder.AddToHashedReadSet(ns, coll, key, ver)
		q.privateReads.Add(ns, coll)
	}
	return val, nil
}

func (q *queryExecutor) GetPrivateDataHash(ns, coll, key string) ([]byte, error) {
	if err := q.validateCollName(ns, coll); err != nil {
		return nil, err
	}
	if err := q.checkDone(); err != nil {
		return nil, err
	}
	var versionedValue *statedb.VersionedValue
	var err error
	if versionedValue, err = q.txmgr.db.GetPrivateDataHash(ns, coll, key); err != nil {
		return nil, err
	}
	valHash, _, ver := decomposeVersionedValue(versionedValue)
	if q.collectReadset {
		q.rwsetBuilder.AddToHashedReadSet(ns, coll, key, ver)
	}
	return valHash, nil
}

// GetPrivateDataMetadata implements method in interface `ledger.QueryExecutor`
func (q *queryExecutor) GetPrivateDataMetadata(ns, coll, key string) (map[string][]byte, error) {
	if !q.collectReadset {
		// reads versions are not getting recorded, retrieve metadata value via optimized path
		return q.getPrivateDataMetadataByHash(ns, coll, util.ComputeStringHash(key))
	}
	if err := q.validateCollName(ns, coll); err != nil {
		return nil, err
	}
	if err := q.checkDone(); err != nil {
		return nil, err
	}
	_, metadataBytes, err := q.getPrivateDataValueHash(ns, coll, key)
	if err != nil {
		return nil, err
	}
	return statemetadata.Deserialize(metadataBytes)
}

func (q *queryExecutor) getPrivateDataValueHash(ns, coll, key string) (valueHash, metadataBytes []byte, err error) {
	if err := q.validateCollName(ns, coll); err != nil {
		return nil, nil, err
	}
	if err := q.checkDone(); err != nil {
		return nil, nil, err
	}
	var versionedValue *statedb.VersionedValue
	if versionedValue, err = q.txmgr.db.GetPrivateDataHash(ns, coll, key); err != nil {
		return nil, nil, err
	}
	valHash, metadata, ver := decomposeVersionedValue(versionedValue)
	if q.collectReadset {
		q.rwsetBuilder.AddToHashedReadSet(ns, coll, key, ver)
	}
	return valHash, metadata, nil
}

// GetPrivateDataMetadataByHash implements method in interface `ledger.QueryExecutor`
func (q *queryExecutor) GetPrivateDataMetadataByHash(ns, coll string, keyhash []byte) (map[string][]byte, error) {
	return q.getPrivateDataMetadataByHash(ns, coll, keyhash)
}

func (q *queryExecutor) getPrivateDataMetadataByHash(ns, coll string, keyhash []byte) (map[string][]byte, error) {
	if err := q.validateCollName(ns, coll); err != nil {
		return nil, err
	}
	if err := q.checkDone(); err != nil {
		return nil, err
	}
	if q.collectReadset {
		// this requires to improve rwset builder to accept a keyhash
		return nil, errors.New("retrieving private data metadata by keyhash is not supported in simulation. This function is only available for query as yet")
	}
	metadataBytes, err := q.txmgr.db.GetPrivateDataMetadataByHash(ns, coll, keyhash)
	if err != nil {
		return nil, err
	}
	return statemetadata.Deserialize(metadataBytes)
}

// GetPrivateDataMultipleKeys implements method in interface `ledger.QueryExecutor`
func (q *queryExecutor) GetPrivateDataMultipleKeys(ns, coll string, keys []string) ([][]byte, error) {
	if err := q.validateCollName(ns, coll); err != nil {
		return nil, err
	}
	if err := q.checkDone(); err != nil {
		return nil, err
	}
	versionedValues, err := q.txmgr.db.GetPrivateDataMultipleKeys(ns, coll, keys)
	if err != nil {
		return nil, nil
	}
	values := make([][]byte, len(versionedValues))
	for i, versionedValue := range versionedValues {
		val, _, ver := decomposeVersionedValue(versionedValue)
		if q.collectReadset {
			q.rwsetBuilder.AddToHashedReadSet(ns, coll, keys[i], ver)
			q.privateReads.Add(ns, coll)
		}
		values[i] = val
	}
	return values, nil
}

// GetPrivateDataRangeScanIterator implements method in interface `ledger.QueryExecutor`
func (q *queryExecutor) GetPrivateDataRangeScanIterator(ns, coll, startKey, endKey string) (commonledger.ResultsIterator, error) {
	if err := q.validateCollName(ns, coll); err != nil {
		return nil, err
	}
	if err := q.checkDone(); err != nil {
		return nil, err
	}
	dbItr, err := q.txmgr.db.GetPrivateDataRangeScanIterator(ns, coll, startKey, endKey)
	if err != nil {
		return nil, err
	}
	return &pvtdataResultsItr{ns, coll, dbItr}, nil
}

// ExecuteQueryOnPrivateData implements method in interface `ledger.QueryExecutor`
func (q *queryExecutor) ExecuteQueryOnPrivateData(ns, coll, query string) (commonledger.ResultsIterator, error) {
	if err := q.validateCollName(ns, coll); err != nil {
		return nil, err
	}
	if err := q.checkDone(); err != nil {
		return nil, err
	}
	dbItr, err := q.txmgr.db.ExecuteQueryOnPrivateData(ns, coll, query)
	if err != nil {
		return nil, err
	}
	return &pvtdataResultsItr{ns, coll, dbItr}, nil
}

// Done implements method in interface `ledger.QueryExecutor`
func (q *queryExecutor) Done() {
	logger.Debugf("Done with transaction simulation / query execution [%s]", q.txid)
	if q.doneInvoked {
		return
	}

	defer func() {
		q.txmgr.commitRWLock.RUnlock()
		q.doneInvoked = true
		for _, itr := range q.itrs {
			itr.Close()
		}
	}()
}

func (q *queryExecutor) checkDone() error {
	if q.doneInvoked {
		return errors.New("this instance should not be used after calling Done()")
	}
	return nil
}

func (q *queryExecutor) validateCollName(ns, coll string) error {
	return q.collNameValidator.validateCollName(ns, coll)
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

func newResultsItr(ns string, startKey string, endKey string, pageSize int32,
	db statedb.VersionedDB, rwsetBuilder *rwsetutil.RWSetBuilder, enableHashing bool,
	maxDegree uint32, hashFunc rwsetutil.HashFunc) (*resultsItr, error) {
	var err error
	var dbItr statedb.ResultsIterator
	if pageSize == 0 {
		dbItr, err = db.GetStateRangeScanIterator(ns, startKey, endKey)
	} else {
		dbItr, err = db.GetStateRangeScanIteratorWithPagination(ns, startKey, endKey, pageSize)
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
		resultsHelper, err := rwsetutil.NewRangeQueryResultsHelper(enableHashing, maxDegree, hashFunc)
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
	if err := itr.updateRangeQueryInfo(queryResult); err != nil {
		return nil, err
	}
	if queryResult == nil {
		return nil, nil
	}

	return &queryresult.KV{
		Namespace: queryResult.Namespace,
		Key:       queryResult.Key,
		Value:     queryResult.Value,
	}, nil
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
//  1. The EndKey - set to either a) latest key that is to be returned to the caller (if the iterator is not exhausted)
//     because, we do not know if the caller is again going to invoke Next() or not.
//     or b) the last key that was supplied in the original query (if the iterator is exhausted)
//  2. The ItrExhausted - set to true if the iterator is going to return nil as a result of the Next() call
func (itr *resultsItr) updateRangeQueryInfo(queryResult *statedb.VersionedKV) error {
	if itr.rwSetBuilder == nil {
		return nil
	}

	if queryResult == nil {
		// caller scanned till the iterator got exhausted.
		// So, set the endKey to the actual endKey supplied in the query
		itr.rangeQueryInfo.ItrExhausted = true
		itr.rangeQueryInfo.EndKey = itr.endKey
		return nil
	}

	if err := itr.rangeQueryResultsHelper.AddResult(rwsetutil.NewKVRead(queryResult.Key, queryResult.Version)); err != nil {
		return err
	}
	// Set the end key to the latest key retrieved by the caller.
	// Because, the caller may actually not invoke the Next() function again
	itr.rangeQueryInfo.EndKey = queryResult.Key
	return nil
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
	logger.Debugf("queryResultsItr.Next() returned a record:%s", string(queryResult.Value))

	if itr.RWSetBuilder != nil {
		itr.RWSetBuilder.AddToReadSet(queryResult.Namespace, queryResult.Key, queryResult.Version)
	}
	return &queryresult.KV{
		Namespace: queryResult.Namespace,
		Key:       queryResult.Key,
		Value:     queryResult.Value,
	}, nil
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
	return &queryresult.KV{
		Namespace: itr.ns,
		Key:       queryResult.Key,
		Value:     queryResult.Value,
	}, nil
}

// Close implements method in interface ledger.ResultsIterator
func (itr *pvtdataResultsItr) Close() {
	itr.dbItr.Close()
}

func (q *queryExecutor) addRangeQueryInfo() {
	if !q.collectReadset {
		return
	}
	for _, itr := range q.itrs {
		results, hash, err := itr.rangeQueryResultsHelper.Done()
		if err != nil {
			q.err = err
			return
		}
		if results != nil {
			rwsetutil.SetRawReads(itr.rangeQueryInfo, results)
		}
		if hash != nil {
			rwsetutil.SetMerkelSummary(itr.rangeQueryInfo, hash)
		}
		q.rwsetBuilder.AddToRangeQuerySet(itr.ns, itr.rangeQueryInfo)
	}
}
