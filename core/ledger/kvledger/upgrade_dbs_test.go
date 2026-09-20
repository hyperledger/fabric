/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	"fmt"
	"testing"

	"github.com/hyperledger/fabric/common/ledger/dataformat"
	"github.com/hyperledger/fabric/core/ledger/mock"
	"github.com/hyperledger/fabric/internal/fileutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestUpgradeWrongFormat(t *testing.T) {
	conf := testConfig(t)
	conf.HistoryDBConfig.Enabled = false
	provider := testutilNewProvider(conf, t, &mock.DeployedChaincodeInfoProvider{})

	// change format to a wrong value to test upgradeFormat error path
	err := provider.idStore.db.Put(formatKey, []byte("x.0"), true)
	provider.Close()
	require.NoError(t, err)

	stateDBPath := StateDBPath(conf.RootFSPath)
	isEmpty, err := fileutil.DirEmpty(stateDBPath)
	require.NoError(t, err)
	require.False(t, isEmpty)

	err = UpgradeDBs(conf)
	expectedErr := errors.WithMessage(
		&dataformat.ErrFormatMismatch{
			ExpectedFormat: dataformat.PreviousFormat,
			Format:         "x.0",
			DBInfo:         fmt.Sprintf("leveldb for channel-IDs at [%s]", LedgerProviderPath(conf.RootFSPath)),
		},
		"error while checking whether upgrade is required",
	)
	require.EqualError(t, err, expectedErr.Error())
	isEmpty, err = fileutil.DirEmpty(stateDBPath)
	require.NoError(t, err)
	require.False(t, isEmpty)
}

func TestUpgradeAlreadyUptodateFormat(t *testing.T) {
	conf := testConfig(t)
	conf.HistoryDBConfig.Enabled = false
	provider := testutilNewProvider(conf, t, &mock.DeployedChaincodeInfoProvider{})
	provider.Close()

	stateDBPath := StateDBPath(conf.RootFSPath)
	isEmpty, err := fileutil.DirEmpty(stateDBPath)
	require.NoError(t, err)
	require.False(t, isEmpty)

	err = UpgradeDBs(conf)
	require.EqualError(t, err, "the data format is already up to date. No upgrade is required")
	isEmpty, err = fileutil.DirEmpty(stateDBPath)
	require.NoError(t, err)
	require.False(t, isEmpty)
}
