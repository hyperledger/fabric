/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	"path/filepath"

	"github.com/hyperledger/fabric/common/ledger/blkstorage/fsblkstorage"
)

// ResetAllKVLedgers resets all ledger to the genesis block.
func ResetAllKVLedgers(rootFSPath string) error {
	logger.Info("Resetting all ledgers to genesis block")
	logger.Infof("Ledger data folder from config = [%s]", rootFSPath)
	if err := dropDBs(rootFSPath); err != nil {
		return err
	}
	if err := resetBlockStorage(rootFSPath); err != nil {
		return err
	}
	logger.Info("Done!")
	return nil
}

// LoadPreResetHeight returns the prereset height of all ledgers.
func LoadPreResetHeight(rootFSPath string) (map[string]uint64, error) {
	blockstorePath := filepath.Join(rootFSPath, "chains")
	logger.Infof("Loading prereset height from path [%s]", blockstorePath)
	return fsblkstorage.LoadPreResetHeight(blockstorePath)
}

// ClearPreResetHeight removes the prereset height recorded in the file system.
func ClearPreResetHeight(rootFSPath string) error {
	blockstorePath := filepath.Join(rootFSPath, "chains")
	logger.Infof("Clearing off prereset height files from path [%s]", blockstorePath)
	return fsblkstorage.ClearPreResetHeight(blockstorePath)
}

func resetBlockStorage(rootFSPath string) error {
	blockstorePath := filepath.Join(rootFSPath, "chains")
	logger.Infof("Resetting BlockStore to genesis block at location [%s]", blockstorePath)
	return fsblkstorage.ResetBlockStore(blockstorePath)
}
