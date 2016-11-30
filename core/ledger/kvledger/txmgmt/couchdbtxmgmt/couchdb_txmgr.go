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
	"sync"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt"
	"github.com/hyperledger/fabric/core/ledger/util/couchdb"
	"github.com/hyperledger/fabric/core/ledger/util/db"
	"github.com/op/go-logging"

	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	putils "github.com/hyperledger/fabric/protos/utils"
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
	db           *db.DB
	updateSet    *updateSet
	commitRWLock sync.RWMutex
	couchDB      *couchdb.CouchDBConnectionDef // COUCHDB new properties for CouchDB
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
func (txmgr *CouchDBTxMgr) ValidateAndPrepare(block *common.Block) (*common.Block, []*pb.InvalidTransaction, error) {
	logger.Debugf("===COUCHDB=== Entering CouchDBTxMgr.ValidateAndPrepare()")
	invalidTxs := []*pb.InvalidTransaction{}
	var valid bool
	txmgr.updateSet = newUpdateSet()
	logger.Debugf("Validating a block with [%d] transactions", len(block.Data.Data))
	for _, envBytes := range block.Data.Data {
		// extract actions from the envelope message
		respPayload, err := putils.GetActionFromEnvelope(envBytes)
		if err != nil {
			return nil, nil, err
		}

		//preparation for extracting RWSet from transaction
		txRWSet := &txmgmt.TxReadWriteSet{}

		// Get the Result from the Action
		// and then Unmarshal it into a TxReadWriteSet using custom unmarshalling
		if err = txRWSet.Unmarshal(respPayload.Results); err != nil {
			return nil, nil, err
		}

		// trace the first 2000 characters of RWSet only, in case it is huge
		if logger.IsEnabledFor(logging.DEBUG) {
			txRWSetString := txRWSet.String()
			if len(txRWSetString) < 2000 {
				logger.Debugf("validating txRWSet:[%s]", txRWSetString)
			} else {
				logger.Debugf("validating txRWSet:[%s...]", txRWSetString[0:2000])
			}
		}

		if valid, err = txmgr.validateTx(txRWSet); err != nil {
			return nil, nil, err
		}

		if valid {
			if err := txmgr.addWriteSetToBatch(txRWSet); err != nil {
				return nil, nil, err
			}
		} else {
			invalidTxs = append(invalidTxs, &pb.InvalidTransaction{
				Transaction: &pb.Transaction{ /* FIXME */ }, Cause: pb.InvalidTransaction_RWConflictDuringCommit})
		}
	}
	logger.Debugf("===COUCHDB=== Exiting CouchDBTxMgr.ValidateAndPrepare()")
	return block, invalidTxs, nil
}

// Shutdown implements method in interface `txmgmt.TxMgr`
func (txmgr *CouchDBTxMgr) Shutdown() {
	txmgr.db.Close()
}

func (txmgr *CouchDBTxMgr) validateTx(txRWSet *txmgmt.TxReadWriteSet) (bool, error) {

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

	docBytes, _, _ := txmgr.couchDB.ReadDoc(string(compositeKey)) // TODO add error handling

	// trace the first 200 bytes of value only, in case it is huge
	if docBytes != nil && logger.IsEnabledFor(logging.DEBUG) {
		if len(docBytes) < 200 {
			logger.Debugf("===COUCHDB=== getCommittedValueAndVersion() Read docBytes %s", docBytes)
		} else {
			logger.Debugf("===COUCHDB=== getCommittedValueAndVersion() Read docBytes %s...", docBytes[0:200])
		}
	}

	var version uint64 = 1 //TODO - version hardcoded to 1 is a temporary value for the prototype
	return docBytes, version, nil
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
