/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
	"github.com/hyperledger/fabric/core/ledger/ledgerstorage"
)

// RollbackKVLedger rollbacks a ledger to a specified block number
func RollbackKVLedger(ledgerID string, blockNum uint64) error {
	blockstorePath := ledgerconfig.GetBlockStorePath()
	if err := ledgerstorage.ValidateRollbackParams(blockstorePath, ledgerID, blockNum); err != nil {
		return err
	}

	logger.Infof("Dropping databases")
	if err := dropDBs(); err != nil {
		return err
	}

	logger.Info("Rollbacking ledger store")
	return ledgerstorage.Rollback(blockstorePath, ledgerID, blockNum)
}
