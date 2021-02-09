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
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/gogo/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/gateway"
	"github.com/hyperledger/fabric/integration/nwo"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/tedsuo/ifrit"
	"google.golang.org/grpc"
)

var _ = Describe("GatewayService", func() {
	var (
		testDir       string
		client        *docker.Client
		network       *nwo.Network
		process       ifrit.Process
		orderer       *nwo.Orderer
		org1Peer0     *nwo.Peer
		org2Peer0     *nwo.Peer
		conn          *grpc.ClientConn
		gatewayClient gateway.GatewayClient
	)

	BeforeEach(func() {
		var err error
		testDir, err = ioutil.TempDir("", "gateway")
		Expect(err).NotTo(HaveOccurred())

		client, err = docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())
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

	Describe("evaluate with result", func() {
		BeforeEach(func() {
			config := nwo.BasicEtcdRaft()
			network = nwo.New(config, testDir, client, StartPort(), components)

			network.GatewayEnabled = true

			network.GenerateConfigTree()
			network.Bootstrap()

			networkRunner := network.NetworkGroupRunner()
			process = ifrit.Invoke(networkRunner)
			Eventually(process.Ready(), network.EventuallyTimeout).Should(BeClosed())

			orderer = network.Orderer("orderer")
			org1Peer0 = network.Peer("Org1", "peer0")
			org2Peer0 = network.Peer("Org2", "peer0")

			network.CreateAndJoinChannel(orderer, "testchannel")
			network.UpdateChannelAnchors(orderer, "testchannel")
			network.VerifyMembership(network.PeersWithChannel("testchannel"), "testchannel")

			nwo.EnableCapabilities(network, "testchannel", "Application", "V2_0", orderer, org1Peer0, org2Peer0)

			chaincodePath, err := filepath.Abs("../chaincode/gateway")
			Expect(err).NotTo(HaveOccurred())

			chaincode := nwo.Chaincode{
				Name:            "gatewaycc",
				Version:         "0.0",
				Path:            chaincodePath,
				Lang:            "golang",
				PackageFile:     filepath.Join(testDir, "gatewaycc.tar.gz"),
				Ctor:            `{"Args":[]}`,
				SignaturePolicy: `AND ('Org1MSP.peer')`,
				Sequence:        "1",
				InitRequired:    false,
				Label:           "gatewaycc",
			}

			nwo.DeployChaincode(network, "testchannel", orderer, chaincode)

			conn = network.PeerClientConn(org1Peer0)
			gatewayClient = gateway.NewGatewayClient(conn)
		})

		AfterEach(func() {
			conn.Close()
		})

		Context("when I evaluate an echo transaction with the arguments [\"conga\"]", func() {
			It("should respond with \"conga\"", func() {
				ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
				defer cancel()

				signingIdentity := network.PeerUserSigner(org1Peer0, "User1")
				txn := NewProposedTransaction(signingIdentity, "testchannel", "gatewaycc", "Echo", []byte("conga"))

				result, err := gatewayClient.Evaluate(ctx, txn)
				Expect(err).NotTo(HaveOccurred())
				expectedResult := &gateway.Result{
					Value: []byte("conga"),
				}
				Expect(result.Value).To(Equal(expectedResult.Value))
				Expect(proto.Equal(result, expectedResult)).To(BeTrue())
			})
		})
	})
})
