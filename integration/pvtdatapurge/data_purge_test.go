/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtdatapurge

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
	"github.com/hyperledger/fabric/integration/channelparticipation"
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

const channelID = "testchannel"

var _ = Describe("Pvtdata purge", func() {
	var (
		config                                     *nwo.Config
		applicationCapabilitiesVersion             string
		testDir                                    string
		network                                    *nwo.Network
		orderer                                    *nwo.Orderer
		org1Peer0, org2Peer0, org3Peer0, org3Peer1 *nwo.Peer
		processes                                  map[string]ifrit.Process
		peerRunners                                map[string]*ginkgomon.Runner
		cancel                                     context.CancelFunc
		chaincode                                  *nwo.Chaincode
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
		processes[orderer.ID()] = process

		for _, peer := range network.Peers {
			startPeer(network, processes, peerRunners, peer)
		}

		channelparticipation.JoinOrdererJoinPeersAppChannel(network, channelID, orderer, ordererRunner)

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

		org1Peer0 = network.Peer("Org1", "peer0")
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
			config = nwo.ThreeOrgEtcdRaftNoSysChan()
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
			config = nwo.ThreeOrgEtcdRaftNoSysChan()
			config.Profiles[0].Blocks = &nwo.Blocks{
				BatchTimeout:      1,
				MaxMessageCount:   30,
				AbsoluteMaxBytes:  99,
				PreferredMaxBytes: 512,
			}
			applicationCapabilitiesVersion = "V2_5"
		})

		It("should prevent purged data being included in responses after the purge transaction has been committed", func() {
			By("Adding marbles and confirming they exist in private state")
			marblechaincodeutil.AddMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-1", "color":"green", "size":42, "owner":"simon", "price":180}`, org2Peer0)
			marblechaincodeutil.AddMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-2", "color":"red", "size":24, "owner":"heather", "price":635}`, org2Peer0)
			marblechaincodeutil.AddMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-3", "color":"black", "size":12, "owner":"bob", "price":2}`, org2Peer0)
			marblechaincodeutil.AddMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-4", "color":"yellow", "size":180, "owner":"liz", "price":100}`, org2Peer0)

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

			By("Purging 2 marbles in separate blocks and confirming they are longer exist in private state")
			marblechaincodeutil.PurgeMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-0"}`, org2Peer0)
			marblechaincodeutil.PurgeMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-1"}`, org2Peer0)

			marblechaincodeutil.AssertDoesNotExistInCollectionM(network, channelID, chaincode.Name, "test-marble-0", org2Peer0)
			marblechaincodeutil.AssertDoesNotExistInCollectionMPD(network, channelID, chaincode.Name, "test-marble-0", org2Peer0)
			marblechaincodeutil.AssertDoesNotExistInCollectionM(network, channelID, chaincode.Name, "test-marble-1", org2Peer0)
			marblechaincodeutil.AssertDoesNotExistInCollectionMPD(network, channelID, chaincode.Name, "test-marble-1", org2Peer0)

			By("Purging 3 more marbles in a single block and confirming they are longer exist in private state")
			var wg sync.WaitGroup
			wg.Add(3)
			for i := 2; i < 5; i++ {
				go func(marblePurge string) {
					defer GinkgoRecover()
					defer wg.Done()
					marblechaincodeutil.PurgeMarble(network, orderer, channelID, chaincode.Name, marblePurge, org2Peer0)
				}(fmt.Sprintf(`{"name":"test-marble-%d"}`, i))
			}
			wg.Wait()
			Expect(nwo.GetLedgerHeight(network, org2Peer0, channelID)).To(Equal(14)) // Anchor peers are in genesis block

			marblechaincodeutil.AssertDoesNotExistInCollectionM(network, channelID, chaincode.Name, "test-marble-2", org2Peer0)
			marblechaincodeutil.AssertDoesNotExistInCollectionMPD(network, channelID, chaincode.Name, "test-marble-2", org2Peer0)
			marblechaincodeutil.AssertDoesNotExistInCollectionM(network, channelID, chaincode.Name, "test-marble-3", org2Peer0)
			marblechaincodeutil.AssertDoesNotExistInCollectionMPD(network, channelID, chaincode.Name, "test-marble-3", org2Peer0)
			marblechaincodeutil.AssertDoesNotExistInCollectionM(network, channelID, chaincode.Name, "test-marble-4", org2Peer0)
			marblechaincodeutil.AssertDoesNotExistInCollectionMPD(network, channelID, chaincode.Name, "test-marble-4", org2Peer0)

			By("Adding 2 new marbles and confirming only the new marbles exist in block events")
			marblechaincodeutil.AddMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-5", "color":"black", "size":12, "owner":"bob", "price":2}`, org2Peer0)
			marblechaincodeutil.AddMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-6", "color":"yellow", "size":180, "owner":"liz", "price":100}`, org2Peer0)

			assertBlockEventsOnlyContainUnpurgedPrivateData(network, org2Peer0, chaincode.Name, []string{"test-marble-5", "\x00color~name\x00black\x00test-marble-5\x00", "test-marble-6", "\x00color~name\x00yellow\x00test-marble-6\x00"})

			By("Adding a marble and setting state-basedendorsement policy to Org3, future endorsements from Org2 will be invalidated")
			marblechaincodeutil.AddMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-7-sbe", "color":"black", "size":12, "owner":"bob", "price":2}`, org2Peer0)
			marblechaincodeutil.SetMarblePolicy(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-7-sbe","org":"Org3"}`, org2Peer0)

			marblePurgeBase64 := base64.StdEncoding.EncodeToString([]byte(`{"name":"test-marble-7-sbe"}`))

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

			marblechaincodeutil.AssertPresentInCollectionM(network, channelID, chaincode.Name, "test-marble-7-sbe", org2Peer0)
			marblechaincodeutil.AssertPresentInCollectionMPD(network, channelID, chaincode.Name, "test-marble-7-sbe", org2Peer0)

			By("Starting a new peer, it should pull private data from existing peers without errors or warnings")
			process := addPeer(network, orderer, org3Peer1)
			processes[org3Peer1.ID()] = process

			nwo.PackageAndInstallChaincode(network, *chaincode, org3Peer1)
			network.VerifyMembership(network.Peers, channelID, chaincode.Name)

			marblechaincodeutil.AssertDoesNotExistInCollectionM(network, channelID, chaincode.Name, `test-marble-4`, org3Peer1)
			marblechaincodeutil.AssertDoesNotExistInCollectionMPD(network, channelID, chaincode.Name, `test-marble-4`, org3Peer1)
			marblechaincodeutil.AssertPresentInCollectionM(network, channelID, chaincode.Name, `test-marble-5`, org3Peer1)
			marblechaincodeutil.AssertPresentInCollectionMPD(network, channelID, chaincode.Name, `test-marble-5`, org3Peer1)
			marblechaincodeutil.AssertPresentInCollectionM(network, channelID, chaincode.Name, `test-marble-6`, org3Peer1)
			marblechaincodeutil.AssertPresentInCollectionMPD(network, channelID, chaincode.Name, `test-marble-6`, org3Peer1)

			By("Adding data with new peer down, and then restarting new peer with other peers down to test reconciliation")

			// stop new peer
			stopPeer(network, processes, org3Peer1)
			network.Peers = []*nwo.Peer{org1Peer0, org2Peer0, org3Peer0}
			network.VerifyMembership(network.Peers, channelID)

			// add two marbles and purge one
			marblechaincodeutil.AddMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-8-keep", "color":"green", "size":42, "owner":"simon", "price":180}`, org2Peer0)
			marblechaincodeutil.AddMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-9-purge", "color":"red", "size":24, "owner":"heather", "price":635}`, org2Peer0)
			marblechaincodeutil.PurgeMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-9-purge"}`, org2Peer0)

			// stop original peers
			stopPeer(network, processes, org1Peer0)
			stopPeer(network, processes, org2Peer0)
			stopPeer(network, processes, org3Peer0)
			network.Peers = []*nwo.Peer{}

			// restart new peer by itself and wait for "Could not fetch" message
			startPeer(network, processes, peerRunners, org3Peer1)
			network.Peers = append(network.Peers, org3Peer1)
			runner := peerRunners[org3Peer1.ID()]
			Eventually(runner.Err(), network.EventuallyTimeout).Should(gbytes.Say("Could not fetch \\(or mark to reconcile later\\) 2 eligible collection private write sets for block \\[\\d+\\] \\(0 from local cache, 0 from transient store, 0 from other peers\\)\\. Will commit block with missing private write sets:\\[txID: [0123456789abcdef]+, seq: \\d+, namespace: marblesp, collection: collectionMarblePrivateDetails"))

			// restart original peers
			startPeer(network, processes, peerRunners, org1Peer0)
			network.Peers = append(network.Peers, org1Peer0)

			startPeer(network, processes, peerRunners, org2Peer0)
			network.Peers = append(network.Peers, org2Peer0)

			startPeer(network, processes, peerRunners, org3Peer0)
			network.Peers = append(network.Peers, org3Peer0)

			// Wait for peers to connect and reconciliation on org3Peer1
			Eventually(runner.Err(), network.EventuallyTimeout).Should(gbytes.Say("Reconciliation cycle finished successfully"))

			// We can't rely on a single log message about successful reconciliation, because sometimes the reconciliation completes in one cycle (5s) and sometimes in two cycles (10s), so wait for another potential cycle
			// Although sleep is generally not a good practice in tests, in this case we can't rely on the Eventually in the query for the private data,
			// because prior to reconciliation the query will return an error "private data matching public hash version is not available" and will not get re-tried in the Eventually.
			time.Sleep(10 * time.Second)

			marblechaincodeutil.AssertPresentInCollectionM(network, channelID, chaincode.Name, `test-marble-8-keep`, org3Peer1)
			marblechaincodeutil.AssertPresentInCollectionMPD(network, channelID, chaincode.Name, `test-marble-8-keep`, org3Peer1)
			marblechaincodeutil.AssertDoesNotExistInCollectionM(network, channelID, chaincode.Name, `test-marble-9-purge`, org3Peer1)
			marblechaincodeutil.AssertDoesNotExistInCollectionMPD(network, channelID, chaincode.Name, `test-marble-9-purge`, org3Peer1)

			By("Adding two marbles, removing Org3 from the collection, purging the marbles, and confirming that they still got purged from Org3")

			// Make sure all peers are up and connected after prior test, before continuing with this test
			network.VerifyMembership(network.Peers, channelID)

			marblechaincodeutil.AddMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-10-purge-after-ineligible", "color":"white", "size":4, "owner":"liz", "price":4}`, org2Peer0)
			marblechaincodeutil.AddMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-11-purge-after-ineligible", "color":"orange", "size":80, "owner":"clive", "price":88}`, org2Peer0)

			marblechaincodeutil.AssertPresentInCollectionM(network, channelID, chaincode.Name, "test-marble-10-purge-after-ineligible", org3Peer0)
			marblechaincodeutil.AssertPresentInCollectionMPD(network, channelID, chaincode.Name, "test-marble-10-purge-after-ineligible", org3Peer0)
			marblechaincodeutil.AssertPresentInCollectionM(network, channelID, chaincode.Name, "test-marble-11-purge-after-ineligible", org3Peer0)
			marblechaincodeutil.AssertPresentInCollectionMPD(network, channelID, chaincode.Name, "test-marble-11-purge-after-ineligible", org3Peer0)

			chaincode.Version = "1.1"
			chaincode.CollectionsConfig = CollectionConfig("remove_org3_config.json")
			chaincode.Sequence = "2"
			nwo.DeployChaincode(network, channelID, orderer, *chaincode)

			marblechaincodeutil.PurgeMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-10-purge-after-ineligible"}`, org2Peer0)
			marblechaincodeutil.PurgeMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-11-purge-after-ineligible"}`, org2Peer0)

			marblechaincodeutil.AssertDoesNotExistInCollectionM(network, channelID, chaincode.Name, "test-marble-10-purge-after-ineligible", org3Peer0)
			marblechaincodeutil.AssertDoesNotExistInCollectionMPD(network, channelID, chaincode.Name, "test-marble-10-purge-after-ineligible", org3Peer0)
			marblechaincodeutil.AssertDoesNotExistInCollectionM(network, channelID, chaincode.Name, "test-marble-11-purge-after-ineligible", org3Peer0)
			marblechaincodeutil.AssertDoesNotExistInCollectionMPD(network, channelID, chaincode.Name, "test-marble-11-purge-after-ineligible", org3Peer0)

			runner = peerRunners[org3Peer0.ID()]
			Eventually(runner.Err(), network.EventuallyTimeout).Should(gbytes.Say("Purging private data from private data storage channel=testchannel chaincode=marblesp collection=collectionMarblePrivateDetails key=test-marble-10-purge-after-ineligible blockNum=\\d+ tranNum=\\d+"))
			Eventually(runner.Err(), network.EventuallyTimeout).Should(gbytes.Say("Purging private data from private data storage channel=testchannel chaincode=marblesp collection=collectionMarblePrivateDetails key=test-marble-11-purge-after-ineligible blockNum=\\d+ tranNum=\\d+"))
			Eventually(runner.Err(), network.EventuallyTimeout).Should(gbytes.Say("Purged private data from private data storage channel=testchannel numKeysPurged=\\d+ numPrivateDataStoreRecordsPurged=\\d+"))

			By("Adding a marble, Purging a marble, and then Re-Adding the same marble should be possible")
			marblechaincodeutil.AddMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-12-re-add", "color":"black", "size":12, "owner":"bob", "price":2}`, org2Peer0)

			marblechaincodeutil.AssertPresentInCollectionM(network, channelID, chaincode.Name, "test-marble-12-re-add", org2Peer0)
			marblechaincodeutil.AssertPresentInCollectionMPD(network, channelID, chaincode.Name, "test-marble-12-re-add", org2Peer0)

			marblechaincodeutil.PurgeMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-12-re-add"}`, org2Peer0)

			marblechaincodeutil.AssertDoesNotExistInCollectionM(network, channelID, chaincode.Name, "test-marble-12-re-add", org2Peer0)
			marblechaincodeutil.AssertDoesNotExistInCollectionMPD(network, channelID, chaincode.Name, "test-marble-12-re-add", org2Peer0)

			marblechaincodeutil.AddMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-12-re-add", "color":"black", "size":12, "owner":"bob", "price":2}`, org2Peer0)

			marblechaincodeutil.AssertPresentInCollectionM(network, channelID, chaincode.Name, "test-marble-12-re-add", org2Peer0)
			marblechaincodeutil.AssertPresentInCollectionMPD(network, channelID, chaincode.Name, "test-marble-12-re-add", org2Peer0)
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

func addPeer(n *nwo.Network, orderer *nwo.Orderer, peer *nwo.Peer) ifrit.Process {
	process := ifrit.Invoke(n.PeerRunner(peer))
	Eventually(process.Ready(), n.EventuallyTimeout).Should(BeClosed())

	n.JoinChannel(channelID, orderer, peer)
	ledgerHeight := nwo.GetLedgerHeight(n, n.Peers[0], channelID)
	sess, err := n.PeerAdminSession(
		peer,
		commands.ChannelFetch{
			Block:      "newest",
			ChannelID:  channelID,
			Orderer:    n.OrdererAddress(orderer, nwo.ListenPort),
			OutputFile: filepath.Join(n.RootDir, "newest_block.pb"),
		},
	)
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess.Err).To(gbytes.Say(fmt.Sprintf("Received block: %d", ledgerHeight-1)))

	n.Peers = append(n.Peers, peer)
	nwo.WaitUntilEqualLedgerHeight(n, channelID, nwo.GetLedgerHeight(n, n.Peers[0], channelID), n.Peers...)

	return process
}
