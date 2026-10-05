/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statecouchdb

import (
	"bytes"
	"encoding/gob"

	db "github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/common/ledger/util/dbfactory"
	"github.com/hyperledger/fabric/core/ledger/internal/version"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
)

var redoLogKey = []byte{byte(0)}

type redoLoggerProvider struct {
	dbProvider db.Provider
}

type redoLogger struct {
	dbHandle db.DBHandle
}

type redoRecord struct {
	UpdateBatch *statedb.UpdateBatch
	Version     *version.Height
}

func newRedoLoggerProvider(dirPath, dbType string) (*redoLoggerProvider, error) {
	provider, err := dbfactory.NewProvider(dbType, dirPath, "")
	if err != nil {
		return nil, err
	}
	return &redoLoggerProvider{dbProvider: provider}, nil
}

func (p *redoLoggerProvider) newRedoLogger(dbName string) *redoLogger {
	return &redoLogger{
		dbHandle: p.dbProvider.GetDBHandle(dbName),
	}
}

func (p *redoLoggerProvider) close() {
	p.dbProvider.Close()
}

func (l *redoLogger) persist(r *redoRecord) error {
	v, err := encodeRedologVal(r)
	if err != nil {
		return err
	}
	return l.dbHandle.Put(redoLogKey, v, true)
}

func (l *redoLogger) load() (*redoRecord, error) {
	v, err := l.dbHandle.Get(redoLogKey)
	if err != nil || v == nil {
		return nil, err
	}
	return decodeRedologVal(v)
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
