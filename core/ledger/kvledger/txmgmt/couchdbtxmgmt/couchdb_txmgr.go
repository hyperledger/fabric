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

package couchdbtxmgmt

import (
	"bytes"
	"encoding/json"
	"errors"
	"sync"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/util/couchdb"
	"github.com/hyperledger/fabric/core/ledger/util/db"
	"github.com/op/go-logging"

	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwset"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/protos/common"
	putils "github.com/hyperledger/fabric/protos/utils"
)

var logger = logging.MustGetLogger("couchdbtxmgmt")

var compositeKeySep = []byte{0x00}

// Conf - configuration for `CouchDBTxMgr`
type Conf struct {
	DBPath string
}

type versionedValue struct {
	value   []byte
	version *version.Height
}

type updateSet struct {
	m map[string]*versionedValue
}

// Savepoint docid (key) for couchdb
const savepointDocID = "statedb_savepoint"

// Savepoint data for couchdb
type couchSavepointData struct {
	BlockNum  uint64 `json:"BlockNum"`
	UpdateSeq string `json:"UpdateSeq"`
}

func newUpdateSet() *updateSet {
	return &updateSet{make(map[string]*versionedValue)}
}

func (u *updateSet) add(compositeKey []byte, vv *versionedValue) {
	u.m[string(compositeKey)] = vv
}

func (u *updateSet) exists(compositeKey []byte) bool {
	_, ok := u.m[string(compositeKey)]
	return ok
}

func (u *updateSet) get(compositeKey []byte) *versionedValue {
	return u.m[string(compositeKey)]
}

// CouchDBTxMgr a simple implementation of interface `txmgmt.TxMgr`.
// This implementation uses a read-write lock to prevent conflicts between transaction simulation and committing
type CouchDBTxMgr struct {
	db           *db.DB
	updateSet    *updateSet
	commitRWLock sync.RWMutex
	couchDB      *couchdb.CouchDBConnectionDef // COUCHDB new properties for CouchDB
	blockNum     uint64                        // block number corresponding to updateSet
}

// CouchConnection provides connection info for CouchDB
//TODO not currently used
type CouchConnection struct {
	host   string
	port   int
	dbName string
	id     string
	pw     string
}

// NewCouchDBTxMgr constructs a `CouchDBTxMgr`
func NewCouchDBTxMgr(conf *Conf, couchDBConnectURL string, dbName string, id string, pw string) *CouchDBTxMgr {

	// TODO cleanup this RocksDB handle
	db := db.CreateDB(&db.Conf{DBPath: conf.DBPath})
	db.Open()

	couchDB, err := couchdb.CreateCouchDBConnectionAndDB(couchDBConnectURL, dbName, id, pw)
	if err != nil {
		logger.Errorf("Error during NewCouchDBTxMgr(): %s\n", err.Error())
		return nil
	}

	// db and stateIndexCF will not be used for CouchDB. TODO to cleanup
	return &CouchDBTxMgr{db: db, couchDB: couchDB}
}

// NewQueryExecutor implements method in interface `txmgmt.TxMgr`
func (txmgr *CouchDBTxMgr) NewQueryExecutor() (ledger.QueryExecutor, error) {
	return &CouchDBQueryExecutor{txmgr}, nil
}

// NewTxSimulator implements method in interface `txmgmt.TxMgr`
func (txmgr *CouchDBTxMgr) NewTxSimulator() (ledger.TxSimulator, error) {
	s := &CouchDBTxSimulator{CouchDBQueryExecutor{txmgr}, make(map[string]*nsRWs), false}
	s.txmgr.commitRWLock.RLock()
	return s, nil
}

// ValidateAndPrepare implements method in interface `txmgmt.TxMgr`
func (txmgr *CouchDBTxMgr) ValidateAndPrepare(block *common.Block, doMVCCValidation bool) error {
	if doMVCCValidation == true {
		logger.Debugf("===COUCHDB=== Entering CouchDBTxMgr.ValidateAndPrepare()")
		logger.Debugf("Validating a block with [%d] transactions", len(block.Data.Data))
	} else {
		logger.Debugf("New block arrived for write set computation:%#v", block)
		logger.Debugf("Computing write set for a block with [%d] transactions", len(block.Data.Data))
	}
	var valid bool
	txmgr.updateSet = newUpdateSet()
	txmgr.blockNum = block.Header.Number
	txsFilter := util.NewFilterBitArrayFromBytes(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])

	for txIndex, envBytes := range block.Data.Data {
		if txsFilter.IsSet(uint(txIndex)) {
			// Skiping invalid transaction
			logger.Debug("Skipping transaction marked as invalid, txIndex=", txIndex)
			continue
		}

		// extract actions from the envelope message
		respPayload, err := putils.GetActionFromEnvelope(envBytes)
		if err != nil {
			return err
		}

		//preparation for extracting RWSet from transaction
		txRWSet := &rwset.TxReadWriteSet{}

		// Get the Result from the Action
		// and then Unmarshal it into a TxReadWriteSet using custom unmarshalling
		if err = txRWSet.Unmarshal(respPayload.Results); err != nil {
			return err
		}

		// trace the first 2000 characters of RWSet only, in case it is huge
		if logger.IsEnabledFor(logging.DEBUG) {
			txRWSetString := txRWSet.String()
			operation := "validating"
			if doMVCCValidation == false {
				operation = "computing write set from"
			}
			if len(txRWSetString) < 2000 {
				logger.Debugf(operation+" txRWSet:[%s]", txRWSetString)
			} else {
				logger.Debugf(operation+" txRWSet:[%s...]", txRWSetString[0:2000])
			}
		}
		if doMVCCValidation == true {
			if valid, err = txmgr.validateTx(txRWSet); err != nil {
				return err
			}
		} else {
			valid = true
		}

		if valid {
			if err := txmgr.addWriteSetToBatch(txRWSet, version.NewHeight(block.Header.Number, uint64(txIndex+1))); err != nil {
				return err
			}
		} else {
			// Unset bit in byte array corresponded to the invalid transaction
			txsFilter.Set(uint(txIndex))
		}
	}

	block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] = txsFilter.ToBytes()
	logger.Debugf("===COUCHDB=== Exiting CouchDBTxMgr.ValidateAndPrepare()")
	return nil
}

// Shutdown implements method in interface `txmgmt.TxMgr`
func (txmgr *CouchDBTxMgr) Shutdown() {
	txmgr.db.Close()
}

func (txmgr *CouchDBTxMgr) validateTx(txRWSet *rwset.TxReadWriteSet) (bool, error) {

	var err error
	var currentVersion *version.Height

	for _, nsRWSet := range txRWSet.NsRWs {
		ns := nsRWSet.NameSpace
		for _, kvRead := range nsRWSet.Reads {
			compositeKey := constructCompositeKey(ns, kvRead.Key)
			if txmgr.updateSet != nil && txmgr.updateSet.exists(compositeKey) {
				return false, nil
			}
			if currentVersion, err = txmgr.getCommitedVersion(ns, kvRead.Key); err != nil {
				return false, err
			}
			if !version.AreSame(currentVersion, kvRead.Version) {
				logger.Debugf("Version mismatch for key [%s:%s]. Current version = [%d], Version in readSet [%d]",
					ns, kvRead.Key, currentVersion, kvRead.Version)
				return false, nil
			}
		}
	}
	return true, nil
}

func (txmgr *CouchDBTxMgr) addWriteSetToBatch(txRWSet *rwset.TxReadWriteSet, txHeight *version.Height) error {
	if txmgr.updateSet == nil {
		txmgr.updateSet = newUpdateSet()
	}
	for _, nsRWSet := range txRWSet.NsRWs {
		ns := nsRWSet.NameSpace
		for _, kvWrite := range nsRWSet.Writes {
			compositeKey := constructCompositeKey(ns, kvWrite.Key)
			txmgr.updateSet.add(compositeKey, &versionedValue{kvWrite.Value, txHeight})
		}
	}
	return nil
}

// Commit implements method in interface `txmgmt.TxMgr`
func (txmgr *CouchDBTxMgr) Commit() error {
	logger.Debugf("===COUCHDB=== Entering CouchDBTxMgr.Commit()")

	if txmgr.updateSet == nil {
		panic("validateAndPrepare() method should have been called before calling commit()")
	}

	txmgr.commitRWLock.Lock()
	defer txmgr.commitRWLock.Unlock()
	defer func() { txmgr.updateSet = nil }()

	for k, v := range txmgr.updateSet.m {

		if couchdb.IsJSON(string(v.value)) {

			// SaveDoc using couchdb client and use JSON format
			rev, err := txmgr.couchDB.SaveDoc(k, "", v.value, nil)
			if err != nil {
				logger.Errorf("===COUCHDB=== Error during Commit(): %s\n", err.Error())
				return err
			}
			if rev != "" {
				logger.Debugf("===COUCHDB=== Saved document revision number: %s\n", rev)
			}

		} else {

			//Create an attachment structure and load the bytes
			attachment := &couchdb.Attachment{}
			attachment.AttachmentBytes = v.value
			attachment.ContentType = "application/octet-stream"
			attachment.Name = "valueBytes"

			attachments := []couchdb.Attachment{}
			attachments = append(attachments, *attachment)

			// SaveDoc using couchdb client and use attachment
			rev, err := txmgr.couchDB.SaveDoc(k, "", nil, attachments)
			if err != nil {
				logger.Errorf("===COUCHDB=== Error during Commit(): %s\n", err.Error())
				return err
			}
			if rev != "" {
				logger.Debugf("===COUCHDB=== Saved document revision number: %s\n", rev)
			}

		}

	}

	// Record a savepoint
	err := txmgr.recordSavepoint()
	if err != nil {
		logger.Errorf("===COUCHDB=== Error during recordSavepoint: %s\n", err.Error())
		return err
	}

	logger.Debugf("===COUCHDB=== Exiting CouchDBTxMgr.Commit()")
	return nil
}

// recordSavepoint Record a savepoint in statedb.
// Couch parallelizes writes in cluster or sharded setup and ordering is not guaranteed.
// Hence we need to fence the savepoint with sync. So ensure_full_commit is called before AND after writing savepoint document
// TODO: Optimization - merge 2nd ensure_full_commit with savepoint by using X-Couch-Full-Commit header
func (txmgr *CouchDBTxMgr) recordSavepoint() error {
	var err error
	var savepointDoc couchSavepointData
	// ensure full commit to flush all changes until now to disk
	dbResponse, err := txmgr.couchDB.EnsureFullCommit()
	if err != nil || dbResponse.Ok != true {
		logger.Errorf("====COUCHDB==== Failed to perform full commit\n")
		return errors.New("Failed to perform full commit")
	}

	// construct savepoint document
	// UpdateSeq would be useful if we want to get all db changes since a logical savepoint
	dbInfo, _, err := txmgr.couchDB.GetDatabaseInfo()
	if err != nil {
		logger.Errorf("====COUCHDB==== Failed to get DB info %s\n", err.Error())
		return err
	}
	savepointDoc.BlockNum = txmgr.blockNum
	savepointDoc.UpdateSeq = dbInfo.UpdateSeq

	savepointDocJSON, err := json.Marshal(savepointDoc)
	if err != nil {
		logger.Errorf("====COUCHDB==== Failed to create savepoint data %s\n", err.Error())
		return err
	}

	// SaveDoc using couchdb client and use JSON format
	_, err = txmgr.couchDB.SaveDoc(savepointDocID, "", savepointDocJSON, nil)
	if err != nil {
		logger.Errorf("====CouchDB==== Failed to save the savepoint to DB %s\n", err.Error())
	}

	// ensure full commit to flush savepoint to disk
	dbResponse, err = txmgr.couchDB.EnsureFullCommit()
	if err != nil || dbResponse.Ok != true {
		logger.Errorf("====COUCHDB==== Failed to perform full commit\n")
		return errors.New("Failed to perform full commit")
	}
	return nil
}

// GetBlockNumFromSavepoint Reads the savepoint from database and returns the corresponding block number.
// If no savepoint is found, it returns 0
func (txmgr *CouchDBTxMgr) GetBlockNumFromSavepoint() (uint64, error) {
	var err error
	savepointJSON, _, err := txmgr.couchDB.ReadDoc(savepointDocID)
	if err != nil {
		// TODO: differentiate between 404 and some other error code
		logger.Errorf("====COUCHDB==== Failed to read savepoint data %s\n", err.Error())
		return 0, err
	}

	savepointDoc := &couchSavepointData{}
	err = json.Unmarshal(savepointJSON, &savepointDoc)
	if err != nil {
		logger.Errorf("====COUCHDB==== Failed to read savepoint data %s\n", err.Error())
		return 0, err
	}

	return savepointDoc.BlockNum, nil
}

// Rollback implements method in interface `txmgmt.TxMgr`
func (txmgr *CouchDBTxMgr) Rollback() {
	txmgr.updateSet = nil
	txmgr.blockNum = 0
}

func (txmgr *CouchDBTxMgr) getCommitedVersion(ns string, key string) (*version.Height, error) {
	var err error
	var version *version.Height
	if _, version, err = txmgr.getCommittedValueAndVersion(ns, key); err != nil {
		return nil, err
	}
	return version, nil
}

func (txmgr *CouchDBTxMgr) getCommittedValueAndVersion(ns string, key string) ([]byte, *version.Height, error) {

	compositeKey := constructCompositeKey(ns, key)

	docBytes, _, _ := txmgr.couchDB.ReadDoc(string(compositeKey)) // TODO add error handling

	// trace the first 200 bytes of value only, in case it is huge
	if docBytes != nil && logger.IsEnabledFor(logging.DEBUG) {
		if len(docBytes) < 200 {
			logger.Debugf("===COUCHDB=== getCommittedValueAndVersion() Read docBytes %s", docBytes)
		} else {
			logger.Debugf("===COUCHDB=== getCommittedValueAndVersion() Read docBytes %s...", docBytes[0:200])
		}
	}

	ver := version.NewHeight(1, 1) //TODO - version hardcoded to 1 is a temporary value for the prototype
	return docBytes, ver, nil
}

//getRangeScanner contructs composite start and end keys based on the namespace then calls the CouchDB range scanner
//TODO the limit and offset are currently hard coded.  The limit should eventually be a config option
func (txmgr *CouchDBTxMgr) getRangeScanner(namespace string, startKey string, endKey string) (*kvScanner, error) {
	var compositeStartKey []byte
	var compositeEndKey []byte
	if startKey != "" {
		compositeStartKey = constructCompositeKey(namespace, startKey)
	}
	if endKey != "" {
		compositeEndKey = constructCompositeKey(namespace, endKey)
	}

	queryResult, _ := txmgr.couchDB.ReadDocRange(string(compositeStartKey), string(compositeEndKey), 1000, 0)

	return newKVScanner(namespace, *queryResult), nil
}

//getQuery calls the CouchDB query documents method (CouchDB _find API)
//TODO the limit and offset are currently hard coded.  The limit should eventually be a config option
func (txmgr *CouchDBTxMgr) getQuery(query string) (*queryScanner, error) {

	//TODO - limit is currently set at 1000,  eventually this will need to be changed
	//to reflect a config option and potentially return an exception if the threshold is exceeded
	queryResult, _ := txmgr.couchDB.QueryDocuments(query, 1000, 0)

	return newQueryScanner(*queryResult), nil
}

func encodeValue(value []byte, version uint64) []byte {
	versionBytes := proto.EncodeVarint(version)
	deleteMarker := 0
	if value == nil {
		deleteMarker = 1
	}
	deleteMarkerBytes := proto.EncodeVarint(uint64(deleteMarker))
	encodedValue := append(versionBytes, deleteMarkerBytes...)
	if value != nil {
		encodedValue = append(encodedValue, value...)
	}
	return encodedValue
}

func decodeValue(encodedValue []byte) ([]byte, uint64) {
	version, len1 := proto.DecodeVarint(encodedValue)
	deleteMarker, len2 := proto.DecodeVarint(encodedValue[len1:])
	if deleteMarker == 1 {
		return nil, version
	}
	value := encodedValue[len1+len2:]
	return value, version
}

func constructCompositeKey(ns string, key string) []byte {
	compositeKey := []byte(ns)
	compositeKey = append(compositeKey, compositeKeySep...)
	compositeKey = append(compositeKey, []byte(key)...)
	return compositeKey
}

func splitCompositeKey(compositeKey []byte) (string, string) {
	split := bytes.SplitN(compositeKey, compositeKeySep, 2)
	return string(split[0]), string(split[1])
}

type kvScanner struct {
	cursor    int
	namespace string
	results   []couchdb.QueryResult
}

type committedKV struct {
	key     string
	version *version.Height
	value   []byte
}

func newKVScanner(namespace string, queryResults []couchdb.QueryResult) *kvScanner {
	return &kvScanner{-1, namespace, queryResults}
}

func (scanner *kvScanner) next() (*committedKV, error) {

	scanner.cursor++

	if scanner.cursor >= len(scanner.results) {
		return nil, nil
	}

	selectedValue := scanner.results[scanner.cursor]

	_, key := splitCompositeKey([]byte(selectedValue.ID))

	//TODO - change hardcoded version when version support is available in CouchDB
	return &committedKV{key, version.NewHeight(1, 1), selectedValue.Value}, nil

}

func (scanner *kvScanner) close() {

	scanner = nil
}

type queryScanner struct {
	cursor  int
	results []couchdb.QueryResult
}

type queryRecord struct {
	namespace string
	key       string
	version   *version.Height
	record    []byte
}

func newQueryScanner(queryResults []couchdb.QueryResult) *queryScanner {
	return &queryScanner{-1, queryResults}
}

func (scanner *queryScanner) next() (*queryRecord, error) {

	scanner.cursor++

	if scanner.cursor >= len(scanner.results) {
		return nil, nil
	}

	selectedValue := scanner.results[scanner.cursor]

	namespace, key := splitCompositeKey([]byte(selectedValue.ID))

	//TODO - change hardcoded version when version support is available in CouchDB
	return &queryRecord{namespace, key, version.NewHeight(1, 1), selectedValue.Value}, nil

}

func (scanner *queryScanner) close() {

	scanner = nil
}
