/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package raft

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger-labs/SmartBFT/pkg/types"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	protosorderer "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/hyperledger/fabric-protos-go-apiv2/orderer/smartbft"
	"github.com/hyperledger/fabric/common/capabilities"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/integration/channelparticipation"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	"github.com/hyperledger/fabric/integration/ordererclient"
	"github.com/hyperledger/fabric/internal/configtxlator/update"
	"github.com/hyperledger/fabric/protoutil"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
	ginkgomon "github.com/tedsuo/ifrit/ginkgomon_v2"
	"google.golang.org/protobuf/proto"
)

var _ = Describe("ConsensusTypeMigration", func() {
	var (
		testDir string
		client  *docker.Client
		network *nwo.Network

		o1Proc, o2Proc, o3Proc, o4Proc ifrit.Process

		o1Runner *ginkgomon.Runner
		o2Runner *ginkgomon.Runner
		o3Runner *ginkgomon.Runner
		o4Runner *ginkgomon.Runner
	)

	BeforeEach(func() {
		var err error
		testDir, err = os.MkdirTemp("", "consensus-type-migration")
		Expect(err).NotTo(HaveOccurred())

		client, err = docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		for _, oProc := range []ifrit.Process{o1Proc, o2Proc, o3Proc, o4Proc} {
			if oProc != nil {
				oProc.Signal(syscall.SIGTERM)
				Eventually(oProc.Wait(), network.EventuallyTimeout).Should(Receive())
			}
		}

		if network != nil {
			network.Cleanup()
		}

		_ = os.RemoveAll(testDir)
	})

	// This test executes the migration on an etcdraft based system.
	// The migration includes change to maintenance mode, config updates needed for migration to a bft based system and
	// exiting maintenance mode back to the normal state.
	// This test restarts the orderers and ensures they operate under the new configuration.
	Describe("Raft to BFT migration", func() {
		It("migrates from Raft to BFT", func() {
			// === Step 1: Create and run Raft based system with 4 nodes ===
			By("1) Starting Raft based system with 4 nodes")
			networkConfig := nwo.MultiNodeEtcdRaft()
			networkConfig.Orderers = append(networkConfig.Orderers, &nwo.Orderer{Name: "orderer4", Organization: "OrdererOrg"})
			networkConfig.Profiles[0].Orderers = []string{"orderer1", "orderer2", "orderer3", "orderer4"}
			network = nwo.New(networkConfig, testDir, client, StartPort(), components)

			o1, o2, o3, o4 := network.Orderer("orderer1"), network.Orderer("orderer2"), network.Orderer("orderer3"), network.Orderer("orderer4")
			network.GenerateConfigTree()
			network.Bootstrap()

			runOrderers := func() {
				o1Runner = network.OrdererRunner(o1)
				o2Runner = network.OrdererRunner(o2)
				o3Runner = network.OrdererRunner(o3)
				o4Runner = network.OrdererRunner(o4)

				o1Runner.Command.Env = append(o1Runner.Command.Env, "FABRIC_LOGGING_SPEC=orderer.common.cluster=debug:orderer.consensus.smartbft=debug:policies.ImplicitOrderer=debug")
				o2Runner.Command.Env = append(o2Runner.Command.Env, "FABRIC_LOGGING_SPEC=orderer.common.cluster=debug:orderer.consensus.smartbft=debug:policies.ImplicitOrderer=debug")
				o3Runner.Command.Env = append(o3Runner.Command.Env, "FABRIC_LOGGING_SPEC=orderer.common.cluster=debug:orderer.consensus.smartbft=debug:policies.ImplicitOrderer=debug")
				o4Runner.Command.Env = append(o4Runner.Command.Env, "FABRIC_LOGGING_SPEC=orderer.common.cluster=debug:orderer.consensus.smartbft=debug:policies.ImplicitOrderer=debug")

				o1Proc = ifrit.Invoke(o1Runner)
				o2Proc = ifrit.Invoke(o2Runner)
				o3Proc = ifrit.Invoke(o3Runner)
				o4Proc = ifrit.Invoke(o4Runner)

				Eventually(o1Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
				Eventually(o2Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
				Eventually(o3Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
				Eventually(o4Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			}

			runOrderers()

			channelparticipation.JoinOrderersAppChannelCluster(network, "testchannel", o1, o2, o3, o4)
			FindLeader([]*ginkgomon.Runner{o1Runner, o2Runner, o3Runner, o4Runner})

			// === Step 2: Create a transaction with orderer1 ===
			By("2) Performing operation with orderer1")
			env := CreateBroadcastEnvelope(network, o1, "testchannel", []byte("foo"))
			resp, err := ordererclient.Broadcast(network, o1, env)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Status).To(Equal(common.Status_SUCCESS))

			block := FetchBlock(network, o1, 1, "testchannel")
			Expect(block).NotTo(BeNil())

			peer := network.Peer("Org1", "peer0")
			peer2 := network.Peer("Org2", "peer0")

			// config update that should fail
			By("Config update with global level endpoints")
			config := nwo.GetConfig(network, peer, o1, "testchannel")
			updatedConfig := proto.Clone(config).(*common.Config)
			addGlobalLevelEndpointsToConfig(updatedConfig)
			updateOrdererEndpointsConfigFails(network, o1, "testchannel", config, updatedConfig, peer, peer, peer2)

			// config update that succeeds but causes a failure during the migration step
			By("Config update with empty endpoints per organization")
			updatedConfig = proto.Clone(config).(*common.Config)
			cleanEndpointsPerOrgFromConfig(updatedConfig)
			updateOrdererOrgEndpointsConfigSucceeds(network, o1, "testchannel", config, updatedConfig, peer, peer, peer2)

			// === Step 3: Config update on standard channel, State=MAINTENANCE, enter maintenance-mode ===
			By("3) Change to maintenance mode")
			config, updatedConfig = prepareTransition(network, peer, o1, "testchannel",
				"etcdraft", protosorderer.ConsensusType_STATE_NORMAL,
				"etcdraft", nil, protosorderer.ConsensusType_STATE_MAINTENANCE, 0)
			nwo.UpdateOrdererConfig(network, o1, "testchannel", config, updatedConfig, peer, o1)

			// === Step 4: Config update, State=MAINTENANCE, type=BFT ===
			By("4) Config updates: migration from Raft to BFT")

			bftMetadata := protoutil.MarshalOrPanic(prepareBftMetadata())

			// NOTE: migration should fail since there are empty orderer organizations endpoints
			By("Migration from Raft to BFT fails")
			config, updatedConfig = prepareTransition(network, peer, o1, "testchannel",
				"etcdraft", protosorderer.ConsensusType_STATE_MAINTENANCE,
				"BFT", bftMetadata, protosorderer.ConsensusType_STATE_MAINTENANCE, 1)
			UpdateOrdererConfigFails(network, o1, "testchannel", config, updatedConfig, peer, o1)

			// config update to add orderer organizations endpoints
			By("Config update to add orderer organizations endpoints")
			config = nwo.GetConfig(network, peer, o1, "testchannel")
			updatedConfig = proto.Clone(config).(*common.Config)
			addEndpointsPerOrgInConfig(updatedConfig)
			updateOrdererOrgEndpointsConfigSucceeds(network, o1, "testchannel", config, updatedConfig, peer, peer, peer2)

			// now, we can migrate
			By("Migration from Raft to BFT succeeds")
			config, updatedConfig = prepareTransition(network, peer, o1, "testchannel",
				"etcdraft", protosorderer.ConsensusType_STATE_MAINTENANCE,
				"BFT", bftMetadata, protosorderer.ConsensusType_STATE_MAINTENANCE, 1)
			currentBlockNumber := nwo.CurrentConfigBlockNumber(network, peer, o1, "testchannel")
			nwo.UpdateOrdererConfig(network, o1, "testchannel", config, updatedConfig, peer, o1)

			// === Step 5: Check block reception in all orderers and restart all orderers ===
			By("5) Checking all orderers received the last block and restarting all orderers")

			// check block was committed in all orderers
			assertBlockReceptionInAllOrderers(network.Orderers[1:], peer, network, "testchannel", currentBlockNumber)

			for _, oProc := range []ifrit.Process{o1Proc, o2Proc, o3Proc, o4Proc} {
				if oProc != nil {
					oProc.Signal(syscall.SIGTERM)
					Eventually(oProc.Wait(), network.EventuallyTimeout).Should(Receive())
				}
			}

			runOrderers()

			// === Step 6: Waiting for followers to see the leader, again ===
			By("6) Waiting for followers to see the leader, again")
			Eventually(o2Runner.Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1 channel=testchannel"))
			Eventually(o3Runner.Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1 channel=testchannel"))
			Eventually(o4Runner.Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1 channel=testchannel"))

			// === Step 7: Config update on standard channel, State=NORMAL, exit maintenance-mode ===
			By("7) Exit maintenance mode")
			config, updatedConfig = prepareTransition(network, peer, o1, "testchannel",
				"BFT", protosorderer.ConsensusType_STATE_MAINTENANCE,
				"BFT", bftMetadata, protosorderer.ConsensusType_STATE_NORMAL, 1)
			currentBlockNumber = nwo.CurrentConfigBlockNumber(network, peer, o1, "testchannel")
			nwo.UpdateOrdererConfig(network, o1, "testchannel", config, updatedConfig, peer, o1)

			// === Step 8: Run a transaction to ensure BFT works ===
			By("8) Running a transaction with orderer1 to ensure BFT works and check that the tx was committed in all orderers")
			env = CreateBroadcastEnvelope(network, o1, "testchannel", []byte("foo"))
			resp, err = ordererclient.Broadcast(network, o1, env)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Status).To(Equal(common.Status_SUCCESS))

			// check block was successfully committed in all orderers
			assertBlockReceptionInAllOrderers(network.Orderers[1:], peer, network, "testchannel", currentBlockNumber)
		})
	})

	Describe("Enforcing config updates during Raft to BFT migration", func() {
		var (
			o1   *nwo.Orderer
			o2   *nwo.Orderer
			o3   *nwo.Orderer
			o4   *nwo.Orderer
			peer *nwo.Peer
		)

		BeforeEach(func() {
			networkConfig := nwo.MultiNodeEtcdRaft()
			networkConfig.Orderers = append(networkConfig.Orderers, &nwo.Orderer{Name: "orderer4", Organization: "OrdererOrg"})
			networkConfig.Profiles[0].Orderers = []string{"orderer1", "orderer2", "orderer3", "orderer4"}

			network = nwo.New(networkConfig, testDir, client, StartPort(), components)

			o1, o2, o3, o4 = network.Orderer("orderer1"), network.Orderer("orderer2"), network.Orderer("orderer3"), network.Orderer("orderer4")
			network.GenerateConfigTree()
			network.Bootstrap()

			runOrderers := func() {
				o1Runner = network.OrdererRunner(o1)
				o2Runner = network.OrdererRunner(o2)
				o3Runner = network.OrdererRunner(o3)
				o4Runner = network.OrdererRunner(o4)

				o1Runner.Command.Env = append(o1Runner.Command.Env, "FABRIC_LOGGING_SPEC=orderer.common.cluster=debug:orderer.consensus.smartbft=debug:policies.ImplicitOrderer=debug")
				o2Runner.Command.Env = append(o2Runner.Command.Env, "FABRIC_LOGGING_SPEC=orderer.common.cluster=debug:orderer.consensus.smartbft=debug:policies.ImplicitOrderer=debug")
				o3Runner.Command.Env = append(o3Runner.Command.Env, "FABRIC_LOGGING_SPEC=orderer.common.cluster=debug:orderer.consensus.smartbft=debug:policies.ImplicitOrderer=debug")
				o4Runner.Command.Env = append(o4Runner.Command.Env, "FABRIC_LOGGING_SPEC=orderer.common.cluster=debug:orderer.consensus.smartbft=debug:policies.ImplicitOrderer=debug")

				o1Proc = ifrit.Invoke(o1Runner)
				o2Proc = ifrit.Invoke(o2Runner)
				o3Proc = ifrit.Invoke(o3Runner)
				o4Proc = ifrit.Invoke(o4Runner)

				Eventually(o1Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
				Eventually(o2Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
				Eventually(o3Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
				Eventually(o4Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			}

			runOrderers()

			channelparticipation.JoinOrderersAppChannelCluster(network, "testchannel", o1, o2, o3, o4)
			FindLeader([]*ginkgomon.Runner{o1Runner, o2Runner, o3Runner, o4Runner})

			peer = network.Peer("Org1", "peer0")
		})

		// This test executes the "green path" migration config updates on an etcdraft based system, on a standard
		// channel, and verifies that these config updates have the desired effect.
		// The green path is entering maintenance mode, and then changing the consensus type.
		// In maintenance mode we check that normal transactions are blocked.
		// We also check that after entering maintenance mode, we can exit it without making any changes - the "abort path".
		It("executes raft2fbft green path with extra checks", func() {
			// === The abort path ======================================================================================

			// === Step 1: Config update on standard channel, MAINTENANCE ===
			By("1) Config update on standard channel, State=MAINTENANCE, enter maintenance-mode")
			config, updatedConfig := prepareTransition(network, peer, o1, "testchannel",
				"etcdraft", protosorderer.ConsensusType_STATE_NORMAL,
				"etcdraft", nil, protosorderer.ConsensusType_STATE_MAINTENANCE, 0)
			nwo.UpdateOrdererConfig(network, o1, "testchannel", config, updatedConfig, peer, o1)

			By("1) Verify: standard channel config changed")
			std1EntryBlockNum := nwo.CurrentConfigBlockNumber(network, peer, o1, "testchannel")
			Expect(std1EntryBlockNum).ToNot(Equal(0))
			config = nwo.GetConfig(network, peer, o1, "testchannel")
			consensusTypeValue := extractOrdererConsensusType(config)
			validateConsensusTypeValue(consensusTypeValue, "etcdraft", protosorderer.ConsensusType_STATE_MAINTENANCE)

			By("1) Verify: Normal TX's on standard channel are blocked")
			assertTxFailed(network, o1, "testchannel")

			// In maintenance mode deliver requests are open to those entities that satisfy the /Channel/Orderer/Readers policy
			By("1) Verify: delivery request from peer is blocked")
			err := checkPeerDeliverRequest(o1, peer, network, "testchannel")
			Expect(err).To(MatchError(errors.New("FORBIDDEN")))

			// === Step 2: config update on standard channel, State=NORMAL, abort ===
			By("2) Config update on standard channel, State=NORMAL, exit maintenance-mode - abort path")
			config, updatedConfig = prepareTransition(network, peer, o1, "testchannel",
				"etcdraft", protosorderer.ConsensusType_STATE_MAINTENANCE,
				"etcdraft", nil, protosorderer.ConsensusType_STATE_NORMAL, 0)
			nwo.UpdateOrdererConfig(network, o1, "testchannel", config, updatedConfig, peer, o1)

			By("2) Verify: standard channel config changed")
			std1BlockNum := nwo.CurrentConfigBlockNumber(network, peer, o1, "testchannel")
			Expect(std1BlockNum).To(Equal(std1EntryBlockNum + 1))

			By("2) Verify: standard channel delivery requests from peer unblocked")
			err = checkPeerDeliverRequest(o1, peer, network, "testchannel")
			Expect(err).NotTo(HaveOccurred())

			By("2) Verify: Normal TX's on standard channel are permitted again")
			assertBlockCreation(network, o1, nil, "testchannel", std1EntryBlockNum+2)

			// === The green path ======================================================================================

			// === Step 3: Config update on standard channel, MAINTENANCE, again ===
			By("3) Config update on standard channel, State=MAINTENANCE, enter maintenance-mode again")
			config, updatedConfig = prepareTransition(network, peer, o1, "testchannel",
				"etcdraft", protosorderer.ConsensusType_STATE_NORMAL,
				"etcdraft", nil, protosorderer.ConsensusType_STATE_MAINTENANCE, 0)
			nwo.UpdateOrdererConfig(network, o1, "testchannel", config, updatedConfig, peer, o1)

			By("3) Verify: standard channel config changed")
			std1EntryBlockNum = nwo.CurrentConfigBlockNumber(network, peer, o1, "testchannel")
			Expect(std1EntryBlockNum).To(Equal(uint64(4)))

			config = nwo.GetConfig(network, peer, o1, "testchannel")
			consensusTypeValue = extractOrdererConsensusType(config)
			validateConsensusTypeValue(consensusTypeValue, "etcdraft", protosorderer.ConsensusType_STATE_MAINTENANCE)

			By("3) Verify: delivery request from peer is blocked")
			err = checkPeerDeliverRequest(o1, peer, network, "testchannel")
			Expect(err).To(MatchError(errors.New("FORBIDDEN")))

			By("3) Verify: Normal TX's on standard channel are blocked")
			assertTxFailed(network, o1, "testchannel")

			// === Step 4: config update on standard channel, State=MAINTENANCE, type=BFT ===
			bftMetadata := protoutil.MarshalOrPanic(prepareBftMetadata())

			By("4) config update on standard channel, State=MAINTENANCE, type=BFT")
			config, updatedConfig = prepareTransition(network, peer, o1, "testchannel",
				"etcdraft", protosorderer.ConsensusType_STATE_MAINTENANCE,
				"BFT", bftMetadata, protosorderer.ConsensusType_STATE_MAINTENANCE, 1)
			nwo.UpdateOrdererConfig(network, o1, "testchannel", config, updatedConfig, peer, o1)

			By("4) Verify: standard channel config changed")
			std1EntryBlockNum = nwo.CurrentConfigBlockNumber(network, peer, o1, "testchannel")
			Expect(std1EntryBlockNum).To(Equal(uint64(5)))

			By("4) Verify: validate consensus type value in all orderers o1, o2, o3, o4")
			for _, o := range []*nwo.Orderer{o1, o2, o3, o4} {
				config = nwo.GetConfig(network, peer, o, "testchannel")
				consensusTypeValue = extractOrdererConsensusType(config)
				validateConsensusTypeValue(consensusTypeValue, "BFT", protosorderer.ConsensusType_STATE_MAINTENANCE)
			}
		})

		// This test executes the migration flow and checks that forbidden transitions are rejected.
		// These transitions are enforced by the maintenance filter:
		// - Entry to & exit from maintenance mode can only change ConsensusType.State.
		// - In maintenance mode one can only change ConsensusType.Type & ConsensusType.Metadata & Orderers.ConsenterMapping.
		// - ConsensusType.Type can only change from "etcdraft" to "BFT", and only in maintenance mode.
		It("executes raft2bft forbidden transitions", func() {
			// === Step 1: ===
			By("1) Config update on standard channel, changing both ConsensusType State & Type is forbidden")
			assertTransitionFailed(network, peer, o1, "testchannel",
				"etcdraft", protosorderer.ConsensusType_STATE_NORMAL,
				"BFT", nil, protosorderer.ConsensusType_STATE_MAINTENANCE, 1)

			// === Step 2: ===
			By("2) Config update on standard channel, both ConsensusType State & some other value is forbidden")
			config, updatedConfig := prepareTransition(network, peer, o1, "testchannel",
				"etcdraft", protosorderer.ConsensusType_STATE_NORMAL,
				"etcdraft", nil, protosorderer.ConsensusType_STATE_MAINTENANCE, 0)
			updateConfigWithBatchTimeout(updatedConfig)
			updateOrdererConfigFailed(network, o1, "testchannel", config, updatedConfig, peer, o1)

			// === Step 3: ===
			By("3) Config update on standard channel, State=MAINTENANCE, enter maintenance-mode")
			config, updatedConfig = prepareTransition(network, peer, o1, "testchannel",
				"etcdraft", protosorderer.ConsensusType_STATE_NORMAL,
				"etcdraft", nil, protosorderer.ConsensusType_STATE_MAINTENANCE, 0)
			nwo.UpdateOrdererConfig(network, o1, "testchannel", config, updatedConfig, peer, o1)

			By("3) Verify: standard channel config changed")
			std1StartBlockNum := nwo.CurrentConfigBlockNumber(network, peer, o1, "testchannel")
			Expect(std1StartBlockNum).ToNot(Equal(0))
			config = nwo.GetConfig(network, peer, o1, "testchannel")
			consensusTypeValue := extractOrdererConsensusType(config)
			validateConsensusTypeValue(consensusTypeValue, "etcdraft", protosorderer.ConsensusType_STATE_MAINTENANCE)

			// === Step 4: ===
			By("4) Config update on standard channel, change ConsensusType.Type to unsupported type, forbidden")
			assertTransitionFailed(network, peer, o1, "testchannel",
				"etcdraft", protosorderer.ConsensusType_STATE_MAINTENANCE,
				"hesse", nil, protosorderer.ConsensusType_STATE_MAINTENANCE, 0)

			// === Step 5: ===
			By("5) Config update on standard channel, change ConsensusType.Type and State, forbidden")
			assertTransitionFailed(network, peer, o1, "testchannel",
				"etcdraft", protosorderer.ConsensusType_STATE_MAINTENANCE,
				"BFT", nil, protosorderer.ConsensusType_STATE_NORMAL, 1)

			// === Step 6: Config update on standard channel, changing ConsensusType.Type with invalid bft metadata ===
			By("6) changing ConsensusType.Type with invalid BFT metadata")
			invalidBftMetadata := protoutil.MarshalOrPanic(prepareInvalidBftMetadata())
			assertTransitionFailed(network, peer, o1, "testchannel",
				"etcdraft", protosorderer.ConsensusType_STATE_MAINTENANCE,
				"BFT", invalidBftMetadata, protosorderer.ConsensusType_STATE_MAINTENANCE, 1)

			By("6) changing ConsensusType.Type with missing BFT metadata")
			assertTransitionFailed(network, peer, o1, "testchannel",
				"etcdraft", protosorderer.ConsensusType_STATE_MAINTENANCE,
				"BFT", nil, protosorderer.ConsensusType_STATE_MAINTENANCE, 1)

			By("6) changing ConsensusType.Type with missing bft consenters mapping")
			bftMetadata := protoutil.MarshalOrPanic(prepareBftMetadata())
			assertTransitionFailed(network, peer, o1, "testchannel",
				"etcdraft", protosorderer.ConsensusType_STATE_MAINTENANCE,
				"BFT", bftMetadata, protosorderer.ConsensusType_STATE_MAINTENANCE, 0)

			By("6) changing ConsensusType.Type with corrupt bft consenters mapping")
			assertTransitionFailed(network, peer, o1, "testchannel",
				"etcdraft", protosorderer.ConsensusType_STATE_MAINTENANCE,
				"BFT", bftMetadata, protosorderer.ConsensusType_STATE_MAINTENANCE, 2)

			// === Step 7: Config update on standard channel, changing both ConsensusType.Type and other value is permitted ===
			By("7) changing both ConsensusType.Type and other value is permitted")
			// Change consensus type and batch-timeout
			config, updatedConfig = prepareTransition(network, peer, o1, "testchannel",
				"etcdraft", protosorderer.ConsensusType_STATE_MAINTENANCE,
				"BFT", bftMetadata, protosorderer.ConsensusType_STATE_MAINTENANCE, 1)
			updateConfigWithBatchTimeout(updatedConfig)
			nwo.UpdateOrdererConfig(network, o1, "testchannel", config, updatedConfig, peer, o1)

			By("7) Verify: standard channel config changed")
			std1BlockNum := nwo.CurrentConfigBlockNumber(network, peer, o1, "testchannel")
			Expect(std1BlockNum).To(Equal(std1StartBlockNum + 1))
			config = nwo.GetConfig(network, peer, o1, "testchannel")
			consensusTypeValue = extractOrdererConsensusType(config)
			validateConsensusTypeValue(consensusTypeValue, "BFT", protosorderer.ConsensusType_STATE_MAINTENANCE)

			// === Step 8: ===
			By("8) Config update on standard channel, changing value other than ConsensusType.Type is permitted")
			updatedConfig = proto.Clone(config).(*common.Config)
			updateConfigWithBatchTimeout(updatedConfig)
			nwo.UpdateOrdererConfig(network, o1, "testchannel", config, updatedConfig, peer, o1)

			By("8) Verify: standard channel config changed")
			std1BlockNum = nwo.CurrentConfigBlockNumber(network, peer, o1, "testchannel")
			Expect(std1BlockNum).To(Equal(std1StartBlockNum + 2))

			// === Step 9: ===
			By("9) Config update on standard channel, both ConsensusType State & some other value is forbidden")
			config, updatedConfig = prepareTransition(network, peer, o1, "testchannel",
				"BFT", protosorderer.ConsensusType_STATE_MAINTENANCE,
				"BFT", nil, protosorderer.ConsensusType_STATE_NORMAL, 1)
			updateConfigWithBatchTimeout(updatedConfig)
			updateOrdererConfigFailed(network, o1, "testchannel", config, updatedConfig, peer, o1)
		})

		// Note:
		// This test aims to check some other config value in maintenance mode, reboot, and exit maintenance mode.
		// The config value is unrelated to consensus type migration.
		It("Config value change unrelated to consensus type migration", func() {
			// === Step 1: Config update on standard channel, MAINTENANCE ===
			By("1) Config update on standard channel, State=MAINTENANCE")
			config, updatedConfig := prepareTransition(network, peer, o1, "testchannel",
				"etcdraft", protosorderer.ConsensusType_STATE_NORMAL,
				"etcdraft", nil, protosorderer.ConsensusType_STATE_MAINTENANCE, 0)
			nwo.UpdateOrdererConfig(network, o1, "testchannel", config, updatedConfig, peer, o1)

			By("1) Verify: standard channel config changed")
			chan1StartBlockNum := nwo.CurrentConfigBlockNumber(network, peer, o1, "testchannel")
			Expect(chan1StartBlockNum).ToNot(Equal(0))

			config = nwo.GetConfig(network, peer, o1, "testchannel")
			consensusTypeValue := extractOrdererConsensusType(config)
			validateConsensusTypeValue(consensusTypeValue, "etcdraft", protosorderer.ConsensusType_STATE_MAINTENANCE)

			By("1) Verify: Normal TX's on standard channel are blocked")
			assertTxFailed(network, o1, "testchannel")

			// In maintenance mode deliver requests are open to those entities that satisfy the /Channel/Orderer/Readers policy
			By("1) Verify: delivery request from peer is blocked")
			err := checkPeerDeliverRequest(o1, peer, network, "testchannel")
			Expect(err).To(MatchError(errors.New("FORBIDDEN")))

			// === Step 2: config update on standard channel, State=MAINTENANCE, type=etcdraft ===
			By("2) Config update on standard channel, State=MAINTENANCE, type=etcdraft")
			config = nwo.GetConfig(network, peer, o1, "testchannel")
			consensusTypeValue = extractOrdererConsensusType(config)
			validateConsensusTypeValue(consensusTypeValue, "etcdraft", protosorderer.ConsensusType_STATE_MAINTENANCE)
			updatedConfig = proto.Clone(config).(*common.Config)
			updateConfigWithBatchTimeout(updatedConfig)
			nwo.UpdateOrdererConfig(network, o1, "testchannel", config, updatedConfig, peer, o1)

			By("2) Verify: standard channel config changed")
			chan1BlockNum := nwo.CurrentConfigBlockNumber(network, peer, o1, "testchannel")
			Expect(chan1BlockNum).To(Equal(chan1StartBlockNum + 1))

			// === Step 3: kill ===
			By("3) killing orderer1")
			o1Proc.Signal(syscall.SIGKILL)
			Eventually(o1Proc.Wait(), network.EventuallyTimeout).Should(Receive(MatchError("exit status 137")))

			// === Step 4: restart ===
			By("4) restarting orderer1")
			network.Consensus.Type = "etcdraft"

			o1Runner = network.OrdererRunner(o1)
			o1Proc = ifrit.Invoke(o1Runner)

			Eventually(o1Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())

			assertBlockReception(
				map[string]int{
					"testchannel": int(chan1BlockNum),
				},
				[]*nwo.Orderer{o1},
				peer,
				network,
			)

			Eventually(o1Runner.Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Raft leader changed: 0 -> "))
			Eventually(o1Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())

			By("5) Release - executing config transaction on standard channel with restarted orderer")
			config, updatedConfig = prepareTransition(network, peer, o1, "testchannel",
				"etcdraft", protosorderer.ConsensusType_STATE_MAINTENANCE,
				"etcdraft", nil, protosorderer.ConsensusType_STATE_NORMAL, 0)
			nwo.UpdateOrdererConfig(network, o1, "testchannel", config, updatedConfig, peer, o1)

			By("6) Verify: standard channel config changed")
			chan1BlockNum = nwo.CurrentConfigBlockNumber(network, peer, o1, "testchannel")
			Expect(chan1BlockNum).To(Equal(chan1StartBlockNum + 2))

			By("7) Executing transaction on standard channel with restarted orderer")
			assertBlockCreation(network, o1, peer, "testchannel", chan1StartBlockNum+3)
			assertBlockCreation(network, o1, nil, "testchannel", chan1StartBlockNum+4)
		})
	})
})

func validateConsensusTypeValue(value *protosorderer.ConsensusType, cType string, state protosorderer.ConsensusType_State) {
	Expect(value.Type).To(Equal(cType))
	Expect(value.State).To(Equal(state))
}

func extractOrdererConsensusType(config *common.Config) *protosorderer.ConsensusType {
	var consensusTypeValue protosorderer.ConsensusType
	consensusTypeConfigValue := config.ChannelGroup.Groups["Orderer"].Values["ConsensusType"]
	err := proto.Unmarshal(consensusTypeConfigValue.Value, &consensusTypeValue)
	Expect(err).NotTo(HaveOccurred())
	return &consensusTypeValue
}

func updateConfigWithConsensusType(
	consensusType string,
	consensusMetadata []byte,
	migState protosorderer.ConsensusType_State,
	updatedConfig *common.Config,
	consensusTypeValue *protosorderer.ConsensusType,
) {
	if consensusTypeValue.Type != consensusType {
		consensusTypeValue.Metadata = consensusMetadata
	}
	consensusTypeValue.Type = consensusType
	consensusTypeValue.State = migState
	updatedConfig.ChannelGroup.Groups["Orderer"].Values["ConsensusType"] = &common.ConfigValue{
		ModPolicy: "Admins",
		Value:     protoutil.MarshalOrPanic(consensusTypeValue),
	}
}

func updateConfigWithBatchTimeout(updatedConfig *common.Config) {
	batchTimeoutConfigValue := updatedConfig.ChannelGroup.Groups["Orderer"].Values["BatchTimeout"]
	batchTimeoutValue := new(protosorderer.BatchTimeout)
	err := proto.Unmarshal(batchTimeoutConfigValue.Value, batchTimeoutValue)
	Expect(err).NotTo(HaveOccurred())
	toDur, err := time.ParseDuration(batchTimeoutValue.Timeout)
	Expect(err).NotTo(HaveOccurred())
	toDur = toDur + time.Duration(100000000)
	batchTimeoutValue.Timeout = toDur.String()
	By(fmt.Sprintf("Increasing BatchTimeout to %s", batchTimeoutValue.Timeout))
	updatedConfig.ChannelGroup.Groups["Orderer"].Values["BatchTimeout"] = &common.ConfigValue{
		ModPolicy: "Admins",
		Value:     protoutil.MarshalOrPanic(batchTimeoutValue),
	}
}

func checkPeerDeliverRequest(o *nwo.Orderer, submitter *nwo.Peer, network *nwo.Network, channelName string) error {
	c := commands.ChannelFetch{
		ChannelID:  channelName,
		Block:      "newest",
		OutputFile: "/dev/null",
		Orderer:    network.OrdererAddress(o, nwo.ListenPort),
	}

	sess, err := network.PeerUserSession(submitter, "User1", c)
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit())
	sessErr := string(sess.Err.Contents())
	sessExitCode := sess.ExitCode()
	if sessExitCode != 0 && strings.Contains(sessErr, "FORBIDDEN") {
		return errors.New("FORBIDDEN")
	}
	if sessExitCode == 0 && strings.Contains(sessErr, "Received block: ") {
		return nil
	}

	return fmt.Errorf("Unexpected result: ExitCode=%d, Err=%s", sessExitCode, sessErr)
}

func updateOrdererConfigFailed(n *nwo.Network, orderer *nwo.Orderer, channel string, current, updated *common.Config, peer *nwo.Peer, additionalSigners ...*nwo.Orderer) {
	sess := nwo.UpdateOrdererConfigSession(n, orderer, channel, current, updated, peer, additionalSigners...)
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(1))
	Expect(sess.Err).NotTo(gbytes.Say("Successfully submitted channel update"))
}

func updateOrdererEndpointsConfigFails(n *nwo.Network, orderer *nwo.Orderer, channel string, current, updated *common.Config, peer *nwo.Peer, additionalSigners ...*nwo.Peer) {
	tempDir, err := os.MkdirTemp("", "updateConfig")
	Expect(err).NotTo(HaveOccurred())
	defer os.RemoveAll(tempDir)

	// compute update
	configUpdate, err := update.Compute(current, updated)
	Expect(err).NotTo(HaveOccurred())
	configUpdate.ChannelId = channel

	signedEnvelope, err := protoutil.CreateSignedEnvelope(
		common.HeaderType_CONFIG_UPDATE,
		channel,
		nil, // local signer
		&common.ConfigUpdateEnvelope{ConfigUpdate: protoutil.MarshalOrPanic(configUpdate)},
		0, // message version
		0, // epoch
	)
	Expect(err).NotTo(HaveOccurred())
	Expect(signedEnvelope).NotTo(BeNil())

	updateFile := filepath.Join(tempDir, "update.pb")
	err = os.WriteFile(updateFile, protoutil.MarshalOrPanic(signedEnvelope), 0o600)
	Expect(err).NotTo(HaveOccurred())

	for _, signer := range additionalSigners {
		sess, err := n.PeerAdminSession(signer, commands.SignConfigTx{
			File:       updateFile,
			ClientAuth: n.ClientAuthRequired,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	}

	sess, err := n.OrdererAdminSession(orderer, peer, commands.SignConfigTx{
		File:       updateFile,
		ClientAuth: n.ClientAuthRequired,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))

	sess, err = n.PeerAdminSession(peer, commands.ChannelUpdate{
		ChannelID:  channel,
		Orderer:    n.OrdererAddress(orderer, nwo.ListenPort),
		File:       updateFile,
		ClientAuth: n.ClientAuthRequired,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(1))
	Expect(sess.Err).To(gbytes.Say("error applying config update to existing channel 'testchannel': initializing channelconfig failed: global OrdererAddresses are not allowed with V3_0 capability, use org specific addresses only"))
}

func prepareTransition(
	network *nwo.Network, peer *nwo.Peer, orderer *nwo.Orderer, channel string, // Auxiliary
	fromConsensusType string, fromMigState protosorderer.ConsensusType_State, // From
	toConsensusType string, toConsensusMetadata []byte, toMigState protosorderer.ConsensusType_State, toConsenterMapping int, // To
) (current, updated *common.Config) {
	current = nwo.GetConfig(network, peer, orderer, channel)
	updated = proto.Clone(current).(*common.Config)
	consensusTypeValue := extractOrdererConsensusType(current)
	validateConsensusTypeValue(consensusTypeValue, fromConsensusType, fromMigState)
	updateConfigWithConsensusType(toConsensusType, toConsensusMetadata, toMigState, updated, consensusTypeValue)
	if toConsensusType == "BFT" {
		// 1: updating valid consenters mapping
		// 2: updating invalid consenters mapping
		if toConsenterMapping == 1 {
			updateBFTOrderersConfig(network, updated)
		} else if toConsenterMapping == 2 {
			updateInvalidBFTOrderersConfig(network, updated)
		}
	}
	return current, updated
}

func assertTransitionFailed(
	network *nwo.Network, peer *nwo.Peer, orderer *nwo.Orderer, channel string, // Auxiliary
	fromConsensusType string, fromMigState protosorderer.ConsensusType_State, // From
	toConsensusType string, toConsensusMetadata []byte, toMigState protosorderer.ConsensusType_State, toConsenterMapping int, // To
) {
	current, updated := prepareTransition(
		network, peer, orderer, channel,
		fromConsensusType, fromMigState,
		toConsensusType, toConsensusMetadata, toMigState, toConsenterMapping)
	updateOrdererConfigFailed(network, orderer, channel, current, updated, peer, orderer)
}

func assertBlockCreation(network *nwo.Network, orderer *nwo.Orderer, peer *nwo.Peer, channelID string, blkNum uint64) {
	var signer *nwo.SigningIdentity
	signer = network.OrdererUserSigner(orderer, "Admin")
	if peer != nil {
		signer = network.PeerUserSigner(peer, "Admin")
	}
	env := createBroadcastEnvelope(network, signer, channelID, []byte("hola"))
	resp, err := ordererclient.Broadcast(network, orderer, env)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.Status).To(Equal(common.Status_SUCCESS))

	denv := createDeliverEnvelope(network, signer, blkNum, channelID)
	blk, err := ordererclient.Deliver(network, orderer, denv)
	Expect(err).NotTo(HaveOccurred())
	Expect(blk).ToNot(BeNil())
}

func assertTxFailed(network *nwo.Network, orderer *nwo.Orderer, channelID string) {
	signer := network.OrdererUserSigner(orderer, "Admin")
	env := createBroadcastEnvelope(network, signer, channelID, []byte("hola"))
	resp, err := ordererclient.Broadcast(network, orderer, env)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.Status).To(Equal(common.Status_SERVICE_UNAVAILABLE))
	Expect(resp.Info).To(Equal("normal transactions are rejected: maintenance mode"))
}

func createBroadcastEnvelope(n *nwo.Network, signer *nwo.SigningIdentity, channel string, data []byte) *common.Envelope {
	env, err := protoutil.CreateSignedEnvelope(
		common.HeaderType_MESSAGE,
		channel,
		signer,
		&common.Envelope{Payload: data},
		0,
		0,
	)
	Expect(err).NotTo(HaveOccurred())

	return env
}

// CreateDeliverEnvelope creates a deliver env to seek for specified block.
func createDeliverEnvelope(n *nwo.Network, signer *nwo.SigningIdentity, blkNum uint64, channel string) *common.Envelope {
	specified := &protosorderer.SeekPosition{
		Type: &protosorderer.SeekPosition_Specified{
			Specified: &protosorderer.SeekSpecified{Number: blkNum},
		},
	}
	env, err := protoutil.CreateSignedEnvelope(
		common.HeaderType_DELIVER_SEEK_INFO,
		channel,
		signer,
		&protosorderer.SeekInfo{
			Start:    specified,
			Stop:     specified,
			Behavior: protosorderer.SeekInfo_BLOCK_UNTIL_READY,
		},
		0,
		0,
	)
	Expect(err).NotTo(HaveOccurred())

	return env
}

func updateBFTOrderersConfig(network *nwo.Network, config *common.Config) {
	orderersVal := &common.Orderers{
		ConsenterMapping: computeConsenterMappings(network),
	}

	policies.EncodeBFTBlockVerificationPolicy(orderersVal.ConsenterMapping, config.ChannelGroup.Groups["Orderer"])

	config.ChannelGroup.Groups["Orderer"].Values["Orderers"] = &common.ConfigValue{
		Value:     protoutil.MarshalOrPanic(orderersVal),
		ModPolicy: "/Channel/Orderer/Admins",
	}
}

func updateInvalidBFTOrderersConfig(network *nwo.Network, config *common.Config) {
	orderersVal := &common.Orderers{
		ConsenterMapping: computeConsenterMappings(network),
	}

	orderersVal.ConsenterMapping[0].Port = orderersVal.ConsenterMapping[0].Port + 1

	policies.EncodeBFTBlockVerificationPolicy(orderersVal.ConsenterMapping, config.ChannelGroup.Groups["Orderer"])

	config.ChannelGroup.Groups["Orderer"].Values["Orderers"] = &common.ConfigValue{
		Value:     protoutil.MarshalOrPanic(orderersVal),
		ModPolicy: "/Channel/Orderer/Admins",
	}
}

func computeConsenterMappings(network *nwo.Network) []*common.Consenter {
	var consenters []*common.Consenter

	for i, orderer := range network.Orderers {
		ecertPath := network.OrdererCert(orderer)
		ecert, err := os.ReadFile(ecertPath)
		Expect(err).To(Not(HaveOccurred()))

		tlsDir := network.OrdererLocalTLSDir(orderer)
		tlsPath := filepath.Join(tlsDir, "server.crt")
		tlsCert, err := os.ReadFile(tlsPath)
		Expect(err).To(Not(HaveOccurred()))

		consenters = append(consenters, &common.Consenter{
			ServerTlsCert: tlsCert,
			ClientTlsCert: tlsCert,
			MspId:         network.Organization(orderer.Organization).MSPID,
			Host:          "127.0.0.1",
			Port:          uint32(network.PortsByOrdererID[orderer.ID()][nwo.ClusterPort]),
			Id:            uint32(i + 1),
			Identity:      ecert,
		})
	}

	return consenters
}

func CreateBroadcastEnvelope(n *nwo.Network, entity interface{}, channel string, data []byte) *common.Envelope {
	var signer *nwo.SigningIdentity
	switch creator := entity.(type) {
	case *nwo.Peer:
		signer = n.PeerUserSigner(creator, "Admin")
	case *nwo.Orderer:
		signer = n.OrdererUserSigner(creator, "Admin")
	}
	Expect(signer).NotTo(BeNil())

	env, err := protoutil.CreateSignedEnvelope(
		common.HeaderType_ENDORSER_TRANSACTION,
		channel,
		signer,
		&common.Envelope{Payload: data},
		0,
		0,
	)
	Expect(err).NotTo(HaveOccurred())

	return env
}

// assertBlockReception asserts that the given orderers have the expected
// newest block number for the specified channels
func assertBlockReceptionInAllOrderers(orderers []*nwo.Orderer, peer *nwo.Peer, network *nwo.Network, channelId string, currentBlockNumber uint64) {
	for _, orderer := range orderers {
		ccb := func() uint64 {
			return nwo.CurrentConfigBlockNumber(network, peer, orderer, channelId)
		}
		Eventually(ccb, network.EventuallyTimeout).Should(BeNumerically(">", currentBlockNumber))

	}
}

// prepareBftMetadata prepare the bft consensusType.Metadata
func prepareBftMetadata() *smartbft.Options {
	bftMetadata := &smartbft.Options{
		RequestBatchMaxCount:      types.DefaultConfig.RequestBatchMaxCount,
		RequestBatchMaxBytes:      types.DefaultConfig.RequestBatchMaxBytes,
		RequestBatchMaxInterval:   types.DefaultConfig.RequestBatchMaxInterval.String(),
		IncomingMessageBufferSize: types.DefaultConfig.IncomingMessageBufferSize,
		RequestPoolSize:           types.DefaultConfig.RequestPoolSize,
		RequestForwardTimeout:     types.DefaultConfig.RequestForwardTimeout.String(),
		RequestComplainTimeout:    types.DefaultConfig.RequestComplainTimeout.String(),
		RequestAutoRemoveTimeout:  types.DefaultConfig.RequestAutoRemoveTimeout.String(),
		ViewChangeResendInterval:  types.DefaultConfig.ViewChangeResendInterval.String(),
		ViewChangeTimeout:         types.DefaultConfig.ViewChangeTimeout.String(),
		LeaderHeartbeatTimeout:    types.DefaultConfig.LeaderHeartbeatTimeout.String(),
		LeaderHeartbeatCount:      types.DefaultConfig.LeaderHeartbeatCount,
		CollectTimeout:            types.DefaultConfig.CollectTimeout.String(),
		SyncOnStart:               types.DefaultConfig.SyncOnStart,
		SpeedUpViewChange:         types.DefaultConfig.SpeedUpViewChange,
	}
	return bftMetadata
}

// prepareBftMetadata prepare the bft consensusType.Metadata
func prepareInvalidBftMetadata() *smartbft.Options {
	bftMetadata := &smartbft.Options{
		RequestBatchMaxCount:      0,
		RequestBatchMaxBytes:      types.DefaultConfig.RequestBatchMaxBytes,
		RequestBatchMaxInterval:   types.DefaultConfig.RequestBatchMaxInterval.String(),
		IncomingMessageBufferSize: types.DefaultConfig.IncomingMessageBufferSize,
		RequestPoolSize:           types.DefaultConfig.RequestPoolSize,
		RequestForwardTimeout:     types.DefaultConfig.RequestForwardTimeout.String(),
		RequestComplainTimeout:    types.DefaultConfig.RequestComplainTimeout.String(),
		RequestAutoRemoveTimeout:  types.DefaultConfig.RequestAutoRemoveTimeout.String(),
		ViewChangeResendInterval:  types.DefaultConfig.ViewChangeResendInterval.String(),
		ViewChangeTimeout:         types.DefaultConfig.ViewChangeTimeout.String(),
		LeaderHeartbeatTimeout:    types.DefaultConfig.LeaderHeartbeatTimeout.String(),
		LeaderHeartbeatCount:      types.DefaultConfig.LeaderHeartbeatCount,
		CollectTimeout:            types.DefaultConfig.CollectTimeout.String(),
		SyncOnStart:               types.DefaultConfig.SyncOnStart,
		SpeedUpViewChange:         types.DefaultConfig.SpeedUpViewChange,
	}
	return bftMetadata
}

func addGlobalLevelEndpointsToConfig(config *common.Config) {
	globalEndpoint := []string{"127.0.0.1:7050"}
	config.ChannelGroup.Values[channelconfig.OrdererAddressesKey] = &common.ConfigValue{
		Value: protoutil.MarshalOrPanic(&common.OrdererAddresses{
			Addresses: globalEndpoint,
		}),
		ModPolicy: "/Channel/Admins",
	}

	topCapabilities := make(map[string]bool)
	topCapabilities[capabilities.ChannelV3_0] = true
	config.ChannelGroup.Values[channelconfig.CapabilitiesKey] = &common.ConfigValue{
		Value:     protoutil.MarshalOrPanic(channelconfig.CapabilitiesValue(topCapabilities).Value()),
		ModPolicy: channelconfig.AdminsPolicyKey,
	}
}

func cleanEndpointsPerOrgFromConfig(config *common.Config) {
	config.ChannelGroup.Groups["Orderer"].Groups["OrdererOrg"].Values[channelconfig.EndpointsKey] = &common.ConfigValue{ModPolicy: "/Channel/Admins"}
}

func addEndpointsPerOrgInConfig(config *common.Config) {
	ordererOrgEndpoint := &common.OrdererAddresses{
		Addresses: []string{"127.0.0.1:7050"},
	}
	config.ChannelGroup.Groups["Orderer"].Groups["OrdererOrg"].Values[channelconfig.EndpointsKey] = &common.ConfigValue{Value: protoutil.MarshalOrPanic(ordererOrgEndpoint), ModPolicy: "/Channel/Application/Writers"}
}

func updateOrdererOrgEndpointsConfigSucceeds(n *nwo.Network, orderer *nwo.Orderer, channel string, current, updated *common.Config, peer *nwo.Peer, additionalSigners ...*nwo.Peer) {
	tempDir, err := os.MkdirTemp("", "updateConfig")
	Expect(err).NotTo(HaveOccurred())
	defer os.RemoveAll(tempDir)

	// compute update
	configUpdate, err := update.Compute(current, updated)
	Expect(err).NotTo(HaveOccurred())
	configUpdate.ChannelId = channel

	signedEnvelope, err := protoutil.CreateSignedEnvelope(
		common.HeaderType_CONFIG_UPDATE,
		channel,
		nil, // local signer
		&common.ConfigUpdateEnvelope{ConfigUpdate: protoutil.MarshalOrPanic(configUpdate)},
		0, // message version
		0, // epoch
	)
	Expect(err).NotTo(HaveOccurred())
	Expect(signedEnvelope).NotTo(BeNil())

	updateFile := filepath.Join(tempDir, "update.pb")
	err = os.WriteFile(updateFile, protoutil.MarshalOrPanic(signedEnvelope), 0o600)
	Expect(err).NotTo(HaveOccurred())

	for _, signer := range additionalSigners {
		sess, err := n.PeerAdminSession(signer, commands.SignConfigTx{
			File:       updateFile,
			ClientAuth: n.ClientAuthRequired,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	}

	sess, err := n.OrdererAdminSession(orderer, peer, commands.SignConfigTx{
		File:       updateFile,
		ClientAuth: n.ClientAuthRequired,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))

	sess, err = n.OrdererAdminSession(orderer, peer, commands.ChannelUpdate{
		ChannelID:  channel,
		Orderer:    n.OrdererAddress(orderer, nwo.ListenPort),
		File:       updateFile,
		ClientAuth: n.ClientAuthRequired,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess.Err).To(gbytes.Say("Successfully submitted channel update"))
}

// UpdateOrdererConfig computes, signs, and submits a configuration update
// which requires orderers signature. the update should fail.
func UpdateOrdererConfigFails(n *nwo.Network, orderer *nwo.Orderer, channel string, current, updated *common.Config, submitter *nwo.Peer, additionalSigners ...*nwo.Orderer) {
	tempDir, err := os.MkdirTemp(n.RootDir, "updateConfig")
	Expect(err).NotTo(HaveOccurred())
	updateFile := filepath.Join(tempDir, "update.pb")
	defer os.RemoveAll(tempDir)

	nwo.ComputeUpdateOrdererConfig(updateFile, n, channel, current, updated, submitter, additionalSigners...)

	Eventually(func() bool {
		sess, err := n.OrdererAdminSession(orderer, submitter, commands.ChannelUpdate{
			ChannelID:  channel,
			Orderer:    n.OrdererAddress(orderer, nwo.ListenPort),
			File:       updateFile,
			ClientAuth: n.ClientAuthRequired,
		})
		Expect(err).NotTo(HaveOccurred())

		sess.Wait(n.EventuallyTimeout)
		if sess.ExitCode() != 0 {
			return false
		}

		return strings.Contains(string(sess.Err.Contents()), "Successfully submitted channel update")
	}, n.EventuallyTimeout).Should(BeFalse())
}
