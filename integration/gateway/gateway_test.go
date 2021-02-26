/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gateway

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/gateway"
	"github.com/hyperledger/fabric/integration/nwo"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/tedsuo/ifrit"
)

var _ = Describe("GatewayService", func() {
	var (
		testDir string
		network *nwo.Network
		process ifrit.Process
	)

	BeforeEach(func() {
		var err error
		testDir, err = ioutil.TempDir("", "gateway")
		Expect(err).NotTo(HaveOccurred())

		client, err := docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())

		config := nwo.BasicEtcdRaft()
		network = nwo.New(config, testDir, client, StartPort(), components)

		network.GatewayEnabled = true

		network.GenerateConfigTree()
		network.Bootstrap()

		networkRunner := network.NetworkGroupRunner()
		process = ifrit.Invoke(networkRunner)
		Eventually(process.Ready(), network.EventuallyTimeout).Should(BeClosed())

		orderer := network.Orderer("orderer")
		network.CreateAndJoinChannel(orderer, "testchannel")
		network.UpdateChannelAnchors(orderer, "testchannel")
		network.VerifyMembership(
			network.PeersWithChannel("testchannel"),
			"testchannel",
		)
		nwo.EnableCapabilities(
			network,
			"testchannel",
			"Application", "V2_0",
			orderer,
			network.PeersWithChannel("testchannel")...,
		)

		chaincode := nwo.Chaincode{
			Name:            "simplecc",
			Version:         "0.0",
			Path:            components.Build("github.com/hyperledger/fabric/integration/chaincode/simple/cmd"),
			Lang:            "binary",
			PackageFile:     filepath.Join(testDir, "simplecc.tar.gz"),
			Ctor:            `{"Args":["init","a","100","b","200"]}`,
			SignaturePolicy: `AND ('Org1MSP.peer')`,
			Sequence:        "1",
			InitRequired:    false,
			Label:           "simplecc_label",
		}

		nwo.DeployChaincode(network, "testchannel", orderer, chaincode)
	})

	AfterEach(func() {
		if process != nil {
			process.Signal(syscall.SIGTERM)
			Eventually(process.Wait(), network.EventuallyTimeout).Should(Receive())
		}
		if network != nil {
			network.Cleanup()
		}
		os.RemoveAll(testDir)
	})

	Describe("calling Evaluate", func() {
		It("should respond with the expected result", func() {
			org1Peer0 := network.Peer("Org1", "peer0")
			conn := network.PeerClientConn(org1Peer0)
			defer conn.Close()
			gatewayClient := gateway.NewGatewayClient(conn)
			ctx, cancel := context.WithTimeout(context.Background(), network.EventuallyTimeout)
			defer cancel()

			signingIdentity := network.PeerUserSigner(org1Peer0, "User1")
			txn := NewProposedTransaction(signingIdentity, "testchannel", "simplecc", "echo", []byte("conga"))

			result, err := gatewayClient.Evaluate(ctx, txn)
			Expect(err).NotTo(HaveOccurred())
			expectedResult := &gateway.Result{
				Value: []byte("conga"),
			}
			Expect(result.Value).To(Equal(expectedResult.Value))
			Expect(proto.Equal(result, expectedResult)).To(BeTrue(), "Expected\n\t%#v\nto proto.Equal\n\t%#v", result, expectedResult)
		})
	})
})
