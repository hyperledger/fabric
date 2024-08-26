/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package e2e

import (
	"context"
	"crypto/rand"
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
	"github.com/hyperledger/fabric/protoutil"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"github.com/tedsuo/ifrit"
	ginkgomon "github.com/tedsuo/ifrit/ginkgomon_v2"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var _ = Describe("FilterProposalTimeWindow", func() {
	var (
		testDir                     string
		client                      *docker.Client
		network                     *nwo.Network
		chaincode                   nwo.Chaincode
		ordererRunner               *ginkgomon.Runner
		ordererProcess, peerProcess ifrit.Process
	)

	BeforeEach(func() {
		var err error
		testDir, err = os.MkdirTemp("", "e2e")
		Expect(err).NotTo(HaveOccurred())

		client, err = docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())

		chaincode = nwo.Chaincode{
			Name:            "mycc",
			Version:         "0.0",
			Path:            components.Build("github.com/hyperledger/fabric/integration/chaincode/simple/cmd"),
			Lang:            "binary",
			PackageFile:     filepath.Join(testDir, "simplecc.tar.gz"),
			Ctor:            `{"Args":["init","a","100","b","200"]}`,
			SignaturePolicy: `AND ('Org1MSP.member','Org2MSP.member')`,
			Sequence:        "1",
			InitRequired:    true,
			Label:           "my_prebuilt_chaincode",
		}
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

	BeforeEach(func() {
		network = nwo.New(nwo.BasicEtcdRaft(), testDir, client, StartPort(), components)
		network.GenerateConfigTree()
		network.Bootstrap()

		// Start all the fabric processes
		ordererRunner, ordererProcess, peerProcess = network.StartSingleOrdererNetwork("orderer")

		By("getting the orderer by name")
		orderer := network.Orderer("orderer")

		By("setting up the channel")
		channelparticipation.JoinOrdererJoinPeersAppChannel(network, "testchannel", orderer, ordererRunner)

		By("enabling new lifecycle capabilities")
		nwo.EnableCapabilities(network, "testchannel", "Application", "V2_5", orderer, network.Peer("Org1", "peer0"), network.Peer("Org2", "peer0"))

		By("deploying the chaincode")
		nwo.DeployChaincode(network, "testchannel", orderer, chaincode)

		By("getting the client peer by name")
		peer := network.Peer("Org1", "peer0")

		RunQueryInvokeQuery(network, orderer, peer, "testchannel")
	})

	It("filters proposal", func() {
		By("normal time")
		err := endorser(network, timestamppb.Now())
		Expect(err).NotTo(HaveOccurred())

		By("add 30 minute")
		err = endorser(network, timestamppb.New(time.Now().Add(time.Minute*30)))
		rpcErr := status.Convert(err)
		Expect(rpcErr.Message()).To(Equal("failed to collect enough transaction endorsements, see attached details for more info"))
		Expect(len(rpcErr.Details())).To(BeNumerically(">", 0))
		Expect(rpcErr.Details()[0].(*gateway.ErrorDetail).Message).To(ContainSubstring("request unauthorized due to incorrect timestamp"))

		By("sub 30 minute")
		err = endorser(network, timestamppb.New(time.Now().Add(-time.Minute*30)))
		rpcErr = status.Convert(err)
		Expect(rpcErr.Message()).To(Equal("failed to collect enough transaction endorsements, see attached details for more info"))
		Expect(len(rpcErr.Details())).To(BeNumerically(">", 0))
		Expect(rpcErr.Details()[0].(*gateway.ErrorDetail).Message).To(ContainSubstring("request unauthorized due to incorrect timestamp"))
	})
})

func endorser(network *nwo.Network, needTime *timestamppb.Timestamp) error {
	peerOrg1 := network.Peer("Org1", "peer0")
	signingIdentity := network.PeerUserSigner(peerOrg1, "User1")
	creator, err := signingIdentity.Serialize()
	Expect(err).NotTo(HaveOccurred())

	conn := network.PeerClientConn(peerOrg1)
	gatewayClient := gateway.NewGatewayClient(conn)

	invocationSpec := &peer.ChaincodeInvocationSpec{
		ChaincodeSpec: &peer.ChaincodeSpec{
			Type:        peer.ChaincodeSpec_NODE,
			ChaincodeId: &peer.ChaincodeID{Name: "mycc"},
			Input:       &peer.ChaincodeInput{Args: [][]byte{[]byte("invoke"), []byte("a"), []byte("b"), []byte("10")}},
		},
	}

	proposal, transactionID, err := createChaincodeProposalWithTransient(
		common.HeaderType_ENDORSER_TRANSACTION,
		"testchannel",
		invocationSpec,
		creator,
		nil,
		needTime,
	)
	Expect(err).NotTo(HaveOccurred())

	proposedTransaction, err := protoutil.GetSignedProposal(proposal, signingIdentity)
	Expect(err).NotTo(HaveOccurred())

	mspid := network.Organization(peerOrg1.Organization).MSPID

	endorseRequest := &gateway.EndorseRequest{
		TransactionId:          transactionID,
		ChannelId:              "testchannel",
		ProposedTransaction:    proposedTransaction,
		EndorsingOrganizations: []string{mspid},
	}

	ctx, cancel := context.WithTimeout(context.Background(), network.EventuallyTimeout)
	defer cancel()
	_, err = gatewayClient.Endorse(ctx, endorseRequest)

	return err
}

// createChaincodeProposalWithTransient creates a proposal from given input
// It returns the proposal and the transaction id associated to the proposal
func createChaincodeProposalWithTransient(typ common.HeaderType, channelID string, cis *peer.ChaincodeInvocationSpec, creator []byte, transientMap map[string][]byte, needTime *timestamppb.Timestamp) (*peer.Proposal, string, error) {
	// generate a random nonce
	nonce, err := getRandomNonce()
	if err != nil {
		return nil, "", err
	}

	// compute txid
	txid := protoutil.ComputeTxID(nonce, creator)

	return createChaincodeProposalWithTxIDNonceAndTransient(txid, typ, channelID, cis, nonce, creator, transientMap, needTime)
}

func getRandomNonce() ([]byte, error) {
	key := make([]byte, 24)

	_, err := rand.Read(key)
	if err != nil {
		return nil, errors.Wrap(err, "error getting random bytes")
	}
	return key, nil
}

// createChaincodeProposalWithTxIDNonceAndTransient creates a proposal from
// given input
func createChaincodeProposalWithTxIDNonceAndTransient(txid string, typ common.HeaderType, channelID string, cis *peer.ChaincodeInvocationSpec, nonce, creator []byte, transientMap map[string][]byte, needTime *timestamppb.Timestamp) (*peer.Proposal, string, error) {
	ccHdrExt := &peer.ChaincodeHeaderExtension{ChaincodeId: cis.ChaincodeSpec.ChaincodeId}
	ccHdrExtBytes, err := proto.Marshal(ccHdrExt)
	if err != nil {
		return nil, "", errors.Wrap(err, "error marshaling ChaincodeHeaderExtension")
	}

	cisBytes, err := proto.Marshal(cis)
	if err != nil {
		return nil, "", errors.Wrap(err, "error marshaling ChaincodeInvocationSpec")
	}

	ccPropPayload := &peer.ChaincodeProposalPayload{Input: cisBytes, TransientMap: transientMap}
	ccPropPayloadBytes, err := proto.Marshal(ccPropPayload)
	if err != nil {
		return nil, "", errors.Wrap(err, "error marshaling ChaincodeProposalPayload")
	}

	hdr := &common.Header{
		ChannelHeader: protoutil.MarshalOrPanic(
			&common.ChannelHeader{
				Type:      int32(typ),
				TxId:      txid,
				Timestamp: needTime,
				ChannelId: channelID,
				Extension: ccHdrExtBytes,
				Epoch:     0,
			},
		),
		SignatureHeader: protoutil.MarshalOrPanic(
			&common.SignatureHeader{
				Nonce:   nonce,
				Creator: creator,
			},
		),
	}

	hdrBytes, err := proto.Marshal(hdr)
	if err != nil {
		return nil, "", err
	}

	prop := &peer.Proposal{
		Header:  hdrBytes,
		Payload: ccPropPayloadBytes,
	}
	return prop, txid, nil
}
