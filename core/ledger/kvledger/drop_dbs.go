/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	"os"

	"github.com/pkg/errors"
)

func dropDBs(rootFSPath string) error {
	// During block commits to stateDB, the transaction manager updates the bookkeeperDB and one of the
	// state listener updates the config historyDB. As we drop the stateDB, we need to drop the
	// configHistoryDB and bookkeeperDB too so that during the peer startup after the reset/rollback,
	// we can get a correct configHistoryDB.
	// Note that it is necessary to drop the stateDB first before dropping the config history and
	// bookkeeper. Suppose if the config or bookkeeper is dropped first and the peer reset/rollback
	// command fails before dropping the stateDB, peer cannot start with consistent data (if the
	// user decides to start the peer without retrying the reset/rollback) as the stateDB would
	// not be rebuilt.
	if err := dropStateLevelDB(rootFSPath); err != nil {
		return err
	}
	if err := dropConfigHistoryDB(rootFSPath); err != nil {
		return err
	}
	if err := dropBookkeeperDB(rootFSPath); err != nil {
		return err
	}
	if err := dropHistoryDB(rootFSPath); err != nil {
		return err
	}
	return nil
}

func dropStateLevelDB(rootFSPath string) error {
	stateLeveldbPath := StateDBPath(rootFSPath)
	logger.Infof("Dropping StateLevelDB at location [%s] ...if present", stateLeveldbPath)
	return os.RemoveAll(stateLeveldbPath)
}

func dropConfigHistoryDB(rootFSPath string) error {
	configHistoryDBPath := ConfigHistoryDBPath(rootFSPath)
	logger.Infof("Dropping ConfigHistoryDB at location [%s]", configHistoryDBPath)
	err := os.RemoveAll(configHistoryDBPath)
	return errors.Wrapf(err, "error removing the ConfigHistoryDB located at %s", configHistoryDBPath)
}

func dropBookkeeperDB(rootFSPath string) error {
	bookkeeperDBPath := BookkeeperDBPath(rootFSPath)
	logger.Infof("Dropping BookkeeperDB at location [%s]", bookkeeperDBPath)
	err := os.RemoveAll(bookkeeperDBPath)
	return errors.Wrapf(err, "error removing the BookkeeperDB located at %s", bookkeeperDBPath)
}

func dropHistoryDB(rootFSPath string) error {
	historyDBPath := HistoryDBPath(rootFSPath)
	logger.Infof("Dropping HistoryDB at location [%s] ...if present", historyDBPath)
	return os.RemoveAll(historyDBPath)
}
