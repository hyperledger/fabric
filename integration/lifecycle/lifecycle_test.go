/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-config/protolator"
	"github.com/hyperledger/fabric-config/protolator/protoext/ordererext"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/integration/channelparticipation"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	"github.com/hyperledger/fabric/integration/nwo/fabricconfig"
	"github.com/hyperledger/fabric/integration/nwo/runner"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	. "github.com/onsi/gomega/gstruct"
	"github.com/tedsuo/ifrit"
	ginkgomon "github.com/tedsuo/ifrit/ginkgomon_v2"
)

var _ = Describe("Lifecycle", func() {
	var (
		client                      *docker.Client
		testDir                     string
		network                     *nwo.Network
		processes                   = map[string]ifrit.Process{}
		ordererRunner               *ginkgomon.Runner
		ordererProcess, peerProcess ifrit.Process

		termFiles []string
	)

	BeforeEach(func() {
		var err error
		testDir, err = ioutil.TempDir("", "lifecycle")
		Expect(err).NotTo(HaveOccurred())

		client, err = docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())

		network = nwo.New(nwo.BasicEtcdRaftNoSysChan(), testDir, client, StartPort(), components)

		// Generate config
		network.GenerateConfigTree()

		// configure only one of four peers (Org1, peer0) to use couchdb.
		// Note that we do not support a channel with mixed DBs.
		// However, for testing, it would be fine to use couchdb for one
		// peer. We're using couchdb here to ensure all supported character
		// classes in chaincode names/versions work on the supported db types.
		couchDB := &runner.CouchDB{}
		couchProcess := ifrit.Invoke(couchDB)
		Eventually(couchProcess.Ready(), runner.DefaultStartTimeout).Should(BeClosed())
		Consistently(couchProcess.Wait()).ShouldNot(Receive())
		couchAddr := couchDB.Address()
		peer := network.Peer("Org1", "peer0")
		core := network.ReadPeerConfig(peer)
		core.Ledger.State.StateDatabase = "CouchDB"
		core.Ledger.State.CouchDBConfig.CouchDBAddress = couchAddr
		processes[couchDB.Name] = couchProcess
		setTermFileEnvForBinaryExternalBuilder := func(envKey, termFile string, externalBuilders []fabricconfig.ExternalBuilder) {
			os.Setenv(envKey, termFile)
			for i, e := range externalBuilders {
				if e.Name == "binary" {
					e.PropagateEnvironment = append(e.PropagateEnvironment, envKey)
					externalBuilders[i] = e
				}
			}
		}
		org1TermFile := filepath.Join(testDir, "org1-term-file")
		setTermFileEnvForBinaryExternalBuilder("ORG1_TERM_FILE", org1TermFile, core.Chaincode.ExternalBuilders)
		network.WritePeerConfig(peer, core)

		peer = network.Peer("Org2", "peer0")
		core = network.ReadPeerConfig(peer)
		org2TermFile := filepath.Join(testDir, "org2-term-file")
		setTermFileEnvForBinaryExternalBuilder("ORG2_TERM_FILE", org2TermFile, core.Chaincode.ExternalBuilders)
		network.WritePeerConfig(peer, core)

		termFiles = []string{org1TermFile, org2TermFile}

		// bootstrap the network
		network.Bootstrap()

		ordererRunner, ordererProcess, peerProcess = network.StartSingleOrdererNetwork("orderer")
	})

	AfterEach(func() {
		// Shutdown processes and cleanup
		if ordererProcess != nil {
			ordererProcess.Signal(syscall.SIGTERM)
			Eventually(ordererProcess.Wait(), network.EventuallyTimeout).Should(Receive())
		}

		if peerProcess != nil {
			peerProcess.Signal(syscall.SIGTERM)
			Eventually(peerProcess.Wait(), network.EventuallyTimeout).Should(Receive())
		}

		for _, p := range processes {
			p.Signal(syscall.SIGTERM)
			Eventually(p.Wait(), network.EventuallyTimeout).Should(Receive())
		}
		network.Cleanup()

		os.RemoveAll(testDir)
	})

	It("deploys and executes chaincode using _lifecycle and upgrades it", func() {
		orderer := network.Orderer("orderer")
		testPeers := network.PeersWithChannel("testchannel")
		org1peer0 := network.Peer("Org1", "peer0")

		chaincodePath := components.Build("github.com/hyperledger/fabric/integration/chaincode/simple/cmd")
		chaincode := nwo.Chaincode{
			Name:                "My_1st-Chaincode",
			Version:             "Version-0.0",
			Path:                chaincodePath,
			Lang:                "binary",
			PackageFile:         filepath.Join(testDir, "simplecc.tar.gz"),
			Ctor:                `{"Args":["init","a","100","b","200"]}`,
			ChannelConfigPolicy: "/Channel/Application/Endorsement",
			Sequence:            "1",
			InitRequired:        true,
			Label:               "my_simple_chaincode",
		}

		By("setting up the channel")
		channelparticipation.JoinOrdererJoinPeersAppChannel(network, "testchannel", orderer, ordererRunner)
		network.VerifyMembership(network.PeersWithChannel("testchannel"), "testchannel")
		nwo.EnableCapabilities(network, "testchannel", "Application", "V2_0", orderer, network.Peer("Org1", "peer0"), network.Peer("Org2", "peer0"))

		By("deploying the chaincode")
		nwo.PackageChaincodeBinary(chaincode)
		chaincode.SetPackageIDFromPackageFile()

		nwo.InstallChaincode(network, chaincode, testPeers...)

		By("verifying the installed chaincode package matches the one that was submitted")
		sess, err := network.PeerAdminSession(testPeers[0], commands.ChaincodeGetInstalledPackage{
			PackageID:       chaincode.PackageID,
			OutputDirectory: testDir,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
		fileBytes, err := ioutil.ReadFile(chaincode.PackageFile)
		Expect(err).NotTo(HaveOccurred())
		fileBytesFromPeer, err := ioutil.ReadFile(filepath.Join(network.RootDir, chaincode.PackageID+".tar.gz"))
		Expect(err).NotTo(HaveOccurred())
		Expect(fileBytesFromPeer).To(Equal(fileBytes))

		nwo.ApproveChaincodeForMyOrg(network, "testchannel", orderer, chaincode, testPeers...)

		nwo.CheckCommitReadinessUntilReady(network, "testchannel", chaincode, network.PeerOrgs(), testPeers...)
		nwo.CommitChaincode(network, "testchannel", orderer, chaincode, testPeers[0], testPeers...)
		nwo.InitChaincode(network, "testchannel", orderer, chaincode, testPeers...)

		By("ensuring the chaincode can be invoked and queried")
		endorsers := []*nwo.Peer{
			network.Peer("Org1", "peer0"),
			network.Peer("Org2", "peer0"),
		}
		RunQueryInvokeQuery(network, orderer, "My_1st-Chaincode", 100, endorsers...)

		By("setting a bad package ID to temporarily disable endorsements on org1")
		savedPackageID := chaincode.PackageID
		// note that in theory it should be sufficient to set it to an
		// empty string, but the ApproveChaincodeForMyOrg
		// function fills the packageID field if empty
		chaincode.PackageID = "bad"
		nwo.ApproveChaincodeForMyOrg(network, "testchannel", orderer, chaincode, org1peer0)

		By("querying the chaincode and expecting the invocation to fail")
		sess, err = network.PeerUserSession(org1peer0, "User1", commands.ChaincodeQuery{
			ChannelID: "testchannel",
			Name:      "My_1st-Chaincode",
			Ctor:      `{"Args":["query","a"]}`,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(1))
		Expect(sess.Err).To(gbytes.Say("Error: endorsement failure during query. response: status:500 " +
			"message:\"make sure the chaincode My_1st-Chaincode has been successfully defined on channel testchannel and try " +
			"again: chaincode definition for 'My_1st-Chaincode' exists, but chaincode is not installed\""))

		By("setting the correct package ID to restore the chaincode")
		chaincode.PackageID = savedPackageID
		nwo.ApproveChaincodeForMyOrg(network, "testchannel", orderer, chaincode, org1peer0)

		By("querying the chaincode and expecting the invocation to succeed")
		sess, err = network.PeerUserSession(org1peer0, "User1", commands.ChaincodeQuery{
			ChannelID: "testchannel",
			Name:      "My_1st-Chaincode",
			Ctor:      `{"Args":["query","a"]}`,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
		Expect(sess).To(gbytes.Say("90"))

		By("upgrading the chaincode to sequence 2 using a new chaincode package")
		previousLabel := chaincode.Label
		previousPackageID := chaincode.PackageID
		chaincode.Label = "my_simple_chaincode_updated"
		chaincode.PackageFile = filepath.Join(testDir, "simplecc-updated.tar.gz")
		chaincode.Sequence = "2"
		nwo.PackageChaincodeBinary(chaincode)
		chaincode.SetPackageIDFromPackageFile()
		nwo.InstallChaincode(network, chaincode, testPeers...)

		nwo.ApproveChaincodeForMyOrg(network, "testchannel", orderer, chaincode, testPeers...)

		nwo.CheckCommitReadinessUntilReady(network, "testchannel", chaincode, network.PeerOrgs(), testPeers...)
		nwo.CommitChaincode(network, "testchannel", orderer, chaincode, testPeers[0], testPeers...)

		By("listing the installed chaincodes and verifying the channel/chaincode definitions that are using the chaincode package")
		nwo.QueryInstalledReferences(network, "testchannel", chaincode.Label, chaincode.PackageID, network.Peer("Org2", "peer0"), []string{"My_1st-Chaincode", "Version-0.0"})

		By("checking for term files that are written when the chaincode is stopped via SIGTERM")
		for _, termFile := range termFiles {
			Expect(termFile).To(BeARegularFile())
		}

		By("ensuring the previous chaincode package is no longer referenced by a chaincode definition on the channel")
		Expect(nwo.QueryInstalled(network, network.Peer("Org2", "peer0"))()).To(
			ContainElement(MatchFields(IgnoreExtras,
				Fields{
					"Label":      Equal(previousLabel),
					"PackageId":  Equal(previousPackageID),
					"References": Not(HaveKey("testchannel")),
				},
			)),
		)

		By("ensuring the chaincode can still be invoked and queried")
		RunQueryInvokeQuery(network, orderer, "My_1st-Chaincode", 90, endorsers...)

		By("deploying another chaincode using the same chaincode package")
		anotherChaincode := nwo.Chaincode{
			Name:                "Your_Chaincode",
			Version:             "Version+0_0",
			Path:                chaincodePath,
			Lang:                "binary",
			PackageFile:         filepath.Join(testDir, "simplecc.tar.gz"),
			Ctor:                `{"Args":["init","a","100","b","200"]}`,
			ChannelConfigPolicy: "/Channel/Application/Endorsement",
			Sequence:            "1",
			InitRequired:        true,
			Label:               "my_simple_chaincode",
		}
		nwo.DeployChaincode(network, "testchannel", orderer, anotherChaincode)

		By("listing the installed chaincodes and verifying the channel/chaincode definitions that are using the chaincode package")
		anotherChaincode.SetPackageIDFromPackageFile()
		nwo.QueryInstalledReferences(network, "testchannel", anotherChaincode.Label, anotherChaincode.PackageID, network.Peer("Org2", "peer0"), []string{"Your_Chaincode", "Version+0_0"})

		By("adding a new org")
		org3 := &nwo.Organization{
			MSPID:         "Org3MSP",
			Name:          "Org3",
			Domain:        "org3.example.com",
			EnableNodeOUs: true,
			Users:         2,
			CA: &nwo.CA{
				Hostname: "ca",
			},
		}

		org3peer0 := &nwo.Peer{
			Name:         "peer0",
			Organization: "Org3",
			Channels:     testPeers[0].Channels,
		}

		network.AddOrg(org3, org3peer0)
		GenerateOrgUpdateMaterials(network, org3peer0)

		By("starting the org3 peer")
		pr := network.PeerRunner(org3peer0)
		org3Process := ifrit.Invoke(pr)
		processes[org3peer0.ID()] = org3Process
		Eventually(org3Process.Ready(), network.EventuallyTimeout).Should(BeClosed())

		By("updating the channel config to include org3")
		// get the current channel config
		currentConfig := nwo.GetConfig(network, testPeers[0], orderer, "testchannel")
		updatedConfig := proto.Clone(currentConfig).(*common.Config)

		// get the configtx info for org3
		sess, err = network.ConfigTxGen(commands.PrintOrg{
			ConfigPath: network.RootDir,
			PrintOrg:   "Org3",
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
		org3Group := &ordererext.DynamicOrdererOrgGroup{ConfigGroup: &common.ConfigGroup{}}
		err = protolator.DeepUnmarshalJSON(bytes.NewBuffer(sess.Out.Contents()), org3Group)
		Expect(err).NotTo(HaveOccurred())

		// update the channel config to include org3
		updatedConfig.ChannelGroup.Groups["Application"].Groups["Org3"] = org3Group.ConfigGroup
		nwo.UpdateConfig(network, orderer, "testchannel", currentConfig, updatedConfig, true, testPeers[0], testPeers...)

		By("joining the org3 peers to the channel")
		network.JoinChannel("testchannel", orderer, org3peer0)

		// update testPeers now that org3 has joined
		testPeers = network.PeersWithChannel("testchannel")

		// wait until all peers, particularly those in org3, have received the block
		// containing the updated config
		maxLedgerHeight := nwo.GetMaxLedgerHeight(network, "testchannel", testPeers...)
		nwo.WaitUntilEqualLedgerHeight(network, "testchannel", maxLedgerHeight, testPeers...)

		By("querying definitions by org3 before performing any chaincode actions")
		sess, err = network.PeerAdminSession(network.Peer("Org2", "peer0"), commands.ChaincodeListCommitted{
			ChannelID: "testchannel",
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))

		By("installing the chaincode to the org3 peers")
		nwo.InstallChaincode(network, chaincode, org3peer0)

		By("ensuring org3 peers do not execute the chaincode before approving the definition")
		org3AndOrg1PeerAddresses := []string{
			network.PeerAddress(org3peer0, nwo.ListenPort),
			network.PeerAddress(org1peer0, nwo.ListenPort),
		}

		sess, err = network.PeerUserSession(org3peer0, "User1", commands.ChaincodeInvoke{
			ChannelID:     "testchannel",
			Orderer:       network.OrdererAddress(orderer, nwo.ListenPort),
			Name:          "My_1st-Chaincode",
			Ctor:          `{"Args":["invoke","a","b","10"]}`,
			PeerAddresses: org3AndOrg1PeerAddresses,
			WaitForEvent:  true,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(1))
		Expect(sess.Err).To(gbytes.Say("chaincode definition for 'My_1st-Chaincode' at sequence 2 on channel 'testchannel' has not yet been approved by this org"))

		By("org3 approving the chaincode definition")
		nwo.ApproveChaincodeForMyOrg(network, "testchannel", orderer, chaincode, network.PeersInOrg("Org3")...)
		nwo.EnsureChaincodeCommitted(network, "testchannel", chaincode.Name, chaincode.Version, chaincode.Sequence, []*nwo.Organization{network.Organization("Org1"), network.Organization("Org2"), network.Organization("Org3")}, org3peer0)

		By("ensuring chaincode can be invoked and queried by org3")
		org3andOrg1Endorsers := []*nwo.Peer{
			network.Peer("Org3", "peer0"),
			network.Peer("Org1", "peer0"),
		}
		RunQueryInvokeQuery(network, orderer, "My_1st-Chaincode", 80, org3andOrg1Endorsers...)

		By("deploying a chaincode without an endorsement policy specified")
		chaincode = nwo.Chaincode{
			Name:         "defaultpolicycc",
			Version:      "0.0",
			Path:         chaincodePath,
			Lang:         "binary",
			PackageFile:  filepath.Join(testDir, "modulecc.tar.gz"),
			Ctor:         `{"Args":["init","a","100","b","200"]}`,
			Sequence:     "1",
			InitRequired: true,
			Label:        "my_simple_chaincode",
		}

		nwo.DeployChaincode(network, "testchannel", orderer, chaincode)

		By("attempting to invoke the chaincode without a majority")
		sess, err = network.PeerUserSession(org3peer0, "User1", commands.ChaincodeInvoke{
			ChannelID:    "testchannel",
			Orderer:      network.OrdererAddress(orderer, nwo.ListenPort),
			Name:         "defaultpolicycc",
			Ctor:         `{"Args":["invoke","a","b","10"]}`,
			WaitForEvent: true,
		})
		Expect(err).ToNot(HaveOccurred())
		Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(1))
		Expect(sess.Err).To(gbytes.Say(`\QError: transaction invalidated with status (ENDORSEMENT_POLICY_FAILURE)\E`))

		By("attempting to invoke the chaincode with a majority")
		sess, err = network.PeerUserSession(org3peer0, "User1", commands.ChaincodeInvoke{
			ChannelID:     "testchannel",
			Orderer:       network.OrdererAddress(orderer, nwo.ListenPort),
			Name:          "defaultpolicycc",
			Ctor:          `{"Args":["invoke","a","b","10"]}`,
			PeerAddresses: org3AndOrg1PeerAddresses,
			WaitForEvent:  true,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
		Expect(sess.Err).To(gbytes.Say(`\Qcommitted with status (VALID)\E`))
		Expect(sess.Err).To(gbytes.Say(`Chaincode invoke successful. result: status:200`))
	})
})
