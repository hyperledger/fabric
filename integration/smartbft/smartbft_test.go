/*
 *
 * Copyright IBM Corp. All Rights Reserved.
 *
 * SPDX-License-Identifier: Apache-2.0
 * /
 *
 */

package smartbft

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/golang/protobuf/proto"

	"github.com/hyperledger/fabric/integration/channelparticipation"

	"github.com/tedsuo/ifrit/grouper"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric-config/configtx"
	"github.com/hyperledger/fabric-config/configtx/orderer"
	"github.com/hyperledger/fabric-protos-go/common"
	conftx "github.com/hyperledger/fabric/integration/configtx"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
	ginkgomon "github.com/tedsuo/ifrit/ginkgomon_v2"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

var _ = Describe("EndToEnd Smart BFT configuration test", func() {
	var (
		testDir          string
		client           *docker.Client
		network          *nwo.Network
		networkProcess   ifrit.Process
		ordererProcesses []ifrit.Process
		peerProcesses    ifrit.Process
	)

	BeforeEach(func() {
		networkProcess = nil
		ordererProcesses = nil
		peerProcesses = nil
		var err error
		testDir, err = ioutil.TempDir("", "e2e-smartbft-test")
		Expect(err).NotTo(HaveOccurred())

		client, err = docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if networkProcess != nil {
			networkProcess.Signal(syscall.SIGTERM)
			Eventually(networkProcess.Wait(), network.EventuallyTimeout).Should(Receive())
		}
		if peerProcesses != nil {
			peerProcesses.Signal(syscall.SIGTERM)
			Eventually(peerProcesses.Wait(), network.EventuallyTimeout).Should(Receive())
		}
		if network != nil {
			network.Cleanup()
		}
		for _, ordererInstance := range ordererProcesses {
			ordererInstance.Signal(syscall.SIGTERM)
			Eventually(ordererInstance.Wait(), network.EventuallyTimeout).Should(Receive())
		}
		os.RemoveAll(testDir)
	})

	Describe("smartbft network", func() {
		It("smartbft multiple nodes stop start all nodes", func() {
			networkConfig := nwo.MultiNodeSmartBFT()
			networkConfig.SystemChannel = nil
			networkConfig.Channels = nil
			channel := "testchannel1"

			network = nwo.New(networkConfig, testDir, client, StartPort(), components)
			network.Consortiums = nil
			network.Consensus.ChannelParticipationEnabled = true
			network.Consensus.BootstrapMethod = "none"
			network.GenerateConfigTree()
			network.Bootstrap()

			var ordererRunners []*ginkgomon.Runner
			for _, orderer := range network.Orderers {
				runner := network.OrdererRunner(orderer)
				runner.Command.Env = append(runner.Command.Env, "FABRIC_LOGGING_SPEC=orderer.consensus.smartbft=debug:grpc=debug")
				ordererRunners = append(ordererRunners, runner)
				proc := ifrit.Invoke(runner)
				ordererProcesses = append(ordererProcesses, proc)
				Eventually(proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			}

			peerGroupRunner, _ := peerGroupRunners(network)
			peerProcesses = ifrit.Invoke(peerGroupRunner)
			Eventually(peerProcesses.Ready(), network.EventuallyTimeout).Should(BeClosed())
			peer := network.Peer("Org1", "peer0")

			joinChannel(network, channel)

			By("Waiting for followers to see the leader")
			Eventually(ordererRunners[1].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1"))
			Eventually(ordererRunners[2].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1"))
			Eventually(ordererRunners[3].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1"))

			By("Joining peers to testchannel1")
			network.JoinChannel(channel, network.Orderers[0], network.PeersWithChannel(channel)...)

			By("Deploying chaincode")
			deployChaincode(network, channel, testDir)

			By("querying the chaincode")
			sess, err := network.PeerUserSession(peer, "User1", commands.ChaincodeQuery{
				ChannelID: channel,
				Name:      "mycc",
				Ctor:      `{"Args":["query","a"]}`,
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
			Expect(sess).To(gbytes.Say("100"))

			By("invoking the chaincode")
			invokeQuery(network, peer, network.Orderers[1], channel, 90)

			By("Taking down all the orderers")
			for _, proc := range ordererProcesses {
				proc.Signal(syscall.SIGTERM)
				Eventually(proc.Wait(), network.EventuallyTimeout).Should(Receive())
			}

			ordererRunners = nil
			ordererProcesses = nil
			By("Bringing up all the nodes")
			for _, orderer := range network.Orderers {
				runner := network.OrdererRunner(orderer)
				runner.Command.Env = append(runner.Command.Env, "FABRIC_LOGGING_SPEC=orderer.consensus.smartbft=debug:grpc=debug")
				ordererRunners = append(ordererRunners, runner)
				proc := ifrit.Invoke(runner)
				ordererProcesses = append(ordererProcesses, proc)
				Eventually(proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			}

			By("Waiting for followers to see the leader, again")
			Eventually(ordererRunners[1].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1 channel=testchannel1"))
			Eventually(ordererRunners[2].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1 channel=testchannel1"))
			Eventually(ordererRunners[3].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1 channel=testchannel1"))

			By("invoking the chaincode, again")
			invokeQuery(network, peer, network.Orderers[2], channel, 80)
		})

		It("smartbft node addition and removal", func() {
			networkConfig := nwo.MultiNodeSmartBFT()
			networkConfig.SystemChannel = nil
			networkConfig.Channels = nil

			network = nwo.New(networkConfig, testDir, client, StartPort(), components)
			network.Consortiums = nil
			network.Consensus.ChannelParticipationEnabled = true
			network.Consensus.BootstrapMethod = "none"
			network.GenerateConfigTree()
			network.Bootstrap()

			network.EventuallyTimeout *= 2

			var ordererRunners []*ginkgomon.Runner
			for _, orderer := range network.Orderers {
				runner := network.OrdererRunner(orderer)
				runner.Command.Env = append(runner.Command.Env, "FABRIC_LOGGING_SPEC=orderer.common.cluster=debug:orderer.consensus.smartbft=debug:policies.ImplicitOrderer=debug")
				ordererRunners = append(ordererRunners, runner)
				proc := ifrit.Invoke(runner)
				ordererProcesses = append(ordererProcesses, proc)
				Eventually(proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			}

			peerRunner := network.PeerGroupRunner()
			peerProcesses = ifrit.Invoke(peerRunner)

			Eventually(peerProcesses.Ready(), network.EventuallyTimeout).Should(BeClosed())
			peer := network.Peer("Org1", "peer0")

			sess, err := network.ConfigTxGen(commands.OutputBlock{
				ChannelID:   "testchannel1",
				Profile:     network.Profiles[0].Name,
				ConfigPath:  network.RootDir,
				OutputBlock: network.OutputBlockPath("testchannel1"),
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))

			genesisBlockBytes, err := os.ReadFile(network.OutputBlockPath("testchannel1"))
			Expect(err).NotTo(HaveOccurred())

			genesisBlock := &common.Block{}
			err = proto.Unmarshal(genesisBlockBytes, genesisBlock)
			Expect(err).NotTo(HaveOccurred())

			expectedChannelInfoPT := channelparticipation.ChannelInfo{
				Name:              "testchannel1",
				URL:               "/participation/v1/channels/testchannel1",
				Status:            "active",
				ConsensusRelation: "consenter",
				Height:            1,
			}

			for _, o := range network.Orderers {
				By("joining " + o.Name + " to channel as a consenter")
				channelparticipation.Join(network, o, "testchannel1", genesisBlock, expectedChannelInfoPT)
				channelInfo := channelparticipation.ListOne(network, o, "testchannel1")
				Expect(channelInfo).To(Equal(expectedChannelInfoPT))
			}

			By("Waiting for followers to see the leader")
			Eventually(ordererRunners[1].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1"))
			Eventually(ordererRunners[2].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1"))
			Eventually(ordererRunners[3].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1"))

			channel := "testchannel1"
			By(fmt.Sprintf("Peers with Channel %s are %+v\n", channel, network.PeersWithChannel(channel)))
			orderer := network.Orderers[0]
			network.JoinChannel(channel, orderer, network.PeersWithChannel(channel)...)

			nwo.DeployChaincode(network, channel, network.Orderers[0], nwo.Chaincode{
				Name:            "mycc",
				Version:         "0.0",
				Path:            components.Build("github.com/hyperledger/fabric/integration/chaincode/simple/cmd"),
				Ctor:            `{"Args":["init","a","100","b","200"]}`,
				SignaturePolicy: `AND ('Org1MSP.member','Org2MSP.member')`,
				Sequence:        "1",
				InitRequired:    true,
				Label:           "my_prebuilt_chaincode",
				Lang:            "binary",
				PackageFile:     filepath.Join(testDir, "simplecc.tar.gz"),
			})

			By("Deployed chaincode successfully")
			assertBlockReception(map[string]int{"testchannel1": 4}, network.Orderers, peer, network)

			By("Transacting on testchannel1")
			invokeQuery(network, peer, orderer, channel, 90)
			invokeQuery(network, peer, orderer, channel, 80)
			assertBlockReception(map[string]int{"testchannel1": 6}, network.Orderers, peer, network)

			By("Adding a new consenter")
			orderer5 := &nwo.Orderer{
				Name:         "orderer5",
				Organization: "OrdererOrg",
			}
			network.Orderers = append(network.Orderers, orderer5)

			ports := nwo.Ports{}
			for _, portName := range nwo.OrdererPortNames() {
				ports[portName] = network.ReservePort()
			}
			network.PortsByOrdererID[orderer5.ID()] = ports

			network.GenerateCryptoConfig()
			network.GenerateOrdererConfig(orderer5)

			sess, err = network.Cryptogen(commands.Extend{
				Config: network.CryptoConfigPath(),
				Input:  network.CryptoPath(),
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))

			ordererCertificatePath := filepath.Join(network.OrdererLocalTLSDir(orderer5), "server.crt")
			ordererCertificate, err := ioutil.ReadFile(ordererCertificatePath)
			Expect(err).NotTo(HaveOccurred())

			ordererIdentity, err := ioutil.ReadFile(network.OrdererCert(orderer5))
			Expect(err).NotTo(HaveOccurred())

			nwo.UpdateConsenters(network, peer, orderer, channel, func(orderers *common.Orderers) {
				orderers.ConsenterMapping = append(orderers.ConsenterMapping, &common.Consenter{
					MspId:         "OrdererMSP",
					Id:            5,
					Identity:      ordererIdentity,
					ServerTlsCert: ordererCertificate,
					ClientTlsCert: ordererCertificate,
					Host:          "127.0.0.1",
					Port:          uint32(network.OrdererPort(orderer5, nwo.ClusterPort)),
				})
			})
			assertBlockReception(map[string]int{"testchannel1": 7}, network.Orderers[:4], peer, network)

			By("Waiting for followers to see the leader")
			Eventually(ordererRunners[0].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 3 channel=testchannel1"))
			Eventually(ordererRunners[1].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 3 channel=testchannel1"))
			Eventually(ordererRunners[3].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 3 channel=testchannel1"))

			By("Launching the added orderer: " + orderer5.Name)
			orderer5Runner := network.OrdererRunner(orderer5)
			orderer5Runner.Command.Env = append(orderer5Runner.Command.Env, "FABRIC_LOGGING_SPEC=grpc=debug:orderer.consensus.smartbft=debug:policies.ImplicitOrderer=debug")
			ordererRunners = append(ordererRunners, orderer5Runner)
			proc := ifrit.Invoke(orderer5Runner)
			ordererProcesses = append(ordererProcesses, proc)
			Eventually(proc.Ready(), network.EventuallyTimeout).Should(BeClosed())

			By("Get latest config block")
			configBlock := nwo.GetConfigBlock(network, peer, orderer, "testchannel1")
			Expect(configBlock).NotTo(Equal(nil))

			By("Joining " + orderer5.Name + " to channel as a consenter")
			expectedChannelInfoPT = channelparticipation.ChannelInfo{
				Name:              "testchannel1",
				URL:               "/participation/v1/channels/testchannel1",
				Status:            "onboarding",
				ConsensusRelation: "consenter",
				Height:            0,
			}
			channelparticipation.Join(network, orderer5, "testchannel1", configBlock, expectedChannelInfoPT)

			expectedChannelInfoPT = channelparticipation.ChannelInfo{
				Name:              "testchannel1",
				URL:               "/participation/v1/channels/testchannel1",
				Status:            "active",
				ConsensusRelation: "consenter",
				Height:            8,
			}
			Eventually(func() channelparticipation.ChannelInfo {
				return channelparticipation.ListOne(network, orderer5, "testchannel1")
			}, network.EventuallyTimeout).Should(Equal(expectedChannelInfoPT))

			By("Waiting for the added orderer to see the leader")
			Eventually(orderer5Runner.Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1 channel=testchannel1"))

			By("Make sure the peers get the config blocks, again")
			waitForBlockReceptionByPeer(peer, network, "testchannel1", 7)

			By("Killing the leader orderer")
			ordererProcesses[0].Signal(syscall.SIGTERM)
			Eventually(ordererProcesses[0].Wait(), network.EventuallyTimeout).Should(Receive())

			By("Waiting for view change to occur")
			Eventually(ordererRunners[1].Err(), network.EventuallyTimeout*2, time.Second).Should(gbytes.Say("Changing to leader role, current view: 1, current leader: 2 channel=testchannel1"))
			Eventually(ordererRunners[2].Err(), network.EventuallyTimeout*2, time.Second).Should(gbytes.Say("Changing to follower role, current view: 1, current leader: 2 channel=testchannel1"))
			Eventually(ordererRunners[3].Err(), network.EventuallyTimeout*2, time.Second).Should(gbytes.Say("Changing to follower role, current view: 1, current leader: 2 channel=testchannel1"))
			Eventually(ordererRunners[4].Err(), network.EventuallyTimeout*2, time.Second).Should(gbytes.Say("Changing to follower role, current view: 1, current leader: 2 channel=testchannel1"))

			By("Bringing the previous leader back up")
			runner := network.OrdererRunner(network.Orderers[0], "FABRIC_LOGGING_SPEC=orderer.common.cluster=debug:orderer.consensus.smartbft=debug:policies.ImplicitOrderer=debug")
			ordererRunners[0] = runner
			proc = ifrit.Invoke(runner)
			ordererProcesses[0] = proc
			Eventually(proc.Ready(), network.EventuallyTimeout).Should(BeClosed())

			By("Making sure previous leader abdicates")
			Eventually(ordererRunners[0].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Changing to follower role, current view: 1, current leader: 2 channel=testchannel1"))

			By("Making sure the previous leader synchronizes")
			Eventually(ordererRunners[0].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Starting view with number 1, sequence 8, and decisions 0 channel=testchannel1"))

			By("Making sure previous leader sees the new leader")
			Eventually(ordererRunners[0].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 2 channel=testchannel1"))

			By("Ensure all nodes are in sync")
			assertBlockReception(map[string]int{"testchannel1": 7}, network.Orderers, peer, network)

			By("Transacting on testchannel1 a few times")
			invokeQuery(network, peer, network.Orderers[4], channel, 70)
			invokeQuery(network, peer, network.Orderers[4], channel, 60)

			By("Invoking again")
			invokeQuery(network, peer, network.Orderers[4], channel, 50)

			By("Ensure all nodes are in sync")
			assertBlockReception(map[string]int{"testchannel1": 10}, network.Orderers, peer, network)

			time.Sleep(time.Second * 5)
			invokeQuery(network, peer, network.Orderers[4], channel, 40)

			By("Ensuring added node participates in consensus")
			Eventually(ordererRunners[0].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Deciding on seq 11"))

			By("Ensure all nodes are in sync, again")
			assertBlockReception(map[string]int{"testchannel1": 11}, network.Orderers, peer, network)

			By("Removing the added node from the channels")
			nwo.UpdateConsenters(network, peer, network.Orderers[2], "testchannel1", func(orderers *common.Orderers) {
				orderers.ConsenterMapping = orderers.ConsenterMapping[:4]
			})
			Eventually(ordererRunners[4].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Evicted in reconfiguration, shutting down channel=testchannel1"))

			By("Ensure all nodes are in sync after node 5 evicted")
			assertBlockReception(map[string]int{"testchannel1": 12}, network.Orderers, peer, network)

			By("Make sure the peers get the config blocks, again")
			waitForBlockReceptionByPeer(peer, network, "testchannel1", 12)

			restart := func(i int) {
				orderer := network.Orderers[i]
				By(fmt.Sprintf("Killing %s", orderer.Name))
				ordererProcesses[i].Signal(syscall.SIGTERM)
				Eventually(ordererProcesses[i].Wait(), network.EventuallyTimeout).Should(Receive())

				By(fmt.Sprintf("Launching %s", orderer.Name))
				runner := network.OrdererRunner(orderer)
				runner.Command.Env = append(runner.Command.Env, "FABRIC_LOGGING_SPEC=orderer.common.cluster=debug:orderer.consensus.smartbft=debug:policies.ImplicitOrderer=debug")
				ordererRunners[i] = runner
				proc := ifrit.Invoke(runner)
				ordererProcesses[i] = proc
				Eventually(proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			}

			By("Restarting the removed node")
			restart(4)

			By("Transact again")
			invokeQuery(network, peer, network.Orderers[2], channel, 30)

			assertBlockReception(map[string]int{"testchannel1": 13}, network.Orderers[:4], peer, network)

			// Drain the buffer
			n := len(orderer5Runner.Err().Contents())
			orderer5Runner.Err().Read(make([]byte, n))

			By("Adding back orderer5 into testchannel1 channel consenters")
			nwo.UpdateConsenters(network, peer, orderer, channel, func(orderers *common.Orderers) {
				orderers.ConsenterMapping = append(orderers.ConsenterMapping, &common.Consenter{
					MspId:         "OrdererMSP",
					Id:            5,
					Identity:      ordererIdentity,
					ServerTlsCert: ordererCertificate,
					ClientTlsCert: ordererCertificate,
					Host:          "127.0.0.1",
					Port:          uint32(network.OrdererPort(orderer5, nwo.ClusterPort)),
				})
			})

			By("Ensuring all nodes got the block that adds the consenter to the application channel")
			assertBlockReception(map[string]int{"testchannel1": 14}, network.Orderers, peer, network)

			By("Transact after orderer5 rejoined the consenters set")
			invokeQuery(network, peer, network.Orderers[0], channel, 20)

			By("Transact last time")
			invokeQuery(network, peer, network.Orderers[4], channel, 10)

			assertBlockReception(map[string]int{"testchannel1": 16}, network.Orderers, peer, network)
		})

		It("smartbft assisted synchronization no rotation", func() {
			networkConfig := nwo.MultiNodeSmartBFT()
			networkConfig.SystemChannel = nil
			networkConfig.Channels = nil
			channel := "testchannel1"

			network = nwo.New(networkConfig, testDir, client, StartPort(), components)
			network.Consortiums = nil
			network.Consensus.ChannelParticipationEnabled = true
			network.Consensus.BootstrapMethod = "none"
			network.GenerateConfigTree()
			network.Bootstrap()

			var ordererRunners []*ginkgomon.Runner
			for _, orderer := range network.Orderers {
				runner := network.OrdererRunner(orderer)
				runner.Command.Env = append(runner.Command.Env, "FABRIC_LOGGING_SPEC=orderer.consensus.smartbft=debug:grpc=debug")
				ordererRunners = append(ordererRunners, runner)
				proc := ifrit.Invoke(runner)
				ordererProcesses = append(ordererProcesses, proc)
				Eventually(proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			}

			peerGroupRunner, _ := peerGroupRunners(network)
			peerProcesses = ifrit.Invoke(peerGroupRunner)
			Eventually(peerProcesses.Ready(), network.EventuallyTimeout).Should(BeClosed())
			peer := network.Peer("Org1", "peer0")

			By("Join channel")
			joinChannel(network, channel)

			By("Waiting for followers to see the leader")
			Eventually(ordererRunners[1].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1"))
			Eventually(ordererRunners[2].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1"))
			Eventually(ordererRunners[3].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1"))

			orderer := network.Orderers[0]

			By("Joining peers to testchannel1")
			network.JoinChannel(channel, network.Orderers[0], network.PeersWithChannel(channel)...)

			assertBlockReception(map[string]int{"testchannel1": 0}, network.Orderers, peer, network)

			By("Restarting all nodes")
			for i := 0; i < 4; i++ {
				orderer := network.Orderers[i]
				By(fmt.Sprintf("Killing %s", orderer.Name))
				ordererProcesses[i].Signal(syscall.SIGTERM)
				Eventually(ordererProcesses[i].Wait(), network.EventuallyTimeout).Should(Receive())

				By(fmt.Sprintf("Launching %s", orderer.Name))
				runner := network.OrdererRunner(orderer)
				runner.Command.Env = append(runner.Command.Env, "FABRIC_LOGGING_SPEC=orderer.consensus.smartbft=debug:policies.ImplicitOrderer=debug")
				ordererRunners[i] = runner
				proc := ifrit.Invoke(runner)
				ordererProcesses[i] = proc
				Eventually(proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			}

			By("Deploying chaincode")
			deployChaincode(network, channel, testDir)

			assertBlockReception(map[string]int{"testchannel1": 4}, network.Orderers, peer, network)

			By("Taking down a follower node")
			ordererProcesses[3].Signal(syscall.SIGTERM)
			Eventually(ordererProcesses[3].Wait(), network.EventuallyTimeout).Should(Receive())

			invokeQuery(network, peer, orderer, channel, 90)
			invokeQuery(network, peer, orderer, channel, 80)
			invokeQuery(network, peer, orderer, channel, 70)
			invokeQuery(network, peer, orderer, channel, 60)

			By("Bringing up the follower node")
			runner := network.OrdererRunner(network.Orderers[3])
			runner.Command.Env = append(runner.Command.Env, "FABRIC_LOGGING_SPEC=orderer.consensus.smartbft=debug:orderer.common.cluster.puller=debug")
			proc := ifrit.Invoke(runner)
			ordererProcesses[3] = proc
			Eventually(proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			Eventually(runner.Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Starting view with number 0, sequence 5"))

			By("Waiting communication to be established from the leader")
			Eventually(runner.Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1"))
			Eventually(runner.Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1"))
			Eventually(runner.Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1"))

			assertBlockReception(map[string]int{"testchannel1": 8}, network.Orderers, peer, network)

			invokeQuery(network, peer, orderer, channel, 50)
			time.Sleep(time.Second * 2)
			invokeQuery(network, peer, orderer, channel, 40)
			time.Sleep(time.Second * 2)
			invokeQuery(network, peer, orderer, channel, 30)
			time.Sleep(time.Second * 2)
			invokeQuery(network, peer, orderer, channel, 20)
			time.Sleep(time.Second * 2)
			invokeQuery(network, peer, orderer, channel, 10)

			By("Submitting to orderer4")
			invokeQuery(network, peer, network.Orderers[3], channel, 0)
			assertBlockReception(map[string]int{"testchannel1": 14}, network.Orderers, peer, network)

			By("Ensuring follower participates in consensus")
			Eventually(runner.Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Deciding on seq 14"))
		})

		It("smartbft autonomous synchronization", func() {
			networkConfig := nwo.MultiNodeSmartBFT()
			networkConfig.SystemChannel = nil
			networkConfig.Channels = nil
			channel := "testchannel1"

			network = nwo.New(networkConfig, testDir, client, StartPort(), components)
			network.Consortiums = nil
			network.Consensus.ChannelParticipationEnabled = true
			network.Consensus.BootstrapMethod = "none"
			network.GenerateConfigTree()
			network.Bootstrap()
			network.EventuallyTimeout = time.Minute * 2

			var ordererRunners []*ginkgomon.Runner
			for _, orderer := range network.Orderers {
				runner := network.OrdererRunner(orderer)
				runner.Command.Env = append(runner.Command.Env, "FABRIC_LOGGING_SPEC=orderer.consensus.smartbft=debug:grpc=debug")
				ordererRunners = append(ordererRunners, runner)
				proc := ifrit.Invoke(runner)
				ordererProcesses = append(ordererProcesses, proc)
				Eventually(proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			}

			peerGroupRunner, _ := peerGroupRunners(network)
			peerProcesses = ifrit.Invoke(peerGroupRunner)
			Eventually(peerProcesses.Ready(), network.EventuallyTimeout).Should(BeClosed())
			peer := network.Peer("Org1", "peer0")

			joinChannel(network, channel)

			By("Waiting for followers to see the leader")
			Eventually(ordererRunners[1].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1"))
			Eventually(ordererRunners[2].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1"))
			Eventually(ordererRunners[3].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1"))

			By("Joining peers to testchannel1")
			network.JoinChannel(channel, network.Orderers[0], network.PeersWithChannel(channel)...)

			By("Deploying chaincode")
			deployChaincode(network, channel, testDir)

			assertBlockReception(map[string]int{"testchannel1": 4}, network.Orderers, peer, network)

			By("Taking down the leader node")
			ordererProcesses[0].Signal(syscall.SIGTERM)
			Eventually(ordererProcesses[0].Wait(), network.EventuallyTimeout).Should(Receive())

			By("Waiting for a view change to occur")
			Eventually(ordererRunners[1].Err(), network.EventuallyTimeout*2, time.Second).Should(gbytes.Say("Changing to leader role, current view: 1, current leader: 2 channel=testchannel1"))
			Eventually(ordererRunners[2].Err(), network.EventuallyTimeout*2, time.Second).Should(gbytes.Say("Changing to follower role, current view: 1, current leader: 2 channel=testchannel1"))
			Eventually(ordererRunners[3].Err(), network.EventuallyTimeout*2, time.Second).Should(gbytes.Say("Changing to follower role, current view: 1, current leader: 2 channel=testchannel1"))

			orderer := network.Orderers[1]

			By("Invoking once")
			invokeQuery(network, peer, orderer, channel, 90)
			By("Invoking twice")
			invokeQuery(network, peer, orderer, channel, 80)
			By("Invoking three times")
			invokeQuery(network, peer, orderer, channel, 70)
			By("Invoking four times")
			invokeQuery(network, peer, orderer, channel, 60)

			By("Bringing up the leader node")
			runner := network.OrdererRunner(network.Orderers[0])
			runner.Command.Env = append(runner.Command.Env, "FABRIC_LOGGING_SPEC=orderer.consensus.smartbft=debug:grpc=debug")
			proc := ifrit.Invoke(runner)
			ordererProcesses[0] = proc

			select {
			case err := <-proc.Wait():
				Fail(err.Error())
			case <-proc.Ready():
			}
			Eventually(proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			Eventually(runner.Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Starting view with number 0, sequence 5"))

			By("Waiting for node to synchronize itself")
			Eventually(runner.Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Finished synchronizing with cluster"))

			By("Waiting for node to understand it synced a view change")
			Eventually(runner.Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Node 1 was informed of a new view 1 channel=testchannel1"))

			By("Waiting for all nodes to have the latest block sequence")
			assertBlockReception(map[string]int{"testchannel1": 8}, network.Orderers, peer, network)

			By("Ensuring the follower is functioning properly")
			invokeQuery(network, peer, orderer, channel, 50)
			invokeQuery(network, peer, orderer, channel, 40)
			assertBlockReception(map[string]int{"testchannel1": 10}, network.Orderers, peer, network)
		})

		It("smartbft multiple nodes view change", func() {
			networkConfig := nwo.MultiNodeSmartBFT()
			networkConfig.SystemChannel = nil
			networkConfig.Channels = nil
			channel := "testchannel1"

			network = nwo.New(networkConfig, testDir, client, StartPort(), components)
			network.Consortiums = nil
			network.Consensus.ChannelParticipationEnabled = true
			network.Consensus.BootstrapMethod = "none"
			network.GenerateConfigTree()
			network.Bootstrap()

			var ordererRunners []*ginkgomon.Runner
			for _, orderer := range network.Orderers {
				runner := network.OrdererRunner(orderer)
				runner.Command.Env = append(runner.Command.Env, "FABRIC_LOGGING_SPEC=orderer.consensus.smartbft=debug:grpc=debug")
				ordererRunners = append(ordererRunners, runner)
				proc := ifrit.Invoke(runner)
				ordererProcesses = append(ordererProcesses, proc)
				Eventually(proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			}

			peerGroupRunner, _ := peerGroupRunners(network)
			peerProcesses = ifrit.Invoke(peerGroupRunner)
			Eventually(peerProcesses.Ready(), network.EventuallyTimeout).Should(BeClosed())
			peer := network.Peer("Org1", "peer0")

			joinChannel(network, channel)

			By("Waiting for followers to see the leader")
			Eventually(ordererRunners[1].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1"))
			Eventually(ordererRunners[2].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1"))
			Eventually(ordererRunners[3].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1"))

			By("Joining peers to testchannel1")
			network.JoinChannel(channel, network.Orderers[0], network.PeersWithChannel(channel)...)

			By("Deploying chaincode")
			deployChaincode(network, channel, testDir)

			assertBlockReception(map[string]int{"testchannel1": 4}, network.Orderers, peer, network)

			By("Taking down the leader node")
			ordererProcesses[0].Signal(syscall.SIGTERM)
			Eventually(ordererProcesses[0].Wait(), network.EventuallyTimeout).Should(Receive())

			By("Submitting a request to all followers to force a view change")

			endpoints := fmt.Sprintf("%s,%s,%s",
				network.OrdererAddress(network.Orderers[1], nwo.ListenPort),
				network.OrdererAddress(network.Orderers[2], nwo.ListenPort),
				network.OrdererAddress(network.Orderers[3], nwo.ListenPort))

			sess, err := network.PeerUserSession(peer, "User1", commands.ChaincodeInvoke{
				ChannelID: channel,
				Orderer:   endpoints,
				Name:      "mycc",
				Ctor:      `{"Args":["issue","x1","100"]}`,
				PeerAddresses: []string{
					network.PeerAddress(network.Peer("Org1", "peer0"), nwo.ListenPort),
					network.PeerAddress(network.Peer("Org2", "peer0"), nwo.ListenPort),
				},
				WaitForEvent: false,
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))

			By("Waiting for view change to occur")
			Eventually(ordererRunners[1].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("ViewChanged, the new view is 1"))
			Eventually(ordererRunners[2].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("ViewChanged, the new view is 1"))
			Eventually(ordererRunners[3].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("ViewChanged, the new view is 1"))

			By("Waiting for circulating transaction to be re-proposed")
			queryExpect(network, peer, channel, "x1", 100)
		})

		It("smartbft iterated addition and iterated removal", func() {
			networkConfig := nwo.MultiNodeSmartBFT()
			networkConfig.SystemChannel = nil
			networkConfig.Channels = nil

			network = nwo.New(networkConfig, testDir, client, StartPort(), components)
			network.Consortiums = nil
			network.Consensus.ChannelParticipationEnabled = true
			network.Consensus.BootstrapMethod = "none"
			network.GenerateConfigTree()
			network.Bootstrap()

			network.EventuallyTimeout *= 2

			orderer := network.Orderers[0]
			channel := "testchannel1"
			var ordererRunners []*ginkgomon.Runner
			for _, orderer := range network.Orderers {
				runner := network.OrdererRunner(orderer)
				runner.Command.Env = append(runner.Command.Env, "FABRIC_LOGGING_SPEC=orderer.common.cluster=debug:orderer.consensus.smartbft=debug:policies.ImplicitOrderer=debug")
				ordererRunners = append(ordererRunners, runner)
				proc := ifrit.Invoke(runner)
				ordererProcesses = append(ordererProcesses, proc)
				Eventually(proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			}

			peerRunner := network.PeerGroupRunner()
			peerProcesses = ifrit.Invoke(peerRunner)

			Eventually(peerProcesses.Ready(), network.EventuallyTimeout).Should(BeClosed())

			sess, err := network.ConfigTxGen(commands.OutputBlock{
				ChannelID:   "testchannel1",
				Profile:     network.Profiles[0].Name,
				ConfigPath:  network.RootDir,
				OutputBlock: network.OutputBlockPath("testchannel1"),
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))

			genesisBlockBytes, err := os.ReadFile(network.OutputBlockPath("testchannel1"))
			Expect(err).NotTo(HaveOccurred())

			genesisBlock := &common.Block{}
			err = proto.Unmarshal(genesisBlockBytes, genesisBlock)
			Expect(err).NotTo(HaveOccurred())

			expectedChannelInfoPT := channelparticipation.ChannelInfo{
				Name:              "testchannel1",
				URL:               "/participation/v1/channels/testchannel1",
				Status:            "active",
				ConsensusRelation: "consenter",
				Height:            1,
			}

			for _, o := range network.Orderers {
				By("joining " + o.Name + " to channel as a consenter")
				channelparticipation.Join(network, o, "testchannel1", genesisBlock, expectedChannelInfoPT)
				channelInfo := channelparticipation.ListOne(network, o, "testchannel1")
				Expect(channelInfo).To(Equal(expectedChannelInfoPT))
			}

			By("Waiting for followers to see the leader")
			Eventually(ordererRunners[1].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1"))
			Eventually(ordererRunners[2].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1"))
			Eventually(ordererRunners[3].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1"))

			peer := network.Peer("Org1", "peer0")

			for i := 0; i < 6; i++ {
				fmt.Fprintf(GinkgoWriter, "adding orderer %d", i+5)

				By("Adding a new consenter with Id " + strconv.Itoa(i+5))
				name := fmt.Sprintf("orderer%d", i+5)

				newOrderer := &nwo.Orderer{
					Name:         name,
					Organization: "OrdererOrg",
				}
				network.Orderers = append(network.Orderers, newOrderer)

				ports := nwo.Ports{}
				for _, portName := range nwo.OrdererPortNames() {
					ports[portName] = network.ReservePort()
				}
				network.PortsByOrdererID[newOrderer.ID()] = ports

				network.GenerateCryptoConfig()
				network.GenerateOrdererConfig(newOrderer)

				sess, err := network.Cryptogen(commands.Extend{
					Config: network.CryptoConfigPath(),
					Input:  network.CryptoPath(),
				})
				Expect(err).NotTo(HaveOccurred())
				Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))

				ordererCertificatePath := filepath.Join(network.OrdererLocalTLSDir(newOrderer), "server.crt")
				ordererCertificate, err := ioutil.ReadFile(ordererCertificatePath)
				Expect(err).NotTo(HaveOccurred())

				ordererIdentity, err := ioutil.ReadFile(network.OrdererCert(newOrderer))
				Expect(err).NotTo(HaveOccurred())

				By(fmt.Sprintf("Adding consenter with certificate %s", string(ordererIdentity)))

				nwo.UpdateConsenters(network, peer, orderer, channel, func(orderers *common.Orderers) {
					orderers.ConsenterMapping = append(orderers.ConsenterMapping, &common.Consenter{
						MspId:         "OrdererMSP",
						Id:            uint32(5 + i),
						Identity:      ordererIdentity,
						ServerTlsCert: ordererCertificate,
						ClientTlsCert: ordererCertificate,
						Host:          "127.0.0.1",
						Port:          uint32(network.OrdererPort(newOrderer, nwo.ClusterPort)),
					})
				})

				assertBlockReception(map[string]int{"testchannel1": 1 + i}, network.Orderers[:4+i], peer, network)

				By("Planting last config block in the orderer's file system")
				configBlock := nwo.GetConfigBlock(network, peer, orderer, "testchannel1")
				Expect(configBlock).NotTo(Equal(nil))

				fmt.Fprintf(GinkgoWriter, "Launching orderer %d", 5+i)
				runner := network.OrdererRunner(newOrderer)
				runner.Command.Env = append(runner.Command.Env, "FABRIC_LOGGING_SPEC=orderer.common.cluster=debug:orderer.consensus.smartbft=debug:policies.ImplicitOrderer=debug")
				ordererRunners = append(ordererRunners, runner)
				proc := ifrit.Invoke(runner)
				ordererRunners = append(ordererRunners, runner)
				ordererProcesses = append(ordererProcesses, proc)
				Eventually(proc.Ready(), network.EventuallyTimeout).Should(BeClosed())

				By(">>>> joining " + newOrderer.Name + " to channel as a consenter")
				expectedChannelInfoPT = channelparticipation.ChannelInfo{
					Name:              "testchannel1",
					URL:               "/participation/v1/channels/testchannel1",
					Status:            "onboarding",
					ConsensusRelation: "consenter",
					Height:            0,
				}
				channelparticipation.Join(network, newOrderer, "testchannel1", configBlock, expectedChannelInfoPT)

				expectedChannelInfoPT = channelparticipation.ChannelInfo{
					Name:              "testchannel1",
					URL:               "/participation/v1/channels/testchannel1",
					Status:            "active",
					ConsensusRelation: "consenter",
					Height:            uint64(2 + i),
				}
				Eventually(func() channelparticipation.ChannelInfo {
					return channelparticipation.ListOne(network, newOrderer, "testchannel1")
				}, network.EventuallyTimeout).Should(Equal(expectedChannelInfoPT))

				By("Waiting for the added orderer to see the leader")
				Eventually(runner.Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1 channel=testchannel1"))

				By("Ensure all orderers are in sync")
				assertBlockReception(map[string]int{"testchannel1": 1 + i}, network.Orderers, peer, network)

			} // for loop that adds orderers

			lastOrdererRunner := ordererRunners[len(ordererRunners)-1]
			lastOrderer := network.Orderers[len(network.Orderers)-1]
			// Put the endpoint of the last 4 orderers instead of the first 4
			var lastOrdererEndpoints []string
			for i := 1; i <= 4; i++ {
				o := network.Orderers[len(network.Orderers)-i]
				ordererEndpoint := fmt.Sprintf("127.0.0.1:%d", network.OrdererPort(o, nwo.ListenPort))
				lastOrdererEndpoints = append(lastOrdererEndpoints, ordererEndpoint)
			}

			By(fmt.Sprintf("Updating the addresses of the orderers to be %s", lastOrdererEndpoints))
			nwo.UpdateOrdererEndpoints(network, peer, lastOrderer, channel, lastOrdererEndpoints...)

			By("Shrinking the cluster back")
			for i := 0; i < 6; i++ {
				By(fmt.Sprintf("Waiting for the added orderer to see the leader %d", i+1))
				Eventually(lastOrdererRunner.Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say(fmt.Sprintf("Message from %d", 1+i)))
				By(fmt.Sprintf("Removing the added node from the application channel (block %d)", 8+i))
				nwo.UpdateConsenters(network, peer, lastOrderer, channel, func(orderers *common.Orderers) {
					orderers.ConsenterMapping = orderers.ConsenterMapping[1:]
				})

				assertBlockReception(map[string]int{"testchannel1": 8 + i}, network.Orderers[7:], peer, network)
			}
		})

		It("smartbft reconfiguration prevents blacklisting", func() {
			channel := "testchannel1"
			networkConfig := nwo.MultiNodeSmartBFT()
			networkConfig.SystemChannel = nil
			networkConfig.Channels = nil

			network = nwo.New(networkConfig, testDir, client, StartPort(), components)
			network.Consortiums = nil
			network.Consensus.ChannelParticipationEnabled = true
			network.Consensus.BootstrapMethod = "none"
			network.GenerateConfigTree()
			network.Bootstrap()

			network.EventuallyTimeout *= 2

			var ordererRunners []*ginkgomon.Runner
			for _, orderer := range network.Orderers {
				runner := network.OrdererRunner(orderer)
				runner.Command.Env = append(runner.Command.Env, "FABRIC_LOGGING_SPEC=orderer.common.cluster=debug:orderer.consensus.smartbft=debug:policies.ImplicitOrderer=debug")
				ordererRunners = append(ordererRunners, runner)
				proc := ifrit.Invoke(runner)
				ordererProcesses = append(ordererProcesses, proc)
				Eventually(proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			}

			peerRunner := network.PeerGroupRunner()
			peerProcesses = ifrit.Invoke(peerRunner)

			Eventually(peerProcesses.Ready(), network.EventuallyTimeout).Should(BeClosed())

			peer := network.Peer("Org1", "peer0")

			joinChannel(network, channel)

			By("Waiting for followers to see the leader")
			Eventually(ordererRunners[1].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1"))
			Eventually(ordererRunners[2].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1"))
			Eventually(ordererRunners[3].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1"))

			By("Joining peers to testchannel1")
			network.JoinChannel(channel, network.Orderers[0], network.PeersWithChannel(channel)...)

			By("Deploying chaincode")
			deployChaincode(network, channel, testDir)

			assertBlockReception(map[string]int{"testchannel1": 4}, network.Orderers, peer, network)

			By("Transacting on testchannel1")
			invokeQuery(network, peer, network.Orderers[0], channel, 90)
			invokeQuery(network, peer, network.Orderers[0], channel, 80)
			assertBlockReception(map[string]int{"testchannel1": 6}, network.Orderers, peer, network)

			By("Adding a new consenter")

			orderer5 := &nwo.Orderer{
				Name:         "orderer5",
				Organization: "OrdererOrg",
			}
			network.Orderers = append(network.Orderers, orderer5)

			ports := nwo.Ports{}
			for _, portName := range nwo.OrdererPortNames() {
				ports[portName] = network.ReservePort()
			}
			network.PortsByOrdererID[orderer5.ID()] = ports

			network.GenerateCryptoConfig()
			network.GenerateOrdererConfig(orderer5)

			sess, err := network.Cryptogen(commands.Extend{
				Config: network.CryptoConfigPath(),
				Input:  network.CryptoPath(),
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))

			ordererCertificatePath := filepath.Join(network.OrdererLocalTLSDir(orderer5), "server.crt")
			ordererCertificate, err := ioutil.ReadFile(ordererCertificatePath)
			Expect(err).NotTo(HaveOccurred())

			ordererIdentity, err := ioutil.ReadFile(network.OrdererCert(orderer5))
			Expect(err).NotTo(HaveOccurred())

			nwo.UpdateConsenters(network, peer, network.Orderers[0], channel, func(orderers *common.Orderers) {
				orderers.ConsenterMapping = append(orderers.ConsenterMapping, &common.Consenter{
					MspId:         "OrdererMSP",
					Id:            5,
					Identity:      ordererIdentity,
					ServerTlsCert: ordererCertificate,
					ClientTlsCert: ordererCertificate,
					Host:          "127.0.0.1",
					Port:          uint32(network.OrdererPort(orderer5, nwo.ClusterPort)),
				})
			})

			assertBlockReception(map[string]int{"testchannel1": 7}, network.Orderers[:4], peer, network)

			By("Waiting for followers to see the leader after config update")
			Eventually(ordererRunners[1].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1 channel=testchannel1"))
			Eventually(ordererRunners[2].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1 channel=testchannel1"))
			Eventually(ordererRunners[3].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1 channel=testchannel1"))

			By("Planting last config block in the orderer's file system")
			configBlock := nwo.GetConfigBlock(network, peer, network.Orderers[0], "testchannel1")
			Expect(err).NotTo(HaveOccurred())

			By("Launching the added orderer")
			orderer5Runner := network.OrdererRunner(orderer5)
			orderer5Runner.Command.Env = append(orderer5Runner.Command.Env, "FABRIC_LOGGING_SPEC=grpc=debug:orderer.consensus.smartbft=debug:policies.ImplicitOrderer=debug")
			ordererRunners = append(ordererRunners, orderer5Runner)
			proc := ifrit.Invoke(orderer5Runner)
			ordererProcesses = append(ordererProcesses, proc)
			select {
			case err := <-proc.Wait():
				Fail(err.Error())
			case <-proc.Ready():
			}
			Eventually(proc.Ready(), network.EventuallyTimeout).Should(BeClosed())

			By("joining " + orderer5.Name + " to channel as a consenter")
			expectedChannelInfo := channelparticipation.ChannelInfo{
				Name:              "testchannel1",
				URL:               "/participation/v1/channels/testchannel1",
				Status:            "onboarding",
				ConsensusRelation: "consenter",
				Height:            0,
			}
			channelparticipation.Join(network, orderer5, "testchannel1", configBlock, expectedChannelInfo)

			expectedChannelInfo = channelparticipation.ChannelInfo{
				Name:              "testchannel1",
				URL:               "/participation/v1/channels/testchannel1",
				Status:            "active",
				ConsensusRelation: "consenter",
				Height:            8,
			}
			Eventually(func() channelparticipation.ChannelInfo {
				return channelparticipation.ListOne(network, orderer5, "testchannel1")
			}, network.EventuallyTimeout).Should(Equal(expectedChannelInfo))

			By("Waiting for the added orderer to see the leader")
			Eventually(orderer5Runner.Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1 channel=testchannel1"))

			By("Killing the leader orderer")
			ordererProcesses[0].Signal(syscall.SIGTERM)
			Eventually(ordererProcesses[0].Wait(), network.EventuallyTimeout).Should(Receive())

			By("Waiting for view change to occur")
			Eventually(ordererRunners[1].Err(), network.EventuallyTimeout*2, time.Second).Should(gbytes.Say("Changing to leader role, current view: 1, current leader: 2 channel=testchannel1"))
			Eventually(ordererRunners[2].Err(), network.EventuallyTimeout*2, time.Second).Should(gbytes.Say("Changing to follower role, current view: 1, current leader: 2 channel=testchannel1"))
			Eventually(ordererRunners[3].Err(), network.EventuallyTimeout*2, time.Second).Should(gbytes.Say("Changing to follower role, current view: 1, current leader: 2 channel=testchannel1"))
			Eventually(ordererRunners[4].Err(), network.EventuallyTimeout*2, time.Second).Should(gbytes.Say("Changing to follower role, current view: 1, current leader: 2 channel=testchannel1"))

			assertBlockReception(map[string]int{"testchannel1": 7}, network.Orderers[1:], peer, network)

			By("Transacting")
			invokeQuery(network, peer, network.Orderers[2], channel, 70)

			By("Ensuring blacklisting is skipped due to reconfig")
			Eventually(ordererRunners[1].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Skipping verifying prev commit signatures due to verification sequence advancing from 0 to 1 channel=testchannel1"))
			Eventually(ordererRunners[2].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Skipping verifying prev commit signatures due to verification sequence advancing from 0 to 1 channel=testchannel1"))
			Eventually(ordererRunners[3].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Skipping verifying prev commit signatures due to verification sequence advancing from 0 to 1 channel=testchannel1"))
			Eventually(ordererRunners[4].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Skipping verifying prev commit signatures due to verification sequence advancing from 0 to 1 channel=testchannel1"))

			assertBlockReception(map[string]int{"testchannel1": 8}, network.Orderers[1:], peer, network)
		})
	})
})

func invokeQuery(network *nwo.Network, peer *nwo.Peer, orderer *nwo.Orderer, channel string, expectedBalance int) {
	sess, err := network.PeerUserSession(peer, "User1", commands.ChaincodeInvoke{
		ChannelID: channel,
		Orderer:   network.OrdererAddress(orderer, nwo.ListenPort),
		Name:      "mycc",
		Ctor:      `{"Args":["invoke","a","b","10"]}`,
		PeerAddresses: []string{
			network.PeerAddress(network.Peer("Org1", "peer0"), nwo.ListenPort),
			network.PeerAddress(network.Peer("Org2", "peer0"), nwo.ListenPort),
		},
		WaitForEvent: true,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess.Err).To(gbytes.Say("Chaincode invoke successful. result: status:200"))

	queryExpect(network, peer, channel, "a", expectedBalance)
}

func queryExpect(network *nwo.Network, peer *nwo.Peer, channel string, key string, expectedBalance int) {
	Eventually(func() string {
		sess, err := network.PeerUserSession(peer, "User1", commands.ChaincodeQuery{
			ChannelID: channel,
			Name:      "mycc",
			Ctor:      fmt.Sprintf(`{"Args":["query","%s"]}`, key),
		})
		Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit())
		if sess.ExitCode() != 0 {
			return fmt.Sprintf("exit code is %d: %s, %v", sess.ExitCode(), string(sess.Err.Contents()), err)
		}

		outStr := strings.TrimSpace(string(sess.Out.Contents()))
		if outStr != fmt.Sprintf("%d", expectedBalance) {
			return fmt.Sprintf("Error: expected: %d, received %s", expectedBalance, outStr)
		}
		return ""
	}, network.EventuallyTimeout, time.Second).Should(BeEmpty())
}

// assertBlockReception asserts that the given orderers have expected heights for the given channel--> height mapping
func assertBlockReception(expectedSequencesPerChannel map[string]int, orderers []*nwo.Orderer, p *nwo.Peer, n *nwo.Network) {
	defer GinkgoRecover()
	assertReception := func(channelName string, blockSeq int) {
		for _, orderer := range orderers {
			waitForBlockReception(orderer, p, n, channelName, blockSeq)
		}
	}

	for channelName, blockSeq := range expectedSequencesPerChannel {
		assertReception(channelName, blockSeq)
	}
}

func waitForBlockReception(o *nwo.Orderer, submitter *nwo.Peer, network *nwo.Network, channelName string, blockSeq int) {
	c := commands.ChannelFetch{
		ChannelID:  channelName,
		Block:      "newest",
		OutputFile: "/dev/null",
		Orderer:    network.OrdererAddress(o, nwo.ListenPort),
	}
	Eventually(func() string {
		sess, err := network.OrdererAdminSession(o, submitter, c)
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit())
		if sess.ExitCode() != 0 {
			return fmt.Sprintf("exit code is %d: %s", sess.ExitCode(), string(sess.Err.Contents()))
		}
		sessErr := string(sess.Err.Contents())
		expected := fmt.Sprintf("Received block: %d", blockSeq)
		if strings.Contains(sessErr, expected) {
			return ""
		}
		return sessErr
	}, network.EventuallyTimeout, time.Second).Should(BeEmpty())
}

func waitForBlockReceptionByPeer(peer *nwo.Peer, network *nwo.Network, channelName string, blockSeq uint64) {
	Eventually(func() bool {
		blockNumFromPeer := nwo.CurrentConfigBlockNumber(network, peer, nil, channelName)
		return blockNumFromPeer == blockSeq
	}, network.EventuallyTimeout, time.Second).Should(BeTrue())
}

func peerGroupRunners(n *nwo.Network) (ifrit.Runner, []*ginkgomon.Runner) {
	runners := []*ginkgomon.Runner{}
	members := grouper.Members{}
	for _, p := range n.Peers {
		runner := n.PeerRunner(p)
		members = append(members, grouper.Member{Name: p.ID(), Runner: runner})
		runners = append(runners, runner)
	}
	return grouper.NewParallel(syscall.SIGTERM, members), runners
}

func extractTarGZ(archive []byte, baseDir string) error {
	gzReader, err := gzip.NewReader(bytes.NewBuffer(archive))
	if err != nil {
		return err
	}

	tarReader := tar.NewReader(gzReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}

		if err != nil {
			return err
		}

		filePath := filepath.Join(baseDir, header.Name)
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.Mkdir(filePath, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			fd, err := os.Create(filePath)
			if err != nil {
				return err
			}
			_, err = io.Copy(fd, tarReader)
			if err != nil {
				return err
			}
		}
	}

	return nil
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

func joinChannel(network *nwo.Network, channel string) {
	sess, err := network.ConfigTxGen(commands.OutputBlock{
		ChannelID:   channel,
		Profile:     network.Profiles[0].Name,
		ConfigPath:  network.RootDir,
		OutputBlock: network.OutputBlockPath(channel),
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))

	genesisBlockBytes, err := os.ReadFile(network.OutputBlockPath(channel))
	Expect(err).NotTo(HaveOccurred())

	genesisBlock := &common.Block{}
	err = proto.Unmarshal(genesisBlockBytes, genesisBlock)
	Expect(err).NotTo(HaveOccurred())

	expectedChannelInfoPT := channelparticipation.ChannelInfo{
		Name:              channel,
		URL:               "/participation/v1/channels/" + channel,
		Status:            "active",
		ConsensusRelation: "consenter",
		Height:            1,
	}

	for _, o := range network.Orderers {
		By("joining " + o.Name + " to channel as a consenter")
		channelparticipation.Join(network, o, channel, genesisBlock, expectedChannelInfoPT)
		channelInfo := channelparticipation.ListOne(network, o, channel)
		Expect(channelInfo).To(Equal(expectedChannelInfoPT))
	}
}

func deployChaincode(network *nwo.Network, channel string, testDir string) {
	nwo.DeployChaincode(network, channel, network.Orderers[0], nwo.Chaincode{
		Name:            "mycc",
		Version:         "0.0",
		Path:            components.Build("github.com/hyperledger/fabric/integration/chaincode/simple/cmd"),
		Lang:            "binary",
		PackageFile:     filepath.Join(testDir, "simplecc.tar.gz"),
		Ctor:            `{"Args":["init","a","100","b","200"]}`,
		SignaturePolicy: `AND ('Org1MSP.member','Org2MSP.member')`,
		Sequence:        "1",
		InitRequired:    true,
		Label:           "my_prebuilt_chaincode",
	})
}
