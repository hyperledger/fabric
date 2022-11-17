/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtdata

import (
	"context"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset/kvrwset"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	"github.com/hyperledger/fabric/integration/pvtdata/marblechaincodeutil"
	"github.com/hyperledger/fabric/protoutil"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
	ginkgomon "github.com/tedsuo/ifrit/ginkgomon_v2"
)

var _ = Describe("Pvtdata purge", func() {
	var (
		config                          *nwo.Config
		applicationCapabilitiesVersion  string
		testDir                         string
		network                         *nwo.Network
		orderer                         *nwo.Orderer
		org2Peer0, org3Peer0, org3Peer1 *nwo.Peer
		processes                       map[string]ifrit.Process
		peerRunners                     map[string]*ginkgomon.Runner
		cancel                          context.CancelFunc
		chaincode                       *nwo.Chaincode
	)

	JustBeforeEach(func() {
		var err error
		testDir, err = ioutil.TempDir("", "purgedata")
		Expect(err).NotTo(HaveOccurred())

		// Add additional peer before generating config tree
		org3Peer1 = &nwo.Peer{
			Name:         "peer1",
			Organization: "Org3",
			Channels: []*nwo.PeerChannel{
				{Name: "testchannel", Anchor: true},
			},
		}
		config.Peers = append(config.Peers, org3Peer1)

		network = nwo.New(config, testDir, nil, StartPort(), components)

		network.GenerateConfigTree()
		network.Bootstrap()

		// Remove additional peer from the network to be added later when required
		peers := []*nwo.Peer{}
		for _, p := range config.Peers {
			if p.Organization != org3Peer1.Organization || p.Name != org3Peer1.Name {
				peers = append(peers, p)
			}
		}
		network.Peers = peers

		processes = map[string]ifrit.Process{}
		peerRunners = map[string]*ginkgomon.Runner{}

		orderer = network.Orderer("orderer")
		ordererRunner := network.OrdererRunner(orderer)
		process := ifrit.Invoke(ordererRunner)
		Eventually(process.Ready(), network.EventuallyTimeout).Should(BeClosed())
		processes["OrdererGroupRunner"] = process

		for _, peer := range network.Peers {
			startPeer(network, processes, peerRunners, peer)
		}

		orderer = network.Orderer("orderer")

		network.CreateAndJoinChannel(orderer, channelID)
		network.UpdateChannelAnchors(orderer, channelID)
		network.VerifyMembership(
			network.PeersWithChannel(channelID),
			channelID,
		)
		nwo.EnableCapabilities(
			network,
			channelID,
			"Application", applicationCapabilitiesVersion,
			orderer,
			network.PeersWithChannel(channelID)...,
		)

		chaincode = &nwo.Chaincode{
			Name:              "marblesp",
			Version:           "0.0",
			Path:              components.Build("github.com/hyperledger/fabric/integration/chaincode/marbles_private/cmd"),
			Lang:              "binary",
			PackageFile:       filepath.Join(testDir, "purgecc.tar.gz"),
			Ctor:              `{"Args":[]}`,
			SignaturePolicy:   `OR ('Org1MSP.member','Org2MSP.member', 'Org3MSP.member')`,
			CollectionsConfig: CollectionConfig("collections_config9.json"),
			Sequence:          "1",
			InitRequired:      false,
			Label:             "purgecc_label",
		}

		nwo.DeployChaincode(network, channelID, orderer, *chaincode)

		org2Peer0 = network.Peer("Org2", "peer0")
		org3Peer0 = network.Peer("Org3", "peer0")

		_, cancel = context.WithTimeout(context.Background(), network.EventuallyTimeout)

		marblechaincodeutil.AddMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-0", "color":"blue", "size":35, "owner":"tom", "price":99}`, org2Peer0)
	})

	AfterEach(func() {
		cancel()

		for _, process := range processes {
			process.Signal(syscall.SIGTERM)
			Eventually(process.Wait(), network.EventuallyTimeout).Should(Receive())
		}
		if network != nil {
			network.Cleanup()
		}
		os.RemoveAll(testDir)
	})

	When("the purge private data capability is not enabled", func() {
		BeforeEach(func() {
			config = nwo.ThreeOrgEtcdRaft()
			applicationCapabilitiesVersion = "V2_0"
		})

		It("should fail with an error if the purge capability has not been enabled on the channel", func() {
			marblePurgeBase64 := base64.StdEncoding.EncodeToString([]byte(`{"name":"test-marble-0"}`))

			purgeCommand := commands.ChaincodeInvoke{
				ChannelID: channelID,
				Orderer:   network.OrdererAddress(orderer, nwo.ListenPort),
				Name:      chaincode.Name,
				Ctor:      `{"Args":["purge"]}`,
				Transient: fmt.Sprintf(`{"marble_purge":"%s"}`, marblePurgeBase64),
				PeerAddresses: []string{
					network.PeerAddress(org2Peer0, nwo.ListenPort),
				},
				WaitForEvent: true,
			}

			marblechaincodeutil.AssertInvokeChaincodeFails(network, org2Peer0, purgeCommand, "Failed to purge state:PURGE_PRIVATE_DATA failed: transaction ID: [a-f0-9]{64}: purge private data is not enabled, channel application capability of V2_5 or later is required")
		})
	})

	When("the purge private data capability is enabled", func() {
		BeforeEach(func() {
			config = nwo.ThreeOrgEtcdRaft()
			config.Profiles[0].Blocks = &nwo.Blocks{
				BatchTimeout:      6,
				MaxMessageCount:   30,
				AbsoluteMaxBytes:  99,
				PreferredMaxBytes: 512,
			}
			applicationCapabilitiesVersion = "V2_5"
		})

		It("should prevent purged data being included in responses after the purge transaction has been committed", func() {
			marblechaincodeutil.AddMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-1", "color":"green", "size":42, "owner":"simon", "price":180}`, org2Peer0)
			marblechaincodeutil.AddMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-2", "color":"red", "size":24, "owner":"heather", "price":635}`, org2Peer0)
			marblechaincodeutil.AddMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-3", "color":"black", "size":12, "owner":"bob", "price":2}`, org2Peer0)
			marblechaincodeutil.AddMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-4", "color":"yellow", "size":180, "owner":"liz", "price":100}`, org2Peer0)
			marblechaincodeutil.AddMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-5", "color":"pink", "size":60, "owner":"joe", "price":999}`, org2Peer0)
			marblechaincodeutil.AddMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-6", "color":"purple", "size":1, "owner":"clive", "price":1984}`, org2Peer0)

			marblechaincodeutil.AssertPresentInCollectionM(network, channelID, chaincode.Name, "test-marble-0", org2Peer0)
			marblechaincodeutil.AssertPresentInCollectionMPD(network, channelID, chaincode.Name, "test-marble-0", org2Peer0)
			marblechaincodeutil.AssertPresentInCollectionM(network, channelID, chaincode.Name, "test-marble-1", org2Peer0)
			marblechaincodeutil.AssertPresentInCollectionMPD(network, channelID, chaincode.Name, "test-marble-1", org2Peer0)
			marblechaincodeutil.AssertPresentInCollectionM(network, channelID, chaincode.Name, "test-marble-2", org2Peer0)
			marblechaincodeutil.AssertPresentInCollectionMPD(network, channelID, chaincode.Name, "test-marble-2", org2Peer0)
			marblechaincodeutil.AssertPresentInCollectionM(network, channelID, chaincode.Name, "test-marble-3", org2Peer0)
			marblechaincodeutil.AssertPresentInCollectionMPD(network, channelID, chaincode.Name, "test-marble-3", org2Peer0)
			marblechaincodeutil.AssertPresentInCollectionM(network, channelID, chaincode.Name, "test-marble-4", org2Peer0)
			marblechaincodeutil.AssertPresentInCollectionMPD(network, channelID, chaincode.Name, "test-marble-4", org2Peer0)
			marblechaincodeutil.AssertPresentInCollectionM(network, channelID, chaincode.Name, "test-marble-5", org2Peer0)
			marblechaincodeutil.AssertPresentInCollectionMPD(network, channelID, chaincode.Name, "test-marble-5", org2Peer0)
			marblechaincodeutil.AssertPresentInCollectionM(network, channelID, chaincode.Name, "test-marble-6", org2Peer0)
			marblechaincodeutil.AssertPresentInCollectionMPD(network, channelID, chaincode.Name, "test-marble-6", org2Peer0)

			marblechaincodeutil.PurgeMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-0"}`, org2Peer0)
			marblechaincodeutil.PurgeMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-1"}`, org2Peer0)

			marblechaincodeutil.AssertDoesNotExistInCollectionM(network, channelID, chaincode.Name, "test-marble-0", org2Peer0)
			marblechaincodeutil.AssertDoesNotExistInCollectionMPD(network, channelID, chaincode.Name, "test-marble-0", org2Peer0)
			marblechaincodeutil.AssertDoesNotExistInCollectionM(network, channelID, chaincode.Name, "test-marble-1", org2Peer0)
			marblechaincodeutil.AssertDoesNotExistInCollectionMPD(network, channelID, chaincode.Name, "test-marble-1", org2Peer0)

			marblechaincodeutil.AssertPresentInCollectionM(network, channelID, chaincode.Name, "test-marble-2", org2Peer0)
			marblechaincodeutil.AssertPresentInCollectionMPD(network, channelID, chaincode.Name, "test-marble-2", org2Peer0)
			marblechaincodeutil.AssertPresentInCollectionM(network, channelID, chaincode.Name, "test-marble-3", org2Peer0)
			marblechaincodeutil.AssertPresentInCollectionMPD(network, channelID, chaincode.Name, "test-marble-3", org2Peer0)
			marblechaincodeutil.AssertPresentInCollectionM(network, channelID, chaincode.Name, "test-marble-4", org2Peer0)
			marblechaincodeutil.AssertPresentInCollectionMPD(network, channelID, chaincode.Name, "test-marble-4", org2Peer0)
			marblechaincodeutil.AssertPresentInCollectionM(network, channelID, chaincode.Name, "test-marble-5", org2Peer0)
			marblechaincodeutil.AssertPresentInCollectionMPD(network, channelID, chaincode.Name, "test-marble-5", org2Peer0)
			marblechaincodeutil.AssertPresentInCollectionM(network, channelID, chaincode.Name, "test-marble-6", org2Peer0)
			marblechaincodeutil.AssertPresentInCollectionMPD(network, channelID, chaincode.Name, "test-marble-6", org2Peer0)

			// Purge multiple marbles in a single block
			var wg sync.WaitGroup
			wg.Add(5)
			for i := 2; i < 7; i++ {
				go func(marblePurge string) {
					defer GinkgoRecover()
					defer wg.Done()
					marblechaincodeutil.PurgeMarble(network, orderer, channelID, chaincode.Name, marblePurge, org2Peer0)
				}(fmt.Sprintf(`{"name":"test-marble-%d"}`, i))
			}
			wg.Wait()
			Expect(nwo.GetLedgerHeight(network, org2Peer0, channelID)).To(Equal(19))

			marblechaincodeutil.AssertDoesNotExistInCollectionM(network, channelID, chaincode.Name, "test-marble-0", org2Peer0)
			marblechaincodeutil.AssertDoesNotExistInCollectionMPD(network, channelID, chaincode.Name, "test-marble-0", org2Peer0)
			marblechaincodeutil.AssertDoesNotExistInCollectionM(network, channelID, chaincode.Name, "test-marble-1", org2Peer0)
			marblechaincodeutil.AssertDoesNotExistInCollectionMPD(network, channelID, chaincode.Name, "test-marble-1", org2Peer0)
			marblechaincodeutil.AssertDoesNotExistInCollectionM(network, channelID, chaincode.Name, "test-marble-2", org2Peer0)
			marblechaincodeutil.AssertDoesNotExistInCollectionMPD(network, channelID, chaincode.Name, "test-marble-2", org2Peer0)
			marblechaincodeutil.AssertDoesNotExistInCollectionM(network, channelID, chaincode.Name, "test-marble-3", org2Peer0)
			marblechaincodeutil.AssertDoesNotExistInCollectionMPD(network, channelID, chaincode.Name, "test-marble-3", org2Peer0)
			marblechaincodeutil.AssertDoesNotExistInCollectionM(network, channelID, chaincode.Name, "test-marble-4", org2Peer0)
			marblechaincodeutil.AssertDoesNotExistInCollectionMPD(network, channelID, chaincode.Name, "test-marble-4", org2Peer0)
			marblechaincodeutil.AssertDoesNotExistInCollectionM(network, channelID, chaincode.Name, "test-marble-5", org2Peer0)
			marblechaincodeutil.AssertDoesNotExistInCollectionMPD(network, channelID, chaincode.Name, "test-marble-5", org2Peer0)
			marblechaincodeutil.AssertDoesNotExistInCollectionM(network, channelID, chaincode.Name, "test-marble-6", org2Peer0)
			marblechaincodeutil.AssertDoesNotExistInCollectionMPD(network, channelID, chaincode.Name, "test-marble-6", org2Peer0)
			marblechaincodeutil.AssertDoesNotExistInCollectionM(network, channelID, chaincode.Name, "test-marble-7", org2Peer0)
			marblechaincodeutil.AssertDoesNotExistInCollectionMPD(network, channelID, chaincode.Name, "test-marble-7", org2Peer0)
		})

		It("should prevent purged data being included block event replays after the purge transaction has been committed", func() {
			marblechaincodeutil.AddMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-10", "color":"green", "size":42, "owner":"simon", "price":180}`, org2Peer0)
			marblechaincodeutil.AddMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-100", "color":"red", "size":24, "owner":"heather", "price":635}`, org2Peer0)
			marblechaincodeutil.AddMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-1000", "color":"black", "size":12, "owner":"bob", "price":2}`, org2Peer0)

			marblechaincodeutil.AssertPresentInCollectionM(network, channelID, chaincode.Name, "test-marble-0", org2Peer0)
			marblechaincodeutil.AssertPresentInCollectionMPD(network, channelID, chaincode.Name, "test-marble-0", org2Peer0)
			marblechaincodeutil.AssertPresentInCollectionM(network, channelID, chaincode.Name, "test-marble-10", org2Peer0)
			marblechaincodeutil.AssertPresentInCollectionMPD(network, channelID, chaincode.Name, "test-marble-10", org2Peer0)
			marblechaincodeutil.AssertPresentInCollectionM(network, channelID, chaincode.Name, "test-marble-100", org2Peer0)
			marblechaincodeutil.AssertPresentInCollectionMPD(network, channelID, chaincode.Name, "test-marble-100", org2Peer0)
			marblechaincodeutil.AssertPresentInCollectionM(network, channelID, chaincode.Name, "test-marble-1000", org2Peer0)
			marblechaincodeutil.AssertPresentInCollectionMPD(network, channelID, chaincode.Name, "test-marble-1000", org2Peer0)

			marblechaincodeutil.PurgeMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-0"}`, org2Peer0)
			marblechaincodeutil.PurgeMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-1000"}`, org2Peer0)

			assertBlockEventsOnlyContainUnpurgedPrivateData(network, org2Peer0, chaincode.Name, []string{"test-marble-10", "\x00color~name\x00green\x00test-marble-10\x00", "test-marble-100", "\x00color~name\x00red\x00test-marble-100\x00"})
		})

		// 2. The endorsement policy is evaluated correctly for a purge transaction under
		//    different endorsement policy settings (e.g., collection level/ key-hash based)
		//    Note: The endorsement policy level tests need not to be prioritized over other
		//    behaviour, and they need not to be very exhaustive since they should be covered
		//    by existing write/delete operations
		PIt("should correctly enforce collection level endorsement policies")
		It("should correctly enforce key-hash based endorsement policies", func() {
			marblechaincodeutil.AssertPresentInCollectionM(network, channelID, chaincode.Name, "test-marble-0", org2Peer0)
			marblechaincodeutil.AssertPresentInCollectionMPD(network, channelID, chaincode.Name, "test-marble-0", org2Peer0)

			marblechaincodeutil.SetMarblePolicy(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-0","org":"Org3"}`, org2Peer0)

			marblePurgeBase64 := base64.StdEncoding.EncodeToString([]byte(`{"name":"test-marble-0"}`))

			purgeCommand := commands.ChaincodeInvoke{
				ChannelID: channelID,
				Orderer:   network.OrdererAddress(orderer, nwo.ListenPort),
				Name:      chaincode.Name,
				Ctor:      `{"Args":["purge"]}`,
				Transient: fmt.Sprintf(`{"marble_purge":"%s"}`, marblePurgeBase64),
				PeerAddresses: []string{
					network.PeerAddress(org2Peer0, nwo.ListenPort),
				},
				WaitForEvent: true,
			}

			marblechaincodeutil.AssertInvokeChaincodeFails(network, org2Peer0, purgeCommand, "Error: transaction invalidated with status \\(ENDORSEMENT_POLICY_FAILURE\\) - proposal response: <nil>")

			marblechaincodeutil.AssertPresentInCollectionM(network, channelID, chaincode.Name, "test-marble-0", org2Peer0)
			marblechaincodeutil.AssertPresentInCollectionMPD(network, channelID, chaincode.Name, "test-marble-0", org2Peer0)
		})
		PIt("should correctly enforce other endorsement policies (TBC)")

		It("should remove all purged data from a previously eligible peer", func() {
			marblechaincodeutil.AddMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-80", "color":"white", "size":4, "owner":"liz", "price":4}`, org2Peer0)
			marblechaincodeutil.AddMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-8080", "color":"orange", "size":80, "owner":"clive", "price":88}`, org2Peer0)

			marblechaincodeutil.AssertPresentInCollectionM(network, channelID, chaincode.Name, "test-marble-0", org2Peer0)
			marblechaincodeutil.AssertPresentInCollectionMPD(network, channelID, chaincode.Name, "test-marble-0", org2Peer0)
			marblechaincodeutil.AssertPresentInCollectionM(network, channelID, chaincode.Name, "test-marble-80", org2Peer0)
			marblechaincodeutil.AssertPresentInCollectionMPD(network, channelID, chaincode.Name, "test-marble-80", org2Peer0)
			marblechaincodeutil.AssertPresentInCollectionM(network, channelID, chaincode.Name, "test-marble-8080", org2Peer0)
			marblechaincodeutil.AssertPresentInCollectionMPD(network, channelID, chaincode.Name, "test-marble-8080", org2Peer0)

			chaincode.Version = "1.1"
			chaincode.CollectionsConfig = CollectionConfig("remove_org3_config.json")
			chaincode.Sequence = "2"
			nwo.DeployChaincode(network, channelID, orderer, *chaincode)

			marblechaincodeutil.PurgeMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-0"}`, org2Peer0)
			marblechaincodeutil.PurgeMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-80"}`, org2Peer0)

			marblechaincodeutil.AssertDoesNotExistInCollectionM(network, channelID, chaincode.Name, "test-marble-0", org3Peer0)
			marblechaincodeutil.AssertDoesNotExistInCollectionMPD(network, channelID, chaincode.Name, "test-marble-0", org3Peer0)
			marblechaincodeutil.AssertDoesNotExistInCollectionM(network, channelID, chaincode.Name, "test-marble-80", org3Peer0)
			marblechaincodeutil.AssertDoesNotExistInCollectionMPD(network, channelID, chaincode.Name, "test-marble-80", org3Peer0)

			runner := peerRunners[org3Peer0.ID()]
			Eventually(runner.Err(), network.EventuallyTimeout).Should(gbytes.Say("Purging private data from private data storage channel=testchannel chaincode=marblesp collection=collectionMarblePrivateDetails key=test-marble-0 blockNum=\\d+ tranNum=\\d+"))
			Eventually(runner.Err(), network.EventuallyTimeout).Should(gbytes.Say("Purging private data from private data storage channel=testchannel chaincode=marblesp collection=collectionMarblePrivateDetails key=test-marble-80 blockNum=\\d+ tranNum=\\d+"))
			Eventually(runner.Err(), network.EventuallyTimeout).Should(gbytes.Say("Purged private data from private data storage channel=testchannel numKeysPurged=\\d+ numPrivateDataStoreRecordsPurged=\\d+"))
		})

		It("should enable new peers to start and pull private data from existing peers without errors or warnings", func() {
			marblechaincodeutil.AddMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-1", "color":"green", "size":42, "owner":"simon", "price":180}`, org2Peer0)
			marblechaincodeutil.AddMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-2", "color":"red", "size":24, "owner":"heather", "price":635}`, org2Peer0)
			marblechaincodeutil.AddMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-3", "color":"black", "size":12, "owner":"bob", "price":2}`, org2Peer0)

			marblechaincodeutil.PurgeMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-1"}`, org2Peer0)
			marblechaincodeutil.PurgeMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-3"}`, org2Peer0)

			process := addPeer(network, orderer, org3Peer1)
			processes[org3Peer1.ID()] = process

			nwo.PackageAndInstallChaincode(network, *chaincode, org3Peer1)
			network.VerifyMembership(network.Peers, channelID, chaincode.Name)

			marblechaincodeutil.AssertPresentInCollectionM(network, channelID, chaincode.Name, `test-marble-0`, org3Peer1)
			marblechaincodeutil.AssertPresentInCollectionMPD(network, channelID, chaincode.Name, `test-marble-0`, org3Peer1)
			marblechaincodeutil.AssertDoesNotExistInCollectionM(network, channelID, chaincode.Name, `test-marble-1`, org3Peer1)
			marblechaincodeutil.AssertDoesNotExistInCollectionMPD(network, channelID, chaincode.Name, `test-marble-1`, org3Peer1)
			marblechaincodeutil.AssertPresentInCollectionM(network, channelID, chaincode.Name, `test-marble-2`, org3Peer1)
			marblechaincodeutil.AssertPresentInCollectionMPD(network, channelID, chaincode.Name, `test-marble-2`, org3Peer1)
			marblechaincodeutil.AssertDoesNotExistInCollectionM(network, channelID, chaincode.Name, `test-marble-3`, org3Peer1)
			marblechaincodeutil.AssertDoesNotExistInCollectionMPD(network, channelID, chaincode.Name, `test-marble-3`, org3Peer1)
		})

		It("should enable successful peer reconciliation with partial write-sets", func() {
			marblechaincodeutil.AddMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-1", "color":"green", "size":42, "owner":"simon", "price":180}`, org2Peer0)
			marblechaincodeutil.AddMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-2", "color":"red", "size":24, "owner":"heather", "price":635}`, org2Peer0)
			marblechaincodeutil.AddMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-3", "color":"black", "size":12, "owner":"bob", "price":2}`, org2Peer0)

			marblechaincodeutil.PurgeMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-1"}`, org2Peer0)
			marblechaincodeutil.PurgeMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-3"}`, org2Peer0)

			ledgerHeight := nwo.GetLedgerHeight(network, network.Peers[0], channelID)

			for _, peer := range network.Peers {
				stopPeer(network, processes, peer)
			}
			stoppedPeers := network.Peers
			network.Peers = []*nwo.Peer{}

			startNewPeer(network, orderer, org3Peer1, ledgerHeight, processes, peerRunners)

			nwo.PackageAndInstallChaincode(network, *chaincode, org3Peer1)
			network.VerifyMembership([]*nwo.Peer{org3Peer1}, channelID, chaincode.Name)

			runner := peerRunners[org3Peer1.ID()]
			Eventually(runner.Err(), network.EventuallyTimeout).Should(gbytes.Say("Could not fetch \\(or mark to reconcile later\\) 2 eligible collection private write sets for block \\[\\d+\\] \\(0 from local cache, 0 from transient store, 0 from other peers\\)\\. Will commit block with missing private write sets:\\[txID: [0123456789abcdef]+, seq: \\d+, namespace: marblesp, collection: collectionMarblePrivateDetails"))

			for _, peer := range stoppedPeers {
				startPeer(network, processes, peerRunners, peer)
			}
			network.Peers = append(stoppedPeers, org3Peer1)

			// Wait for reconciliation to complete
			time.Sleep(30 * time.Second)

			marblechaincodeutil.AssertPresentInCollectionM(network, channelID, chaincode.Name, `test-marble-0`, org3Peer1)
			marblechaincodeutil.AssertPresentInCollectionMPD(network, channelID, chaincode.Name, `test-marble-0`, org3Peer1)
			marblechaincodeutil.AssertDoesNotExistInCollectionM(network, channelID, chaincode.Name, `test-marble-1`, org3Peer1)
			marblechaincodeutil.AssertDoesNotExistInCollectionMPD(network, channelID, chaincode.Name, `test-marble-1`, org3Peer1)
			marblechaincodeutil.AssertPresentInCollectionM(network, channelID, chaincode.Name, `test-marble-2`, org3Peer1)
			marblechaincodeutil.AssertPresentInCollectionMPD(network, channelID, chaincode.Name, `test-marble-2`, org3Peer1)
			marblechaincodeutil.AssertDoesNotExistInCollectionM(network, channelID, chaincode.Name, `test-marble-3`, org3Peer1)
			marblechaincodeutil.AssertDoesNotExistInCollectionMPD(network, channelID, chaincode.Name, `test-marble-3`, org3Peer1)
		})

		It("should not remove new data after a previous purge operation", func() {
			marblechaincodeutil.AddMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-10", "color":"green", "size":42, "owner":"simon", "price":180}`, org2Peer0)
			marblechaincodeutil.AddMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-100", "color":"red", "size":24, "owner":"heather", "price":635}`, org2Peer0)
			marblechaincodeutil.AddMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-1000", "color":"black", "size":12, "owner":"bob", "price":2}`, org2Peer0)

			marblechaincodeutil.AssertPresentInCollectionM(network, channelID, chaincode.Name, "test-marble-0", org2Peer0)
			marblechaincodeutil.AssertPresentInCollectionMPD(network, channelID, chaincode.Name, "test-marble-0", org2Peer0)
			marblechaincodeutil.AssertPresentInCollectionM(network, channelID, chaincode.Name, "test-marble-10", org2Peer0)
			marblechaincodeutil.AssertPresentInCollectionMPD(network, channelID, chaincode.Name, "test-marble-10", org2Peer0)
			marblechaincodeutil.AssertPresentInCollectionM(network, channelID, chaincode.Name, "test-marble-100", org2Peer0)
			marblechaincodeutil.AssertPresentInCollectionMPD(network, channelID, chaincode.Name, "test-marble-100", org2Peer0)
			marblechaincodeutil.AssertPresentInCollectionM(network, channelID, chaincode.Name, "test-marble-1000", org2Peer0)
			marblechaincodeutil.AssertPresentInCollectionMPD(network, channelID, chaincode.Name, "test-marble-1000", org2Peer0)

			marblechaincodeutil.PurgeMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-0"}`, org2Peer0)
			marblechaincodeutil.PurgeMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-1000"}`, org2Peer0)

			marblechaincodeutil.AssertDoesNotExistInCollectionM(network, channelID, chaincode.Name, "test-marble-0", org2Peer0)
			marblechaincodeutil.AssertDoesNotExistInCollectionMPD(network, channelID, chaincode.Name, "test-marble-0", org2Peer0)
			marblechaincodeutil.AssertDoesNotExistInCollectionM(network, channelID, chaincode.Name, "test-marble-1000", org2Peer0)
			marblechaincodeutil.AssertDoesNotExistInCollectionMPD(network, channelID, chaincode.Name, "test-marble-1000", org2Peer0)

			marblechaincodeutil.AddMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-1000", "color":"violet", "size":1000, "owner":"siobhán", "price":99}`, org2Peer0)
			marblechaincodeutil.AddMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-9000", "color":"brown", "size":9000, "owner":"charles", "price":9000}`, org2Peer0)

			marblechaincodeutil.AssertPresentInCollectionM(network, channelID, chaincode.Name, "test-marble-1000", org2Peer0)
			marblechaincodeutil.AssertPresentInCollectionMPD(network, channelID, chaincode.Name, "test-marble-1000", org2Peer0)
			marblechaincodeutil.AssertPresentInCollectionM(network, channelID, chaincode.Name, "test-marble-9000", org2Peer0)
			marblechaincodeutil.AssertPresentInCollectionMPD(network, channelID, chaincode.Name, "test-marble-9000", org2Peer0)
		})
	})
})

func assertBlockEventsOnlyContainUnpurgedPrivateData(network *nwo.Network, peer *nwo.Peer, chaincodeName string, expectedPrivateDataKeys []string) {
	ledgerHeight := nwo.GetLedgerHeight(network, peer, channelID)
	conn := network.PeerClientConn(peer)

	ctx, cancel := context.WithTimeout(context.Background(), network.EventuallyTimeout)
	defer cancel()

	dp, err := pb.NewDeliverClient(conn).DeliverWithPrivateData(ctx)
	Expect(err).NotTo(HaveOccurred())
	defer dp.CloseSend()

	signingIdentity := network.PeerUserSigner(peer, "User1")

	deliverEnvelope, err := protoutil.CreateSignedEnvelope(
		cb.HeaderType_DELIVER_SEEK_INFO,
		channelID,
		signingIdentity,
		&ab.SeekInfo{
			Behavior: ab.SeekInfo_BLOCK_UNTIL_READY,
			Start: &ab.SeekPosition{
				Type: &ab.SeekPosition_Oldest{Oldest: &ab.SeekOldest{}},
			},
			Stop: &ab.SeekPosition{
				Type: &ab.SeekPosition_Specified{
					Specified: &ab.SeekSpecified{Number: uint64(ledgerHeight)},
				},
			},
		},
		0,
		0,
	)
	Expect(err).NotTo(HaveOccurred())
	err = dp.Send(deliverEnvelope)
	Expect(err).NotTo(HaveOccurred())

	privateDataKeys := getPrivateDataKeys(dp, ledgerHeight)

	Expect(privateDataKeys).To(ConsistOf(expectedPrivateDataKeys))
}

func getPrivateDataKeys(client pb.Deliver_DeliverWithPrivateDataClient, ledgerHeight int) []string {
	pvtKeys := make(map[string]struct{})
	blockCount := 0

	for {
		msg, err := client.Recv()
		Expect(err).NotTo(HaveOccurred())

		switch t := msg.Type.(type) {
		case *pb.DeliverResponse_BlockAndPrivateData:
			for _, txPvtRwset := range t.BlockAndPrivateData.PrivateDataMap {
				if txPvtRwset == nil {
					continue
				}

				for _, nsPvtRwset := range txPvtRwset.NsPvtRwset {
					if nsPvtRwset.Namespace != "marblesp" {
						continue
					}

					for _, col := range nsPvtRwset.CollectionPvtRwset {
						Expect(col.CollectionName).Should(SatisfyAny(
							Equal("collectionMarbles"),
							Equal("collectionMarblePrivateDetails")))

						kvRwset := kvrwset.KVRWSet{}
						err := proto.Unmarshal(col.GetRwset(), &kvRwset)
						Expect(err).NotTo(HaveOccurred())
						for _, kvWrite := range kvRwset.Writes {
							pvtKeys[kvWrite.Key] = struct{}{}
						}
					}
				}
			}
		}

		blockCount++
		if blockCount == ledgerHeight {
			var keys []string
			for key := range pvtKeys {
				keys = append(keys, key)
			}
			return keys
		}
	}
}

func startPeer(network *nwo.Network, processes map[string]ifrit.Process, runners map[string]*ginkgomon.Runner, peer *nwo.Peer) {
	if _, ok := processes[peer.ID()]; !ok {
		r := network.PeerRunner(peer)
		p := ifrit.Invoke(r)
		runners[peer.ID()] = r
		processes[peer.ID()] = p
		Eventually(p.Ready(), network.EventuallyTimeout).Should(BeClosed())
	}
}

func stopPeer(network *nwo.Network, processes map[string]ifrit.Process, peer *nwo.Peer) {
	id := peer.ID()
	if p, ok := processes[peer.ID()]; ok {
		p.Signal(syscall.SIGTERM)
		Eventually(p.Wait(), network.EventuallyTimeout).Should(Receive())
		delete(processes, id)
	}
}

func startNewPeer(network *nwo.Network, orderer *nwo.Orderer, peer *nwo.Peer, ledgerHeight int, processes map[string]ifrit.Process, runners map[string]*ginkgomon.Runner) {
	startPeer(network, processes, runners, peer)

	network.JoinChannel(channelID, orderer, peer)
	sess, err := network.PeerAdminSession(
		peer,
		commands.ChannelFetch{
			Block:      "newest",
			ChannelID:  channelID,
			Orderer:    network.OrdererAddress(orderer, nwo.ListenPort),
			OutputFile: filepath.Join(network.RootDir, "newest_block.pb"),
		},
	)
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess.Err).To(gbytes.Say(fmt.Sprintf("Received block: %d", ledgerHeight-1)))

	network.Peers = append(network.Peers, peer)
	nwo.WaitUntilEqualLedgerHeight(network, channelID, ledgerHeight-1, network.Peers...)
}
