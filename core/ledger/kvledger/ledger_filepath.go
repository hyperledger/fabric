/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	"path/filepath"
)

func fileLockPath(rootFSPath string) string {
	return filepath.Join(rootFSPath, "fileLock")
}

// LedgerProviderPath returns the absolute path of ledgerprovider
func LedgerProviderPath(rootFSPath string) string {
	return filepath.Join(rootFSPath, "ledgerProvider")
}

// BlockStorePath returns the absolute path of block storage
func BlockStorePath(rootFSPath string) string {
	return filepath.Join(rootFSPath, "chains")
}

// PvtDataStorePath returns the absolute path of pvtdata storage
func PvtDataStorePath(rootFSPath string) string {
	return filepath.Join(rootFSPath, "pvtdataStore")
}

// StateDBPath returns the absolute path of state level DB
func StateDBPath(rootFSPath string) string {
	return filepath.Join(rootFSPath, "stateLeveldb")
}

// HistoryDBPath returns the absolute path of history DB
func HistoryDBPath(rootFSPath string) string {
	return filepath.Join(rootFSPath, "historyLeveldb")
}

// ConfigHistoryDBPath returns the absolute path of configHistory DB
func ConfigHistoryDBPath(rootFSPath string) string {
	return filepath.Join(rootFSPath, "configHistory")
}

// BookkeeperDBPath return the absolute path of bookkeeper DB
func BookkeeperDBPath(rootFSPath string) string {
	return filepath.Join(rootFSPath, "bookkeeper")
}
