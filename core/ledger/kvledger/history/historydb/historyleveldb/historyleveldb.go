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
	"github.com/hyperledger/fabric/common/ledger/util/leveldbhelper"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/history/historydb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwset"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
	"github.com/hyperledger/fabric/protos/common"
	putils "github.com/hyperledger/fabric/protos/utils"
	logging "github.com/op/go-logging"
)

var logger = logging.MustGetLogger("historyleveldb")

var compositeKeySep = []byte{0x00}
var savePointKey = []byte{0x00}
var emptyValue = []byte{}

// HistoryDBProvider implements interface HistoryDBProvider
type HistoryDBProvider struct {
	dbProvider *leveldbhelper.Provider
}

// NewHistoryDBProvider instantiates HistoryDBProvider
func NewHistoryDBProvider() *HistoryDBProvider {
	dbPath := ledgerconfig.GetHistoryLevelDBPath()
	logger.Debugf("constructing HistoryDBProvider dbPath=%s", dbPath)
	dbProvider := leveldbhelper.NewProvider(&leveldbhelper.Conf{DBPath: dbPath})
	return &HistoryDBProvider{dbProvider}
}

// GetDBHandle gets the handle to a named database
func (provider *HistoryDBProvider) GetDBHandle(dbName string) (historydb.HistoryDB, error) {
	return newHistoryDB(provider.dbProvider.GetDBHandle(dbName), dbName), nil
}

// Close closes the underlying db
func (provider *HistoryDBProvider) Close() {
	provider.dbProvider.Close()
}

// historyDB implements HistoryDB interface
type historyDB struct {
	db     *leveldbhelper.DBHandle
	dbName string
}

// newHistoryDB constructs an instance of HistoryDB
func newHistoryDB(db *leveldbhelper.DBHandle, dbName string) *historyDB {
	return &historyDB{db, dbName}
}

// Open implements method in HistoryDB interface
func (historyDB *historyDB) Open() error {
	// do nothing because shared db is used
	return nil
}

// Close implements method in HistoryDB interface
func (historyDB *historyDB) Close() {
	// do nothing because shared db is used
}

// Commit implements method in HistoryDB interface
func (historyDB *historyDB) Commit(block *common.Block) error {

	logger.Debugf("Entering HistoryLevelDB.Commit()")

	//Get the blocknumber off of the header
	blockNo := block.Header.Number
	//Set the starting tranNo to 0
	var tranNo uint64

	dbBatch := leveldbhelper.NewUpdateBatch()

	logger.Debugf("Updating history for blockNo: %v with [%d] transactions",
		blockNo, len(block.Data.Data))

	//TODO add check for invalid trans in bit array
	for _, envBytes := range block.Data.Data {
		tranNo++

		env, err := putils.GetEnvelopeFromBlock(envBytes)
		if err != nil {
			return err
		}

		payload, err := putils.GetPayload(env)
		if err != nil {
			return err
		}

		if common.HeaderType(payload.Header.ChainHeader.Type) == common.HeaderType_ENDORSER_TRANSACTION {

			logger.Debugf("Updating history for tranNo: %d", tranNo)
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
			// for each transaction, loop through the namespaces and writesets
			// and add a history record for each write
			for _, nsRWSet := range txRWSet.NsRWs {
				ns := nsRWSet.NameSpace

				for _, kvWrite := range nsRWSet.Writes {
					writeKey := kvWrite.Key

					logger.Debugf("Writing history record for: ns=%s, key=%s, blockNo=%d tranNo=%d",
						ns, writeKey, blockNo, tranNo)

					//composite key for history records is in the form ns~key~blockNo~tranNo
					compositeHistoryKey := historydb.ConstructCompositeHistoryKey(ns, writeKey, blockNo, tranNo)

					// No value is required, write an empty byte array (emptyValue) since Put() of nil is not allowed
					dbBatch.Put(compositeHistoryKey, emptyValue)
				}
			}

		} else {
			logger.Debugf("Skipping transaction %d since it is not an endorsement transaction\n", tranNo)
		}

	}

	// add savepoint for recovery purpose
	height := version.NewHeight(blockNo, tranNo)
	dbBatch.Put(savePointKey, height.ToBytes())

	// write the block's history records and savepoint to LevelDB
	if err := historyDB.db.WriteBatch(dbBatch, false); err != nil {
		return err
	}

	return nil
}

// NewHistoryQueryExecutor implements method in HistoryDB interface
func (historyDB *historyDB) NewHistoryQueryExecutor() (ledger.HistoryQueryExecutor, error) {
	return &LevelHistoryDBQueryExecutor{historyDB}, nil
}

// GetBlockNumFromSavepoint implements method in HistoryDB interface
func (historyDB *historyDB) GetBlockNumFromSavepoint() (uint64, error) {
	versionBytes, err := historyDB.db.Get(savePointKey)
	if err != nil || versionBytes == nil {
		return 0, err
	}
	height, _ := version.NewHeightFromBytes(versionBytes)
	return height.BlockNum, nil
}
