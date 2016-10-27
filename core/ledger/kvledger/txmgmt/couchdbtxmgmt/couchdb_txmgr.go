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
	"fmt"
	"sync"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/couchdbtxmgmt/couchdb"
	"github.com/hyperledger/fabric/core/ledger/util/db"
	"github.com/hyperledger/fabric/protos"
	"github.com/op/go-logging"
	"github.com/tecbot/gorocksdb"
)

var logger = logging.MustGetLogger("couchdbtxmgmt")

// Conf - configuration for `CouchDBTxMgr`
type Conf struct {
	DBPath string
}

type versionedValue struct {
	value   []byte
	version uint64
}

type updateSet struct {
	m map[string]*versionedValue
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
	db                  *db.DB
	stateIndexCF        *gorocksdb.ColumnFamilyHandle
	updateSet           *updateSet
	commitRWLock        sync.RWMutex
	couchConnectionInfo CouchConnection // COUCHDB new properties for CouchDB
}

// CouchConnection provides connection info for CouchDB
type CouchConnection struct {
	host   string
	port   int
	dbName string
	id     string
	pw     string
}

// NewCouchDBTxMgr constructs a `CouchDBTxMgr`
func NewCouchDBTxMgr(conf *Conf, host string, port int, dbName string, id string, pw string) *CouchDBTxMgr {
	couchConnectionInfo := CouchConnection{host: host, port: port, dbName: dbName, id: id, pw: pw}
	db := db.CreateDB(&db.Conf{DBPath: conf.DBPath, CFNames: []string{}})
	db.Open()
	// db and stateIndexCF will not be used for CouchDB. TODO to cleanup
	return &CouchDBTxMgr{db: db, stateIndexCF: db.GetDefaultCFHandle(), couchConnectionInfo: couchConnectionInfo}
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
func (txmgr *CouchDBTxMgr) ValidateAndPrepare(block *protos.Block2) (*protos.Block2, []*protos.InvalidTransaction, error) {
	logger.Debugf("===COUCHDB=== Entering CouchDBTxMgr.ValidateAndPrepare()")
	validatedBlock := &protos.Block2{}
	//TODO pull PreviousBlockHash from db
	validatedBlock.PreviousBlockHash = block.PreviousBlockHash
	invalidTxs := []*protos.InvalidTransaction{}
	var valid bool
	var err error
	txmgr.updateSet = newUpdateSet()
	logger.Debugf("Validating a block with [%d] transactions", len(block.Transactions))
	for _, txBytes := range block.Transactions {
		tx := &protos.Transaction2{}
		err = proto.Unmarshal(txBytes, tx)
		if err != nil {
			return nil, nil, err
		}
		numEndorsements := len(tx.EndorsedActions)
		if numEndorsements == 0 {
			return nil, nil, fmt.Errorf("Tx contains no EndorsedActions")
		}

		// Eventually we'll want to support multiple EndorsedActions in a tran, see FAB-445
		// But for now, we'll return an error if there are multiple EndorsedActions
		if numEndorsements > 1 {
			return nil, nil, fmt.Errorf("Tx contains more than one [%d] EndorsedActions", numEndorsements)
		}

		// Get the actionBytes from the EndorsedAction
		// and then Unmarshal it into an Action using protobuf unmarshalling
		action := &protos.Action{}
		actionBytes := tx.EndorsedActions[0].ActionBytes
		err = proto.Unmarshal(actionBytes, action)
		if err != nil {
			return nil, nil, err
		}

		// Get the SimulationResult from the Action
		// and then Unmarshal it into a TxReadWriteSet using custom unmarshalling
		txRWSet := &txmgmt.TxReadWriteSet{}
		if err = txRWSet.Unmarshal(action.SimulationResult); err != nil {
			return nil, nil, err
		}

		logger.Debugf("validating txRWSet:[%s]", txRWSet)
		if valid, err = txmgr.validateTx(txRWSet); err != nil {
			return nil, nil, err
		}

		if valid {
			if err := txmgr.addWriteSetToBatch(txRWSet); err != nil {
				return nil, nil, err
			}
			validatedBlock.Transactions = append(validatedBlock.Transactions, txBytes)
		} else {
			invalidTxs = append(invalidTxs, &protos.InvalidTransaction{
				Transaction: tx, Cause: protos.InvalidTransaction_RWConflictDuringCommit})
		}
	}
	logger.Debugf("===COUCHDB=== Exiting CouchDBTxMgr.ValidateAndPrepare()")
	return validatedBlock, invalidTxs, nil
}

// Shutdown implements method in interface `txmgmt.TxMgr`
func (txmgr *CouchDBTxMgr) Shutdown() {
	txmgr.db.Close()
}

func (txmgr *CouchDBTxMgr) validateTx(txRWSet *txmgmt.TxReadWriteSet) (bool, error) {
	logger.Debugf("Validating txRWSet:%s", txRWSet)
	var err error
	var currentVersion uint64

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
			if currentVersion != kvRead.Version {
				logger.Debugf("Version mismatch for key [%s:%s]. Current version = [%d], Version in readSet [%d]",
					ns, kvRead.Key, currentVersion, kvRead.Version)
				return false, nil
			}
		}
	}
	return true, nil
}

func (txmgr *CouchDBTxMgr) addWriteSetToBatch(txRWSet *txmgmt.TxReadWriteSet) error {
	var err error
	var currentVersion uint64

	if txmgr.updateSet == nil {
		txmgr.updateSet = newUpdateSet()
	}
	for _, nsRWSet := range txRWSet.NsRWs {
		ns := nsRWSet.NameSpace
		for _, kvWrite := range nsRWSet.Writes {
			compositeKey := constructCompositeKey(ns, kvWrite.Key)
			versionedVal := txmgr.updateSet.get(compositeKey)
			if versionedVal != nil {
				currentVersion = versionedVal.version
			} else {
				currentVersion, err = txmgr.getCommitedVersion(ns, kvWrite.Key)
				if err != nil {
					return err
				}
			}
			txmgr.updateSet.add(compositeKey, &versionedValue{kvWrite.Value, currentVersion + 1})
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

	db, _ := couchdb.CreateConnectionDefinition(txmgr.couchConnectionInfo.host,
		txmgr.couchConnectionInfo.port,
		txmgr.couchConnectionInfo.dbName,
		txmgr.couchConnectionInfo.id,
		txmgr.couchConnectionInfo.pw)

	for k, v := range txmgr.updateSet.m {

		txmgr.commitRWLock.Lock()
		defer txmgr.commitRWLock.Unlock()
		defer func() { txmgr.updateSet = nil }()

		// SaveDoc using couchdb client
		rev, err := db.SaveDoc(k, v.value)

		if err != nil {
			logger.Errorf("===COUCHDB=== Error during Commit(): %s\n", err.Error())
			return err
		}
		if rev != "" {
			logger.Debugf("===COUCHDB=== Saved document revision number: %s\n", rev)
		}

	}

	logger.Debugf("===COUCHDB=== Exiting CouchDBTxMgr.Commit()")
	return nil
}

// Rollback implements method in interface `txmgmt.TxMgr`
func (txmgr *CouchDBTxMgr) Rollback() {
	txmgr.updateSet = nil
}

func (txmgr *CouchDBTxMgr) getCommitedVersion(ns string, key string) (uint64, error) {
	var err error
	var version uint64
	if _, version, err = txmgr.getCommittedValueAndVersion(ns, key); err != nil {
		return 0, err
	}
	return version, nil
}

func (txmgr *CouchDBTxMgr) getCommittedValueAndVersion(ns string, key string) ([]byte, uint64, error) {

	compositeKey := constructCompositeKey(ns, key)
	/* Don't get from RocksDB since we are using CouchDB here
	var encodedValue []byte
	var err error
	if encodedValue, err = txmgr.db.Get(txmgr.stateIndexCF, compositeKey); err != nil {
		return nil, 0, err
	}
	if encodedValue == nil {
		return nil, 0, nil
	}
	value, version := decodeValue(encodedValue)
	*/

	db, _ := couchdb.CreateConnectionDefinition(txmgr.couchConnectionInfo.host,
		txmgr.couchConnectionInfo.port,
		txmgr.couchConnectionInfo.dbName,
		txmgr.couchConnectionInfo.id,
		txmgr.couchConnectionInfo.pw)

	jsonBytes, _, _ := db.ReadDoc(string(compositeKey)) // TODO add error handling

	if jsonBytes != nil {
		jsonString := string(jsonBytes[:])
		logger.Debugf("===COUCHDB=== getCommittedValueAndVersion() Read jsonString:\n   %s", jsonString)
	}
	value := jsonBytes
	var version uint64 = 1 //TODO - version hardcoded to 1 is a temporary value for the prototype
	return value, version, nil
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
	compositeKey = append(compositeKey, byte(0))
	compositeKey = append(compositeKey, []byte(key)...)
	return compositeKey
}
