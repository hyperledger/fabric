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
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/tedsuo/ifrit"
)

var _ = Describe("GatewayService with endorser discovery", func() {
	var (
		testDir   string
		network   *nwo.Network
		orderer   *nwo.Orderer
		org1Peer0 *nwo.Peer
		org2Peer0 *nwo.Peer
		org3Peer0 *nwo.Peer
		process   ifrit.Process
		peerCerts map[string]string
	)

	loadPeerCert := func(peer *nwo.Peer) string {
		peerCert, err := ioutil.ReadFile(network.PeerCert(peer))
		Expect(err).NotTo(HaveOccurred())
		return string(peerCert)
	}

	BeforeEach(func() {
		var err error
		testDir, err = ioutil.TempDir("", "gateway")
		Expect(err).NotTo(HaveOccurred())

		client, err := docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())

		config := nwo.ThreeOrgRaft()
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

		peerCerts = map[string]string{
			loadPeerCert(org1Peer0): org1Peer0.ID(),
			loadPeerCert(org2Peer0): org2Peer0.ID(),
			loadPeerCert(org3Peer0): org3Peer0.ID(),
		}

		chaincode := nwo.Chaincode{
			Name:            "gatewaycc",
			Version:         "0.0",
			Path:            components.Build("github.com/hyperledger/fabric/integration/chaincode/simple/cmd"),
			Lang:            "binary",
			PackageFile:     filepath.Join(testDir, "gatewaycc.tar.gz"),
			Ctor:            `{"Args":["init","a","100","b","200"]}`,
			SignaturePolicy: `AND ('Org1MSP.peer')`,
			Sequence:        "1",
			InitRequired:    true,
			Label:           "gatewaycc_label",
		}
		nwo.DeployChaincode(network, "testchannel", orderer, chaincode)

		chaincode = nwo.Chaincode{
			Name:              "sbecc",
			Version:           "0.0",
			Path:              components.Build("github.com/hyperledger/fabric/integration/chaincode/keylevelep/cmd"),
			Ctor:              `{"Args":["init"]}`,
			Lang:              "binary",
			PackageFile:       filepath.Join(testDir, "sbecc.tar.gz"),
			Policy:            `OR ('Org3MSP.member')`,
			SignaturePolicy:   `OR ('Org3MSP.member')`,
			Sequence:          "1",
			InitRequired:      true,
			Label:             "sbecc_label",
			CollectionsConfig: filepath.Join("testdata", "collections_config_sbe.json"),
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

	verifyEndorsers := func(endorseResponse *gateway.EndorseResponse, expectedEndorsers []string) {
		preparedTransaction := endorseResponse.GetPreparedTransaction()
		payload, err := protoutil.UnmarshalPayload(preparedTransaction.GetPayload())
		Expect(err).NotTo(HaveOccurred())
		transaction, err := protoutil.UnmarshalTransaction(payload.GetData())
		Expect(err).NotTo(HaveOccurred())
		Expect(transaction.GetActions()).To(HaveLen(1))
		action, err := protoutil.UnmarshalChaincodeActionPayload(transaction.Actions[0].GetPayload())
		Expect(err).NotTo(HaveOccurred())
		endorsements := action.GetAction().GetEndorsements()

		actualEndorsers := []string{}
		for _, endorsement := range endorsements {
			id, err := protoutil.UnmarshalSerializedIdentity(endorsement.GetEndorser())
			Expect(err).NotTo(HaveOccurred())
			actualEndorsers = append(actualEndorsers, peerCerts[string(id.IdBytes)])
		}

		Expect(actualEndorsers).To(ConsistOf(expectedEndorsers))
	}

	submitTransaction := func(
		gatewayClient gateway.GatewayClient,
		signer *nwo.SigningIdentity,
		channel string,
		chaincode string,
		transactionName string,
		arguments []string,
		transientData map[string][]byte,
		expectedEndorsers []string,
	) {
		args := [][]byte{}
		for _, arg := range arguments {
			args = append(args, []byte(arg))
		}
		proposedTransaction, transactionID := NewProposedTransaction(
			signer,
			channel,
			chaincode,
			transactionName,
			transientData,
			args...,
		)

		endorseRequest := &gateway.EndorseRequest{
			TransactionId:       transactionID,
			ChannelId:           channel,
			ProposedTransaction: proposedTransaction,
		}

		ctx, cancel := context.WithTimeout(context.Background(), network.EventuallyTimeout)
		defer cancel()

		endorseResponse, err := gatewayClient.Endorse(ctx, endorseRequest)
		Expect(err).NotTo(HaveOccurred())

		preparedTransaction := endorseResponse.GetPreparedTransaction()
		preparedTransaction.Signature, err = signer.Sign(preparedTransaction.Payload)
		Expect(err).NotTo(HaveOccurred())

		verifyEndorsers(endorseResponse, expectedEndorsers)

		submitRequest := &gateway.SubmitRequest{
			TransactionId:       transactionID,
			ChannelId:           channel,
			PreparedTransaction: preparedTransaction,
		}
		_, err = gatewayClient.Submit(ctx, submitRequest)
		Expect(err).NotTo(HaveOccurred())

		idBytes, err := signer.Serialize()
		Expect(err).NotTo(HaveOccurred())

		statusRequest := &gateway.CommitStatusRequest{
			ChannelId:     "testchannel",
			Identity:      idBytes,
			TransactionId: transactionID,
		}
		statusRequestBytes, err := proto.Marshal(statusRequest)
		Expect(err).NotTo(HaveOccurred())

		signature, err := signer.Sign(statusRequestBytes)
		Expect(err).NotTo(HaveOccurred())

		signedStatusRequest := &gateway.SignedCommitStatusRequest{
			Request:   statusRequestBytes,
			Signature: signature,
		}

		_, err = gatewayClient.CommitStatus(ctx, signedStatusRequest)
		Expect(err).NotTo(HaveOccurred())
	}

	evaluateTransaction := func(
		gatewayClient gateway.GatewayClient,
		signer *nwo.SigningIdentity,
		channel string,
		chaincode string,
		transactionName string,
		arguments []string,
		transientData map[string][]byte,
	) *peer.Response {
		args := [][]byte{}
		for _, arg := range arguments {
			args = append(args, []byte(arg))
		}
		proposedTransaction, transactionID := NewProposedTransaction(
			signer,
			channel,
			chaincode,
			transactionName,
			transientData,
			args...,
		)

		evaluateRequest := &gateway.EvaluateRequest{
			TransactionId:       transactionID,
			ChannelId:           channel,
			ProposedTransaction: proposedTransaction,
		}

		ctx, cancel := context.WithTimeout(context.Background(), network.EventuallyTimeout)
		defer cancel()

		evaluateResponse, err := gatewayClient.Evaluate(ctx, evaluateRequest)
		Expect(err).NotTo(HaveOccurred())

		return evaluateResponse.GetResult()
	}

	It("SBE policy should cause extra endorsers to be selected", func() {
		conn := network.PeerClientConn(org1Peer0)
		defer conn.Close()
		gatewayClient := gateway.NewGatewayClient(conn)
		signingIdentity := network.PeerUserSigner(org1Peer0, "User1")

		// add org2 to SBE policy - requires endorsement from org3 peer (chaincode policy)
		submitTransaction(
			gatewayClient,
			signingIdentity,
			"testchannel",
			"sbecc",
			"addorgs",
			[]string{"pub", "Org2MSP"},
			nil,
			[]string{org3Peer0.ID()},
		)

		// add org1 to SBE policy - requires endorsement from org2 & org3 peers
		submitTransaction(
			gatewayClient,
			signingIdentity,
			"testchannel",
			"sbecc",
			"addorgs",
			[]string{"pub", "Org1MSP"},
			nil,
			[]string{org2Peer0.ID(), org3Peer0.ID()},
		)

		// remove org2 to SBE policy - requires endorsement from org1, org2 & org3 peers
		submitTransaction(
			gatewayClient,
			signingIdentity,
			"testchannel",
			"sbecc",
			"delorgs",
			[]string{"pub", "Org2MSP"},
			nil,
			[]string{org1Peer0.ID(), org2Peer0.ID(), org3Peer0.ID()},
		)
	})

	It("Writing to private collections should select endorsers based on collection policy", func() {
		conn := network.PeerClientConn(org3Peer0)
		defer conn.Close()
		gatewayClient := gateway.NewGatewayClient(conn)
		signingIdentity := network.PeerUserSigner(org3Peer0, "User1")

		// write to private collection - requires endorsement from org2 peer (collection policy)
		submitTransaction(
			gatewayClient,
			signingIdentity,
			"testchannel",
			"sbecc",
			"setval",
			[]string{"priv", "gateway test value"},
			nil,
			[]string{org2Peer0.ID()},
		)

		// check the value was set
		result := evaluateTransaction(
			gatewayClient,
			signingIdentity,
			"testchannel",
			"sbecc",
			"getval",
			[]string{"priv"},
			nil,
		)

		Expect(result.Payload).To(Equal([]byte("gateway test value")))
	})

	It("should endorse chaincode on org3 and also seek endorsement from another org for cc2cc call", func() {
		conn := network.PeerClientConn(org3Peer0)
		defer conn.Close()
		gatewayClient := gateway.NewGatewayClient(conn)
		signingIdentity := network.PeerUserSigner(org3Peer0, "User1")

		submitTransaction(
			gatewayClient,
			signingIdentity,
			"testchannel",
			"sbecc",
			"cc2cc",
			[]string{"testchannel", "gatewaycc", "invoke", "a", "b", "10"},
			nil,
			[]string{org1Peer0.ID(), org3Peer0.ID()},
		)

		// check the transaction really worked - query via cc2cc call
		result := evaluateTransaction(
			gatewayClient,
			signingIdentity,
			"testchannel",
			"sbecc",
			"cc2cc",
			[]string{"testchannel", "gatewaycc", "query", "a"},
			nil,
		)

		Expect(result.Payload).To(Equal([]byte("90")))
	})
})
