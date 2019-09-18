/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	configtxtest "github.com/hyperledger/fabric/common/configtx/test"
	"github.com/stretchr/testify/require"
)

func TestPauseAndResume(t *testing.T) {
	conf, cleanup := testConfig(t)
	conf.HistoryDBConfig.Enabled = false
	defer cleanup()
	provider := testutilNewProvider(conf, t)

	numLedgers := 10
	activeLedgerIDs, err := provider.List()
	require.NoError(t, err)
	require.Len(t, activeLedgerIDs, 0)
	genesisBlocks := make([]*common.Block, numLedgers)
	for i := 0; i < numLedgers; i++ {
		genesisBlock, _ := configtxtest.MakeGenesisBlock(constructTestLedgerID(i))
		genesisBlocks[i] = genesisBlock
		provider.Create(genesisBlock)
	}
	activeLedgerIDs, err = provider.List()
	require.NoError(t, err)
	require.Len(t, activeLedgerIDs, numLedgers)
	provider.Close()

	// pause channels
	pausedLedgers := []int{1, 3, 5}
	for _, i := range pausedLedgers {
		err = PauseChannel(conf.RootFSPath, constructTestLedgerID(i))
		require.NoError(t, err)
	}
	// pause again should not fail
	err = PauseChannel(conf.RootFSPath, constructTestLedgerID(1))
	require.NoError(t, err)
	// verify ledger status after pause
	provider = testutilNewProvider(conf, t)
	assertLedgerStatus(t, provider, genesisBlocks, numLedgers, pausedLedgers)
	provider.Close()

	// resume channels
	resumedLedgers := []int{1, 5}
	for _, i := range resumedLedgers {
		err = ResumeChannel(conf.RootFSPath, constructTestLedgerID(i))
		require.NoError(t, err)
	}
	// resume again should not fail
	err = ResumeChannel(conf.RootFSPath, constructTestLedgerID(1))
	require.NoError(t, err)
	// verify ledger status after resume
	pausedLedgersAfterResume := []int{3}
	provider = testutilNewProvider(conf, t)
	defer provider.Close()
	assertLedgerStatus(t, provider, genesisBlocks, numLedgers, pausedLedgersAfterResume)

	// open paused channel should fail
	_, err = provider.Open(constructTestLedgerID(3))
	require.Equal(t, ErrInactiveLedger, err)
}

func TestPauseAndResumeErrors(t *testing.T) {
	conf, cleanup := testConfig(t)
	conf.HistoryDBConfig.Enabled = false
	defer cleanup()
	provider := testutilNewProvider(conf, t)

	ledgerID := constructTestLedgerID(0)
	genesisBlock, _ := configtxtest.MakeGenesisBlock(ledgerID)
	provider.Create(genesisBlock)
	// purposely set an invalid metatdata
	provider.idStore.db.Put(provider.idStore.encodeLedgerKey(ledgerID, metadataKeyPrefix), []byte("invalid"), true)

	// fail if provider is open (e.g., peer is up running)
	err := PauseChannel(conf.RootFSPath, constructTestLedgerID(0))
	require.Error(t, err, "as another peer node command is executing, wait for that command to complete its execution or terminate it before retrying")

	err = ResumeChannel(conf.RootFSPath, constructTestLedgerID(0))
	require.Error(t, err, "as another peer node command is executing, wait for that command to complete its execution or terminate it before retrying")

	provider.Close()

	// fail if ledgerID does not exists
	err = PauseChannel(conf.RootFSPath, "dummy")
	require.Error(t, err, "LedgerID does not exist")

	err = ResumeChannel(conf.RootFSPath, "dummy")
	require.Error(t, err, "LedgerID does not exist")

	// error if metadata cannot be unmarshaled
	err = PauseChannel(conf.RootFSPath, ledgerID)
	require.EqualError(t, err, "error unmarshalling ledger metadata: unexpected EOF")

	err = ResumeChannel(conf.RootFSPath, ledgerID)
	require.EqualError(t, err, "error unmarshalling ledger metadata: unexpected EOF")
}

// verify status for paused ledgers and non-paused ledgers
func assertLedgerStatus(t *testing.T, provider *Provider, genesisBlocks []*common.Block, numLedgers int, pausedLedgers []int) {
	s := provider.idStore

	activeLedgerIDs, err := provider.List()
	require.NoError(t, err)
	require.Len(t, activeLedgerIDs, numLedgers-len(pausedLedgers))
	for i := 0; i < numLedgers; i++ {
		if !contains(pausedLedgers, i) {
			require.Contains(t, activeLedgerIDs, constructTestLedgerID(i))
		}
	}

	for i := 0; i < numLedgers; i++ {
		active, exists, err := s.ledgerIDActive(constructTestLedgerID(i))
		require.NoError(t, err)
		if !contains(pausedLedgers, i) {
			require.True(t, active)
			require.True(t, exists)
		} else {
			require.False(t, active)
			require.True(t, exists)
		}

		// every channel (paused or non-paused) should have an entry for genesis block
		gbBytes, err := s.db.Get(s.encodeLedgerKey(constructTestLedgerID(i), ledgerKeyPrefix))
		require.NoError(t, err)
		gb := &common.Block{}
		require.NoError(t, proto.Unmarshal(gbBytes, gb))
		require.True(t, proto.Equal(gb, genesisBlocks[i]), "proto messages are not equal")

	}
}

func contains(slice []int, val int) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}
