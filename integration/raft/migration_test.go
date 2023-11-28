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

	"github.com/SmartBFT-Go/consensus/pkg/types"
	"github.com/hyperledger/fabric-protos-go/orderer/smartbft"
	"github.com/hyperledger/fabric/common/policies"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	protosorderer "github.com/hyperledger/fabric-protos-go/orderer"
	protosraft "github.com/hyperledger/fabric-protos-go/orderer/etcdraft"
	"github.com/hyperledger/fabric/integration/channelparticipation"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	"github.com/hyperledger/fabric/integration/ordererclient"
	"github.com/hyperledger/fabric/protoutil"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
	ginkgomon "github.com/tedsuo/ifrit/ginkgomon_v2"
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

	Describe("Raft to BFT migration", func() {
		It("migrates from Raft to BFT", func() {
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

			By("performing operation with orderer1")
			env := CreateBroadcastEnvelope(network, o1, "testchannel", []byte("foo"))
			resp, err := ordererclient.Broadcast(network, o1, env)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Status).To(Equal(common.Status_SUCCESS))

			block := FetchBlock(network, o1, 1, "testchannel")
			Expect(block).NotTo(BeNil())

			By("Change to maintenance mode")

			peer := network.Peer("Org1", "peer0")
			orderer := network.Orderer("orderer1")

			addBFTInConfigTree(network, peer, orderer, "testchannel")

			By("Config update on standard channel, State=MAINTENANCE, enter maintenance-mode")
			config, updatedConfig := prepareTransition(network, peer, orderer, "testchannel",
				"etcdraft", protosorderer.ConsensusType_STATE_NORMAL,
				"etcdraft", nil, protosorderer.ConsensusType_STATE_MAINTENANCE)
			nwo.UpdateOrdererConfig(network, orderer, "testchannel", config, updatedConfig, peer, orderer)

			bftMetadata := protoutil.MarshalOrPanic(&smartbft.Options{
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
			})
			config, updatedConfig = prepareTransition(network, peer, orderer, "testchannel",
				"etcdraft", protosorderer.ConsensusType_STATE_MAINTENANCE,
				"BFT", bftMetadata, protosorderer.ConsensusType_STATE_MAINTENANCE)
			nwo.UpdateOrdererConfig(network, orderer, "testchannel", config, updatedConfig, peer, orderer)

			time.Sleep(time.Second) // TODO: check block was committed in all orderers

			for _, oProc := range []ifrit.Process{o1Proc, o2Proc, o3Proc, o4Proc} {
				if oProc != nil {
					oProc.Signal(syscall.SIGTERM)
					Eventually(oProc.Wait(), network.EventuallyTimeout).Should(Receive())
				}
			}

			runOrderers()

			// TODO:
			By("Waiting for followers to see the leader, again")
			Eventually(o2Runner.Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1 channel=testchannel"))
			Eventually(o3Runner.Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1 channel=testchannel"))
			Eventually(o4Runner.Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1 channel=testchannel"))

			// Exit maintenance mode

			config, updatedConfig = prepareTransition(network, peer, orderer, "testchannel",
				"BFT", protosorderer.ConsensusType_STATE_MAINTENANCE,
				"BFT", bftMetadata, protosorderer.ConsensusType_STATE_NORMAL)
			nwo.UpdateOrdererConfig(network, orderer, "testchannel", config, updatedConfig, peer, orderer)

			// Now, run a transaction to ensure BFT works.
			env = CreateBroadcastEnvelope(network, o1, "testchannel", []byte("foo"))
			resp, err = ordererclient.Broadcast(network, o1, env)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Status).To(Equal(common.Status_SUCCESS))

			time.Sleep(time.Second * 5)

			// TODO: check block was successfully committed in all orderers
		})
	})

	// These tests execute the migration config updates on an etcdraft based system, but do not restart the orderer
	// to a "future-type" consensus-type that currently does not exist. However, these tests try to maintain as much as
	// possible the testing infrastructure for consensus-type migration that existed when "solo" and "kafka" were still
	// supported. When a future type that can be migrated to from raft becomes available, some test-cases within this
	// suite will need to be completed and revised.
	Describe("Raft to future-type migration", func() {
		var (
			orderer            *nwo.Orderer
			peer               *nwo.Peer
			channel1, channel2 string
		)

		BeforeEach(func() {
			network = nwo.New(nwo.MultiChannelEtcdRaft(), testDir, client, StartPort(), components)
			network.GenerateConfigTree()
			network.Bootstrap()

			orderer = network.Orderer("orderer")
			peer = network.Peer("Org1", "peer0")

			channel1 = "testchannel"
			channel2 = "testchannel2"

			o1Runner = network.OrdererRunner(orderer)
			o1Proc = ifrit.Invoke(o1Runner)
			Eventually(o1Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())

			By("Create & join first channel")
			channelparticipation.JoinOrdererAppChannel(network, channel1, orderer, o1Runner)
		})

		// This test executes the "green path" migration config updates on an etcdraft based system, on a standard
		// channel, and verifies that these config updates have the desired effect.
		//
		// The green path is entering maintenance mode, and then changing the consensus type.
		// In maintenance mode we check that normal transactions are blocked.
		//
		// We also check that after entering maintenance mode, we can exit it without making any changes - the "abort path".
		It("executes raft2future green path", func() {
			// === The abort path ======================================================================================

			// === Step 1: Config update on standard channel, MAINTENANCE ===
			By("1) Config update on standard channel, State=MAINTENANCE, enter maintenance-mode")
			config, updatedConfig := prepareTransition(network, peer, orderer, channel1,
				"etcdraft", protosorderer.ConsensusType_STATE_NORMAL,
				"etcdraft", nil, protosorderer.ConsensusType_STATE_MAINTENANCE)
			nwo.UpdateOrdererConfig(network, orderer, channel1, config, updatedConfig, peer, orderer)

			By("1) Verify: standard channel config changed")
			std1EntryBlockNum := nwo.CurrentConfigBlockNumber(network, peer, orderer, channel1)
			Expect(std1EntryBlockNum).ToNot(Equal(0))
			config = nwo.GetConfig(network, peer, orderer, channel1)
			consensusTypeValue := extractOrdererConsensusType(config)
			validateConsensusTypeValue(consensusTypeValue, "etcdraft", protosorderer.ConsensusType_STATE_MAINTENANCE)

			By("1) Verify: Normal TX's on standard channel are blocked")
			assertTxFailed(network, orderer, channel1)

			// In maintenance mode deliver requests are open to those entities that satisfy the /Channel/Orderer/Readers policy
			By("1) Verify: delivery request from peer is blocked")
			err := checkPeerDeliverRequest(orderer, peer, network, channel1)
			Expect(err).To(MatchError(errors.New("FORBIDDEN")))

			// === Step 2: Create a new channel
			By("2) Create & join second channel")
			channelparticipation.JoinOrdererAppChannel(network, channel2, orderer, o1Runner)
			assertBlockCreation(network, orderer, peer, channel2, 1)

			By("2) Verify: delivery request from peer is not blocked on new channel")
			err = checkPeerDeliverRequest(orderer, peer, network, channel2)
			Expect(err).NotTo(HaveOccurred())

			// === Step 3: config update on standard channel, State=NORMAL, abort ===
			By("3) Config update on standard channel, State=NORMAL, exit maintenance-mode - abort path")
			config, updatedConfig = prepareTransition(network, peer, orderer, channel1,
				"etcdraft", protosorderer.ConsensusType_STATE_MAINTENANCE,
				"etcdraft", nil, protosorderer.ConsensusType_STATE_NORMAL)
			nwo.UpdateOrdererConfig(network, orderer, channel1, config, updatedConfig, peer, orderer)

			By("3) Verify: standard channel config changed")
			std1BlockNum := nwo.CurrentConfigBlockNumber(network, peer, orderer, channel1)
			Expect(std1BlockNum).To(Equal(std1EntryBlockNum + 1))

			By("3) Verify: standard channel delivery requests from peer unblocked")
			err = checkPeerDeliverRequest(orderer, peer, network, channel1)
			Expect(err).NotTo(HaveOccurred())

			By("3) Verify: Normal TX's on standard channel are permitted again")
			assertBlockCreation(network, orderer, nil, channel1, 3)

			// === The green path ======================================================================================

			// === Step 4: Config update on standard channel1, MAINTENANCE, again ===
			By("4) Config update on standard channel1, State=MAINTENANCE, enter maintenance-mode again")
			config, updatedConfig = prepareTransition(network, peer, orderer, channel1,
				"etcdraft", protosorderer.ConsensusType_STATE_NORMAL,
				"etcdraft", nil, protosorderer.ConsensusType_STATE_MAINTENANCE)
			nwo.UpdateOrdererConfig(network, orderer, channel1, config, updatedConfig, peer, orderer)

			By("4) Verify: standard channel config changed")
			std1EntryBlockNum = nwo.CurrentConfigBlockNumber(network, peer, orderer, channel1)
			Expect(std1EntryBlockNum).ToNot(Equal(0))

			config = nwo.GetConfig(network, peer, orderer, channel1)
			consensusTypeValue = extractOrdererConsensusType(config)
			validateConsensusTypeValue(consensusTypeValue, "etcdraft", protosorderer.ConsensusType_STATE_MAINTENANCE)

			By("4) Verify: delivery request from peer is blocked")
			err = checkPeerDeliverRequest(orderer, peer, network, channel1)
			Expect(err).To(MatchError(errors.New("FORBIDDEN")))

			By("4) Verify: Normal TX's on standard channel are blocked")
			assertTxFailed(network, orderer, channel1)

			// === Step 5: Config update on standard channel2, MAINTENANCE ===
			By("5) Config update on standard channel2, State=MAINTENANCE, enter maintenance-mode again")
			config, updatedConfig = prepareTransition(network, peer, orderer, channel2,
				"etcdraft", protosorderer.ConsensusType_STATE_NORMAL,
				"etcdraft", nil, protosorderer.ConsensusType_STATE_MAINTENANCE)
			nwo.UpdateOrdererConfig(network, orderer, channel2, config, updatedConfig, peer, orderer)

			By("5) Verify: standard channel config changed")
			std2EntryBlockNum := nwo.CurrentConfigBlockNumber(network, peer, orderer, channel2)
			Expect(std2EntryBlockNum).ToNot(Equal(0))

			config = nwo.GetConfig(network, peer, orderer, channel2)
			consensusTypeValue = extractOrdererConsensusType(config)
			validateConsensusTypeValue(consensusTypeValue, "etcdraft", protosorderer.ConsensusType_STATE_MAINTENANCE)

			By("5) Verify: delivery request from peer is blocked")
			err = checkPeerDeliverRequest(orderer, peer, network, channel2)
			Expect(err).To(MatchError(errors.New("FORBIDDEN")))

			By("5) Verify: Normal TX's on standard channel are blocked")
			assertTxFailed(network, orderer, channel2)

			// Note:
			// The following testing steps should be completed once we have a consensus-type ("future-type") that can be
			// migrated to from etcdraft.

			// === Step 6: config update on standard channel1, State=MAINTENANCE, type=future-type ===

			// === Step 7: config update on standard channel2, State=MAINTENANCE, type=future-type ===
		})

		// This test executes the migration flow and checks that forbidden transitions are rejected.
		// These transitions are enforced by the maintenance filter:
		// - Entry to & exit from maintenance mode can only change ConsensusType.State.
		// - In maintenance mode one can only change ConsensusType.Type & ConsensusType.Metadata.
		// - ConsensusType.Type can only change from "etcdraft" to "future-type" (to be replaced by future consensus
		//   protocol), and only in maintenance mode.
		It("executes raft2future forbidden transitions", func() {
			// === Step 1: ===
			By("1) Config update on standard channel, changing both ConsensusType State & Type is forbidden")
			assertTransitionFailed(network, peer, orderer, channel1,
				"etcdraft", protosorderer.ConsensusType_STATE_NORMAL,
				"testing-only", nil, protosorderer.ConsensusType_STATE_MAINTENANCE)

			// === Step 2: ===
			By("2) Config update on standard channel, both ConsensusType State & some other value is forbidden")
			config, updatedConfig := prepareTransition(network, peer, orderer, channel1,
				"etcdraft", protosorderer.ConsensusType_STATE_NORMAL,
				"etcdraft", nil, protosorderer.ConsensusType_STATE_MAINTENANCE)
			updateConfigWithBatchTimeout(updatedConfig)
			updateOrdererConfigFailed(network, orderer, channel1, config, updatedConfig, peer, orderer)

			// === Step 3: ===
			By("3) Config update on standard channel, State=MAINTENANCE, enter maintenance-mode")
			config, updatedConfig = prepareTransition(network, peer, orderer, channel1,
				"etcdraft", protosorderer.ConsensusType_STATE_NORMAL,
				"etcdraft", nil, protosorderer.ConsensusType_STATE_MAINTENANCE)
			nwo.UpdateOrdererConfig(network, orderer, channel1, config, updatedConfig, peer, orderer)

			By("3) Verify: standard channel config changed")
			std1StartBlockNum := nwo.CurrentConfigBlockNumber(network, peer, orderer, channel1)
			Expect(std1StartBlockNum).ToNot(Equal(0))
			config = nwo.GetConfig(network, peer, orderer, channel1)
			consensusTypeValue := extractOrdererConsensusType(config)
			validateConsensusTypeValue(consensusTypeValue, "etcdraft", protosorderer.ConsensusType_STATE_MAINTENANCE)

			// === Step 4: ===
			By("4) Config update on standard channel, change ConsensusType.Type to unsupported type, forbidden")
			assertTransitionFailed(network, peer, orderer, channel1,
				"etcdraft", protosorderer.ConsensusType_STATE_MAINTENANCE,
				"hesse", nil, protosorderer.ConsensusType_STATE_MAINTENANCE)

			// === Step 5: ===
			By("5) Config update on standard channel, change ConsensusType.Type and State, forbidden")
			assertTransitionFailed(network, peer, orderer, channel1,
				"etcdraft", protosorderer.ConsensusType_STATE_MAINTENANCE,
				"testing-only", nil, protosorderer.ConsensusType_STATE_NORMAL)

			// Note:
			// The following testing steps should be completed once we have a consensus-type ("future-type" that can be
			// migrated to from etcdraft.

			// === Step 6: Config update on standard channel, changing both ConsensusType.Type and other value is permitted ===
			By("6) changing both ConsensusType.Type and other value is permitted")
			// Change consensus type and batch-timeout, for example

			// === Step 7: ===
			By("7) Config update on standard channel, changing value other than ConsensusType.Type is permitted")
			config = nwo.GetConfig(network, peer, orderer, channel1)
			consensusTypeValue = extractOrdererConsensusType(config)
			validateConsensusTypeValue(consensusTypeValue, "etcdraft", protosorderer.ConsensusType_STATE_MAINTENANCE)
			updatedConfig = proto.Clone(config).(*common.Config)
			updateConfigWithBatchTimeout(updatedConfig)
			nwo.UpdateOrdererConfig(network, orderer, channel1, config, updatedConfig, peer, orderer)

			By("7) Verify: standard channel config changed")
			std1BlockNum := nwo.CurrentConfigBlockNumber(network, peer, orderer, channel1)
			Expect(std1BlockNum).To(Equal(std1StartBlockNum + 1))

			// === Step 8: ===
			By("8) Config update on standard channel, both ConsensusType State & some other value is forbidden")
			config, updatedConfig = prepareTransition(network, peer, orderer, channel1,
				"etcdraft", protosorderer.ConsensusType_STATE_MAINTENANCE,
				"etcdraft", nil, protosorderer.ConsensusType_STATE_NORMAL)
			updateConfigWithBatchTimeout(updatedConfig)
			updateOrdererConfigFailed(network, orderer, channel1, config, updatedConfig, peer, orderer)
		})

		// Note:
		// Instead of booting to a future-type which does not exist yet, we change some other config value in
		// maintenance mode, reboot, and exit maintenance mode. Once we have a future consensus-type that can be
		// migrated to from raft is available, this test should be completed.
		It("executes bootstrap to future-type - single node", func() {
			// === Step 1: Config update on standard channel, MAINTENANCE ===
			By("1) Config update on standard channel, State=MAINTENANCE")
			config, updatedConfig := prepareTransition(network, peer, orderer, channel1,
				"etcdraft", protosorderer.ConsensusType_STATE_NORMAL,
				"etcdraft", nil, protosorderer.ConsensusType_STATE_MAINTENANCE)
			nwo.UpdateOrdererConfig(network, orderer, channel1, config, updatedConfig, peer, orderer)

			By("1) Verify: standard channel config changed")
			chan1StartBlockNum := nwo.CurrentConfigBlockNumber(network, peer, orderer, channel1)
			Expect(chan1StartBlockNum).ToNot(Equal(0))

			config = nwo.GetConfig(network, peer, orderer, channel1)
			consensusTypeValue := extractOrdererConsensusType(config)
			validateConsensusTypeValue(consensusTypeValue, "etcdraft", protosorderer.ConsensusType_STATE_MAINTENANCE)

			By("1) Verify: Normal TX's on standard channel are blocked")
			assertTxFailed(network, orderer, channel1)

			// In maintenance mode deliver requests are open to those entities that satisfy the /Channel/Orderer/Readers policy
			By("1) Verify: delivery request from peer is blocked")
			err := checkPeerDeliverRequest(orderer, peer, network, channel1)
			Expect(err).To(MatchError(errors.New("FORBIDDEN")))

			// === Step 2: config update on standard channel, State=MAINTENANCE, type=etcdraft, (in-lieu of future-type) other value changed ===
			By("2) Config update on standard channel, State=MAINTENANCE, type=etcdraft, (in-lieu of future-type) other value changed")
			config = nwo.GetConfig(network, peer, orderer, channel1)
			consensusTypeValue = extractOrdererConsensusType(config)
			validateConsensusTypeValue(consensusTypeValue, "etcdraft", protosorderer.ConsensusType_STATE_MAINTENANCE)
			updatedConfig = proto.Clone(config).(*common.Config)
			updateConfigWithBatchTimeout(updatedConfig)
			nwo.UpdateOrdererConfig(network, orderer, channel1, config, updatedConfig, peer, orderer)

			By("2) Verify: standard channel config changed")
			chan1BlockNum := nwo.CurrentConfigBlockNumber(network, peer, orderer, channel1)
			Expect(chan1BlockNum).To(Equal(chan1StartBlockNum + 1))

			// === Step 3: kill ===
			By("3) killing orderer1")
			o1Proc.Signal(syscall.SIGKILL)
			Eventually(o1Proc.Wait(), network.EventuallyTimeout).Should(Receive(MatchError("exit status 137")))

			// === Step 4: restart ===
			By("4) restarting orderer1")
			network.Consensus.Type = "etcdraft" // Note: change to future-type

			o1Runner = network.OrdererRunner(orderer)
			o1Proc = ifrit.Invoke(o1Runner)

			Eventually(o1Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())

			assertBlockReception(
				map[string]int{
					channel1: int(chan1BlockNum),
				},
				[]*nwo.Orderer{orderer},
				peer,
				network,
			)

			Eventually(o1Runner.Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Raft leader changed: 0 -> "))
			Eventually(o1Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())

			By("5) Release - executing config transaction on standard channel with restarted orderer")
			config, updatedConfig = prepareTransition(network, peer, orderer, channel1,
				"etcdraft", protosorderer.ConsensusType_STATE_MAINTENANCE,
				"etcdraft", nil, protosorderer.ConsensusType_STATE_NORMAL)
			nwo.UpdateOrdererConfig(network, orderer, channel1, config, updatedConfig, peer, orderer)

			By("6) Verify: standard channel config changed")
			chan1BlockNum = nwo.CurrentConfigBlockNumber(network, peer, orderer, channel1)
			Expect(chan1BlockNum).To(Equal(chan1StartBlockNum + 2))

			By("7) Executing transaction on standard channel with restarted orderer")
			assertBlockCreation(network, orderer, peer, channel1, chan1StartBlockNum+3)
			assertBlockCreation(network, orderer, nil, channel1, chan1StartBlockNum+4)

			By("8) Create new channel, executing transaction with restarted orderer")
			channelparticipation.JoinOrdererAppChannel(network, channel2, orderer, o1Runner)

			assertBlockCreation(network, orderer, peer, channel2, 1)
			assertBlockCreation(network, orderer, nil, channel2, 2)

			By("9) Extending the network configuration to add a new orderer")
			// Add another orderer
			orderer2 := &nwo.Orderer{
				Name:         "orderer2",
				Organization: "OrdererOrg",
			}
			ports := nwo.Ports{}
			for _, portName := range nwo.OrdererPortNames() {
				ports[portName] = network.ReservePort()
			}
			network.PortsByOrdererID[orderer2.ID()] = ports
			network.Orderers = append(network.Orderers, orderer2)
			network.GenerateOrdererConfig(orderer2)
			extendNetwork(network)

			secondOrdererCertificatePath := filepath.Join(network.OrdererLocalTLSDir(orderer2), "server.crt")
			secondOrdererCertificate, err := os.ReadFile(secondOrdererCertificatePath)
			Expect(err).NotTo(HaveOccurred())

			By("10) Adding the second orderer to second channel")
			addConsenter(network, peer, orderer, channel2, protosraft.Consenter{
				ServerTlsCert: secondOrdererCertificate,
				ClientTlsCert: secondOrdererCertificate,
				Host:          "127.0.0.1",
				Port:          uint32(network.OrdererPort(orderer2, nwo.ClusterPort)),
			})

			By("11) Obtaining the last config block from the orderer")
			configBlock := nwo.GetConfigBlock(network, peer, orderer, channel2)
			err = os.WriteFile(filepath.Join(testDir, "channel2_block.pb"), protoutil.MarshalOrPanic(configBlock), 0o644)
			Expect(err).NotTo(HaveOccurred())

			By("12) Waiting for the existing orderer to relinquish its leadership")
			Eventually(o1Runner.Err(), network.EventuallyTimeout).Should(gbytes.Say("1 stepped down to follower since quorum is not active"))
			Eventually(o1Runner.Err(), network.EventuallyTimeout).Should(gbytes.Say("No leader is present, cluster size is 2"))

			By("13) Launching the second orderer")
			o2Runner = network.OrdererRunner(orderer2)
			o2Proc = ifrit.Invoke(o2Runner)
			Eventually(o2Proc.Ready(), network.EventuallyTimeout).Should(BeClosed())

			By("14) Joining the second orderer to channel2")
			expectedChannelInfo := channelparticipation.ChannelInfo{
				Name:              channel2,
				URL:               fmt.Sprintf("/participation/v1/channels/%s", channel2),
				Status:            "onboarding",
				ConsensusRelation: "consenter",
				Height:            0,
			}
			channelparticipation.Join(network, orderer2, channel2, configBlock, expectedChannelInfo)
			Eventually(o2Runner.Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Raft leader changed: 0 -> "))

			assertBlockReception(map[string]int{
				channel2: int(nwo.CurrentConfigBlockNumber(network, peer, orderer, channel2)),
			}, []*nwo.Orderer{orderer2}, peer, network)

			By("15) Executing transaction against second orderer on channel2")
			assertBlockCreation(network, orderer2, nil, channel2, 3)
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

func prepareTransition(
	network *nwo.Network, peer *nwo.Peer, orderer *nwo.Orderer, channel string, // Auxiliary
	fromConsensusType string, fromMigState protosorderer.ConsensusType_State, // From
	toConsensusType string, toConsensusMetadata []byte, toMigState protosorderer.ConsensusType_State, // To
) (current, updated *common.Config) {
	current = nwo.GetConfig(network, peer, orderer, channel)
	updated = proto.Clone(current).(*common.Config)
	consensusTypeValue := extractOrdererConsensusType(current)
	validateConsensusTypeValue(consensusTypeValue, fromConsensusType, fromMigState)
	updateConfigWithConsensusType(toConsensusType, toConsensusMetadata, toMigState, updated, consensusTypeValue)
	return current, updated
}

func assertTransitionFailed(
	network *nwo.Network, peer *nwo.Peer, orderer *nwo.Orderer, channel string, // Auxiliary
	fromConsensusType string, fromMigState protosorderer.ConsensusType_State, // From
	toConsensusType string, toConsensusMetadata []byte, toMigState protosorderer.ConsensusType_State, // To
) {
	current, updated := prepareTransition(
		network, peer, orderer, channel,
		fromConsensusType, fromMigState,
		toConsensusType, toConsensusMetadata, toMigState)
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

func addBFTInConfigTree(network *nwo.Network, peer *nwo.Peer, orderer *nwo.Orderer, channel string) {
	config := nwo.GetConfig(network, peer, orderer, channel)

	updatedConfig := proto.Clone(config).(*common.Config)

	orderersVal := &common.Orderers{
		ConsenterMapping: computeConsenterMappings(network),
	}

	policies.EncodeBFTBlockVerificationPolicy(orderersVal.ConsenterMapping, updatedConfig.ChannelGroup.Groups["Orderer"])

	updatedConfig.ChannelGroup.Groups["Orderer"].Values["Orderers"] = &common.ConfigValue{
		Value:     protoutil.MarshalOrPanic(orderersVal),
		ModPolicy: "/Channel/Orderer/Admins",
	}

	nwo.UpdateOrdererConfig(network, orderer, channel, config, updatedConfig, peer, orderer)
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
