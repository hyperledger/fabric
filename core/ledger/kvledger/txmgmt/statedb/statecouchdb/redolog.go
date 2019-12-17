/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package statecouchdb

import (
	"bytes"
	"encoding/gob"

	"github.com/hyperledger/fabric/common/ledger/util/leveldbhelper"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
)

var redologKeyPrefix = []byte{byte(0)}

type redoLoggerProvider struct {
	leveldbProvider *leveldbhelper.Provider
}
type redoLogger struct {
	dbName   string
	dbHandle *leveldbhelper.DBHandle
}

type redoRecord struct {
	UpdateBatch *statedb.UpdateBatch
	Version     *version.Height
}

func newRedoLoggerProvider(dirPath string) (*redoLoggerProvider, error) {
	provider, err := leveldbhelper.NewProvider(&leveldbhelper.Conf{DBPath: dirPath})
	if err != nil {
		return nil, err
	}
	return &redoLoggerProvider{leveldbProvider: provider}, nil
}

func (p *redoLoggerProvider) newRedoLogger(dbName string) *redoLogger {
	return &redoLogger{
		dbHandle: p.leveldbProvider.GetDBHandle(dbName),
	}
}

func (p *redoLoggerProvider) close() {
	p.leveldbProvider.Close()
}

func (l *redoLogger) persist(r *redoRecord) error {
	k := encodeRedologKey(l.dbName)
	v, err := encodeRedologVal(r)
	if err != nil {
		return err
	}
	return l.dbHandle.Put(k, v, true)
}

func (l *redoLogger) load() (*redoRecord, error) {
	k := encodeRedologKey(l.dbName)
	v, err := l.dbHandle.Get(k)
	if err != nil || v == nil {
		return nil, err
	}
	return decodeRedologVal(v)
}

func encodeRedologKey(dbName string) []byte {
	return append(redologKeyPrefix, []byte(dbName)...)
}

func encodeRedologVal(r *redoRecord) ([]byte, error) {
	buf := bytes.NewBuffer(nil)
	encoder := gob.NewEncoder(buf)
	if err := encoder.Encode(r); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decodeRedologVal(b []byte) (*redoRecord, error) {
	decoder := gob.NewDecoder(bytes.NewBuffer(b))
	var r *redoRecord
	if err := decoder.Decode(&r); err != nil {
		return nil, err
	}
	return r, nil
}
