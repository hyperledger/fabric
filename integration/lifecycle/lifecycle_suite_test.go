/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	pcommon "github.com/hyperledger/fabric-protos-go/common"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/integration"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	ic "github.com/hyperledger/fabric/internal/peer/chaincode"
	"github.com/hyperledger/fabric/internal/peer/common"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protoutil"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/pkg/errors"
	"github.com/tedsuo/ifrit"
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

func RestartNetwork(process *ifrit.Process, network *nwo.Network) {
	(*process).Signal(syscall.SIGTERM)
	Eventually((*process).Wait(), network.EventuallyTimeout).Should(Receive())
	networkRunner := network.NetworkGroupRunner()
	*process = ifrit.Invoke(networkRunner)
	Eventually((*process).Ready(), network.EventuallyTimeout).Should(BeClosed())
}

func LoadLocalMSPAt(dir, id, mspType string) (msp.MSP, error) {
	if mspType != "bccsp" {
		return nil, errors.Errorf("invalid msp type, expected 'bccsp', got %s", mspType)
	}
	conf, err := msp.GetLocalMspConfig(dir, nil, id)
	if err != nil {
		return nil, err
	}
	ks, err := sw.NewFileBasedKeyStore(nil, filepath.Join(dir, "keystore"), true)
	if err != nil {
		return nil, err
	}
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	if err != nil {
		return nil, err
	}
	thisMSP, err := msp.NewBccspMspWithKeyStore(msp.MSPv1_0, ks, cryptoProvider)
	if err != nil {
		return nil, err
	}
	err = thisMSP.Setup(conf)
	if err != nil {
		return nil, err
	}
	return thisMSP, nil
}

func Signer(dir string) (msp.SigningIdentity, []byte) {
	MSP, err := LoadLocalMSPAt(dir, "Org1MSP", "bccsp")
	Expect(err).To(BeNil())
	Expect(MSP).NotTo(BeNil())
	Expect(err).To(BeNil(), "failed getting signer for %s", dir)
	signer, err := MSP.GetDefaultSigningIdentity()
	Expect(err).To(BeNil())
	Expect(signer).NotTo(BeNil())
	serialisedSigner, err := signer.Serialize()
	Expect(err).To(BeNil())
	Expect(serialisedSigner).NotTo(BeNil())

	return signer, serialisedSigner
}

func SignedProposal(channel, chaincode string, signer msp.SigningIdentity, serialisedSigner []byte, args ...string) (*pb.SignedProposal, *pb.Proposal, string) {
	byteArgs := make([][]byte, 0, len(args))
	for _, arg := range args {
		byteArgs = append(byteArgs, []byte(arg))
	}

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
		serialisedSigner,
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

func grpcClient(tlsRootCertFile string) *comm.GRPCClient {
	caPEM, res := ioutil.ReadFile(tlsRootCertFile)
	Expect(res).To(BeNil())

	gClient, err := comm.NewGRPCClient(comm.ClientConfig{
		Timeout: 3 * time.Second,
		KaOpts:  comm.DefaultKeepaliveOptions,
		SecOpts: comm.SecureOptions{
			UseTLS:        true,
			ServerRootCAs: [][]byte{caPEM},
		},
	})
	Expect(err).NotTo(HaveOccurred())
	Expect(gClient).NotTo(BeNil())

	return gClient
}

func EndorserClient(address, tlsRootCertFile string) pb.EndorserClient {
	peerClient := &common.PeerClient{
		CommonClient: common.CommonClient{
			GRPCClient: grpcClient(tlsRootCertFile),
			Address:    address,
		},
	}

	ec, err := peerClient.Endorser()
	Expect(err).NotTo(HaveOccurred())
	Expect(ec).NotTo(BeNil())

	return ec
}

func OrdererClient(address, tlsRootCertFile string) common.BroadcastClient {
	ordererClient := &common.OrdererClient{
		CommonClient: common.CommonClient{
			GRPCClient: grpcClient(tlsRootCertFile),
			Address:    address,
		},
	}

	bc, err := ordererClient.Broadcast()
	Expect(err).NotTo(HaveOccurred())
	Expect(bc).NotTo(BeNil())

	return &common.BroadcastGRPCClient{Client: bc}
}

func DeliverClient(address, tlsRootCertFile string) pb.DeliverClient {
	peerClient := &common.PeerClient{
		CommonClient: common.CommonClient{
			GRPCClient: grpcClient(tlsRootCertFile),
			Address:    address,
		},
	}

	pd, err := peerClient.PeerDeliver()
	Expect(err).NotTo(HaveOccurred())
	Expect(pd).NotTo(BeNil())

	return pd
}

func CommitTx(network *nwo.Network, env *pcommon.Envelope, peer *nwo.Peer, dc pb.DeliverClient, oc common.BroadcastClient, signer msp.SigningIdentity, txid string) error {
	var cancelFunc context.CancelFunc
	ctx, cancelFunc := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelFunc()

	dg := ic.NewDeliverGroup([]pb.DeliverClient{dc}, []string{network.PeerAddress(peer, nwo.ListenPort)}, signer, tls.Certificate{}, "testchannel", txid)

	err := dg.Connect(ctx)
	Expect(err).NotTo(HaveOccurred())

	err = oc.Send(env)
	Expect(err).NotTo(HaveOccurred())

	return dg.Wait(ctx)
}

// GenerateOrgUpdateMeterials generates the necessary configtx and
// crypto materials for a new org's peers to join a network
func GenerateOrgUpdateMaterials(n *nwo.Network, peers ...*nwo.Peer) {
	orgUpdateNetwork := *n
	orgUpdateNetwork.Peers = peers
	orgUpdateNetwork.Templates = &nwo.Templates{
		ConfigTx: nwo.OrgUpdateConfigTxTemplate,
		Crypto:   nwo.OrgUpdateCryptoTemplate,
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
