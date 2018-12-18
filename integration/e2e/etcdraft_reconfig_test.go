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
	"sync"
	"syscall"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	"github.com/hyperledger/fabric/protos/orderer/etcdraft"
	"github.com/hyperledger/fabric/protos/utils"
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
		testDir   string
		client    *docker.Client
		network   *nwo.Network
		chaincode nwo.Chaincode
		peer      *nwo.Peer

		peerProcesses    ifrit.Process
		ordererProcesses []ifrit.Process
		ordererRunners   []*ginkgomon.Runner
	)

	BeforeEach(func() {
		var err error
		testDir, err = ioutil.TempDir("", "e2e-etcfraft_reconfig")
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
			//   | system channel height | testchannel  height  | update description
			// ------------------------------------------------------------------------
			// 0 |            2          |         1            | adding consenter 1'
			// 1 |            3          |         2            | removing consenter 1
			// 2 |            4          |         3            | adding consenter 2'
			// 3 |            5          |         4            | removing consenter 2
			// 4 |            6          |         5            | adding consenter 3'
			// 5 |            7          |         6            | removing consenter 3
			// 6 |            8          |         6            | creating channel testchannel2
			// 7 |            9          |         7            | adding consenter 4
			// 8 |            9          |         8            | deploying chaincode on testchannel
			// 9 |            9          |         9            | invoking chaincode on testchannel

			layout := nwo.MultiNodeEtcdRaft()
			layout.Channels = append(layout.Channels, &nwo.Channel{
				Name:    "testchannel2",
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
				o4Runner := network.OrdererRunner(orderers[i])
				ordererRunners = append(ordererRunners, o4Runner)
				ordererProcesses[i] = ifrit.Invoke(grouper.Member{Name: orderers[i].ID(), Runner: o4Runner})
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

			network.Orderers = append(network.Orderers, &nwo.Orderer{Name: "orderer4", Organization: "OrdererOrg"})
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
				"systemchannel": 9,
				"testchannel":   7,
			}, orderers, peer, network)

			By("Transacting on testchannel")
			peers := network.PeersWithChannel("testchannel")
			network.JoinChannel("testchannel", o1, peers...)
			nwo.DeployChaincode(network, "testchannel", o2, chaincode)

			By("Waiting for orderers to sync")
			assertBlockReception(map[string]int{
				"systemchannel": 9,
				"testchannel":   8,
			}, orderers, peer, network)

			RunQueryInvokeQuery(network, o1, peer, "testchannel")

			By("Transacting on testchannel once more")
			assertBlockReception(map[string]int{
				"systemchannel": 9,
				"testchannel":   9,
			}, orderers, peer, network)

			// Get the last config block of the system channel
			configBlock := nwo.GetConfigBlock(network, peer, o1, "systemchannel")
			// Plant it in the file system of orderer4, the new node to be onboarded.
			err = ioutil.WriteFile(filepath.Join(testDir, "systemchannel_block.pb"), utils.MarshalOrPanic(configBlock), 06444)
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
				"systemchannel": 9,
				"testchannel":   9,
			}, orderers, peer, network)

			By("Ensuring orderer4 doesn't serve testchannel2")
			sess, err := network.OrdererAdminSession(o4, peer, commands.ChannelFetch{
				ChannelID:  "testchannel2",
				Block:      "config",
				Orderer:    network.OrdererAddress(o4, nwo.ListenPort),
				OutputFile: "/dev/null",
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(1))
			Expect(sess.Err).To(gbytes.Say("NOT_FOUND"))

			By("Ensuring that all orderers don't log errors to the log")
			assertNoErrorsAreLogged(ordererRunners)
		})
	})
})

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
	newCert []byte
}

// refreshOrdererPEMs rotates all TLS certificates of all nodes,
// and returns the deltas
func refreshOrdererPEMs(n *nwo.Network) []*certificateChange {
	var fileChanges []*certificateChange
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

		err = ioutil.WriteFile(certChange.dstFile, newCertBytes, 06444)
		Expect(err).NotTo(HaveOccurred())

		if !strings.Contains(certChange.dstFile, "server.crt") {
			continue
		}
		serverCertChanges = append(serverCertChanges, certChange)
		certChange.newCert = newCertBytes
		certChange.oldCert = previousCertBytes
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
		Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
		sessErr := string(sess.Err.Contents())
		expected := fmt.Sprintf("Received block: %d", blockSeq)
		if strings.Contains(sessErr, expected) {
			return ""
		}
		return sessErr
	}, network.EventuallyTimeout).Should(BeEmpty())
}

func assertNoErrorsAreLogged(ordererRunners []*ginkgomon.Runner) {
	var wg sync.WaitGroup
	wg.Add(len(ordererRunners))

	assertNoErrors := func(runner *ginkgomon.Runner) {
		buff := runner.Buffer()
		// Advance buffer read cursor to the end by reading everything
		buff.Read(make([]byte, len(buff.Contents())))
		// Starting from now, there shouldn't be any error strings logged
		// for 5 seconds.
		select {
		case <-time.After(time.Second * 5):
		case <-buff.Detect("ERRO"):
			Fail("Detected 'ERRO' in the buffer")
		}
		buff.CancelDetects()
	}

	for _, runner := range ordererRunners {
		go func(runner *ginkgomon.Runner) {
			defer wg.Done()
			assertNoErrors(runner)
		}(runner)
	}
	wg.Wait()
}
