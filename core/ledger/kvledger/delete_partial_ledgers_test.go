/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	"testing"

	"github.com/hyperledger/fabric/core/ledger/kvledger/msgs"
	"github.com/hyperledger/fabric/core/ledger/mock"
	"github.com/stretchr/testify/require"
)

func TestDeleteUnderDeletionLedger(t *testing.T) {
	conf := testConfig(t)
	conf.HistoryDBConfig.Enabled = true

	provider := testutilNewProvider(conf, t, &mock.DeployedChaincodeInfoProvider{})
	defer provider.Close()

	// Set up a ledger and mark it as "under deletion."  This emulates the
	// state when a peer was in the process of unjoining a channel and the
	// unjoin operation crashed mid delete.
	ledgerID := constructTestLedger(t, provider, 0)
	verifyLedgerIDExists(t, provider, ledgerID, msgs.Status_ACTIVE)

	// doom the newly created ledger
	require.NoError(t, provider.idStore.updateLedgerStatus(ledgerID, msgs.Status_UNDER_DELETION))
	verifyLedgerIDExists(t, provider, ledgerID, msgs.Status_UNDER_DELETION)

	// explicitly call the delete on the provider.
	provider.deletePartialLedgers()

	// goodbye, ledger.
	verifyLedgerDoesNotExist(t, provider, ledgerID)
}

func TestDeletePartialLedgers(t *testing.T) {
	conf := testConfig(t)
	conf.HistoryDBConfig.Enabled = true

	provider := testutilNewProvider(conf, t, &mock.DeployedChaincodeInfoProvider{})
	defer provider.Close()

	targetStatus := []msgs.Status{
		msgs.Status_ACTIVE,
		msgs.Status_UNDER_DELETION,
		msgs.Status_ACTIVE,
		msgs.Status_UNDER_DELETION,
		msgs.Status_UNDER_CONSTRUCTION,
	}

	constructPartialLedgers(t, provider, targetStatus)

	// delete the under deletion ledgers
	err := provider.deletePartialLedgers()
	require.NoError(t, err)

	verifyPartialLedgers(t, provider, targetStatus)
}

func TestNewProviderDeletesPartialLedgers(t *testing.T) {
	conf := testConfig(t)
	conf.HistoryDBConfig.Enabled = true

	provider := testutilNewProvider(conf, t, &mock.DeployedChaincodeInfoProvider{})

	targetStatus := []msgs.Status{
		msgs.Status_ACTIVE,
		msgs.Status_UNDER_DELETION,
		msgs.Status_UNDER_CONSTRUCTION,
		msgs.Status_ACTIVE,
		msgs.Status_UNDER_DELETION,
	}

	constructPartialLedgers(t, provider, targetStatus)

	// Close and re-open the provider.  The initialization should have scrubbed the partial ledgers.
	provider.Close()
	provider = testutilNewProvider(conf, t, &mock.DeployedChaincodeInfoProvider{})
	defer provider.Close()

	verifyPartialLedgers(t, provider, targetStatus)
}

// Construct a series of test ledgers, each with a target status.
func constructPartialLedgers(t *testing.T, provider *Provider, targetStatus []msgs.Status) {
	for i := 0; i < len(targetStatus); i++ {
		ledgerID := constructTestLedger(t, provider, i)
		require.NoError(t, provider.idStore.updateLedgerStatus(ledgerID, targetStatus[i]))
		verifyLedgerIDExists(t, provider, ledgerID, targetStatus[i])
	}
}

// Check that all UNDER_CONSTRUCTION and UNDER_DELETION ledgers were scrubbed.
func verifyPartialLedgers(t *testing.T, provider *Provider, targetStatus []msgs.Status) {
	// Also double-check that deleted ledgers do not appear in the provider listing.
	activeLedgers, err := provider.List()
	require.NoError(t, err)

	for i := 0; i < len(targetStatus); i++ {
		ledgerID := constructTestLedgerID(i)
		if targetStatus[i] == msgs.Status_UNDER_CONSTRUCTION || targetStatus[i] == msgs.Status_UNDER_DELETION {
			verifyLedgerDoesNotExist(t, provider, ledgerID)
			require.NotContains(t, ledgerID, activeLedgers)
		} else {
			verifyLedgerIDExists(t, provider, ledgerID, targetStatus[i])
			require.Contains(t, activeLedgers, ledgerID)
		}
	}
}
