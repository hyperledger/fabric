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
	"github.com/hyperledger/fabric/integration/channelparticipation"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/protoutil"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/tedsuo/ifrit"
	ginkgomon "github.com/tedsuo/ifrit/ginkgomon_v2"
)

var _ = Describe("GatewayService with endorser discovery", func() {
	var (
		testDir                     string
		network                     *nwo.Network
		orderer                     *nwo.Orderer
		org1Peer0                   *nwo.Peer
		org2Peer0                   *nwo.Peer
		org3Peer0                   *nwo.Peer
		ordererRunner               *ginkgomon.Runner
		ordererProcess, peerProcess ifrit.Process
		peerCerts                   map[string]string
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

		config := nwo.ThreeOrgEtcdRaftNoSysChan()
		network = nwo.New(config, testDir, client, StartPort(), components)

		network.GenerateConfigTree()
		network.Bootstrap()

		// Start all the fabric processes
		ordererRunner, ordererProcess, peerProcess = network.StartSingleOrdererNetwork("orderer")

		orderer = network.Orderer("orderer")
		channelparticipation.JoinOrdererJoinPeersAppChannel(network, "testchannel", orderer, ordererRunner)

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
			SignaturePolicy:   `OR ('Org3MSP.member')`,
			Sequence:          "1",
			InitRequired:      true,
			Label:             "sbecc_label",
			CollectionsConfig: filepath.Join("testdata", "collections_config_sbe.json"),
		}
		nwo.DeployChaincode(network, "testchannel", orderer, chaincode)

		chaincode = nwo.Chaincode{
			Name:              "readpvtcc",
			Version:           "0.0",
			Path:              components.Build("github.com/hyperledger/fabric/integration/chaincode/keylevelep/cmd"),
			Ctor:              `{"Args":["init"]}`,
			Lang:              "binary",
			PackageFile:       filepath.Join(testDir, "readpvtcc.tar.gz"),
			SignaturePolicy:   `OR ('Org1MSP.member', 'Org3MSP.member')`,
			Sequence:          "1",
			InitRequired:      true,
			Label:             "readpvtcc_label",
			CollectionsConfig: filepath.Join("testdata", "collections_config_read.json"),
		}
		nwo.DeployChaincode(network, "testchannel", orderer, chaincode)
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

		if expectedEndorsers != nil {
			verifyEndorsers(endorseResponse, expectedEndorsers)
		}

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

		statusResponse, err := gatewayClient.CommitStatus(ctx, signedStatusRequest)
		Expect(err).NotTo(HaveOccurred())
		Expect(statusResponse.Result).To(Equal(peer.TxValidationCode_VALID))

		chaincodeAction, err := protoutil.GetActionFromEnvelopeMsg(endorseResponse.GetPreparedTransaction())
		Expect(err).NotTo(HaveOccurred())

		return chaincodeAction.GetResponse()
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

	It("setting SBE policy should override chaincode policy", func() {
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

		// add org1 to SBE policy - requires endorsement from org2 peer (SBE policy)
		submitTransaction(
			gatewayClient,
			signingIdentity,
			"testchannel",
			"sbecc",
			"addorgs",
			[]string{"pub", "Org1MSP"},
			nil,
			[]string{org2Peer0.ID()},
		)

		// remove org2 to SBE policy - requires endorsement from org1, org2 peers (SBE policy)
		submitTransaction(
			gatewayClient,
			signingIdentity,
			"testchannel",
			"sbecc",
			"delorgs",
			[]string{"pub", "Org2MSP"},
			nil,
			[]string{org1Peer0.ID(), org2Peer0.ID()},
		)
	})

	It("setting SBE policy on private key then writing to should override the collection & chaincode policies", func() {
		conn := network.PeerClientConn(org1Peer0)
		defer conn.Close()
		gatewayClient := gateway.NewGatewayClient(conn)
		signingIdentity := network.PeerUserSigner(org1Peer0, "User1")

		// write to private collection - requires endorsement from org2 peer (collection policy)
		submitTransaction(
			gatewayClient,
			signingIdentity,
			"testchannel",
			"sbecc",
			"setval",
			[]string{"priv", "initial private value"},
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
		Expect(result.Payload).To(Equal([]byte("initial private value")))

		// add org1 to SBE policy - requires endorsement from org2 peer (collection policy)
		submitTransaction(
			gatewayClient,
			signingIdentity,
			"testchannel",
			"sbecc",
			"addorgs",
			[]string{"priv", "Org1MSP"},
			nil,
			[]string{org2Peer0.ID()},
		)

		// write to private collection - requires endorsement from org1 peer (SBE policy)
		submitTransaction(
			gatewayClient,
			signingIdentity,
			"testchannel",
			"sbecc",
			"setval",
			[]string{"priv", "updated private value"},
			nil,
			[]string{org1Peer0.ID()},
		)

		// check the value was set
		result = evaluateTransaction(
			gatewayClient,
			signingIdentity,
			"testchannel",
			"sbecc",
			"getval",
			[]string{"priv"},
			nil,
		)

		Expect(result.Payload).To(Equal([]byte("updated private value")))
	})

	It("writing to public and private keys should combine the collection & chaincode policies", func() {
		conn := network.PeerClientConn(org1Peer0)
		defer conn.Close()
		gatewayClient := gateway.NewGatewayClient(conn)
		signingIdentity := network.PeerUserSigner(org1Peer0, "User1")

		// write to both pub&priv - requires endorsement from org2 (collection policy) and org3 (chaincode policy)
		submitTransaction(
			gatewayClient,
			signingIdentity,
			"testchannel",
			"sbecc",
			"setval",
			[]string{"both", "initial value"},
			nil,
			[]string{org2Peer0.ID(), org3Peer0.ID()},
		)

		// check the private value was set
		result := evaluateTransaction(
			gatewayClient,
			signingIdentity,
			"testchannel",
			"sbecc",
			"getval",
			[]string{"priv"},
			nil,
		)
		Expect(result.Payload).To(Equal([]byte("initial value")))

		// check the public value was set
		result = evaluateTransaction(
			gatewayClient,
			signingIdentity,
			"testchannel",
			"sbecc",
			"getval",
			[]string{"pub"},
			nil,
		)
		Expect(result.Payload).To(Equal([]byte("initial value")))
	})

	It("should combine chaincode and private SBE policies", func() {
		conn := network.PeerClientConn(org1Peer0)
		defer conn.Close()
		gatewayClient := gateway.NewGatewayClient(conn)
		signingIdentity := network.PeerUserSigner(org1Peer0, "User1")

		// write to both public & private states - requires endorsement from org2 peer (collection policy) and org3 peer (chaincode) policy
		submitTransaction(
			gatewayClient,
			signingIdentity,
			"testchannel",
			"sbecc",
			"setval",
			[]string{"both", "initial private & public value"},
			nil,
			[]string{org2Peer0.ID(), org3Peer0.ID()},
		)

		// add org1 to SBE policy for private state - requires endorsement from org2 peer (collection policy)
		submitTransaction(
			gatewayClient,
			signingIdentity,
			"testchannel",
			"sbecc",
			"addorgs",
			[]string{"priv", "Org1MSP"},
			nil,
			[]string{org2Peer0.ID()},
		)

		// write to both pub&priv - requires endorsement from org1 (SBE policy) and org3 (chaincode policy)
		submitTransaction(
			gatewayClient,
			signingIdentity,
			"testchannel",
			"sbecc",
			"setval",
			[]string{"both", "chaincode and SBE policies"},
			nil,
			[]string{org1Peer0.ID(), org3Peer0.ID()},
		)

		// check the private value was set
		result := evaluateTransaction(
			gatewayClient,
			signingIdentity,
			"testchannel",
			"sbecc",
			"getval",
			[]string{"priv"},
			nil,
		)
		Expect(result.Payload).To(Equal([]byte("chaincode and SBE policies")))
	})

	It("should combine collection and public SBE policies", func() {
		conn := network.PeerClientConn(org3Peer0)
		defer conn.Close()
		gatewayClient := gateway.NewGatewayClient(conn)
		signingIdentity := network.PeerUserSigner(org3Peer0, "User1")

		// add org1 to SBE policy - requires endorsement from org3 peer (chaincode policy)
		submitTransaction(
			gatewayClient,
			signingIdentity,
			"testchannel",
			"sbecc",
			"addorgs",
			[]string{"pub", "Org1MSP"},
			nil,
			[]string{org3Peer0.ID()},
		)

		// write to both pub&priv - requires endorsement from org1 (SBE policy) and org2 (collection policy)
		submitTransaction(
			gatewayClient,
			signingIdentity,
			"testchannel",
			"sbecc",
			"setval",
			[]string{"both", "collection and SBE policies"},
			nil,
			[]string{org1Peer0.ID(), org2Peer0.ID()},
		)

		// check the private value was set
		result := evaluateTransaction(
			gatewayClient,
			signingIdentity,
			"testchannel",
			"sbecc",
			"getval",
			[]string{"pub"},
			nil,
		)
		Expect(result.Payload).To(Equal([]byte("collection and SBE policies")))
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

	It("reading private data should use the collection ownership policy, not signature policy", func() {
		conn := network.PeerClientConn(org1Peer0)
		defer conn.Close()
		gatewayClient := gateway.NewGatewayClient(conn)
		signingIdentity := network.PeerUserSigner(org1Peer0, "User1")

		submitTransaction(
			gatewayClient,
			signingIdentity,
			"testchannel",
			"readpvtcc",
			"setval",
			[]string{"priv", "abcd"},
			nil,
			nil, // don't care - not testing this part
		)

		result := submitTransaction(
			gatewayClient,
			signingIdentity,
			"testchannel",
			"readpvtcc",
			"getval",
			[]string{"priv"},
			nil,
			[]string{org1Peer0.ID()},
		)

		Expect(result.GetPayload()).To(Equal([]byte("abcd")))
	})
})
