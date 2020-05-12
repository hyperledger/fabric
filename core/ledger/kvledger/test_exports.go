/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	"testing"

	"github.com/hyperledger/fabric/common/ledger/util/leveldbhelper"
	"github.com/stretchr/testify/require"
)

// UpgradeIDStoreFormat updates ledger idStore to current format
func UpgradeIDStoreFormat(t *testing.T, rootFSPath string) {
	dbPath := LedgerProviderPath(rootFSPath)
	db := leveldbhelper.CreateDB(&leveldbhelper.Conf{DBPath: dbPath})
	db.Open()
	defer db.Close()

	idStore := &idStore{db, dbPath}
	require.NoError(t, idStore.upgradeFormat())
}
