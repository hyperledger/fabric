/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"syscall"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric/integration/channelparticipation"
	"github.com/hyperledger/fabric/integration/nwo"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/tedsuo/ifrit"
	ginkgomon "github.com/tedsuo/ifrit/ginkgomon_v2"
)

var _ = Describe("etcdraft network using ccenv-1.4", func() {
	var (
		client                      *docker.Client
		testDir                     string
		network                     *nwo.Network
		ordererRunner               *ginkgomon.Runner
		ordererProcess, peerProcess ifrit.Process
	)

	BeforeEach(func() {
		if runtime.GOARCH != "amd64" {
			Skip("ccenv-1.4 image may not be available for this platform")
		}
		var err error
		testDir, err = ioutil.TempDir("", "lifecycle")
		Expect(err).NotTo(HaveOccurred())

		client, err = docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())

		network = nwo.New(nwo.BasicEtcdRaftNoSysChan(), testDir, client, StartPort(), components)
		network.GenerateConfigTree()
		for _, peer := range network.PeersWithChannel("testchannel") {
			core := network.ReadPeerConfig(peer)
			core.Chaincode.Builder = "$(DOCKER_NS)/fabric-ccenv:1.4"
			network.WritePeerConfig(peer, core)
		}
		network.Bootstrap()

		// Start all the fabric processes
		ordererRunner, ordererProcess, peerProcess = network.StartSingleOrdererNetwork("orderer")
	})

	AfterEach(func() {
		// Shutdown processes and cleanup
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

	It("deploys and executes chaincode (simple)", func() {
		By("deploying the chaincode using LSCC on a channel with V1_4 application capabilities")
		orderer := network.Orderer("orderer")
		endorsers := []*nwo.Peer{
			network.Peer("Org1", "peer0"),
			network.Peer("Org2", "peer0"),
		}

		cwd, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())

		// The chaincode in the CDS file for this test was packaged using
		// the cli container created via the docker-compose.yaml in this directory.
		// At the time of packaging, hyperledger/fabric-tools:1.4 had
		// image id '18ed4db0cd57'.
		//
		// It was packaged using the following command:
		// peer chaincode package --name mycc --version 0.0 --lang golang --path github.com/chaincode/simple-v14 mycc-0_0-v14.cds
		chaincode := nwo.Chaincode{
			Name:        "mycc",
			Version:     "0.0",
			PackageFile: filepath.Join(cwd, "testdata/mycc-0_0-v14.cds"),
			Ctor:        `{"Args":["init","a","100","b","200"]}`,
			Policy:      `AND ('Org1MSP.member','Org2MSP.member')`,
		}

		channelparticipation.JoinOrdererJoinPeersAppChannel(network, "testchannel", orderer, ordererRunner)
		nwo.DeployChaincodeLegacy(network, "testchannel", orderer, chaincode)
		RunQueryInvokeQuery(network, orderer, "mycc", 100, endorsers...)
	})
})
