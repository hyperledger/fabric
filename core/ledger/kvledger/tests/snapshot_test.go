/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tests

import (
	"testing"
	"time"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSnapshotGenerationAndBootstrap(t *testing.T) {
	env := newEnvWithInitializer(t,
		&ledgermgmt.Initializer{
			MembershipInfoProvider: &membershipInfoProvider{myOrgMSPID: "org-myOrg"},
		},
	)
	defer env.cleanup()
	env.initLedgerMgmt()
	originalLedger := env.createTestLedgerFromGenesisBlk("ledger1")

	// block-1
	originalLedger.simulateDeployTx("myChaincode",
		[]*collConf{
			{
				name:    "collection-1",
				btl:     0,
				members: []string{"org-myOrg"},
			},
			{
				name:    "collection-2",
				btl:     2,
				members: []string{"org-another-Org"},
			},
		},
	)
	originalLedger.cutBlockAndCommitLegacy()

	// block-2
	txAndPvtdataBlk2 := originalLedger.simulateDataTx("txid-1", func(s *simulator) {
		s.setState("myChaincode", "key-1", "value-1")
		s.setPvtdata("myChaincode", "collection-1", "private-key-1", "private-value-1")
	})
	originalLedger.cutBlockAndCommitLegacy()

	// block-3
	txAndPvtdataBlk3 := originalLedger.simulateDataTx("txid-2", func(s *simulator) {
		s.setState("myChaincode", "key-2", "value-2")
		s.setPvtdata("myChaincode", "collection-2", "private-key-2", "private-value-2")
	})
	originalLedger.cutBlockAndCommitLegacy()

	// generate snapshot and bootstrap another ledger
	snapshotDir := originalLedger.generateSnapshot()
	anotherEnv := newEnvWithInitializer(t,
		&ledgermgmt.Initializer{
			MembershipInfoProvider: &membershipInfoProvider{myOrgMSPID: "org-myOrg"},
		},
	)

	defer anotherEnv.cleanup()
	anotherEnv.initLedgerMgmt()
	testLedger := anotherEnv.createTestLedgerFromSnapshot(snapshotDir)

	// verify basic queries on the bootstapped ledger
	originalBCInfo, err := originalLedger.lgr.GetBlockchainInfo()
	require.NoError(t, err)
	testLedger.verifyBlockchainInfo(
		&common.BlockchainInfo{
			Height:            originalBCInfo.Height,
			CurrentBlockHash:  originalBCInfo.CurrentBlockHash,
			PreviousBlockHash: originalBCInfo.PreviousBlockHash,
			BootstrappingSnapshotInfo: &common.BootstrappingSnapshotInfo{
				LastBlockInSnapshot: originalBCInfo.Height - 1,
			},
		},
	)

	lgr := testLedger.lgr
	_, err = lgr.GetBlockByNumber(3)
	require.EqualError(t, err, "cannot serve block [3]. The ledger is bootstrapped from a snapshot. First available block = [4]")

	_, _, err = lgr.GetTxValidationCodeByTxID("txid-1")
	require.EqualError(t, err, "details for the TXID [txid-1] not available. Ledger bootstrapped from a snapshot. First available block = [4]")

	testLedger.verifyTXIDExists("txid-1", "txid-2")

	testLedger.verifyPubState("myChaincode", "key-1", "value-1")
	testLedger.verifyPvtdataHashState("myChaincode", "collection-1", "private-key-1", util.ComputeHash([]byte("private-value-1")))

	testLedger.simulateDataTx("", func(s *simulator) {
		_, err := s.GetPrivateData("myChaincode", "collection-1", "private-key-1")
		require.EqualError(t, err, "private data matching public hash version is not available. Public hash version = {BlockNum: 2, TxNum: 0}, Private data version = <nil>")
	})

	// verify that the relevant missing data entries has been created in the pvtdata store
	// missing data reported should only be for collection-1, as ledger is not eligible for collection-2
	expectedMissingPvtData := make(ledger.MissingPvtDataInfo)
	expectedMissingPvtData.Add(2, 0, "myChaincode", "collection-1")
	testLedger.verifyMissingPvtDataSameAs(10, expectedMissingPvtData)

	// Block-4
	// simulate chaincode upgrade to make this ledger eligible for collection-2
	// This helps verify (via a follow up query) that the missing data entries for ineligible collections were populated
	testLedger.simulateUpgradeTx("myChaincode",
		[]*collConf{
			{
				name:    "collection-1",
				btl:     0,
				members: []string{"org-myOrg"},
			},
			{
				name:    "collection-2",
				btl:     2,
				members: []string{"org-myOrg", "org-another-Org"},
			},
		},
	)
	testLedger.cutBlockAndCommitLegacy()

	// Now, missing data reported should include for both collection-1 and collection-2
	expectedMissingPvtDataAfterUpgrade := make(ledger.MissingPvtDataInfo)
	expectedMissingPvtDataAfterUpgrade.Add(2, 0, "myChaincode", "collection-1")
	expectedMissingPvtDataAfterUpgrade.Add(3, 0, "myChaincode", "collection-2")
	require.Eventually(
		t,
		func() bool {
			missingDataTracker, err := testLedger.lgr.GetMissingPvtDataTracker()
			require.NoError(t, err)
			missingPvtData, err := missingDataTracker.GetMissingPvtDataInfoForMostRecentBlocks(10)
			require.NoError(t, err)
			return assert.ObjectsAreEqual(expectedMissingPvtDataAfterUpgrade, missingPvtData)
		},
		time.Second,
		time.Millisecond,
	)

	// try committing tampered data via reconciler
	// This verifies that the boot KVHahses were populated and are used as expected
	temperedPvtDataBlk2 := testLedger.simulateDataTx("", func(s *simulator) {
		s.setPvtdata("myChaincode", "collection-1", "private-key-1", "tampered_private-value-1")
	})
	testLedger.discardSimulation()

	temperedPvtDataBlk3 := testLedger.simulateDataTx("", func(s *simulator) {
		s.setPvtdata("myChaincode", "collection-2", "private-key-2", "tampered_private-value-2")
	})
	testLedger.discardSimulation()

	_, err = testLedger.lgr.CommitPvtDataOfOldBlocks(
		[]*ledger.ReconciledPvtdata{
			{
				BlockNum: 2,
				WriteSets: ledger.TxPvtDataMap{
					0: &ledger.TxPvtData{
						SeqInBlock: 0,
						WriteSet:   temperedPvtDataBlk2.Pvtws,
					},
				},
			},
			{
				BlockNum: 3,
				WriteSets: ledger.TxPvtDataMap{
					0: &ledger.TxPvtData{
						SeqInBlock: 0,
						WriteSet:   temperedPvtDataBlk3.Pvtws,
					},
				},
			},
		}, nil,
	)
	require.NoError(t, err)
	testLedger.verifyMissingPvtDataSameAs(10, expectedMissingPvtDataAfterUpgrade)

	// try committing legitimate pvtdata via reconciler
	_, err = testLedger.lgr.CommitPvtDataOfOldBlocks(
		[]*ledger.ReconciledPvtdata{
			{
				BlockNum: 2,
				WriteSets: ledger.TxPvtDataMap{
					0: &ledger.TxPvtData{
						SeqInBlock: 0,
						WriteSet:   txAndPvtdataBlk2.Pvtws,
					},
				},
			},
			{
				BlockNum: 3,
				WriteSets: ledger.TxPvtDataMap{
					0: &ledger.TxPvtData{
						SeqInBlock: 0,
						WriteSet:   txAndPvtdataBlk3.Pvtws,
					},
				},
			},
		}, nil,
	)

	require.NoError(t, err)
	testLedger.verifyMissingPvtDataSameAs(10, ledger.MissingPvtDataInfo{})
	testLedger.verifyInPvtdataStore(2, nil,
		[]*ledger.TxPvtData{
			{
				SeqInBlock: 0,
				WriteSet:   txAndPvtdataBlk2.Pvtws,
			},
		},
	)
	testLedger.verifyInPvtdataStore(3, nil,
		[]*ledger.TxPvtData{
			{
				SeqInBlock: 0,
				WriteSet:   txAndPvtdataBlk3.Pvtws,
			},
		},
	)
	testLedger.verifyPvtState("myChaincode", "collection-1", "private-key-1", "private-value-1")
	testLedger.verifyPvtState("myChaincode", "collection-2", "private-key-2", "private-value-2")

	// commit two random blocks and pvtdata committed in block-3 for collection-2 should expire
	// this verifyies that the expiry entries were created during bootstrap
	for i := 0; i < 2; i++ {
		testLedger.simulateDataTx("", func(s *simulator) {
			s.setState("myChaincode", "random-key", "random-val")
		})
		testLedger.cutBlockAndCommitLegacy()
	}
	testLedger.verifyInPvtdataStore(3, nil, nil)
	testLedger.verifyPvtState("myChaincode", "collection-2", "private-key-2", "")
}
