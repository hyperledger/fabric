/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	"github.com/hyperledger/fabric/common/ledger/blkstorage/fsblkstorage"
	"github.com/hyperledger/fabric/common/ledger/util/leveldbhelper"
	"github.com/pkg/errors"
)

// RebuildDBs drops existing ledger databases.
// Dropped database will be rebuilt upon server restart
func RebuildDBs(rootFSPath string) error {
	fileLockPath := fileLockPath(rootFSPath)
	fileLock := leveldbhelper.NewFileLock(fileLockPath)
	if err := fileLock.Lock(); err != nil {
		return errors.Wrap(err, "as another peer node command is executing,"+
			" wait for that command to complete its execution or terminate it before retrying")
	}
	defer fileLock.Unlock()

	if err := dropDBs(rootFSPath); err != nil {
		return err
	}

	blockstorePath := BlockStorePath(rootFSPath)
	return fsblkstorage.DeleteBlockStoreIndex(blockstorePath)
}
