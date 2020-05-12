/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	"github.com/hyperledger/fabric/common/ledger/blkstorage"
	"github.com/hyperledger/fabric/common/ledger/util/leveldbhelper"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb/statecouchdb"
	"github.com/pkg/errors"
)

// UpgradeDBs upgrades existing ledger databases to the latest formats.
// It checks the format of idStore and does not drop any databases
// if the format is already the latest version. Otherwise, it drops
// ledger databases and upgrades the idStore format.
func UpgradeDBs(config *ledger.Config) error {
	rootFSPath := config.RootFSPath
	fileLockPath := fileLockPath(rootFSPath)
	fileLock := leveldbhelper.NewFileLock(fileLockPath)
	if err := fileLock.Lock(); err != nil {
		return errors.Wrap(err, "as another peer node command is executing,"+
			" wait for that command to complete its execution or terminate it before retrying")
	}
	defer fileLock.Unlock()

	logger.Infof("Ledger data folder from config = [%s]", rootFSPath)

	if config.StateDBConfig.StateDatabase == "CouchDB" {
		if err := statecouchdb.DropApplicationDBs(config.StateDBConfig.CouchDB); err != nil {
			return err
		}
	}
	if err := dropDBs(rootFSPath); err != nil {
		return err
	}
	if err := blkstorage.DeleteBlockStoreIndex(BlockStorePath(rootFSPath)); err != nil {
		return err
	}

	dbPath := LedgerProviderPath(rootFSPath)
	db := leveldbhelper.CreateDB(&leveldbhelper.Conf{DBPath: dbPath})
	db.Open()
	defer db.Close()
	idStore := &idStore{db, dbPath}
	return idStore.upgradeFormat()
}
