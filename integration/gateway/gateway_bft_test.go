/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gateway

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"syscall"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/gateway"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/hyperledger/fabric/integration/channelparticipation"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
	ginkgomon "github.com/tedsuo/ifrit/ginkgomon_v2"
	"github.com/tedsuo/ifrit/grouper"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

var _ = Describe("GatewayService with BFT ordering service", func() {
	var (
		testDir          string
		network          *nwo.Network
		ordererProcesses map[string]ifrit.Process
		peerProcesses    ifrit.Process
		channel          = "testchannel1"
		peerGinkgoRunner []*ginkgomon.Runner
		ordererRunners   []*ginkgomon.Runner
	)

	BeforeEach(func() {
		var err error
		testDir, err = os.MkdirTemp("", "gateway")
		Expect(err).NotTo(HaveOccurred())

		client, err := docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())

		networkConfig := nwo.MultiNodeSmartBFT()
		network = nwo.New(networkConfig, testDir, client, StartPort(), components)

		network.GenerateConfigTree()
		network.Bootstrap()

		ordererProcesses = make(map[string]ifrit.Process)
		for _, orderer := range network.Orderers {
			runner := network.OrdererRunner(orderer,
				"ORDERER_GENERAL_BACKOFF_MAXDELAY=20s")
			ordererRunners = append(ordererRunners, runner)
			proc := ifrit.Invoke(runner)
			ordererProcesses[orderer.Name] = proc
			Eventually(proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
		}

		var peerGroupRunner ifrit.Runner
		peerGroupRunner, peerGinkgoRunner = peerGroupRunners(network)
		peerProcesses = ifrit.Invoke(peerGroupRunner)
		Eventually(peerProcesses.Ready(), network.EventuallyTimeout).Should(BeClosed())

		By("Joining orderers to channel")
		joinChannel(network, channel)

		By("Joining peers to channel")
		network.JoinChannel(channel, network.Orderers[0], network.PeersWithChannel(channel)...)

		orderer := network.Orderers[0]

		By("Deploying chaincode")
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
		nwo.DeployChaincode(network, channel, orderer, chaincode)
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

	It("Submit transaction", func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		org1Peer0 := network.Peer("Org1", "peer0")
		conn := network.PeerClientConn(org1Peer0)
		defer conn.Close()
		gw := gateway.NewGatewayClient(conn)
		signer := network.PeerUserSigner(org1Peer0, "User1")

		By("Submitting a new transaction 1")
		submitRequest := prepareTransaction(ctx, gw, signer, channel, "gatewaycc", "invoke", []string{"a", "b", "10"}, network.EventuallyTimeout)
		err := submitWithTimeout(ctx, gw, submitRequest, network.EventuallyTimeout)
		Expect(err).NotTo(HaveOccurred())

		waitForCommit(ctx, gw, signer, channel, submitRequest.TransactionId, network.EventuallyTimeout)

		By("Checking the ledger state 1")
		result := evaluateTransaction(ctx, gw, signer, channel, "gatewaycc", "query", []string{"a"}, network.EventuallyTimeout)
		Expect(result.Payload).To(Equal([]byte("90")))

		By("Resubmitting the same transaction 1")
		err = submitWithTimeout(ctx, gw, submitRequest, network.EventuallyTimeout)
		Expect(err).To(HaveOccurred())
		rpcErr := status.Convert(err)
		Expect(rpcErr.Message()).To(Equal("insufficient number of orderers could successfully process transaction to satisfy quorum requirement"))
		Expect(len(rpcErr.Details())).To(BeNumerically(">", 0))
		Expect(rpcErr.Details()[0].(*gateway.ErrorDetail).Message).To(Equal("received unsuccessful response from orderer: status=SERVICE_UNAVAILABLE, info=failed to submit request: request already processed"))

		By("Shutting down orderer2")
		ordererProcesses["orderer2"].Signal(syscall.SIGTERM)
		Eventually(ordererProcesses["orderer2"].Wait(), network.EventuallyTimeout).Should(Receive())

		By("Submitting a new transaction 2")
		submitRequest = prepareTransaction(ctx, gw, signer, channel, "gatewaycc", "invoke", []string{"a", "b", "10"}, network.EventuallyTimeout)
		err = submitWithTimeout(ctx, gw, submitRequest, network.EventuallyTimeout)
		Expect(err).NotTo(HaveOccurred())

		waitForCommit(ctx, gw, signer, channel, submitRequest.TransactionId, network.EventuallyTimeout*2)

		By("Checking the ledger state 2")
		result = evaluateTransaction(ctx, gw, signer, channel, "gatewaycc", "query", []string{"a"}, network.EventuallyTimeout)
		Expect(result.Payload).To(Equal([]byte("80")))

		By("Shutting down orderer1 - no longer quorate")
		ordererProcesses["orderer1"].Signal(syscall.SIGTERM)
		Eventually(ordererProcesses["orderer1"].Wait(), network.EventuallyTimeout).Should(Receive())

		By("Submitting a new transaction 3")
		submitRequest = prepareTransaction(ctx, gw, signer, channel, "gatewaycc", "invoke", []string{"a", "b", "10"}, network.EventuallyTimeout)
		err = submitWithTimeout(ctx, gw, submitRequest, network.EventuallyTimeout)
		Expect(err).To(HaveOccurred())
		rpcErr = status.Convert(err)
		Expect(rpcErr.Message()).To(Equal("insufficient number of orderers could successfully process transaction to satisfy quorum requirement"))

		peerLog := peerGinkgoRunner[0].Err().Contents()
		lastDateTime := scanLastDateTimeInLog(peerLog)
		// move cursor to end of log
		Eventually(peerGinkgoRunner[0].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say(lastDateTime))

		By("Restarting orderer2")
		runner := network.OrdererRunner(network.Orderers[1],
			"ORDERER_GENERAL_BACKOFF_MAXDELAY=20s")
		ordererRunners[1] = runner
		ordererProcesses["orderer2"] = ifrit.Invoke(runner)
		Eventually(ordererProcesses["orderer2"].Ready(), network.EventuallyTimeout).Should(BeClosed())
		// wait for peer to connect to orderer2
		Eventually(peerGinkgoRunner[0].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("picks a new address \"127.0.0.1:22005\" to connect\n.*Subchannel Connectivity change to READY"))
		Eventually(ordererRunners[1].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Starting view with number 0"))
		// awaiting the selection of a new leader
		Eventually(ordererRunners[1].Err(), network.EventuallyTimeout*2, time.Second).Should(gbytes.Say("Starting view with number"))

		By("Resubmitting the same transaction 2")
		err = submitWithTimeout(ctx, gw, submitRequest, network.EventuallyTimeout)
		Expect(err).NotTo(HaveOccurred())
		waitForCommit(ctx, gw, signer, channel, submitRequest.TransactionId, network.EventuallyTimeout*3)

		By("Checking the ledger state 3")
		result = evaluateTransaction(ctx, gw, signer, channel, "gatewaycc", "query", []string{"a"}, network.EventuallyTimeout)
		Expect(result.Payload).To(Equal([]byte("70")))

		By("Submitting a new transaction 4")
		submitRequest = prepareTransaction(ctx, gw, signer, channel, "gatewaycc", "invoke", []string{"a", "b", "10"}, network.EventuallyTimeout)
		err = submitWithTimeout(ctx, gw, submitRequest, network.EventuallyTimeout)
		Expect(err).NotTo(HaveOccurred())

		waitForCommit(ctx, gw, signer, channel, submitRequest.TransactionId, network.EventuallyTimeout)

		By("Checking the ledger state 4")
		result = evaluateTransaction(ctx, gw, signer, channel, "gatewaycc", "query", []string{"a"}, network.EventuallyTimeout)
		Expect(result.Payload).To(Equal([]byte("60")))
	})
})

func scanLastDateTimeInLog(data []byte) string {
	last := bytes.LastIndexAny(data, "\n\r") + 1
	data = data[:last]
	// Remove empty lines (strip EOL chars)
	data = bytes.TrimRight(data, "\n\r")
	// We have no non-empty lines, so advance but do not return a token.
	if len(data) == 0 {
		return ""
	}

	token := data[bytes.LastIndexAny(data, "\n\r")+1:]
	token = token[5:29]
	return string(token)
}

func submitWithTimeout(ctx context.Context, gw gateway.GatewayClient, submitRequest *gateway.SubmitRequest, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	_, err := gw.Submit(ctx, submitRequest)
	return err
}

func prepareTransaction(
	ctx context.Context,
	gatewayClient gateway.GatewayClient,
	signer *nwo.SigningIdentity,
	channel string,
	chaincode string,
	transactionName string,
	arguments []string,
	timeout time.Duration,
) *gateway.SubmitRequest {
	args := [][]byte{}
	for _, arg := range arguments {
		args = append(args, []byte(arg))
	}
	proposedTransaction, transactionID := NewProposedTransaction(
		signer,
		channel,
		chaincode,
		transactionName,
		nil,
		args...,
	)

	endorseRequest := &gateway.EndorseRequest{
		TransactionId:       transactionID,
		ChannelId:           channel,
		ProposedTransaction: proposedTransaction,
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	endorseResponse, err := gatewayClient.Endorse(ctx, endorseRequest)
	Expect(err).NotTo(HaveOccurred())

	preparedTransaction := endorseResponse.GetPreparedTransaction()
	preparedTransaction.Signature, err = signer.Sign(preparedTransaction.Payload)
	Expect(err).NotTo(HaveOccurred())

	return &gateway.SubmitRequest{
		TransactionId:       transactionID,
		ChannelId:           channel,
		PreparedTransaction: preparedTransaction,
	}
}

func waitForCommit(
	ctx context.Context,
	gatewayClient gateway.GatewayClient,
	signer *nwo.SigningIdentity,
	channel string,
	transactionId string,
	timeout time.Duration,
) {
	idBytes, err := signer.Serialize()
	Expect(err).NotTo(HaveOccurred())

	statusRequest := &gateway.CommitStatusRequest{
		ChannelId:     channel,
		Identity:      idBytes,
		TransactionId: transactionId,
	}
	statusRequestBytes, err := proto.Marshal(statusRequest)
	Expect(err).NotTo(HaveOccurred())

	signature, err := signer.Sign(statusRequestBytes)
	Expect(err).NotTo(HaveOccurred())

	signedStatusRequest := &gateway.SignedCommitStatusRequest{
		Request:   statusRequestBytes,
		Signature: signature,
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	statusResponse, err := gatewayClient.CommitStatus(ctx, signedStatusRequest)
	Expect(err).NotTo(HaveOccurred())
	Expect(statusResponse.Result).To(Equal(peer.TxValidationCode_VALID))
}

func evaluateTransaction(
	ctx context.Context,
	gatewayClient gateway.GatewayClient,
	signer *nwo.SigningIdentity,
	channel string,
	chaincode string,
	transactionName string,
	arguments []string,
	timeout time.Duration,
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
		nil,
		args...,
	)

	evaluateRequest := &gateway.EvaluateRequest{
		TransactionId:       transactionID,
		ChannelId:           channel,
		ProposedTransaction: proposedTransaction,
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	evaluateResponse, err := gatewayClient.Evaluate(ctx, evaluateRequest)
	Expect(err).NotTo(HaveOccurred())

	return evaluateResponse.GetResult()
}

func peerGroupRunners(n *nwo.Network) (ifrit.Runner, []*ginkgomon.Runner) {
	runners := []*ginkgomon.Runner{}
	members := grouper.Members{}
	for _, p := range n.Peers {
		runner := n.PeerRunner(p)
		members = append(members, grouper.Member{Name: p.ID(), Runner: runner})
		runners = append(runners, runner)
	}
	return grouper.NewParallel(syscall.SIGTERM, members), runners
}

func joinChannel(network *nwo.Network, channel string) {
	sess, err := network.ConfigTxGen(commands.OutputBlock{
		ChannelID:   channel,
		Profile:     network.Profiles[0].Name,
		ConfigPath:  network.RootDir,
		OutputBlock: network.OutputBlockPath(channel),
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))

	genesisBlockBytes, err := os.ReadFile(network.OutputBlockPath(channel))
	Expect(err).NotTo(HaveOccurred())

	genesisBlock := &common.Block{}
	err = proto.Unmarshal(genesisBlockBytes, genesisBlock)
	Expect(err).NotTo(HaveOccurred())

	expectedChannelInfoPT := channelparticipation.ChannelInfo{
		Name:              channel,
		URL:               "/participation/v1/channels/" + channel,
		Status:            "active",
		ConsensusRelation: "consenter",
		Height:            1,
	}

	for _, o := range network.Orderers {
		By("joining " + o.Name + " to channel as a consenter")
		channelparticipation.Join(network, o, channel, genesisBlock, expectedChannelInfoPT)
		channelInfo := channelparticipation.ListOne(network, o, channel)
		Expect(channelInfo).To(Equal(expectedChannelInfoPT))
	}
}
