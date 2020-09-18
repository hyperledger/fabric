/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package confighistory

import (
	"bytes"
	"encoding/binary"
	"math"

	"github.com/hyperledger/fabric/common/ledger/util/leveldbhelper"
	"github.com/pkg/errors"
)

const (
	keyPrefix     = "s"
	separatorByte = byte(0)
	nsStopper     = byte(1)
)

type compositeKey struct {
	ns, key  string
	blockNum uint64
}

type compositeKV struct {
	*compositeKey
	value []byte
}

type dbProvider struct {
	*leveldbhelper.Provider
}

type db struct {
	*leveldbhelper.DBHandle
}

type batch struct {
	*leveldbhelper.UpdateBatch
}

func newDBProvider(dbPath string) (*dbProvider, error) {
	logger.Debugf("Opening db for config history: db path = %s", dbPath)
	p, err := leveldbhelper.NewProvider(&leveldbhelper.Conf{DBPath: dbPath})
	if err != nil {
		return nil, err
	}
	return &dbProvider{Provider: p}, nil
}

func (d *db) newBatch() *batch {
	return &batch{d.DBHandle.NewUpdateBatch()}
}

func (p *dbProvider) getDB(id string) *db {
	return &db{p.GetDBHandle(id)}
}

func (b *batch) add(ns, key string, blockNum uint64, value []byte) {
	logger.Debugf("add() - {%s, %s, %d}", ns, key, blockNum)
	k, v := encodeCompositeKey(ns, key, blockNum), value
	b.Put(k, v)
}

func (d *db) writeBatch(batch *batch, sync bool) error {
	return d.WriteBatch(batch.UpdateBatch, sync)
}

func (d *db) mostRecentEntryBelow(blockNum uint64, ns, key string) (*compositeKV, error) {
	logger.Debugf("mostRecentEntryBelow() - {%s, %s, %d}", ns, key, blockNum)
	if blockNum == 0 {
		return nil, errors.New("blockNum should be greater than 0")
	}

	startKey := encodeCompositeKey(ns, key, blockNum-1)
	stopKey := append(encodeCompositeKey(ns, key, 0), byte(0))

	itr, err := d.GetIterator(startKey, stopKey)
	if err != nil {
		return nil, err
	}
	defer itr.Release()
	if !itr.Next() {
		logger.Debugf("Key no entry found. Returning nil")
		return nil, nil
	}
	k, v := decodeCompositeKey(itr.Key()), itr.Value()
	return &compositeKV{k, v}, nil
}

func (d *db) entryAt(blockNum uint64, ns, key string) (*compositeKV, error) {
	logger.Debugf("entryAt() - {%s, %s, %d}", ns, key, blockNum)
	keyBytes := encodeCompositeKey(ns, key, blockNum)
	valBytes, err := d.Get(keyBytes)
	if err != nil {
		return nil, err
	}
	if valBytes == nil {
		return nil, nil
	}
	k, v := decodeCompositeKey(keyBytes), valBytes
	return &compositeKV{k, v}, nil
}

func (d *db) getNamespaceIterator(ns string) (*leveldbhelper.Iterator, error) {
	nsStartKey := []byte(keyPrefix + ns)
	nsStartKey = append(nsStartKey, separatorByte)
	nsEndKey := []byte(keyPrefix + ns)
	nsEndKey = append(nsEndKey, nsStopper)
	return d.GetIterator(nsStartKey, nsEndKey)
}

func encodeCompositeKey(ns, key string, blockNum uint64) []byte {
	b := []byte(keyPrefix + ns)
	b = append(b, separatorByte)
	b = append(b, []byte(key)...)
	return append(b, encodeBlockNum(blockNum)...)
}

func decodeCompositeKey(b []byte) *compositeKey {
	blockNumStartIndex := len(b) - 8
	nsKeyBytes, blockNumBytes := b[1:blockNumStartIndex], b[blockNumStartIndex:]
	separatorIndex := bytes.Index(nsKeyBytes, []byte{separatorByte})
	ns, key := nsKeyBytes[0:separatorIndex], nsKeyBytes[separatorIndex+1:]
	return &compositeKey{string(ns), string(key), decodeBlockNum(blockNumBytes)}
}

func encodeBlockNum(blockNum uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, math.MaxUint64-blockNum)
	return b
}

func decodeBlockNum(blockNumBytes []byte) uint64 {
	return math.MaxUint64 - binary.BigEndian.Uint64(blockNumBytes)
}
