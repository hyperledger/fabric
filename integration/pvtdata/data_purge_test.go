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
	"syscall"

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
	"github.com/tedsuo/ifrit"
)

var _ = Describe("Pvtdata purge", func() {
	var (
		applicationCapabilitiesVersion string
		testDir                        string
		network                        *nwo.Network
		orderer                        *nwo.Orderer
		org2Peer0                      *nwo.Peer
		process                        ifrit.Process
		cancel                         context.CancelFunc
		chaincode                      *nwo.Chaincode
	)

	JustBeforeEach(func() {
		var err error
		testDir, err = ioutil.TempDir("", "purgedata")
		Expect(err).NotTo(HaveOccurred())

		config := nwo.ThreeOrgRaft()
		network = nwo.New(config, testDir, nil, StartPort(), components)

		network.GenerateConfigTree()
		network.Bootstrap()

		networkRunner := network.NetworkGroupRunner()
		process = ifrit.Invoke(networkRunner)
		Eventually(process.Ready(), network.EventuallyTimeout).Should(BeClosed())

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
			CollectionsConfig: CollectionConfig("collections_config1.json"),
			Sequence:          "1",
			InitRequired:      false,
			Label:             "purgecc_label",
		}

		nwo.DeployChaincode(network, channelID, orderer, *chaincode)

		org2Peer0 = network.Peer("Org2", "peer0")

		_, cancel = context.WithTimeout(context.Background(), network.EventuallyTimeout)

		marblechaincodeutil.AddMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-0", "color":"blue", "size":35, "owner":"tom", "price":99}`, org2Peer0)
	})

	AfterEach(func() {
		cancel()

		if process != nil {
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
			applicationCapabilitiesVersion = "V2_5"
		})

		It("should prevent purged data being included in responses after the purge transaction has been committed", func() {
			marblechaincodeutil.AssertPresentInCollectionM(network, channelID, chaincode.Name, `test-marble-0`, org2Peer0)
			marblechaincodeutil.AssertPresentInCollectionMPD(network, channelID, chaincode.Name, "test-marble-0", org2Peer0)

			marblechaincodeutil.PurgeMarble(network, orderer, channelID, chaincode.Name, `{"name":"test-marble-0"}`, org2Peer0)

			marblechaincodeutil.AssertDoesNotExistInCollectionM(network, channelID, chaincode.Name, `test-marble-0`, org2Peer0)
			marblechaincodeutil.AssertDoesNotExistInCollectionMPD(network, channelID, chaincode.Name, `test-marble-0`, org2Peer0)
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

		// 1. User is able to submit a purge transaction that involves more than one keys
		PIt("should accept multiple keys for purging in the same transaction")

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

		// 3. Data is purged on an eligible peer
		//    - Add a few keys into a collection
		//    - Issue a purge transaction for some of the keys
		//    - Verify that all the versions of the intended keys are purged while the remaining keys still exist
		//    - Repeat above to purge all keys to test the corner case
		PIt("should remove all purged data from an eligible peer")

		// 4.	Data is purged on previously eligible but now ineligible peer
		//    - Add a few keys into a collection
		//    - Submit a collection config update to remove an org
		//    - Issue a purge transaction to delete few keys
		//    - The removed orgs peer should have purged the historical versions of intended key
		PIt("should remove all purged data from a previously eligible peer")

		// 5. A new peer able to reconcile from a purged peer
		//    - Stop one of the peers of an eligible org
		//    - Add a few keys into a collection
		//    - Issue a purge transaction for some of the keys
		//    - Start the stopped peer and the peer should reconcile the partial available data
		PIt("should enable successful peer reconciliation with partial write-sets")

		// 7. Further writes to private data after a purge operation are not purged
		//    - Add a few keys into a collection
		//    - Issue a purge transaction
		//    - Add the purged data back
		//    - The subsequently added data should not be purged as a
		//      side-effect of the previous purge operation
		PIt("should not remove new data after a previous purge operation")
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
