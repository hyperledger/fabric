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

package couchdbhistmgr

import (
	"encoding/json"
	"fmt"

	"github.com/hyperledger/fabric/core/ledger"
	helper "github.com/hyperledger/fabric/core/ledger/kvledger/history/histmgr"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwset"
	"github.com/hyperledger/fabric/core/ledger/util/couchdb"
	"github.com/hyperledger/fabric/protos/common"
	putils "github.com/hyperledger/fabric/protos/utils"
	logging "github.com/op/go-logging"
)

// Savepoint docid (key) for couchdb
const savepointDocID = "histdb_savepoint"

// Savepoint data for couchdb
type couchSavepointData struct {
	BlockNum  uint64 `json:"BlockNum"`
	UpdateSeq string `json:"UpdateSeq"`
}

var logger = logging.MustGetLogger("couchdbhistmgr")

// CouchDBHistMgr a simple implementation of interface `histmgr.HistMgr'.
// TODO This implementation does not currently use a lock but may need one to ensure query's are consistent
type CouchDBHistMgr struct {
	couchDB *couchdb.CouchDatabase // COUCHDB new properties for CouchDB
}

// NewCouchDBHistMgr constructs a new `CouchDB HistMgr`
func NewCouchDBHistMgr(couchDBConnectURL string, dbName string, id string, pw string) *CouchDBHistMgr {

	//TODO locking has not been implemented but may need some sort of locking to insure queries are valid data.

	couchInstance, err := couchdb.CreateCouchInstance(couchDBConnectURL, id, pw)
	couchDB, err := couchdb.CreateCouchDatabase(*couchInstance, dbName)
	if err != nil {
		logger.Errorf("===HISTORYDB=== Error during NewCouchDBHistMgr(): %s\n", err.Error())
		return nil
	}

	return &CouchDBHistMgr{couchDB: couchDB}
}

// NewHistoryQueryExecutor implements method in interface `histr.HistMgr'.
func (histmgr *CouchDBHistMgr) NewHistoryQueryExecutor() (ledger.HistoryQueryExecutor, error) {
	return &CouchDBHistQueryExecutor{histmgr}, nil
}

// Commit implements method in interface `histmgr.HistMgr`
// This writes to a separate history database.
// TODO dpending on how invalid transactions are handled may need to filter what history commits.
func (histmgr *CouchDBHistMgr) Commit(block *common.Block) error {
	logger.Debugf("===HISTORYDB=== Entering CouchDBHistMgr.Commit()")

	//Get the blocknumber off of the header
	blockNo := block.Header.Number
	//Set the starting tranNo to 0
	var tranNo uint64

	logger.Debugf("===HISTORYDB=== Updating history for blockNo: %v with [%d] transactions",
		blockNo, len(block.Data.Data))
	for _, envBytes := range block.Data.Data {
		tranNo++
		logger.Debugf("===HISTORYDB=== Updating history for tranNo: %v", tranNo)

		// extract actions from the envelope message
		respPayload, err := putils.GetActionFromEnvelope(envBytes)
		if err != nil {
			return err
		}

		//preparation for extracting RWSet from transaction
		txRWSet := &rwset.TxReadWriteSet{}

		// Get the Result from the Action and then Unmarshal
		// it into a TxReadWriteSet using custom unmarshalling
		if err = txRWSet.Unmarshal(respPayload.Results); err != nil {
			return err
		}

		//Transactions that have data that is not JSON such as binary data,
		// the write value will not write to history database.
		//These types of transactions will have the key written to the history
		// database to support history key scans.  We do not write the binary
		// value to CouchDB since the purpose of the history database value is
		// for query andbinary data can not be queried.
		for _, nsRWSet := range txRWSet.NsRWs {
			ns := nsRWSet.NameSpace

			for _, kvWrite := range nsRWSet.Writes {
				writeKey := kvWrite.Key
				writeValue := kvWrite.Value
				compositeKey := helper.ConstructCompositeKey(ns, writeKey, blockNo, tranNo)
				var bytesDoc []byte

				logger.Debugf("===HISTORYDB=== ns (namespace or cc id) = %v, writeKey: %v, compositeKey: %v, writeValue = %v",
					ns, writeKey, compositeKey, writeValue)

				if couchdb.IsJSON(string(writeValue)) {
					//logger.Debugf("===HISTORYDB=== yes JSON store writeValue = %v", string(writeValue))
					bytesDoc = writeValue
				} else {
					//For data that is not in JSON format only store the key
					//logger.Debugf("===HISTORYDB=== not JSON only store key")
					bytesDoc = []byte(`{}`)
				}

				// SaveDoc using couchdb client and use JSON format
				rev, err := histmgr.couchDB.SaveDoc(compositeKey, "", bytesDoc, nil)
				if err != nil {
					logger.Errorf("===HISTORYDB=== Error during Commit(): %s\n", err.Error())
					return err
				}
				if rev != "" {
					logger.Debugf("===HISTORYDB=== Saved document revision number: %s\n", rev)
				}
			}
		}
	}

	// Record a savepoint
	err := histmgr.recordSavepoint(blockNo)
	if err != nil {
		logger.Debugf("===COUCHDB=== Error during recordSavepoint: %s\n", err)
		return err
	}

	return nil
}

// recordSavepoint Record a savepoint in historydb.
// Couch parallelizes writes in cluster or sharded setup and ordering is not guaranteed.
// Hence we need to fence the savepoint with sync. So ensure_full_commit is called before AND after writing savepoint document
// TODO: Optimization - merge 2nd ensure_full_commit with savepoint by using X-Couch-Full-Commit header
func (txmgr *CouchDBHistMgr) recordSavepoint(blockNo uint64) error {
	var err error
	var savepointDoc couchSavepointData
	// ensure full commit to flush all changes until now to disk
	dbResponse, err := txmgr.couchDB.EnsureFullCommit()
	if err != nil || dbResponse.Ok != true {
		logger.Debugf("====COUCHDB==== Failed to perform full commit\n")
		return fmt.Errorf("Failed to perform full commit. Err: %s", err)
	}

	// construct savepoint document
	// UpdateSeq would be useful if we want to get all db changes since a logical savepoint
	dbInfo, _, err := txmgr.couchDB.GetDatabaseInfo()
	if err != nil {
		logger.Debugf("====COUCHDB==== Failed to get DB info %s\n", err)
		return err
	}
	savepointDoc.BlockNum = blockNo
	savepointDoc.UpdateSeq = dbInfo.UpdateSeq

	savepointDocJSON, err := json.Marshal(savepointDoc)
	if err != nil {
		logger.Debugf("====COUCHDB==== Failed to create savepoint data %s\n", err)
		return err
	}

	// SaveDoc using couchdb client and use JSON format
	_, err = txmgr.couchDB.SaveDoc(savepointDocID, "", savepointDocJSON, nil)
	if err != nil {
		logger.Debugf("====CouchDB==== Failed to save the savepoint to DB %s\n", err)
		return err
	}
	return nil
}

//getTransactionsForNsKey contructs composite start and end keys based on the namespace and key then calls the CouchDB range scanner
func (histmgr *CouchDBHistMgr) getTransactionsForNsKey(namespace string, key string, includeValues bool) (*histScanner, error) {
	var compositeStartKey []byte
	var compositeEndKey []byte
	if key != "" {
		compositeStartKey = helper.ConstructPartialCompositeKey(namespace, key, false)
		compositeEndKey = helper.ConstructPartialCompositeKey(namespace, key, true)
	}

	//TODO the limit should not be hardcoded.  Need the config.
	//TODO Implement includeValues so that values are not returned in the readDocRange
	queryResult, _ := histmgr.couchDB.ReadDocRange(string(compositeStartKey), string(compositeEndKey), 1000, 0)

	return newHistScanner(compositeStartKey, *queryResult), nil
}

// GetBlockNumFromSavepoint Reads the savepoint from database and returns the corresponding block number.
// If no savepoint is found, it returns 0
func (txmgr *CouchDBHistMgr) GetBlockNumFromSavepoint() (uint64, error) {
	var err error
	savepointJSON, _, err := txmgr.couchDB.ReadDoc(savepointDocID)
	if err != nil {
		// TODO: differentiate between 404 and some other error code
		logger.Debugf("====COUCHDB==== Failed to read savepoint data %s\n", err)
		return 0, err
	}

	savepointDoc := &couchSavepointData{}
	err = json.Unmarshal(savepointJSON, &savepointDoc)
	if err != nil {
		logger.Debugf("====COUCHDB==== Failed to read savepoint data %s\n", err)
		return 0, err
	}

	return savepointDoc.BlockNum, nil
}

type histScanner struct {
	cursor              int
	compositePartialKey []byte
	results             []couchdb.QueryResult
}

type historicValue struct {
	blockNumTranNum string
	value           []byte
}

func newHistScanner(compositePartialKey []byte, queryResults []couchdb.QueryResult) *histScanner {
	return &histScanner{-1, compositePartialKey, queryResults}
}

func (scanner *histScanner) next() (*historicValue, error) {

	scanner.cursor++

	if scanner.cursor >= len(scanner.results) {
		return nil, nil
	}

	selectedValue := scanner.results[scanner.cursor]

	_, blockNumTranNum := helper.SplitCompositeKey(scanner.compositePartialKey, []byte(selectedValue.ID))

	return &historicValue{blockNumTranNum, selectedValue.Value}, nil

}

func (scanner *histScanner) close() {
	scanner = nil
}
