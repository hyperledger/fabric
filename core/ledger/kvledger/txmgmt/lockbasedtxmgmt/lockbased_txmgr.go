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

package lockbasedtxmgmt

import (
	"bytes"
	"encoding/binary"
	"reflect"
	"sync"

	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt"
	"github.com/hyperledger/fabric/core/ledger/kvledger/version"
	"github.com/hyperledger/fabric/core/ledger/util/db"
	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	putils "github.com/hyperledger/fabric/protos/utils"
	"github.com/op/go-logging"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/iterator"
)

var logger = logging.MustGetLogger("lockbasedtxmgmt")

var compositeKeySep = []byte{0x00}

// Conf - configuration for `LockBasedTxMgr`
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

// savepoint key
const savepointKey = "savepoint"

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

// LockBasedTxMgr a simple implementation of interface `txmgmt.TxMgr`.
// This implementation uses a read-write lock to prevent conflicts between transaction simulation and committing
type LockBasedTxMgr struct {
	db           *db.DB
	updateSet    *updateSet
	commitRWLock sync.RWMutex
	blockNum     uint64
}

// NewLockBasedTxMgr constructs a `LockBasedTxMgr`
func NewLockBasedTxMgr(conf *Conf) *LockBasedTxMgr {
	db := db.CreateDB(&db.Conf{DBPath: conf.DBPath})
	db.Open()
	return &LockBasedTxMgr{db: db}
}

// NewQueryExecutor implements method in interface `txmgmt.TxMgr`
func (txmgr *LockBasedTxMgr) NewQueryExecutor() (ledger.QueryExecutor, error) {
	qe := &RWLockQueryExecutor{txmgr, false}
	qe.txmgr.commitRWLock.RLock()
	return qe, nil
}

// NewTxSimulator implements method in interface `txmgmt.TxMgr`
func (txmgr *LockBasedTxMgr) NewTxSimulator() (ledger.TxSimulator, error) {
	s := &LockBasedTxSimulator{RWLockQueryExecutor{txmgr, false}, make(map[string]*nsRWs)}
	s.txmgr.commitRWLock.RLock()
	return s, nil
}

// ValidateAndPrepare implements method in interface `txmgmt.TxMgr`
func (txmgr *LockBasedTxMgr) ValidateAndPrepare(block *common.Block) (*common.Block, []*pb.InvalidTransaction, error) {
	logger.Debugf("New block arrived for validation:%#v", block)
	invalidTxs := []*pb.InvalidTransaction{}
	var valid bool
	txmgr.updateSet = newUpdateSet()
	txmgr.blockNum = block.Header.Number
	logger.Debugf("Validating a block with [%d] transactions", len(block.Data.Data))
	for txIndex, envBytes := range block.Data.Data {
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
		//TODO add the validation info to the bitmap in the metadata of the block
		if valid {
			committingTxHeight := version.NewHeight(block.Header.Number, uint64(txIndex+1))
			if err := txmgr.addWriteSetToBatch(txRWSet, committingTxHeight); err != nil {
				return nil, nil, err
			}
		} else {
			invalidTxs = append(invalidTxs, &pb.InvalidTransaction{
				Transaction: &pb.Transaction{ /* FIXME */ }, Cause: pb.InvalidTransaction_RWConflictDuringCommit})
		}
	}
	return block, invalidTxs, nil
}

// Shutdown implements method in interface `txmgmt.TxMgr`
func (txmgr *LockBasedTxMgr) Shutdown() {
	txmgr.db.Close()
}

func (txmgr *LockBasedTxMgr) validateTx(txRWSet *txmgmt.TxReadWriteSet) (bool, error) {

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

func (txmgr *LockBasedTxMgr) addWriteSetToBatch(txRWSet *txmgmt.TxReadWriteSet, txHeight *version.Height) error {
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
func (txmgr *LockBasedTxMgr) Commit() error {
	batch := &leveldb.Batch{}
	if txmgr.updateSet == nil {
		panic("validateAndPrepare() method should have been called before calling commit()")
	}
	for k, v := range txmgr.updateSet.m {
		if v.value == nil {
			batch.Delete([]byte(k))
		} else {
			batch.Put([]byte(k), encodeValue(v.value, v.version))
		}
	}

	// record the savepoint along with batch
	if txmgr.blockNum != 0 {
		savepointValue := make([]byte, reflect.TypeOf(txmgr.blockNum).Size())
		binary.LittleEndian.PutUint64(savepointValue, txmgr.blockNum)
		// Need a composite key for iterator to function correctly - use separator itself as special/hidden namespace
		batch.Put(constructCompositeKey(string(compositeKeySep), savepointKey), savepointValue)
	}

	txmgr.commitRWLock.Lock()
	defer txmgr.commitRWLock.Unlock()
	defer func() { txmgr.updateSet = nil }()
	if err := txmgr.db.WriteBatch(batch, false); err != nil {
		return err
	}
	return nil
}

// GetBlockNumFromSavepoint returns the block num recorded in savepoint,
// returns 0 if NO savepoint is found
func (txmgr *LockBasedTxMgr) GetBlockNumFromSavepoint() (uint64, error) {
	var blockNum uint64
	savepointValue, err := txmgr.db.Get(constructCompositeKey(string(compositeKeySep), savepointKey))
	if err != nil {
		return 0, err
	}

	// savepointValue is not encoded with version
	blockNum = binary.LittleEndian.Uint64(savepointValue)
	return blockNum, nil
}

// Rollback implements method in interface `txmgmt.TxMgr`
func (txmgr *LockBasedTxMgr) Rollback() {
	txmgr.updateSet = nil
	txmgr.blockNum = 0
}

func (txmgr *LockBasedTxMgr) getCommitedVersion(ns string, key string) (*version.Height, error) {
	var err error
	var version *version.Height
	if _, version, err = txmgr.getCommittedValueAndVersion(ns, key); err != nil {
		return nil, err
	}
	return version, nil
}

func (txmgr *LockBasedTxMgr) getCommittedValueAndVersion(ns string, key string) ([]byte, *version.Height, error) {
	compositeKey := constructCompositeKey(ns, key)
	var encodedValue []byte
	var err error
	if encodedValue, err = txmgr.db.Get(compositeKey); err != nil {
		return nil, nil, err
	}
	if encodedValue == nil {
		return nil, nil, nil
	}
	value, version := decodeValue(encodedValue)
	return value, version, nil
}

func (txmgr *LockBasedTxMgr) getCommittedRangeScanner(namespace string, startKey string, endKey string) (*kvScanner, error) {
	var compositeStartKey []byte
	var compositeEndKey []byte
	if startKey != "" {
		compositeStartKey = constructCompositeKey(namespace, startKey)
	}
	if endKey != "" {
		compositeEndKey = constructCompositeKey(namespace, endKey)
	}

	dbItr := txmgr.db.GetIterator(compositeStartKey, compositeEndKey)
	return newKVScanner(namespace, dbItr), nil
}

func encodeValue(value []byte, version *version.Height) []byte {
	encodedValue := version.ToBytes()
	if value != nil {
		encodedValue = append(encodedValue, value...)
	}
	return encodedValue
}

func decodeValue(encodedValue []byte) ([]byte, *version.Height) {
	version, n := version.NewHeightFromBytes(encodedValue)
	value := encodedValue[n:]
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
	namespace string
	dbItr     iterator.Iterator
}

type committedKV struct {
	key     string
	version *version.Height
	value   []byte
}

func (cKV *committedKV) isDelete() bool {
	return cKV.value == nil
}

func newKVScanner(namespace string, dbItr iterator.Iterator) *kvScanner {
	return &kvScanner{namespace, dbItr}
}

func (scanner *kvScanner) next() (*committedKV, error) {
	if !scanner.dbItr.Next() {
		return nil, nil
	}
	_, key := splitCompositeKey(scanner.dbItr.Key())
	value, version := decodeValue(scanner.dbItr.Value())
	return &committedKV{key, version, value}, nil
}

func (scanner *kvScanner) close() {
	scanner.dbItr.Release()
}
