/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package e2e

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/orderer/etcdraft"
	"github.com/hyperledger/fabric/protoutil"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/ginkgomon"
	"github.com/tedsuo/ifrit/grouper"
)

var _ = Describe("EndToEnd reconfiguration and onboarding", func() {
	var (
		testDir string
		client  *docker.Client
		network *nwo.Network
		mycc    nwo.Chaincode
		mycc2   nwo.Chaincode
		mycc3   nwo.Chaincode
		peer    *nwo.Peer

		peerProcesses    ifrit.Process
		ordererProcesses []ifrit.Process
		ordererRunners   []*ginkgomon.Runner
	)

	BeforeEach(func() {
		ordererRunners = nil
		ordererProcesses = nil
		peerProcesses = nil

		var err error
		testDir, err = ioutil.TempDir("", "e2e-etcfraft_reconfig")
		Expect(err).NotTo(HaveOccurred())

		client, err = docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())

		mycc = nwo.Chaincode{
			Name:    "mycc",
			Version: "0.0",
			Path:    "github.com/hyperledger/fabric/integration/chaincode/simple/cmd",
			Ctor:    `{"Args":["init","a","100","b","200"]}`,
			Policy:  `OR ('Org1MSP.member','Org2MSP.member')`,
		}
		mycc2 = nwo.Chaincode{
			Name:    "mycc2",
			Version: "0.0",
			Path:    "github.com/hyperledger/fabric/integration/chaincode/simple/cmd",
			Ctor:    `{"Args":["init","a","100","b","200"]}`,
			Policy:  `OR ('Org1MSP.member','Org2MSP.member')`,
		}
		mycc3 = nwo.Chaincode{
			Name:    "mycc3",
			Version: "0.0",
			Path:    "github.com/hyperledger/fabric/integration/chaincode/simple/cmd",
			Ctor:    `{"Args":["init","a","100","b","200"]}`,
			Policy:  `OR ('Org1MSP.member','Org2MSP.member')`,
		}
	})

	AfterEach(func() {
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

	When("a single node cluster is expanded", func() {
		It("is still possible to onboard the new cluster member", func() {
			launch := func(o *nwo.Orderer) {
				runner := network.OrdererRunner(o)
				ordererRunners = append(ordererRunners, runner)

				process := ifrit.Invoke(grouper.Member{Name: o.ID(), Runner: runner})
				Eventually(process.Ready()).Should(BeClosed())
				ordererProcesses = append(ordererProcesses, process)
			}

			layout := nwo.BasicEtcdRaft()
			network = nwo.New(layout, testDir, client, BasePort(), components)
			orderer := network.Orderer("orderer")

			peer = network.Peer("Org1", "peer1")

			network.GenerateConfigTree()
			network.Bootstrap()

			By("Launching the orderer")
			launch(orderer)

			By("Checking that it elected itself as a leader")
			findLeader(ordererRunners)

			By("Extending the network configuration to add a new orderer")
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
			secondOrdererCertificate, err := ioutil.ReadFile(secondOrdererCertificatePath)
			Expect(err).NotTo(HaveOccurred())

			By("Adding the second orderer")
			nwo.AddConsenter(network, peer, orderer, "systemchannel", etcdraft.Consenter{
				ServerTlsCert: secondOrdererCertificate,
				ClientTlsCert: secondOrdererCertificate,
				Host:          "127.0.0.1",
				Port:          uint32(network.OrdererPort(orderer2, nwo.ListenPort)),
			})

			By("Obtaining the last config block from the orderer")
			// Get the last config block of the system channel
			configBlock := nwo.GetConfigBlock(network, peer, orderer, "systemchannel")
			// Plant it in the file system of orderer2, the new node to be onboarded.
			err = ioutil.WriteFile(filepath.Join(testDir, "systemchannel_block.pb"), protoutil.MarshalOrPanic(configBlock), 0644)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for the existing orderer to relinquish its leadership")
			Eventually(ordererRunners[0].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("1 stepped down to follower since quorum is not active"))
			Eventually(ordererRunners[0].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("No leader is present, cluster size is 2"))
			By("Launching the second orderer")
			launch(orderer2)
			By("Waiting for a leader to be re-elected")
			findLeader(ordererRunners)
		})
	})

	When("the orderer certificates are all rotated", func() {
		It("is still possible to onboard new orderers", func() {
			// In this test, we have 3 OSNs and we rotate their TLS certificates one by one,
			// by adding the future certificate to the channel, killing the OSN to make it
			// grab the new certificate, and then removing the old certificate from the channel.

			// After we completely rotate all the certificates, we put the last config block
			// of the system channel into the file system of orderer4, and then launch it,
			// and ensure it onboards and pulls channels testchannel only, and not testchannel2
			// which it is not part of.

			// Consenter i after its certificate is rotated is denoted as consenter i'
			// The blocks of channels contain the following updates:
			//    | system channel height | testchannel  height  | update description
			// ------------------------------------------------------------------------
			// 0  |            2          |         1            | adding consenter 1'
			// 1  |            3          |         2            | removing consenter 1
			// 2  |            4          |         3            | adding consenter 2'
			// 3  |            5          |         4            | removing consenter 2
			// 4  |            6          |         5            | adding consenter 3'
			// 5  |            7          |         6            | removing consenter 3
			// 6  |            8          |         6            | creating channel testchannel2
			// 7  |            9          |         6            | creating channel testchannel3
			// 8  |            10         |         7            | adding consenter 4
			// 9  |            10         |         8            | deploying chaincode on testchannel
			// 10 |            10         |         9            | invoking chaincode on testchannel

			layout := nwo.MultiNodeEtcdRaft()
			layout.Channels = append(layout.Channels, &nwo.Channel{
				Name:    "testchannel2",
				Profile: "TwoOrgsChannel",
			}, &nwo.Channel{
				Name:    "testchannel3",
				Profile: "TwoOrgsChannel",
			})
			network = nwo.New(layout, testDir, client, BasePort(), components)
			o1, o2, o3 := network.Orderer("orderer1"), network.Orderer("orderer2"), network.Orderer("orderer3")
			orderers := []*nwo.Orderer{o1, o2, o3}

			peer = network.Peer("Org1", "peer1")

			network.GenerateConfigTree()
			network.Bootstrap()

			By("Launching the orderers")
			for _, o := range orderers {
				runner := network.OrdererRunner(o)
				ordererRunners = append(ordererRunners, runner)
				process := ifrit.Invoke(runner)
				ordererProcesses = append(ordererProcesses, process)
			}

			for _, ordererProc := range ordererProcesses {
				Eventually(ordererProc.Ready()).Should(BeClosed())
			}

			By("Launching the peers")
			peerGroup := network.PeerGroupRunner()
			peerProcesses = ifrit.Invoke(peerGroup)
			Eventually(peerProcesses.Ready()).Should(BeClosed())

			By("Checking that all orderers are online")
			assertBlockReception(map[string]int{
				"systemchannel": 0,
			}, orderers, peer, network)

			By("Creating a channel and checking that all orderers got the channel creation")
			network.CreateChannel("testchannel", network.Orderers[0], peer)
			assertBlockReception(map[string]int{
				"systemchannel": 1,
				"testchannel":   0,
			}, orderers, peer, network)

			By("Preparing new certificates for the orderer nodes")
			extendNetwork(network)
			certificateRotations := refreshOrdererPEMs(network)

			expectedBlockHeightsPerChannel := []map[string]int{
				{"systemchannel": 2, "testchannel": 1},
				{"systemchannel": 3, "testchannel": 2},
				{"systemchannel": 4, "testchannel": 3},
				{"systemchannel": 5, "testchannel": 4},
				{"systemchannel": 6, "testchannel": 5},
				{"systemchannel": 7, "testchannel": 6},
			}

			for i, rotation := range certificateRotations {
				o := network.Orderers[i]
				port := network.OrdererPort(o, nwo.ListenPort)

				By(fmt.Sprintf("Adding the future certificate of orderer node %d", i))
				for _, channelName := range []string{"systemchannel", "testchannel"} {
					nwo.AddConsenter(network, peer, o, channelName, etcdraft.Consenter{
						ServerTlsCert: rotation.newCert,
						ClientTlsCert: rotation.newCert,
						Host:          "127.0.0.1",
						Port:          uint32(port),
					})
				}

				By("Waiting for all orderers to sync")
				assertBlockReception(expectedBlockHeightsPerChannel[i*2], orderers, peer, network)

				By("Killing the orderer")
				ordererProcesses[i].Signal(syscall.SIGTERM)
				Eventually(ordererProcesses[i].Wait(), network.EventuallyTimeout).Should(Receive())

				By("Starting the orderer again")
				ordererRunner := network.OrdererRunner(orderers[i])
				ordererRunners = append(ordererRunners, ordererRunner)
				ordererProcesses[i] = ifrit.Invoke(grouper.Member{Name: orderers[i].ID(), Runner: ordererRunner})
				Eventually(ordererProcesses[i].Ready()).Should(BeClosed())

				By("And waiting for it to stabilize")
				assertBlockReception(expectedBlockHeightsPerChannel[i*2], orderers, peer, network)

				By("Removing the previous certificate of the old orderer")
				for _, channelName := range []string{"systemchannel", "testchannel"} {
					nwo.RemoveConsenter(network, peer, network.Orderers[(i+1)%len(network.Orderers)], channelName, rotation.oldCert)
				}

				By("Waiting for all orderers to sync")
				assertBlockReception(expectedBlockHeightsPerChannel[i*2+1], orderers, peer, network)
			}

			By("Creating testchannel2")
			network.CreateChannel("testchannel2", network.Orderers[0], peer)
			assertBlockReception(map[string]int{
				"systemchannel": 8,
			}, orderers, peer, network)

			By("Creating testchannel3")
			network.CreateChannel("testchannel3", network.Orderers[0], peer)
			assertBlockReception(map[string]int{
				"systemchannel": 9,
			}, orderers, peer, network)

			o4 := &nwo.Orderer{
				Name:         "orderer4",
				Organization: "OrdererOrg",
			}

			By("Configuring orderer4 in the network")
			ports := nwo.Ports{}
			for _, portName := range nwo.OrdererPortNames() {
				ports[portName] = network.ReservePort()
			}
			network.PortsByOrdererID[o4.ID()] = ports

			network.Orderers = append(network.Orderers, o4)
			network.GenerateOrdererConfig(network.Orderer("orderer4"))

			By("Adding orderer4 to the channels")
			orderer4CertificatePath := filepath.Join(testDir, "crypto", "ordererOrganizations", "example.com",
				"orderers", "orderer4.example.com", "tls", "server.crt")
			orderer4Certificate, err := ioutil.ReadFile(orderer4CertificatePath)
			Expect(err).NotTo(HaveOccurred())
			for _, channel := range []string{"systemchannel", "testchannel"} {
				nwo.AddConsenter(network, peer, o1, channel, etcdraft.Consenter{
					ServerTlsCert: orderer4Certificate,
					ClientTlsCert: orderer4Certificate,
					Host:          "127.0.0.1",
					Port:          uint32(network.OrdererPort(o4, nwo.ListenPort)),
				})
			}

			By("Ensuring all orderers know about orderer4's addition")
			assertBlockReception(map[string]int{
				"systemchannel": 10,
				"testchannel":   7,
			}, orderers, peer, network)

			By("Joining the peer to testchannel")
			network.JoinChannel("testchannel", o1, peer)
			By("Joining the peer to testchannel2")
			network.JoinChannel("testchannel2", o1, peer)
			By("Joining the peer to testchannel3")
			network.JoinChannel("testchannel3", o1, peer)

			By("Deploying mycc and mycc2 and mycc3 to testchannel and testchannel2 and testchannel3")
			deployChaincodes(network, peer, o2, mycc, mycc2, mycc3)

			By("Waiting for orderers to sync")
			assertBlockReception(map[string]int{
				"testchannel": 8,
			}, orderers, peer, network)

			By("Transacting on testchannel once more")
			assertInvoke(network, peer, o1, mycc.Name, "testchannel", "Chaincode invoke successful. result: status:200", 0)

			assertBlockReception(map[string]int{
				"testchannel": 9,
			}, orderers, peer, network)

			By("Corrupting the readers policy of testchannel3")
			revokeReaderAccess(network, "testchannel3", o3, peer)

			// Get the last config block of the system channel
			configBlock := nwo.GetConfigBlock(network, peer, o1, "systemchannel")
			// Plant it in the file system of orderer4, the new node to be onboarded.
			err = ioutil.WriteFile(filepath.Join(testDir, "systemchannel_block.pb"), protoutil.MarshalOrPanic(configBlock), 0644)
			Expect(err).NotTo(HaveOccurred())

			By("Launching orderer4")
			orderers = append(orderers, o4)
			orderer4Runner := network.OrdererRunner(o4)
			ordererRunners = append(ordererRunners, orderer4Runner)
			// Spawn orderer4's process
			o4process := ifrit.Invoke(grouper.Member{Name: o4.ID(), Runner: orderer4Runner})
			Eventually(o4process.Ready()).Should(BeClosed())
			ordererProcesses = append(ordererProcesses, o4process)

			By("And waiting for it to sync with the rest of the orderers")
			assertBlockReception(map[string]int{
				"systemchannel": 10,
				"testchannel":   9,
			}, orderers, peer, network)

			By("Ensuring orderer4 doesn't serve testchannel2 and testchannel3")
			assertInvoke(network, peer, o4, mycc2.Name, "testchannel2", "channel testchannel2 is not serviced by me", 1)
			assertInvoke(network, peer, o4, mycc3.Name, "testchannel3", "channel testchannel3 is not serviced by me", 1)
			Expect(string(orderer4Runner.Err().Contents())).To(ContainSubstring("I do not belong to channel testchannel2 or am forbidden pulling it (not in the channel), skipping chain retrieval"))
			Expect(string(orderer4Runner.Err().Contents())).To(ContainSubstring("I do not belong to channel testchannel3 or am forbidden pulling it (forbidden pulling the channel), skipping chain retrieval"))

			By("Adding orderer4 to testchannel2")
			nwo.AddConsenter(network, peer, o1, "testchannel2", etcdraft.Consenter{
				ServerTlsCert: orderer4Certificate,
				ClientTlsCert: orderer4Certificate,
				Host:          "127.0.0.1",
				Port:          uint32(network.OrdererPort(o4, nwo.ListenPort)),
			})

			By("Waiting for orderer4 and to replicate testchannel2")
			assertBlockReception(map[string]int{
				"testchannel2": 2,
			}, []*nwo.Orderer{o4}, peer, network)

			By("Ensuring orderer4 doesn't have any errors in the logs")
			Expect(orderer4Runner.Err()).ToNot(gbytes.Say("ERRO"))

			By("Ensuring that all orderers don't log errors to the log")
			assertNoErrorsAreLogged(ordererRunners)

			By("Submitting a transaction through orderer4")
			assertInvoke(network, peer, o4, mycc2.Name, "testchannel2", "Chaincode invoke successful. result: status:200", 0)

			By("And ensuring it is propagated amongst all orderers")
			assertBlockReception(map[string]int{
				"testchannel2": 3,
			}, orderers, peer, network)
		})
	})

	When("an orderer node is evicted", func() {
		BeforeEach(func() {
			ordererRunners = nil
			ordererProcesses = nil

			network = nwo.New(nwo.MultiNodeEtcdRaft(), testDir, client, BasePort(), components)

			peer = network.Peer("Org1", "peer1")

			network.GenerateConfigTree()
			network.Bootstrap()

			o1, o2, o3 := network.Orderer("orderer1"), network.Orderer("orderer2"), network.Orderer("orderer3")
			orderers := []*nwo.Orderer{o1, o2, o3}
			By("Launching the orderers")
			for _, o := range orderers {
				runner := network.OrdererRunner(o)
				ordererRunners = append(ordererRunners, runner)
				process := ifrit.Invoke(runner)
				ordererProcesses = append(ordererProcesses, process)
			}

			for _, ordererProc := range ordererProcesses {
				Eventually(ordererProc.Ready()).Should(BeClosed())
			}
		})

		AfterEach(func() {
			for _, ordererInstance := range ordererProcesses {
				ordererInstance.Signal(syscall.SIGTERM)
				Eventually(ordererInstance.Wait(), network.EventuallyTimeout).Should(Receive())
			}
		})

		It("doesn't complain and does it obediently", func() {
			o1, o2, o3 := network.Orderer("orderer1"), network.Orderer("orderer2"), network.Orderer("orderer3")
			orderers := []*nwo.Orderer{o1, o2, o3}

			By("Waiting for them to elect a leader")
			evictedNode := findLeader(ordererRunners) - 1

			By("Removing the leader from system channel")
			serverCertBytes, err := ioutil.ReadFile(filepath.Join(network.OrdererLocalTLSDir(network.Orderers[evictedNode]), "server.crt"))
			Expect(err).To(Not(HaveOccurred()))

			By("Removing the leader from both system channel and application channel")
			nwo.RemoveConsenter(network, peer, network.Orderers[(evictedNode+1)%3], "systemchannel", serverCertBytes)
			fmt.Fprintln(GinkgoWriter, "Ensuring the other orderers detect the eviction of the node on channel", "systemchannel")
			Eventually(ordererRunners[(evictedNode+1)%3].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Deactivated node"))
			Eventually(ordererRunners[(evictedNode+2)%3].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Deactivated node"))

			fmt.Fprintln(GinkgoWriter, "Ensuring the evicted orderer stops rafting on channel", "systemchannel")
			stopMSg := fmt.Sprintf("Raft node stopped channel=%s", "systemchannel")
			Eventually(ordererRunners[evictedNode].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say(stopMSg))

			By("Ensuring the evicted orderer now doesn't serve clients")
			ensureEvicted(orderers[evictedNode], peer, network, "systemchannel")

			By("Ensuring that all orderers don't log errors to the log")
			assertNoErrorsAreLogged(ordererRunners)
		})

		It("notices it even if it is down at the time of its eviction", func() {
			o1 := network.Orderer("orderer1")
			o2 := network.Orderer("orderer2")
			o3 := network.Orderer("orderer3")

			orderers := []*nwo.Orderer{o1, o2, o3}

			By("Waiting for them to elect a leader")
			findLeader(ordererRunners)

			By("Creating a channel")
			network.CreateChannel("testchannel", o1, peer)

			assertBlockReception(map[string]int{
				"testchannel":   0,
				"systemchannel": 1,
			}, []*nwo.Orderer{o1, o2, o3}, peer, network)

			By("Killing the orderer")
			ordererProcesses[0].Signal(syscall.SIGTERM)
			Eventually(ordererProcesses[0].Wait(), network.EventuallyTimeout).Should(Receive())

			// We need to wait for stabilization, as we might have killed the leader OSN.
			By("Waiting for the channel to stabilize after killing the orderer")
			assertBlockReception(map[string]int{
				"testchannel":   0,
				"systemchannel": 1,
			}, []*nwo.Orderer{o2, o3}, peer, network)

			By("Removing the first orderer from an application channel")
			extendNetwork(network)
			certificatesOfOrderers := refreshOrdererPEMs(network)
			nwo.RemoveConsenter(network, peer, o2, "testchannel", certificatesOfOrderers[0].oldCert)

			certPath := certificatesOfOrderers[0].dstFile
			keyFile := strings.Replace(certPath, "server.crt", "server.key", -1)
			err := ioutil.WriteFile(certPath, certificatesOfOrderers[0].oldCert, 0644)
			Expect(err).To(Not(HaveOccurred()))
			err = ioutil.WriteFile(keyFile, certificatesOfOrderers[0].oldKey, 0644)
			Expect(err).To(Not(HaveOccurred()))

			By("Starting the orderer again")
			ordererRunner := network.OrdererRunner(orderers[0])
			ordererRunners[0] = ordererRunner
			ordererProcesses[0] = ifrit.Invoke(grouper.Member{Name: orderers[0].ID(), Runner: ordererRunner})
			Eventually(ordererProcesses[0].Ready()).Should(BeClosed())

			By("Ensuring the remaining OSNs reject authentication")
			Eventually(ordererRunners[1].Err(), time.Minute, time.Second).Should(gbytes.Say("certificate extracted from TLS connection isn't authorized"))
			Eventually(ordererRunners[2].Err(), time.Minute, time.Second).Should(gbytes.Say("certificate extracted from TLS connection isn't authorized"))

			By("Ensuring it detects its eviction")
			evictionDetection := gbytes.Say("Detected our own eviction from the chain in block 1 channel=testchannel")
			Eventually(ordererRunner.Err(), time.Minute, time.Second).Should(evictionDetection)

			By("Ensuring all blocks are pulled up to the block that evicts the OSN")
			Eventually(ordererRunner.Err(), time.Minute, time.Second).Should(gbytes.Say("Pulled all blocks up to eviction block. channel=testchannel"))

			By("Killing the evicted orderer")
			ordererProcesses[0].Signal(syscall.SIGTERM)
			Eventually(ordererProcesses[0].Wait(), network.EventuallyTimeout).Should(Receive())

			By("Starting the evicted orderer again")
			ordererRunner = network.OrdererRunner(orderers[0])
			ordererRunners[0] = ordererRunner
			ordererProcesses[0] = ifrit.Invoke(grouper.Member{Name: orderers[0].ID(), Runner: ordererRunner})
			Eventually(ordererProcesses[0].Ready()).Should(BeClosed())

			By("Ensuring the evicted orderer starts up marked the channel is inactive")
			Eventually(ordererRunner.Err(), time.Minute, time.Second).Should(gbytes.Say("Found 1 inactive chains"))

			iDoNotBelong := "I do not belong to channel testchannel or am forbidden pulling it"
			Eventually(ordererRunner.Err(), time.Minute, time.Second).Should(gbytes.Say(iDoNotBelong))

			By("Adding the evicted orderer back to the application channel")
			nwo.AddConsenter(network, peer, o2, "testchannel", etcdraft.Consenter{
				ServerTlsCert: certificatesOfOrderers[0].oldCert,
				ClientTlsCert: certificatesOfOrderers[0].oldCert,
				Host:          "127.0.0.1",
				Port:          uint32(network.OrdererPort(orderers[0], nwo.ListenPort)),
			})

			By("Ensuring the re-added orderer joins the Raft cluster")
			findLeader([]*ginkgomon.Runner{ordererRunner})
		})
	})

	When("an orderer node is joined", func() {
		It("isn't influenced by outdated orderers", func() {
			o4 := &nwo.Orderer{
				Name:         "orderer4",
				Organization: "OrdererOrg",
			}
			layout := nwo.MultiNodeEtcdRaft()
			layout.Orderers = append(layout.Orderers, o4)
			layout.Profiles[0].Orderers = append(layout.Profiles[0].Orderers, o4.Name)

			network = nwo.New(layout, testDir, client, BasePort(), components)
			orderers := []*nwo.Orderer{layout.Orderers[0], layout.Orderers[1], layout.Orderers[2]}
			network.GenerateConfigTree()
			network.Bootstrap()

			peer = network.Peer("Org1", "peer1")

			By("Launching 3 out of 4 orderers")
			for _, o := range orderers {
				runner := network.OrdererRunner(o)
				ordererRunners = append(ordererRunners, runner)
				process := ifrit.Invoke(runner)
				ordererProcesses = append(ordererProcesses, process)
			}

			By("Checking that all orderers are online")
			assertBlockReception(map[string]int{
				"systemchannel": 0,
			}, orderers, peer, network)

			By("Configuring the fifth orderer in the network")
			o5 := &nwo.Orderer{
				Name:         "orderer1new",
				Organization: "OrdererOrg",
			}
			ports := nwo.Ports{}
			for _, portName := range nwo.OrdererPortNames() {
				ports[portName] = network.ReservePort()
			}
			network.PortsByOrdererID[o5.ID()] = ports

			network.Orderers = append(network.Orderers, o5)
			network.GenerateOrdererConfig(o5)

			extendNetwork(network)

			orderer5CertificatePath := filepath.Join(network.OrdererLocalTLSDir(o5), "server.crt")
			orderer5Certificate, err := ioutil.ReadFile(orderer5CertificatePath)
			Expect(err).NotTo(HaveOccurred())

			By("Adding the fifth orderr")
			nwo.AddConsenter(network, peer, orderers[0], "systemchannel", etcdraft.Consenter{
				ServerTlsCert: orderer5Certificate,
				ClientTlsCert: orderer5Certificate,
				Host:          "127.0.0.1",
				Port:          uint32(network.OrdererPort(o5, nwo.ListenPort)),
			})

			// Backup previous system channel block
			genesisBootBlock, err := ioutil.ReadFile(filepath.Join(testDir, "systemchannel_block.pb"))
			Expect(err).NotTo(HaveOccurred())
			restoreBootBlock := func() {
				err = ioutil.WriteFile(filepath.Join(testDir, "systemchannel_block.pb"), genesisBootBlock, 0644)
				Expect(err).NotTo(HaveOccurred())
			}

			// Get the last config block of the system channel
			configBlock := nwo.GetConfigBlock(network, peer, network.Orderers[0], "systemchannel")
			// Plant it in the file system of orderer5, the new node to be onboarded.
			err = ioutil.WriteFile(filepath.Join(testDir, "systemchannel_block.pb"), protoutil.MarshalOrPanic(configBlock), 0644)
			Expect(err).NotTo(HaveOccurred())

			launch := func(o *nwo.Orderer) {
				orderers = append(orderers, o)
				runner := network.OrdererRunner(o)
				ordererRunners = append(ordererRunners, runner)

				process := ifrit.Invoke(grouper.Member{Name: o.ID(), Runner: runner})
				Eventually(process.Ready()).Should(BeClosed())
				ordererProcesses = append(ordererProcesses, process)
			}

			By("Launching orderer5")
			launch(o5)

			By("Checking that orderer5 has onboarded the network")
			assertBlockReception(map[string]int{
				"systemchannel": 1,
			}, []*nwo.Orderer{o5}, peer, network)

			By("Kill all orderers but orderer5")
			for i := 0; i < 3; i++ {
				ordererProcesses[i].Signal(syscall.SIGTERM)
				Eventually(ordererProcesses[i].Wait(), network.EventuallyTimeout).Should(Receive())
			}

			By("Launching the orderer that was never started")
			restoreBootBlock()
			launch(o4)

			By("Waiting until it suspects its eviction from the channel")
			o4Runner := ordererRunners[len(ordererRunners)-1]
			Eventually(o4Runner.Err(), time.Minute, time.Second).Should(gbytes.Say("Suspecting our own eviction from the channel"))

			By("Resurrecting the dead orderers")
			for i, o := range []*nwo.Orderer{layout.Orderers[0], layout.Orderers[1], layout.Orderers[2]} {
				runner := network.OrdererRunner(o)
				ordererRunners[i] = runner
				process := ifrit.Invoke(runner)
				ordererProcesses[i] = process
			}

			By("Waiting for all orderers to serve the channel again")
			assertBlockReception(map[string]int{
				"systemchannel": 1,
			}, orderers, peer, network)

			By("Waiting until orderer 4 pulls the block it is missing")
			Eventually(o4Runner.Err()).Should(gbytes.Say("Got block 1"))

			By("Ensuring orderer 4 doesn't suspect its own eviction anymore")
			Consistently(o4Runner.Err(), time.Second*12, time.Second).ShouldNot(gbytes.Say("Suspecting our own eviction from the channel"))
		})
	})
})

func ensureEvicted(evictedOrderer *nwo.Orderer, submitter *nwo.Peer, network *nwo.Network, channel string) {
	c := commands.ChannelFetch{
		ChannelID:  channel,
		Block:      "newest",
		OutputFile: "/dev/null",
		Orderer:    network.OrdererAddress(evictedOrderer, nwo.ListenPort),
	}

	sess, err := network.OrdererAdminSession(evictedOrderer, submitter, c)
	Expect(err).NotTo(HaveOccurred())

	Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit())
	Expect(sess.Err).To(gbytes.Say("SERVICE_UNAVAILABLE"))
}

var extendedCryptoConfig = `---
OrdererOrgs:
- Name: OrdererOrg
  Domain: example.com
  EnableNodeOUs: false
  CA:
    Hostname: ca
  Specs:
  - Hostname: orderer1
    SANS:
    - localhost
    - 127.0.0.1
    - ::1
  - Hostname: orderer1new
    SANS:
    - localhost
    - 127.0.0.1
    - ::1
  - Hostname: orderer2
    SANS:
    - localhost
    - 127.0.0.1
    - ::1
  - Hostname: orderer2new
    SANS:
    - localhost
    - 127.0.0.1
    - ::1
  - Hostname: orderer3
    SANS:
    - localhost
    - 127.0.0.1
    - ::1
  - Hostname: orderer3new
    SANS:
    - localhost
    - 127.0.0.1
    - ::1
  - Hostname: orderer4
    SANS:
    - localhost
    - 127.0.0.1
    - ::1
`

type certificateChange struct {
	srcFile string
	dstFile string
	oldCert []byte
	oldKey  []byte
	newCert []byte
}

// extendNetwork rotates adds an additional orderer
func extendNetwork(n *nwo.Network) {
	// Overwrite the current crypto-config with additional orderers
	cryptoConfigYAML, err := ioutil.TempFile("", "crypto-config.yaml")
	Expect(err).NotTo(HaveOccurred())
	defer os.Remove(cryptoConfigYAML.Name())

	err = ioutil.WriteFile(cryptoConfigYAML.Name(), []byte(extendedCryptoConfig), 0644)
	Expect(err).NotTo(HaveOccurred())

	// Invoke cryptogen extend to add new orderers
	sess, err := n.Cryptogen(commands.Extend{
		Config: cryptoConfigYAML.Name(),
		Input:  n.CryptoPath(),
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
}

// refreshOrdererPEMs rotates all TLS certificates of all nodes,
// and returns the deltas
func refreshOrdererPEMs(n *nwo.Network) []*certificateChange {
	var fileChanges []*certificateChange
	// Populate source to destination files
	filepath.Walk(filepath.Join(n.RootDir, "crypto"), func(path string, info os.FileInfo, err error) error {
		if !strings.Contains(path, "/tls/") {
			return nil
		}
		if strings.Contains(path, "new") {
			fileChanges = append(fileChanges, &certificateChange{
				srcFile: path,
				dstFile: strings.Replace(path, "new", "", -1),
			})
		}
		return nil
	})

	var serverCertChanges []*certificateChange

	// Overwrite the destination files with the contents of the source files.
	for _, certChange := range fileChanges {
		previousCertBytes, err := ioutil.ReadFile(certChange.dstFile)
		Expect(err).NotTo(HaveOccurred())

		newCertBytes, err := ioutil.ReadFile(certChange.srcFile)
		Expect(err).NotTo(HaveOccurred())

		err = ioutil.WriteFile(certChange.dstFile, newCertBytes, 0644)
		Expect(err).NotTo(HaveOccurred())

		if !strings.Contains(certChange.dstFile, "server.crt") {
			continue
		}

		// Read the previous key file
		previousKeyBytes, err := ioutil.ReadFile(strings.Replace(certChange.dstFile, "server.crt", "server.key", -1))
		Expect(err).NotTo(HaveOccurred())

		serverCertChanges = append(serverCertChanges, certChange)
		certChange.newCert = newCertBytes
		certChange.oldCert = previousCertBytes
		certChange.oldKey = previousKeyBytes
	}
	return serverCertChanges
}

// assertBlockReception asserts that the given orderers have expected heights for the given channel--> height mapping
func assertBlockReception(expectedHeightsPerChannel map[string]int, orderers []*nwo.Orderer, p *nwo.Peer, n *nwo.Network) {
	assertReception := func(channelName string, blockSeq int) {
		var wg sync.WaitGroup
		wg.Add(len(orderers))
		for _, orderer := range orderers {
			go func(orderer *nwo.Orderer) {
				defer GinkgoRecover()
				defer wg.Done()
				waitForBlockReception(orderer, p, n, channelName, blockSeq)
			}(orderer)
		}
		wg.Wait()
	}

	var wg sync.WaitGroup
	wg.Add(len(expectedHeightsPerChannel))

	for channelName, blockSeq := range expectedHeightsPerChannel {
		go func(channelName string, blockSeq int) {
			defer GinkgoRecover()
			defer wg.Done()
			assertReception(channelName, blockSeq)
		}(channelName, blockSeq)
	}
	wg.Wait()
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

func assertNoErrorsAreLogged(ordererRunners []*ginkgomon.Runner) {
	var wg sync.WaitGroup
	wg.Add(len(ordererRunners))

	assertNoErrors := func(runner *ginkgomon.Runner) {
		buff := runner.Err()
		readOutput := func() string {
			out := bytes.Buffer{}
			// Read until no new input is detected
			for {
				b := make([]byte, 1024)
				n, _ := buff.Read(b)
				if n == 0 {
					break
				}
				bytesRead := make([]byte, n)
				copy(bytesRead, b)
				out.Write(bytesRead)
			}
			return out.String()
		}
		Eventually(readOutput, time.Minute, time.Second*5).Should(Not(ContainSubstring("ERRO")))
	}

	for _, runner := range ordererRunners {
		go func(runner *ginkgomon.Runner) {
			defer GinkgoRecover()
			defer wg.Done()
			assertNoErrors(runner)
		}(runner)
	}
	wg.Wait()
}

func deployChaincodes(n *nwo.Network, p *nwo.Peer, o *nwo.Orderer, mycc nwo.Chaincode, mycc2 nwo.Chaincode, mycc3 nwo.Chaincode) {
	var wg sync.WaitGroup
	wg.Add(3)
	for channel, chaincode := range map[string]nwo.Chaincode{
		"testchannel":  mycc,
		"testchannel2": mycc2,
		"testchannel3": mycc3,
	} {
		go func(channel string, cc nwo.Chaincode) {
			defer GinkgoRecover()
			defer wg.Done()
			nwo.DeployChaincode(n, channel, o, cc, p)
		}(channel, chaincode)
	}

	wg.Wait()
}

func assertInvoke(network *nwo.Network, peer *nwo.Peer, o *nwo.Orderer, cc string, channel string, expectedOutput string, expectedStatus int) {
	sess, err := network.PeerUserSession(peer, "User1", commands.ChaincodeInvoke{
		ChannelID: channel,
		Orderer:   network.OrdererAddress(o, nwo.ListenPort),
		Name:      cc,
		Ctor:      `{"Args":["invoke","a","b","10"]}`,
		PeerAddresses: []string{
			network.PeerAddress(peer, nwo.ListenPort),
		},
		WaitForEvent: true,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(expectedStatus))
	Expect(sess.Err).To(gbytes.Say(expectedOutput))
}

func revokeReaderAccess(network *nwo.Network, channel string, orderer *nwo.Orderer, peer *nwo.Peer) {
	config := nwo.GetConfig(network, peer, orderer, channel)
	updatedConfig := proto.Clone(config).(*common.Config)

	// set the policy
	adminPolicy := protoutil.MarshalOrPanic(&common.ImplicitMetaPolicy{
		SubPolicy: "Admins",
		Rule:      common.ImplicitMetaPolicy_MAJORITY,
	})
	updatedConfig.ChannelGroup.Groups["Orderer"].Policies["Readers"].Policy.Value = adminPolicy
	nwo.UpdateOrdererConfig(network, orderer, channel, config, updatedConfig, peer, orderer)
}
