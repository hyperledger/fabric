/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	"github.com/hyperledger/fabric/common/ledger/blkstorage/fsblkstorage"
	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
)

// ResetAllKVLedgers resets all ledger to the genesis block.
func ResetAllKVLedgers() error {
	logger.Info("Resetting all ledgers to genesis block")
	ledgerDataFolder := ledgerconfig.GetRootPath()
	logger.Infof("Ledger data folder from config = [%s]", ledgerDataFolder)

	if err := dropDBs(); err != nil {
		return err
	}

	if err := resetBlockStorage(); err != nil {
		return err
	}
	logger.Info("Done!")
	return nil
}

// LoadPreResetHeight returns the pre-reset height of all ledgers.
func LoadPreResetHeight() (map[string]uint64, error) {
	blockstorePath := ledgerconfig.GetBlockStorePath()
	logger.Infof("Loading prereset height from path [%s]", blockstorePath)
	return fsblkstorage.LoadPreResetHeight(blockstorePath)
}

func ClearPreResetHeight() error {
	blockstorePath := ledgerconfig.GetBlockStorePath()
	logger.Infof("Clearing off prereset height files from path [%s]", blockstorePath)
	return fsblkstorage.ClearPreResetHeight(blockstorePath)
}

func resetBlockStorage() error {
	blockstorePath := ledgerconfig.GetBlockStorePath()
	logger.Infof("Resetting BlockStore to genesis block at location [%s]", blockstorePath)
	return fsblkstorage.ResetBlockStore(blockstorePath)
}
