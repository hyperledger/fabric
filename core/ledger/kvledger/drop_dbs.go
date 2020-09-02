/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	"io/ioutil"
	"os"
	"path/filepath"

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
	logger.Infof("Dropping all contents in StateLevelDB at location [%s] ...if present", stateLeveldbPath)
	return RemoveContents(stateLeveldbPath)
}

func dropConfigHistoryDB(rootFSPath string) error {
	configHistoryDBPath := ConfigHistoryDBPath(rootFSPath)
	logger.Infof("Dropping all contents in ConfigHistoryDB at location [%s] ...if present", configHistoryDBPath)
	return RemoveContents(configHistoryDBPath)
}

func dropBookkeeperDB(rootFSPath string) error {
	bookkeeperDBPath := BookkeeperDBPath(rootFSPath)
	logger.Infof("Dropping all contents in BookkeeperDB at location [%s] ...if present", bookkeeperDBPath)
	return RemoveContents(bookkeeperDBPath)
}

func dropHistoryDB(rootFSPath string) error {
	historyDBPath := HistoryDBPath(rootFSPath)
	logger.Infof("Dropping all contents under in HistoryDB at location [%s] ...if present", historyDBPath)
	return RemoveContents(historyDBPath)
}

// RemoveContents removes all the files and subdirs under the specified directory.
// It returns nil if the specified directory does not exist.
func RemoveContents(dir string) error {
	contents, err := ioutil.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return errors.Wrapf(err, "error reading directory %s", dir)
	}

	for _, c := range contents {
		if err = os.RemoveAll(filepath.Join(dir, c.Name())); err != nil {
			return errors.Wrapf(err, "error removing %s under directory %s", c.Name(), dir)
		}
	}
	return syncDir(dir)
}
