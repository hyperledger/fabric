/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package e2e

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-lib-go/healthz"
	"github.com/hyperledger/fabric/common/tools/configtxgen/encoder"
	"github.com/hyperledger/fabric/common/tools/configtxgen/localconfig"
	"github.com/hyperledger/fabric/core/aclmgmt/resources"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	"github.com/hyperledger/fabric/integration/nwo/fabricconfig"
	"github.com/hyperledger/fabric/protos/common"
	protosorderer "github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/orderer/etcdraft"
	"github.com/hyperledger/fabric/protos/utils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/ginkgomon"
)

var _ = Describe("EndToEnd", func() {
	var (
		testDir   string
		client    *docker.Client
		network   *nwo.Network
		chaincode nwo.Chaincode
		process   ifrit.Process
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
		if network != nil {
			network.Cleanup()
		}
		// os.RemoveAll(testDir)
	})

	Describe("basic solo network with 2 orgs", func() {
		var datagramReader *DatagramReader

		BeforeEach(func() {
			datagramReader = NewDatagramReader()
			go datagramReader.Start()

			network = nwo.New(nwo.BasicSolo(), testDir, client, BasePort(), components)
			network.MetricsProvider = "statsd"
			network.StatsdEndpoint = datagramReader.Address()
			network.Profiles = append(network.Profiles, &nwo.Profile{
				Name:          "TwoOrgsBaseProfileChannel",
				Consortium:    "SampleConsortium",
				Orderers:      []string{"orderer"},
				Organizations: []string{"Org1", "Org2"},
			})
			network.Channels = append(network.Channels, &nwo.Channel{
				Name:        "baseprofilechannel",
				Profile:     "TwoOrgsBaseProfileChannel",
				BaseProfile: "TwoOrgsOrdererGenesis",
			})

			network.GenerateConfigTree()
			network.Bootstrap()

			networkRunner := network.NetworkGroupRunner()
			process = ifrit.Invoke(networkRunner)
			Eventually(process.Ready(), network.EventuallyTimeout).Should(BeClosed())
		})

		AfterEach(func() {
			if datagramReader != nil {
				datagramReader.Close()
			}
		})

		It("executes a basic solo network with 2 orgs", func() {
			By("getting the orderer by name")
			orderer := network.Orderer("orderer")

			By("setting up the channel")
			network.CreateAndJoinChannel(orderer, "testchannel")

			By("deploying the chaincode")
			nwo.DeployChaincode(network, "testchannel", orderer, chaincode)

			By("getting the client peer by name")
			peer := network.Peer("Org1", "peer1")

			RunQueryInvokeQuery(network, orderer, peer, "testchannel")
			RunRespondWith(network, orderer, peer, "testchannel")

			By("waiting for DeliverFiltered stats to be emitted")
			metricsWriteInterval := 5 * time.Second
			Eventually(datagramReader, 2*metricsWriteInterval).Should(gbytes.Say("stream_request_duration.protos_Deliver.DeliverFiltered."))

			CheckPeerStatsdStreamMetrics(datagramReader.String())
			CheckPeerStatsdMetrics(datagramReader.String(), "org1_peer0")
			CheckPeerStatsdMetrics(datagramReader.String(), "org2_peer1")
			CheckOrdererStatsdMetrics(datagramReader.String(), "ordererorg_orderer")

			By("setting up a channel from a base profile")
			additionalPeer := network.Peer("Org2", "peer1")
			network.CreateChannel("baseprofilechannel", orderer, peer, additionalPeer)
		})
	})

	Describe("basic kafka network with 2 orgs", func() {
		BeforeEach(func() {
			network = nwo.New(nwo.BasicKafka(), testDir, client, BasePort(), components)
			network.MetricsProvider = "prometheus"
			network.GenerateConfigTree()
			network.Bootstrap()

			networkRunner := network.NetworkGroupRunner()
			process = ifrit.Invoke(networkRunner)
			Eventually(process.Ready(), network.EventuallyTimeout).Should(BeClosed())
		})

		It("executes a basic kafka network with 2 orgs", func() {
			orderer := network.Orderer("orderer")
			peer := network.Peer("Org1", "peer1")

			network.CreateAndJoinChannel(orderer, "testchannel")
			nwo.DeployChaincode(network, "testchannel", orderer, chaincode)
			RunQueryInvokeQuery(network, orderer, peer, "testchannel")

			CheckPeerOperationEndpoints(network, network.Peer("Org2", "peer1"))
			CheckOrdererOperationEndpoints(network, orderer)
		})
	})

	Describe("basic single node etcdraft network", func() {
		var (
			peerRunners    []*ginkgomon.Runner
			processes      map[string]ifrit.Process
			ordererProcess ifrit.Process
		)

		BeforeEach(func() {
			network = nwo.New(nwo.MultiChannelEtcdRaft(), testDir, client, BasePort(), components)
			network.GenerateConfigTree()
			for _, peer := range network.Peers {
				core := network.ReadPeerConfig(peer)
				core.Peer.Gossip.UseLeaderElection = false
				core.Peer.Gossip.OrgLeader = true
				core.Peer.Deliveryclient.ReconnectTotalTimeThreshold = time.Duration(time.Second)
				network.WritePeerConfig(peer, core)
			}
			network.Bootstrap()

			ordererRunner := network.OrdererGroupRunner()
			ordererProcess = ifrit.Invoke(ordererRunner)
			Eventually(ordererProcess.Ready(), network.EventuallyTimeout).Should(BeClosed())

			peerRunners = make([]*ginkgomon.Runner, len(network.Peers))
			processes = map[string]ifrit.Process{}
			for i, peer := range network.Peers {
				pr := network.PeerRunner(peer)
				peerRunners[i] = pr
				p := ifrit.Invoke(pr)
				processes[peer.ID()] = p
				Eventually(p.Ready(), network.EventuallyTimeout).Should(BeClosed())
			}
		})

		AfterEach(func() {
			if ordererProcess != nil {
				ordererProcess.Signal(syscall.SIGTERM)
				Eventually(ordererProcess.Wait(), network.EventuallyTimeout).Should(Receive())
			}
			for _, p := range processes {
				p.Signal(syscall.SIGTERM)
				Eventually(p.Wait(), network.EventuallyTimeout).Should(Receive())
			}
		})

		It("creates two channels with two orgs trying to reconfigure and update metadata", func() {
			orderer := network.Orderer("orderer")
			peer := network.Peer("Org1", "peer1")

			By("Create first channel and deploy the chaincode")
			network.CreateAndJoinChannel(orderer, "testchannel1")
			nwo.DeployChaincode(network, "testchannel1", orderer, chaincode)
			RunQueryInvokeQuery(network, orderer, peer, "testchannel1")

			By("Create second channel and deploy chaincode")
			network.CreateAndJoinChannel(orderer, "testchannel2")
			nwo.InstantiateChaincode(network, "testchannel2", orderer, chaincode, peer, network.PeersWithChannel("testchannel2")...)
			RunQueryInvokeQuery(network, orderer, peer, "testchannel2")

			By("Update consensus metadata to increase snapshot interval")
			snapDir := path.Join(network.RootDir, "orderers", orderer.ID(), "etcdraft", "snapshot", "testchannel1")
			files, err := ioutil.ReadDir(snapDir)
			Expect(err).NotTo(HaveOccurred())
			numOfSnaps := len(files)

			nwo.UpdateConsensusMetadata(network, peer, orderer, "testchannel1", func(originalMetadata []byte) []byte {
				metadata := &etcdraft.ConfigMetadata{}
				err := proto.Unmarshal(originalMetadata, metadata)
				Expect(err).NotTo(HaveOccurred())

				// update max in flight messages
				metadata.Options.MaxInflightBlocks = 1000
				metadata.Options.SnapshotIntervalSize = 10 * 1024 * 1024 // 10 MB

				// write metadata back
				newMetadata, err := proto.Marshal(metadata)
				Expect(err).NotTo(HaveOccurred())
				return newMetadata
			})

			// assert that no new snapshot is taken because SnapshotIntervalSize has just enlarged
			files, err = ioutil.ReadDir(snapDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(files)).To(Equal(numOfSnaps))

			By("ensuring that static leaders do not give up on retrieving blocks after the orderer goes down")
			ordererProcess.Signal(syscall.SIGTERM)
			Eventually(ordererProcess.Wait(), network.EventuallyTimeout).Should(Receive())
			for _, peerRunner := range peerRunners {
				Eventually(peerRunner.Err(), network.EventuallyTimeout).Should(gbytes.Say("peer is a static leader, ignoring peer.deliveryclient.reconnectTotalTimeThreshold"))
			}

		})
	})

	Describe("three node etcdraft network with 2 orgs", func() {
		BeforeEach(func() {
			network = nwo.New(nwo.MultiNodeEtcdRaft(), testDir, client, BasePort(), components)
			network.GenerateConfigTree()
			network.Bootstrap()

			networkRunner := network.NetworkGroupRunner()
			process = ifrit.Invoke(networkRunner)
			Eventually(process.Ready(), network.EventuallyTimeout).Should(BeClosed())
		})

		// This tests:
		//
		// 1. channel creation with raft orderer,
		// 2. all the nodes on three-node raft cluster are in sync wrt blocks,
		// 3. raft orderer processes type A config updates and delivers the
		//    config blocks to the peers.
		It("executes an etcdraft network with 2 orgs and three orderer nodes", func() {
			orderer1 := network.Orderer("orderer1")
			orderer2 := network.Orderer("orderer2")
			orderer3 := network.Orderer("orderer3")
			peer := network.Peer("Org1", "peer1")
			org1Peer0 := network.Peer("Org1", "peer0")
			blockFile1 := filepath.Join(testDir, "newest_orderer1_block.pb")
			blockFile2 := filepath.Join(testDir, "newest_orderer2_block.pb")
			blockFile3 := filepath.Join(testDir, "newest_orderer3_block.pb")

			fetchLatestBlock := func(targetOrderer *nwo.Orderer, blockFile string) {
				c := commands.ChannelFetch{
					ChannelID:  "testchannel",
					Block:      "newest",
					OutputFile: blockFile,
				}
				if targetOrderer != nil {
					c.Orderer = network.OrdererAddress(targetOrderer, nwo.ListenPort)
				}
				sess, err := network.PeerAdminSession(org1Peer0, c)
				Expect(err).NotTo(HaveOccurred())
				Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
			}

			By("creating a new chain and having the peers join it to test for channel creation")
			network.CreateAndJoinChannel(orderer1, "testchannel")
			nwo.DeployChaincode(network, "testchannel", orderer1, chaincode)
			RunQueryInvokeQuery(network, orderer1, peer, "testchannel")

			// the above can work even if the orderer nodes are not in the same Raft
			// cluster; we need to verify all the three orderer nodes are in sync wrt
			// blocks.
			By("fetching the latest blocks from all the orderer nodes and testing them for equality")
			fetchLatestBlock(orderer1, blockFile1)
			fetchLatestBlock(orderer2, blockFile2)
			fetchLatestBlock(orderer3, blockFile3)
			b1 := nwo.UnmarshalBlockFromFile(blockFile1)
			b2 := nwo.UnmarshalBlockFromFile(blockFile2)
			b3 := nwo.UnmarshalBlockFromFile(blockFile3)
			Expect(bytes.Equal(b1.Header.Bytes(), b2.Header.Bytes())).To(BeTrue())
			Expect(bytes.Equal(b2.Header.Bytes(), b3.Header.Bytes())).To(BeTrue())

			By("updating ACL policies to test for type A configuration updates")
			invokeChaincode := commands.ChaincodeInvoke{
				ChannelID:    "testchannel",
				Orderer:      network.OrdererAddress(orderer1, nwo.ListenPort),
				Name:         chaincode.Name,
				Ctor:         `{"Args":["invoke","a","b","10"]}`,
				WaitForEvent: true,
			}
			// setting the filtered block event ACL policy to org2/Admins
			policyName := resources.Event_FilteredBlock
			policy := "/Channel/Application/org2/Admins"
			SetACLPolicy(network, "testchannel", policyName, policy, "orderer1")
			// invoking chaincode as a forbidden Org1 Admin identity
			sess, err := network.PeerAdminSession(org1Peer0, invokeChaincode)
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess.Err, network.EventuallyTimeout).Should(gbytes.Say(`\Qdeliver completed with status (FORBIDDEN)\E`))
		})
	})

	Describe("Invalid Raft config metadata", func() {
		It("refuses to start orderer or rejects config update", func() {
			By("Creating malformed genesis block")
			network = nwo.New(nwo.BasicEtcdRaft(), testDir, client, BasePort(), components)
			network.GenerateConfigTree()
			network.Bootstrap()

			sysProfile := localconfig.Load(network.SystemChannel.Profile, network.RootDir)
			Expect(sysProfile.Orderer).NotTo(BeNil())
			sysProfile.Orderer.EtcdRaft.Options.ElectionTick = sysProfile.Orderer.EtcdRaft.Options.HeartbeatTick
			pgen := encoder.New(sysProfile)
			genesisBlock := pgen.GenesisBlockForChannel(network.SystemChannel.Name)
			data, err := proto.Marshal(genesisBlock)
			Expect(err).NotTo(HaveOccurred())
			ioutil.WriteFile(network.OutputBlockPath(network.SystemChannel.Name), data, 0644)

			By("Starting orderer with malformed genesis block")
			ordererRunner := network.OrdererGroupRunner()
			process = ifrit.Invoke(ordererRunner)
			Eventually(process.Wait, network.EventuallyTimeout).Should(Receive()) // orderer process should exit
			os.RemoveAll(testDir)

			By("Starting orderer with correct genesis block")
			testDir, err = ioutil.TempDir("", "e2e")
			Expect(err).NotTo(HaveOccurred())
			network = nwo.New(nwo.BasicEtcdRaft(), testDir, client, BasePort(), components)
			network.GenerateConfigTree()
			network.Bootstrap()

			orderer := network.Orderer("orderer")
			runner := network.OrdererRunner(orderer)
			process = ifrit.Invoke(runner)
			Eventually(process.Ready, network.EventuallyTimeout).Should(BeClosed())

			By("Waiting for system channel to be ready")
			findLeader([]*ginkgomon.Runner{runner})

			By("Creating malformed channel creation config tx")
			channel := "testchannel"
			sysProfile = localconfig.Load(network.SystemChannel.Profile, network.RootDir)
			Expect(sysProfile.Orderer).NotTo(BeNil())
			appProfile := localconfig.Load(network.ProfileForChannel(channel), network.RootDir)
			Expect(appProfile).NotTo(BeNil())
			o := *sysProfile.Orderer
			appProfile.Orderer = &o
			appProfile.Orderer.EtcdRaft = proto.Clone(sysProfile.Orderer.EtcdRaft).(*etcdraft.ConfigMetadata)
			appProfile.Orderer.EtcdRaft.Options.HeartbeatTick = appProfile.Orderer.EtcdRaft.Options.ElectionTick
			configtx, err := encoder.MakeChannelCreationTransactionWithSystemChannelContext(channel, nil, appProfile, sysProfile)
			Expect(err).NotTo(HaveOccurred())
			data, err = proto.Marshal(configtx)
			Expect(err).NotTo(HaveOccurred())
			ioutil.WriteFile(network.CreateChannelTxPath(channel), data, 0644)

			By("Submitting malformed channel creation config tx to orderer")
			peer1org1 := network.Peer("Org1", "peer1")
			peer1org2 := network.Peer("Org2", "peer1")

			network.CreateChannelFail(channel, orderer, peer1org1, peer1org1, peer1org2, orderer)
			Consistently(process.Wait).ShouldNot(Receive()) // malformed tx should not crash orderer
			Expect(runner.Err()).To(gbytes.Say(`rejected by Configure: ElectionTick \(10\) must be greater than HeartbeatTick \(10\)`))

			By("Submitting channel config update with illegal value")
			channel = network.SystemChannel.Name
			config := nwo.GetConfig(network, peer1org1, orderer, channel)
			updatedConfig := proto.Clone(config).(*common.Config)

			consensusTypeConfigValue := updatedConfig.ChannelGroup.Groups["Orderer"].Values["ConsensusType"]
			consensusTypeValue := &protosorderer.ConsensusType{}
			Expect(proto.Unmarshal(consensusTypeConfigValue.Value, consensusTypeValue)).To(Succeed())

			metadata := &etcdraft.ConfigMetadata{}
			Expect(proto.Unmarshal(consensusTypeValue.Metadata, metadata)).To(Succeed())

			metadata.Options.HeartbeatTick = 10
			metadata.Options.ElectionTick = 10

			newMetadata, err := proto.Marshal(metadata)
			Expect(err).NotTo(HaveOccurred())
			consensusTypeValue.Metadata = newMetadata

			updatedConfig.ChannelGroup.Groups["Orderer"].Values["ConsensusType"] = &common.ConfigValue{
				ModPolicy: "Admins",
				Value:     utils.MarshalOrPanic(consensusTypeValue),
			}

			nwo.UpdateOrdererConfigFail(network, orderer, channel, config, updatedConfig, peer1org1, orderer)
		})
	})

	Describe("etcd raft, checking valid configuration update of type B", func() {
		BeforeEach(func() {
			network = nwo.New(nwo.BasicEtcdRaft(), testDir, client, BasePort(), components)
			network.GenerateConfigTree()
			network.Bootstrap()

			networkRunner := network.NetworkGroupRunner()
			process = ifrit.Invoke(networkRunner)
			Eventually(process.Ready(), network.EventuallyTimeout).Should(BeClosed())
		})

		It("executes a basic etcdraft network with a single Raft node", func() {
			orderer := network.Orderer("orderer")
			peer := network.Peer("Org1", "peer1")

			channel := "testchannel"
			network.CreateAndJoinChannel(orderer, channel)
			nwo.DeployChaincode(network, "testchannel", orderer, chaincode)
			RunQueryInvokeQuery(network, orderer, peer, "testchannel")

			snapDir := path.Join(network.RootDir, "orderers", orderer.ID(), "etcdraft", "snapshot", channel)
			files, err := ioutil.ReadDir(snapDir)
			Expect(err).NotTo(HaveOccurred())
			numOfSnaps := len(files)

			nwo.UpdateConsensusMetadata(network, peer, orderer, channel, func(originalMetadata []byte) []byte {
				metadata := &etcdraft.ConfigMetadata{}
				err := proto.Unmarshal(originalMetadata, metadata)
				Expect(err).NotTo(HaveOccurred())

				// update max in flight messages
				metadata.Options.MaxInflightBlocks = 1000
				metadata.Options.SnapshotIntervalSize = 10 * 1024 * 1024 // 10 MB

				// write metadata back
				newMetadata, err := proto.Marshal(metadata)
				Expect(err).NotTo(HaveOccurred())
				return newMetadata
			})

			// assert that no new snapshot is taken because SnapshotIntervalSize has just enlarged
			files, err = ioutil.ReadDir(snapDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(files)).To(Equal(numOfSnaps))
		})
	})

	Describe("basic single node etcdraft network with 2 orgs and 2 channels", func() {
		BeforeEach(func() {
			network = nwo.New(nwo.MultiChannelEtcdRaft(), testDir, client, BasePort(), components)
			network.GenerateConfigTree()
			network.Bootstrap()

			networkRunner := network.NetworkGroupRunner()
			process = ifrit.Invoke(networkRunner)
			Eventually(process.Ready(), network.EventuallyTimeout).Should(BeClosed())
		})

		It("executes a basic etcdraft network with 2 orgs and 2 channels", func() {
			orderer := network.Orderer("orderer")
			peer := network.Peer("Org1", "peer1")

			network.CreateAndJoinChannel(orderer, "testchannel1")
			nwo.DeployChaincode(network, "testchannel1", orderer, chaincode)

			network.CreateAndJoinChannel(orderer, "testchannel2")
			nwo.InstantiateChaincode(network, "testchannel2", orderer, chaincode, peer, network.PeersWithChannel("testchannel2")...)

			RunQueryInvokeQuery(network, orderer, peer, "testchannel2")
			RunQueryInvokeQuery(network, orderer, peer, "testchannel1")
		})
	})

	Describe("single node etcdraft network with remapped orderer endpoints", func() {
		BeforeEach(func() {
			network = nwo.New(nwo.MinimalRaft(), testDir, client, BasePort(), components)
			network.GenerateConfigTree()

			ordererMSPDir := network.OrdererOrgMSPDir(network.OrdererOrgs()[0])
			modifiedOrdererMSPDir := filepath.Join(network.RootDir, "TLSLessOrdererMSP")

			configtxConfig := network.ReadConfigTxConfig()
			ordererOrg := configtxConfig.Profiles["SampleDevModeEtcdRaft"].Orderer.Organizations[0]
			ordererOrg.MSPDir = modifiedOrdererMSPDir
			ordererEndpoints := ordererOrg.OrdererEndpoints
			correctOrdererEndpoint := ordererEndpoints[0]
			ordererEndpoints[0] = "127.0.0.1:1"
			network.WriteConfigTxConfig(configtxConfig)

			peer := network.Peer("Org1", "peer0")
			peerConfig := network.ReadPeerConfig(peer)
			peerConfig.Peer.Deliveryclient.AddressOverrides = []*fabricconfig.AddressOverride{
				{
					From:        "127.0.0.1:1",
					To:          correctOrdererEndpoint,
					CACertsFile: network.CACertsBundlePath(),
				},
			}
			network.WritePeerConfig(peer, peerConfig)

			_, err := network.DockerClient.CreateNetwork(
				docker.CreateNetworkOptions{
					Name:   network.NetworkID,
					Driver: "bridge",
				},
			)
			Expect(err).NotTo(HaveOccurred())

			sess, err := network.Cryptogen(commands.Generate{
				Config: network.CryptoConfigPath(),
				Output: network.CryptoPath(),
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))

			// Make a copy of the orderer MSP, stripping out the TLS certs
			filepath.Walk(ordererMSPDir, func(path string, info os.FileInfo, err error) error {
				Expect(err).NotTo(HaveOccurred())

				relPath, err := filepath.Rel(ordererMSPDir, path)
				Expect(err).NotTo(HaveOccurred())

				relDir := filepath.Dir(relPath)
				err = os.MkdirAll(filepath.Join(modifiedOrdererMSPDir, relDir), 0700)
				Expect(err).NotTo(HaveOccurred())

				if info.IsDir() {
					return nil
				}

				switch filepath.Base(relDir) {
				case "tlscacerts", "tlsintermediatecerts":
					return nil
				default:
				}

				s, err := os.OpenFile(path, os.O_RDONLY, info.Mode())
				Expect(err).NotTo(HaveOccurred())
				defer s.Close()

				target := filepath.Join(modifiedOrdererMSPDir, relPath)
				f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, info.Mode())
				Expect(err).NotTo(HaveOccurred())

				if _, err := io.Copy(f, s); err != nil {
					return err
				}

				f.Close()
				return nil
			})

			sess, err = network.ConfigTxGen(commands.OutputBlock{
				ChannelID:   network.SystemChannel.Name,
				Profile:     network.SystemChannel.Profile,
				ConfigPath:  network.RootDir,
				OutputBlock: network.OutputBlockPath(network.SystemChannel.Name),
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))

			for _, c := range network.Channels {
				sess, err := network.ConfigTxGen(commands.CreateChannelTx{
					ChannelID:             c.Name,
					Profile:               c.Profile,
					BaseProfile:           c.BaseProfile,
					ConfigPath:            network.RootDir,
					OutputCreateChannelTx: network.CreateChannelTxPath(c.Name),
				})
				Expect(err).NotTo(HaveOccurred())
				Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
			}

			network.ConcatenateTLSCACertificates()

			networkRunner := network.NetworkGroupRunner()
			process = ifrit.Invoke(networkRunner)
			Eventually(process.Ready(), network.EventuallyTimeout).Should(BeClosed())
		})

		It("creates and updates channel", func() {
			orderer := network.Orderer("orderer")

			network.CreateAndJoinChannel(orderer, "testchannel")

			// The below call waits for the config update to commit on the peer, so
			// it will fail if the orderer addresses are wrong.
			nwo.EnableCapabilities(network, "testchannel", "Application", "V1_4_2", orderer, network.Peer("Org1", "peer0"), network.Peer("Org2", "peer0"))
		})
	})
})

func RunQueryInvokeQuery(n *nwo.Network, orderer *nwo.Orderer, peer *nwo.Peer, channel string) {
	By("querying the chaincode")
	sess, err := n.PeerUserSession(peer, "User1", commands.ChaincodeQuery{
		ChannelID: channel,
		Name:      "mycc",
		Ctor:      `{"Args":["query","a"]}`,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess).To(gbytes.Say("100"))

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

	sess, err = n.PeerUserSession(peer, "User1", commands.ChaincodeQuery{
		ChannelID: channel,
		Name:      "mycc",
		Ctor:      `{"Args":["query","a"]}`,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess).To(gbytes.Say("90"))
}

func RunRespondWith(n *nwo.Network, orderer *nwo.Orderer, peer *nwo.Peer, channel string) {
	By("responding with a 300")
	sess, err := n.PeerUserSession(peer, "User1", commands.ChaincodeInvoke{
		ChannelID: channel,
		Orderer:   n.OrdererAddress(orderer, nwo.ListenPort),
		Name:      "mycc",
		Ctor:      `{"Args":["respond","300","response-message","response-payload"]}`,
		PeerAddresses: []string{
			n.PeerAddress(n.Peer("Org2", "peer1"), nwo.ListenPort),
		},
		WaitForEvent: true,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, time.Minute).Should(gexec.Exit(0))
	Expect(sess.Err).To(gbytes.Say("Chaincode invoke successful. result: status:300"))

	By("responding with a 400")
	sess, err = n.PeerUserSession(peer, "User1", commands.ChaincodeInvoke{
		ChannelID: channel,
		Orderer:   n.OrdererAddress(orderer, nwo.ListenPort),
		Name:      "mycc",
		Ctor:      `{"Args":["respond","400","response-message","response-payload"]}`,
		PeerAddresses: []string{
			n.PeerAddress(n.Peer("Org2", "peer1"), nwo.ListenPort),
		},
		WaitForEvent: true,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, time.Minute).Should(gexec.Exit(1))
	Expect(sess.Err).To(gbytes.Say(`Error: endorsement failure during invoke.`))
}

func CheckPeerStatsdMetrics(contents, prefix string) {
	By("checking for peer statsd metrics")
	Expect(contents).To(ContainSubstring(prefix + ".logging.entries_checked.info:"))
	Expect(contents).To(ContainSubstring(prefix + ".logging.entries_written.info:"))
	Expect(contents).To(ContainSubstring(prefix + ".go.mem.gc_completed_count:"))
	Expect(contents).To(ContainSubstring(prefix + ".grpc.server.unary_requests_received.protos_Endorser.ProcessProposal:"))
	Expect(contents).To(ContainSubstring(prefix + ".grpc.server.unary_requests_completed.protos_Endorser.ProcessProposal.OK:"))
	Expect(contents).To(ContainSubstring(prefix + ".grpc.server.unary_request_duration.protos_Endorser.ProcessProposal.OK:"))
	Expect(contents).To(ContainSubstring(prefix + ".ledger.blockchain_height"))
	Expect(contents).To(ContainSubstring(prefix + ".ledger.blockstorage_commit_time"))
	Expect(contents).To(ContainSubstring(prefix + ".ledger.blockstorage_and_pvtdata_commit_time"))
}

func CheckPeerStatsdStreamMetrics(contents string) {
	By("checking for stream metrics")
	Expect(contents).To(ContainSubstring(".grpc.server.stream_requests_received.protos_Deliver.DeliverFiltered:"))
	Expect(contents).To(ContainSubstring(".grpc.server.stream_requests_completed.protos_Deliver.DeliverFiltered.Unknown:"))
	Expect(contents).To(ContainSubstring(".grpc.server.stream_request_duration.protos_Deliver.DeliverFiltered.Unknown:"))
	Expect(contents).To(ContainSubstring(".grpc.server.stream_messages_received.protos_Deliver.DeliverFiltered"))
	Expect(contents).To(ContainSubstring(".grpc.server.stream_messages_sent.protos_Deliver.DeliverFiltered"))
}

func CheckOrdererStatsdMetrics(contents, prefix string) {
	By("checking for AtomicBroadcast")
	Expect(contents).To(ContainSubstring(prefix + ".grpc.server.stream_request_duration.orderer_AtomicBroadcast.Broadcast.OK"))
	Expect(contents).To(ContainSubstring(prefix + ".grpc.server.stream_request_duration.orderer_AtomicBroadcast.Deliver."))

	By("checking for orderer metrics")
	Expect(contents).To(ContainSubstring(prefix + ".logging.entries_checked.info:"))
	Expect(contents).To(ContainSubstring(prefix + ".logging.entries_written.info:"))
	Expect(contents).To(ContainSubstring(prefix + ".go.mem.gc_completed_count:"))
	Expect(contents).To(ContainSubstring(prefix + ".grpc.server.stream_requests_received.orderer_AtomicBroadcast.Deliver:"))
	Expect(contents).To(ContainSubstring(prefix + ".grpc.server.stream_requests_completed.orderer_AtomicBroadcast.Deliver."))
	Expect(contents).To(ContainSubstring(prefix + ".grpc.server.stream_messages_received.orderer_AtomicBroadcast.Deliver"))
	Expect(contents).To(ContainSubstring(prefix + ".grpc.server.stream_messages_sent.orderer_AtomicBroadcast.Deliver"))
	Expect(contents).To(ContainSubstring(prefix + ".ledger.blockchain_height"))
	Expect(contents).To(ContainSubstring(prefix + ".ledger.blockstorage_commit_time"))
}

func OrdererOperationalClients(network *nwo.Network, orderer *nwo.Orderer) (authClient, unauthClient *http.Client) {
	return operationalClients(network.OrdererLocalTLSDir(orderer))
}

func PeerOperationalClients(network *nwo.Network, peer *nwo.Peer) (authClient, unauthClient *http.Client) {
	return operationalClients(network.PeerLocalTLSDir(peer))
}

func operationalClients(tlsDir string) (authClient, unauthClient *http.Client) {
	clientCert, err := tls.LoadX509KeyPair(
		filepath.Join(tlsDir, "server.crt"),
		filepath.Join(tlsDir, "server.key"),
	)
	Expect(err).NotTo(HaveOccurred())

	clientCertPool := x509.NewCertPool()
	caCert, err := ioutil.ReadFile(filepath.Join(tlsDir, "ca.crt"))
	Expect(err).NotTo(HaveOccurred())
	clientCertPool.AppendCertsFromPEM(caCert)

	authenticatedClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				Certificates: []tls.Certificate{clientCert},
				RootCAs:      clientCertPool,
			},
		},
	}
	unauthenticatedClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: clientCertPool},
		},
	}

	return authenticatedClient, unauthenticatedClient
}

func CheckPeerOperationEndpoints(network *nwo.Network, peer *nwo.Peer) {
	metricsURL := fmt.Sprintf("https://127.0.0.1:%d/metrics", network.PeerPort(peer, nwo.OperationsPort))
	logspecURL := fmt.Sprintf("https://127.0.0.1:%d/logspec", network.PeerPort(peer, nwo.OperationsPort))
	healthURL := fmt.Sprintf("https://127.0.0.1:%d/healthz", network.PeerPort(peer, nwo.OperationsPort))

	authClient, unauthClient := PeerOperationalClients(network, peer)

	CheckPeerPrometheusMetrics(authClient, metricsURL)
	CheckLogspecOperations(authClient, logspecURL)
	CheckHealthEndpoint(authClient, healthURL)

	By("getting the logspec without a client cert")
	resp, err := unauthClient.Get(logspecURL)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))

	By("ensuring health checks do not require a client cert")
	CheckHealthEndpoint(unauthClient, healthURL)
}

func CheckOrdererOperationEndpoints(network *nwo.Network, orderer *nwo.Orderer) {
	metricsURL := fmt.Sprintf("https://127.0.0.1:%d/metrics", network.OrdererPort(orderer, nwo.OperationsPort))
	logspecURL := fmt.Sprintf("https://127.0.0.1:%d/logspec", network.OrdererPort(orderer, nwo.OperationsPort))
	healthURL := fmt.Sprintf("https://127.0.0.1:%d/healthz", network.OrdererPort(orderer, nwo.OperationsPort))

	authClient, unauthClient := OrdererOperationalClients(network, orderer)

	CheckOrdererPrometheusMetrics(authClient, metricsURL)
	CheckLogspecOperations(authClient, logspecURL)
	CheckHealthEndpoint(authClient, healthURL)

	By("getting the logspec without a client cert")
	resp, err := unauthClient.Get(logspecURL)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))

	By("ensuring health checks do not require a client cert")
	CheckHealthEndpoint(unauthClient, healthURL)
}

func CheckPeerPrometheusMetrics(client *http.Client, url string) {
	By("hitting the prometheus metrics endpoint")
	resp, err := client.Get(url)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	resp.Body.Close()

	Eventually(getBody(client, url)).Should(ContainSubstring(`# TYPE grpc_server_stream_request_duration histogram`))

	By("checking for some expected metrics")
	body := getBody(client, url)()
	Expect(body).To(ContainSubstring(`# TYPE go_gc_duration_seconds summary`))
	Expect(body).To(ContainSubstring(`# TYPE grpc_server_stream_request_duration histogram`))
	Expect(body).To(ContainSubstring(`grpc_server_stream_request_duration_count{code="Unknown",method="DeliverFiltered",service="protos_Deliver"}`))
	Expect(body).To(ContainSubstring(`grpc_server_stream_messages_received{method="DeliverFiltered",service="protos_Deliver"}`))
	Expect(body).To(ContainSubstring(`grpc_server_stream_messages_sent{method="DeliverFiltered",service="protos_Deliver"}`))
	Expect(body).To(ContainSubstring(`# TYPE grpc_comm_conn_closed counter`))
	Expect(body).To(ContainSubstring(`# TYPE grpc_comm_conn_opened counter`))
	Expect(body).To(ContainSubstring(`ledger_blockchain_height`))
	Expect(body).To(ContainSubstring(`ledger_blockstorage_commit_time_bucket`))
	Expect(body).To(ContainSubstring(`ledger_blockstorage_and_pvtdata_commit_time_bucket`))
}

func CheckOrdererPrometheusMetrics(client *http.Client, url string) {
	By("hitting the prometheus metrics endpoint")
	resp, err := client.Get(url)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	resp.Body.Close()

	Eventually(getBody(client, url)).Should(ContainSubstring(`# TYPE grpc_server_stream_request_duration histogram`))

	By("checking for some expected metrics")
	body := getBody(client, url)()
	Expect(body).To(ContainSubstring(`# TYPE go_gc_duration_seconds summary`))
	Expect(body).To(ContainSubstring(`# TYPE grpc_server_stream_request_duration histogram`))
	Expect(body).To(ContainSubstring(`grpc_server_stream_request_duration_sum{code="OK",method="Deliver",service="orderer_AtomicBroadcast"`))
	Expect(body).To(ContainSubstring(`grpc_server_stream_request_duration_sum{code="OK",method="Broadcast",service="orderer_AtomicBroadcast"`))
	Expect(body).To(ContainSubstring(`# TYPE grpc_comm_conn_closed counter`))
	Expect(body).To(ContainSubstring(`# TYPE grpc_comm_conn_opened counter`))
	Expect(body).To(ContainSubstring(`ledger_blockchain_height`))
	Expect(body).To(ContainSubstring(`ledger_blockstorage_commit_time_bucket`))
}

func CheckLogspecOperations(client *http.Client, logspecURL string) {
	By("getting the logspec")
	resp, err := client.Get(logspecURL)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	bodyBytes, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	Expect(err).NotTo(HaveOccurred())
	Expect(string(bodyBytes)).To(MatchJSON(`{"spec":"info"}`))

	updateReq, err := http.NewRequest(http.MethodPut, logspecURL, strings.NewReader(`{"spec":"debug"}`))
	Expect(err).NotTo(HaveOccurred())

	By("setting the logspec")
	resp, err = client.Do(updateReq)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusNoContent))

	resp, err = client.Get(logspecURL)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	bodyBytes, err = ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	Expect(err).NotTo(HaveOccurred())
	Expect(string(bodyBytes)).To(MatchJSON(`{"spec":"debug"}`))
}

func CheckHealthEndpoint(client *http.Client, url string) {
	body := getBody(client, url)()

	var healthStatus healthz.HealthStatus
	err := json.Unmarshal([]byte(body), &healthStatus)
	Expect(err).NotTo(HaveOccurred())
	Expect(healthStatus.Status).To(Equal(healthz.StatusOK))
}

func getBody(client *http.Client, url string) func() string {
	return func() string {
		resp, err := client.Get(url)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		bodyBytes, err := ioutil.ReadAll(resp.Body)
		Expect(err).NotTo(HaveOccurred())
		resp.Body.Close()
		return string(bodyBytes)
	}
}
