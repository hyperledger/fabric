/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import "github.com/hyperledger/fabric/internal/fileutil"

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
	return dropHistoryDB(rootFSPath)
}

func dropStateLevelDB(rootFSPath string) error {
	stateLeveldbPath := StateDBPath(rootFSPath)
	logger.Infof("Dropping all contents in StateLevelDB at location [%s] ...if present", stateLeveldbPath)
	return fileutil.RemoveContents(stateLeveldbPath)
}

func dropConfigHistoryDB(rootFSPath string) error {
	configHistoryDBPath := ConfigHistoryDBPath(rootFSPath)
	logger.Infof("Dropping all contents in ConfigHistoryDB at location [%s] ...if present", configHistoryDBPath)
	return fileutil.RemoveContents(configHistoryDBPath)
}

func dropBookkeeperDB(rootFSPath string) error {
	bookkeeperDBPath := BookkeeperDBPath(rootFSPath)
	logger.Infof("Dropping all contents in BookkeeperDB at location [%s] ...if present", bookkeeperDBPath)
	return fileutil.RemoveContents(bookkeeperDBPath)
}

func dropHistoryDB(rootFSPath string) error {
	historyDBPath := HistoryDBPath(rootFSPath)
	logger.Infof("Dropping all contents under in HistoryDB at location [%s] ...if present", historyDBPath)
	return fileutil.RemoveContents(historyDBPath)
}
