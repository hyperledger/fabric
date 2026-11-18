/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msp

import (
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/asn1"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/hyperledger/fabric-lib-go/bccsp"
	"github.com/hyperledger/fabric-lib-go/bccsp/sw"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	mspprotos "github.com/hyperledger/fabric-protos-go-apiv2/msp"
	peer "github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/hyperledger/fabric/common/capabilities"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	oqs "github.com/hyperledger/fabric/pq-crypto"
	"github.com/hyperledger/fabric/protoutil"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
	"google.golang.org/protobuf/proto"
)

var _ = Describe("Pactway hybrid transaction signature rejection", func() {
	It("rejects a client proposal with an invalid PQ signature", func() {
		testDir, err := os.MkdirTemp("", "hybrid-invalid-signature")
		Expect(err).NotTo(HaveOccurred())
		oldProfile, oldProfileSet := os.LookupEnv("PACTWAY_HYBRID_PROFILE")
		oldDevelopment, oldDevelopmentSet := os.LookupEnv("PACTWAY_ISOLATED_DEVELOPMENT")
		Expect(os.Setenv("PACTWAY_HYBRID_PROFILE", oqs.TemporaryHybridProfileV1)).To(Succeed())
		Expect(os.Setenv("PACTWAY_ISOLATED_DEVELOPMENT", "true")).To(Succeed())

		var network *nwo.Network
		var ordererProcess, peerProcess ifrit.Process
		DeferCleanup(func() {
			for _, process := range []ifrit.Process{peerProcess, ordererProcess} {
				if process != nil {
					process.Signal(syscall.SIGTERM)
					Eventually(process.Wait(), network.EventuallyTimeout).Should(Receive())
				}
			}
			if network != nil {
				network.Cleanup()
			}
			Expect(os.RemoveAll(testDir)).To(Succeed())
			restoreEnvironment("PACTWAY_HYBRID_PROFILE", oldProfile, oldProfileSet)
			restoreEnvironment("PACTWAY_ISOLATED_DEVELOPMENT", oldDevelopment, oldDevelopmentSet)
		})

		network = nwo.New(nwo.BasicEtcdRaft(), testDir, nil, StartPort(), components)
		ordererOrg := network.Organization("OrdererOrg")
		ordererOrg.MSPID = "OrdererOrgMSP"
		ordererOrg.EnableNodeOUs = true
		network.Profiles[0].ChannelCapabilities = []string{"V3_0", capabilities.PactwayHybridCertificateV1}
		network.GenerateConfigTree()
		sess, err := network.Cryptogen(commands.Generate{
			Config: network.CryptoConfigPath(),
			Output: network.CryptoPath(),
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))

		configDir, err := filepath.Abs(filepath.Join("..", "..", "sampleconfig"))
		Expect(err).NotTo(HaveOccurred())
		output := filepath.Join(testDir, "hybrid-profile")
		generateHybridNetwork(output, configDir)
		installHybridNetworkMSPs(network, output)
		for _, channel := range network.Channels {
			sess, err := network.ConfigTxGen(commands.OutputBlock{
				ChannelID: channel.Name, Profile: channel.Profile, ConfigPath: network.RootDir,
				OutputBlock: network.OutputBlockPath(channel.Name),
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
		}
		network.ConcatenateTLSCACertificates()

		orderer := network.Orderer("orderer")
		org1Peer := network.Peer("Org1", "peer0")
		org2Peer := network.Peer("Org2", "peer0")
		runner, startedOrderer, startedPeers := network.StartSingleOrdererNetwork(orderer.Name)
		ordererProcess, peerProcess = startedOrderer, startedPeers
		nwo.JoinOrdererJoinPeersAppChannel(network, "testchannel", orderer, runner)
		nwo.EnableCapabilities(network, "testchannel", "Application", "V2_0", orderer, org1Peer, org2Peer)

		userMSPDir := network.PeerUserMSPDir(org1Peer, "User1")
		keyStoreDir := filepath.Join(userMSPDir, "keystore")
		keyStore, err := sw.NewFileBasedKeyStore(nil, keyStoreDir, true)
		Expect(err).NotTo(HaveOccurred())
		csp, err := sw.NewDefaultSecurityLevelWithKeystore(keyStore)
		Expect(err).NotTo(HaveOccurred())
		certificatePEM, err := os.ReadFile(filepath.Join(userMSPDir, "signcerts", "cert.pem"))
		Expect(err).NotTo(HaveOccurred())
		certificateBlock, _ := pem.Decode(certificatePEM)
		Expect(certificateBlock).NotTo(BeNil())
		certificate, err := x509.ParseCertificate(certificateBlock.Bytes)
		Expect(err).NotTo(HaveOccurred())
		classicalKey := replayPrivateKey(csp, keyStoreDir, certificate.RawSubjectPublicKeyInfo)
		serializedIdentity, err := proto.Marshal(&mspprotos.SerializedIdentity{
			Mspid:   "Org1MSP",
			IdBytes: certificatePEM,
		})
		Expect(err).NotTo(HaveOccurred())

		conn := network.PeerClientConn(org1Peer)
		defer conn.Close()
		invocation := &peer.ChaincodeInvocationSpec{
			ChaincodeSpec: &peer.ChaincodeSpec{
				Type:        peer.ChaincodeSpec_GOLANG,
				ChaincodeId: &peer.ChaincodeID{Name: "not-installed"},
				Input:       &peer.ChaincodeInput{Args: [][]byte{[]byte("invoke")}},
			},
		}
		proposal, _, err := protoutil.CreateChaincodeProposal(
			common.HeaderType_ENDORSER_TRANSACTION, "testchannel", invocation, serializedIdentity,
		)
		Expect(err).NotTo(HaveOccurred())
		signer := invalidPQProposalSigner{serialized: serializedIdentity, csp: csp, classicalKey: classicalKey}
		signedProposal, err := protoutil.GetSignedProposal(proposal, signer)
		Expect(err).NotTo(HaveOccurred())
		var signature invalidHybridSignature
		rest, err := asn1.Unmarshal(signedProposal.GetSignature(), &signature)
		Expect(err).NotTo(HaveOccurred())
		Expect(rest).To(BeEmpty())
		proposalDigest := sha256.Sum256(signedProposal.GetProposalBytes())
		hybridDigestInput := append(append([]byte{}, proposalDigest[:]...), signature.QuantumSign.RightAlign()...)
		hybridDigest := sha512.Sum384(hybridDigestInput)
		classicalPublicKey, ok := certificate.PublicKey.(*ecdsa.PublicKey)
		Expect(ok).To(BeTrue())
		Expect(ecdsa.VerifyASN1(
			classicalPublicKey, hybridDigest[:], signature.ClassicalSign.RightAlign(),
		)).To(BeTrue(), "the classical signature component must be valid")

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		response, err := peer.NewEndorserClient(conn).ProcessProposal(ctx, signedProposal)
		if err != nil {
			Expect(strings.ToLower(err.Error())).To(ContainSubstring("access denied"))
			Expect(err.Error()).To(ContainSubstring("Org1MSP"))
			return
		}
		Expect(response).NotTo(BeNil())
		Expect(response.GetResponse()).NotTo(BeNil())
		Expect(response.GetResponse().GetStatus()).NotTo(Equal(int32(200)))
		Expect(strings.ToLower(response.GetResponse().GetMessage())).To(ContainSubstring("quantum signature"))
	})
})

type invalidPQProposalSigner struct {
	serialized   []byte
	csp          bccsp.BCCSP
	classicalKey bccsp.Key
}

func (s invalidPQProposalSigner) Serialize() ([]byte, error) {
	return s.serialized, nil
}

func (s invalidPQProposalSigner) Sign(message []byte) ([]byte, error) {
	digest := sha256.Sum256(message)
	invalidPQKey, err := s.csp.KeyGen(&bccsp.OQSKeyGenOpts{
		Temporary:       true,
		SignatureScheme: string(oqs.MLDSA65ProfileV1),
	})
	if err != nil {
		return nil, err
	}
	pqSignature, err := s.csp.Sign(invalidPQKey, digest[:], nil)
	if err != nil {
		return nil, err
	}
	hybridDigestInput := append(append([]byte{}, digest[:]...), pqSignature...)
	hybridDigest := sha512.Sum384(hybridDigestInput)
	classicalSignature, err := s.csp.Sign(s.classicalKey, hybridDigest[:], nil)
	if err != nil {
		return nil, err
	}
	return asn1.Marshal(invalidHybridSignature{
		ClassicalSign: asn1.BitString{Bytes: classicalSignature, BitLength: len(classicalSignature) * 8},
		QuantumSign:   asn1.BitString{Bytes: pqSignature, BitLength: len(pqSignature) * 8},
	})
}

type invalidHybridSignature struct {
	ClassicalSign asn1.BitString
	QuantumSign   asn1.BitString
}
