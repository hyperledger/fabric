/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package dbfactory is the single place where a database type becomes a working
// store. Adding a store means two steps: a new package that implements db.DB,
// db.Provider and db.FileLock, and that provides CreateDB, NewProvider,
// NewFileLock and RetrieveDataFormatInfo over it; and a branch for its type
// constant in each of the four functions here. The type arrives here as a
// parameter, so no other package learns about the new store.
//
// Only goleveldb is implemented. An empty or unknown type falls back to it, and
// that fallback is a safeguard for parsing the configuration rather than a way
// to pick a store.
package dbfactory

import (
	db "github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/common/ledger/util/leveldbhelper"
)

// CreateDB creates a database of the specified type.
func CreateDB(dbType string, dbPath string, expectedFormat string) db.DB {
	createLevelDB := func() db.DB {
		return leveldbhelper.CreateDB(&leveldbhelper.Conf{
			DBPath:         dbPath,
			ExpectedFormat: expectedFormat,
		})
	}

	switch dbType {
	case db.GoLevelDB:
		return createLevelDB()
	default:
		return createLevelDB()
	}
}

// NewProvider creates a provider of the specified type.
func NewProvider(dbType string, dbPath string, expectedFormat string) (db.Provider, error) {
	newLevelDBProvider := func() (db.Provider, error) {
		return leveldbhelper.NewProvider(&leveldbhelper.Conf{
			DBPath:         dbPath,
			ExpectedFormat: expectedFormat,
		})
	}

	switch dbType {
	case db.GoLevelDB:
		return newLevelDBProvider()
	default:
		return newLevelDBProvider()
	}
}

// NewFileLock creates a FileLock based on the dbType.
func NewFileLock(dbType, filePath string) db.FileLock {
	newLevelDBFileLock := func() db.FileLock {
		return leveldbhelper.NewFileLock(filePath)
	}

	switch dbType {
	case db.GoLevelDB:
		return newLevelDBFileLock()
	default:
		return newLevelDBFileLock()
	}
}

func RetrieveDataFormatInfo(dbType, dbPath string) (formatVerison string, isDBEmpty bool, err error) {
	retrieveLevelDBDataFormatInfo := func() (string, bool, error) {
		return leveldbhelper.RetrieveDataFormatInfo(dbPath)
	}

	switch dbType {
	case db.GoLevelDB:
		return retrieveLevelDBDataFormatInfo()
	default:
		return retrieveLevelDBDataFormatInfo()
	}
}
