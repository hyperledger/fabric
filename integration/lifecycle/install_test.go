/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric/integration/channelparticipation"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	"github.com/hyperledger/fabric/integration/nwo/fabricconfig"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
	ginkgomon "github.com/tedsuo/ifrit/ginkgomon_v2"
)

var _ = Describe("chaincode install", func() {
	var (
		client  *docker.Client
		testDir string

		network                     *nwo.Network
		ordererRunner               *ginkgomon.Runner
		ordererProcess, peerProcess ifrit.Process
	)

	BeforeEach(func() {
		var err error
		testDir, err = ioutil.TempDir("", "lifecycle")
		Expect(err).NotTo(HaveOccurred())

		client, err = docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())

		cwd, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())

		network = nwo.New(nwo.BasicEtcdRaftNoSysChan(), testDir, client, StartPort(), components)
		network.ExternalBuilders = append(network.ExternalBuilders, fabricconfig.ExternalBuilder{
			Path:                 filepath.Join(cwd, "..", "externalbuilders", "golang"),
			Name:                 "external-golang",
			PropagateEnvironment: []string{"GOPATH", "GOCACHE", "GOPROXY", "HOME", "PATH"},
		})
		network.GenerateConfigTree()
		network.Bootstrap()

		// Start all the fabric processes
		ordererRunner, ordererProcess, peerProcess = network.StartSingleOrdererNetwork("orderer")
	})

	AfterEach(func() {
		if ordererProcess != nil {
			ordererProcess.Signal(syscall.SIGTERM)
			Eventually(ordererProcess.Wait(), network.EventuallyTimeout).Should(Receive())
		}

		if peerProcess != nil {
			peerProcess.Signal(syscall.SIGTERM)
			Eventually(peerProcess.Wait(), network.EventuallyTimeout).Should(Receive())
		}
		network.Cleanup()
		os.RemoveAll(testDir)
	})

	Describe("external builder failures", func() {
		var (
			orderer   *nwo.Orderer
			org1Peer  *nwo.Peer
			org2Peer  *nwo.Peer
			chaincode nwo.Chaincode
		)

		BeforeEach(func() {
			packageTempDir, err := ioutil.TempDir(network.RootDir, "chaincode-package")
			Expect(err).NotTo(HaveOccurred())

			orderer = network.Orderer("orderer")
			org1Peer = network.Peer("Org1", "peer0")
			org2Peer = network.Peer("Org2", "peer0")
			channelparticipation.JoinOrdererJoinPeersAppChannel(network, "testchannel", orderer, ordererRunner)

			chaincode = nwo.Chaincode{
				Name:            "failure-external",
				Version:         "0.0",
				Lang:            "golang",
				Path:            "github.com/hyperledger/fabric/integration/chaincode/simple/cmd",
				Ctor:            `{"Args":["init","a","100","b","200"]}`,
				Policy:          `OR ('Org1MSP.member','Org2MSP.member')`,
				SignaturePolicy: `OR ('Org1MSP.member','Org2MSP.member')`,
				Sequence:        "1",
				InitRequired:    false,
				Label:           "failure-external",
				PackageFile:     filepath.Join(packageTempDir, "chaincode-package"),
			}
		})

		It("legacy does not fallback to internal platforms", func() {
			By("packaging the chaincode using the legacy lifecycle")
			nwo.PackageChaincodeLegacy(network, chaincode, org1Peer)

			By("installing the chaincode using the legacy lifecycle")
			sess, err := network.PeerAdminSession(org1Peer, commands.ChaincodeInstallLegacy{
				Name:        chaincode.Name,
				Version:     chaincode.Version,
				Path:        chaincode.Path,
				Lang:        chaincode.Lang,
				PackageFile: chaincode.PackageFile,
				ClientAuth:  network.ClientAuthRequired,
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(1))

			Expect(sess.Err).To(gbytes.Say(`\Qexternal builder 'external-golang' failed: exit status 1\E`))
		})

		It("_lifecycle does not fallback to internal platforms when an external builder fails", func() {
			By("enabling V2_0 application capabilities")
			nwo.EnableCapabilities(network, "testchannel", "Application", "V2_0", orderer, org1Peer, org2Peer)

			By("packaging the chaincode")
			nwo.PackageChaincode(network, chaincode, org1Peer)

			By("installing the chaincode using _lifecycle")
			sess, err := network.PeerAdminSession(org1Peer, commands.ChaincodeInstall{
				PackageFile: chaincode.PackageFile,
				ClientAuth:  network.ClientAuthRequired,
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(1))

			Expect(sess.Err).To(gbytes.Say(`\Qexternal builder 'external-golang' failed: exit status 1\E`))
		})
	})
})
