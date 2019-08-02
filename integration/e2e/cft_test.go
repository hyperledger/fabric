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
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
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
				port := network.OrdererPort(network.Orderers[i], nwo.ListenPort)
				runner.Command.Env = append(runner.Command.Env, fmt.Sprintf("ORDERER_GENERAL_CLUSTER_LISTENPORT=%d", port))
				runner.Command.Env = append(runner.Command.Env, fmt.Sprintf("ORDERER_GENERAL_LISTENPORT=%d", network.ReservePort()))
				runner.Command.Env = append(runner.Command.Env, "ORDERER_GENERAL_CLUSTER_TLSHANDSHAKETIMESHIFT=90s")
				runner.Command.Env = append(runner.Command.Env, "ORDERER_GENERAL_CLUSTER_LISTENADDRESS=127.0.0.1")
				tlsCertPath := filepath.Join(network.OrdererLocalTLSDir(network.Orderers[i]), "server.crt")
				tlsKeyPath := filepath.Join(network.OrdererLocalTLSDir(network.Orderers[i]), "server.key")
				runner.Command.Env = append(runner.Command.Env, fmt.Sprintf("ORDERER_GENERAL_CLUSTER_SERVERCERTIFICATE=%s", tlsCertPath))
				runner.Command.Env = append(runner.Command.Env, fmt.Sprintf("ORDERER_GENERAL_CLUSTER_SERVERPRIVATEKEY=%s", tlsKeyPath))
				runner.Command.Env = append(runner.Command.Env, fmt.Sprintf("ORDERER_GENERAL_CLUSTER_ROOTCAS=%s", ordererTLSCACertPath))
				runner.Command.Env = append(runner.Command.Env, "ORDERER_METRICS_PROVIDER=disabled")
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
