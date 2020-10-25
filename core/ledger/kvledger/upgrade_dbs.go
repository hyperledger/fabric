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

	dbPath := LedgerProviderPath(rootFSPath)
	db := leveldbhelper.CreateDB(&leveldbhelper.Conf{DBPath: dbPath})
	db.Open()
	defer db.Close()
	idStore := &idStore{db, dbPath}

	// Check upfront whether we should upgrade the data format before dropping databases.
	// If someone mistakenly executes the upgrade command in a peer that has some channels that
	// are bootstrapped from a snapshot, the peer will not be able to start as the data for those channels
	// cannot be recovered
	isEligible, err := idStore.checkUpgradeEligibility()
	if err != nil {
		return errors.WithMessage(err, "error while checking whether upgrade is required")
	}
	if !isEligible {
		return errors.New("the data format is already up to date. No upgrade is required")
	}

	if config.StateDBConfig.StateDatabase == ledger.CouchDB {
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

	return idStore.upgradeFormat()
}
