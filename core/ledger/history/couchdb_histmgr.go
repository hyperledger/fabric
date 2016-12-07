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
	"bytes"
	"strconv"

	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt"
	"github.com/hyperledger/fabric/core/ledger/util/couchdb"
	"github.com/hyperledger/fabric/protos/common"
	putils "github.com/hyperledger/fabric/protos/utils"
	logging "github.com/op/go-logging"
)

var logger = logging.MustGetLogger("history")

// CouchDBHistMgr a simple implementation of interface `histmgmt.HistMgr'.
// TODO This implementation does not currently use a lock but may need one to ensure query's are consistent
type CouchDBHistMgr struct {
	couchDB *couchdb.CouchDBConnectionDef // COUCHDB new properties for CouchDB
}

// NewCouchDBHistMgr constructs a new `CouchDB HistMgr`
func NewCouchDBHistMgr(couchDBConnectURL string, dbName string, id string, pw string) *CouchDBHistMgr {

	//TODO locking has not been implemented but may need some sort of locking to insure queries are valid data.

	couchDB, err := couchdb.CreateCouchDBConnectionAndDB(couchDBConnectURL, dbName, id, pw)
	if err != nil {
		logger.Errorf("===HISTORYDB=== Error during NewCouchDBHistMgr(): %s\n", err.Error())
		return nil
	}

	return &CouchDBHistMgr{couchDB: couchDB}
}

// Commit implements method in interface `histmgmt.HistMgr`
// This writes to a separate history database.
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
		txRWSet := &txmgmt.TxReadWriteSet{}

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
				compositeKey := constructCompositeKey(ns, writeKey, blockNo, tranNo)
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
	return nil
}

func constructCompositeKey(ns string, key string, blocknum uint64, trannum uint64) string {
	//History Key is:  "namespace key blocknum trannum"", with namespace being the chaincode id

	// TODO - We will likely want sortable varint encoding, rather then a simple number, in order to support sorted key scans
	var buffer bytes.Buffer
	buffer.WriteString(ns)
	buffer.WriteByte(0)
	buffer.WriteString(key)
	buffer.WriteByte(0)
	buffer.WriteString(strconv.Itoa(int(blocknum)))
	buffer.WriteByte(0)
	buffer.WriteString(strconv.Itoa(int(trannum)))

	return buffer.String()
}
