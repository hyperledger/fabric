/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	"testing"

	configtxtest "github.com/hyperledger/fabric/common/configtx/test"
	"github.com/hyperledger/fabric/core/ledger/mock"
	"github.com/stretchr/testify/require"
)

func TestUnjoinChannel(t *testing.T) {
	conf := testConfig(t)
	conf.HistoryDBConfig.Enabled = true

	ledgerID := "ledger_unjoin"

	provider := testutilNewProvider(conf, t, &mock.DeployedChaincodeInfoProvider{})
	activeLedgerIDs, err := provider.List()
	require.NoError(t, err)
	require.Len(t, activeLedgerIDs, 0)

	genesisBlock, err := configtxtest.MakeGenesisBlock(ledgerID)
	require.NoError(t, err)
	_, err = provider.CreateFromGenesisBlock(genesisBlock)
	require.NoError(t, err)

	activeLedgerIDs, err = provider.List()
	require.NoError(t, err)
	require.Len(t, activeLedgerIDs, 1)
	require.Contains(t, activeLedgerIDs, ledgerID)
	provider.Close()

	// Unjoin the channel from the peer
	err = UnjoinChannel(conf, ledgerID)
	require.NoError(t, err)

	// channel should no longer be present in the channel list
	provider = testutilNewProvider(conf, t, &mock.DeployedChaincodeInfoProvider{})
	defer provider.Close()

	activeLedgerIDs, err = provider.List()
	require.NoError(t, err)
	require.Len(t, activeLedgerIDs, 0)
	require.NotContains(t, activeLedgerIDs, ledgerID)

	// check underlying databases have been removed
	verifyLedgerDoesNotExist(t, provider, ledgerID)
}

// Unjoining an unjoined channel is an error.
func TestUnjoinUnjoinedChannelErrors(t *testing.T) {
	conf := testConfig(t)
	conf.HistoryDBConfig.Enabled = false

	ledgerID := "ledger_unjoin_unjoined"

	provider := testutilNewProvider(conf, t, &mock.DeployedChaincodeInfoProvider{})
	genesisBlock, err := configtxtest.MakeGenesisBlock(ledgerID)
	require.NoError(t, err)
	_, err = provider.CreateFromGenesisBlock(genesisBlock)
	require.NoError(t, err)
	provider.Close()

	// unjoin the channel
	require.NoError(t, UnjoinChannel(conf, ledgerID))

	provider = testutilNewProvider(conf, t, &mock.DeployedChaincodeInfoProvider{})
	activeLedgerIDs, err := provider.List()
	require.NoError(t, err)
	require.Len(t, activeLedgerIDs, 0)
	provider.Close()

	// unjoining an unjoined channel is an error.
	require.EqualError(t, UnjoinChannel(conf, ledgerID),
		"unjoin channel [ledger_unjoin_unjoined]: cannot update ledger status, ledger [ledger_unjoin_unjoined] does not exist")
}

func TestUnjoinWithRunningPeerErrors(t *testing.T) {
	conf := testConfig(t)
	conf.HistoryDBConfig.Enabled = false

	provider := testutilNewProvider(conf, t, &mock.DeployedChaincodeInfoProvider{})
	defer provider.Close()

	ledgerID := constructTestLedgerID(1)
	genesisBlock, _ := configtxtest.MakeGenesisBlock(ledgerID)
	_, err := provider.CreateFromGenesisBlock(genesisBlock)
	require.NoError(t, err)

	// Fail when provider is open (e.g. peer is running)
	require.ErrorContains(t, UnjoinChannel(conf, ledgerID),
		"as another peer node command is executing, wait for that command to complete its execution or terminate it before retrying: lock is already acquired on file")
}

func TestUnjoinWithMissingChannelErrors(t *testing.T) {
	conf := testConfig(t)
	conf.HistoryDBConfig.Enabled = false

	// fail if channel does not exist
	require.EqualError(t, UnjoinChannel(conf, "__invalid_channel"),
		"unjoin channel [__invalid_channel]: cannot update ledger status, ledger [__invalid_channel] does not exist")
}

func TestUnjoinChannelWithInvalidMetadataErrors(t *testing.T) {
	conf := testConfig(t)
	conf.HistoryDBConfig.Enabled = false

	provider := testutilNewProvider(conf, t, &mock.DeployedChaincodeInfoProvider{})

	ledgerID := constructTestLedgerID(99)
	genesisBlock, _ := configtxtest.MakeGenesisBlock(ledgerID)
	_, err := provider.CreateFromGenesisBlock(genesisBlock)
	require.NoError(t, err)

	// purposely set an invalid metatdata
	require.NoError(t, provider.idStore.db.Put(metadataKey(ledgerID), []byte("invalid"), true))
	provider.Close()

	// fail if metadata can not be unmarshaled
	require.ErrorContains(t, UnjoinChannel(conf, ledgerID),
		"unjoin channel [ledger_000099]: error unmarshalling ledger metadata")
}
