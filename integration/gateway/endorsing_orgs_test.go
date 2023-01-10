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
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/protoutil"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/tedsuo/ifrit"
)

var _ = Describe("GatewayService with endorsing orgs", func() {
	var (
		testDir   string
		network   *nwo.Network
		orderer   *nwo.Orderer
		org1Peer0 *nwo.Peer
		org2Peer0 *nwo.Peer
		org3Peer0 *nwo.Peer
		process   ifrit.Process
	)

	BeforeEach(func() {
		var err error
		testDir, err = ioutil.TempDir("", "gateway")
		Expect(err).NotTo(HaveOccurred())

		client, err := docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())

		config := nwo.ThreeOrgEtcdRaft()
		network = nwo.New(config, testDir, client, StartPort(), components)

		network.GenerateConfigTree()
		network.Bootstrap()

		networkRunner := network.NetworkGroupRunner()
		process = ifrit.Invoke(networkRunner)
		Eventually(process.Ready(), network.EventuallyTimeout).Should(BeClosed())

		orderer = network.Orderer("orderer")
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

		org1Peer0 = network.Peer("Org1", "peer0")
		org2Peer0 = network.Peer("Org2", "peer0")
		org3Peer0 = network.Peer("Org3", "peer0")

		chaincode := nwo.Chaincode{
			Name:              "pvtmarblescc",
			Version:           "0.0",
			Path:              components.Build("github.com/hyperledger/fabric/integration/chaincode/marbles_private/cmd"),
			Lang:              "binary",
			PackageFile:       filepath.Join(testDir, "pvtmarblescc.tar.gz"),
			Policy:            `OR ('Org1MSP.member', 'Org2MSP.member', 'Org3MSP.member')`,
			SignaturePolicy:   `OR ('Org1MSP.member', 'Org2MSP.member', 'Org3MSP.member')`,
			Sequence:          "1",
			InitRequired:      false,
			Label:             "pvtmarblescc_label",
			CollectionsConfig: filepath.Join("testdata", "collections_config_anyorg.json"),
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

	It("should execute chaincode on a peer in the specified org", func() {
		peers := [3]*nwo.Peer{org1Peer0, org2Peer0, org3Peer0}

		// Submit transactions using every peer's gateway, with every peer as the endorsing
		// org, to reduce the likelihood of tests passing by chance
		for _, p := range peers {
			conn := network.PeerClientConn(p)
			defer conn.Close()
			gatewayClient := gateway.NewGatewayClient(conn)
			ctx, cancel := context.WithTimeout(context.Background(), network.EventuallyTimeout)
			defer cancel()

			signingIdentity := network.PeerUserSigner(p, "User1")
			for _, o := range peers {
				mspid := network.Organization(o.Organization).MSPID

				submitCheckEndorsingOrgsTransaction(ctx, gatewayClient, signingIdentity, mspid)
			}
		}
	})
})

func submitCheckEndorsingOrgsTransaction(ctx context.Context, client gateway.GatewayClient, signingIdentity *nwo.SigningIdentity, mspids ...string) {
	transientData := make(map[string][]byte)
	for _, m := range mspids {
		transientData[m] = []byte(`true`)
	}

	proposedTransaction, transactionID := NewProposedTransaction(signingIdentity, "testchannel", "pvtmarblescc", "checkEndorsingOrg", transientData)

	endorseRequest := &gateway.EndorseRequest{
		TransactionId:          transactionID,
		ChannelId:              "testchannel",
		ProposedTransaction:    proposedTransaction,
		EndorsingOrganizations: mspids,
	}

	endorseResponse, err := client.Endorse(ctx, endorseRequest)
	Expect(err).NotTo(HaveOccurred())

	chaincodeAction, err := protoutil.GetActionFromEnvelopeMsg(endorseResponse.GetPreparedTransaction())
	Expect(err).NotTo(HaveOccurred())

	result := chaincodeAction.GetResponse()

	expectedPayload := "Peer mspid OK"
	Expect(string(result.Payload)).To(Equal(expectedPayload))
	expectedResult := &peer.Response{
		Status:  200,
		Message: "",
		Payload: []uint8(expectedPayload),
	}
	Expect(proto.Equal(result, expectedResult)).To(BeTrue(), "Expected\n\t%#v\nto proto.Equal\n\t%#v", result, expectedResult)
}
