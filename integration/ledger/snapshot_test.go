/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ledger

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	cb "github.com/hyperledger/fabric-protos-go/common"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/integration/chaincode/kvexecutor"
	"github.com/hyperledger/fabric/integration/channelparticipation"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	"github.com/hyperledger/fabric/integration/nwo/runner"
	"github.com/hyperledger/fabric/integration/pvtdata/marblechaincodeutil"
	"github.com/hyperledger/fabric/protoutil"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
)

const testchannelID = "testchannel"

var _ = Describe("Snapshot Generation and Bootstrap", func() {
	var (
		setup                 *setup
		helper                *marblesTestHelper
		couchProcess          []ifrit.Process
		legacyChaincode       nwo.Chaincode
		newlifecycleChaincode nwo.Chaincode
	)

	BeforeEach(func() {
		By("initializing and starting the network")
		setup = initAndStartFourOrgsNetwork()

		helper = &marblesTestHelper{
			networkHelper: &networkHelper{
				Network:   setup.network,
				orderer:   setup.orderer,
				peers:     setup.peers,
				testDir:   setup.testDir,
				channelID: setup.channelID,
			},
		}
	})

	AfterEach(func() {
		setup.cleanup()
		for _, proc := range couchProcess {
			proc.Signal(syscall.SIGTERM)
			Eventually(proc.Wait(), setup.network.EventuallyTimeout).Should(Receive())
		}
		os.RemoveAll(setup.network.RootDir)
	})

	When("chaincode has no private data collections", func() {
		BeforeEach(func() {
			legacyChaincode = nwo.Chaincode{
				Name:        "marbles",
				Version:     "0.0",
				Path:        chaincodePathWithIndex,
				Ctor:        `{"Args":[]}`,
				Policy:      `OR ('Org1MSP.member','Org2MSP.member','Org3MSP.member','Org4MSP.member')`,
				PackageFile: filepath.Join(setup.testDir, "marbles_legacy.tar.gz"),
			}

			newlifecycleChaincode = nwo.Chaincode{
				Name:            "marbles",
				Version:         "0.0",
				Path:            components.Build(chaincodePathWithIndex),
				Lang:            "binary",
				CodeFiles:       filesWithIndex,
				PackageFile:     filepath.Join(setup.testDir, "marbles.tar.gz"),
				SignaturePolicy: `OR ('Org1MSP.member','Org2MSP.member','Org3MSP.member','Org4MSP.member')`,
				Sequence:        "1",
				Label:           "marbles",
			}

			By("deploying legacy chaincode on initial peers (stateleveldb)")
			nwo.DeployChaincodeLegacy(setup.network, testchannelID, setup.orderer, legacyChaincode)
		})

		// Below test does the following when peers are using either leveldb or couchdb.
		// Note that we do not support a channel with mixed DBs. However, for testing,
		// it would be fine to use mixed DBs to test with both couchdb and leveldb
		// - create snapshots on the 2 peers and verify they are same
		// - bootstrap a peer (couchdb) in an existing org from the snapshot
		// - bootstrap a peer (leveldb) in a new org from the snapshot
		// - verify couchdb index exists
		// - verify chaincode invocation, history, qscc, channel config update
		// - upgrade to new lifecycle chaincode
		// - create a new snapshot again from a peer (couchdb) bootstrapped from a snapshot
		// - bootstrap peers (couchdb) in existing org and new org from the new snapshot
		// - verify couchdb index exists
		// - verify chaincode invocation, history, qscc
		// - verify chaincode upgrade and new chaincode install on all peers
		It("generates snapshot and bootstraps from snapshots", func() {
			org1peer0 := setup.network.Peer("Org1", "peer0")
			org2peer0 := setup.network.Peer("Org2", "peer0")

			By("invoking marbles chaincode")
			testKey := "marble-0"
			helper.invokeMarblesChaincode(legacyChaincode.Name, org1peer0, "initMarble", "marble-0", "blue", "35", "tom")
			helper.invokeMarblesChaincode(legacyChaincode.Name, org1peer0, "initMarble", "marble-1", "red", "100", "tom")

			By("getting an existing transaction from a block before snapshot is generated")
			txenvBeforeSnapshot, txidBeforeSnapshot := getTxFromLastBlock(setup.network, org1peer0)

			// verify snapshot commands with different parameters")
			blockNum := nwo.GetLedgerHeight(setup.network, org1peer0, testchannelID) - 1
			verifySnapshotRequestCmds(setup.network, org1peer0, testchannelID, blockNum)

			// test 1: generate snapshots on 2 peers for the same blockNum and verify they are same
			_, snapshotDir := generateAndCompareSnapshots(setup.network, org1peer0, org2peer0, blockNum)

			// test 2: bootstrap a peer in an existing org from snapshot and verify
			By("starting new peer org2peer1 in existing org2 (couchdb)")
			org2peer1, couchProc := startPeer(setup, "Org2", "peer1", testchannelID, true)
			couchProcess = append(couchProcess, couchProc)

			By("installing legacy chaincode on new peer org2peer1")
			nwo.InstallChaincodeLegacy(setup.network, legacyChaincode, org2peer1)

			By("joining new peer org2peer1 to the channel")
			joinBySnapshot(setup.network, setup.orderer, org2peer1, testchannelID, snapshotDir, blockNum)

			By("verifying index created on org2peer1")
			verifySizeIndexExists(setup.network, testchannelID, setup.orderer, org2peer1, "marbles")

			By("invoking marbles chaincode on bootstrapped peer org2peer1")
			helper.invokeMarblesChaincode(legacyChaincode.Name, org2peer1, "transferMarble", testKey, "newowner2")

			By("verifying history on peer org2peer1")
			expectedHistory := []*marbleHistoryResult{
				{IsDelete: "false", Value: newMarble(testKey, "blue", 35, "newowner2")},
			}
			helper.assertGetHistoryForMarble(legacyChaincode.Name, org2peer1, expectedHistory, testKey)

			verifyQSCC(setup.network, org2peer1, testchannelID, blockNum, txidBeforeSnapshot)

			// test 3: bootstrap a peer in a new org from snapshot and verify
			By("starting a peer Org3.peer0 in new org3 (stateleveldb)")
			org3peer0, _ := startPeer(setup, "Org3", "peer0", testchannelID, false)

			By("installing legacy chaincode on new peer org3peer0")
			nwo.InstallChaincodeLegacy(setup.network, legacyChaincode, org3peer0)

			By("joining new peer org3peer0 to the channel")
			joinBySnapshot(setup.network, setup.orderer, org3peer0, testchannelID, snapshotDir, blockNum)

			By("invoking marbles chaincode on bootstrapped peer org3peer0")
			helper.invokeMarblesChaincode(legacyChaincode.Name, org3peer0, "transferMarble", testKey, "newowner3")

			By("getting an existing transaction from a block after snapshot is generated")
			txenvAfterSnapshot, txidAfterSnapshot := getTxFromLastBlock(setup.network, org1peer0)

			By("verifying history on peer org3peer0")
			expectedHistory = []*marbleHistoryResult{
				{IsDelete: "false", Value: newMarble(testKey, "blue", 35, "newowner3")},
				{IsDelete: "false", Value: newMarble(testKey, "blue", 35, "newowner2")},
			}
			helper.assertGetHistoryForMarble(legacyChaincode.Name, org3peer0, expectedHistory, testKey)
			verifyQSCC(setup.network, org3peer0, testchannelID, blockNum, txidBeforeSnapshot)

			// verify DUPLICATE_TXID error when resubmitting old tx on a peer bootstrapped from snapshot (v1_4 capability)
			By("resubmitting an old transaction committed before snapshot, expecting duplicated txid error")
			err := commitTx(setup.network, setup.orderer, org3peer0, testchannelID, txenvBeforeSnapshot, txidBeforeSnapshot)
			Expect(err.Error()).To(ContainSubstring("transaction invalidated with status (DUPLICATE_TXID)"))
			By("resubmitting an old transaction committed after snapshot, expecting duplicated txid error")
			err = commitTx(setup.network, setup.orderer, org3peer0, testchannelID, txenvAfterSnapshot, txidAfterSnapshot)
			Expect(err.Error()).To(Equal("transaction invalidated with status (DUPLICATE_TXID)"))

			// test 4: upgrade legacy chaincode to new lifecycle
			By("enabling V2_0 capabilities")
			channelPeers := setup.network.PeersWithChannel(testchannelID)
			nwo.EnableCapabilities(setup.network, testchannelID, "Application", "V2_0", setup.orderer, channelPeers...)

			By("upgrading legacy chaincode to new lifecycle chaincode")
			nwo.DeployChaincode(setup.network, testchannelID, setup.orderer, newlifecycleChaincode, channelPeers...)

			By("invoking chaincode after upgraded to new lifecycle chaincode")
			helper.invokeMarblesChaincode(newlifecycleChaincode.Name, org1peer0, "initMarble", "marble-upgrade", "blue", "35", "tom")

			// test 5: generate snapshot again on a peer bootstrapped from a snapshot and upgraded to new lifecycle chaincode
			blockNumForNextSnapshot := nwo.GetLedgerHeight(setup.network, org2peer1, testchannelID)
			By(fmt.Sprintf("generating a snapshot at blockNum %d on org2peer1 that was bootstrapped by a snapshot", blockNumForNextSnapshot))
			submitSnapshotRequest(setup.network, testchannelID, blockNumForNextSnapshot, org2peer1, false, "Snapshot request submitted successfully")

			// invoke chaincode to trigger snapshot generation
			// 1st call should be committed before snapshot generation, 2nd call should be committed after snapshot generation
			helper.invokeMarblesChaincode(newlifecycleChaincode.Name, org2peer1, "transferMarble", testKey, "newowner_beforesnapshot")
			helper.invokeMarblesChaincode(newlifecycleChaincode.Name, org2peer1, "transferMarble", testKey, "newowner_aftersnapshot")

			By("verifying snapshot completed on org2peer1")
			verifyNoPendingSnapshotRequest(setup.network, org2peer1, testchannelID)
			nextSnapshotDir := filepath.Join(setup.network.PeerDir(org2peer1), "filesystem", "snapshots", "completed", testchannelID, strconv.Itoa(blockNumForNextSnapshot))

			By("getting an existing transaction from a block after new snapshot is generated")
			helper.invokeMarblesChaincode(legacyChaincode.Name, org2peer1, "initMarble", "marble-3", "red", "100", "tom")
			txenvAfterSnapshot, txidAfterSnapshot = getTxFromLastBlock(setup.network, org1peer0)

			// test 6: bootstrap a peer in a different org from the new snapshot
			By("starting a peer (org1peer1) in existing org1 (couchdb)")
			org1peer1, couchProc := startPeer(setup, "Org1", "peer1", testchannelID, true)
			couchProcess = append(couchProcess, couchProc)

			By("installing new lifecycle chaincode on peer org1peer1")
			nwo.InstallChaincode(setup.network, newlifecycleChaincode, org1peer1)

			By("joining new peer org1peer1 to the channel")
			joinBySnapshot(setup.network, setup.orderer, org1peer1, testchannelID, nextSnapshotDir, blockNumForNextSnapshot)

			By("verifying index created on org1peer1")
			verifySizeIndexExists(setup.network, testchannelID, setup.orderer, org1peer1, "marbles")

			By("verifying history on peer org1peer1")
			expectedHistory = []*marbleHistoryResult{
				{IsDelete: "false", Value: newMarble(testKey, "blue", 35, "newowner_aftersnapshot")},
			}
			helper.assertGetHistoryForMarble(newlifecycleChaincode.Name, org1peer1, expectedHistory, testKey)

			verifyQSCC(setup.network, org1peer1, testchannelID, blockNumForNextSnapshot, txidBeforeSnapshot)

			// test 7: bootstrap a peer in a new org from the new snapshot
			By("starting a peer (org4peer0) in new org4 (couchdb)")
			org4peer0, couchProc := startPeer(setup, "Org4", "peer0", testchannelID, true)
			couchProcess = append(couchProcess, couchProc)

			By("joining new peer org4peer0 to the channel")
			joinBySnapshot(setup.network, setup.orderer, org4peer0, testchannelID, nextSnapshotDir, blockNumForNextSnapshot)

			By("installing and approving chaincode on new peer org4peer0")
			installAndApproveChaincode(setup.network, setup.orderer, org4peer0, testchannelID, newlifecycleChaincode, []string{"Org1", "Org2", "Org3", "Org4"})

			By("verifying index created on org4peer0")
			verifySizeIndexExists(setup.network, testchannelID, setup.orderer, org2peer1, "marbles")

			By("invoking chaincode on bootstrapped peer org4peer0")
			helper.invokeMarblesChaincode(newlifecycleChaincode.Name, org4peer0, "delete", testKey)

			By("verifying history on peer org4peer0")
			expectedHistory = []*marbleHistoryResult{
				{IsDelete: "true"},
				{IsDelete: "false", Value: newMarble(testKey, "blue", 35, "newowner_aftersnapshot")},
			}
			helper.assertGetHistoryForMarble(newlifecycleChaincode.Name, org4peer0, expectedHistory, testKey)

			verifyQSCC(setup.network, org4peer0, testchannelID, blockNumForNextSnapshot, txidBeforeSnapshot)

			// verify DUPLICATE_TXID error when resubmitting old tx on a peer bootstrapped from snapshot (v_20 capability)
			By("resubmitting an old transaction committed before snapshot, expecting duplicated txid error")
			err = commitTx(setup.network, setup.orderer, org4peer0, testchannelID, txenvBeforeSnapshot, txidBeforeSnapshot)
			Expect(err.Error()).To(ContainSubstring("transaction invalidated with status (DUPLICATE_TXID)"))
			By("resubmitting an old transaction committed after snapshot, expecting duplicated txid error")
			err = commitTx(setup.network, setup.orderer, org4peer0, testchannelID, txenvAfterSnapshot, txidAfterSnapshot)
			Expect(err.Error()).To(Equal("transaction invalidated with status (DUPLICATE_TXID)"))

			// test 8: verify cscc works correctly to get an orderer endpoint from the channel config
			// even if the peer does not have a channel config block when bootstrapped from snapshot
			By("invoking chaincode without passing orderer endpoint on org4peer0")
			invokeWithoutPassingOrdererEndPoint(setup.network, org4peer0, testchannelID, newlifecycleChaincode.Name, "initMarble", "marble-cscctest", "blue", "35", "tom")

			// test 9: verify chaincode upgrade and install after bootstrapping
			By("upgrading chaincode to version 2.0 on all peers after bootstrapping from snapshot")
			newlifecycleChaincode.Version = "2.0"
			newlifecycleChaincode.Sequence = "2"
			nwo.DeployChaincode(setup.network, testchannelID, setup.orderer, newlifecycleChaincode)

			By("deploying a new chaincode on all the peers after bootstrapping from snapshot")
			cc2 := nwo.Chaincode{
				Name:            "kvexecutor",
				Version:         "1.0",
				Path:            components.Build("github.com/hyperledger/fabric/integration/chaincode/kvexecutor/cmd"),
				Lang:            "binary",
				SignaturePolicy: `OR ('Org1MSP.member','Org2MSP.member', 'Org3MSP.member', 'Org4MSP.member')`,
				PackageFile:     filepath.Join(setup.testDir, "kvexecutor20.tar.gz"),
				Label:           "kvexecutor-20",
				Sequence:        "1",
			}
			nwo.DeployChaincode(setup.network, testchannelID, setup.orderer, cc2)

			By("invoking the new chaincode")
			kvdata := []kvexecutor.KVData{
				{Key: "key1", Value: "value1"},
				{Key: "key2", Value: "value2"},
			}
			invokeAndQueryKVExecutorChaincode(setup.network, setup.orderer, testchannelID, cc2, kvdata, setup.network.PeersWithChannel(testchannelID)...)
		})
	})

	When("chaincode has private data collections", func() {
		BeforeEach(func() {
			newlifecycleChaincode = nwo.Chaincode{
				Name:              "marblesp",
				Version:           "1.0",
				Path:              components.Build("github.com/hyperledger/fabric/integration/chaincode/marbles_private/cmd"),
				Lang:              "binary",
				PackageFile:       filepath.Join(setup.testDir, "marbles-pvtdata.tar.gz"),
				Label:             "marbles-private-20",
				SignaturePolicy:   `OR ('Org1MSP.member','Org2MSP.member')`,
				CollectionsConfig: filepath.Join("testdata", "collection_configs", "collections_config1.json"),
				Sequence:          "1",
			}

			// start org3peer0 so that we have majority number of orgs (3 out of 4) to satify the channel config update policy
			org3peer0, _ := startPeer(setup, "Org3", "peer0", testchannelID, false)
			setup.network.JoinChannel(testchannelID, setup.orderer, org3peer0)

			By("ensuring new peer has joined gossip network")
			setup.network.VerifyMembership(setup.network.PeersWithChannel(testchannelID), testchannelID)

			By("enabling V2_0 capabilities")
			channelPeers := setup.network.PeersWithChannel(testchannelID)
			nwo.EnableCapabilities(setup.network, testchannelID, "Application", "V2_0", setup.orderer, channelPeers...)

			By("deploying newlifecycle chaincode on initial peers (leveldb)")
			nwo.DeployChaincode(setup.network, testchannelID, setup.orderer, newlifecycleChaincode)
		})

		// This test verifies the following:
		// bootstrapped peer can pull private data
		// bootstrapped peer can supply private data to other bootstrapped peer
		It("generates snapshot and bootstraps from snapshots", func() {
			org1peer0 := setup.network.Peer("Org1", "peer0")
			org2peer0 := setup.network.Peer("Org2", "peer0")
			channelPeers := setup.network.PeersWithChannel(testchannelID)

			// prepare test data: add and delete marble1, add and transfer marble1
			By("adding marble1")
			marblechaincodeutil.AddMarble(setup.network, setup.orderer, testchannelID, newlifecycleChaincode.Name,
				`{"name":"marble1", "color":"blue", "size":35, "owner":"tom", "price":99}`, org2peer0)
			By("deleting marble1")
			marblechaincodeutil.DeleteMarble(setup.network, setup.orderer, testchannelID, newlifecycleChaincode.Name,
				`{"name":"marble1"}`, org2peer0)

			By("verifying the deletion of marble1")
			marblechaincodeutil.AssertDoesNotExistInCollectionM(setup.network, testchannelID, newlifecycleChaincode.Name, "marble1", channelPeers...)
			marblechaincodeutil.AssertDoesNotExistInCollectionMPD(setup.network, testchannelID, newlifecycleChaincode.Name, "marble1", channelPeers...)

			By("adding marble2")
			marblechaincodeutil.AddMarble(setup.network, setup.orderer, testchannelID, newlifecycleChaincode.Name,
				`{"name":"marble2", "color":"blue", "size":35, "owner":"tom", "price":99}`, org2peer0)
			By("transferring marble2")
			marblechaincodeutil.TransferMarble(setup.network, setup.orderer, testchannelID, newlifecycleChaincode.Name,
				`{"name":"marble2", "owner":"jerry"}`, org2peer0)

			assertPvtdataPresencePerCollectionConfig1(setup.network, newlifecycleChaincode.Name, "marble2")

			By("verifying the new ownership of marble2")
			marblechaincodeutil.AssertOwnershipInCollectionM(setup.network, testchannelID, newlifecycleChaincode.Name, "marble2", "jerry", org1peer0, org2peer0)

			// test 1: generate snapshots on 2 peers for the same blockNum and verify they are same
			blockNum := nwo.GetLedgerHeight(setup.network, org2peer0, testchannelID) - 1
			_, snapshotDir := generateAndCompareSnapshots(setup.network, org1peer0, org2peer0, blockNum)

			// test 2: bootstrap a new peer org2peer1 from snapshot and verify pvtdata
			By("starting new peer org2peer1 (couchdb)")
			org2peer1, couchProc := startPeer(setup, "Org2", "peer1", testchannelID, true)
			couchProcess = append(couchProcess, couchProc)

			By("installing chaincode on peer org2peer1")
			nwo.InstallChaincode(setup.network, newlifecycleChaincode, org2peer1)

			By("joining peer org2peer1 to the channel by snapshot")
			joinBySnapshot(setup.network, setup.orderer, org2peer1, testchannelID, snapshotDir, blockNum)

			By("waiting for pvtdata to be reconciled on org2peer1")
			waitForMarblePvtdataReconciliation(setup.network, org2peer1, testchannelID, newlifecycleChaincode.Name, []string{"marble2"})

			// verify pvtdata reconciliation after joinbysnapshot
			By("verifying marble2 pvtdata reconciliation on org2peer1")
			assertPvtdataPresencePerCollectionConfig1(setup.network, newlifecycleChaincode.Name, "marble2", org2peer1)
			By("verifying the new ownership of marble2")
			marblechaincodeutil.AssertOwnershipInCollectionM(setup.network, testchannelID, newlifecycleChaincode.Name, "marble2", "jerry", org2peer1)

			By("verifying marble1 does not exist")
			marblechaincodeutil.AssertDoesNotExistInCollectionM(setup.network, testchannelID, newlifecycleChaincode.Name, "marble1", org2peer1)
			marblechaincodeutil.AssertDoesNotExistInCollectionMPD(setup.network, testchannelID, newlifecycleChaincode.Name, "marble1", org2peer1)

			// test 3: submit a request to generate snapshot again on a peer (org2peer1) bootstrapped from a snapshot
			blockNumForNextSnapshot := nwo.GetLedgerHeight(setup.network, org2peer1, testchannelID)
			By(fmt.Sprintf("generating a snapshot at blockNum %d on org2peer1 that was bootstrapped by a snapshot", blockNumForNextSnapshot))
			submitSnapshotRequest(setup.network, testchannelID, blockNumForNextSnapshot, org2peer1, false, "Snapshot request submitted successfully")

			// block for marble3 tx is in snapshot, but block for marble4 tx is post snapshot
			By("adding marble3")
			marblechaincodeutil.AddMarble(setup.network, setup.orderer, testchannelID, newlifecycleChaincode.Name,
				`{"name":"marble3", "color":"blue", "size":35, "owner":"tom", "price":99}`, org2peer1)
			assertPvtdataPresencePerCollectionConfig1(setup.network, newlifecycleChaincode.Name, "marble3")

			By("adding marble4")
			marblechaincodeutil.AddMarble(setup.network, setup.orderer, testchannelID, newlifecycleChaincode.Name,
				`{"name":"marble4", "color":"blue", "size":35, "owner":"tom", "price":99}`, org2peer1)
			assertPvtdataPresencePerCollectionConfig1(setup.network, newlifecycleChaincode.Name, "marble4")

			By("verifying snapshot completed on org2peer1")
			verifyNoPendingSnapshotRequest(setup.network, org2peer1, testchannelID)
			nextSnapshotDir := filepath.Join(setup.network.PeerDir(org2peer1), "filesystem", "snapshots", "completed", testchannelID, strconv.Itoa(blockNumForNextSnapshot))

			// stop all the peers and only restart org2peer1
			setup.stopPeers()
			setup.startPeer(org2peer1)
			setup.peers = []*nwo.Peer{org2peer1}
			setup.network.Peers = setup.peers

			// test 4: bootstrap a new peer org2peer2 by snapshot and verify pvtdata reconciliation
			By("starting a peer (org2peer2) in existing org (leveldb)")
			org2peer2, _ := startPeer(setup, "Org2", "peer2", testchannelID, false)

			By("installing new lifecycle chaincode2 on peer org2peer2")
			nwo.InstallChaincode(setup.network, newlifecycleChaincode, org2peer2)

			By("joining peer org2peer2 to the channel by snapshot")
			joinBySnapshot(setup.network, setup.orderer, org2peer2, testchannelID, nextSnapshotDir, blockNumForNextSnapshot)

			By("waiting for pvtdata to be reconciled on org2peer2")
			waitForMarblePvtdataReconciliation(setup.network, org2peer2, testchannelID, newlifecycleChaincode.Name, []string{"marble2", "marble3", "marble4"})

			By("verifying marble4 pvtdata reconciliation on org2peer2")
			assertPvtdataPresencePerCollectionConfig1(setup.network, newlifecycleChaincode.Name, "marble4", org2peer2)
			By("verifying marble3 pvtdata reconciliation on org2peer2")
			assertPvtdataPresencePerCollectionConfig1(setup.network, newlifecycleChaincode.Name, "marble3", org2peer2)
			By("verifying marble2 pvtdata reconciliation on org2peer2")
			assertPvtdataPresencePerCollectionConfig1(setup.network, newlifecycleChaincode.Name, "marble2", org2peer2)
			By("verifying the new ownership of marble2")
			marblechaincodeutil.AssertOwnershipInCollectionM(setup.network, testchannelID, newlifecycleChaincode.Name, "marble2", "jerry", org2peer2)
			By("verifying marble1 does not exist")
			marblechaincodeutil.AssertDoesNotExistInCollectionM(setup.network, testchannelID, newlifecycleChaincode.Name, "marble1", org2peer2)
			marblechaincodeutil.AssertDoesNotExistInCollectionMPD(setup.network, testchannelID, newlifecycleChaincode.Name, "marble1", org2peer2)

			// test 5: bootstrap a new peer Org2peer3 from genesis block to verify pvtdata reconciliation
			By("startinging a peer Org2peer3 in an new org (leveldb)")
			org2peer3, _ := startPeer(setup, "Org2", "peer3", testchannelID, false)

			By("installing newlifecycleChaincode on new peer Org2peer3")
			nwo.InstallChaincode(setup.network, newlifecycleChaincode, org2peer3)

			By("joining peer Org2peer3 to the channel by genesis block")
			setup.network.JoinChannel(testchannelID, setup.orderer, org2peer3)

			By("waiting for the new peer to have the same ledger height")
			channelHeight := nwo.GetMaxLedgerHeight(setup.network, testchannelID, org2peer1)
			nwo.WaitUntilEqualLedgerHeight(setup.network, testchannelID, channelHeight, org2peer3)

			By("waiting for pvtdata to be reconciled on org2peer3")
			waitForMarblePvtdataReconciliation(setup.network, org2peer3, testchannelID, newlifecycleChaincode.Name, []string{"marble2", "marble3", "marble4"})

			By("verifying marble4 pvtdata reconciliation on org2peer3")
			assertPvtdataPresencePerCollectionConfig1(setup.network, newlifecycleChaincode.Name, "marble4", org2peer3)
			By("verifying marble3 pvtdata reconciliation on org2peer3")
			assertPvtdataPresencePerCollectionConfig1(setup.network, newlifecycleChaincode.Name, "marble3", org2peer3)
			By("verifying marble2 pvtdata reconciliation on org2peer3")
			assertPvtdataPresencePerCollectionConfig1(setup.network, newlifecycleChaincode.Name, "marble2", org2peer3)
			By("verifying the new ownership of marble2")
			marblechaincodeutil.AssertOwnershipInCollectionM(setup.network, testchannelID, newlifecycleChaincode.Name, "marble2", "jerry", org2peer3)
			By("verifying marble1 does not exist")
			marblechaincodeutil.AssertDoesNotExistInCollectionM(setup.network, testchannelID, newlifecycleChaincode.Name, "marble1", org2peer3)
			marblechaincodeutil.AssertDoesNotExistInCollectionMPD(setup.network, testchannelID, newlifecycleChaincode.Name, "marble1", org2peer3)

			// verify pvtdata hash on bootstrapped peers
			peers := []*nwo.Peer{org2peer1, org2peer2, org2peer3}
			for i := 2; i <= 4; i++ {
				name := fmt.Sprintf("marble%d", i)
				owner := "tom"
				if name == "marble2" {
					owner = "jerry"
				}

				By(fmt.Sprintf("verifying getMarbleHash for %s from all peers that has the chaincode instantiated", name))
				expectedBytes := util.ComputeStringHash(fmt.Sprintf(`{"docType":"marble","name":"%s","color":"blue","size":35,"owner":"%s"}`, name, owner))
				marblechaincodeutil.AssertMarblesPrivateHashM(setup.network, testchannelID, newlifecycleChaincode.Name, name, expectedBytes, peers)

				By(fmt.Sprintf("verifying getMarblePrivateDetailsHash for %s from all peers that has the chaincode instantiated", name))
				expectedBytes = util.ComputeStringHash(fmt.Sprintf(`{"docType":"marblePrivateDetails","name":"%s","price":99}`, name))
				marblechaincodeutil.AssertMarblesPrivateDetailsHashMPD(setup.network, testchannelID, newlifecycleChaincode.Name, name, expectedBytes, peers)
			}
		})
	})
})

func configPeerWithCouchDB(s *setup, peer *nwo.Peer) ifrit.Process {
	couchDB := &runner.CouchDB{}
	couchProc := ifrit.Invoke(couchDB)
	Eventually(couchProc.Ready(), runner.DefaultStartTimeout).Should(BeClosed())
	Consistently(couchProc.Wait()).ShouldNot(Receive())

	core := s.network.ReadPeerConfig(peer)
	core.Ledger.State.StateDatabase = "CouchDB"
	core.Ledger.State.CouchDBConfig.CouchDBAddress = couchDB.Address()

	By("configuring peer to couchdb address " + couchDB.Address())
	s.network.WritePeerConfig(peer, core)

	return couchProc
}

// initAndStartFourOrgsNetwork creates a network with multiple orgs.
// Initially only start Org1.peer0 and Org2.peer0 are started and join the channel.
func initAndStartFourOrgsNetwork() *setup {
	var err error
	testDir, err := ioutil.TempDir("", "snapshot")
	Expect(err).NotTo(HaveOccurred())

	client, err := docker.NewClientFromEnv()
	Expect(err).NotTo(HaveOccurred())

	config := nwo.BasicEtcdRaftNoSysChan()

	for _, peer := range config.Peers {
		peer.Channels = []*nwo.PeerChannel{
			{Name: testchannelID, Anchor: true},
		}
	}

	// add more peers to Org1 and Org2
	config.Peers = append(
		config.Peers,
		&nwo.Peer{
			Name:         "peer1",
			Organization: "Org1",
			Channels:     []*nwo.PeerChannel{},
		},
		&nwo.Peer{
			Name:         "peer1",
			Organization: "Org2",
			Channels:     []*nwo.PeerChannel{},
		},
		&nwo.Peer{
			Name:         "peer2",
			Organization: "Org2",
			Channels:     []*nwo.PeerChannel{},
		},
		&nwo.Peer{
			Name:         "peer3",
			Organization: "Org2",
			Channels:     []*nwo.PeerChannel{},
		},
	)

	// add org3 with one peer
	config.Organizations = append(config.Organizations, &nwo.Organization{
		Name:          "Org3",
		MSPID:         "Org3MSP",
		Domain:        "org3.example.com",
		EnableNodeOUs: true,
		Users:         2,
		CA:            &nwo.CA{Hostname: "ca"},
	})
	config.Profiles[0].Organizations = append(config.Profiles[0].Organizations, "Org3")
	config.Peers = append(config.Peers, &nwo.Peer{
		Name:         "peer0",
		Organization: "Org3",
		Channels: []*nwo.PeerChannel{
			{Name: testchannelID, Anchor: true},
		},
	})

	// add org4 with one peer
	config.Organizations = append(config.Organizations, &nwo.Organization{
		Name:          "Org4",
		MSPID:         "Org4MSP",
		Domain:        "org4.example.com",
		EnableNodeOUs: true,
		Users:         2,
		CA:            &nwo.CA{Hostname: "ca"},
	})
	config.Profiles[0].Organizations = append(config.Profiles[0].Organizations, "Org4")
	config.Peers = append(config.Peers, &nwo.Peer{
		Name:         "peer0",
		Organization: "Org4",
		Channels: []*nwo.PeerChannel{
			{Name: testchannelID, Anchor: true},
		},
	})

	n := nwo.New(config, testDir, client, StartPort(), components)
	n.GenerateConfigTree()
	n.Bootstrap()

	// set ReconcileSleepInterval to 1 second to reconcile pvtdata faster
	// set DeprioritizedDataReconcilerInterval to 2 seconds to resume reconciliation quickly
	// to prevent CI flake in case peer connection is temporarily lost.
	for _, p := range n.Peers {
		core := n.ReadPeerConfig(p)
		core.Peer.Gossip.PvtData.ReconcileSleepInterval = 1 * time.Second
		core.Ledger.PvtdataStore.DeprioritizedDataReconcilerInterval = 2 * time.Second
		n.WritePeerConfig(p, core)
	}

	// set org2peer2 and org2peer3's gossip bootstrap endpoints pointing to org2peer1
	org2peer1 := n.Peer("Org2", "peer1")
	for _, p := range []*nwo.Peer{n.Peer("Org2", "peer2"), n.Peer("Org2", "peer3")} {
		core := n.ReadPeerConfig(p)
		core.Peer.Gossip.Bootstrap = n.PeerAddress(org2peer1, nwo.ListenPort)
		n.WritePeerConfig(p, core)
	}

	// only keep Org1.peer0 and Org2.peer0 so we can add other peers back later to test join channel by snapshot
	peers := []*nwo.Peer{}
	for _, p := range n.Peers {
		if p.ID() == "Org1.peer0" || p.ID() == "Org2.peer0" {
			peers = append(peers, p)
		}
	}
	n.Peers = peers

	setup := &setup{
		testDir:   testDir,
		network:   n,
		peers:     peers,
		channelID: testchannelID,
		orderer:   n.Orderer("orderer"),
	}
	Expect(setup.testDir).To(Equal(setup.network.RootDir))

	By("starting orderer")
	setup.startOrderer()

	By("starting peers")
	setup.startPeers()

	By("creating and joining testchannel")
	channelparticipation.JoinOrdererJoinPeersAppChannel(setup.network, "testchannel", setup.orderer, setup.ordererRunner)

	By("verifying membership for testchannel")
	n.VerifyMembership(n.PeersWithChannel(testchannelID), testchannelID)

	return setup
}

// verifySnapshotRequestCmds invokes snapshot commands and verify the expected rersults.
// At the end, there will be no pending request.
func verifySnapshotRequestCmds(n *nwo.Network, peer *nwo.Peer, channel string, blockNum int) {
	By("submitting snaphost request for a future blockNum, expecting success")
	submitSnapshotRequest(n, channel, blockNum+10, peer, false, "Snapshot request submitted successfully")
	By("submitting snaphost request at same blockNum again, expecting error")
	submitSnapshotRequest(n, channel, blockNum+10, peer, true,
		fmt.Sprintf("duplicate snapshot request for block number %d", blockNum+10))
	By("submitting snaphost request for a previous blockNum, expecting error")
	submitSnapshotRequest(n, channel, blockNum-1, peer, true,
		fmt.Sprintf("requested snapshot for block number %d cannot be less than the last committed block number %d", blockNum-1, blockNum))
	By("listing pending snaphost requests, expecting success")
	pendingRequests := listPendingSnapshotRequests(n, channel, peer, n.PeerAddress(peer, nwo.ListenPort), false)
	Expect(pendingRequests).To(ContainSubstring(fmt.Sprintf("Successfully got pending snapshot requests: [%d]\n", blockNum+10)))
	By("canceling a pending snaphost request, expecting success")
	cancelSnapshotRequest(n, channel, blockNum+10, peer, n.PeerAddress(peer, nwo.ListenPort), false, "Snapshot request cancelled successfully")
	By("canceling the same snaphost request, expecting error")
	cancelSnapshotRequest(n, channel, blockNum+10, peer, n.PeerAddress(peer, nwo.ListenPort), true,
		fmt.Sprintf("no snapshot request exists for block number %d", blockNum+10))
	By("listing pending snaphost requests, expecting success")
	pendingRequests = listPendingSnapshotRequests(n, channel, peer, n.PeerAddress(peer, nwo.ListenPort), false)
	Expect(pendingRequests).To(ContainSubstring("Successfully got pending snapshot requests: []\n"))
}

func generateAndCompareSnapshots(n *nwo.Network, peer1, peer2 *nwo.Peer, blockNumForSnapshot int) (string, string) {
	By(fmt.Sprintf("submitting snapshot request at blockNum %d on peer %s", blockNumForSnapshot, peer1.ID()))
	submitSnapshotRequest(n, testchannelID, blockNumForSnapshot, peer1, false, "Snapshot request submitted successfully")

	By(fmt.Sprintf("submitting snaphost request at blockNum %d on peer %s", blockNumForSnapshot, peer2.ID()))
	submitSnapshotRequest(n, testchannelID, blockNumForSnapshot, peer2, false, "Snapshot request submitted successfully")

	By("verifying snapshot completed on peer1")
	verifyNoPendingSnapshotRequest(n, peer1, testchannelID)

	By("verifying snapshot completed on peer2")
	verifyNoPendingSnapshotRequest(n, peer2, testchannelID)

	By("comparing snapshot metadata generated on different peers for the same block number")
	snapshotDir1 := filepath.Join(n.PeerDir(peer1), "filesystem", "snapshots", "completed", testchannelID, strconv.Itoa(blockNumForSnapshot))
	snapshotDir2 := filepath.Join(n.PeerDir(peer2), "filesystem", "snapshots", "completed", testchannelID, strconv.Itoa(blockNumForSnapshot))
	compareSnapshotMetadata(snapshotDir1, snapshotDir2)

	return snapshotDir1, snapshotDir2
}

func verifyNoPendingSnapshotRequest(n *nwo.Network, peer *nwo.Peer, channelID string) {
	checkPending := func() []byte {
		return listPendingSnapshotRequests(n, channelID, peer, n.PeerAddress(peer, nwo.ListenPort), false)
	}
	Eventually(checkPending, n.EventuallyTimeout, 10*time.Second).Should(ContainSubstring("Successfully got pending snapshot requests: []\n"))
}

func submitSnapshotRequest(n *nwo.Network, channel string, blockNum int, peer *nwo.Peer, expectedError bool, expectedMsg string) {
	sess, err := n.PeerAdminSession(peer, commands.SnapshotSubmitRequest{
		ChannelID:   channel,
		BlockNumber: strconv.Itoa(blockNum),
		ClientAuth:  n.ClientAuthRequired,
		PeerAddress: n.PeerAddress(peer, nwo.ListenPort),
	})
	Expect(err).NotTo(HaveOccurred())
	if !expectedError {
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
		Expect(sess).To(gbytes.Say(expectedMsg))
	} else {
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(1))
		Expect(sess.Err).To(gbytes.Say(expectedMsg))
	}
}

func cancelSnapshotRequest(n *nwo.Network, channel string, blockNum int, peer *nwo.Peer, peerAddress string, expectedError bool, expectedMsg string) {
	sess, err := n.PeerAdminSession(peer, commands.SnapshotCancelRequest{
		ChannelID:   channel,
		BlockNumber: strconv.Itoa(blockNum),
		ClientAuth:  n.ClientAuthRequired,
		PeerAddress: peerAddress,
	})
	Expect(err).NotTo(HaveOccurred())
	if !expectedError {
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
		Expect(sess).To(gbytes.Say(expectedMsg))
	} else {
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(1))
		Expect(sess.Err).To(gbytes.Say(expectedMsg))
	}
}

func listPendingSnapshotRequests(n *nwo.Network, channel string, peer *nwo.Peer, peerAddress string, expectedError bool) []byte {
	sess, err := n.PeerAdminSession(peer, commands.SnapshotListPending{
		ChannelID:   channel,
		ClientAuth:  n.ClientAuthRequired,
		PeerAddress: peerAddress,
	})
	Expect(err).NotTo(HaveOccurred())

	if expectedError {
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(1))
		return sess.Err.Contents()
	}
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	return sess.Buffer().Contents()
}

func compareSnapshotMetadata(snapshotDir1, snapshotDir2 string) {
	for _, snapshotDir := range []string{snapshotDir1, snapshotDir2} {
		By("verifying snapshot dir exists: " + snapshotDir)
		Expect(snapshotDir).To(BeADirectory())
	}

	// compare metadata files
	for _, file := range []string{"_snapshot_signable_metadata.json", "_snapshot_additional_metadata.json"} {
		By("comparing metadata file from snapshots on multiple peers: " + file)
		fileContent1, err := ioutil.ReadFile(filepath.Join(snapshotDir1, file))
		Expect(err).NotTo(HaveOccurred())
		fileContent2, err := ioutil.ReadFile(filepath.Join(snapshotDir2, file))
		Expect(err).NotTo(HaveOccurred())
		Expect(fileContent1).To(Equal(fileContent2))
	}
}

// startPeer starts a peer to prepare for join channel test
func startPeer(s *setup, orgName, peerName, channelID string, useCouchDB bool) (*nwo.Peer, ifrit.Process) {
	peer := &nwo.Peer{
		Name:         peerName,
		Organization: orgName,
		Channels: []*nwo.PeerChannel{
			{Name: channelID},
		},
	}
	s.network.Peers = append(s.network.Peers, peer)
	s.peers = append(s.peers, peer)

	var couchProc ifrit.Process
	if useCouchDB {
		By("starting couch process and configuring it for peer " + peer.ID())
		couchProc = configPeerWithCouchDB(s, peer)
	}

	By("starting the new peer " + peer.ID())
	s.startPeer(peer)

	return peer, couchProc
}

func joinBySnapshot(n *nwo.Network, orderer *nwo.Orderer, peer *nwo.Peer, channelID string, snapshotDir string, lastBlockInSnapshot int) {
	channelHeight := nwo.GetMaxLedgerHeight(n, channelID, n.PeersWithChannel(channelID)...)

	By(fmt.Sprintf("joining a peer via snapshot %s", snapshotDir))
	n.JoinChannelBySnapshot(snapshotDir, peer)

	By("calling JoinBySnapshotStatus")
	checkStatus := func() string { return n.JoinBySnapshotStatus(peer) }
	Eventually(checkStatus, n.EventuallyTimeout, 10*time.Second).Should(ContainSubstring("No joinbysnapshot operation is in progress"))

	By("waiting for the new peer to have the same ledger height")
	nwo.WaitUntilEqualLedgerHeight(n, channelID, channelHeight, peer)

	By("verifying blockchain info on peer " + peer.ID())
	sess, err := n.PeerUserSession(peer, "Admin", commands.ChannelInfo{
		ChannelID: channelID,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	channelInfoStr := strings.TrimPrefix(string(sess.Buffer().Contents()[:]), "Blockchain info:")
	bcInfo := cb.BlockchainInfo{}
	err = json.Unmarshal([]byte(channelInfoStr), &bcInfo)
	Expect(err).NotTo(HaveOccurred())
	Expect(bcInfo.Height).To(Equal(uint64(channelHeight)))
	Expect(bcInfo.BootstrappingSnapshotInfo.LastBlockInSnapshot).To(Equal(uint64(lastBlockInSnapshot)))
}

func verifyQSCC(n *nwo.Network, peer *nwo.Peer, channelID string, lastBlockInSnapshot int, txidBeforeSnapshot string) {
	peerID := peer.ID()
	By("verifying qscc GetBlockByNumber returns an error for block number before snapshot on peer " + peerID)
	resp := callQSCC(n, peer, "qscc", "GetBlockByNumber", 1, channelID, strconv.Itoa(lastBlockInSnapshot))
	Expect(resp).To(ContainSubstring(fmt.Sprintf("The ledger is bootstrapped from a snapshot. First available block = [%d]", lastBlockInSnapshot+1)))

	By("verifying qscc GetBlockByNumber succeeds for a block number after snapshot on peer " + peerID)
	callQSCC(n, peer, "qscc", "GetBlockByNumber", 0, channelID, fmt.Sprintf("%d", lastBlockInSnapshot+1))

	By("verifying qscc GetBlockByTxID returns an error for a txid before snapshot on peer " + peerID)
	resp = callQSCC(n, peer, "qscc", "GetBlockByTxID", 1, channelID, txidBeforeSnapshot)
	Expect(resp).To(ContainSubstring(fmt.Sprintf("Failed to get block for txID %s, error details for the TXID [%s] not available. Ledger bootstrapped from a snapshot. First available block = [%d]",
		txidBeforeSnapshot, txidBeforeSnapshot, lastBlockInSnapshot+1)))

	By("verifying qscc GetBlockByTxID succeeds for a txid after snapshot on peer " + peerID)
	_, newTxid := getTxFromLastBlock(n, peer)
	callQSCC(n, peer, "qscc", "GetBlockByTxID", 0, channelID, newTxid)

	By("verifying qscc GetTransactionByID returns an error for a txid before snapshot on peer " + peerID)
	resp = callQSCC(n, peer, "qscc", "GetTransactionByID", 1, channelID, txidBeforeSnapshot)
	Expect(resp).To(ContainSubstring(fmt.Sprintf("Failed to get transaction with id %s, error details for the TXID [%s] not available. Ledger bootstrapped from a snapshot. First available block = [%d]",
		txidBeforeSnapshot, txidBeforeSnapshot, lastBlockInSnapshot+1)))

	By("verifying qscc GetTransactionByID succeeds for a txid after snapshot on peer " + peerID)
	callQSCC(n, peer, "qscc", "GetTransactionByID", 0, channelID, newTxid)
}

func installAndApproveChaincode(n *nwo.Network, orderer *nwo.Orderer, peer *nwo.Peer, channelID string, chaincode nwo.Chaincode, orgNames []string) {
	nwo.InstallChaincode(n, chaincode, peer)
	checkOrgs := make([]*nwo.Organization, len(orgNames))
	for i, orgName := range orgNames {
		checkOrgs[i] = n.Organization(orgName)
	}
	nwo.ApproveChaincodeForMyOrg(n, channelID, orderer, chaincode, n.PeersInOrg(peer.Organization)...)
	nwo.EnsureChaincodeCommitted(n, channelID, chaincode.Name, chaincode.Version, chaincode.Sequence, checkOrgs, peer)
}

// getTxFromLastBlock gets a transaction id from the latest block that has been
// marshaled and stored on the filesystem
func getTxFromLastBlock(n *nwo.Network, peer *nwo.Peer) (*cb.Envelope, string) {
	blockfile := filepath.Join(n.RootDir, "newest_block.pb")
	fetchNewest := commands.ChannelFetch{
		ChannelID:  "testchannel",
		Block:      "newest",
		OutputFile: blockfile,
	}
	sess, err := n.PeerAdminSession(peer, fetchNewest)
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess.Err).To(gbytes.Say("Received block: "))

	block := nwo.UnmarshalBlockFromFile(blockfile)
	txEnvelope, err := protoutil.UnmarshalEnvelope(block.Data.Data[0])
	Expect(err).NotTo(HaveOccurred())
	txID, err := protoutil.GetOrComputeTxIDFromEnvelope(block.Data.Data[0])
	Expect(err).NotTo(HaveOccurred())

	return txEnvelope, txID
}

func invokeAndQueryKVExecutorChaincode(n *nwo.Network, orderer *nwo.Orderer, channelID string, chaincode nwo.Chaincode, kvdata []kvexecutor.KVData, peers ...*nwo.Peer) {
	By("invoking kvexecutor chaincode")
	writeInputBytes, err := json.Marshal(kvdata)
	Expect(err).NotTo(HaveOccurred())
	writeInputBase64 := base64.StdEncoding.EncodeToString(writeInputBytes)

	peerAddresses := make([]string, 0)
	for _, peer := range peers {
		peerAddresses = append(peerAddresses, n.PeerAddress(peer, nwo.ListenPort))
	}

	invokeCommand := commands.ChaincodeInvoke{
		ChannelID:     channelID,
		Orderer:       n.OrdererAddress(orderer, nwo.ListenPort),
		Name:          chaincode.Name,
		Ctor:          fmt.Sprintf(`{"Args":["readWriteKVs","%s","%s"]}`, "", writeInputBase64),
		PeerAddresses: peerAddresses,
		WaitForEvent:  true,
	}
	invokeChaincode(n, peers[0], invokeCommand)

	channelPeers := n.PeersWithChannel(channelID)
	nwo.WaitUntilEqualLedgerHeight(n, channelID, nwo.GetLedgerHeight(n, peers[0], channelID), channelPeers...)

	By("querying kvexecutor chaincode")
	expectedMsg, err := json.Marshal(kvdata)
	Expect(err).NotTo(HaveOccurred())

	readInputBytes, err := json.Marshal(kvdata)
	Expect(err).NotTo(HaveOccurred())
	readInputBase64 := base64.StdEncoding.EncodeToString(readInputBytes)

	querycommand := commands.ChaincodeQuery{
		ChannelID: channelID,
		Name:      chaincode.Name,
		Ctor:      fmt.Sprintf(`{"Args":["readWriteKVs","%s","%s"]}`, readInputBase64, ""),
	}
	queryChaincode(n, peers[0], querycommand, string(expectedMsg), true)
}

func invokeChaincode(n *nwo.Network, peer *nwo.Peer, command commands.ChaincodeInvoke) {
	sess, err := n.PeerUserSession(peer, "User1", command)
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess.Err).To(gbytes.Say("Chaincode invoke successful."))
}

func queryChaincode(n *nwo.Network, peer *nwo.Peer, command commands.ChaincodeQuery, expectedMessage string, expectSuccess bool) {
	sess, err := n.PeerUserSession(peer, "User1", command)
	Expect(err).NotTo(HaveOccurred())
	if expectSuccess {
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
		Expect(sess).To(gbytes.Say(expectedMessage))
	} else {
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit())
		Expect(sess.Err).To(gbytes.Say(expectedMessage))
	}
}

func callQSCC(n *nwo.Network, peer *nwo.Peer, scc, operation string, retCode int, args ...string) []byte {
	args = append([]string{operation}, args...)
	chaincodeQuery := commands.ChaincodeQuery{
		ChannelID: testchannelID,
		Name:      scc,
		Ctor:      toCLIChaincodeArgs(args...),
	}

	sess, err := n.PeerAdminSession(peer, chaincodeQuery)
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(retCode))
	if retCode != 0 {
		return sess.Err.Contents()
	}
	return sess.Out.Contents()
}

func toCLIChaincodeArgs(args ...string) string {
	type cliArgs struct {
		Args []string
	}
	cArgs := &cliArgs{Args: args}
	cArgsJSON, err := json.Marshal(cArgs)
	Expect(err).NotTo(HaveOccurred())
	return string(cArgsJSON)
}

// waitForMarblePvtdataReconciliation queries the chaincode until it returns a success exit code, which means the data is available.
func waitForMarblePvtdataReconciliation(n *nwo.Network, peer *nwo.Peer, channelID, chaincodeName string, marbleNames []string) {
	for _, marbleName := range marbleNames {
		for _, funcName := range []string{"readMarble", "readMarblePrivateDetails"} {
			query := fmt.Sprintf(`{"Args":["%s","%s"]}`, funcName, marbleName)
			command := commands.ChaincodeQuery{
				ChannelID: channelID,
				Name:      chaincodeName,
				Ctor:      query,
			}
			queryData := func() int {
				sess, err := n.PeerUserSession(peer, "User1", command)
				Expect(err).NotTo(HaveOccurred())
				return sess.Wait(n.EventuallyTimeout).ExitCode()
			}
			Eventually(queryData, n.EventuallyTimeout).Should(Equal(0))
		}
	}
}

func assertPvtdataPresencePerCollectionConfig1(n *nwo.Network, chaincodeName, marbleName string, peers ...*nwo.Peer) {
	if len(peers) == 0 {
		peers = n.Peers
	}
	for _, peer := range peers {
		switch peer.Organization {
		case "Org1":
			By("asserting collection data M in org1 peer " + peer.ID() + " for " + marbleName)
			marblechaincodeutil.AssertPresentInCollectionM(n, testchannelID, chaincodeName, marbleName, peer)
			By("asserting no collection data MPD in org1 peer " + peer.ID() + " for " + marbleName)
			marblechaincodeutil.AssertNotPresentInCollectionMPD(n, testchannelID, chaincodeName, marbleName, peer)

		case "Org2":
			By("asserting collection data M in org2 peer " + peer.ID() + " for " + marbleName)
			marblechaincodeutil.AssertPresentInCollectionM(n, testchannelID, chaincodeName, marbleName, peer)
			By("asserting collection data MPD in org2 peer " + peer.ID() + " for " + marbleName)
			marblechaincodeutil.AssertPresentInCollectionMPD(n, testchannelID, chaincodeName, marbleName, peer)

		case "Org3":
			By("asserting no collection data M in org3 peer " + peer.ID() + " for " + marbleName)
			marblechaincodeutil.AssertNotPresentInCollectionM(n, testchannelID, chaincodeName, marbleName, peer)
			By("asserting collection data MPD in org3 peer " + peer.ID() + " for " + marbleName)
			marblechaincodeutil.AssertPresentInCollectionMPD(n, testchannelID, chaincodeName, marbleName, peer)
		}
	}
}

// invokeWithoutPassingOrdererEndPoint does not pass orderer endpoint to a chaincode invoke command.
// As a result, the command will send a cscc query to the peer and cscc will return the orderer endpoint from the channel config.
func invokeWithoutPassingOrdererEndPoint(n *nwo.Network, peer *nwo.Peer, channelID, chaincodeName string, funcAndArgs ...string) {
	command := commands.ChaincodeInvoke{
		ChannelID: channelID,
		Name:      chaincodeName,
		Ctor:      prepareChaincodeInvokeArgs(funcAndArgs...),
		PeerAddresses: []string{
			n.PeerAddress(peer, nwo.ListenPort),
		},
		WaitForEvent: true,
	}
	invokeChaincode(n, peer, command)
	nwo.WaitUntilEqualLedgerHeight(n, channelID, nwo.GetLedgerHeight(n, peer, channelID), n.PeersWithChannel(channelID)...)
}

// commitTx commits a transaction for a given transaction envelope
func commitTx(n *nwo.Network, orderer *nwo.Orderer, peer *nwo.Peer, channelID string, tx *cb.Envelope, txid string) error {
	By("getting the signer for user1 on peer " + peer.ID())
	signer := n.PeerUserSigner(peer, "User1")

	By("creating the deliver client to peer " + peer.ID())
	pcc := n.PeerClientConn(peer)
	defer pcc.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	df, err := pb.NewDeliverClient(pcc).DeliverFiltered(ctx)
	Expect(err).NotTo(HaveOccurred())
	defer df.CloseSend()

	By("starting filtered delivery on peer " + peer.ID())
	deliverEnvelope, err := protoutil.CreateSignedEnvelope(
		cb.HeaderType_DELIVER_SEEK_INFO,
		channelID,
		signer,
		&ab.SeekInfo{
			Behavior: ab.SeekInfo_BLOCK_UNTIL_READY,
			Start: &ab.SeekPosition{
				Type: &ab.SeekPosition_Newest{Newest: &ab.SeekNewest{}},
			},
			Stop: &ab.SeekPosition{
				Type: &ab.SeekPosition_Specified{
					Specified: &ab.SeekSpecified{Number: math.MaxUint64},
				},
			},
		},
		0,
		0,
	)
	Expect(err).NotTo(HaveOccurred())
	err = df.Send(deliverEnvelope)
	Expect(err).NotTo(HaveOccurred())

	By("creating orderer client and send transaction to the orderer" + orderer.ID())
	occ := n.OrdererClientConn(orderer)
	defer occ.Close()
	broadcastClient, err := ab.NewAtomicBroadcastClient(occ).Broadcast(context.Background())
	Expect(err).NotTo(HaveOccurred())

	err = broadcastClient.Send(tx)
	Expect(err).NotTo(HaveOccurred())

	By("waiting for deliver event on peer " + peer.ID())
	for {
		resp, err := df.Recv()
		if err != nil {
			return err
		}
		fb, ok := resp.Type.(*pb.DeliverResponse_FilteredBlock)
		if !ok {
			return fmt.Errorf("unexpected filtered block, received %T", resp.Type)
		}
		for _, tx := range fb.FilteredBlock.FilteredTransactions {
			if tx.Txid != txid {
				continue
			}
			if tx.TxValidationCode != pb.TxValidationCode_VALID {
				return fmt.Errorf("transaction invalidated with status (%s)", tx.TxValidationCode)
			}
			return nil
		}
	}
}
