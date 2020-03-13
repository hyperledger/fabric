/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/hyperledger/fabric/common/ledger/dataformat"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/common/ledger/util/leveldbhelper"
	"github.com/stretchr/testify/require"
)

func TestUpgradeDBs(t *testing.T) {
	conf, cleanup := testConfig(t)
	defer cleanup()
	provider := testutilNewProvider(conf, t)

	// upgrade should fail when provider is still open
	err := UpgradeDBs(conf.RootFSPath)
	require.Error(t, err, "as another peer node command is executing, wait for that command to complete its execution or terminate it before retrying")
	provider.Close()

	// load v11 ledger data for upgrade
	rootFSPath := conf.RootFSPath
	require.NoError(t, testutil.Unzip("tests/testdata/v11/sample_ledgers/ledgersData.zip", rootFSPath, false))
	v11LedgerIDs := getLedgerIDs(t, rootFSPath)
	require.NoError(t, UpgradeIDStoreFormat(rootFSPath))

	err = UpgradeDBs(rootFSPath)
	require.NoError(t, err)

	// verify idStore has formatKey and metadata entries
	idStore, err := openIDStore(LedgerProviderPath(conf.RootFSPath))
	require.NoError(t, err)
	formatVersion, err := idStore.db.Get(formatKey)
	require.NoError(t, err)
	require.Equal(t, []byte(dataformat.Version20), formatVersion)
	metadataLedgerIDs, err := idStore.getActiveLedgerIDs()
	require.NoError(t, err)
	require.ElementsMatch(t, v11LedgerIDs, metadataLedgerIDs)
	idStore.close()

	// verify blockstoreIndex, configHistory, history, state, bookkeeper dbs are deleted
	_, err = os.Stat(filepath.Join(BlockStorePath(rootFSPath), "index"))
	require.Equal(t, os.IsNotExist(err), true)
	_, err = os.Stat(ConfigHistoryDBPath(rootFSPath))
	require.Equal(t, os.IsNotExist(err), true)
	_, err = os.Stat(HistoryDBPath(rootFSPath))
	require.Equal(t, os.IsNotExist(err), true)
	_, err = os.Stat(StateDBPath(rootFSPath))
	require.Equal(t, os.IsNotExist(err), true)
	_, err = os.Stat(BookkeeperDBPath(rootFSPath))
	require.Equal(t, os.IsNotExist(err), true)

	// upgrade again should be successful
	err = UpgradeDBs(rootFSPath)
	require.NoError(t, err)
}

func TestUpgradeIDStoreWrongFormat(t *testing.T) {
	conf, cleanup := testConfig(t)
	conf.HistoryDBConfig.Enabled = false
	defer cleanup()
	provider := testutilNewProvider(conf, t)

	// change format to a wrong value
	err := provider.idStore.db.Put(formatKey, []byte("x.0"), true)
	provider.Close()
	require.NoError(t, err)

	err = UpgradeIDStoreFormat(conf.RootFSPath)
	expectedErr := &dataformat.ErrVersionMismatch{
		ExpectedVersion: "",
		Version:         "x.0",
		DBInfo:          fmt.Sprintf("leveldb for channel-IDs at [%s]", LedgerProviderPath(conf.RootFSPath)),
	}
	require.EqualError(t, err, expectedErr.Error())
}

// getLedgerIDs returns ledger ids using ledgerKeyPrefix (available in both old format and new format)
func getLedgerIDs(t *testing.T, rootFSPath string) []string {
	dbPath := LedgerProviderPath(rootFSPath)
	db := leveldbhelper.CreateDB(&leveldbhelper.Conf{DBPath: dbPath})
	db.Open()
	idStore := &idStore{db, dbPath}
	defer db.Close()
	itr := db.GetIterator(ledgerKeyPrefix, ledgerKeyStop)
	defer itr.Release()
	var ledgerIDs []string
	for itr.Next() {
		require.NoError(t, itr.Error())
		ledgerIDs = append(ledgerIDs, idStore.decodeLedgerID(itr.Key(), ledgerKeyPrefix))
	}
	return ledgerIDs
}
