/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package raft

import (
	"crypto"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-config/configtx"
	"github.com/hyperledger/fabric-config/configtx/orderer"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/integration/channelparticipation"
	conftx "github.com/hyperledger/fabric/integration/configtx"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	"github.com/hyperledger/fabric/integration/ordererclient"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/ginkgomon"
)

var _ = Describe("ChannelParticipation", func() {
	var (
		testDir          string
		client           *docker.Client
		network          *nwo.Network
		ordererProcesses []ifrit.Process
		ordererRunners   []*ginkgomon.Runner
	)

	BeforeEach(func() {
		var err error
		testDir, err = ioutil.TempDir("", "channel-participation")
		Expect(err).NotTo(HaveOccurred())

		client, err = docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())

		ordererProcesses = []ifrit.Process{}
		ordererRunners = []*ginkgomon.Runner{}
	})

	AfterEach(func() {
		for _, ordererProcess := range ordererProcesses {
			ordererProcess.Signal(syscall.SIGTERM)
			Eventually(ordererProcess.Wait(), network.EventuallyTimeout).Should(Receive())
		}
		if network != nil {
			network.Cleanup()
		}
		os.RemoveAll(testDir)
	})

	restartOrderer := func(o *nwo.Orderer, index int) {
		ordererProcesses[index].Signal(syscall.SIGKILL)
		Eventually(ordererProcesses[index].Wait(), network.EventuallyTimeout).Should(Receive(MatchError("exit status 137")))
		ordererRunner := network.OrdererRunner(o)
		ordererProcess := ifrit.Invoke(ordererRunner)
		Eventually(ordererProcess.Ready(), network.EventuallyTimeout).Should(BeClosed())
		ordererProcesses[index] = ordererProcess
		ordererRunners[index] = ordererRunner
	}

	Describe("three node etcdraft network without a system channel", func() {
		startOrderer := func(o *nwo.Orderer) {
			ordererRunner := network.OrdererRunner(o)
			ordererProcess := ifrit.Invoke(ordererRunner)
			Eventually(ordererProcess.Ready(), network.EventuallyTimeout).Should(BeClosed())
			Eventually(ordererRunner.Err(), network.EventuallyTimeout).Should(gbytes.Say(
				"Registrar initializing without a system channel, number of application channels: 0, with 0 consensus.Chain\\(s\\) and 0 follower.Chain\\(s\\)"))
			ordererProcesses = append(ordererProcesses, ordererProcess)
			ordererRunners = append(ordererRunners, ordererRunner)
		}

		BeforeEach(func() {
			network = nwo.New(nwo.MultiNodeEtcdRaft(), testDir, client, StartPort(), components)
			network.Consensus.ChannelParticipationEnabled = true
			network.Consensus.BootstrapMethod = "none"
			network.GenerateConfigTree()
			network.Bootstrap()
		})

		It("starts an orderer but rejects channel creation requests via the legacy channel creation", func() {
			orderer1 := network.Orderer("orderer1")
			startOrderer(orderer1)

			channelparticipation.List(network, orderer1, nil)

			By("attempting to create a channel without a system channel defined")
			sess, err := network.PeerAdminSession(network.Peer("Org1", "peer0"), commands.ChannelCreate{
				ChannelID:   "testchannel",
				Orderer:     network.OrdererAddress(orderer1, nwo.ListenPort),
				File:        network.CreateChannelTxPath("testchannel"),
				OutputBlock: "/dev/null",
				ClientAuth:  network.ClientAuthRequired,
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(1))
			Eventually(sess.Err, network.EventuallyTimeout).Should(gbytes.Say("channel creation request not allowed because the orderer system channel is not defined"))
		})

		It("joins application channels from genesis block and removes a channel using the channel participation API", func() {
			orderer1 := network.Orderer("orderer1")
			orderer2 := network.Orderer("orderer2")
			orderer3 := network.Orderer("orderer3")
			orderers := []*nwo.Orderer{orderer1, orderer2, orderer3}
			members := []*nwo.Orderer{orderer1, orderer2}
			peer := network.Peer("Org1", "peer0")

			By("starting all three orderers")
			for _, o := range orderers {
				startOrderer(o)
				channelparticipation.List(network, o, nil)
			}

			genesisBlock := applicationChannelGenesisBlock(network, members, peer, "participation-trophy")
			expectedChannelInfoPT := channelparticipation.ChannelInfo{
				Name:            "participation-trophy",
				URL:             "/participation/v1/channels/participation-trophy",
				Status:          "active",
				ClusterRelation: "member",
				Height:          1,
			}

			for _, o := range members {
				By("joining " + o.Name + " to channel as a member")
				channelparticipation.Join(network, o, "participation-trophy", genesisBlock, expectedChannelInfoPT)
				channelInfo := channelparticipation.ListOne(network, o, "participation-trophy")
				Expect(channelInfo).To(Equal(expectedChannelInfoPT))
			}

			submitTxn(orderer1, peer, network, members, 1, channelparticipation.ChannelInfo{
				Name:            "participation-trophy",
				URL:             "/participation/v1/channels/participation-trophy",
				Status:          "active",
				ClusterRelation: "member",
				Height:          2,
			})

			submitTxn(orderer2, peer, network, members, 2, channelparticipation.ChannelInfo{
				Name:            "participation-trophy",
				URL:             "/participation/v1/channels/participation-trophy",
				Status:          "active",
				ClusterRelation: "member",
				Height:          3,
			})

			By("joining orderer3 to the channel as a follower")
			// make sure we can join using a config block from one of the other orderers
			configBlockPT := nwo.GetConfigBlock(network, peer, orderer2, "participation-trophy")
			expectedChannelInfoPTFollower := channelparticipation.ChannelInfo{
				Name:            "participation-trophy",
				URL:             "/participation/v1/channels/participation-trophy",
				Status:          "onboarding",
				ClusterRelation: "follower",
				Height:          0,
			}
			channelparticipation.Join(network, orderer3, "participation-trophy", configBlockPT, expectedChannelInfoPTFollower)

			By("ensuring orderer3 completes onboarding successfully")
			expectedChannelInfoPTFollower.Status = "active"
			expectedChannelInfoPTFollower.Height = 3
			Eventually(func() channelparticipation.ChannelInfo {
				return channelparticipation.ListOne(network, orderer3, "participation-trophy")
			}, network.EventuallyTimeout).Should(Equal(expectedChannelInfoPTFollower))

			By("adding orderer3 to the consenters set")
			channelConfig := nwo.GetConfig(network, peer, orderer1, "participation-trophy")
			c := configtx.New(channelConfig)
			err := c.Orderer().AddConsenter(consenterChannelConfig(network, orderer3))
			Expect(err).NotTo(HaveOccurred())
			computeSignSubmitConfigUpdate(network, orderer1, peer, c, "participation-trophy")

			By("ensuring orderer3 transitions from follower to member")
			// config update above added a block
			expectedChannelInfoPT.Height = 4
			Eventually(func() channelparticipation.ChannelInfo {
				return channelparticipation.ListOne(network, orderer3, "participation-trophy")
			}, network.EventuallyTimeout).Should(Equal(expectedChannelInfoPT))

			By("submitting transaction to orderer3 to ensure it is active")
			submitTxn(orderer3, peer, network, orderers, 4, channelparticipation.ChannelInfo{
				Name:            "participation-trophy",
				URL:             "/participation/v1/channels/participation-trophy",
				Status:          "active",
				ClusterRelation: "member",
				Height:          5,
			})

			By("joining orderer1 to another channel as a member")
			genesisBlockAPT := applicationChannelGenesisBlock(network, []*nwo.Orderer{orderer1}, peer, "another-participation-trophy")
			expectedChannelInfoAPT := channelparticipation.ChannelInfo{
				Name:            "another-participation-trophy",
				URL:             "/participation/v1/channels/another-participation-trophy",
				Status:          "active",
				ClusterRelation: "member",
				Height:          1,
			}
			channelparticipation.Join(network, orderer1, "another-participation-trophy", genesisBlockAPT, expectedChannelInfoAPT)
			channelInfo := channelparticipation.ListOne(network, orderer1, "another-participation-trophy")
			Expect(channelInfo).To(Equal(expectedChannelInfoAPT))

			By("listing all channels for orderer1")
			channelparticipation.List(network, orderer1, []string{"participation-trophy", "another-participation-trophy"})

			By("removing orderer1 from the consenter set")
			channelConfig = nwo.GetConfig(network, peer, orderer2, "participation-trophy")
			c = configtx.New(channelConfig)
			err = c.Orderer().RemoveConsenter(consenterChannelConfig(network, orderer1))
			Expect(err).NotTo(HaveOccurred())
			computeSignSubmitConfigUpdate(network, orderer2, peer, c, "participation-trophy")

			By("ensuring orderer1 transitions to a follower")
			Eventually(func() channelparticipation.ChannelInfo {
				return channelparticipation.ListOne(network, orderer1, "participation-trophy")
			}, network.EventuallyTimeout).Should(Equal(channelparticipation.ChannelInfo{
				Name:            "participation-trophy",
				URL:             "/participation/v1/channels/participation-trophy",
				Status:          "active",
				ClusterRelation: "follower",
				Height:          6,
			}))

			members = []*nwo.Orderer{orderer2, orderer3}
			submitTxn(orderer2, peer, network, members, 6, channelparticipation.ChannelInfo{
				Name:            "participation-trophy",
				URL:             "/participation/v1/channels/participation-trophy",
				Status:          "active",
				ClusterRelation: "member",
				Height:          7,
			})

			By("ensuring orderer1 pulls the latest block as a follower")
			Eventually(func() channelparticipation.ChannelInfo {
				return channelparticipation.ListOne(network, orderer1, "participation-trophy")
			}, network.EventuallyTimeout).Should(Equal(channelparticipation.ChannelInfo{
				Name:            "participation-trophy",
				URL:             "/participation/v1/channels/participation-trophy",
				Status:          "active",
				ClusterRelation: "follower",
				Height:          7,
			}))

			By("removing orderer1 from a channel")
			channelparticipation.Remove(network, orderer1, "participation-trophy")

			By("listing all channels for orderer1")
			channelparticipation.List(network, orderer1, []string{"another-participation-trophy"})

			By("ensuring the channel is still usable by submitting a transaction to each remaining consenter for the channel")
			submitTxn(orderer2, peer, network, members, 7, channelparticipation.ChannelInfo{
				Name:            "participation-trophy",
				URL:             "/participation/v1/channels/participation-trophy",
				Status:          "active",
				ClusterRelation: "member",
				Height:          8,
			})

			submitTxn(orderer3, peer, network, members, 8, channelparticipation.ChannelInfo{
				Name:            "participation-trophy",
				URL:             "/participation/v1/channels/participation-trophy",
				Status:          "active",
				ClusterRelation: "member",
				Height:          9,
			})

			By("attempting to join with an invalid block")
			channelparticipationJoinFailure(network, orderer3, "nice-try", &common.Block{}, http.StatusBadRequest, "invalid join block: block is not a config block")

			By("attempting to join a channel that already exists")
			channelparticipationJoinFailure(network, orderer3, "participation-trophy", genesisBlock, http.StatusMethodNotAllowed, "cannot join: channel already exists")

			By("attempting to join system channel when app channels already exist")
			systemChannelBlockBytes, err := ioutil.ReadFile(network.OutputBlockPath(network.SystemChannel.Name))
			Expect(err).NotTo(HaveOccurred())
			systemChannelBlock := &common.Block{}
			err = proto.Unmarshal(systemChannelBlockBytes, systemChannelBlock)
			Expect(err).NotTo(HaveOccurred())
			channelparticipationJoinFailure(network, orderer3, "systemchannel", systemChannelBlock, http.StatusForbidden, "cannot join: application channels already exist")
		})

		It("joins application channels with join-block as member via channel participation api", func() {
			orderer1 := network.Orderer("orderer1")
			orderer2 := network.Orderer("orderer2")
			orderer3 := network.Orderer("orderer3")
			orderers := []*nwo.Orderer{orderer1, orderer2}
			peer := network.Peer("Org1", "peer0")

			By("starting two orderers")
			for _, o := range orderers {
				startOrderer(o)
				channelparticipation.List(network, o, nil)
			}

			genesisBlock := applicationChannelGenesisBlock(network, orderers, peer, "participation-trophy")
			expectedChannelInfoPT := channelparticipation.ChannelInfo{
				Name:            "participation-trophy",
				URL:             "/participation/v1/channels/participation-trophy",
				Status:          "active",
				ClusterRelation: "member",
				Height:          1,
			}

			for _, o := range orderers {
				By("joining " + o.Name + " to channel as a member")
				channelparticipation.Join(network, o, "participation-trophy", genesisBlock, expectedChannelInfoPT)
				channelInfo := channelparticipation.ListOne(network, o, "participation-trophy")
				Expect(channelInfo).To(Equal(expectedChannelInfoPT))
			}

			submitTxn(orderer1, peer, network, orderers, 1, channelparticipation.ChannelInfo{
				Name:            "participation-trophy",
				URL:             "/participation/v1/channels/participation-trophy",
				Status:          "active",
				ClusterRelation: "member",
				Height:          2,
			})

			submitTxn(orderer2, peer, network, orderers, 2, channelparticipation.ChannelInfo{
				Name:            "participation-trophy",
				URL:             "/participation/v1/channels/participation-trophy",
				Status:          "active",
				ClusterRelation: "member",
				Height:          3,
			})

			By("submitting a channel config update")
			channelConfig := nwo.GetConfig(network, peer, orderer1, "participation-trophy")
			c := configtx.New(channelConfig)
			err := c.Orderer().AddCapability("V1_1")
			Expect(err).NotTo(HaveOccurred())
			computeSignSubmitConfigUpdate(network, orderer1, peer, c, "participation-trophy")

			currentBlockNumber := nwo.CurrentConfigBlockNumber(network, peer, orderer1, "participation-trophy")
			Expect(currentBlockNumber).To(BeNumerically(">", 1))

			By("starting third orderer")
			startOrderer(orderer3)
			channelparticipation.List(network, orderer3, nil)

			By("adding orderer3 to the consenters set")
			channelConfig = nwo.GetConfig(network, peer, orderer2, "participation-trophy")
			c = configtx.New(channelConfig)
			err = c.Orderer().AddConsenter(consenterChannelConfig(network, orderer3))
			Expect(err).NotTo(HaveOccurred())
			computeSignSubmitConfigUpdate(network, orderer2, peer, c, "participation-trophy")

			By("joining orderer3 to the channel as a member")
			// make sure we can join using a config block from one of the other orderers
			configBlockPT := nwo.GetConfigBlock(network, peer, orderer2, "participation-trophy")
			expectedChannelInfoMember := channelparticipation.ChannelInfo{
				Name:            "participation-trophy",
				URL:             "/participation/v1/channels/participation-trophy",
				Status:          "onboarding",
				ClusterRelation: "member",
				Height:          0,
			}
			channelparticipation.Join(network, orderer3, "participation-trophy", configBlockPT, expectedChannelInfoMember)

			By("ensuring orderer3 completes onboarding successfully")
			expectedChannelInfoMember.Status = "active"
			expectedChannelInfoMember.Height = 5
			Eventually(func() channelparticipation.ChannelInfo {
				return channelparticipation.ListOne(network, orderer3, "participation-trophy")
			}, network.EventuallyTimeout).Should(Equal(expectedChannelInfoMember))

			submitTxn(orderer3, peer, network, orderers, 5, channelparticipation.ChannelInfo{
				Name:            "participation-trophy",
				URL:             "/participation/v1/channels/participation-trophy",
				Status:          "active",
				ClusterRelation: "member",
				Height:          6,
			})
		})

		It("joins application channels with join-block as follower via channel participation api", func() {
			orderer1 := network.Orderer("orderer1")
			orderer2 := network.Orderer("orderer2")
			orderer3 := network.Orderer("orderer3")
			orderers := []*nwo.Orderer{orderer1, orderer2}
			peer := network.Peer("Org1", "peer0")

			By("starting two orderers")
			for _, o := range orderers {
				startOrderer(o)
				channelparticipation.List(network, o, nil)
			}

			genesisBlock := applicationChannelGenesisBlock(network, orderers, peer, "participation-trophy")
			expectedChannelInfoPT := channelparticipation.ChannelInfo{
				Name:            "participation-trophy",
				URL:             "/participation/v1/channels/participation-trophy",
				Status:          "active",
				ClusterRelation: "member",
				Height:          1,
			}

			for _, o := range orderers {
				By("joining " + o.Name + " to channel as a member")
				channelparticipation.Join(network, o, "participation-trophy", genesisBlock, expectedChannelInfoPT)
				channelInfo := channelparticipation.ListOne(network, o, "participation-trophy")
				Expect(channelInfo).To(Equal(expectedChannelInfoPT))
			}

			submitTxn(orderer1, peer, network, orderers, 1, channelparticipation.ChannelInfo{
				Name:            "participation-trophy",
				URL:             "/participation/v1/channels/participation-trophy",
				Status:          "active",
				ClusterRelation: "member",
				Height:          2,
			})

			submitTxn(orderer2, peer, network, orderers, 2, channelparticipation.ChannelInfo{
				Name:            "participation-trophy",
				URL:             "/participation/v1/channels/participation-trophy",
				Status:          "active",
				ClusterRelation: "member",
				Height:          3,
			})

			By("submitting a channel config update")
			channelConfig := nwo.GetConfig(network, peer, orderer1, "participation-trophy")
			c := configtx.New(channelConfig)
			err := c.Orderer().AddCapability("V1_1")
			Expect(err).NotTo(HaveOccurred())
			computeSignSubmitConfigUpdate(network, orderer1, peer, c, "participation-trophy")

			currentBlockNumber := nwo.CurrentConfigBlockNumber(network, peer, orderer1, "participation-trophy")
			Expect(currentBlockNumber).To(BeNumerically(">", 1))

			By("starting third orderer")
			startOrderer(orderer3)
			channelparticipation.List(network, orderer3, nil)

			By("joining orderer3 to the channel as a follower")
			// make sure we can join using a config block from one of the other orderers
			configBlockPT := nwo.GetConfigBlock(network, peer, orderer2, "participation-trophy")
			expectedChannelInfoPTFollower := channelparticipation.ChannelInfo{
				Name:            "participation-trophy",
				URL:             "/participation/v1/channels/participation-trophy",
				Status:          "onboarding",
				ClusterRelation: "follower",
				Height:          0,
			}
			channelparticipation.Join(network, orderer3, "participation-trophy", configBlockPT, expectedChannelInfoPTFollower)

			By("ensuring orderer3 completes onboarding successfully")
			expectedChannelInfoPTFollower.Status = "active"
			expectedChannelInfoPTFollower.Height = 4
			Eventually(func() channelparticipation.ChannelInfo {
				return channelparticipation.ListOne(network, orderer3, "participation-trophy")
			}, network.EventuallyTimeout).Should(Equal(expectedChannelInfoPTFollower))

			By("adding orderer3 to the consenters set")
			channelConfig = nwo.GetConfig(network, peer, orderer1, "participation-trophy")
			c = configtx.New(channelConfig)
			err = c.Orderer().AddConsenter(consenterChannelConfig(network, orderer3))
			Expect(err).NotTo(HaveOccurred())
			computeSignSubmitConfigUpdate(network, orderer1, peer, c, "participation-trophy")

			By("ensuring orderer3 transitions from follower to member")
			// config update above added a block
			expectedChannelInfoPT.Height = 5
			Eventually(func() channelparticipation.ChannelInfo {
				return channelparticipation.ListOne(network, orderer3, "participation-trophy")
			}, network.EventuallyTimeout).Should(Equal(expectedChannelInfoPT))

			submitTxn(orderer3, peer, network, orderers, 5, channelparticipation.ChannelInfo{
				Name:            "participation-trophy",
				URL:             "/participation/v1/channels/participation-trophy",
				Status:          "active",
				ClusterRelation: "member",
				Height:          6,
			})
		})

		It("creates the system channel with a genesis block", func() {
			peer := network.Peer("Org1", "peer0")
			orderer1 := network.Orderer("orderer1")
			orderer2 := network.Orderer("orderer2")
			orderer3 := network.Orderer("orderer3")
			orderers := []*nwo.Orderer{orderer1, orderer2, orderer3}
			for _, o := range orderers {
				startOrderer(o)
			}

			systemChannelBlockBytes, err := ioutil.ReadFile(network.OutputBlockPath("systemchannel"))
			Expect(err).NotTo(HaveOccurred())
			systemChannelBlock := &common.Block{}
			err = proto.Unmarshal(systemChannelBlockBytes, systemChannelBlock)
			Expect(err).NotTo(HaveOccurred())

			expectedChannelInfo := channelparticipation.ChannelInfo{
				Name:            "systemchannel",
				URL:             "/participation/v1/channels/systemchannel",
				Status:          "inactive",
				ClusterRelation: "member",
				Height:          1,
			}

			By("joining orderers to systemchannel")
			for _, o := range orderers {
				channelparticipation.Join(network, o, "systemchannel", systemChannelBlock, expectedChannelInfo)
			}

			By("attempting to join a channel when system channel is present")
			channelparticipationJoinFailure(network, orderer1, "systemchannel", systemChannelBlock, http.StatusMethodNotAllowed, "cannot join: system channel exists")

			By("attempting to submit a transaction to systemchannel")
			for _, o := range orderers {
				By("submitting transaction to " + o.Name)
				env := CreateBroadcastEnvelope(network, peer, "systemchannel", []byte("hello"))
				resp, err := ordererclient.Broadcast(network, o, env)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.Status).To(Equal(common.Status_FORBIDDEN))
			}

			By("restarting all orderers")
			for i, o := range orderers {
				restartOrderer(o, i)
			}

			By("creating a channel")
			network.CreateChannel("testchannel", orderer1, peer)

			expectedChannelInfo = channelparticipation.ChannelInfo{
				Name:            "testchannel",
				URL:             "/participation/v1/channels/testchannel",
				Status:          "active",
				ClusterRelation: "member",
				Height:          1,
			}
			for _, o := range orderers {
				By("listing single channel for " + o.Name)
				Eventually(func() channelparticipation.ChannelInfo {
					return channelparticipation.ListOne(network, o, "testchannel")
				}, network.EventuallyTimeout).Should(Equal(expectedChannelInfo))
			}

			for _, o := range orderers {
				By("listing the channels for " + o.Name)
				channelparticipation.List(network, o, []string{"testchannel"}, "systemchannel")
			}

			expectedChannelInfo = channelparticipation.ChannelInfo{
				Name:            "systemchannel",
				URL:             "/participation/v1/channels/systemchannel",
				Status:          "active",
				ClusterRelation: "member",
				Height:          2,
			}
			for _, o := range orderers {
				By("listing single channel for " + o.Name)
				Eventually(func() channelparticipation.ChannelInfo {
					return channelparticipation.ListOne(network, o, "systemchannel")
				}, network.EventuallyTimeout).Should(Equal(expectedChannelInfo))
			}

			By("submitting transaction to orderer to confirm channel is usable")
			env := CreateBroadcastEnvelope(network, peer, "testchannel", []byte("hello"))
			resp, err := ordererclient.Broadcast(network, orderer1, env)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Status).To(Equal(common.Status_SUCCESS))
		})

	})

	Describe("three node etcdraft network with a system channel", func() {
		startOrderer := func(o *nwo.Orderer) {
			ordererRunner := network.OrdererRunner(o)
			ordererProcess := ifrit.Invoke(ordererRunner)
			Eventually(ordererProcess.Ready(), network.EventuallyTimeout).Should(BeClosed())
			ordererProcesses = append(ordererProcesses, ordererProcess)
			ordererRunners = append(ordererRunners, ordererRunner)
		}

		restartOrderer := func(o *nwo.Orderer, index int) {
			ordererProcesses[index].Signal(syscall.SIGKILL)
			Eventually(ordererProcesses[index].Wait(), network.EventuallyTimeout).Should(Receive(MatchError("exit status 137")))
			ordererRunner := network.OrdererRunner(o)
			ordererProcess := ifrit.Invoke(ordererRunner)
			Eventually(ordererProcess.Ready(), network.EventuallyTimeout).Should(BeClosed())
			ordererProcesses[index] = ordererProcess
			ordererRunners[index] = ordererRunner
		}

		BeforeEach(func() {
			network = nwo.New(nwo.MultiNodeEtcdRaft(), testDir, client, StartPort(), components)
			network.GenerateConfigTree()
			network.Bootstrap()
		})

		It("joins channels using the legacy channel creation mechanism and then removes the system channel to transition to the channel participation API", func() {
			orderer1 := network.Orderer("orderer1")
			orderer2 := network.Orderer("orderer2")
			orderer3 := network.Orderer("orderer3")
			orderers := []*nwo.Orderer{orderer1, orderer2, orderer3}
			peer := network.Peer("Org1", "peer0")
			for _, o := range orderers {
				startOrderer(o)
			}

			By("creating an application channel using system channel")
			network.CreateChannel("testchannel", orderer1, peer)

			By("broadcasting envelopes to each orderer")
			for _, o := range orderers {
				env := CreateBroadcastEnvelope(network, peer, "testchannel", []byte("hello"))
				Eventually(broadcastTransactionFunc(network, o, env), network.EventuallyTimeout).Should(Equal(common.Status_SUCCESS))
			}

			By("enabling the channel participation API on each orderer")
			network.Consensus.ChannelParticipationEnabled = true
			network.Consensus.BootstrapMethod = "none"
			for i, o := range orderers {
				network.GenerateOrdererConfig(o)
				restartOrderer(o, i)
			}

			By("listing the channels")
			expectedChannelInfo := channelparticipation.ChannelInfo{
				Name:            "testchannel",
				URL:             "/participation/v1/channels/testchannel",
				Status:          "active",
				ClusterRelation: "member",
				Height:          4,
			}
			for _, o := range orderers {
				By("listing single channel")
				Eventually(func() channelparticipation.ChannelInfo {
					return channelparticipation.ListOne(network, o, "testchannel")
				}, network.EventuallyTimeout).Should(Equal(expectedChannelInfo))
				By("listing all channels")
				channelparticipation.List(network, o, []string{"testchannel"}, "systemchannel")
			}

			By("removing orderer3 from the consenters set")
			channelConfig := nwo.GetConfig(network, peer, orderer2, "testchannel")
			c := configtx.New(channelConfig)
			err := c.Orderer().RemoveConsenter(consenterChannelConfig(network, orderer3))
			Expect(err).NotTo(HaveOccurred())
			computeSignSubmitConfigUpdate(network, orderer2, peer, c, "testchannel")

			By("ensuring orderer3 transitions to inactive/config-tracker")
			Eventually(func() channelparticipation.ChannelInfo {
				return channelparticipation.ListOne(network, orderer3, "testchannel")
			}, network.EventuallyTimeout).Should(Equal(channelparticipation.ChannelInfo{
				Name:            "testchannel",
				URL:             "/participation/v1/channels/testchannel",
				Status:          "inactive",
				ClusterRelation: "config-tracker",
				Height:          5,
			}))

			By("restarting orderer3 to ensure it still reports inactive/config-tracker")
			restartOrderer(orderer3, 2)
			Eventually(func() channelparticipation.ChannelInfo {
				return channelparticipation.ListOne(network, orderer3, "testchannel")
			}, network.EventuallyTimeout).Should(Equal(channelparticipation.ChannelInfo{
				Name:            "testchannel",
				URL:             "/participation/v1/channels/testchannel",
				Status:          "inactive",
				ClusterRelation: "config-tracker",
				Height:          5,
			}))

			By("attempting to join a channel when the system channel is present")
			genesisBlock := applicationChannelGenesisBlock(network, orderers, peer, "participation-trophy")
			channelparticipationJoinFailure(network, orderers[0], "participation-trophy", genesisBlock, http.StatusMethodNotAllowed, "cannot join: system channel exists")

			By("attempting to remove a channel when the system channel is present")
			channelparticipationRemoveFailure(network, orderers[0], "testchannel", http.StatusMethodNotAllowed, "cannot remove: system channel exists")

			By("putting the system channel into maintenance mode")
			channelConfig = nwo.GetConfig(network, peer, orderer2, "systemchannel")
			c = configtx.New(channelConfig)
			err = c.Orderer().SetConsensusState(orderer.ConsensusStateMaintenance)
			Expect(err).NotTo(HaveOccurred())
			computeSignSubmitConfigUpdate(network, orderer2, peer, c, "systemchannel")

			By("removing the system channel with the channel participation API")
			for _, o := range orderers {
				channelparticipation.Remove(network, o, "systemchannel")
			}

			By("listing the channels again")
			for _, o := range orderers[:1] {
				channelparticipation.List(network, o, []string{"testchannel"})
			}
			channelparticipation.List(network, orderer3, []string{})

			By("broadcasting envelopes to each active orderer")
			submitTxn(orderer1, peer, network, orderers[:1], 5, channelparticipation.ChannelInfo{
				Name:            "testchannel",
				URL:             "/participation/v1/channels/testchannel",
				Status:          "active",
				ClusterRelation: "member",
				Height:          6,
			})

			submitTxn(orderer2, peer, network, orderers[:1], 6, channelparticipation.ChannelInfo{
				Name:            "testchannel",
				URL:             "/participation/v1/channels/testchannel",
				Status:          "active",
				ClusterRelation: "member",
				Height:          7,
			})

			By("using the channel participation API to join a new channel")
			expectedChannelInfoPT := channelparticipation.ChannelInfo{
				Name:            "participation-trophy",
				URL:             "/participation/v1/channels/participation-trophy",
				Status:          "active",
				ClusterRelation: "member",
				Height:          1,
			}

			for _, o := range orderers {
				By("joining " + o.Name + " to channel as a member")
				channelparticipation.Join(network, o, "participation-trophy", genesisBlock, expectedChannelInfoPT)
				channelInfo := channelparticipation.ListOne(network, o, "participation-trophy")
				Expect(channelInfo).To(Equal(expectedChannelInfoPT))
			}

			submitTxn(orderer1, peer, network, orderers, 1, channelparticipation.ChannelInfo{
				Name:            "participation-trophy",
				URL:             "/participation/v1/channels/participation-trophy",
				Status:          "active",
				ClusterRelation: "member",
				Height:          2,
			})

			submitTxn(orderer2, peer, network, orderers, 2, channelparticipation.ChannelInfo{
				Name:            "participation-trophy",
				URL:             "/participation/v1/channels/participation-trophy",
				Status:          "active",
				ClusterRelation: "member",
				Height:          3,
			})

			submitTxn(orderer3, peer, network, orderers, 3, channelparticipation.ChannelInfo{
				Name:            "participation-trophy",
				URL:             "/participation/v1/channels/participation-trophy",
				Status:          "active",
				ClusterRelation: "member",
				Height:          4,
			})
		})
	})

})

func submitTxn(o *nwo.Orderer, peer *nwo.Peer, n *nwo.Network, orderers []*nwo.Orderer, expectedBlkNum int, expectedChannelInfo channelparticipation.ChannelInfo) {
	By("submitting a transaction to " + o.Name)
	env := CreateBroadcastEnvelope(n, peer, expectedChannelInfo.Name, []byte("hello"))
	Eventually(broadcastTransactionFunc(n, o, env), n.EventuallyTimeout, time.Second).Should(Equal(common.Status_SUCCESS))
	expectedBlockNumPerChannel := map[string]int{expectedChannelInfo.Name: expectedBlkNum}
	assertBlockReception(expectedBlockNumPerChannel, orderers, peer, n)

	By("checking the channel info on " + o.Name)
	channelInfo := channelparticipation.ListOne(n, o, expectedChannelInfo.Name)
	Expect(channelInfo).To(Equal(expectedChannelInfo))
}

func applicationChannelGenesisBlock(n *nwo.Network, orderers []*nwo.Orderer, p *nwo.Peer, channel string) *common.Block {
	ordererOrgs, consenters := ordererOrganizationsAndConsenters(n, orderers)
	peerOrgs := peerOrganizations(n, p)

	channelConfig := configtx.Channel{
		Orderer: configtx.Orderer{
			OrdererType:   "etcdraft",
			Organizations: ordererOrgs,
			EtcdRaft: orderer.EtcdRaft{
				Consenters: consenters,
				Options: orderer.EtcdRaftOptions{
					TickInterval:         "500ms",
					ElectionTick:         10,
					HeartbeatTick:        1,
					MaxInflightBlocks:    5,
					SnapshotIntervalSize: 16 * 1024 * 1024, // 16 MB
				},
			},
			Policies: map[string]configtx.Policy{
				"Readers": {
					Type: "ImplicitMeta",
					Rule: "ANY Readers",
				},
				"Writers": {
					Type: "ImplicitMeta",
					Rule: "ANY Writers",
				},
				"Admins": {
					Type: "ImplicitMeta",
					Rule: "MAJORITY Admins",
				},
				"BlockValidation": {
					Type: "ImplicitMeta",
					Rule: "ANY Writers",
				},
			},
			Capabilities: []string{"V2_0"},
			BatchSize: orderer.BatchSize{
				MaxMessageCount:   100,
				AbsoluteMaxBytes:  1024 * 1024,
				PreferredMaxBytes: 512 * 1024,
			},
			BatchTimeout: 2 * time.Second,
			State:        "STATE_NORMAL",
		},
		Application: configtx.Application{
			Organizations: peerOrgs,
			Capabilities:  []string{"V2_0"},
			Policies: map[string]configtx.Policy{
				"Readers": {
					Type: "ImplicitMeta",
					Rule: "ANY Readers",
				},
				"Writers": {
					Type: "ImplicitMeta",
					Rule: "ANY Writers",
				},
				"Admins": {
					Type: "ImplicitMeta",
					Rule: "MAJORITY Admins",
				},
				"Endorsement": {
					Type: "ImplicitMeta",
					Rule: "MAJORITY Endorsement",
				},
				"LifecycleEndorsement": {
					Type: "ImplicitMeta",
					Rule: "MAJORITY Endorsement",
				},
			},
		},
		Capabilities: []string{"V2_0"},
		Policies: map[string]configtx.Policy{
			"Readers": {
				Type: "ImplicitMeta",
				Rule: "ANY Readers",
			},
			"Writers": {
				Type: "ImplicitMeta",
				Rule: "ANY Writers",
			},
			"Admins": {
				Type: "ImplicitMeta",
				Rule: "MAJORITY Admins",
			},
		},
	}

	genesisBlock, err := configtx.NewApplicationChannelGenesisBlock(channelConfig, channel)
	Expect(err).NotTo(HaveOccurred())

	return genesisBlock
}

// parseCertificate loads the PEM-encoded x509 certificate at the specified
// path.
func parseCertificate(path string) *x509.Certificate {
	certBytes, err := ioutil.ReadFile(path)
	Expect(err).NotTo(HaveOccurred())
	pemBlock, _ := pem.Decode(certBytes)
	cert, err := x509.ParseCertificate(pemBlock.Bytes)
	Expect(err).NotTo(HaveOccurred())
	return cert
}

// parsePrivateKey loads the PEM-encoded private key at the specified path.
func parsePrivateKey(path string) crypto.PrivateKey {
	pkBytes, err := ioutil.ReadFile(path)
	Expect(err).NotTo(HaveOccurred())
	pemBlock, _ := pem.Decode(pkBytes)
	privateKey, err := x509.ParsePKCS8PrivateKey(pemBlock.Bytes)
	Expect(err).NotTo(HaveOccurred())
	return privateKey
}

func ordererOrganizationsAndConsenters(n *nwo.Network, orderers []*nwo.Orderer) ([]configtx.Organization, []orderer.Consenter) {
	ordererOrgsMap := map[string]*configtx.Organization{}
	consenters := make([]orderer.Consenter, len(orderers))

	for i, o := range orderers {
		rootCert := parseCertificate(n.OrdererCACert(o))
		adminCert := parseCertificate(n.OrdererUserCert(o, "Admin"))
		tlsRootCert := parseCertificate(filepath.Join(n.OrdererLocalTLSDir(o), "ca.crt"))

		orgConfig, ok := ordererOrgsMap[o.Organization]
		if !ok {
			orgConfig := configtxOrganization(n.Organization(o.Organization), rootCert, adminCert, tlsRootCert)
			orgConfig.OrdererEndpoints = []string{
				n.OrdererAddress(o, nwo.ListenPort),
			}
			ordererOrgsMap[o.Organization] = orgConfig
		} else {
			orgConfig.OrdererEndpoints = append(orgConfig.OrdererEndpoints, n.OrdererAddress(o, nwo.ListenPort))
			orgConfig.MSP.RootCerts = append(orgConfig.MSP.RootCerts, rootCert)
			orgConfig.MSP.Admins = append(orgConfig.MSP.Admins, adminCert)
			orgConfig.MSP.TLSRootCerts = append(orgConfig.MSP.TLSRootCerts, tlsRootCert)
		}

		consenters[i] = consenterChannelConfig(n, o)
	}

	ordererOrgs := []configtx.Organization{}
	for _, o := range ordererOrgsMap {
		ordererOrgs = append(ordererOrgs, *o)
	}

	return ordererOrgs, consenters
}

func peerOrganizations(n *nwo.Network, p *nwo.Peer) []configtx.Organization {
	rootCert := parseCertificate(n.PeerCACert(p))
	adminCert := parseCertificate(n.PeerUserCert(p, "Admin"))
	tlsRootCert := parseCertificate(filepath.Join(n.PeerLocalTLSDir(p), "ca.crt"))

	peerOrg := configtxOrganization(n.Organization(p.Organization), rootCert, adminCert, tlsRootCert)

	return []configtx.Organization{*peerOrg}
}

func configtxOrganization(org *nwo.Organization, rootCert, adminCert, tlsRootCert *x509.Certificate) *configtx.Organization {
	orgConfig := &configtx.Organization{
		Name: org.Name,
		Policies: map[string]configtx.Policy{
			"Readers": {
				Type: "Signature",
				Rule: fmt.Sprintf("OR('%s.member')", org.MSPID),
			},
			"Writers": {
				Type: "Signature",
				Rule: fmt.Sprintf("OR('%s.member')", org.MSPID),
			},
			"Admins": {
				Type: "Signature",
				Rule: fmt.Sprintf("OR('%s.member')", org.MSPID),
			},
		},
		MSP: configtx.MSP{
			Name:         org.MSPID,
			RootCerts:    []*x509.Certificate{rootCert},
			Admins:       []*x509.Certificate{adminCert},
			TLSRootCerts: []*x509.Certificate{tlsRootCert},
		},
	}

	return orgConfig
}

func computeSignSubmitConfigUpdate(n *nwo.Network, o *nwo.Orderer, p *nwo.Peer, c configtx.ConfigTx, channel string) {
	configUpdate, err := c.ComputeMarshaledUpdate(channel)
	Expect(err).NotTo(HaveOccurred())

	signingIdentity := configtx.SigningIdentity{
		Certificate: parseCertificate(n.OrdererUserCert(o, "Admin")),
		PrivateKey:  parsePrivateKey(n.OrdererUserKey(o, "Admin")),
		MSPID:       n.Organization(o.Organization).MSPID,
	}
	signature, err := signingIdentity.CreateConfigSignature(configUpdate)
	Expect(err).NotTo(HaveOccurred())

	configUpdateEnvelope, err := configtx.NewEnvelope(configUpdate, signature)
	Expect(err).NotTo(HaveOccurred())
	err = signingIdentity.SignEnvelope(configUpdateEnvelope)
	Expect(err).NotTo(HaveOccurred())

	currentBlockNumber := nwo.CurrentConfigBlockNumber(n, p, o, channel)

	Eventually(broadcastTransactionFunc(n, o, configUpdateEnvelope), n.EventuallyTimeout).Should(Equal(common.Status_SUCCESS))

	ccb := func() uint64 { return nwo.CurrentConfigBlockNumber(n, p, o, channel) }
	Eventually(ccb, n.EventuallyTimeout).Should(BeNumerically(">", currentBlockNumber))
}

func broadcastTransactionFunc(n *nwo.Network, o *nwo.Orderer, env *common.Envelope) func() common.Status {
	return func() common.Status {
		resp, err := ordererclient.Broadcast(n, o, env)
		Expect(err).NotTo(HaveOccurred())
		return resp.Status
	}
}

func consenterChannelConfig(n *nwo.Network, o *nwo.Orderer) orderer.Consenter {
	host, port := conftx.OrdererClusterHostPort(n, o)
	tlsCert := parseCertificate(filepath.Join(n.OrdererLocalTLSDir(o), "server.crt"))
	return orderer.Consenter{
		Address: orderer.EtcdAddress{
			Host: host,
			Port: port,
		},
		ClientTLSCert: tlsCert,
		ServerTLSCert: tlsCert,
	}
}

type errorResponse struct {
	Error string `json:"error"`
}

func channelparticipationJoinFailure(n *nwo.Network, o *nwo.Orderer, channel string, block *common.Block, expectedStatus int, expectedError string) {
	blockBytes, err := proto.Marshal(block)
	Expect(err).NotTo(HaveOccurred())
	url := fmt.Sprintf("https://127.0.0.1:%d/participation/v1/channels", n.OrdererPort(o, nwo.AdminPort))
	req := channelparticipation.GenerateJoinRequest(url, channel, blockBytes)
	authClient, _ := nwo.OrdererOperationalClients(n, o)

	doBodyFailure(authClient, req, expectedStatus, expectedError)
}

func doBodyFailure(client *http.Client, req *http.Request, expectedStatus int, expectedError string) {
	resp, err := client.Do(req)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(expectedStatus))
	body, err := ioutil.ReadAll(resp.Body)
	Expect(err).NotTo(HaveOccurred())
	resp.Body.Close()

	errorResponse := &errorResponse{}
	err = json.Unmarshal(body, errorResponse)
	Expect(err).NotTo(HaveOccurred())
	Expect(errorResponse.Error).To(Equal(expectedError))
}

func channelparticipationRemoveFailure(n *nwo.Network, o *nwo.Orderer, channel string, expectedStatus int, expectedError string) {
	authClient, _ := nwo.OrdererOperationalClients(n, o)
	url := fmt.Sprintf("https://127.0.0.1:%d/participation/v1/channels/%s", n.OrdererPort(o, nwo.AdminPort), channel)

	req, err := http.NewRequest(http.MethodDelete, url, nil)
	Expect(err).NotTo(HaveOccurred())

	doBodyFailure(authClient, req, expectedStatus, expectedError)
}
