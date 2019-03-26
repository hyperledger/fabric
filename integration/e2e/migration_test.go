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

	"github.com/fsouza/go-dockerclient"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	"github.com/hyperledger/fabric/protos/common"
	protosorderer "github.com/hyperledger/fabric/protos/orderer"
	protosraft "github.com/hyperledger/fabric/protos/orderer/etcdraft"
	"github.com/hyperledger/fabric/protos/utils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
)

var _ bool = Describe("Kafka2RaftMigration", func() {
	var (
		testDir   string
		client    *docker.Client
		network   *nwo.Network
		chaincode nwo.Chaincode

		process                           ifrit.Process
		peerProc, ordererProc, brokerProc ifrit.Process
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

		if ordererProc != nil {
			ordererProc.Signal(syscall.SIGTERM)
			Eventually(ordererProc.Wait(), network.EventuallyTimeout).Should(Receive())
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

	// These tests execute the migration config updates on Kafka based system,
	// and verify that these config update have the desired effect.
	Describe("kafka2raft migration kafka side", func() {
		BeforeEach(func() {
			conf := nwo.BasicKafka()
			conf.OrdererCap.V2_0 = true
			network = nwo.New(conf, testDir, client, BasePort(), components)
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
			network.CreateChannelFail(channel, orderer, peer)

			//=== Step 2: Config update on system channel, MigrationState=COMMIT, type=etcdraft ===
			By("Step 2: Config update on system channel, MigrationState=COMMIT, type=etcdraft")
			config = nwo.GetConfig(network, peer, orderer, syschannel)
			updatedConfig, consensusTypeValue, err = extractOrdererConsensusType(config)
			Expect(err).NotTo(HaveOccurred())

			//Verify previous state
			validateConsensusTypeValue(consensusTypeValue, "kafka", protosorderer.ConsensusType_MIG_STATE_START, 0)

			//Change ConsensusType value
			updateConfigWithConsensusType("etcdraft", []byte{}, protosorderer.ConsensusType_MIG_STATE_COMMIT, migrationContext,
				updatedConfig, consensusTypeValue)
			nwo.UpdateOrdererConfig(network, orderer, syschannel, config, updatedConfig, peer, orderer)
			num := nwo.CurrentConfigBlockNumber(network, peer, orderer, syschannel)
			Expect(num).To(Equal(migrationContext + 1))

			By("Step 2, Verify: system channel config change, OSN ready to restart")
			config = nwo.GetConfig(network, peer, orderer, syschannel)
			updatedConfig, consensusTypeValue, err = extractOrdererConsensusType(config)
			Expect(err).NotTo(HaveOccurred())
			//Verify OSN ready to restart
			validateConsensusTypeValue(consensusTypeValue, "etcdraft", protosorderer.ConsensusType_MIG_STATE_COMMIT, migrationContext)

			By("Step 2, Verify: new channels cannot be created")
			network.CreateChannelFail(channel, orderer, peer, peer)

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
			updateConfigWithConsensusType("etcdraft", []byte{}, protosorderer.ConsensusType_MIG_STATE_CONTEXT, migrationContext,
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
			updateConfigWithConsensusType("etcdraft", []byte{}, protosorderer.ConsensusType_MIG_STATE_COMMIT, migrationContext,
				updatedConfig, consensusTypeValue)
			nwo.UpdateOrdererConfig(network, orderer, syschannel, config, updatedConfig, peer, orderer)

			By("Step 3, Verify: system channel config changed")
			num := nwo.CurrentConfigBlockNumber(network, peer, orderer, syschannel)
			Expect(num).To(Equal(migrationContext + 1))
		})
	})

	// These tests execute the migration config updates on Kafka based system
	// restart the orderer onto a Raft-based system, and verifies that the newly
	// restarted orderer performs as expected.
	Describe("kafka2raft migration raft side", func() {

		// This test executes the "green path" migration config updates on Kafka based system
		// with a single orderer, and a system channel and a standard channel.
		// It then restarts the orderer onto a Raft-based system, and verifies that the
		// newly restarted orderer performs as expected.
		It("executes bootstrap to raft", func() {
			network = nwo.New(kafka2RaftMultiChannel(), testDir, client, BasePort(), components)

			orderer := network.Orderer("orderer")
			peer := network.Peer("Org1", "peer1")

			network.GenerateConfigTree()
			network.Bootstrap()

			tlsPath := "crypto/ordererOrganizations/example.com/orderers/orderer.example.com/tls/"
			Expect(filepath.Join(network.RootDir, tlsPath)).To(Equal(network.OrdererLocalTLSDir(orderer)))
			certBytes, err := ioutil.ReadFile(filepath.Join(network.RootDir, tlsPath, "server.crt"))
			Expect(err).NotTo(HaveOccurred())
			raft_metadata := &protosraft.ConfigMetadata{
				Consenters: []*protosraft.Consenter{
					{
						ClientTlsCert: certBytes,
						ServerTlsCert: certBytes,
						Host:          "127.0.0.1",
						Port:          uint32(31001),
					},
				},
				Options: &protosraft.Options{
					TickInterval:         "500ms",
					ElectionTick:         10,
					HeartbeatTick:        1,
					MaxInflightBlocks:    5,
					SnapshotIntervalSize: 20 * 1024 * 1024,
				}}

			raft_metadata_bytes := utils.MarshalOrPanic(raft_metadata)
			raft_metadata2 := &protosraft.ConfigMetadata{}
			errUnma := proto.Unmarshal(raft_metadata_bytes, raft_metadata2)
			Expect(errUnma).NotTo(HaveOccurred())

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
			updateConfigWithConsensusType("etcdraft", raft_metadata_bytes, protosorderer.ConsensusType_MIG_STATE_CONTEXT, migrationContext,
				updatedConfig, consensusTypeValue)
			nwo.UpdateOrdererConfig(network, orderer, channel, config, updatedConfig, peer, orderer)

			//=== Step 3: config update on system channel, COMMIT ===
			By("Step 3: config update on system channel, MigrationState=COMMIT")
			config = nwo.GetConfig(network, peer, orderer, syschannel)
			updatedConfig, consensusTypeValue, err = extractOrdererConsensusType(config)
			Expect(err).NotTo(HaveOccurred())
			//Change ConsensusType value
			updateConfigWithConsensusType("etcdraft", raft_metadata_bytes, protosorderer.ConsensusType_MIG_STATE_COMMIT, migrationContext,
				updatedConfig, consensusTypeValue)
			nwo.UpdateOrdererConfig(network, orderer, syschannel, config, updatedConfig, peer, orderer)

			By("Step 3, Verify: system channel config changed")
			num := nwo.CurrentConfigBlockNumber(network, peer, orderer, syschannel)
			Expect(num).To(Equal(migrationContext + 1))
			num1 := nwo.CurrentConfigBlockNumber(network, peer, orderer, channel)

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

			waitForBlockReception(orderer, peer, network, syschannel, int(num))
			waitForBlockReception(orderer, peer, network, channel, int(num1))

			By("Step 5, Verify: executing config transaction on system channel with restarted orderer")
			config = nwo.GetConfig(network, peer, orderer, syschannel)
			updatedConfig, consensusTypeValue, err = extractOrdererConsensusType(config)
			Expect(err).NotTo(HaveOccurred())
			//Change ConsensusType value
			updateConfigWithConsensusType("etcdraft", raft_metadata_bytes, protosorderer.ConsensusType_MIG_STATE_NONE, 0,
				updatedConfig, consensusTypeValue)
			nwo.UpdateOrdererConfig(network, orderer, syschannel, config, updatedConfig, peer, orderer)

			By("Step 5, Verify: executing config transaction on standard channel with restarted orderer")
			config = nwo.GetConfig(network, peer, orderer, channel)
			updatedConfig, consensusTypeValue, err = extractOrdererConsensusType(config)
			Expect(err).NotTo(HaveOccurred())
			//Change ConsensusType value
			updateConfigWithConsensusType("etcdraft", raft_metadata_bytes, protosorderer.ConsensusType_MIG_STATE_NONE, 0,
				updatedConfig, consensusTypeValue)
			nwo.UpdateOrdererConfig(network, orderer, channel, config, updatedConfig, peer, orderer)

			By("Step 5, Verify: executing transaction on standard channel with restarted orderer")
			RunExpectQueryInvokeQuery(network, orderer, peer, channel, 80)

			By("Step 5, Verify: create new channel, executing transaction with restarted orderer")
			channel2 := "testchannel2"
			network.CreateAndJoinChannel(orderer, channel2)
			nwo.InstantiateChaincode(network, channel2, orderer, chaincode, peer, network.PeersWithChannel(channel2)...)
			RunExpectQueryRetry(network, orderer, peer, channel2, 100)
			RunExpectQueryInvokeQuery(network, orderer, peer, channel2, 100)
			RunExpectQueryInvokeQuery(network, orderer, peer, channel2, 90)
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

func RunExpectQueryRetry(n *nwo.Network, orderer *nwo.Orderer, peer *nwo.Peer, channel string, expect int) {
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
		Value:     utils.MarshalOrPanic(consensusTypeValue),
	}
}

func kafka2RaftMultiChannel() *nwo.Config {
	config := nwo.BasicKafka()
	config.OrdererCap.V2_0 = true

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
