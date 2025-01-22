/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric/integration/channelparticipation"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/onsi/gomega/gmeasure"
	"github.com/tedsuo/ifrit"
	ginkgomon "github.com/tedsuo/ifrit/ginkgomon_v2"
)

var _ = Describe("Network", func() {
	var (
		client  *docker.Client
		tempDir string
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "nwo")
		Expect(err).NotTo(HaveOccurred())

		client, err = docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		os.RemoveAll(tempDir)
	})

	DescribeTableSubtree("benchmark write batch", func(desc string, useWriteBatch bool, startWriteBatch bool) {
		var network *nwo.Network
		var ordererRunner *ginkgomon.Runner
		var ordererProcess, peerProcess ifrit.Process

		BeforeEach(func() {
			network = nwo.New(nwo.BasicEtcdRaft(), tempDir, client, StartPort(), components)
			network.UseWriteBatch = useWriteBatch

			// Generate config and bootstrap the network
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
		})

		It("deploys and executes experiment bench", func() {
			orderer := network.Orderer("orderer")
			channelparticipation.JoinOrdererJoinPeersAppChannel(network, "testchannel", orderer, ordererRunner)
			peer := network.Peer("Org1", "peer0")

			chaincode := nwo.Chaincode{
				Name:            "mycc",
				Version:         "0.0",
				Path:            "github.com/hyperledger/fabric/integration/chaincode/multi/cmd",
				Lang:            "golang",
				PackageFile:     filepath.Join(tempDir, "multi.tar.gz"),
				Ctor:            `{"Args":["init"]}`,
				SignaturePolicy: `AND ('Org1MSP.member','Org2MSP.member')`,
				Sequence:        "1",
				InitRequired:    true,
				Label:           "my_multi_operations_chaincode",
			}

			network.VerifyMembership(network.PeersWithChannel("testchannel"), "testchannel")

			nwo.EnableCapabilities(
				network,
				"testchannel",
				"Application", "V2_0",
				orderer,
				network.PeersWithChannel("testchannel")...,
			)
			nwo.DeployChaincode(network, "testchannel", orderer, chaincode)

			By("run invoke where only put state")
			experiment := gmeasure.NewExperiment("Only PUT State " + desc)
			AddReportEntry(experiment.Name, experiment)

			experiment.SampleDuration("invoke N-5 cycle-1", func(idx int) {
				RunInvoke(network, orderer, peer, "invoke", startWriteBatch, 1, 0, nil)
			}, gmeasure.SamplingConfig{N: 5})

			experiment.SampleDuration("invoke N-10 cycle-1000", func(idx int) {
				RunInvoke(network, orderer, peer, "invoke", startWriteBatch, 1000, 0, nil)
			}, gmeasure.SamplingConfig{N: 10})

			experiment.SampleDuration("invoke N-10 cycle-10000", func(idx int) {
				RunInvoke(network, orderer, peer, "invoke", startWriteBatch, 10000, 0, nil)
			}, gmeasure.SamplingConfig{N: 10})

			RunGetState(network, peer, "0")
		})
	},
		Entry("without write batch1", "useWriteBatch - true, StartWriteBatch - false", true, false),
		Entry("without write batch2", "useWriteBatch - false, StartWriteBatch - true", false, true),
		Entry("with write batch", "useWriteBatch - true, StartWriteBatch - true", true, true),
	)

	Describe("Put Private key with error", func() {
		var network *nwo.Network
		var ordererRunner *ginkgomon.Runner
		var ordererProcess, peerProcess ifrit.Process

		BeforeEach(func() {
			network = nwo.New(nwo.BasicEtcdRaft(), tempDir, client, StartPort(), components)

			// Generate config and bootstrap the network
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
		})

		It("put private data for error", func() {
			orderer := network.Orderer("orderer")
			channelparticipation.JoinOrdererJoinPeersAppChannel(network, "testchannel", orderer, ordererRunner)
			peer := network.Peer("Org1", "peer0")

			chaincode := nwo.Chaincode{
				Name:            "mycc",
				Version:         "0.0",
				Path:            "github.com/hyperledger/fabric/integration/chaincode/multi/cmd",
				Lang:            "golang",
				PackageFile:     filepath.Join(tempDir, "multi.tar.gz"),
				Ctor:            `{"Args":["init"]}`,
				SignaturePolicy: `AND ('Org1MSP.member','Org2MSP.member')`,
				Sequence:        "1",
				InitRequired:    true,
				Label:           "my_multi_operations_chaincode",
			}

			network.VerifyMembership(network.PeersWithChannel("testchannel"), "testchannel")

			nwo.EnableCapabilities(
				network,
				"testchannel",
				"Application", "V2_0",
				orderer,
				network.PeersWithChannel("testchannel")...,
			)
			nwo.DeployChaincode(network, "testchannel", orderer, chaincode)

			By("put private key, startWriteBatch - false, keys - 1")
			RunInvoke(network, orderer, peer, "put-private-key", false, 1, 1, []string{"collection testchannel/mycc/col", "could not be found"})
			By("put private key, startWriteBatch - false, keys - 2")
			RunInvoke(network, orderer, peer, "put-private-key", false, 2, 1, []string{"collection testchannel/mycc/col", "could not be found"})
			By("put private key, startWriteBatch - false, keys - 3")
			RunInvoke(network, orderer, peer, "put-private-key", false, 3, 1, []string{"collection testchannel/mycc/col", "could not be found"})
			By("put private key, startWriteBatch - true, keys - 1")
			RunInvoke(network, orderer, peer, "put-private-key", true, 1, 1, []string{"collection testchannel/mycc/col", "could not be found"})
			By("put private key, startWriteBatch - true, keys - 2")
			RunInvoke(network, orderer, peer, "put-private-key", true, 2, 1, []string{"collection testchannel/mycc/col", "could not be found"})
			By("put private key, startWriteBatch - true, keys - 3")
			RunInvoke(network, orderer, peer, "put-private-key", true, 3, 1, []string{"collection testchannel/mycc/col", "could not be found"})
		})
	})

	DescribeTableSubtree("benchmark get multiple keys", func(desc string, useGetMultipleKeys bool) {
		var network *nwo.Network
		var ordererRunner *ginkgomon.Runner
		var ordererProcess, peerProcess ifrit.Process

		BeforeEach(func() {
			network = nwo.New(nwo.BasicEtcdRaft(), tempDir, client, StartPort(), components)
			network.UseGetMultipleKeys = useGetMultipleKeys

			// Generate config and bootstrap the network
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
		})

		It("deploys and executes experiment bench", func() {
			orderer := network.Orderer("orderer")
			channelparticipation.JoinOrdererJoinPeersAppChannel(network, "testchannel", orderer, ordererRunner)
			peer := network.Peer("Org1", "peer0")

			chaincode := nwo.Chaincode{
				Name:            "mycc",
				Version:         "0.0",
				Path:            "github.com/hyperledger/fabric/integration/chaincode/multi/cmd",
				Lang:            "golang",
				PackageFile:     filepath.Join(tempDir, "multi.tar.gz"),
				Ctor:            `{"Args":["init"]}`,
				SignaturePolicy: `AND ('Org1MSP.member','Org2MSP.member')`,
				Sequence:        "1",
				InitRequired:    true,
				Label:           "my_multi_operations_chaincode",
			}

			network.VerifyMembership(network.PeersWithChannel("testchannel"), "testchannel")

			nwo.EnableCapabilities(
				network,
				"testchannel",
				"Application", "V2_0",
				orderer,
				network.PeersWithChannel("testchannel")...,
			)
			nwo.DeployChaincode(network, "testchannel", orderer, chaincode)

			RunInvoke(network, orderer, peer, "invoke", true, 10000, 0, nil)

			By("run query get state multiple keys")
			experiment := gmeasure.NewExperiment("Get state multiple keys " + desc)
			AddReportEntry(experiment.Name, experiment)

			experiment.SampleDuration("invoke N-10 cycle-1000", func(idx int) {
				RunGetStateMultipleKeys(network, peer, 1000)
			}, gmeasure.SamplingConfig{N: 10})

			experiment.SampleDuration("invoke N-10 cycle-10000", func(idx int) {
				RunGetStateMultipleKeys(network, peer, 10000)
			}, gmeasure.SamplingConfig{N: 10})
		})
	},
		Entry("without peer support", "without peer support", false),
		Entry("with peer support", "with peer support", true),
	)
})

func RunInvoke(n *nwo.Network, orderer *nwo.Orderer, peer *nwo.Peer, fn string, startWriteBatch bool, numberCallsPut int, exitCode int, expectedError []string) {
	sess, err := n.PeerUserSession(peer, "User1", commands.ChaincodeInvoke{
		ChannelID: "testchannel",
		Orderer:   n.OrdererAddress(orderer, nwo.ListenPort),
		Name:      "mycc",
		Ctor:      `{"Args":["` + fn + `","` + fmt.Sprint(startWriteBatch) + `","` + fmt.Sprint(numberCallsPut) + `"]}`,
		PeerAddresses: []string{
			n.PeerAddress(n.Peer("Org1", "peer0"), nwo.ListenPort),
			n.PeerAddress(n.Peer("Org2", "peer0"), nwo.ListenPort),
		},
		WaitForEvent: true,
	})
	Expect(err).NotTo(HaveOccurred())
	if exitCode != 0 {
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(exitCode))
		for _, msg := range expectedError {
			Expect(sess.Err).To(gbytes.Say(msg))
		}
		return
	}
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess.Err).To(gbytes.Say("Chaincode invoke successful. result: status:200"))
}

func RunGetState(n *nwo.Network, peer *nwo.Peer, keyUniq string) {
	sess, err := n.PeerUserSession(peer, "User1", commands.ChaincodeQuery{
		ChannelID: "testchannel",
		Name:      "mycc",
		Ctor:      `{"Args":["get-key","` + keyUniq + `"]}`,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess).To(gbytes.Say("key" + keyUniq))
}

func RunGetStateMultipleKeys(n *nwo.Network, peer *nwo.Peer, countKeys int) {
	sess, err := n.PeerUserSession(peer, "User1", commands.ChaincodeQuery{
		ChannelID: "testchannel",
		Name:      "mycc",
		Ctor:      `{"Args":["get-multiple-keys","` + fmt.Sprint(countKeys) + `"]}`,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
}
