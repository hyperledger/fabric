/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtdatastorage

import (
	"fmt"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/ledger/util/leveldbhelper"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
	"github.com/hyperledger/fabric/protos/ledger/rwset"
)

var logger = flogging.MustGetLogger("pvtdatastorage")

type provider struct {
	dbProvider *leveldbhelper.Provider
}

type store struct {
	db                 *leveldbhelper.DBHandle
	ledgerid           string
	isEmpty            bool
	lastCommittedBlock uint64
	batchPending       bool
}

type blkTranNumKey []byte

// NewProvider instantiates a StoreProvider
func NewProvider() Provider {
	dbPath := ledgerconfig.GetPvtdataStorePath()
	dbProvider := leveldbhelper.NewProvider(&leveldbhelper.Conf{DBPath: dbPath})
	return &provider{dbProvider: dbProvider}
}

// OpenStore returns a handle to a store
func (p *provider) OpenStore(ledgerid string) (Store, error) {
	dbHandle := p.dbProvider.GetDBHandle(ledgerid)
	s := &store{db: dbHandle, ledgerid: ledgerid}
	if err := s.initState(); err != nil {
		return nil, err
	}
	return s, nil
}

// Close closes the store
func (p *provider) Close() {
	p.dbProvider.Close()
}

func (s *store) initState() error {
	var err error
	if s.isEmpty, s.lastCommittedBlock, err = s.getLastCommittedBlockNum(); err != nil {
		return err
	}
	if s.batchPending, err = s.hasPendingCommit(); err != nil {
		return err
	}
	return nil
}

// Prepare implements the function in the interface `Store`
func (s *store) Prepare(blockNum uint64, pvtData []*ledger.TxPvtData) error {
	if s.batchPending {
		return &ErrIllegalCall{`A pending batch exists as as result of last invoke to "Prepare" call.
			 Invoke "Commit" or "Rollback" on the pending batch before invoking "Prepare" function`}
	}
	expectedBlockNum := s.nextBlockNum()
	if expectedBlockNum != blockNum {
		return &ErrIllegalArgs{fmt.Sprintf("Expected block number=%d, recived block number=%d", expectedBlockNum, blockNum)}
	}

	batch := leveldbhelper.NewUpdateBatch()
	var key, value []byte
	var err error
	for _, txPvtData := range pvtData {
		key = encodePK(blockNum, txPvtData.SeqInBlock)
		if value, err = encodePvtRwSet(txPvtData.WriteSet); err != nil {
			return err
		}
		logger.Debugf("Adding private data to batch blockNum=%d, tranNum=%d", blockNum, txPvtData.SeqInBlock)
		batch.Put(key, value)
	}
	batch.Put(pendingCommitKey, emptyValue)
	if err := s.db.WriteBatch(batch, true); err != nil {
		return err
	}
	s.batchPending = true
	return nil
}

// Commit implements the function in the interface `Store`
func (s *store) Commit() error {
	if !s.batchPending {
		return &ErrIllegalCall{"No pending batch to commit"}
	}
	committingBlockNum := s.nextBlockNum()
	logger.Debugf("Committing pvt data for block = %d", committingBlockNum)
	batch := leveldbhelper.NewUpdateBatch()
	batch.Delete(pendingCommitKey)
	batch.Put(lastCommittedBlkkey, encodeBlockNum(committingBlockNum))
	if err := s.db.WriteBatch(batch, true); err != nil {
		return err
	}
	s.batchPending = false
	s.isEmpty = false
	s.lastCommittedBlock = committingBlockNum
	logger.Debugf("Committed pvt data for block = %d", committingBlockNum)
	return nil
}

// Rollback implements the function in the interface `Store`
func (s *store) Rollback() error {
	var pendingBatchKeys []blkTranNumKey
	var err error
	if !s.batchPending {
		return &ErrIllegalCall{"No pending batch to rollback"}
	}
	rollingbackBlockNum := s.nextBlockNum()
	logger.Debugf("Rolling back pvt data for block = %d", rollingbackBlockNum)

	if pendingBatchKeys, err = s.retrievePendingBatchKeys(); err != nil {
		return err
	}
	batch := leveldbhelper.NewUpdateBatch()
	for _, key := range pendingBatchKeys {
		batch.Delete(key)
	}
	batch.Delete(pendingCommitKey)
	if err := s.db.WriteBatch(batch, true); err != nil {
		return err
	}
	s.batchPending = false
	logger.Debugf("Rolled back pvt data for block = %d", rollingbackBlockNum)
	return nil
}

// GetPvtDataByBlockNum implements the function in the interface `Store`.
// If the store is empty or the last committed block number is smaller then the
// requested block number, an 'ErrOutOfRange' is thrown
func (s *store) GetPvtDataByBlockNum(blockNum uint64, filter ledger.PvtNsCollFilter) ([]*ledger.TxPvtData, error) {
	logger.Debugf("GetPvtDataByBlockNum(): blockNum=%d, filter=%#v", blockNum, filter)
	if s.isEmpty {
		return nil, &ErrOutOfRange{"The store is empty"}
	}

	if blockNum > s.lastCommittedBlock {
		return nil, &ErrOutOfRange{fmt.Sprintf("Last committed block=%d, block requested=%d", s.lastCommittedBlock, blockNum)}
	}
	var pvtData []*ledger.TxPvtData
	startKey, endKey := getKeysForRangeScanByBlockNum(blockNum)
	logger.Debugf("GetPvtDataByBlockNum(): startKey=%#v, endKey=%#v", startKey, endKey)
	itr := s.db.GetIterator(startKey, endKey)
	defer itr.Release()

	var pvtWSet *rwset.TxPvtReadWriteSet
	var err error
	for itr.Next() {
		bNum, tNum := decodePK(itr.Key())
		if pvtWSet, err = decodePvtRwSet(itr.Value()); err != nil {
			return nil, err
		}
		logger.Debugf("Retrieving pvtdata for bNum=%d, tNum=%d", bNum, tNum)
		filteredWSet := TrimPvtWSet(pvtWSet, filter)
		pvtData = append(pvtData, &ledger.TxPvtData{SeqInBlock: tNum, WriteSet: filteredWSet})
	}
	return pvtData, nil
}

// InitLastCommittedBlock implements the function in the interface `Store`
func (s *store) InitLastCommittedBlock(blockNum uint64) error {
	if !(s.isEmpty && !s.batchPending) {
		return &ErrIllegalCall{"The pvtdata store is not empty. InitLastCommittedBlock() function call is not allowed"}
	}
	batch := leveldbhelper.NewUpdateBatch()
	batch.Put(lastCommittedBlkkey, encodeBlockNum(blockNum))
	if err := s.db.WriteBatch(batch, true); err != nil {
		return err
	}
	s.isEmpty = false
	s.lastCommittedBlock = blockNum
	logger.Debugf("InitLastCommittedBlock set to = %d", blockNum)
	return nil
}

// LastCommittedBlockHeight implements the function in the interface `Store`
func (s *store) LastCommittedBlockHeight() (uint64, error) {
	if s.isEmpty {
		return 0, nil
	}
	return s.lastCommittedBlock + 1, nil
}

// HasPendingBatch implements the function in the interface `Store`
func (s *store) HasPendingBatch() (bool, error) {
	return s.batchPending, nil
}

// IsEmpty implements the function in the interface `Store`
func (s *store) IsEmpty() (bool, error) {
	return s.isEmpty, nil
}

// Shutdown implements the function in the interface `Store`
func (s *store) Shutdown() {
	// do nothing
}

func (s *store) nextBlockNum() uint64 {
	if s.isEmpty {
		return 0
	}
	return s.lastCommittedBlock + 1
}

func (s *store) retrievePendingBatchKeys() ([]blkTranNumKey, error) {
	var pendingBatchKeys []blkTranNumKey
	itr := s.db.GetIterator(encodePK(s.nextBlockNum(), 0), nil)
	for itr.Next() {
		pendingBatchKeys = append(pendingBatchKeys, itr.Key())
	}
	return pendingBatchKeys, nil
}

func (s *store) hasPendingCommit() (bool, error) {
	var v []byte
	var err error
	if v, err = s.db.Get(pendingCommitKey); err != nil {
		return false, err
	}
	return v != nil, nil
}

func (s *store) getLastCommittedBlockNum() (bool, uint64, error) {
	var v []byte
	var err error
	if v, err = s.db.Get(lastCommittedBlkkey); v == nil || err != nil {
		return true, 0, err
	}
	return false, decodeBlockNum(v), nil
}

// TrimPvtWSet returns a `TxPvtReadWriteSet` that retains only list of 'ns/collections' supplied in the filter
// A nil filter does not filter any results and returns the original `pvtWSet` as is
func TrimPvtWSet(pvtWSet *rwset.TxPvtReadWriteSet, filter ledger.PvtNsCollFilter) *rwset.TxPvtReadWriteSet {
	if filter == nil {
		return pvtWSet
	}

	var filteredNsRwSet []*rwset.NsPvtReadWriteSet
	for _, ns := range pvtWSet.NsPvtRwset {
		var filteredCollRwSet []*rwset.CollectionPvtReadWriteSet
		for _, coll := range ns.CollectionPvtRwset {
			if filter.Has(ns.Namespace, coll.CollectionName) {
				filteredCollRwSet = append(filteredCollRwSet, coll)
			}
		}
		if filteredCollRwSet != nil {
			filteredNsRwSet = append(filteredNsRwSet,
				&rwset.NsPvtReadWriteSet{
					Namespace:          ns.Namespace,
					CollectionPvtRwset: filteredCollRwSet,
				},
			)
		}
	}
	var filteredTxPvtRwSet *rwset.TxPvtReadWriteSet
	if filteredNsRwSet != nil {
		filteredTxPvtRwSet = &rwset.TxPvtReadWriteSet{
			DataModel:  pvtWSet.GetDataModel(),
			NsPvtRwset: filteredNsRwSet,
		}
	}
	return filteredTxPvtRwSet
}
