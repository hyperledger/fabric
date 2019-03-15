/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package e2e

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	"github.com/hyperledger/fabric/protos/common"
	protosorderer "github.com/hyperledger/fabric/protos/orderer"
	protosraft "github.com/hyperledger/fabric/protos/orderer/etcdraft"
	"github.com/hyperledger/fabric/protoutil"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
)

var _ = Describe("Kafka2RaftMigration", func() {
	var (
		testDir   string
		client    *docker.Client
		network   *nwo.Network
		chaincode nwo.Chaincode

		process                             ifrit.Process
		peerProc, brokerProc                ifrit.Process
		ordererProc, o1Proc, o2Proc, o3Proc ifrit.Process
	)

	BeforeEach(func() {
		var err error
		testDir, err = ioutil.TempDir("", "e2e")
		Expect(err).NotTo(HaveOccurred())

		client, err = docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())

		chaincode = nwo.Chaincode{
			Name:    "mycc",
			Version: "0.0",
			Path:    "github.com/hyperledger/fabric/integration/chaincode/simple/cmd",
			Ctor:    `{"Args":["init","a","100","b","200"]}`,
			Policy:  `AND ('Org1MSP.member','Org2MSP.member')`,
		}
	})

	AfterEach(func() {
		if process != nil {
			process.Signal(syscall.SIGTERM)
			Eventually(process.Wait(), network.EventuallyTimeout).Should(Receive())
		}

		if peerProc != nil {
			peerProc.Signal(syscall.SIGTERM)
			Eventually(peerProc.Wait(), network.EventuallyTimeout).Should(Receive())
		}

		for _, oProc := range []ifrit.Process{ordererProc, o1Proc, o2Proc, o3Proc} {
			if oProc != nil {
				oProc.Signal(syscall.SIGTERM)
				Eventually(oProc.Wait(), network.EventuallyTimeout).Should(Receive())
			}
		}

		if brokerProc != nil {
			brokerProc.Signal(syscall.SIGTERM)
			Eventually(brokerProc.Wait(), network.EventuallyTimeout).Should(Receive())
		}

		if network != nil {
			network.Cleanup()
		}

		os.RemoveAll(testDir)
	})

	// These tests execute the migration config updates on Kafka based system, and verify that these config update
	// have the desired effect. These tests use a single node.
	Describe("kafka2raft migration kafka side", func() {
		BeforeEach(func() {
			network = nwo.New(nwo.BasicKafka(), testDir, client, StartPort(), components)
			network.GenerateConfigTree()
			network.Bootstrap()

			networkRunner := network.NetworkGroupRunner()
			process = ifrit.Invoke(networkRunner)
			Eventually(process.Ready(), network.EventuallyTimeout).Should(BeClosed())
		})

		// This test executes the "green path" migration config updates on Kafka based system
		// with system channel only, and verifies that these config update have the desired effect.
		It("executes kafka2raft system channel green path", func() {
			orderer := network.Orderer("orderer")
			peer := network.Peer("Org1", "peer1")

			syschannel := network.SystemChannel.Name

			//=== No standard channels ===

			//=== Step 1: Config update on system channel, MigrationState=START ===
			By("Step 1: Config update on system channel, MigrationState=START")
			config := nwo.GetConfig(network, peer, orderer, syschannel)
			updatedConfig, consensusTypeValue, err := extractOrdererConsensusType(config)
			Expect(err).NotTo(HaveOccurred())
			//Verify initial ConsensusType value
			validateConsensusTypeValue(consensusTypeValue, "kafka", protosorderer.ConsensusType_MIG_STATE_NONE, 0)
			//Change ConsensusType value
			updateConfigWithConsensusType("kafka", []byte{}, protosorderer.ConsensusType_MIG_STATE_START, 0,
				updatedConfig, consensusTypeValue)
			nwo.UpdateOrdererConfig(network, orderer, syschannel, config, updatedConfig, peer, orderer)

			By("Step 1, Verify: system channel config changed, get migration context")
			migrationContext := nwo.CurrentConfigBlockNumber(network, peer, orderer, syschannel)
			Expect(migrationContext).To(BeNumerically(">", 0))

			By("Step 1, Verify: new channels cannot be created")
			channel := "testchannel"
			network.CreateChannelFail(orderer, channel)

			//=== Step 2: Config update on system channel, MigrationState=COMMIT, type=etcdraft ===
			By("Step 2: Config update on system channel, MigrationState=COMMIT, type=etcdraft")
			config = nwo.GetConfig(network, peer, orderer, syschannel)
			updatedConfig, consensusTypeValue, err = extractOrdererConsensusType(config)
			Expect(err).NotTo(HaveOccurred())

			//Verify previous state
			validateConsensusTypeValue(consensusTypeValue, "kafka", protosorderer.ConsensusType_MIG_STATE_START, 0)

			//Change ConsensusType value
			raftMetadata := prepareRaftMetadata(network)
			updateConfigWithConsensusType("etcdraft", raftMetadata, protosorderer.ConsensusType_MIG_STATE_COMMIT, migrationContext,
				updatedConfig, consensusTypeValue)
			nwo.UpdateOrdererConfig(network, orderer, syschannel, config, updatedConfig, peer, orderer)
			sysBlockNum := nwo.CurrentConfigBlockNumber(network, peer, orderer, syschannel)
			Expect(sysBlockNum).To(Equal(migrationContext + 1))

			By("Step 2, Verify: system channel config change, OSN ready to restart")
			config = nwo.GetConfig(network, peer, orderer, syschannel)
			updatedConfig, consensusTypeValue, err = extractOrdererConsensusType(config)
			Expect(err).NotTo(HaveOccurred())
			//Verify OSN ready to restart
			validateConsensusTypeValue(consensusTypeValue, "etcdraft", protosorderer.ConsensusType_MIG_STATE_COMMIT, migrationContext)

			By("Step 2, Verify: new channels cannot be created")
			network.CreateChannelFail(orderer, channel)

			By("Step 2, Verify: new config updates are not accepted")
			updateConfigWithConsensusType("kafka", []byte{}, protosorderer.ConsensusType_MIG_STATE_START, 0,
				updatedConfig, consensusTypeValue)
			nwo.UpdateOrdererConfigFail(network, orderer, syschannel, config, updatedConfig, peer, orderer)
		})

		// This test executes the "green path" migration config updates on Kafka based system
		// with a system channel and a standard channel, and verifies that these config
		// updates have the desired effect.
		It("executes kafka2raft standard channel green path", func() {
			orderer := network.Orderer("orderer")
			peer := network.Peer("Org1", "peer1")

			syschannel := network.SystemChannel.Name

			channel := "testchannel"
			network.CreateAndJoinChannel(orderer, channel)
			nwo.DeployChaincode(network, channel, orderer, chaincode)
			RunExpectQueryInvokeQuery(network, orderer, peer, channel, 100)
			RunExpectQueryInvokeQuery(network, orderer, peer, channel, 90)

			//=== Step 1: Config update on system channel, START ===
			By("Step 1: Config update on system channel, MigrationState=START")
			config := nwo.GetConfig(network, peer, orderer, syschannel)
			updatedConfig, consensusTypeValue, err := extractOrdererConsensusType(config)
			Expect(err).NotTo(HaveOccurred())
			//Verify initial status
			validateConsensusTypeValue(consensusTypeValue, "kafka", protosorderer.ConsensusType_MIG_STATE_NONE, 0)
			//Change ConsensusType value
			updateConfigWithConsensusType("kafka", []byte{}, protosorderer.ConsensusType_MIG_STATE_START, 0,
				updatedConfig, consensusTypeValue)
			nwo.UpdateOrdererConfig(network, orderer, syschannel, config, updatedConfig, peer, orderer)

			By("Step 1, Verify: system channel config changed, get migration context")
			migrationContext := nwo.CurrentConfigBlockNumber(network, peer, orderer, syschannel)
			Expect(migrationContext).ToNot(Equal(0))

			config = nwo.GetConfig(network, peer, orderer, syschannel)
			updatedConfig, consensusTypeValue, err = extractOrdererConsensusType(config)
			Expect(err).NotTo(HaveOccurred())
			validateConsensusTypeValue(consensusTypeValue, "kafka", protosorderer.ConsensusType_MIG_STATE_START, 0)

			By("Step 1, Verify: TX's on standard channel are blocked")
			RunExpectQueryInvokeFail(network, orderer, peer, channel, 80)

			//=== Step 2: Config update on standard channel, CONTEXT ===
			By("Step 2: Config update on standard channel, MigrationState=CONTEXT")
			config = nwo.GetConfig(network, peer, orderer, channel)
			updatedConfig, consensusTypeValue, err = extractOrdererConsensusType(config)
			Expect(err).NotTo(HaveOccurred())
			//Verify initial status
			validateConsensusTypeValue(consensusTypeValue, "kafka", protosorderer.ConsensusType_MIG_STATE_NONE, 0)
			//Change ConsensusType value
			raftMetadata := prepareRaftMetadata(network)
			updateConfigWithConsensusType("etcdraft", raftMetadata, protosorderer.ConsensusType_MIG_STATE_CONTEXT, migrationContext,
				updatedConfig, consensusTypeValue)
			nwo.UpdateOrdererConfig(network, orderer, channel, config, updatedConfig, peer, orderer)
			//Verify standard channel changed
			By("Step 2, Verify: standard channel config changed")
			config = nwo.GetConfig(network, peer, orderer, channel)
			updatedConfig, consensusTypeValue, err = extractOrdererConsensusType(config)
			Expect(err).NotTo(HaveOccurred())
			validateConsensusTypeValue(consensusTypeValue, "etcdraft", protosorderer.ConsensusType_MIG_STATE_CONTEXT, migrationContext)

			//=== Step 3: config update on system channel, COMMIT ===
			By("Step 3: Config update on system channel, MigrationState=COMMIT")
			config = nwo.GetConfig(network, peer, orderer, syschannel)
			updatedConfig, consensusTypeValue, err = extractOrdererConsensusType(config)
			Expect(err).NotTo(HaveOccurred())
			//Change ConsensusType value
			updateConfigWithConsensusType("etcdraft", raftMetadata, protosorderer.ConsensusType_MIG_STATE_COMMIT, migrationContext,
				updatedConfig, consensusTypeValue)
			nwo.UpdateOrdererConfig(network, orderer, syschannel, config, updatedConfig, peer, orderer)

			By("Step 3, Verify: system channel config changed")
			sysBlockNum := nwo.CurrentConfigBlockNumber(network, peer, orderer, syschannel)
			Expect(sysBlockNum).To(Equal(migrationContext + 1))
		})
	})

	// These tests execute the migration config updates on Kafka based system, restart the orderer onto a Raft-based
	// system, and verifies that the newly restarted orderer performs as expected.
	Describe("kafka2raft migration raft side", func() {

		// This test executes the "green path" migration config updates on Kafka based system
		// with a single orderer, and a system channel and a standard channel.
		// It then restarts the orderer onto a Raft-based system, and verifies that the
		// newly restarted orderer performs as expected.
		It("executes bootstrap to raft", func() {
			network = nwo.New(kafka2RaftMultiChannel(), testDir, client, StartPort(), components)

			orderer := network.Orderer("orderer")
			peer := network.Peer("Org1", "peer1")

			network.GenerateConfigTree()
			network.Bootstrap()

			raftMetadata := prepareRaftMetadata(network)

			brokerGroup := network.BrokerGroupRunner()
			brokerProc = ifrit.Invoke(brokerGroup)
			Eventually(brokerProc.Ready()).Should(BeClosed())

			ordererRunner := network.OrdererRunner(orderer)
			ordererProc = ifrit.Invoke(ordererRunner)
			Eventually(ordererProc.Ready()).Should(BeClosed())

			peerGroup := network.PeerGroupRunner()
			peerProc = ifrit.Invoke(peerGroup)
			Eventually(peerProc.Ready()).Should(BeClosed())

			syschannel := network.SystemChannel.Name

			//=== Step 0: create & join first channel, deploy and invoke chaincode ===
			By("Step 0: create & join first channel, deploy and invoke chaincode")
			channel := "testchannel1"
			network.CreateAndJoinChannel(orderer, channel)
			nwo.DeployChaincode(network, channel, orderer, chaincode)
			RunExpectQueryInvokeQuery(network, orderer, peer, channel, 100)
			RunExpectQueryInvokeQuery(network, orderer, peer, channel, 90)

			//=== Step 1: Config update on system channel, START ===
			By("Step 1: Config update on system channel, MigrationState=START")
			config := nwo.GetConfig(network, peer, orderer, syschannel)
			updatedConfig, consensusTypeValue, err := extractOrdererConsensusType(config)
			Expect(err).NotTo(HaveOccurred())
			//Change ConsensusType value
			updateConfigWithConsensusType("kafka", []byte{}, protosorderer.ConsensusType_MIG_STATE_START, 0,
				updatedConfig, consensusTypeValue)
			nwo.UpdateOrdererConfig(network, orderer, syschannel, config, updatedConfig, peer, orderer)

			By("Step 1, Verify: system channel config changed, get context")
			migrationContext := nwo.CurrentConfigBlockNumber(network, peer, orderer, syschannel)
			Expect(migrationContext).ToNot(Equal(0))

			//=== Step 2: Config update on standard channel, CONTEXT ===
			By("Step 2: Config update on standard channel, MigrationState=CONTEXT")
			config = nwo.GetConfig(network, peer, orderer, channel)
			updatedConfig, consensusTypeValue, err = extractOrdererConsensusType(config)
			Expect(err).NotTo(HaveOccurred())
			//Change ConsensusType value
			updateConfigWithConsensusType("etcdraft", raftMetadata, protosorderer.ConsensusType_MIG_STATE_CONTEXT, migrationContext,
				updatedConfig, consensusTypeValue)
			nwo.UpdateOrdererConfig(network, orderer, channel, config, updatedConfig, peer, orderer)

			//=== Step 3: config update on system channel, COMMIT ===
			By("Step 3: config update on system channel, MigrationState=COMMIT")
			config = nwo.GetConfig(network, peer, orderer, syschannel)
			updatedConfig, consensusTypeValue, err = extractOrdererConsensusType(config)
			Expect(err).NotTo(HaveOccurred())
			//Change ConsensusType value
			updateConfigWithConsensusType("etcdraft", raftMetadata, protosorderer.ConsensusType_MIG_STATE_COMMIT, migrationContext,
				updatedConfig, consensusTypeValue)
			nwo.UpdateOrdererConfig(network, orderer, syschannel, config, updatedConfig, peer, orderer)

			By("Step 3, Verify: system channel config changed")
			sysBlockNum := nwo.CurrentConfigBlockNumber(network, peer, orderer, syschannel)
			Expect(sysBlockNum).To(Equal(migrationContext + 1))
			chan1BlocNum := nwo.CurrentConfigBlockNumber(network, peer, orderer, channel)

			//=== Step 4: kill ===
			By("Step 4: killing orderer1")
			ordererProc.Signal(syscall.SIGKILL)
			Eventually(ordererProc.Wait(), network.EventuallyTimeout).Should(Receive(MatchError("exit status 137")))

			//=== Step 5: restart ===
			By("Step 5: restarting orderer1")
			network.Consensus.Type = "etcdraft"
			ordererRunner = network.OrdererRunner(orderer)
			ordererProc = ifrit.Invoke(ordererRunner)
			Eventually(ordererProc.Ready(), network.EventuallyTimeout).Should(BeClosed())

			waitForBlockReception(orderer, peer, network, syschannel, int(sysBlockNum))
			waitForBlockReception(orderer, peer, network, channel, int(chan1BlocNum))

			Eventually(ordererRunner.Err(), time.Minute, time.Second).Should(gbytes.Say("Raft leader changed: 0 -> "))

			By("Step 5, Verify: executing config transaction on system channel with restarted orderer")
			config = nwo.GetConfig(network, peer, orderer, syschannel)
			updatedConfig, consensusTypeValue, err = extractOrdererConsensusType(config)
			Expect(err).NotTo(HaveOccurred())
			//Change ConsensusType value
			updateConfigWithConsensusType("etcdraft", raftMetadata, protosorderer.ConsensusType_MIG_STATE_NONE, 0,
				updatedConfig, consensusTypeValue)
			nwo.UpdateOrdererConfig(network, orderer, syschannel, config, updatedConfig, peer, orderer)

			By("Step 5, Verify: executing config transaction on standard channel with restarted orderer")
			config = nwo.GetConfig(network, peer, orderer, channel)
			updatedConfig, consensusTypeValue, err = extractOrdererConsensusType(config)
			Expect(err).NotTo(HaveOccurred())
			//Change ConsensusType value
			updateConfigWithConsensusType("etcdraft", raftMetadata, protosorderer.ConsensusType_MIG_STATE_NONE, 0,
				updatedConfig, consensusTypeValue)
			nwo.UpdateOrdererConfig(network, orderer, channel, config, updatedConfig, peer, orderer)

			By("Step 5, Verify: executing transaction on standard channel with restarted orderer")
			RunExpectQueryInvokeQuery(network, orderer, peer, channel, 80)

			By("Step 5, Verify: create new channel, executing transaction with restarted orderer")
			channel2 := "testchannel2"
			network.CreateAndJoinChannel(orderer, channel2)
			nwo.InstantiateChaincode(network, channel2, orderer, chaincode, peer, network.PeersWithChannel(channel2)...)
			RunExpectQueryRetry(network, peer, channel2, 100)
			RunExpectQueryInvokeQuery(network, orderer, peer, channel2, 100)
			RunExpectQueryInvokeQuery(network, orderer, peer, channel2, 90)
		})

		// This test executes the "green path" migration config updates on Kafka based system
		// with a three orderers, a system channel and two standard channels.
		// It then restarts the orderers onto a Raft-based system, and verifies that the
		// newly restarted orderers perform as expected.
		It("executes bootstrap to raft - multi node", func() {
			network = nwo.New(kafka2RaftMultiNode(), testDir, client, StartPort(), components)

			o1, o2, o3 := network.Orderer("orderer1"), network.Orderer("orderer2"), network.Orderer("orderer3")
			peer := network.Peer("Org1", "peer1")

			network.GenerateConfigTree()
			network.Bootstrap()
			//In migration orderers manipulate the GenesisFile, so each orderer must have a separate copy
			separateBootstrap(network)

			brokerGroup := network.BrokerGroupRunner()
			brokerProc = ifrit.Invoke(brokerGroup)
			Eventually(brokerProc.Ready()).Should(BeClosed())

			o1Runner := network.OrdererRunner(o1)
			o2Runner := network.OrdererRunner(o2)
			o3Runner := network.OrdererRunner(o3)

			peerGroup := network.PeerGroupRunner()

			o1Proc = ifrit.Invoke(o1Runner)
			o2Proc = ifrit.Invoke(o2Runner)
			o3Proc = ifrit.Invoke(o3Runner)

			Eventually(o1Proc.Ready()).Should(BeClosed())
			Eventually(o2Proc.Ready()).Should(BeClosed())
			Eventually(o3Proc.Ready()).Should(BeClosed())

			peerProc = ifrit.Invoke(peerGroup)
			Eventually(peerProc.Ready()).Should(BeClosed())

			raftMetadata := prepareRaftMetadata(network)

			syschannel := network.SystemChannel.Name

			By("Step 0: create & join first channel, deploy and invoke chaincode")
			channel1 := "testchannel1"
			network.CreateAndJoinChannel(o1, channel1)
			nwo.DeployChaincode(network, channel1, o1, chaincode)
			RunExpectQueryInvokeQuery(network, o1, peer, channel1, 100)
			RunExpectQueryInvokeQuery(network, o1, peer, channel1, 90)

			//=== Step 1: Config update on system channel, START ===
			By("Step 1: Config update on system channel, MigrationState=START")
			config := nwo.GetConfig(network, peer, o1, syschannel)
			updatedConfig, consensusTypeValue, err := extractOrdererConsensusType(config)
			Expect(err).NotTo(HaveOccurred())
			//Change ConsensusType value
			updateConfigWithConsensusType("kafka", []byte{}, protosorderer.ConsensusType_MIG_STATE_START, 0,
				updatedConfig, consensusTypeValue)
			nwo.UpdateOrdererConfig(network, o1, syschannel, config, updatedConfig, peer, o1)

			By("Step 1, Verify: system channel config changed, get context")
			migrationContext := nwo.CurrentConfigBlockNumber(network, peer, o1, syschannel)
			Expect(migrationContext).ToNot(Equal(0))

			//=== Step 2: Config update on standard channel, CONTEXT ===
			By("Step 2: Config update on standard channel, MigrationState=CONTEXT")
			config = nwo.GetConfig(network, peer, o1, channel1)
			updatedConfig, consensusTypeValue, err = extractOrdererConsensusType(config)
			Expect(err).NotTo(HaveOccurred())
			//Change ConsensusType value
			updateConfigWithConsensusType("etcdraft", raftMetadata, protosorderer.ConsensusType_MIG_STATE_CONTEXT, migrationContext,
				updatedConfig, consensusTypeValue)
			nwo.UpdateOrdererConfig(network, o1, channel1, config, updatedConfig, peer, o1)

			//=== Step 3: config update on system channel, COMMIT ===
			By("Step 3: config update on system channel, MigrationState=COMMIT")
			config = nwo.GetConfig(network, peer, o1, syschannel)
			updatedConfig, consensusTypeValue, err = extractOrdererConsensusType(config)
			Expect(err).NotTo(HaveOccurred())
			//Change ConsensusType value
			updateConfigWithConsensusType("etcdraft", raftMetadata, protosorderer.ConsensusType_MIG_STATE_COMMIT, migrationContext,
				updatedConfig, consensusTypeValue)
			nwo.UpdateOrdererConfig(network, o1, syschannel, config, updatedConfig, peer, o1)

			By("Step 3, Verify: system channel config changed")
			sysBlockNum := nwo.CurrentConfigBlockNumber(network, peer, o1, syschannel)
			Expect(sysBlockNum).To(Equal(migrationContext + 1))
			chan1BlockNum := nwo.CurrentConfigBlockNumber(network, peer, o1, channel1)

			//=== Step 4: kill ===
			By("Step 4: killing orderer1,2,3")
			for _, oProc := range []ifrit.Process{o1Proc, o2Proc, o3Proc} {
				if oProc != nil {
					oProc.Signal(syscall.SIGKILL)
					Eventually(oProc.Wait(), network.EventuallyTimeout).Should(Receive(MatchError("exit status 137")))
				}
			}

			//=== Step 5: restart ===
			By("Step 5: restarting orderer1,2,3")
			network.Consensus.Type = "etcdraft"
			o1Runner = network.OrdererRunner(o1)
			o2Runner = network.OrdererRunner(o2)
			o3Runner = network.OrdererRunner(o3)

			o1Proc = ifrit.Invoke(o1Runner)
			o2Proc = ifrit.Invoke(o2Runner)
			o3Proc = ifrit.Invoke(o3Runner)

			Eventually(o1Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			Eventually(o2Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			Eventually(o3Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())

			waitForBlockReception(o1, peer, network, syschannel, int(sysBlockNum))
			waitForBlockReception(o1, peer, network, channel1, int(chan1BlockNum))
			waitForBlockReception(o2, peer, network, syschannel, int(sysBlockNum))
			waitForBlockReception(o2, peer, network, channel1, int(chan1BlockNum))
			waitForBlockReception(o3, peer, network, syschannel, int(sysBlockNum))
			waitForBlockReception(o3, peer, network, channel1, int(chan1BlockNum))

			Eventually(o1Runner.Err(), time.Minute, time.Second).Should(gbytes.Say("Raft leader changed: 0 -> "))
			Eventually(o2Runner.Err(), time.Minute, time.Second).Should(gbytes.Say("Raft leader changed: 0 -> "))
			Eventually(o3Runner.Err(), time.Minute, time.Second).Should(gbytes.Say("Raft leader changed: 0 -> "))

			By("Step 5, Verify: executing config transaction on system channel with restarted orderer")
			config = nwo.GetConfig(network, peer, o1, syschannel)
			updatedConfig, consensusTypeValue, err = extractOrdererConsensusType(config)
			Expect(err).NotTo(HaveOccurred())
			//Change ConsensusType value
			updateConfigWithConsensusType("etcdraft", raftMetadata, protosorderer.ConsensusType_MIG_STATE_NONE, 0,
				updatedConfig, consensusTypeValue)
			nwo.UpdateOrdererConfig(network, o1, syschannel, config, updatedConfig, peer, o1)

			By("Step 5, Verify: executing config transaction on standard channel with restarted orderer")
			config = nwo.GetConfig(network, peer, o1, channel1)
			updatedConfig, consensusTypeValue, err = extractOrdererConsensusType(config)
			Expect(err).NotTo(HaveOccurred())
			//Change ConsensusType value
			updateConfigWithConsensusType("etcdraft", raftMetadata, protosorderer.ConsensusType_MIG_STATE_NONE, 0,
				updatedConfig, consensusTypeValue)
			nwo.UpdateOrdererConfig(network, o1, channel1, config, updatedConfig, peer, o1)

			By("Step 5, Verify: executing transaction on standard channel with restarted orderer")
			RunExpectQueryInvokeQuery(network, o1, peer, channel1, 80)

			By("Step 5, Verify: create new channel, executing transaction with restarted orderer")
			channel2 := "testchannel2"
			network.CreateAndJoinChannel(o1, channel2)
			nwo.InstantiateChaincode(network, channel2, o1, chaincode, peer, network.PeersWithChannel(channel2)...)
			RunExpectQueryRetry(network, peer, channel2, 100)
			RunExpectQueryInvokeQuery(network, o1, peer, channel2, 100)
			RunExpectQueryInvokeQuery(network, o1, peer, channel2, 90)
		})
	})
})

func RunExpectQueryInvokeQuery(n *nwo.Network, orderer *nwo.Orderer, peer *nwo.Peer, channel string, expect int) {
	By("querying the chaincode")
	sess, err := n.PeerUserSession(peer, "User1", commands.ChaincodeQuery{
		ChannelID: channel,
		Name:      "mycc",
		Ctor:      `{"Args":["query","a"]}`,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess).To(gbytes.Say(fmt.Sprint(expect)))

	By("invoking the chaincode")
	sess, err = n.PeerUserSession(peer, "User1", commands.ChaincodeInvoke{
		ChannelID: channel,
		Orderer:   n.OrdererAddress(orderer, nwo.ListenPort),
		Name:      "mycc",
		Ctor:      `{"Args":["invoke","a","b","10"]}`,
		PeerAddresses: []string{
			n.PeerAddress(n.Peer("Org1", "peer0"), nwo.ListenPort),
			n.PeerAddress(n.Peer("Org2", "peer1"), nwo.ListenPort),
		},
		WaitForEvent: true,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess.Err).To(gbytes.Say("Chaincode invoke successful. result: status:200"))

	By("querying the chaincode again")
	sess, err = n.PeerUserSession(peer, "User1", commands.ChaincodeQuery{
		ChannelID: channel,
		Name:      "mycc",
		Ctor:      `{"Args":["query","a"]}`,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess).To(gbytes.Say(fmt.Sprint(expect - 10)))
}

func RunExpectQueryRetry(n *nwo.Network, peer *nwo.Peer, channel string, expect int) {
	By("querying the chaincode with retry")

	Eventually(func() string {
		sess, err := n.PeerUserSession(peer, "User1", commands.ChaincodeQuery{
			ChannelID: channel,
			Name:      "mycc",
			Ctor:      `{"Args":["query","a"]}`,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit())
		if sess.ExitCode() != 0 {
			sessEC := fmt.Sprintf("exit code is %d: %s", sess.ExitCode(), string(sess.Out.Contents()))
			return sessEC
		}
		sessOut := string(sess.Out.Contents())
		expectStr := fmt.Sprintf("%d", expect)

		if strings.Contains(sessOut, expectStr) {
			return ""
		} else {
			return "waiting..."
		}
	}, n.EventuallyTimeout, time.Second).Should(BeEmpty())
}

func RunExpectQueryInvokeFail(n *nwo.Network, orderer *nwo.Orderer, peer *nwo.Peer, channel string, expect int) {
	By("querying the chaincode")
	sess, err := n.PeerUserSession(peer, "User1", commands.ChaincodeQuery{
		ChannelID: channel,
		Name:      "mycc",
		Ctor:      `{"Args":["query","a"]}`,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess).To(gbytes.Say(fmt.Sprint(expect)))

	sess, err = n.PeerUserSession(peer, "User1", commands.ChaincodeInvoke{
		ChannelID: channel,
		Orderer:   n.OrdererAddress(orderer, nwo.ListenPort),
		Name:      "mycc",
		Ctor:      `{"Args":["invoke","a","b","10"]}`,
		PeerAddresses: []string{
			n.PeerAddress(n.Peer("Org1", "peer0"), nwo.ListenPort),
			n.PeerAddress(n.Peer("Org2", "peer1"), nwo.ListenPort),
		},
		WaitForEvent: true,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).ShouldNot(gexec.Exit(0))
	Expect(sess.Err).NotTo(gbytes.Say("Chaincode invoke successful. result: status:200"))
}

func validateConsensusTypeValue(value *protosorderer.ConsensusType, cType string, state protosorderer.ConsensusType_MigrationState, context uint64) {
	Expect(value.Type).To(Equal(cType))
	Expect(value.MigrationState).To(Equal(state))
	Expect(value.MigrationContext).To(Equal(context))
}

func extractOrdererConsensusType(config *common.Config) (updatedConfig *common.Config, consensusTypeValue *protosorderer.ConsensusType, err error) {
	updatedConfig = proto.Clone(config).(*common.Config)
	consensusTypeConfigValue := updatedConfig.ChannelGroup.Groups["Orderer"].Values["ConsensusType"]
	consensusTypeValue = new(protosorderer.ConsensusType)
	err = proto.Unmarshal(consensusTypeConfigValue.Value, consensusTypeValue)
	return
}

func updateConfigWithConsensusType(consensusType string, consensusMetadata []byte, migState protosorderer.ConsensusType_MigrationState, migContext uint64,
	updatedConfig *common.Config, consensusTypeValue *protosorderer.ConsensusType) {
	consensusTypeValue.Type = consensusType
	consensusTypeValue.Metadata = consensusMetadata
	consensusTypeValue.MigrationState = migState
	consensusTypeValue.MigrationContext = migContext
	updatedConfig.ChannelGroup.Groups["Orderer"].Values["ConsensusType"] = &common.ConfigValue{
		ModPolicy: "Admins",
		Value:     protoutil.MarshalOrPanic(consensusTypeValue),
	}
}

func kafka2RaftMultiChannel() *nwo.Config {
	config := nwo.BasicKafka()

	config.Channels = []*nwo.Channel{
		{Name: "testchannel1", Profile: "TwoOrgsChannel"},
		{Name: "testchannel2", Profile: "TwoOrgsChannel"}}

	for _, peer := range config.Peers {
		peer.Channels = []*nwo.PeerChannel{
			{Name: "testchannel1", Anchor: true},
			{Name: "testchannel2", Anchor: true},
		}
	}
	return config
}

func kafka2RaftMultiNode() *nwo.Config {
	config := nwo.BasicKafka()
	config.Orderers = []*nwo.Orderer{
		{Name: "orderer1", Organization: "OrdererOrg"},
		{Name: "orderer2", Organization: "OrdererOrg"},
		{Name: "orderer3", Organization: "OrdererOrg"},
	}

	config.Profiles = []*nwo.Profile{{
		Name:     "TwoOrgsOrdererGenesis",
		Orderers: []string{"orderer1", "orderer2", "orderer3"},
	}, {
		Name:          "TwoOrgsChannel",
		Consortium:    "SampleConsortium",
		Organizations: []string{"Org1", "Org2"},
	}}

	config.Channels = []*nwo.Channel{
		{Name: "testchannel1", Profile: "TwoOrgsChannel"},
		{Name: "testchannel2", Profile: "TwoOrgsChannel"},
		{Name: "testchannel3", Profile: "TwoOrgsChannel"},
	}

	for _, peer := range config.Peers {
		peer.Channels = []*nwo.PeerChannel{
			{Name: "testchannel1", Anchor: true},
			{Name: "testchannel2", Anchor: true},
			{Name: "testchannel3", Anchor: true},
		}
	}
	return config
}

func prepareRaftMetadata(network *nwo.Network) []byte {
	var consenters []*protosraft.Consenter
	for _, o := range network.Orderers {
		fullTlsPath := network.OrdererLocalTLSDir(o)
		certBytes, err := ioutil.ReadFile(filepath.Join(fullTlsPath, "server.crt"))
		Expect(err).NotTo(HaveOccurred())
		port := network.OrdererPort(o, nwo.ListenPort)

		consenter := &protosraft.Consenter{
			ClientTlsCert: certBytes,
			ServerTlsCert: certBytes,
			Host:          "127.0.0.1",
			Port:          uint32(port),
		}
		consenters = append(consenters, consenter)
	}

	raftMetadata := &protosraft.ConfigMetadata{
		Consenters: consenters,
		Options: &protosraft.Options{
			TickInterval:         "500ms",
			ElectionTick:         10,
			HeartbeatTick:        1,
			MaxInflightBlocks:    5,
			SnapshotIntervalSize: 10 * 1024 * 1024,
		}}

	raftMetadataBytes := protoutil.MarshalOrPanic(raftMetadata)
	raftMetadata2 := &protosraft.ConfigMetadata{}
	errUnma := proto.Unmarshal(raftMetadataBytes, raftMetadata2)
	Expect(errUnma).NotTo(HaveOccurred())

	return raftMetadataBytes
}

// separateBootstrap distributes the GenesisFile to the orderer(s) config dir, so that every orderer has its own copy
// of the GenesisFile. It also changes the orderer's config to point to the new location.
func separateBootstrap(network *nwo.Network) {
	from := filepath.Join(network.RootDir, "systemchannel_block.pb")
	input, errR := ioutil.ReadFile(from)
	Expect(errR).NotTo(HaveOccurred())

	for _, o := range network.Orderers {
		to := filepath.Join(network.OrdererDir(o), "systemchannel_block.pb")
		errW := ioutil.WriteFile(to, input, 0644)
		Expect(errW).NotTo(HaveOccurred())
		oConf := network.ReadOrdererConfig(o)
		oConf.General.GenesisFile = to
		network.WriteOrdererConfig(o, oConf)
	}
}
