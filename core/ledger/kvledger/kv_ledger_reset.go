/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	"os"

	"github.com/hyperledger/fabric/common/ledger/blkstorage/fsblkstorage"
	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
)

func ResetAllKVLedgers() error {
	logger.Info("Resetting all ledgers to genesis block")
	ledgerDataFolder := ledgerconfig.GetRootPath()
	logger.Infof("Ledger data folder from config = [%s]", ledgerDataFolder)
	if err := dropHistoryDB(); err != nil {
		return err
	}
	if err := dropStateLevelDB(); err != nil {
		return err
	}
	if err := resetBlockStorage(); err != nil {
		return err
	}
	logger.Info("Done!")
	return nil
}

func LoadPreResetHeight() (map[string]uint64, error) {
	blockstorePath := ledgerconfig.GetBlockStorePath()
	logger.Infof("Loading prereset height from path [%s]", blockstorePath)
	return fsblkstorage.LoadPreResetHeight(blockstorePath)
}

func dropHistoryDB() error {
	histroryDBPath := ledgerconfig.GetHistoryLevelDBPath()
	logger.Infof("Dropping HistoryDB at location [%s] ...if present", histroryDBPath)
	return os.RemoveAll(histroryDBPath)
}

func dropStateLevelDB() error {
	stateLeveldbPath := ledgerconfig.GetStateLevelDBPath()
	logger.Infof("Dropping StateLevelDB at location [%s] ...if present", stateLeveldbPath)
	return os.RemoveAll(stateLeveldbPath)
}

func resetBlockStorage() error {
	blockstorePath := ledgerconfig.GetBlockStorePath()
	logger.Infof("Resetting BlockStore to genesis block at location [%s]", blockstorePath)
	return fsblkstorage.ResetBlockStore(blockstorePath)
}
