/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"testing"
	"time"

	pcommon "github.com/hyperledger/fabric-protos-go/common"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/integration"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	"github.com/hyperledger/fabric/integration/nwo/template"
	"github.com/hyperledger/fabric/protoutil"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

func TestLifecycle(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Lifecycle Suite")
}

var (
	buildServer *nwo.BuildServer
	components  *nwo.Components
)

var _ = SynchronizedBeforeSuite(func() []byte {
	buildServer = nwo.NewBuildServer()
	buildServer.Serve()

	components = buildServer.Components()
	payload, err := json.Marshal(components)
	Expect(err).NotTo(HaveOccurred())

	return payload
}, func(payload []byte) {
	err := json.Unmarshal(payload, &components)
	Expect(err).NotTo(HaveOccurred())

	flogging.SetWriter(GinkgoWriter)
})

var _ = SynchronizedAfterSuite(func() {
}, func() {
	buildServer.Shutdown()
})

func StartPort() int {
	return integration.LifecyclePort.StartPortForNode()
}

func RunQueryInvokeQuery(n *nwo.Network, orderer *nwo.Orderer, chaincodeName string, initialQueryResult int, peers ...*nwo.Peer) {
	if len(peers) == 0 {
		peers = n.PeersWithChannel("testchannel")
	}

	addresses := make([]string, len(peers))
	for i, peer := range peers {
		addresses[i] = n.PeerAddress(peer, nwo.ListenPort)
	}

	By("querying the chaincode")
	sess, err := n.PeerUserSession(peers[0], "User1", commands.ChaincodeQuery{
		ChannelID: "testchannel",
		Name:      chaincodeName,
		Ctor:      `{"Args":["query","a"]}`,
	})
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	EventuallyWithOffset(1, sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	ExpectWithOffset(1, sess).To(gbytes.Say(fmt.Sprint(initialQueryResult)))

	By("invoking the chaincode")
	sess, err = n.PeerUserSession(peers[0], "User1", commands.ChaincodeInvoke{
		ChannelID:     "testchannel",
		Orderer:       n.OrdererAddress(orderer, nwo.ListenPort),
		Name:          chaincodeName,
		Ctor:          `{"Args":["invoke","a","b","10"]}`,
		PeerAddresses: addresses,
		WaitForEvent:  true,
	})
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	EventuallyWithOffset(1, sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	ExpectWithOffset(1, sess.Err).To(gbytes.Say("Chaincode invoke successful. result: status:200"))

	By("querying the chaincode")
	sess, err = n.PeerUserSession(peers[0], "User1", commands.ChaincodeQuery{
		ChannelID: "testchannel",
		Name:      chaincodeName,
		Ctor:      `{"Args":["query","a"]}`,
	})
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	EventuallyWithOffset(1, sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	ExpectWithOffset(1, sess).To(gbytes.Say(fmt.Sprint(initialQueryResult - 10)))
}

func SignedProposal(channel, chaincode string, signer *nwo.SigningIdentity, args ...string) (*pb.SignedProposal, *pb.Proposal, string) {
	byteArgs := make([][]byte, 0, len(args))
	for _, arg := range args {
		byteArgs = append(byteArgs, []byte(arg))
	}
	serializedSigner, err := signer.Serialize()
	Expect(err).NotTo(HaveOccurred())

	prop, txid, err := protoutil.CreateChaincodeProposalWithTxIDAndTransient(
		pcommon.HeaderType_ENDORSER_TRANSACTION,
		channel,
		&pb.ChaincodeInvocationSpec{
			ChaincodeSpec: &pb.ChaincodeSpec{
				Type: pb.ChaincodeSpec_GOLANG,
				ChaincodeId: &pb.ChaincodeID{
					Name: chaincode,
				},
				Input: &pb.ChaincodeInput{
					Args: byteArgs,
				},
			},
		},
		serializedSigner,
		"",
		nil,
	)
	Expect(err).NotTo(HaveOccurred())
	Expect(prop).NotTo(BeNil())

	signedProp, err := protoutil.GetSignedProposal(prop, signer)
	Expect(err).NotTo(HaveOccurred())
	Expect(signedProp).NotTo(BeNil())

	return signedProp, prop, txid
}

func CommitTx(nw *nwo.Network, tx *pcommon.Envelope, peer *nwo.Peer, dc pb.DeliverClient, oc ab.AtomicBroadcast_BroadcastClient, signer *nwo.SigningIdentity, txid string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	df, err := dc.DeliverFiltered(ctx)
	Expect(err).NotTo(HaveOccurred())
	defer df.CloseSend()

	deliverEnvelope, err := protoutil.CreateSignedEnvelope(
		pcommon.HeaderType_DELIVER_SEEK_INFO,
		"testchannel",
		signer,
		&ab.SeekInfo{
			Behavior: ab.SeekInfo_BLOCK_UNTIL_READY,
			Start: &ab.SeekPosition{
				Type: &ab.SeekPosition_Newest{Newest: &ab.SeekNewest{}},
			},
			Stop: &ab.SeekPosition{
				Type: &ab.SeekPosition_Specified{
					Specified: &ab.SeekSpecified{Number: math.MaxUint64},
				},
			},
		},
		0,
		0,
	)
	Expect(err).NotTo(HaveOccurred())
	err = df.Send(deliverEnvelope)
	Expect(err).NotTo(HaveOccurred())

	err = oc.Send(tx)
	Expect(err).NotTo(HaveOccurred())

	for {
		resp, err := df.Recv()
		if err != nil {
			return err
		}
		fb, ok := resp.Type.(*pb.DeliverResponse_FilteredBlock)
		if !ok {
			return fmt.Errorf("unexpected filtered block, received %T", resp.Type)
		}
		for _, tx := range fb.FilteredBlock.FilteredTransactions {
			if tx.Txid != txid {
				continue
			}
			if tx.TxValidationCode != pb.TxValidationCode_VALID {
				return fmt.Errorf("transaction invalidated with status (%s)", tx.TxValidationCode)
			}
			return nil
		}
	}
}

// GenerateOrgUpdateMeterials generates the necessary configtx and
// crypto materials for a new org's peers to join a network
func GenerateOrgUpdateMaterials(n *nwo.Network, peers ...*nwo.Peer) {
	orgUpdateNetwork := *n
	orgUpdateNetwork.Peers = peers
	orgUpdateNetwork.Templates = &nwo.Templates{
		ConfigTx: template.OrgUpdateConfigTxTemplate,
		Crypto:   template.OrgUpdateCryptoTemplate,
		Core:     n.Templates.CoreTemplate(),
	}

	orgUpdateNetwork.GenerateConfigTxConfig()
	for _, peer := range peers {
		orgUpdateNetwork.GenerateCoreConfig(peer)
	}

	orgUpdateNetwork.GenerateCryptoConfig()
	sess, err := orgUpdateNetwork.Cryptogen(commands.Generate{
		Config: orgUpdateNetwork.CryptoConfigPath(),
		Output: orgUpdateNetwork.CryptoPath(),
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, orgUpdateNetwork.EventuallyTimeout).Should(gexec.Exit(0))

	// refresh TLSCACertificates ca-certs.pem so that it includes
	// the newly generated cert
	n.ConcatenateTLSCACertificates()
}
