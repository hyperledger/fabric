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

// RebuildDBs drops existing ledger databases.
// Dropped database will be rebuilt upon server restart
func RebuildDBs(config *ledger.Config) error {
	rootFSPath := config.RootFSPath
	fileLockPath := fileLockPath(rootFSPath)
	fileLock := leveldbhelper.NewFileLock(fileLockPath)
	if err := fileLock.Lock(); err != nil {
		return errors.Wrap(err, "as another peer node command is executing,"+
			" wait for that command to complete its execution or terminate it before retrying")
	}
	defer fileLock.Unlock()

	blockstorePath := BlockStorePath(rootFSPath)
	ledgerIDs, err := blkstorage.GetLedgersBootstrappedFromSnapshot(blockstorePath)
	if err != nil {
		return errors.WithMessage(err, "error while checking if any ledger has been bootstrapped from snapshot")
	}
	if len(ledgerIDs) > 0 {
		return errors.Errorf("cannot rebuild databases because the peer contains channel(s) %s that were bootstrapped from snapshot", ledgerIDs)
	}

	if config.StateDBConfig.StateDatabase == ledger.CouchDB {
		if err := statecouchdb.DropApplicationDBs(config.StateDBConfig.CouchDB); err != nil {
			return err
		}
	}
	if err := dropDBs(rootFSPath); err != nil {
		return err
	}
	return blkstorage.DeleteBlockStoreIndex(blockstorePath)
}
