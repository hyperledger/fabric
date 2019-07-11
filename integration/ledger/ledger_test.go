/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package e2e

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
	"gopkg.in/yaml.v2"
)

var _ bool = Describe("Rollback & Reset Ledger", func() {
	// at the beginning of each test under this block, we have defined two collections:
	// 1. collectionMarbles - Org1 and Org2 have access to this collection
	// 2. collectionMarblePrivateDetails - Org2 and Org3 have access to this collection
	// when calling QueryChaincode with first arg "readMarble", it will query collectionMarbles[1]
	// when calling QueryChaincode with first arg "readMarblePrivateDetails", it will query collectionMarblePrivateDetails[2]
	var (
		setup  *setup
		helper *testHelper
	)

	BeforeEach(func() {
		setup = initThreeOrgsSetup()
		helper = &testHelper{
			networkHelper: &networkHelper{
				Network:   setup.network,
				orderer:   setup.orderer,
				peers:     setup.peers,
				testDir:   setup.testDir,
				channelID: "testchannel",
			},
		}

		By("installing and instantiating chaincode on all peers")
		chaincode := nwo.Chaincode{
			Name:              "marblesp",
			Version:           "1.0",
			Path:              "github.com/hyperledger/fabric/integration/chaincode/marbles_private/cmd",
			Ctor:              `{"Args":["init"]}`,
			Policy:            `OR ('Org1MSP.member','Org2MSP.member', 'Org3MSP.member')`,
			CollectionsConfig: filepath.Join("testdata", "collection_configs", "collections_config1.json")}
		helper.deployChaincode(chaincode)

		org2peer0 := setup.network.Peer("org2", "peer0")
		height := helper.getLedgerHeight(org2peer0)

		By("creating 5 blocks")
		for i := 1; i <= 5; i++ {
			helper.addMarble("marblesp", fmt.Sprintf(`"marble%d", "blue", "35", "tom", "99"`, i), org2peer0)
			helper.waitUntilEqualLedgerHeight(height + i)
		}

		By("verifying marble1 to marble5 exist in collectionMarbles & collectionMarblePrivateDetails in peer0.org2")
		for i := 1; i <= 5; i++ {
			helper.assertPresentInCollectionM("marblesp", fmt.Sprintf("marble%d", i), org2peer0)
			helper.assertPresentInCollectionMPD("marblesp", fmt.Sprintf("marble%d", i), org2peer0)
		}

	})

	AfterEach(func() {
		setup.cleanup()
	})

	assertPostRollbackOrReset := func() {
		org2peer0 := setup.network.Peer("org2", "peer0")

		By("verifying that the endorsement is disabled")
		setup.startPeer(org2peer0)
		helper.assertDisabledEndorser("marblesp", org2peer0)

		By("bringing the peer0.org2 to recent height by starting the orderer")
		setup.startBrokerAndOrderer()
		helper.waitUntilEndorserEnabled(org2peer0)

		By("verifying marble1 to marble5 exist in collectionMarbles & collectionMarblePrivateDetails in peer0.org2")
		for i := 1; i <= 5; i++ {
			helper.assertPresentInCollectionM("marblesp", fmt.Sprintf("marble%d", i), org2peer0)
			helper.assertPresentInCollectionMPD("marblesp", fmt.Sprintf("marble%d", i), org2peer0)
		}

		By("starting org1 and org3 peer")
		setup.startPeer(setup.peers[0])
		setup.startPeer(setup.peers[2])
		setup.verifyMembership("marblesp")

		By("creating 2 more blocks")
		for i := 6; i <= 7; i++ {
			helper.addMarble("marblesp", fmt.Sprintf(`"marble%d", "blue", "35", "tom", "99"`, i), org2peer0)
			helper.waitUntilEqualLedgerHeight(10 + i - 5)
		}

		By("verifying marble1 to marble7 exist in collectionMarbles & collectionMarblePrivateDetails in peer0.org2")
		for i := 1; i <= 7; i++ {
			helper.assertPresentInCollectionM("marblesp", fmt.Sprintf("marble%d", i), org2peer0)
			helper.assertPresentInCollectionMPD("marblesp", fmt.Sprintf("marble%d", i), org2peer0)
		}

	}

	It("rolls back the ledger to a past block", func() {
		org2peer0 := setup.network.Peer("org2", "peer0")
		// block 0: genesis, block 1: org1Anchor, block 2: org2Anchor, block 3: org3Anchor
		// block 4: chaincode instantiation, block 5 to 9: chaincode invoke to add marbles.
		Expect(helper.getLedgerHeight(org2peer0)).Should(Equal(10))

		By("Rolling back peer0.org2 to block 3 from block 9 while the peer node is online")
		expectedErrMessage := "as another peer node command is executing," +
			" wait for that command to complete its execution or terminate it before retrying"
		helper.rollback(org2peer0, 3, expectedErrMessage, false)

		By("Rolling back peer0.org2 to block 3 from block 9 while the peer node is offline")
		setup.terminateAllProcess()
		helper.rollback(org2peer0, 3, "", true)

		assertPostRollbackOrReset()
	})

	It("resets the ledger to the genesis block", func() {
		org2peer0 := setup.network.Peer("org2", "peer0")
		Expect(helper.getLedgerHeight(org2peer0)).Should(Equal(10))

		By("Resetting peer0.org2 to the genesis block while the peer node is online")
		expectedErrMessage := "as another peer node command is executing," +
			" wait for that command to complete its execution or terminate it before retrying"
		helper.reset(org2peer0, expectedErrMessage, false)

		By("Resetting peer0.org2 to the genesis block while the peer node is offline")
		setup.terminateAllProcess()
		helper.reset(org2peer0, "", true)

		assertPostRollbackOrReset()
	})

})

type setup struct {
	testDir        string
	channelID      string
	network        *nwo.Network
	peers          []*nwo.Peer
	peerProcess    []ifrit.Process
	orderer        *nwo.Orderer
	ordererProcess ifrit.Process
	brokerProcess  ifrit.Process
}

func initThreeOrgsSetup() *setup {
	var err error
	testDir, err := ioutil.TempDir("", "reset-rollback")
	Expect(err).NotTo(HaveOccurred())

	client, err := docker.NewClientFromEnv()
	Expect(err).NotTo(HaveOccurred())

	configBytes, err := ioutil.ReadFile(filepath.Join("testdata", "network.yaml"))
	Expect(err).NotTo(HaveOccurred())

	var networkConfig *nwo.Config
	err = yaml.Unmarshal(configBytes, &networkConfig)
	Expect(err).NotTo(HaveOccurred())

	n := nwo.New(networkConfig, testDir, client, StartPort(), components)
	n.GenerateConfigTree()
	n.Bootstrap()

	peers := []*nwo.Peer{
		n.Peer("org1", "peer0"),
		n.Peer("org2", "peer0"),
		n.Peer("org3", "peer0"),
	}

	setup := &setup{
		testDir:   testDir,
		network:   n,
		peers:     peers,
		channelID: "testchannel",
	}

	setup.startBrokerAndOrderer()

	setup.startPeer(peers[0])
	setup.startPeer(peers[1])
	setup.startPeer(peers[2])

	orderer := n.Orderer("orderer")
	n.CreateAndJoinChannel(orderer, "testchannel")
	n.UpdateChannelAnchors(orderer, "testchannel")
	setup.orderer = orderer

	By("verifying membership")
	setup.verifyMembership()

	return setup
}

func (s *setup) cleanup() {
	s.terminateAllProcess()
	s.network.Cleanup()
	os.RemoveAll(s.testDir)
}

func (s *setup) terminateAllProcess() {
	s.ordererProcess.Signal(syscall.SIGTERM)
	Eventually(s.ordererProcess.Wait(), s.network.EventuallyTimeout).Should(Receive())
	s.ordererProcess = nil

	s.brokerProcess.Signal(syscall.SIGTERM)
	Eventually(s.brokerProcess.Wait(), s.network.EventuallyTimeout).Should(Receive())
	s.brokerProcess = nil

	for _, p := range s.peerProcess {
		p.Signal(syscall.SIGTERM)
		Eventually(p.Wait(), s.network.EventuallyTimeout).Should(Receive())
	}
	s.peerProcess = nil
}

func (s *setup) startPeer(peer *nwo.Peer) {
	peerRunner := s.network.PeerRunner(peer)
	peerProcess := ifrit.Invoke(peerRunner)
	Eventually(peerProcess.Ready(), s.network.EventuallyTimeout).Should(BeClosed())
	s.peerProcess = append(s.peerProcess, peerProcess)
}

func (s *setup) startBrokerAndOrderer() {
	brokerRunner := s.network.BrokerGroupRunner()
	brokerProcess := ifrit.Invoke(brokerRunner)
	Eventually(brokerProcess.Ready(), s.network.EventuallyTimeout).Should(BeClosed())
	s.brokerProcess = brokerProcess

	ordererRunner := s.network.OrdererGroupRunner()
	ordererProcess := ifrit.Invoke(ordererRunner)
	Eventually(ordererProcess.Ready(), s.network.EventuallyTimeout).Should(BeClosed())
	s.ordererProcess = ordererProcess
}

// this function checks that each peer discovered all other peers in the network
func (s *setup) verifyMembership(chaincode ...string) {
	expectedDiscoveredPeers := make([]nwo.DiscoveredPeer, 0, len(s.peers))
	for _, peer := range s.peers {
		expectedDiscoveredPeers = append(expectedDiscoveredPeers, s.network.DiscoveredPeer(peer, chaincode...))
	}
	for _, peer := range s.peers {
		Eventually(nwo.DiscoverPeers(s.network, peer, "User1", s.channelID), s.network.EventuallyTimeout).Should(ConsistOf(expectedDiscoveredPeers))
	}
}

type networkHelper struct {
	*nwo.Network
	orderer   *nwo.Orderer
	peers     []*nwo.Peer
	channelID string
	testDir   string
}

func (nh *networkHelper) deployChaincode(chaincode nwo.Chaincode) {
	nwo.DeployChaincode(nh.Network, nh.channelID, nh.orderer, chaincode)
	nh.waitUntilEqualLedgerHeight(nh.getLedgerHeight(nh.peers[0]))
}

func (nh *networkHelper) waitUntilEqualLedgerHeight(height int) {
	for _, peer := range nh.peers {
		Eventually(func() int {
			return nh.getLedgerHeight(peer)
		}, nh.EventuallyTimeout).Should(Equal(height))
	}
}

func (nh *networkHelper) getLedgerHeight(peer *nwo.Peer) int {
	sess, err := nh.PeerUserSession(peer, "User1", commands.ChannelInfo{
		ChannelID: nh.channelID,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, nh.EventuallyTimeout).Should(gexec.Exit(0))

	channelInfoStr := strings.TrimPrefix(string(sess.Buffer().Contents()[:]), "Blockchain info:")
	var channelInfo = common.BlockchainInfo{}
	json.Unmarshal([]byte(channelInfoStr), &channelInfo)
	return int(channelInfo.Height)
}

func (nh *networkHelper) queryChaincode(peer *nwo.Peer, command commands.ChaincodeQuery, expectedMessage string, expectSuccess bool) {
	sess, err := nh.PeerUserSession(peer, "User1", command)
	Expect(err).NotTo(HaveOccurred())
	if expectSuccess {
		Eventually(sess, nh.EventuallyTimeout).Should(gexec.Exit(0))
		Expect(sess).To(gbytes.Say(expectedMessage))
	} else {
		Eventually(sess, nh.EventuallyTimeout).Should(gexec.Exit())
		Expect(sess.Err).To(gbytes.Say(expectedMessage))
	}
}

func (nh *networkHelper) invokeChaincode(peer *nwo.Peer, command commands.ChaincodeInvoke) {
	sess, err := nh.PeerUserSession(peer, "User1", command)
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, nh.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess.Err).To(gbytes.Say("Chaincode invoke successful."))
}

func (nh *networkHelper) rollback(peer *nwo.Peer, blockNumber int, expectedErrMessage string, expectSuccess bool) {
	rollbackCmd := commands.NodeRollback{ChannelID: nh.channelID, BlockNumber: blockNumber}
	sess, err := nh.PeerUserSession(peer, "User1", rollbackCmd)
	Expect(err).NotTo(HaveOccurred())
	if expectSuccess {
		Eventually(sess, nh.EventuallyTimeout).Should(gexec.Exit(0))
	} else {
		Eventually(sess, nh.EventuallyTimeout).Should(gexec.Exit())
		Expect(sess.Err).To(gbytes.Say(expectedErrMessage))
	}
}

func (nh *networkHelper) reset(peer *nwo.Peer, expectedErrMessage string, expectSuccess bool) {
	resetCmd := commands.NodeReset{}
	sess, err := nh.PeerUserSession(peer, "User1", resetCmd)
	Expect(err).NotTo(HaveOccurred())
	if expectSuccess {
		Eventually(sess, nh.EventuallyTimeout).Should(gexec.Exit(0))
	} else {
		Eventually(sess, nh.EventuallyTimeout).Should(gexec.Exit())
		Expect(sess.Err).To(gbytes.Say(expectedErrMessage))
	}
}

func (nh *networkHelper) waitUntilEndorserEnabled(peer *nwo.Peer) {
	Eventually(func() *gbytes.Buffer {
		sess, err := nh.PeerUserSession(peer, "User1", commands.ChannelInfo{
			ChannelID: nh.channelID,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, nh.EventuallyTimeout).Should(gexec.Exit())
		return sess.Buffer()
	}, "60s", "2s").Should(gbytes.Say("Blockchain info"))
}

type testHelper struct {
	*networkHelper
}

func (th *testHelper) addMarble(chaincodeName, marbleDetails string, peer *nwo.Peer) {
	command := commands.ChaincodeInvoke{
		ChannelID: th.channelID,
		Orderer:   th.OrdererAddress(th.orderer, nwo.ListenPort),
		Name:      chaincodeName,
		Ctor:      fmt.Sprintf(`{"Args":["initMarble",%s]}`, marbleDetails),
		PeerAddresses: []string{
			th.PeerAddress(peer, nwo.ListenPort),
		},
		WaitForEvent: true,
	}
	th.invokeChaincode(peer, command)
}

// assertPresentInCollectionM asserts that the private data for given marble is present in collection
// 'readMarble' at the given peers
func (th *testHelper) assertPresentInCollectionM(chaincodeName, marbleName string, peerList ...*nwo.Peer) {
	command := commands.ChaincodeQuery{
		ChannelID: th.channelID,
		Name:      chaincodeName,
		Ctor:      fmt.Sprintf(`{"Args":["readMarble","%s"]}`, marbleName),
	}
	expectedMsg := fmt.Sprintf(`{"docType":"marble","name":"%s"`, marbleName)
	for _, peer := range peerList {
		th.queryChaincode(peer, command, expectedMsg, true)
	}
}

// assertPresentInCollectionMPD asserts that the private data for given marble is present
// in collection 'readMarblePrivateDetails' at the given peers
func (th *testHelper) assertPresentInCollectionMPD(chaincodeName, marbleName string, peerList ...*nwo.Peer) {
	command := commands.ChaincodeQuery{
		ChannelID: th.channelID,
		Name:      chaincodeName,
		Ctor:      fmt.Sprintf(`{"Args":["readMarblePrivateDetails","%s"]}`, marbleName),
	}
	expectedMsg := fmt.Sprintf(`{"docType":"marblePrivateDetails","name":"%s"`, marbleName)
	for _, peer := range peerList {
		th.queryChaincode(peer, command, expectedMsg, true)
	}
}

func (th *testHelper) assertDisabledEndorser(chaincodeName string, peer *nwo.Peer) {
	command := commands.ChaincodeQuery{
		ChannelID: th.channelID,
		Name:      chaincodeName,
		Ctor:      `{"Args":["readMarble","marble1"]}`,
	}
	expectedMsg := "endorse requests are blocked while ledgers are being rebuilt"
	th.queryChaincode(peer, command, expectedMsg, false)
}
