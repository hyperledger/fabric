/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package e2e

import (
	"crypto/sha256"
	"encoding/hex"
	"io/ioutil"
	"os"
	"syscall"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset/kvrwset"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/policydsl"
	"github.com/hyperledger/fabric/integration/channelparticipation"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	"github.com/hyperledger/fabric/integration/ordererclient"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
	ginkgomon "github.com/tedsuo/ifrit/ginkgomon_v2"
)

var _ = Describe("InstantiationPolicy", func() {
	var (
		testDir                     string
		client                      *docker.Client
		network                     *nwo.Network
		ordererRunner               *ginkgomon.Runner
		ordererProcess, peerProcess ifrit.Process
	)

	BeforeEach(func() {
		var err error
		testDir, err = ioutil.TempDir("", "e2e")
		Expect(err).NotTo(HaveOccurred())

		client, err = docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())
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

	Describe("single node etcdraft network with single peer", func() {
		BeforeEach(func() {
			config := nwo.MinimalRaftNoSysChan()
			config.Profiles[0].Organizations = []string{"Org1", "Org2"}
			network = nwo.New(config, testDir, client, StartPort(), components)
			network.GenerateConfigTree()
			network.Bootstrap()

			// Start all the fabric processes
			ordererRunner, ordererProcess, peerProcess = network.StartSingleOrdererNetwork("orderer")
		})

		It("honors the instantiation policy", func() {
			orderer := network.Orderer("orderer")

			org1Peer := network.Peer("Org1", "peer0")
			org2Peer := network.Peer("Org2", "peer0")

			channelparticipation.JoinOrdererJoinPeersAppChannel(network, "testchannel", orderer, ordererRunner)

			By("attempting to deploy with an unsatisfied instantiation policy")

			goodDeploy := LSCCOperation{
				Operation: "deploy",
				ChannelID: "testchannel",
				Name:      "fakecc",
				Version:   "badip",
				InstantiationOrgs: []*nwo.Organization{
					network.Organization("Org2"),
				},
			}

			ordererclient.Broadcast(network, orderer, goodDeploy.Tx(network.PeerUserSigner(org1Peer, "Admin")))

			nwo.WaitUntilEqualLedgerHeight(network, "testchannel", 2, org1Peer)

			Expect(ListInstantiatedLegacy(network, org1Peer, "testchannel")).NotTo(gbytes.Say("Name: fakecc, Version: badip"))

			By("attempting to deploy with a satisfied instantiation policy")

			badDeploy := LSCCOperation{
				Operation: "deploy",
				ChannelID: "testchannel",
				Name:      "fakecc",
				Version:   "goodip",
				InstantiationOrgs: []*nwo.Organization{
					network.Organization("Org1"),
				},
			}

			ordererclient.Broadcast(network, orderer, badDeploy.Tx(network.PeerUserSigner(org1Peer, "Admin")))

			nwo.WaitUntilEqualLedgerHeight(network, "testchannel", 3, org1Peer)
			Expect(ListInstantiatedLegacy(network, org1Peer, "testchannel")).To(gbytes.Say("Name: fakecc, Version: goodip"))

			By("upgrading without satisfying the previous instantiation policy")

			badUpgrade := LSCCOperation{
				Operation: "upgrade",
				ChannelID: "testchannel",
				Name:      "fakecc",
				Version:   "wrongsubmitter",
				InstantiationOrgs: []*nwo.Organization{
					network.Organization("Org2"),
				},
			}

			ordererclient.Broadcast(network, orderer, badUpgrade.Tx(network.PeerUserSigner(org2Peer, "Admin")))

			nwo.WaitUntilEqualLedgerHeight(network, "testchannel", 4, org1Peer)
			Expect(ListInstantiatedLegacy(network, org1Peer, "testchannel")).NotTo(gbytes.Say("Name: fakecc, Version: wrongsubmitter"))

			By("upgrading while satisfying the previous instantiation policy")

			goodUpgrade := LSCCOperation{
				Operation: "upgrade",
				ChannelID: "testchannel",
				Name:      "fakecc",
				Version:   "rightsubmitter",
				InstantiationOrgs: []*nwo.Organization{
					network.Organization("Org1"),
				},
			}

			ordererclient.Broadcast(network, orderer, goodUpgrade.Tx(network.PeerUserSigner(org1Peer, "Admin")))

			nwo.WaitUntilEqualLedgerHeight(network, "testchannel", 5, org1Peer)
			Expect(ListInstantiatedLegacy(network, org1Peer, "testchannel")).To(gbytes.Say("Name: fakecc, Version: rightsubmitter"))
		})
	})
})

func ListInstantiatedLegacy(n *nwo.Network, p *nwo.Peer, channel string) *gbytes.Buffer {
	sess, err := n.PeerAdminSession(p, commands.ChaincodeListInstantiatedLegacy{
		ChannelID:  channel,
		ClientAuth: n.ClientAuthRequired,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	return sess.Buffer()
}

type LSCCOperation struct {
	Operation         string
	ChannelID         string
	Name              string
	Version           string
	InstantiationOrgs []*nwo.Organization
}

func (lo *LSCCOperation) Tx(signer *nwo.SigningIdentity) *common.Envelope {
	creatorBytes, err := signer.Serialize()
	Expect(err).NotTo(HaveOccurred())

	var instantiationMSPIDs []string
	for _, org := range lo.InstantiationOrgs {
		instantiationMSPIDs = append(instantiationMSPIDs, org.MSPID)
	}
	instantiationPolicy := policydsl.SignedByAnyMember(instantiationMSPIDs)

	nonce, err := time.Now().MarshalBinary()
	Expect(err).NotTo(HaveOccurred())

	timestamp, err := ptypes.TimestampProto(time.Now().UTC())
	Expect(err).NotTo(HaveOccurred())

	hasher := sha256.New()
	hasher.Write(nonce)
	hasher.Write(creatorBytes)
	txid := hex.EncodeToString(hasher.Sum(nil))

	signatureHeaderBytes := protoMarshal(&common.SignatureHeader{
		Creator: creatorBytes,
		Nonce:   nonce,
	})

	channelHeaderBytes := protoMarshal(&common.ChannelHeader{
		ChannelId: lo.ChannelID,
		Extension: protoMarshal(&peer.ChaincodeHeaderExtension{
			ChaincodeId: &peer.ChaincodeID{
				Name: "lscc",
			},
		}),
		Timestamp: timestamp,
		TxId:      txid,
		Type:      int32(common.HeaderType_ENDORSER_TRANSACTION),
	})

	chaincodeDataBytes := protoMarshal(&peer.ChaincodeData{
		Data:                []byte("a-big-bunch-of-fakery"),
		Id:                  []byte("a-friendly-fake-chaincode"),
		Escc:                "escc",
		Vscc:                "vscc",
		Name:                lo.Name,
		Version:             lo.Version,
		InstantiationPolicy: instantiationPolicy,
		Policy:              nil, // EndorsementPolicy deliberately left nil
	})

	proposalPayloadBytes := protoMarshal(&peer.ChaincodeProposalPayload{
		Input: protoMarshal(&peer.ChaincodeInvocationSpec{
			ChaincodeSpec: &peer.ChaincodeSpec{
				Input: &peer.ChaincodeInput{
					Args: [][]byte{
						[]byte(lo.Operation),
						[]byte(lo.ChannelID),
						protoMarshal(&peer.ChaincodeDeploymentSpec{
							ChaincodeSpec: &peer.ChaincodeSpec{
								ChaincodeId: &peer.ChaincodeID{
									Name:    lo.Name,
									Version: lo.Version,
								},
								Input: &peer.ChaincodeInput{
									Args: [][]byte{[]byte("bogus-init-arg")},
								},
								Type: peer.ChaincodeSpec_GOLANG,
							},
						}),
						{}, // Endorsement policy bytes deliberately empty
						[]byte("escc"),
						[]byte("vscc"),
					},
				},
				Type: peer.ChaincodeSpec_GOLANG,
			},
		}),
	})

	propHash := sha256.New()
	propHash.Write(channelHeaderBytes)
	propHash.Write(signatureHeaderBytes)
	propHash.Write(proposalPayloadBytes)
	proposalHash := propHash.Sum(nil)[:]

	proposalResponsePayloadBytes := protoMarshal(&peer.ProposalResponsePayload{
		ProposalHash: proposalHash,
		Extension: protoMarshal(&peer.ChaincodeAction{
			ChaincodeId: &peer.ChaincodeID{
				Name:    "lscc",
				Version: "syscc",
			},
			Events: protoMarshal(&peer.ChaincodeEvent{
				ChaincodeId: "lscc",
				EventName:   lo.Operation,
				Payload: protoMarshal(&peer.LifecycleEvent{
					ChaincodeName: lo.Name,
				}),
			}),
			Response: &peer.Response{
				Payload: chaincodeDataBytes,
				Status:  200,
			},
			Results: protoMarshal(&rwset.TxReadWriteSet{
				DataModel: rwset.TxReadWriteSet_KV,
				NsRwset: []*rwset.NsReadWriteSet{
					{
						Namespace: "lscc",
						Rwset: protoMarshal(&kvrwset.KVRWSet{
							Writes: []*kvrwset.KVWrite{
								{
									Key:   lo.Name,
									Value: chaincodeDataBytes,
								},
							},
						}),
					},
					{
						Namespace: lo.Name,
						Rwset: protoMarshal(&kvrwset.KVRWSet{
							Writes: []*kvrwset.KVWrite{
								{
									Key:   "bogus-key",
									Value: []byte("bogus-value"),
								},
							},
						}),
					},
				},
			}),
		}),
	})

	endorsementSignature, err := signer.Sign(append(proposalResponsePayloadBytes, creatorBytes...))
	Expect(err).NotTo(HaveOccurred())

	payloadBytes := protoMarshal(&common.Payload{
		Header: &common.Header{
			ChannelHeader:   channelHeaderBytes,
			SignatureHeader: signatureHeaderBytes,
		},
		Data: protoMarshal(&peer.Transaction{
			Actions: []*peer.TransactionAction{
				{
					Header: signatureHeaderBytes,
					Payload: protoMarshal(&peer.ChaincodeActionPayload{
						ChaincodeProposalPayload: proposalPayloadBytes,
						Action: &peer.ChaincodeEndorsedAction{
							ProposalResponsePayload: proposalResponsePayloadBytes,
							Endorsements: []*peer.Endorsement{
								{
									Endorser:  creatorBytes,
									Signature: endorsementSignature,
								},
							},
						},
					}),
				},
			},
		}),
	})

	envSignature, err := signer.Sign(payloadBytes)
	Expect(err).NotTo(HaveOccurred())

	return &common.Envelope{
		Payload:   payloadBytes,
		Signature: envSignature,
	}
}

func protoMarshal(msg proto.Message) []byte {
	b, err := proto.Marshal(msg)
	Expect(err).NotTo(HaveOccurred())
	return b
}
