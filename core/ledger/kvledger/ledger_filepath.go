/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	"path/filepath"
	"strconv"
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

// SnapshotsTempDirPath returns the dir path that is used temporarily during the genration or import of the snapshots for a ledger
func SnapshotsTempDirPath(snapshotRootDir string) string {
	return filepath.Join(snapshotRootDir, "temp")
}

// CompletedSnapshotsPath returns the absolute path that is used for persisting the snapshots
func CompletedSnapshotsPath(snapshotRootDir string) string {
	return filepath.Join(snapshotRootDir, "completed")
}

// SnapshotsDirForLedger returns the absolute path of the dir for the snapshots for a specified ledger
func SnapshotsDirForLedger(snapshotRootDir, ledgerID string) string {
	return filepath.Join(CompletedSnapshotsPath(snapshotRootDir), ledgerID)
}

// SnapshotDirForLedgerBlockNum returns the absolute path for a particular snapshot for a ledger
func SnapshotDirForLedgerBlockNum(snapshotRootDir, ledgerID string, blockNumber uint64) string {
	return filepath.Join(SnapshotsDirForLedger(snapshotRootDir, ledgerID), strconv.FormatUint(blockNumber, 10))
}
