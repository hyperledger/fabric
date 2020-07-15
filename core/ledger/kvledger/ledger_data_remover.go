/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	"github.com/hyperledger/fabric/common/ledger/blkstorage"
	"github.com/hyperledger/fabric/core/ledger/confighistory"
	"github.com/hyperledger/fabric/core/ledger/kvledger/bookkeeping"
	"github.com/hyperledger/fabric/core/ledger/kvledger/history"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/privacyenabledstate"
	"github.com/hyperledger/fabric/core/ledger/pvtdatastorage"
)

type ledgerDataRemover struct {
	blkStoreProvider     *blkstorage.BlockStoreProvider
	statedbProvider      *privacyenabledstate.DBProvider
	configHistoryMgr     *confighistory.Mgr
	bookkeepingProvider  *bookkeeping.Provider
	historydbProvider    *history.DBProvider
	pvtdataStoreProvider *pvtdatastorage.Provider
}

// Drop drops channel-specific data from all the ledger DBs, which includes
// stateDB, configHistoryDB, bookkeeperDB, historyDB, pvtdataStore, block index and blocks directory.
// This function can be called multiple times for the same ledgerID. It is not an error if the ledger
// does not exist. The data consistency and concurrency control will be handled outside of this function.
func (r *ledgerDataRemover) Drop(ledgerID string) error {
	logger.Infow("Dropping ledger data", "channel", ledgerID)
	var err error
	if err = r.statedbProvider.Drop(ledgerID); err != nil {
		logger.Errorw("failed to drop ledger data from stateDB", "channel", ledgerID, "error", err)
		return err
	}

	if err = r.configHistoryMgr.Drop(ledgerID); err != nil {
		logger.Errorw("failed to drop ledger data from configHistoryDB", "channel", ledgerID, "error", err)
		return err
	}

	if err := r.bookkeepingProvider.Drop(ledgerID); err != nil {
		logger.Errorw("failed to drop ledger data from bookkeepingDB", "channel", ledgerID, "error", err)
		return err
	}

	if r.historydbProvider != nil {
		if err = r.historydbProvider.Drop(ledgerID); err != nil {
			logger.Errorw("failed to drop ledger data from historyDB", "channel", ledgerID, "error", err)
			return err
		}
	}

	if err = r.pvtdataStoreProvider.Drop(ledgerID); err != nil {
		logger.Errorw("failed to drop ledger data from pvtdataStore", "channel", ledgerID, "error", err)
		return err
	}

	if err = r.blkStoreProvider.Drop(ledgerID); err != nil {
		logger.Errorw("failed to drop ledger data from blockstore", "channel", ledgerID, "error", err)
		return err
	}
	return nil
}
