/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	"path/filepath"

	"github.com/hyperledger/fabric/core/ledger/ledgerstorage"
)

// RollbackKVLedger rollbacks a ledger to a specified block number
func RollbackKVLedger(rootFSPath, ledgerID string, blockNum uint64) error {
	blockstorePath := filepath.Join(rootFSPath, "chains")
	if err := ledgerstorage.ValidateRollbackParams(blockstorePath, ledgerID, blockNum); err != nil {
		return err
	}

	logger.Infof("Dropping databases")
	if err := dropDBs(rootFSPath); err != nil {
		return err
	}

	logger.Info("Rolling back ledger store")
	if err := ledgerstorage.Rollback(blockstorePath, ledgerID, blockNum); err != nil {
		return err
	}
	logger.Infof("The channel [%s] has been successfully rolled back to the block number [%d]", ledgerID, blockNum)
	return nil
}
