/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"

	docker "github.com/fsouza/go-dockerclient"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/integration/channelparticipation"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	"github.com/hyperledger/fabric/protoutil"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
	ginkgomon "github.com/tedsuo/ifrit/ginkgomon_v2"
	"google.golang.org/grpc"
)

var _ = Describe("Release interoperability", func() {
	var (
		client  *docker.Client
		testDir string

		network                     *nwo.Network
		ordererRunner               *ginkgomon.Runner
		ordererProcess, peerProcess ifrit.Process

		orderer   *nwo.Orderer
		endorsers []*nwo.Peer
	)

	BeforeEach(func() {
		var err error
		testDir, err = ioutil.TempDir("", "lifecycle")
		Expect(err).NotTo(HaveOccurred())

		client, err = docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())

		network = nwo.New(nwo.MultiChannelEtcdRaftNoSysChan(), testDir, client, StartPort(), components)
		network.GenerateConfigTree()
		network.Bootstrap()

		// Start all the fabric processes
		ordererRunner, ordererProcess, peerProcess = network.StartSingleOrdererNetwork("orderer")

		orderer = network.Orderer("orderer")
		endorsers = []*nwo.Peer{
			network.Peer("Org1", "peer0"),
			network.Peer("Org2", "peer0"),
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

	It("deploys and executes chaincode, upgrades the channel application capabilities to V2_0, and uses _lifecycle to update the endorsement policy", func() {
		By("deploying the chaincode using LSCC on a channel with V1_4 application capabilities")
		chaincode := nwo.Chaincode{
			Name:    "mycc",
			Version: "0.0",
			Path:    "github.com/hyperledger/fabric/integration/chaincode/simple/cmd",
			Ctor:    `{"Args":["init","a","100","b","200"]}`,
			Policy:  `AND ('Org1MSP.member','Org2MSP.member')`,
		}

		channelparticipation.JoinOrdererJoinPeersAppChannel(network, "testchannel", orderer, ordererRunner)
		nwo.DeployChaincodeLegacy(network, "testchannel", orderer, chaincode)
		RunQueryInvokeQuery(network, orderer, "mycc", 100, endorsers...)

		By("enabling V2_0 application capabilities")
		nwo.EnableCapabilities(network, "testchannel", "Application", "V2_0", orderer, endorsers...)

		By("ensuring that the chaincode is still operational after the upgrade")
		RunQueryInvokeQuery(network, orderer, "mycc", 90, endorsers...)

		By("restarting the network from persistence (1)")
		ordererRunner, ordererProcess, peerProcess = nwo.RestartSingleOrdererNetwork(ordererProcess, peerProcess, network)

		By("ensuring that the chaincode is still operational after the upgrade and restart")
		RunQueryInvokeQuery(network, orderer, "mycc", 80, endorsers...)

		By("attempting to invoke the chaincode without sufficient endorsements")
		sess, err := network.PeerUserSession(endorsers[0], "User1", commands.ChaincodeInvoke{
			ChannelID: "testchannel",
			Orderer:   network.OrdererAddress(orderer, nwo.ListenPort),
			Name:      "mycc",
			Ctor:      `{"Args":["invoke","a","b","10"]}`,
			PeerAddresses: []string{
				network.PeerAddress(endorsers[0], nwo.ListenPort),
			},
			WaitForEvent: true,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(1))
		Expect(sess.Err).To(gbytes.Say(`\Qcommitted with status (ENDORSEMENT_POLICY_FAILURE)\E`))

		By("upgrading the chaincode definition using _lifecycle")
		chaincode = nwo.Chaincode{
			Name:            "mycc",
			Version:         "0.0",
			Path:            components.Build("github.com/hyperledger/fabric/integration/chaincode/simple/cmd"),
			Lang:            "binary",
			PackageFile:     filepath.Join(testDir, "simplecc.tar.gz"),
			SignaturePolicy: `OR ('Org1MSP.member','Org2MSP.member')`,
			Sequence:        "1",
			InitRequired:    false,
			Label:           "my_prebuilt_chaincode",
		}
		nwo.DeployChaincode(network, "testchannel", orderer, chaincode)

		By("querying/invoking/querying the chaincode with the new definition")
		RunQueryInvokeQuery(network, orderer, "mycc", 70, endorsers[0])

		By("restarting the network from persistence (2)")
		ordererRunner, ordererProcess, peerProcess = nwo.RestartSingleOrdererNetwork(ordererProcess, peerProcess, network)

		By("querying/invoking/querying the chaincode with the new definition again")
		RunQueryInvokeQuery(network, orderer, "mycc", 60, endorsers[1])
	})

	Describe("Interoperability scenarios", func() {
		var (
			occ, pcc       *grpc.ClientConn
			userSigner     *nwo.SigningIdentity
			endorserClient pb.EndorserClient
			deliveryClient pb.DeliverClient
			ordererClient  ab.AtomicBroadcast_BroadcastClient
		)

		BeforeEach(func() {
			userSigner = network.PeerUserSigner(endorsers[0], "User1")

			pcc = network.PeerClientConn(endorsers[0])
			endorserClient = pb.NewEndorserClient(pcc)
			deliveryClient = pb.NewDeliverClient(pcc)

			var err error
			occ = network.OrdererClientConn(orderer)
			ordererClient, err = ab.NewAtomicBroadcastClient(occ).Broadcast(context.Background())
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if occ != nil {
				occ.Close()
			}
			if pcc != nil {
				pcc.Close()
			}
		})

		It("deploys a chaincode with the legacy lifecycle, invokes it and the tx is committed only after the chaincode is upgraded via _lifecycle", func() {
			By("deploying the chaincode using the legacy lifecycle")
			chaincode := nwo.Chaincode{
				Name:    "mycc",
				Version: "0.0",
				Path:    "github.com/hyperledger/fabric/integration/chaincode/simple/cmd",
				Ctor:    `{"Args":["init","a","100","b","200"]}`,
				Policy:  `OR ('Org1MSP.member','Org2MSP.member')`,
			}

			channelparticipation.JoinOrdererJoinPeersAppChannel(network, "testchannel", orderer, ordererRunner)
			nwo.DeployChaincodeLegacy(network, "testchannel", orderer, chaincode)
			RunQueryInvokeQuery(network, orderer, "mycc", 100, endorsers...)

			By("invoking the chaincode with the legacy definition and keeping the transaction")
			signedProp, prop, txid := SignedProposal(
				"testchannel",
				"mycc",
				userSigner,
				"invoke",
				"a",
				"b",
				"10",
			)
			presp, err := endorserClient.ProcessProposal(context.Background(), signedProp)
			Expect(err).NotTo(HaveOccurred())
			Expect(presp).NotTo(BeNil())

			env, err := protoutil.CreateSignedTx(prop, userSigner, presp)
			Expect(err).NotTo(HaveOccurred())
			Expect(env).NotTo(BeNil())

			By("enabling V2_0 application capabilities")
			nwo.EnableCapabilities(network, "testchannel", "Application", "V2_0", orderer, network.Peer("Org1", "peer0"), network.Peer("Org2", "peer0"))

			By("upgrading the chaincode definition using _lifecycle")
			chaincode = nwo.Chaincode{
				Name:            "mycc",
				Version:         "0.0",
				Path:            components.Build("github.com/hyperledger/fabric/integration/chaincode/simple/cmd"),
				Lang:            "binary",
				PackageFile:     filepath.Join(testDir, "simplecc.tar.gz"),
				SignaturePolicy: `OR ('Org1MSP.member','Org2MSP.member')`,
				Sequence:        "1",
				InitRequired:    false,
				Label:           "my_prebuilt_chaincode",
			}
			nwo.DeployChaincode(network, "testchannel", orderer, chaincode)

			By("committing the old transaction")
			// FAB-17458: because the endorsed tx doesn't have _lifecycle in read set,
			// it has no read conflict with the cc upgrade via new lifecycle.
			err = CommitTx(network, env, endorsers[0], deliveryClient, ordererClient, userSigner, txid)
			Expect(err).NotTo(HaveOccurred())
		})

		It("deploys a chaincode with the new lifecycle, invokes it and the tx is committed only after the chaincode is upgraded via _lifecycle", func() {
			By("enabling V2_0 application capabilities")
			channelparticipation.JoinOrdererJoinPeersAppChannel(network, "testchannel", orderer, ordererRunner)
			nwo.EnableCapabilities(network, "testchannel", "Application", "V2_0", orderer, network.Peer("Org1", "peer0"), network.Peer("Org2", "peer0"))

			By("deploying the chaincode definition using _lifecycle")
			chaincode := nwo.Chaincode{
				Name:            "mycc",
				Version:         "0.0",
				Path:            components.Build("github.com/hyperledger/fabric/integration/chaincode/simple/cmd"),
				Lang:            "binary",
				PackageFile:     filepath.Join(testDir, "simplecc.tar.gz"),
				SignaturePolicy: `AND ('Org1MSP.member','Org2MSP.member')`,
				Sequence:        "1",
				InitRequired:    true,
				Label:           "my_prebuilt_chaincode",
				Ctor:            `{"Args":["init","a","100","b","200"]}`,
			}
			nwo.DeployChaincode(network, "testchannel", orderer, chaincode)

			By("Invoking the chaincode with the first definition and keeping the transaction")
			signedProp, prop, txid := SignedProposal(
				"testchannel",
				"mycc",
				userSigner,
				"invoke",
				"a",
				"b",
				"10",
			)
			presp, err := endorserClient.ProcessProposal(context.Background(), signedProp)
			Expect(err).NotTo(HaveOccurred())
			Expect(presp).NotTo(BeNil())

			env, err := protoutil.CreateSignedTx(prop, userSigner, presp)
			Expect(err).NotTo(HaveOccurred())
			Expect(env).NotTo(BeNil())

			By("upgrading the chaincode definition using _lifecycle")
			chaincode.Sequence = "2"
			chaincode.SignaturePolicy = `OR ('Org1MSP.member','Org2MSP.member')`
			chaincode.InitRequired = false
			nwo.DeployChaincode(network, "testchannel", orderer, chaincode)

			By("committing the old transaction, expecting to hit an MVCC conflict")
			err = CommitTx(network, env, endorsers[0], deliveryClient, ordererClient, userSigner, txid)
			Expect(err).To(MatchError(ContainSubstring("transaction invalidated with status (MVCC_READ_CONFLICT)")))
		})

		Describe("Chaincode-to-chaincode interoperability", func() {
			var (
				callerDefOld nwo.Chaincode
				callerDefNew nwo.Chaincode
				calleeDefOld nwo.Chaincode
				calleeDefNew nwo.Chaincode
			)

			BeforeEach(func() {
				ccEP := `OR ('Org1MSP.member','Org2MSP.member')`
				callerDefOld = nwo.Chaincode{
					Name:    "caller",
					Version: "0.0",
					Path:    "github.com/hyperledger/fabric/integration/lifecycle/chaincode/caller/cmd",
					Ctor:    `{"Args":[""]}`,
					Policy:  ccEP,
				}
				calleeDefOld = nwo.Chaincode{
					Name:    "callee",
					Version: "0.0",
					Path:    "github.com/hyperledger/fabric/integration/lifecycle/chaincode/callee/cmd",
					Ctor:    `{"Args":[""]}`,
					Policy:  ccEP,
				}
				callerDefNew = nwo.Chaincode{
					Name:            "caller",
					Version:         "0.0",
					Path:            components.Build("github.com/hyperledger/fabric/integration/lifecycle/chaincode/caller/cmd"),
					Lang:            "binary",
					PackageFile:     filepath.Join(testDir, "caller.tar.gz"),
					SignaturePolicy: ccEP,
					Sequence:        "1",
					Label:           "my_prebuilt_caller_chaincode",
					InitRequired:    true,
					Ctor:            `{"Args":[""]}`,
				}
				calleeDefNew = nwo.Chaincode{
					Name:            "callee",
					Version:         "0.0",
					Path:            components.Build("github.com/hyperledger/fabric/integration/lifecycle/chaincode/callee/cmd"),
					Lang:            "binary",
					PackageFile:     filepath.Join(testDir, "callee.tar.gz"),
					SignaturePolicy: ccEP,
					Sequence:        "1",
					Label:           "my_prebuilt_callee_chaincode",
					InitRequired:    true,
					Ctor:            `{"Args":[""]}`,
				}
				By("Creating and joining the channel")
				channelparticipation.JoinOrdererJoinPeersAppChannel(network, "testchannel", orderer, ordererRunner)
			})

			It("Deploys two chaincodes with the new lifecycle and performs a successful cc2cc invocation", func() {
				By("enabling the 2.0 capability on the channel")
				nwo.EnableCapabilities(network, "testchannel", "Application", "V2_0", orderer, network.Peer("Org1", "peer0"), network.Peer("Org2", "peer0"))

				By("deploying the caller chaincode using _lifecycle")
				nwo.DeployChaincode(network, "testchannel", orderer, callerDefNew)

				By("deploying the callee chaincode using _lifecycle")
				nwo.DeployChaincode(network, "testchannel", orderer, calleeDefNew)

				By("invoking the chaincode and generating a transaction")
				signedProp, prop, txid := SignedProposal(
					"testchannel",
					"caller",
					userSigner,
					"INVOKE",
					"callee",
				)
				presp, err := endorserClient.ProcessProposal(context.Background(), signedProp)
				Expect(err).NotTo(HaveOccurred())
				Expect(presp).NotTo(BeNil())

				env, err := protoutil.CreateSignedTx(prop, userSigner, presp)
				Expect(err).NotTo(HaveOccurred())
				Expect(env).NotTo(BeNil())

				By("committing the transaction")
				err = CommitTx(network, env, endorsers[0], deliveryClient, ordererClient, userSigner, txid)
				Expect(err).NotTo(HaveOccurred())

				By("querying the caller chaincode")
				sess, err := network.PeerUserSession(endorsers[0], "User1", commands.ChaincodeQuery{
					ChannelID: "testchannel",
					Name:      "caller",
					Ctor:      `{"Args":["QUERY"]}`,
				})
				Expect(err).NotTo(HaveOccurred())
				Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
				Expect(sess).To(gbytes.Say("caller:bar"))

				By("querying the callee chaincode")
				sess, err = network.PeerUserSession(endorsers[0], "User1", commands.ChaincodeQuery{
					ChannelID: "testchannel",
					Name:      "callee",
					Ctor:      `{"Args":["QUERY"]}`,
				})
				Expect(err).NotTo(HaveOccurred())
				Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
				Expect(sess).To(gbytes.Say("callee:bar"))

				By("enabling the 2.0 capability on channel2")
				channelparticipation.JoinOrdererJoinPeersAppChannel(network, "testchannel2", orderer, ordererRunner)
				nwo.EnableCapabilities(network, "testchannel2", "Application", "V2_0", orderer, network.Peer("Org1", "peer0"), network.Peer("Org2", "peer0"))

				By("deploying the callee chaincode using _lifecycle on channel2")
				nwo.DeployChaincode(network, "testchannel2", orderer, calleeDefNew)

				By("invoking the chaincode on callee on channel2")
				sess, err = network.PeerUserSession(endorsers[0], "User1", commands.ChaincodeInvoke{
					ChannelID:    "testchannel2",
					Orderer:      network.OrdererAddress(orderer, nwo.ListenPort),
					Name:         "callee",
					Ctor:         `{"Args":["INVOKE"]}`,
					WaitForEvent: true,
				})
				Expect(err).NotTo(HaveOccurred())
				Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
				Expect(sess.Err).To(gbytes.Say("Chaincode invoke successful. result: status:200"))

				By("querying the callee chaincode on channel2")
				sess, err = network.PeerUserSession(endorsers[0], "User1", commands.ChaincodeQuery{
					ChannelID: "testchannel2",
					Name:      "callee",
					Ctor:      `{"Args":["QUERY"]}`,
				})
				Expect(err).NotTo(HaveOccurred())
				Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
				Expect(sess).To(gbytes.Say("callee:bar"))

				By("querying (QUERYCALLEE) the callee chaincode on channel2 from caller on channel")
				sess, err = network.PeerUserSession(endorsers[0], "User1", commands.ChaincodeQuery{
					ChannelID: "testchannel",
					Name:      "caller",
					Ctor:      `{"Args":["QUERYCALLEE", "callee", "testchannel2"]}`,
				})
				Expect(err).NotTo(HaveOccurred())
				Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
				Expect(sess).To(gbytes.Say("callee:bar"))

				By("querying (QUERYCALLEE) the callee chaincode from caller on non-existing channel and expecting the invocation to fail")
				sess, err = network.PeerUserSession(endorsers[0], "User1", commands.ChaincodeQuery{
					ChannelID: "testchannel",
					Name:      "caller",
					Ctor:      `{"Args":["QUERYCALLEE", "callee", "nonExistingChannel2"]}`,
				})
				Expect(err).NotTo(HaveOccurred())
				Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(1))
				Expect(sess.Err).To(gbytes.Say(`Error: endorsement failure during query. response: status:500`))
			})

			When("the network starts with new definitions", func() {
				BeforeEach(func() {
					By("enabling the 2.0 capability on the channel")
					nwo.EnableCapabilities(network, "testchannel", "Application", "V2_0", orderer, network.Peer("Org1", "peer0"), network.Peer("Org2", "peer0"))

					By("upgrading the caller with the new definition")
					nwo.DeployChaincode(network, "testchannel", orderer, callerDefNew)

					By("upgrading the callee with the new definition")
					nwo.DeployChaincode(network, "testchannel", orderer, calleeDefNew)
				})

				It("performs a successful cc2cc invocation", func() {
					By("invoking the chaincode and generating a transaction")
					signedProp, prop, txid := SignedProposal(
						"testchannel",
						"caller",
						userSigner,
						"INVOKE",
						"callee",
					)
					presp, err := endorserClient.ProcessProposal(context.Background(), signedProp)
					Expect(err).NotTo(HaveOccurred())
					Expect(presp).NotTo(BeNil())

					env, err := protoutil.CreateSignedTx(prop, userSigner, presp)
					Expect(err).NotTo(HaveOccurred())
					Expect(env).NotTo(BeNil())

					By("committing the transaction")
					err = CommitTx(network, env, endorsers[0], deliveryClient, ordererClient, userSigner, txid)
					Expect(err).NotTo(HaveOccurred())

					By("querying the caller chaincode")
					sess, err := network.PeerUserSession(endorsers[0], "User1", commands.ChaincodeQuery{
						ChannelID: "testchannel",
						Name:      "caller",
						Ctor:      `{"Args":["QUERY"]}`,
					})
					Expect(err).NotTo(HaveOccurred())
					Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
					Expect(sess).To(gbytes.Say("caller:bar"))

					By("querying the callee chaincode")
					sess, err = network.PeerUserSession(endorsers[0], "User1", commands.ChaincodeQuery{
						ChannelID: "testchannel",
						Name:      "callee",
						Ctor:      `{"Args":["QUERY"]}`,
					})
					Expect(err).NotTo(HaveOccurred())
					Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
					Expect(sess).To(gbytes.Say("callee:bar"))
				})

				It("performs a successful cc2cc invocation which doesn't commit because the caller is further upgraded", func() {
					By("invoking the chaincode and generating a transaction")
					signedProp, prop, txid := SignedProposal(
						"testchannel",
						"caller",
						userSigner,
						"INVOKE",
						"callee",
					)
					presp, err := endorserClient.ProcessProposal(context.Background(), signedProp)
					Expect(err).NotTo(HaveOccurred())
					Expect(presp).NotTo(BeNil())

					env, err := protoutil.CreateSignedTx(prop, userSigner, presp)
					Expect(err).NotTo(HaveOccurred())
					Expect(env).NotTo(BeNil())

					By("further upgrading the caller with the new definition")
					callerDefNew.Sequence = "2"
					callerDefNew.InitRequired = false
					nwo.DeployChaincode(network, "testchannel", orderer, callerDefNew)

					By("committing the transaction")
					err = CommitTx(network, env, endorsers[0], deliveryClient, ordererClient, userSigner, txid)
					Expect(err).To(MatchError(ContainSubstring("transaction invalidated with status (MVCC_READ_CONFLICT)")))

					By("querying the caller chaincode")
					sess, err := network.PeerUserSession(endorsers[0], "User1", commands.ChaincodeQuery{
						ChannelID: "testchannel",
						Name:      "caller",
						Ctor:      `{"Args":["QUERY"]}`,
					})
					Expect(err).NotTo(HaveOccurred())
					Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
					Expect(sess).To(gbytes.Say("caller:foo"))

					By("querying the callee chaincode")
					sess, err = network.PeerUserSession(endorsers[0], "User1", commands.ChaincodeQuery{
						ChannelID: "testchannel",
						Name:      "callee",
						Ctor:      `{"Args":["QUERY"]}`,
					})
					Expect(err).NotTo(HaveOccurred())
					Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
					Expect(sess).To(gbytes.Say("callee:foo"))
				})

				It("performs a successful cc2cc invocation which doesn't commit because the callee is further upgraded", func() {
					By("invoking the chaincode and generating a transaction")
					signedProp, prop, txid := SignedProposal(
						"testchannel",
						"caller",
						userSigner,
						"INVOKE",
						"callee",
					)
					presp, err := endorserClient.ProcessProposal(context.Background(), signedProp)
					Expect(err).NotTo(HaveOccurred())
					Expect(presp).NotTo(BeNil())

					env, err := protoutil.CreateSignedTx(prop, userSigner, presp)
					Expect(err).NotTo(HaveOccurred())
					Expect(env).NotTo(BeNil())

					By("further upgrading the callee with the new definition")
					calleeDefNew.Sequence = "2"
					calleeDefNew.InitRequired = false
					nwo.DeployChaincode(network, "testchannel", orderer, calleeDefNew)

					By("committing the transaction")
					err = CommitTx(network, env, endorsers[0], deliveryClient, ordererClient, userSigner, txid)
					Expect(err).To(MatchError(ContainSubstring("transaction invalidated with status (MVCC_READ_CONFLICT)")))

					By("querying the caller chaincode")
					sess, err := network.PeerUserSession(endorsers[0], "User1", commands.ChaincodeQuery{
						ChannelID: "testchannel",
						Name:      "caller",
						Ctor:      `{"Args":["QUERY"]}`,
					})
					Expect(err).NotTo(HaveOccurred())
					Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
					Expect(sess).To(gbytes.Say("caller:foo"))

					By("querying the callee chaincode")
					sess, err = network.PeerUserSession(endorsers[0], "User1", commands.ChaincodeQuery{
						ChannelID: "testchannel",
						Name:      "callee",
						Ctor:      `{"Args":["QUERY"]}`,
					})
					Expect(err).NotTo(HaveOccurred())
					Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
					Expect(sess).To(gbytes.Say("callee:foo"))
				})
			})

			When("the network starts with legacy definitions and then upgrades to 2.0", func() {
				BeforeEach(func() {
					By("deploying the caller chaincode using the legacy lifecycle")
					nwo.DeployChaincodeLegacy(network, "testchannel", orderer, callerDefOld)

					By("deploying the callee chaincode using the legacy lifecycle")
					nwo.DeployChaincodeLegacy(network, "testchannel", orderer, calleeDefOld)

					By("enabling the 2.0 capability on the channel")
					nwo.EnableCapabilities(network, "testchannel", "Application", "V2_0", orderer, network.Peer("Org1", "peer0"), network.Peer("Org2", "peer0"))
				})

				It("upgrades the caller with the new and performs a successful cc2cc invocation", func() {
					By("upgrading the caller with the new definition")
					nwo.DeployChaincode(network, "testchannel", orderer, callerDefNew)

					By("invoking the chaincode and generating a transaction")
					signedProp, prop, txid := SignedProposal(
						"testchannel",
						"caller",
						userSigner,
						"INVOKE",
						"callee",
					)
					presp, err := endorserClient.ProcessProposal(context.Background(), signedProp)
					Expect(err).NotTo(HaveOccurred())
					Expect(presp).NotTo(BeNil())

					env, err := protoutil.CreateSignedTx(prop, userSigner, presp)
					Expect(err).NotTo(HaveOccurred())
					Expect(env).NotTo(BeNil())

					By("committing the transaction")
					err = CommitTx(network, env, endorsers[0], deliveryClient, ordererClient, userSigner, txid)
					Expect(err).NotTo(HaveOccurred())

					By("querying the caller chaincode")
					sess, err := network.PeerUserSession(endorsers[0], "User1", commands.ChaincodeQuery{
						ChannelID: "testchannel",
						Name:      "caller",
						Ctor:      `{"Args":["QUERY"]}`,
					})
					Expect(err).NotTo(HaveOccurred())
					Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
					Expect(sess).To(gbytes.Say("caller:bar"))

					By("querying the callee chaincode")
					sess, err = network.PeerUserSession(endorsers[0], "User1", commands.ChaincodeQuery{
						ChannelID: "testchannel",
						Name:      "callee",
						Ctor:      `{"Args":["QUERY"]}`,
					})
					Expect(err).NotTo(HaveOccurred())
					Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
					Expect(sess).To(gbytes.Say("callee:bar"))
				})

				It("upgrades the callee with the new and performs a successful cc2cc invocation", func() {
					By("upgrading the callee with the new definition")
					nwo.DeployChaincode(network, "testchannel", orderer, calleeDefNew)

					By("invoking the chaincode and generating a transaction")
					signedProp, prop, txid := SignedProposal(
						"testchannel",
						"caller",
						userSigner,
						"INVOKE",
						"callee",
					)
					presp, err := endorserClient.ProcessProposal(context.Background(), signedProp)
					Expect(err).NotTo(HaveOccurred())
					Expect(presp).NotTo(BeNil())

					env, err := protoutil.CreateSignedTx(prop, userSigner, presp)
					Expect(err).NotTo(HaveOccurred())
					Expect(env).NotTo(BeNil())

					By("committing the transaction")
					err = CommitTx(network, env, endorsers[0], deliveryClient, ordererClient, userSigner, txid)
					Expect(err).NotTo(HaveOccurred())

					By("querying the caller chaincode")
					sess, err := network.PeerUserSession(endorsers[0], "User1", commands.ChaincodeQuery{
						ChannelID: "testchannel",
						Name:      "caller",
						Ctor:      `{"Args":["QUERY"]}`,
					})
					Expect(err).NotTo(HaveOccurred())
					Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
					Expect(sess).To(gbytes.Say("caller:bar"))

					By("querying the callee chaincode")
					sess, err = network.PeerUserSession(endorsers[0], "User1", commands.ChaincodeQuery{
						ChannelID: "testchannel",
						Name:      "callee",
						Ctor:      `{"Args":["QUERY"]}`,
					})
					Expect(err).NotTo(HaveOccurred())
					Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
					Expect(sess).To(gbytes.Say("callee:bar"))
				})

				It("performs a cc2cc invocation which fails because in the meantime, the callee is upgraded with the new lifecycle", func() {
					By("invoking the chaincode and generating a transaction")
					signedProp, prop, txid := SignedProposal(
						"testchannel",
						"caller",
						userSigner,
						"INVOKE",
						"callee",
					)
					presp, err := endorserClient.ProcessProposal(context.Background(), signedProp)
					Expect(err).NotTo(HaveOccurred())
					Expect(presp).NotTo(BeNil())

					env, err := protoutil.CreateSignedTx(prop, userSigner, presp)
					Expect(err).NotTo(HaveOccurred())
					Expect(env).NotTo(BeNil())

					By("upgrading the callee with the new definition")
					nwo.DeployChaincode(network, "testchannel", orderer, calleeDefNew)

					By("committing the transaction")
					err = CommitTx(network, env, endorsers[0], deliveryClient, ordererClient, userSigner, txid)
					Expect(err).To(MatchError(ContainSubstring("transaction invalidated with status (MVCC_READ_CONFLICT)")))

					By("querying the caller chaincode")
					sess, err := network.PeerUserSession(endorsers[0], "User1", commands.ChaincodeQuery{
						ChannelID: "testchannel",
						Name:      "caller",
						Ctor:      `{"Args":["QUERY"]}`,
					})
					Expect(err).NotTo(HaveOccurred())
					Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
					Expect(sess).To(gbytes.Say("caller:foo"))

					By("querying the callee chaincode")
					sess, err = network.PeerUserSession(endorsers[0], "User1", commands.ChaincodeQuery{
						ChannelID: "testchannel",
						Name:      "callee",
						Ctor:      `{"Args":["QUERY"]}`,
					})
					Expect(err).NotTo(HaveOccurred())
					Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
					Expect(sess).To(gbytes.Say("callee:foo"))
				})

				It("performs a cc2cc invocation which fails because in the meantime, the caller is upgraded with the new lifecycle", func() {
					By("invoking the chaincode and generating a transaction")
					signedProp, prop, txid := SignedProposal(
						"testchannel",
						"caller",
						userSigner,
						"INVOKE",
						"callee",
					)
					presp, err := endorserClient.ProcessProposal(context.Background(), signedProp)
					Expect(err).NotTo(HaveOccurred())
					Expect(presp).NotTo(BeNil())

					env, err := protoutil.CreateSignedTx(prop, userSigner, presp)
					Expect(err).NotTo(HaveOccurred())
					Expect(env).NotTo(BeNil())

					By("upgrading the caller with the new definition")
					nwo.DeployChaincode(network, "testchannel", orderer, callerDefNew)

					By("committing the transaction")
					err = CommitTx(network, env, endorsers[0], deliveryClient, ordererClient, userSigner, txid)
					Expect(err).To(MatchError(ContainSubstring("transaction invalidated with status (MVCC_READ_CONFLICT)")))

					By("querying the caller chaincode")
					sess, err := network.PeerUserSession(endorsers[0], "User1", commands.ChaincodeQuery{
						ChannelID: "testchannel",
						Name:      "caller",
						Ctor:      `{"Args":["QUERY"]}`,
					})
					Expect(err).NotTo(HaveOccurred())
					Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
					Expect(sess).To(gbytes.Say("caller:foo"))

					By("querying the callee chaincode")
					sess, err = network.PeerUserSession(endorsers[0], "User1", commands.ChaincodeQuery{
						ChannelID: "testchannel",
						Name:      "callee",
						Ctor:      `{"Args":["QUERY"]}`,
					})
					Expect(err).NotTo(HaveOccurred())
					Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
					Expect(sess).To(gbytes.Say("callee:foo"))
				})
			})
		})
	})
})
