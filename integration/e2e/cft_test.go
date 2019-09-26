/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package e2e

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/cmd/common/signer"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/msp"
	"github.com/hyperledger/fabric/protos/utils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/ginkgomon"
	"github.com/tedsuo/ifrit/grouper"
)

var _ = Describe("EndToEnd Crash Fault Tolerance", func() {
	var (
		testDir   string
		client    *docker.Client
		network   *nwo.Network
		chaincode nwo.Chaincode
		peer      *nwo.Peer

		peerProc, ordererProc, o1Proc, o2Proc, o3Proc ifrit.Process
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
		for _, oProc := range []ifrit.Process{o1Proc, o2Proc, o3Proc} {
			if oProc != nil {
				oProc.Signal(syscall.SIGTERM)
				Eventually(oProc.Wait(), network.EventuallyTimeout).Should(Receive())
			}
		}

		if ordererProc != nil {
			ordererProc.Signal(syscall.SIGTERM)
			Eventually(ordererProc.Wait(), network.EventuallyTimeout).Should(Receive())
		}
		if peerProc != nil {
			peerProc.Signal(syscall.SIGTERM)
			Eventually(peerProc.Wait(), network.EventuallyTimeout).Should(Receive())
		}
		if network != nil {
			network.Cleanup()
		}
		os.RemoveAll(testDir)
	})

	fetchLatestBlock := func(targetOrderer *nwo.Orderer, blockFile string) {
		c := commands.ChannelFetch{
			ChannelID:  "testchannel",
			Block:      "newest",
			OutputFile: blockFile,
		}
		if targetOrderer != nil {
			c.Orderer = network.OrdererAddress(targetOrderer, nwo.ListenPort)
		}
		sess, err := network.PeerAdminSession(peer, c)
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
	}

	When("orderer stops and restarts", func() {
		It("keeps network up and running", func() {
			network = nwo.New(nwo.MultiNodeEtcdRaft(), testDir, client, 33000, components)

			o1, o2, o3 := network.Orderer("orderer1"), network.Orderer("orderer2"), network.Orderer("orderer3")
			peer = network.Peer("Org1", "peer1")
			blockFile1 := filepath.Join(testDir, "newest_orderer1_block.pb")
			blockFile2 := filepath.Join(testDir, "newest_orderer2_block.pb")

			network.GenerateConfigTree()
			network.Bootstrap()

			o1Runner := network.OrdererRunner(o1)
			orderers := grouper.Members{
				{Name: o2.ID(), Runner: network.OrdererRunner(o2)},
				{Name: o3.ID(), Runner: network.OrdererRunner(o3)},
			}
			ordererGroup := grouper.NewParallel(syscall.SIGTERM, orderers)
			peerGroup := network.PeerGroupRunner()

			o1Proc = ifrit.Invoke(o1Runner)
			ordererProc = ifrit.Invoke(ordererGroup)
			Eventually(o1Proc.Ready()).Should(BeClosed())
			Eventually(ordererProc.Ready()).Should(BeClosed())
			peerProc = ifrit.Invoke(peerGroup)
			Eventually(peerProc.Ready()).Should(BeClosed())

			By("performing operation with orderer1")
			network.CreateAndJoinChannel(o1, "testchannel")

			By("killing orderer1")
			o1Proc.Signal(syscall.SIGKILL)
			Eventually(o1Proc.Wait(), network.EventuallyTimeout).Should(Receive(MatchError("exit status 137")))

			By("performing operations with running orderer")
			nwo.DeployChaincode(network, "testchannel", o2, chaincode)

			By("restarting orderer1")
			o1Runner = network.OrdererRunner(o1)
			o1Proc = ifrit.Invoke(o1Runner)
			Eventually(o1Proc.Ready()).Should(BeClosed())

			By("executing transaction with restarted orderer")
			RunQueryInvokeQuery(network, o1, peer, "testchannel")

			fetchLatestBlock(o1, blockFile1)
			fetchLatestBlock(o2, blockFile2)
			b1 := nwo.UnmarshalBlockFromFile(blockFile1)
			b2 := nwo.UnmarshalBlockFromFile(blockFile2)
			Expect(b1.Header.Bytes()).To(Equal(b2.Header.Bytes()))
		})
	})

	When("an orderer is behind the latest snapshot on leader", func() {
		It("catches up using the block stored in snapshot", func() {
			// Steps:
			// - start o2 & o3
			// - send several transactions so snapshot is created
			// - kill o2 & o3, so that entries prior to snapshot are not in memory upon restart
			// - start o1 & o2
			// - assert that o1 can catch up with o2 using snapshot

			network = nwo.New(nwo.MultiNodeEtcdRaft(), testDir, client, 33000, components)
			o1, o2, o3 := network.Orderer("orderer1"), network.Orderer("orderer2"), network.Orderer("orderer3")

			peer = network.Peer("Org1", "peer1")
			blockFile1 := filepath.Join(testDir, "newest_orderer1_block.pb")
			blockFile2 := filepath.Join(testDir, "newest_orderer2_block.pb")

			network.GenerateConfigTree()
			network.Bootstrap()

			orderers := grouper.Members{
				{Name: o2.ID(), Runner: network.OrdererRunner(o2)},
				{Name: o3.ID(), Runner: network.OrdererRunner(o3)},
			}
			ordererGroup := grouper.NewParallel(syscall.SIGTERM, orderers)
			peerGroup := network.PeerGroupRunner()

			By("Starting 2/3 of cluster")
			ordererProc = ifrit.Invoke(ordererGroup)
			Eventually(ordererProc.Ready()).Should(BeClosed())
			peerProc = ifrit.Invoke(peerGroup)
			Eventually(peerProc.Ready()).Should(BeClosed())

			By("Creating channel and submitting several transactions to take snapshot")
			network.CreateAndJoinChannel(o2, "testchannel")
			nwo.DeployChaincode(network, "testchannel", o2, chaincode)

			for i := 1; i <= 6; i++ {
				RunInvoke(network, o2, peer, "testchannel")
				Eventually(func() int {
					return RunQuery(network, o2, peer, "testchannel")
				}, network.EventuallyTimeout).Should(Equal(100 - i*10))
			}

			o2SnapDir := path.Join(network.RootDir, "orderers", o2.ID(), "etcdraft", "snapshot")
			Eventually(func() int {
				files, err := ioutil.ReadDir(path.Join(o2SnapDir, "testchannel"))
				Expect(err).NotTo(HaveOccurred())
				return len(files)
			}).Should(Equal(5)) // snapshot interval is 1 KB, every block triggers snapshot

			By("Killing orderers so they don't have blocks prior to latest snapshot in the memory")
			ordererProc.Signal(syscall.SIGKILL)
			Eventually(ordererProc.Wait(), network.EventuallyTimeout).Should(Receive())

			By("Starting lagged orderer and one of up-to-date orderers")
			orderers = grouper.Members{
				{Name: o1.ID(), Runner: network.OrdererRunner(o1)},
				{Name: o2.ID(), Runner: network.OrdererRunner(o2)},
			}
			ordererGroup = grouper.NewParallel(syscall.SIGTERM, orderers)
			ordererProc = ifrit.Invoke(ordererGroup)
			Eventually(ordererProc.Ready()).Should(BeClosed())

			o1SnapDir := path.Join(network.RootDir, "orderers", o1.ID(), "etcdraft", "snapshot")

			By("Asserting that orderer1 has snapshot dir for both system and application channel")
			Eventually(func() int {
				files, err := ioutil.ReadDir(o1SnapDir)
				Expect(err).NotTo(HaveOccurred())
				return len(files)
			}, network.EventuallyTimeout).Should(Equal(2))

			By("Asserting that orderer1 receives and persists snapshot")
			Eventually(func() int {
				files, err := ioutil.ReadDir(path.Join(o1SnapDir, "testchannel"))
				Expect(err).NotTo(HaveOccurred())
				return len(files)
			}, network.EventuallyTimeout).Should(Equal(1))

			By("Asserting cluster is still functional")
			RunInvoke(network, o1, peer, "testchannel")
			Eventually(func() int {
				return RunQuery(network, o1, peer, "testchannel")
			}, network.EventuallyTimeout).Should(Equal(30))

			fetchLatestBlock(o1, blockFile1)
			fetchLatestBlock(o2, blockFile2)
			b1 := nwo.UnmarshalBlockFromFile(blockFile1)
			b2 := nwo.UnmarshalBlockFromFile(blockFile2)
			Expect(b1.Header.Bytes()).To(Equal(b2.Header.Bytes()))
		})
	})

	When("The leader dies", func() {
		It("Elects a new leader", func() {
			network = nwo.New(nwo.MultiNodeEtcdRaft(), testDir, client, 33000, components)

			o1, o2, o3 := network.Orderer("orderer1"), network.Orderer("orderer2"), network.Orderer("orderer3")

			network.GenerateConfigTree()
			network.Bootstrap()

			By("Running the orderer nodes")
			o1Runner := network.OrdererRunner(o1)
			o2Runner := network.OrdererRunner(o2)
			o3Runner := network.OrdererRunner(o3)

			o1Proc = ifrit.Invoke(o1Runner)
			o2Proc = ifrit.Invoke(o2Runner)
			o3Proc = ifrit.Invoke(o3Runner)

			Eventually(o1Proc.Ready()).Should(BeClosed())
			Eventually(o2Proc.Ready()).Should(BeClosed())
			Eventually(o3Proc.Ready()).Should(BeClosed())

			By("Waiting for them to elect a leader")
			ordererProcesses := []ifrit.Process{o1Proc, o2Proc, o3Proc}
			remainingAliveRunners := []*ginkgomon.Runner{o1Runner, o2Runner, o3Runner}
			leader := findLeader(remainingAliveRunners)

			leaderIndex := leader - 1
			By(fmt.Sprintf("Killing the leader (%d)", leader))
			ordererProcesses[leaderIndex].Signal(syscall.SIGTERM)
			By("Waiting for it to die")
			Eventually(ordererProcesses[leaderIndex].Wait(), network.EventuallyTimeout).Should(Receive())

			// Remove the leader from the orderer runners
			remainingAliveRunners = append(remainingAliveRunners[:leaderIndex], remainingAliveRunners[leaderIndex+1:]...)

			By("Waiting for a new leader to be elected")
			leader = findLeader(remainingAliveRunners)
			By(fmt.Sprintf("Orderer %d took over as a leader", leader))
		})
	})

	When("Leader cannot reach quorum", func() {
		It("Steps down", func() {
			network = nwo.New(nwo.MultiNodeEtcdRaft(), testDir, client, 33000, components)

			o1, o2, o3 := network.Orderer("orderer1"), network.Orderer("orderer2"), network.Orderer("orderer3")
			orderers := []*nwo.Orderer{o1, o2, o3}
			peer = network.Peer("Org1", "peer1")
			network.GenerateConfigTree()
			network.Bootstrap()

			By("Running the orderer nodes")
			o1Runner := network.OrdererRunner(o1)
			o2Runner := network.OrdererRunner(o2)
			o3Runner := network.OrdererRunner(o3)
			oRunners := []*ginkgomon.Runner{o1Runner, o2Runner, o3Runner}

			o1Proc = ifrit.Invoke(o1Runner)
			o2Proc = ifrit.Invoke(o2Runner)
			o3Proc = ifrit.Invoke(o3Runner)

			Eventually(o1Proc.Ready()).Should(BeClosed())
			Eventually(o2Proc.Ready()).Should(BeClosed())
			Eventually(o3Proc.Ready()).Should(BeClosed())

			peerGroup := network.PeerGroupRunner()
			peerProc = ifrit.Invoke(peerGroup)
			Eventually(peerProc.Ready()).Should(BeClosed())

			By("Waiting for them to elect a leader")
			ordererProcesses := []ifrit.Process{o1Proc, o2Proc, o3Proc}
			remainingAliveRunners := []*ginkgomon.Runner{o1Runner, o2Runner, o3Runner}
			leaderID := findLeader(remainingAliveRunners)
			leaderIndex := leaderID - 1
			leader := orderers[leaderIndex]

			followerIndices := func() []int {
				f := []int{}
				for i := range ordererProcesses {
					if leaderIndex != i {
						f = append(f, i)
					}
				}

				return f
			}()

			By(fmt.Sprintf("Killing two followers (%d and %d)", followerIndices[0]+1, followerIndices[1]+1))
			ordererProcesses[followerIndices[0]].Signal(syscall.SIGTERM)
			ordererProcesses[followerIndices[1]].Signal(syscall.SIGTERM)

			By("Waiting for followers to die")
			Eventually(ordererProcesses[followerIndices[0]].Wait(), network.EventuallyTimeout).Should(Receive())
			Eventually(ordererProcesses[followerIndices[1]].Wait(), network.EventuallyTimeout).Should(Receive())

			By("Waiting for leader to step down")
			Eventually(oRunners[leaderIndex].Err(), time.Minute, time.Second).Should(gbytes.Say(fmt.Sprintf("%d stepped down to follower since quorum is not active", leaderID)))

			By("Failing to perform operation on leader due to its resignation")
			// This should fail because current leader steps down
			// and there is no leader at this point of time
			sess, err := network.PeerAdminSession(peer, commands.ChannelCreate{
				ChannelID:   "testchannel",
				Orderer:     network.OrdererAddress(leader, nwo.ListenPort),
				File:        network.CreateChannelTxPath("testchannel"),
				OutputBlock: "/dev/null",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(sess.Wait(network.EventuallyTimeout).ExitCode()).To(Equal(1))
		})
	})

	When("orderer TLS certificates expire", func() {
		It("is still possible to recover", func() {
			network = nwo.New(nwo.MultiNodeEtcdRaft(), testDir, client, 33000, components)

			o1, o2, o3 := network.Orderer("orderer1"), network.Orderer("orderer2"), network.Orderer("orderer3")
			peer = network.Peer("Org1", "peer1")

			network.GenerateConfigTree()
			network.Bootstrap()

			ordererDomain := network.Organization(o1.Organization).Domain

			ordererTLSCACertPath := filepath.Join(network.RootDir, "crypto", "ordererOrganizations",
				ordererDomain, "tlsca", fmt.Sprintf("tlsca.%s-cert.pem", ordererDomain))
			ordererTLSCACert, err := ioutil.ReadFile(ordererTLSCACertPath)
			Expect(err).NotTo(HaveOccurred())

			ordererTLSCAKeyPath := filepath.Join(network.RootDir, "crypto", "ordererOrganizations",
				ordererDomain, "tlsca", privateKeyFileName(ordererTLSCACert))

			ordererTLSCAKey, err := ioutil.ReadFile(ordererTLSCAKeyPath)
			Expect(err).NotTo(HaveOccurred())

			serverTLSCerts := make(map[string][]byte)
			for _, orderer := range []*nwo.Orderer{o1, o2, o3} {
				tlsCertPath := filepath.Join(network.OrdererLocalTLSDir(orderer), "server.crt")
				serverTLSCerts[tlsCertPath], err = ioutil.ReadFile(tlsCertPath)
				Expect(err).NotTo(HaveOccurred())
			}

			By("Expiring orderer TLS certificates")
			for filePath, certPEM := range serverTLSCerts {
				expiredCert, earlyMadeCACert := expireCertificate(certPEM, ordererTLSCACert, ordererTLSCAKey)
				err = ioutil.WriteFile(filePath, expiredCert, 600)
				Expect(err).NotTo(HaveOccurred())

				err = ioutil.WriteFile(ordererTLSCACertPath, earlyMadeCACert, 600)
				Expect(err).NotTo(HaveOccurred())
			}

			By("Regenerating config")
			sess, err := network.ConfigTxGen(commands.OutputBlock{
				ChannelID:   network.SystemChannel.Name,
				Profile:     network.SystemChannel.Profile,
				ConfigPath:  network.RootDir,
				OutputBlock: network.OutputBlockPath(network.SystemChannel.Name),
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))

			By("Running the orderer nodes")
			o1Runner := network.OrdererRunner(o1)
			o2Runner := network.OrdererRunner(o2)
			o3Runner := network.OrdererRunner(o3)

			o1Proc = ifrit.Invoke(o1Runner)
			o2Proc = ifrit.Invoke(o2Runner)
			o3Proc = ifrit.Invoke(o3Runner)

			Eventually(o1Proc.Ready()).Should(BeClosed())
			Eventually(o2Proc.Ready()).Should(BeClosed())
			Eventually(o3Proc.Ready()).Should(BeClosed())

			By("Waiting for TLS handshakes to fail")
			Eventually(o1Runner.Err(), time.Minute, time.Second).Should(gbytes.Say("tls: bad certificate"))
			Eventually(o2Runner.Err(), time.Minute, time.Second).Should(gbytes.Say("tls: bad certificate"))
			Eventually(o3Runner.Err(), time.Minute, time.Second).Should(gbytes.Say("tls: bad certificate"))

			By("Killing orderers")
			o1Proc.Signal(syscall.SIGTERM)
			o2Proc.Signal(syscall.SIGTERM)
			o3Proc.Signal(syscall.SIGTERM)

			By("Launching orderers again")
			o1Runner = network.OrdererRunner(o1)
			o2Runner = network.OrdererRunner(o2)
			o3Runner = network.OrdererRunner(o3)

			for i, runner := range []*ginkgomon.Runner{o1Runner, o2Runner, o3Runner} {
				// Switch between the general port and the cluster listener port
				runner.Command.Env = append(runner.Command.Env, "ORDERER_GENERAL_CLUSTER_TLSHANDSHAKETIMESHIFT=90s")
				tlsCertPath := filepath.Join(network.OrdererLocalTLSDir(network.Orderers[i]), "server.crt")
				tlsKeyPath := filepath.Join(network.OrdererLocalTLSDir(network.Orderers[i]), "server.key")
				runner.Command.Env = append(runner.Command.Env, fmt.Sprintf("ORDERER_GENERAL_CLUSTER_SERVERCERTIFICATE=%s", tlsCertPath))
				runner.Command.Env = append(runner.Command.Env, fmt.Sprintf("ORDERER_GENERAL_CLUSTER_SERVERPRIVATEKEY=%s", tlsKeyPath))
				runner.Command.Env = append(runner.Command.Env, fmt.Sprintf("ORDERER_GENERAL_CLUSTER_ROOTCAS=%s", ordererTLSCACertPath))
			}

			o1Proc = ifrit.Invoke(o1Runner)
			o2Proc = ifrit.Invoke(o2Runner)
			o3Proc = ifrit.Invoke(o3Runner)

			Eventually(o1Proc.Ready()).Should(BeClosed())
			Eventually(o2Proc.Ready()).Should(BeClosed())
			Eventually(o3Proc.Ready()).Should(BeClosed())

			By("Waiting for a leader to be elected")
			findLeader([]*ginkgomon.Runner{o1Runner, o2Runner, o3Runner})

		})
	})

	When("admin certificate expires", func() {
		It("is still possible to replace them", func() {
			network = nwo.New(nwo.BasicEtcdRaft(), testDir, client, 33000, components)
			network.GenerateConfigTree()
			network.Bootstrap()

			peer = network.Peer("Org1", "peer1")
			orderer := network.Orderer("orderer")

			ordererDomain := network.Organization(orderer.Organization).Domain

			ordererCACertPath := filepath.Join(network.RootDir, "crypto", "ordererOrganizations",
				ordererDomain, "ca", fmt.Sprintf("ca.%s-cert.pem", ordererDomain))
			ordererCACert, err := ioutil.ReadFile(ordererCACertPath)
			Expect(err).NotTo(HaveOccurred())

			ordererCAKeyPath := filepath.Join(network.RootDir, "crypto", "ordererOrganizations",
				ordererDomain, "ca", privateKeyFileName(ordererCACert))

			ordererCAKey, err := ioutil.ReadFile(ordererCAKeyPath)
			Expect(err).NotTo(HaveOccurred())

			adminCertPath := fmt.Sprintf("Admin@%s-cert.pem", ordererDomain)
			adminCertPath = filepath.Join(network.OrdererUserMSPDir(orderer, "Admin"), "signcerts", adminCertPath)

			originalAdminCert, err := ioutil.ReadFile(adminCertPath)
			Expect(err).NotTo(HaveOccurred())

			expiredAdminCert, earlyCACert := expireCertificate(originalAdminCert, ordererCACert, ordererCAKey)
			err = ioutil.WriteFile(adminCertPath, expiredAdminCert, 600)
			Expect(err).NotTo(HaveOccurred())

			adminPath := filepath.Join(network.RootDir, "crypto", "ordererOrganizations",
				ordererDomain, "msp", "admincerts", fmt.Sprintf("Admin@%s-cert.pem", ordererDomain))
			err = ioutil.WriteFile(adminPath, expiredAdminCert, 600)
			Expect(err).NotTo(HaveOccurred())

			err = ioutil.WriteFile(ordererCACertPath, earlyCACert, 600)
			Expect(err).NotTo(HaveOccurred())

			ordererCACertPath = filepath.Join(network.RootDir, "crypto", "ordererOrganizations",
				ordererDomain, "msp", "cacerts", fmt.Sprintf("ca.%s-cert.pem", ordererDomain))
			err = ioutil.WriteFile(ordererCACertPath, earlyCACert, 600)
			Expect(err).NotTo(HaveOccurred())

			By("Regenerating config")
			sess, err := network.ConfigTxGen(commands.OutputBlock{
				ChannelID:   network.SystemChannel.Name,
				Profile:     network.SystemChannel.Profile,
				ConfigPath:  network.RootDir,
				OutputBlock: network.OutputBlockPath(network.SystemChannel.Name),
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))

			runner := network.OrdererRunner(orderer)
			runner.Command.Env = append(runner.Command.Env, "FABRIC_LOGGING_SPEC=debug")
			ordererProc = ifrit.Invoke(runner)

			By("Waiting for orderer to elect a leader")
			findLeader([]*ginkgomon.Runner{runner})

			By("Creating config update that adds another orderer admin")
			bootBlockPath := filepath.Join(network.RootDir, fmt.Sprintf("%s_block.pb", network.SystemChannel.Name))
			bootBlock, err := ioutil.ReadFile(bootBlockPath)
			Expect(err).NotTo(HaveOccurred())

			current := configFromBootstrapBlock(bootBlock)
			updatedConfig := addAdminCertToConfig(current, originalAdminCert)

			tempDir, err := ioutil.TempDir("", "adminExpirationTest")
			Expect(err).NotTo(HaveOccurred())

			configBlockFile := filepath.Join(tempDir, "update.pb")
			defer os.RemoveAll(tempDir)
			nwo.ComputeUpdateOrdererConfig(configBlockFile, network, network.SystemChannel.Name, current, updatedConfig, peer)

			updateTransaction, err := ioutil.ReadFile(configBlockFile)
			Expect(err).NotTo(HaveOccurred())

			By("Creating config update")
			channelCreateTxn := createConfigTx(updateTransaction, network.SystemChannel.Name, network, orderer, peer)

			By("Updating channel config and failing")
			p, err := Broadcast(network, orderer, channelCreateTxn)
			Expect(err).NotTo(HaveOccurred())
			Expect(p.Status).To(Equal(common.Status_BAD_REQUEST))
			Expect(p.Info).To(ContainSubstring("identity expired"))

			By("Attempting to fetch a block from orderer and failing")
			denv := CreateDeliverEnvelope(network, orderer, 0, network.SystemChannel.Name)
			Expect(denv).NotTo(BeNil())

			block, err := Deliver(network, orderer, denv)
			Expect(denv).NotTo(BeNil())
			Expect(block).To(BeNil())
			Eventually(runner.Err(), time.Minute, time.Second).Should(gbytes.Say("client identity expired"))

			By("Killing orderer")
			ordererProc.Signal(syscall.SIGTERM)

			By("Launching orderers again")
			runner = network.OrdererRunner(orderer)
			runner.Command.Env = append(runner.Command.Env, "ORDERER_GENERAL_AUTHENTICATION_NOEXPIRATIONCHECKS=true")
			ordererProc = ifrit.Invoke(runner)

			By("Waiting for orderer to launch again")
			findLeader([]*ginkgomon.Runner{runner})

			By("Updating channel config and succeeding")
			p, err = Broadcast(network, orderer, channelCreateTxn)
			Expect(err).NotTo(HaveOccurred())
			Expect(p.Status).To(Equal(common.Status_SUCCESS))

			By("Fetching a block from the orderer and succeeding")
			block = FetchBlock(network, orderer, 1, network.SystemChannel.Name)
			Expect(block).NotTo(BeNil())

			By("Restore the original admin cert")
			err = ioutil.WriteFile(adminCertPath, originalAdminCert, 600)
			Expect(err).NotTo(HaveOccurred())

			By("Ensure we can fetch the block using our original un-expired admin cert")
			ccb := func() uint64 {
				return nwo.GetConfigBlock(network, peer, orderer, network.SystemChannel.Name).Header.Number
			}
			Eventually(ccb, network.EventuallyTimeout).Should(Equal(uint64(1)))
		})
	})
})

func RunInvoke(n *nwo.Network, orderer *nwo.Orderer, peer *nwo.Peer, channel string) {
	sess, err := n.PeerUserSession(peer, "User1", commands.ChaincodeInvoke{
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
}

func RunQuery(n *nwo.Network, orderer *nwo.Orderer, peer *nwo.Peer, channel string) int {
	sess, err := n.PeerUserSession(peer, "User1", commands.ChaincodeQuery{
		ChannelID: channel,
		Name:      "mycc",
		Ctor:      `{"Args":["query","a"]}`,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))

	var result int
	i, err := fmt.Sscanf(string(sess.Out.Contents()), "%d", &result)
	Expect(err).NotTo(HaveOccurred())
	Expect(i).To(Equal(1))
	return int(result)
}

func findLeader(ordererRunners []*ginkgomon.Runner) int {
	var wg sync.WaitGroup
	wg.Add(len(ordererRunners))

	findLeader := func(runner *ginkgomon.Runner) int {
		Eventually(runner.Err(), time.Minute, time.Second).Should(gbytes.Say("Raft leader changed: [0-9] -> "))

		idBuff := make([]byte, 1)
		runner.Err().Read(idBuff)

		newLeader, err := strconv.ParseInt(string(idBuff), 10, 32)
		Expect(err).To(BeNil())
		return int(newLeader)
	}

	leaders := make(chan int, len(ordererRunners))

	for _, runner := range ordererRunners {
		go func(runner *ginkgomon.Runner) {
			defer GinkgoRecover()
			defer wg.Done()

			for {
				leader := findLeader(runner)
				if leader != 0 {
					leaders <- leader
					break
				}
			}
		}(runner)
	}

	wg.Wait()

	close(leaders)
	firstLeader := <-leaders
	for leader := range leaders {
		if firstLeader != leader {
			Fail(fmt.Sprintf("First leader is %d but saw %d also as a leader", firstLeader, leader))
		}
	}

	return firstLeader
}

func expireCertificate(certPEM, caCertPEM, caKeyPEM []byte) (expiredcertPEM []byte, earlyMadeCACertPEM []byte) {
	keyAsDER, _ := pem.Decode(caKeyPEM)
	caKeyWithoutType, err := x509.ParsePKCS8PrivateKey(keyAsDER.Bytes)
	Expect(err).NotTo(HaveOccurred())
	caKey := caKeyWithoutType.(*ecdsa.PrivateKey)

	caCertAsDER, _ := pem.Decode(caCertPEM)
	caCert, err := x509.ParseCertificate(caCertAsDER.Bytes)
	Expect(err).NotTo(HaveOccurred())

	certAsDER, _ := pem.Decode(certPEM)
	cert, err := x509.ParseCertificate(certAsDER.Bytes)
	Expect(err).NotTo(HaveOccurred())

	cert.Raw = nil
	caCert.Raw = nil
	// The certificate was made 1 hour ago
	cert.NotBefore = time.Now().Add((-1) * time.Hour)
	// As well as the CA certificate
	caCert.NotBefore = time.Now().Add((-1) * time.Hour)
	// The certificate expires now
	cert.NotAfter = time.Now()

	// The CA signs the certificate
	certBytes, err := x509.CreateCertificate(rand.Reader, cert, caCert, cert.PublicKey, caKey)
	Expect(err).NotTo(HaveOccurred())

	// The CA signs its own certificate
	caCertBytes, err := x509.CreateCertificate(rand.Reader, caCert, caCert, caCert.PublicKey, caKey)
	Expect(err).NotTo(HaveOccurred())

	expiredcertPEM = pem.EncodeToMemory(&pem.Block{Bytes: certBytes, Type: "CERTIFICATE"})
	earlyMadeCACertPEM = pem.EncodeToMemory(&pem.Block{Bytes: caCertBytes, Type: "CERTIFICATE"})
	return
}

func privateKeyFileName(certAsPEM []byte) string {
	certAsDER, _ := pem.Decode(certAsPEM)
	cert, err := x509.ParseCertificate(certAsDER.Bytes)
	Expect(err).NotTo(HaveOccurred())

	ecPubKey := cert.PublicKey.(*ecdsa.PublicKey)
	keyAsBytes := elliptic.Marshal(ecPubKey.Curve, ecPubKey.X, ecPubKey.Y)
	hash := sha256.New()
	hash.Write(keyAsBytes)
	ski := hash.Sum(nil)
	return fmt.Sprintf("%s_sk", hex.EncodeToString(ski))
}

func createConfigTx(txData []byte, channelName string, network *nwo.Network, orderer *nwo.Orderer, peer *nwo.Peer) *common.Envelope {
	ctxEnv, err := utils.UnmarshalEnvelope(txData)
	Expect(err).NotTo(HaveOccurred())

	payload, err := utils.ExtractPayload(ctxEnv)
	Expect(err).NotTo(HaveOccurred())

	configUpdateEnv, err := configtx.UnmarshalConfigUpdateEnvelope(payload.Data)
	Expect(err).NotTo(HaveOccurred())

	conf := signer.Config{
		MSPID:        network.Organization(orderer.Organization).MSPID,
		IdentityPath: network.OrdererUserCert(orderer, "Admin"),
		KeyPath:      network.OrdererUserKey(orderer, "Admin"),
	}

	s, err := signer.NewSigner(conf)
	Expect(err).NotTo(HaveOccurred())

	signConfigUpdate(conf, configUpdateEnv)

	env, err := utils.CreateSignedEnvelope(common.HeaderType_CONFIG_UPDATE, channelName, s, configUpdateEnv, 0, 0)
	Expect(err).NotTo(HaveOccurred())

	return env
}

func signConfigUpdate(conf signer.Config, configUpdateEnv *common.ConfigUpdateEnvelope) *common.ConfigUpdateEnvelope {
	s, err := signer.NewSigner(conf)
	Expect(err).NotTo(HaveOccurred())

	sigHeader := utils.NewSignatureHeaderOrPanic(s)
	Expect(err).NotTo(HaveOccurred())

	configSig := &common.ConfigSignature{
		SignatureHeader: utils.MarshalOrPanic(sigHeader),
	}

	configSig.Signature, err = s.Sign(util.ConcatenateBytes(configSig.SignatureHeader, configUpdateEnv.ConfigUpdate))
	Expect(err).NotTo(HaveOccurred())

	configUpdateEnv.Signatures = append(configUpdateEnv.Signatures, configSig)
	return configUpdateEnv
}

func addAdminCertToConfig(originalConfig *common.Config, additionalAdmin []byte) *common.Config {
	updatedConfig := proto.Clone(originalConfig).(*common.Config)

	rawMSPConfig := updatedConfig.ChannelGroup.Groups["Orderer"].Groups["OrdererOrg"].Values["MSP"]
	mspConfig := &msp.MSPConfig{}
	err := proto.Unmarshal(rawMSPConfig.Value, mspConfig)
	Expect(err).NotTo(HaveOccurred())

	fabricConfig := &msp.FabricMSPConfig{}
	err = proto.Unmarshal(mspConfig.Config, fabricConfig)
	Expect(err).NotTo(HaveOccurred())

	fabricConfig.Admins = append(fabricConfig.Admins, additionalAdmin)
	mspConfig.Config = utils.MarshalOrPanic(fabricConfig)

	rawMSPConfig.Value = utils.MarshalOrPanic(mspConfig)
	return updatedConfig
}

func configFromBootstrapBlock(bootstrapBlock []byte) *common.Config {
	block := &common.Block{}
	err := proto.Unmarshal(bootstrapBlock, block)
	Expect(err).NotTo(HaveOccurred())

	envelope, err := utils.GetEnvelopeFromBlock(block.Data.Data[0])
	Expect(err).NotTo(HaveOccurred())

	payload, err := utils.GetPayload(envelope)
	Expect(err).NotTo(HaveOccurred())

	configEnv := &common.ConfigEnvelope{}
	err = proto.Unmarshal(payload.Data, configEnv)
	Expect(err).NotTo(HaveOccurred())

	return configEnv.Config

}
