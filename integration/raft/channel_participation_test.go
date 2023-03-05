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
	"github.com/hyperledger/fabric/common/ledger/blockledger/fileledger"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/integration/channelparticipation"
	conftx "github.com/hyperledger/fabric/integration/configtx"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/ordererclient"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/tedsuo/ifrit"
	ginkgomon "github.com/tedsuo/ifrit/ginkgomon_v2"
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

	Describe("three node etcdraft network without a system channel", func() {
		startOrderer := func(o *nwo.Orderer) {
			ordererRunner := network.OrdererRunner(o)
			ordererProcess := ifrit.Invoke(ordererRunner)
			Eventually(ordererProcess.Ready(), network.EventuallyTimeout).Should(BeClosed())
			Eventually(ordererRunner.Err(), network.EventuallyTimeout).Should(gbytes.Say("Registrar initializing without a system channel"))
			ordererProcesses = append(ordererProcesses, ordererProcess)
			ordererRunners = append(ordererRunners, ordererRunner)
		}

		BeforeEach(func() {
			network = nwo.New(multiNodeEtcdRaftTwoChannels(), testDir, client, StartPort(), components)
			network.Consensus.ChannelParticipationEnabled = true
			network.Consensus.BootstrapMethod = "none"
			network.GenerateConfigTree()
			network.Bootstrap()
		})

		It("joins application channels from genesis block and removes a channel using the channel participation API", func() {
			orderer1 := network.Orderer("orderer1")
			orderer2 := network.Orderer("orderer2")
			orderer3 := network.Orderer("orderer3")
			orderers := []*nwo.Orderer{orderer1, orderer2, orderer3}
			consenters := []*nwo.Orderer{orderer1, orderer2}
			peer := network.Peer("Org1", "peer0")

			By("starting all three orderers")
			for _, o := range orderers {
				startOrderer(o)
				cl := channelparticipation.List(network, o)
				Expect(cl).To(Equal(channelparticipation.ChannelList{}))
			}

			genesisBlock := applicationChannelGenesisBlock(network, consenters, []*nwo.Peer{peer}, "participation-trophy")
			expectedChannelInfoPT := channelparticipation.ChannelInfo{
				Name:              "participation-trophy",
				URL:               "/participation/v1/channels/participation-trophy",
				Status:            "active",
				ConsensusRelation: "consenter",
				Height:            1,
			}

			for _, o := range consenters {
				By("joining " + o.Name + " to channel as a consenter")
				channelparticipation.Join(network, o, "participation-trophy", genesisBlock, expectedChannelInfoPT)
				channelInfo := channelparticipation.ListOne(network, o, "participation-trophy")
				Expect(channelInfo).To(Equal(expectedChannelInfoPT))
			}

			submitPeerTxn(orderer1, peer, network, channelparticipation.ChannelInfo{
				Name:              "participation-trophy",
				URL:               "/participation/v1/channels/participation-trophy",
				Status:            "active",
				ConsensusRelation: "consenter",
				Height:            2,
			})

			submitPeerTxn(orderer2, peer, network, channelparticipation.ChannelInfo{
				Name:              "participation-trophy",
				URL:               "/participation/v1/channels/participation-trophy",
				Status:            "active",
				ConsensusRelation: "consenter",
				Height:            3,
			})

			By("joining orderer3 to the channel as a follower")
			// make sure we can join using a config block from one of the other orderers
			configBlockPT := nwo.GetConfigBlock(network, peer, orderer2, "participation-trophy")
			expectedChannelInfoPTFollower := channelparticipation.ChannelInfo{
				Name:              "participation-trophy",
				URL:               "/participation/v1/channels/participation-trophy",
				Status:            "onboarding",
				ConsensusRelation: "follower",
				Height:            0,
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

			By("ensuring orderer3 transitions from follower to consenter")
			// config update above added a block
			expectedChannelInfoPT.Height = 4
			Eventually(func() channelparticipation.ChannelInfo {
				return channelparticipation.ListOne(network, orderer3, "participation-trophy")
			}, network.EventuallyTimeout).Should(Equal(expectedChannelInfoPT))

			By("submitting transaction to orderer3 to ensure it is active")
			submitPeerTxn(orderer3, peer, network, channelparticipation.ChannelInfo{
				Name:              "participation-trophy",
				URL:               "/participation/v1/channels/participation-trophy",
				Status:            "active",
				ConsensusRelation: "consenter",
				Height:            5,
			})

			By("joining orderer1 to another channel as a consenter")
			genesisBlockAPT := applicationChannelGenesisBlock(network, []*nwo.Orderer{orderer1}, []*nwo.Peer{peer}, "another-participation-trophy")
			expectedChannelInfoAPT := channelparticipation.ChannelInfo{
				Name:              "another-participation-trophy",
				URL:               "/participation/v1/channels/another-participation-trophy",
				Status:            "active",
				ConsensusRelation: "consenter",
				Height:            1,
			}
			channelparticipation.Join(network, orderer1, "another-participation-trophy", genesisBlockAPT, expectedChannelInfoAPT)
			channelInfo := channelparticipation.ListOne(network, orderer1, "another-participation-trophy")
			Expect(channelInfo).To(Equal(expectedChannelInfoAPT))

			By("listing all channels for orderer1")
			cl := channelparticipation.List(network, orderer1)
			channelparticipation.ChannelListMatcher(cl, []string{"participation-trophy", "another-participation-trophy"})

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
				Name:              "participation-trophy",
				URL:               "/participation/v1/channels/participation-trophy",
				Status:            "active",
				ConsensusRelation: "follower",
				Height:            6,
			}))

			submitPeerTxn(orderer2, peer, network, channelparticipation.ChannelInfo{
				Name:              "participation-trophy",
				URL:               "/participation/v1/channels/participation-trophy",
				Status:            "active",
				ConsensusRelation: "consenter",
				Height:            7,
			})

			By("ensuring orderer1 pulls the latest block as a follower")
			Eventually(func() channelparticipation.ChannelInfo {
				return channelparticipation.ListOne(network, orderer1, "participation-trophy")
			}, network.EventuallyTimeout).Should(Equal(channelparticipation.ChannelInfo{
				Name:              "participation-trophy",
				URL:               "/participation/v1/channels/participation-trophy",
				Status:            "active",
				ConsensusRelation: "follower",
				Height:            7,
			}))

			By("removing orderer1 from a channel")
			channelparticipation.Remove(network, orderer1, "participation-trophy")
			Eventually(func() channelparticipation.ChannelList {
				return channelparticipation.List(network, orderer1)
			}, network.EventuallyTimeout).Should(Equal(channelparticipation.ChannelList{
				SystemChannel: nil,
				Channels: []channelparticipation.ChannelInfoShort{
					{
						Name: "another-participation-trophy",
						URL:  "/participation/v1/channels/another-participation-trophy",
					},
				},
			}))

			By("submitting transaction to orderer1")
			env := CreateBroadcastEnvelope(network, peer, "participation-trophy", []byte("hello"))
			resp, err := ordererclient.Broadcast(network, orderer1, env)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Status).To(Equal(common.Status_BAD_REQUEST))

			By("listing all channels for orderer1")
			cl = channelparticipation.List(network, orderer1)
			channelparticipation.ChannelListMatcher(cl, []string{"another-participation-trophy"})

			By("joining orderer1 to channel it was previously removed from as consenter")
			configBlockPT = nwo.GetConfigBlock(network, peer, orderer2, "participation-trophy")
			expectedChannelInfoPTFollower = channelparticipation.ChannelInfo{
				Name:              "participation-trophy",
				URL:               "/participation/v1/channels/participation-trophy",
				Status:            "onboarding",
				ConsensusRelation: "follower",
				Height:            0,
			}
			channelparticipation.Join(network, orderer1, "participation-trophy", configBlockPT, expectedChannelInfoPTFollower)

			By("ensuring orderer1 completes onboarding successfully")
			expectedChannelInfoPTFollower.Status = "active"
			expectedChannelInfoPTFollower.Height = 7
			Eventually(func() channelparticipation.ChannelInfo {
				return channelparticipation.ListOne(network, orderer1, "participation-trophy")
			}, network.EventuallyTimeout).Should(Equal(expectedChannelInfoPTFollower))

			By("adding orderer1 to the consenters set")
			channelConfig = nwo.GetConfig(network, peer, orderer3, "participation-trophy")
			c = configtx.New(channelConfig)
			err = c.Orderer().AddConsenter(consenterChannelConfig(network, orderer1))
			Expect(err).NotTo(HaveOccurred())
			computeSignSubmitConfigUpdate(network, orderer3, peer, c, "participation-trophy")

			By("ensuring orderer1 transitions from follower to consenter")
			expectedChannelInfoPT = channelparticipation.ChannelInfo{
				Name:              "participation-trophy",
				URL:               "/participation/v1/channels/participation-trophy",
				Status:            "active",
				ConsensusRelation: "consenter",
				Height:            8,
			}
			Eventually(func() channelparticipation.ChannelInfo {
				return channelparticipation.ListOne(network, orderer1, "participation-trophy")
			}, network.EventuallyTimeout).Should(Equal(expectedChannelInfoPT))

			submitPeerTxn(orderer1, peer, network, channelparticipation.ChannelInfo{
				Name:              "participation-trophy",
				URL:               "/participation/v1/channels/participation-trophy",
				Status:            "active",
				ConsensusRelation: "consenter",
				Height:            9,
			})

			By("ensuring the channel is still usable by submitting a transaction to each remaining consenter for the channel")
			submitPeerTxn(orderer2, peer, network, channelparticipation.ChannelInfo{
				Name:              "participation-trophy",
				URL:               "/participation/v1/channels/participation-trophy",
				Status:            "active",
				ConsensusRelation: "consenter",
				Height:            10,
			})

			submitPeerTxn(orderer3, peer, network, channelparticipation.ChannelInfo{
				Name:              "participation-trophy",
				URL:               "/participation/v1/channels/participation-trophy",
				Status:            "active",
				ConsensusRelation: "consenter",
				Height:            11,
			})

			By("attempting to join with an invalid block")
			channelparticipationJoinFailure(network, orderer3, "nice-try", &common.Block{}, http.StatusBadRequest, "invalid join block: block is not a config block")

			By("attempting to join a channel that already exists")
			channelparticipationJoinFailure(network, orderer3, "participation-trophy", genesisBlock, http.StatusMethodNotAllowed, "cannot join: channel already exists")
		})

		It("joins application channels with join-block as consenter via channel participation api", func() {
			orderer1 := network.Orderer("orderer1")
			orderer2 := network.Orderer("orderer2")
			orderer3 := network.Orderer("orderer3")
			orderers := []*nwo.Orderer{orderer1, orderer2}
			peer := network.Peer("Org1", "peer0")

			By("starting two orderers")
			for _, o := range orderers {
				startOrderer(o)
				cl := channelparticipation.List(network, o)
				Expect(cl).To(Equal(channelparticipation.ChannelList{}))
			}

			genesisBlock := applicationChannelGenesisBlock(network, orderers, []*nwo.Peer{peer}, "participation-trophy")
			expectedChannelInfoPT := channelparticipation.ChannelInfo{
				Name:              "participation-trophy",
				URL:               "/participation/v1/channels/participation-trophy",
				Status:            "active",
				ConsensusRelation: "consenter",
				Height:            1,
			}

			for _, o := range orderers {
				By("joining " + o.Name + " to channel as a consenter")
				channelparticipation.Join(network, o, "participation-trophy", genesisBlock, expectedChannelInfoPT)
				channelInfo := channelparticipation.ListOne(network, o, "participation-trophy")
				Expect(channelInfo).To(Equal(expectedChannelInfoPT))
			}

			submitPeerTxn(orderer1, peer, network, channelparticipation.ChannelInfo{
				Name:              "participation-trophy",
				URL:               "/participation/v1/channels/participation-trophy",
				Status:            "active",
				ConsensusRelation: "consenter",
				Height:            2,
			})

			submitPeerTxn(orderer2, peer, network, channelparticipation.ChannelInfo{
				Name:              "participation-trophy",
				URL:               "/participation/v1/channels/participation-trophy",
				Status:            "active",
				ConsensusRelation: "consenter",
				Height:            3,
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
			cl := channelparticipation.List(network, orderer3)
			Expect(cl).To(Equal(channelparticipation.ChannelList{}))

			By("adding orderer3 to the consenters set")
			channelConfig = nwo.GetConfig(network, peer, orderer2, "participation-trophy")
			c = configtx.New(channelConfig)
			err = c.Orderer().AddConsenter(consenterChannelConfig(network, orderer3))
			Expect(err).NotTo(HaveOccurred())
			computeSignSubmitConfigUpdate(network, orderer2, peer, c, "participation-trophy")

			By("joining orderer3 to the channel as a consenter")
			// make sure we can join using a config block from one of the other orderers
			configBlockPT := nwo.GetConfigBlock(network, peer, orderer2, "participation-trophy")
			expectedChannelInfoConsenter := channelparticipation.ChannelInfo{
				Name:              "participation-trophy",
				URL:               "/participation/v1/channels/participation-trophy",
				Status:            "onboarding",
				ConsensusRelation: "consenter",
				Height:            0,
			}
			channelparticipation.Join(network, orderer3, "participation-trophy", configBlockPT, expectedChannelInfoConsenter)

			By("ensuring orderer3 completes onboarding successfully")
			expectedChannelInfoConsenter.Status = "active"
			expectedChannelInfoConsenter.Height = 5
			Eventually(func() channelparticipation.ChannelInfo {
				return channelparticipation.ListOne(network, orderer3, "participation-trophy")
			}, network.EventuallyTimeout).Should(Equal(expectedChannelInfoConsenter))

			submitPeerTxn(orderer3, peer, network, channelparticipation.ChannelInfo{
				Name:              "participation-trophy",
				URL:               "/participation/v1/channels/participation-trophy",
				Status:            "active",
				ConsensusRelation: "consenter",
				Height:            6,
			})
		})

		Context("joining application channels with join-block as follower via channel participation api", func() {
			var (
				orderer1, orderer2, orderer3 *nwo.Orderer
				orderers                     []*nwo.Orderer
				peer                         *nwo.Peer
				genesisBlock, configBlock    *common.Block
			)

			BeforeEach(func() {
				orderer1 = network.Orderer("orderer1")
				orderer2 = network.Orderer("orderer2")
				orderer3 = network.Orderer("orderer3")
				orderers = []*nwo.Orderer{orderer1, orderer2}
				peer = network.Peer("Org1", "peer0")

				By("starting two orderers")
				for _, o := range orderers {
					startOrderer(o)
					cl := channelparticipation.List(network, o)
					Expect(cl).To(Equal(channelparticipation.ChannelList{}))
				}

				genesisBlock = applicationChannelGenesisBlock(network, orderers, []*nwo.Peer{peer}, "participation-trophy")
				expectedChannelInfoPT := channelparticipation.ChannelInfo{
					Name:              "participation-trophy",
					URL:               "/participation/v1/channels/participation-trophy",
					Status:            "active",
					ConsensusRelation: "consenter",
					Height:            1,
				}

				By("joining orderer1 and orderer2 to the channel with a genesis block")
				for _, o := range orderers {
					By("joining " + o.Name + " to channel as a consenter")
					channelparticipation.Join(network, o, "participation-trophy", genesisBlock, expectedChannelInfoPT)
					channelInfo := channelparticipation.ListOne(network, o, "participation-trophy")
					Expect(channelInfo).To(Equal(expectedChannelInfoPT))
				}

				submitPeerTxn(orderer1, peer, network, channelparticipation.ChannelInfo{
					Name:              "participation-trophy",
					URL:               "/participation/v1/channels/participation-trophy",
					Status:            "active",
					ConsensusRelation: "consenter",
					Height:            2,
				})

				submitPeerTxn(orderer2, peer, network, channelparticipation.ChannelInfo{
					Name:              "participation-trophy",
					URL:               "/participation/v1/channels/participation-trophy",
					Status:            "active",
					ConsensusRelation: "consenter",
					Height:            3,
				})

				By("submitting a channel config update")
				channelConfig := nwo.GetConfig(network, peer, orderer1, "participation-trophy")
				c := configtx.New(channelConfig)
				err := c.Orderer().AddCapability("V1_1")
				Expect(err).NotTo(HaveOccurred())
				computeSignSubmitConfigUpdate(network, orderer1, peer, c, "participation-trophy")

				currentBlockNumber := nwo.CurrentConfigBlockNumber(network, peer, orderer1, "participation-trophy")
				Expect(currentBlockNumber).To(BeNumerically(">", 1))

				By("getting the updated config block")
				configBlock = nwo.GetConfigBlock(network, peer, orderer2, "participation-trophy")
			})

			It("joins the channel as a follower using a config block", func() {
				By("starting third orderer")
				startOrderer(orderer3)
				cl := channelparticipation.List(network, orderer3)
				Expect(cl).To(Equal(channelparticipation.ChannelList{}))

				By("joining orderer3 to the channel as a follower")
				// make sure we can join using a config block from one of the other orderers
				expectedChannelInfoPTFollower := channelparticipation.ChannelInfo{
					Name:              "participation-trophy",
					URL:               "/participation/v1/channels/participation-trophy",
					Status:            "onboarding",
					ConsensusRelation: "follower",
					Height:            0,
				}
				channelparticipation.Join(network, orderer3, "participation-trophy", configBlock, expectedChannelInfoPTFollower)

				By("ensuring orderer3 completes onboarding successfully")
				expectedChannelInfoPTFollower.Status = "active"
				expectedChannelInfoPTFollower.Height = 4
				Eventually(func() channelparticipation.ChannelInfo {
					return channelparticipation.ListOne(network, orderer3, "participation-trophy")
				}, network.EventuallyTimeout).Should(Equal(expectedChannelInfoPTFollower))

				By("adding orderer3 to the consenters set")
				channelConfig := nwo.GetConfig(network, peer, orderer1, "participation-trophy")
				c := configtx.New(channelConfig)
				err := c.Orderer().AddConsenter(consenterChannelConfig(network, orderer3))
				Expect(err).NotTo(HaveOccurred())
				computeSignSubmitConfigUpdate(network, orderer1, peer, c, "participation-trophy")

				By("ensuring orderer3 transitions from follower to consenter")
				// config update above added a block
				expectedChannelInfoPT := channelparticipation.ChannelInfo{
					Name:              "participation-trophy",
					URL:               "/participation/v1/channels/participation-trophy",
					Status:            "active",
					ConsensusRelation: "consenter",
					Height:            5,
				}
				Eventually(func() channelparticipation.ChannelInfo {
					return channelparticipation.ListOne(network, orderer3, "participation-trophy")
				}, network.EventuallyTimeout).Should(Equal(expectedChannelInfoPT))

				submitPeerTxn(orderer3, peer, network, channelparticipation.ChannelInfo{
					Name:              "participation-trophy",
					URL:               "/participation/v1/channels/participation-trophy",
					Status:            "active",
					ConsensusRelation: "consenter",
					Height:            6,
				})
			})

			It("recovers from a crash after the join block is written to the pendingops file repo", func() {
				By("simulating the filesystem state at crash")
				joinBlockFileRepoPath := filepath.Join(network.OrdererDir(orderer3), "system", "pendingops", "join")
				err := os.MkdirAll(joinBlockFileRepoPath, 0o755)
				Expect(err).NotTo(HaveOccurred())
				blockPath := filepath.Join(joinBlockFileRepoPath, "participation-trophy.join")
				configBlockBytes, err := proto.Marshal(configBlock)
				Expect(err).NotTo(HaveOccurred())
				err = ioutil.WriteFile(blockPath, configBlockBytes, 0o600)
				Expect(err).NotTo(HaveOccurred())

				By("starting third orderer")
				startOrderer(orderer3)

				By("ensuring orderer3 completes onboarding successfully")
				expectedChannelInfoPTFollower := channelparticipation.ChannelInfo{
					Name:              "participation-trophy",
					URL:               "/participation/v1/channels/participation-trophy",
					Status:            "active",
					ConsensusRelation: "follower",
					Height:            4,
				}
				Eventually(func() channelparticipation.ChannelInfo {
					return channelparticipation.ListOne(network, orderer3, "participation-trophy")
				}, network.EventuallyTimeout).Should(Equal(expectedChannelInfoPTFollower))
			})

			It("recovers from a crash after the join block is written to the pendingops file repo and the ledger directory (but not the ledger) has been created", func() {
				By("simulating the filesystem state at crash")
				joinBlockFileRepoPath := filepath.Join(network.OrdererDir(orderer3), "system", "pendingops", "join")
				err := os.MkdirAll(joinBlockFileRepoPath, 0o755)
				Expect(err).NotTo(HaveOccurred())
				blockPath := filepath.Join(joinBlockFileRepoPath, "participation-trophy.join")
				configBlockBytes, err := proto.Marshal(configBlock)
				Expect(err).NotTo(HaveOccurred())
				err = ioutil.WriteFile(blockPath, configBlockBytes, 0o600)
				Expect(err).NotTo(HaveOccurred())

				// create the ledger directory
				ledgerPath := filepath.Join(network.OrdererDir(orderer3), "system", "chains", "participation-trophy")
				err = os.MkdirAll(ledgerPath, 0o755)
				Expect(err).NotTo(HaveOccurred())

				By("starting third orderer")
				startOrderer(orderer3)

				By("ensuring orderer3 completes onboarding successfully")
				expectedChannelInfoPTFollower := channelparticipation.ChannelInfo{
					Name:              "participation-trophy",
					URL:               "/participation/v1/channels/participation-trophy",
					Status:            "active",
					ConsensusRelation: "follower",
					Height:            4,
				}
				Eventually(func() channelparticipation.ChannelInfo {
					return channelparticipation.ListOne(network, orderer3, "participation-trophy")
				}, network.EventuallyTimeout).Should(Equal(expectedChannelInfoPTFollower))
			})

			It("recovers from a crash after the join block is written to the pendingops file repo and the ledger has been created", func() {
				By("simulating the filesystem state at crash")
				joinBlockFileRepoPath := filepath.Join(network.OrdererDir(orderer3), "system", "pendingops", "join")
				err := os.MkdirAll(joinBlockFileRepoPath, 0o755)
				Expect(err).NotTo(HaveOccurred())
				blockPath := filepath.Join(joinBlockFileRepoPath, "participation-trophy.join")
				configBlockBytes, err := proto.Marshal(configBlock)
				Expect(err).NotTo(HaveOccurred())
				err = ioutil.WriteFile(blockPath, configBlockBytes, 0o600)
				Expect(err).NotTo(HaveOccurred())

				// create the ledger and add the genesis block
				ledgerDir := filepath.Join(network.OrdererDir(orderer3), "system")
				lf, err := fileledger.New(ledgerDir, &disabled.Provider{})
				Expect(err).NotTo(HaveOccurred())
				ledger, err := lf.GetOrCreate("participation-trophy")
				Expect(err).NotTo(HaveOccurred())
				err = ledger.Append(genesisBlock)
				Expect(err).NotTo(HaveOccurred())
				lf.Close()

				By("starting third orderer")
				startOrderer(orderer3)

				By("ensuring orderer3 completes onboarding successfully")
				expectedChannelInfoPTFollower := channelparticipation.ChannelInfo{
					Name:              "participation-trophy",
					URL:               "/participation/v1/channels/participation-trophy",
					Status:            "active",
					ConsensusRelation: "follower",
					Height:            4,
				}
				Eventually(func() channelparticipation.ChannelInfo {
					return channelparticipation.ListOne(network, orderer3, "participation-trophy")
				}, network.EventuallyTimeout).Should(Equal(expectedChannelInfoPTFollower))

				By("killing orderer3")
				ordererProcesses[2].Signal(syscall.SIGKILL)
				Eventually(ordererProcesses[2].Wait(), network.EventuallyTimeout).Should(Receive(MatchError("exit status 137")))

				By("submitting transactions while orderer3 is down")
				submitPeerTxn(orderer1, peer, network, channelparticipation.ChannelInfo{
					Name:              "participation-trophy",
					URL:               "/participation/v1/channels/participation-trophy",
					Status:            "active",
					ConsensusRelation: "consenter",
					Height:            5,
				})

				submitPeerTxn(orderer2, peer, network, channelparticipation.ChannelInfo{
					Name:              "participation-trophy",
					URL:               "/participation/v1/channels/participation-trophy",
					Status:            "active",
					ConsensusRelation: "consenter",
					Height:            6,
				})

				By("restarting orderer3 (follower) and ensuring it catches up to the blocks it missed")
				ordererRunner := network.OrdererRunner(orderer3)
				ordererProcess := ifrit.Invoke(ordererRunner)
				Eventually(ordererProcess.Ready(), network.EventuallyTimeout).Should(BeClosed())
				ordererProcesses[2] = ordererProcess
				ordererRunners[2] = ordererRunner
				expectedChannelInfoPTFollower = channelparticipation.ChannelInfo{
					Name:              "participation-trophy",
					URL:               "/participation/v1/channels/participation-trophy",
					Status:            "active",
					ConsensusRelation: "follower",
					Height:            6,
				}
				Eventually(func() channelparticipation.ChannelInfo {
					return channelparticipation.ListOne(network, orderer3, "participation-trophy")
				}, network.EventuallyTimeout).Should(Equal(expectedChannelInfoPTFollower))
			})
		})

		PIt("removing a channel clears the consensus data and the ledger", func() {
			By("start 3 nodes")
			By("join 3 nodes to a 3-node channel")
			By("remove channel from those 3 nodes")
			By("join node1 to a 1-node channel with the same name")
			// TODO See: https://github.com/hyperledger/fabric/issues/4016
		})

		It("requires a client certificate to connect when TLS is enabled", func() {
			orderer := network.Orderer("orderer1")
			_, unauthClient := nwo.OrdererOperationalClients(network, orderer)
			ordererAddress := fmt.Sprintf("127.0.0.1:%d", network.OrdererPort(orderer, nwo.AdminPort))
			listChannelsURL := fmt.Sprintf("https://%s/participation/v1/channels", ordererAddress)

			_, err := unauthClient.Get(listChannelsURL)
			Expect(err).To(MatchError(fmt.Sprintf("Get \"%s\": dial tcp %s: connect: connection refused", listChannelsURL, ordererAddress)))
		})
	})
})

// submit a transaction signed by the peer and ensure it was
// committed to the ledger
func submitPeerTxn(o *nwo.Orderer, peer *nwo.Peer, n *nwo.Network, expectedChannelInfo channelparticipation.ChannelInfo) {
	env := CreateBroadcastEnvelope(n, peer, expectedChannelInfo.Name, []byte("hello"))
	submitTxn(o, env, n, expectedChannelInfo)
}

// submit a transaction signed by the orderer and ensure it is
// committed to the ledger
func submitOrdererTxn(o *nwo.Orderer, n *nwo.Network, expectedChannelInfo channelparticipation.ChannelInfo) {
	env := CreateBroadcastEnvelope(n, o, expectedChannelInfo.Name, []byte("hello"))
	submitTxn(o, env, n, expectedChannelInfo)
}

// submit the envelope to the orderer and ensure it is committed
// to the ledger
func submitTxn(o *nwo.Orderer, env *common.Envelope, n *nwo.Network, expectedChannelInfo channelparticipation.ChannelInfo) {
	By("submitting a transaction to " + o.Name)
	Eventually(broadcastTransactionFunc(n, o, env), n.EventuallyTimeout, time.Second).Should(Equal(common.Status_SUCCESS))

	By("checking the channel info on " + o.Name)
	Eventually(func() channelparticipation.ChannelInfo {
		return channelparticipation.ListOne(n, o, expectedChannelInfo.Name)
	}, n.EventuallyTimeout).Should(Equal(expectedChannelInfo))
}

func applicationChannelGenesisBlock(n *nwo.Network, orderers []*nwo.Orderer, peers []*nwo.Peer, channel string) *common.Block {
	ordererOrgs, consenters := ordererOrganizationsAndConsenters(n, orderers)
	peerOrgs := peerOrganizations(n, peers)

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

func systemChannelGenesisBlock(n *nwo.Network, orderers []*nwo.Orderer, peers []*nwo.Peer, channel string) *common.Block {
	ordererOrgs, consenters := ordererOrganizationsAndConsenters(n, orderers)
	peerOrgs := peerOrganizations(n, peers)

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
		Consortiums: []configtx.Consortium{
			{
				Name:          n.Consortiums[0].Name,
				Organizations: peerOrgs,
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

	genesisBlock, err := configtx.NewSystemChannelGenesisBlock(channelConfig, channel)
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
			ordererOrgsMap[o.Organization] = &orgConfig
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

// constructs the peer organizations for a config block. It should be passed
// only one peer per organization.
func peerOrganizations(n *nwo.Network, peers []*nwo.Peer) []configtx.Organization {
	peerOrgs := make([]configtx.Organization, len(peers))
	for i, p := range peers {
		rootCert := parseCertificate(n.PeerCACert(p))
		adminCert := parseCertificate(n.PeerUserCert(p, "Admin"))
		tlsRootCert := parseCertificate(filepath.Join(n.PeerLocalTLSDir(p), "ca.crt"))

		peerOrgs[i] = configtxOrganization(n.Organization(p.Organization), rootCert, adminCert, tlsRootCert)
	}

	return peerOrgs
}

func configtxOrganization(org *nwo.Organization, rootCert, adminCert, tlsRootCert *x509.Certificate) configtx.Organization {
	return configtx.Organization{
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
				Rule: fmt.Sprintf("OR('%s.admin')", org.MSPID),
			},
		},
		MSP: configtx.MSP{
			Name:         org.MSPID,
			RootCerts:    []*x509.Certificate{rootCert},
			Admins:       []*x509.Certificate{adminCert},
			TLSRootCerts: []*x509.Certificate{tlsRootCert},
		},
	}
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

	protocol := "http"
	if n.TLSEnabled {
		protocol = "https"
	}
	url := fmt.Sprintf("%s://127.0.0.1:%d/participation/v1/channels", protocol, n.OrdererPort(o, nwo.AdminPort))
	req := channelparticipation.GenerateJoinRequest(url, channel, blockBytes)
	authClient, _ := nwo.OrdererOperationalClients(n, o)

	doBodyFailure(authClient, req, expectedStatus, expectedError)
}

func channelparticipationJoinConnectFailure(n *nwo.Network, o *nwo.Orderer, channel string, block *common.Block, expectedError string) {
	blockBytes, err := proto.Marshal(block)
	Expect(err).NotTo(HaveOccurred())

	protocol := "http"
	if n.TLSEnabled {
		protocol = "https"
	}
	url := fmt.Sprintf("%s://127.0.0.1:%d/participation/v1/channels", protocol, n.OrdererPort(o, nwo.AdminPort))

	req := channelparticipation.GenerateJoinRequest(url, channel, blockBytes)
	authClient, _ := nwo.OrdererOperationalClients(n, o)

	_, err = authClient.Do(req)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring(expectedError))
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

func multiNodeEtcdRaftTwoChannels() *nwo.Config {
	config := nwo.MultiNodeEtcdRaftNoSysChan()
	config.Channels = []*nwo.Channel{
		{Name: "testchannel", Profile: "TwoOrgsAppChannelEtcdRaft"},
		{Name: "testchannel2", Profile: "TwoOrgsAppChannelEtcdRaft"},
	}

	for _, peer := range config.Peers {
		peer.Channels = []*nwo.PeerChannel{
			{Name: "testchannel", Anchor: true},
			{Name: "testchannel2", Anchor: true},
		}
	}

	return config
}
