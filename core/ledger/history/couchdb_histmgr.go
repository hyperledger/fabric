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

package history

import (
	"github.com/hyperledger/fabric/core/ledger/util/couchdb"
	"github.com/hyperledger/fabric/protos/common"
	logging "github.com/op/go-logging"
)

var logger = logging.MustGetLogger("txhistorymgmt")

// CouchDBHistMgr a simple implementation of interface `histmgmt.TxHistMgr'.
// TODO This implementation does not currently use a lock but may need one to insure query's are consistent
type CouchDBHistMgr struct {
	couchDB *couchdb.CouchDBConnectionDef // COUCHDB new properties for CouchDB
}

// NewCouchDBHistMgr constructs a new `CouchDBTxHistMgr`
func NewCouchDBHistMgr(couchDBConnectURL string, dbName string, id string, pw string) *CouchDBHistMgr {

	//TODO locking has not been implemented but may need some sort of locking to insure queries are valid data.

	couchDB, err := couchdb.CreateCouchDBConnectionAndDB(couchDBConnectURL, dbName, id, pw)
	if err != nil {
		logger.Errorf("Error during NewCouchDBHistMgr(): %s\n", err.Error())
		return nil
	}

	// db and stateIndexCF will not be used for CouchDB. TODO to cleanup
	return &CouchDBHistMgr{couchDB: couchDB}
}

// Commit implements method in interface `txhistorymgmt.TxMgr`
func (histmgr *CouchDBHistMgr) Commit(block *common.Block) error {
	logger.Debugf("===HISTORYDB=== Entering CouchDBTxHistMgr.Commit()")
	return nil
}
