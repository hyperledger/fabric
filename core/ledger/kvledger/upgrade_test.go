/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	"testing"

	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/common/ledger/util/leveldbhelper"
	"github.com/stretchr/testify/require"
)

func TestUpgradeDataFormat(t *testing.T) {
	conf, cleanup := testConfig(t)
	conf.HistoryDBConfig.Enabled = false
	defer cleanup()
	provider := testutilNewProvider(conf, t)

	// upgrade should fail when provider is still open
	err := UpgradeDataFormat(conf.RootFSPath)
	require.Error(t, err, "as another peer node command is executing, wait for that command to complete its execution or terminate it before retrying")

	provider.Close()
	err = UpgradeDataFormat(conf.RootFSPath)
	require.NoError(t, err)
}

func TestUpgradeIDStoreFormat(t *testing.T) {
	conf, cleanup := testConfig(t)
	conf.HistoryDBConfig.Enabled = false
	defer cleanup()

	// upgrade v11 ledger data
	testutil.CopyDir("tests/testdata/v11/sample_ledgers/ledgersData", conf.RootFSPath, true)
	v11LedgerIDs := getLedgerIDs(t, conf.RootFSPath)
	require.NoError(t, UpgradeIDStoreFormat(conf.RootFSPath))

	// verify formatVersion and active ledger IDs are set correctly
	idStore, err := openIDStore(LedgerProviderPath(conf.RootFSPath))
	defer idStore.close()
	require.NoError(t, err)

	formatVersion, err := idStore.db.Get(formatKey)
	require.NoError(t, err)
	require.Equal(t, []byte(idStoreFormatVersion), formatVersion)

	metadataLedgerIDs, err := idStore.getActiveLedgerIDs()
	require.NoError(t, err)
	require.ElementsMatch(t, v11LedgerIDs, metadataLedgerIDs)
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
	expectedErr := &leveldbhelper.ErrFormatVersionMismatch{ExpectedFormatVersion: "", DataFormatVersion: "x.0", DBPath: LedgerProviderPath(conf.RootFSPath)}
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
