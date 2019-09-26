/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	"github.com/hyperledger/fabric/common/ledger/blkstorage/fsblkstorage"
	"github.com/hyperledger/fabric/common/ledger/dataformat"
	"github.com/hyperledger/fabric/common/ledger/util/leveldbhelper"
	"github.com/pkg/errors"
)

// UpgradeDBs upgrades existing ledger databases to the latest formats
func UpgradeDBs(rootFSPath string) error {
	fileLockPath := fileLockPath(rootFSPath)
	fileLock := leveldbhelper.NewFileLock(fileLockPath)
	if err := fileLock.Lock(); err != nil {
		return errors.Wrap(err, "as another peer node command is executing,"+
			" wait for that command to complete its execution or terminate it before retrying")
	}
	defer fileLock.Unlock()

	logger.Infof("Ledger data folder from config = [%s]", rootFSPath)

	// upgrades idStore (ledgerProvider) and drop databases
	if err := UpgradeIDStoreFormat(rootFSPath); err != nil {
		return err
	}

	if err := dropDBs(rootFSPath); err != nil {
		return err
	}

	blockstorePath := BlockStorePath(rootFSPath)
	return fsblkstorage.DeleteBlockStoreIndex(blockstorePath)
}

// UpgradeIDStoreFormat upgrades the format for idStore
func UpgradeIDStoreFormat(rootFSPath string) error {
	logger.Debugf("Attempting to upgrade idStore data format to current format %s", dataformat.Version20)

	dbPath := LedgerProviderPath(rootFSPath)
	db := leveldbhelper.CreateDB(&leveldbhelper.Conf{DBPath: dbPath})
	db.Open()
	defer db.Close()

	idStore := &idStore{db, dbPath}
	return idStore.upgradeFormat()
}
