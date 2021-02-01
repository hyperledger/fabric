/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statecouchdb

import (
	"bytes"
	"encoding/gob"

	"github.com/hyperledger/fabric/common/ledger/util/leveldbhelper"
	"github.com/hyperledger/fabric/core/ledger/internal/version"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
)

var redoLogKey = []byte{byte(0)}

type redoLoggerProvider struct {
	leveldbProvider *leveldbhelper.Provider
}

type redoLogger struct {
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
