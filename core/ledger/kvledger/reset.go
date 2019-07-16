/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	"github.com/hyperledger/fabric/common/ledger/blkstorage/fsblkstorage"
	"github.com/hyperledger/fabric/common/ledger/util/leveldbhelper"
	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
	"github.com/pkg/errors"
)

// ResetAllKVLedgers resets all ledger to the genesis block.
func ResetAllKVLedgers() error {
	fileLock := leveldbhelper.NewFileLock(ledgerconfig.GetFileLockPath())
	if err := fileLock.Lock(); err != nil {
		return errors.Wrap(err, "as another peer node command is executing,"+
			" wait for that command to complete its execution or terminate it before retrying")
	}
	defer fileLock.Unlock()

	logger.Info("Resetting all ledgers to genesis block")
	ledgerDataFolder := ledgerconfig.GetRootPath()
	logger.Infof("Ledger data folder from config = [%s]", ledgerDataFolder)

	if err := dropDBs(); err != nil {
		return err
	}

	if err := resetBlockStorage(); err != nil {
		return err
	}
	logger.Info("All channel ledgers have been successfully reset to the genesis block")
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
