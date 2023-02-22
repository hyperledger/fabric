//go:build generate
// +build generate

/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ledger

import (
	"fmt"
	"path/filepath"

	"github.com/hyperledger/fabric/integration/nwo"
	. "github.com/onsi/ginkgo/v2"
)

// This test generate sample ledger data that can be used to verify rebuild ledger function and upgrade function (in a future release).
// It is skipped in general. To generate sample ledger data, use build tag 'generate' and run this test in isolation.
// It does not delete the network directory so that you can copy the generated data to a different directory for unit tests.
// At the end of test, it prints `setup.testDir is <directory>`. Copy the network data under <directory> to
// the unit test directory for rebuild tests as needed.
// It generates the following blocks:
// block 0: genesis
// block 1 to 4: network setup
// block 5 to 8: marblesp chaincode instantiation
// block 9 to 12: marbles chancode instantiation
// block 13: marblesp chaincode invocation
// block 14 to 17: upgrade marblesp chaincode with a new collection config
// block 18: marbles chaincode invocation
var _ = Describe("sample ledger generation", func() {
	var (
		setup       *setup
		helper      *testHelper
		chaincodemp nwo.Chaincode
		chaincodem  nwo.Chaincode
	)

	BeforeEach(func() {
		setup = initThreeOrgsSetup()
		nwo.EnableCapabilities(setup.network, setup.channelID, "Application", "V2_0", setup.orderer, setup.peers...)
		helper = &testHelper{
			networkHelper: &networkHelper{
				Network:   setup.network,
				orderer:   setup.orderer,
				peers:     setup.peers,
				testDir:   setup.testDir,
				channelID: setup.channelID,
			},
		}

		chaincodemp = nwo.Chaincode{
			Name:              "marblesp",
			Version:           "1.0",
			Path:              components.Build("github.com/hyperledger/fabric/integration/chaincode/marbles_private/cmd"),
			Lang:              "binary",
			PackageFile:       filepath.Join(setup.testDir, "marbles-pvtdata.tar.gz"),
			Label:             "marbles-private-20",
			SignaturePolicy:   `OR ('Org1MSP.member','Org2MSP.member', 'Org3MSP.member')`,
			CollectionsConfig: filepath.Join("testdata", "collection_configs", "collections_config1.json"),
			Sequence:          "1",
		}

		chaincodem = nwo.Chaincode{
			Name:            "marbles",
			Version:         "0.0",
			Path:            "github.com/hyperledger/fabric/integration/chaincode/marbles/cmd",
			Lang:            "golang",
			PackageFile:     filepath.Join(setup.testDir, "marbles.tar.gz"),
			Label:           "marbles",
			SignaturePolicy: `OR ('Org1MSP.member','Org2MSP.member', 'Org3MSP.member')`,
			Sequence:        "1",
		}
	})

	AfterEach(func() {
		setup.terminateAllProcess()
		setup.network.Cleanup()
		// do not delete testDir and log it so that we can copy the test data to unit tests for verification purpose
		fmt.Fprintf(GinkgoWriter, "The test dir is %s. Use peers/org2.peer0/filesystem/ledgersData as the sample ledger for kvledger rebuild tests\n", setup.testDir)
	})

	It("creates marbles", func() {
		org2peer0 := setup.network.Peer("org2", "peer0")
		height := helper.getLedgerHeight(org2peer0)

		By(fmt.Sprintf("deploying marblesp chaincode at block height %d", height))
		helper.deployChaincode(chaincodemp)

		height = helper.getLedgerHeight(org2peer0)
		By(fmt.Sprintf("deploying marbles chaincode at block height %d", height))
		helper.deployChaincode(chaincodem)

		height = helper.getLedgerHeight(org2peer0)
		By(fmt.Sprintf("creating marbles1 with marblesp chaincode at block height %d", height))
		helper.addMarble("marblesp", `{"name":"marble1", "color":"blue", "size":35, "owner":"tom", "price":99}`, org2peer0)
		helper.waitUntilEqualLedgerHeight(height + 1)

		By("verifying marble1 exist in collectionMarbles & collectionMarblePrivateDetails in peer0.org2")
		helper.assertPresentInCollectionM("marblesp", "marble1", org2peer0)
		helper.assertPresentInCollectionMPD("marblesp", "marble1", org2peer0)

		By(fmt.Sprintf("upgrading marblesp chaincode at block height %d", helper.getLedgerHeight(org2peer0)))
		chaincodemp.Version = "1.1"
		chaincodemp.CollectionsConfig = filepath.Join("testdata", "collection_configs", "collections_config2.json")
		chaincodemp.Sequence = "2"
		nwo.DeployChaincode(setup.network, setup.channelID, setup.orderer, chaincodemp)

		mhelper := &marblesTestHelper{
			networkHelper: &networkHelper{
				Network:   setup.network,
				orderer:   setup.orderer,
				peers:     setup.peers,
				testDir:   setup.testDir,
				channelID: setup.channelID,
			},
		}
		By(fmt.Sprintf("creating marble100 with marbles chaincode at block height %d", helper.getLedgerHeight(org2peer0)))
		mhelper.invokeMarblesChaincode("marbles", org2peer0, "initMarble", "marble100", "blue", "35", "tom")
		By("transferring marble100 owner")
		mhelper.invokeMarblesChaincode("marbles", org2peer0, "transferMarble", "marble100", "jerry")

		By("verifying marble100 new owner after transfer by color")
		expectedResult := newMarble("marble100", "blue", 35, "jerry")
		mhelper.assertMarbleExists("marbles", org2peer0, expectedResult, "marble100")

		By("getting history for marble100")
		expectedHistoryResult := []*marbleHistoryResult{
			{IsDelete: "false", Value: newMarble("marble100", "blue", 35, "jerry")},
			{IsDelete: "false", Value: newMarble("marble100", "blue", 35, "tom")},
		}
		mhelper.assertGetHistoryForMarble("marbles", org2peer0, expectedHistoryResult, "marble100")
	})
})
