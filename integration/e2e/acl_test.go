/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package e2e

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/core/aclmgmt/resources"
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
)

var _ = Describe("EndToEndACL", func() {
	var (
		testDir                     string
		client                      *docker.Client
		network                     *nwo.Network
		chaincode                   nwo.Chaincode
		ordererRunner               *ginkgomon.Runner
		ordererProcess, peerProcess ifrit.Process

		orderer   *nwo.Orderer
		org1Peer0 *nwo.Peer
		org2Peer0 *nwo.Peer
	)

	BeforeEach(func() {
		var err error
		testDir, err = ioutil.TempDir("", "acl-e2e")
		Expect(err).NotTo(HaveOccurred())

		client, err = docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())

		// Speed up test by reducing the number of peers we
		// bring up and install chaincode to.
		etcdRaftConfig := nwo.BasicEtcdRaftNoSysChan()
		etcdRaftConfig.RemovePeer("Org1", "peer1")
		etcdRaftConfig.RemovePeer("Org2", "peer1")
		Expect(etcdRaftConfig.Peers).To(HaveLen(2))

		network = nwo.New(etcdRaftConfig, testDir, client, StartPort(), components)
		network.GenerateConfigTree()
		network.Bootstrap()
		// Start all the fabric processes
		ordererRunner, ordererProcess, peerProcess = network.StartSingleOrdererNetwork("orderer")

		orderer = network.Orderer("orderer")
		org1Peer0 = network.Peer("Org1", "peer0")
		org2Peer0 = network.Peer("Org2", "peer0")

		chaincode = nwo.Chaincode{
			Name:    "mycc",
			Version: "0.0",
			Path:    "github.com/hyperledger/fabric/integration/chaincode/simple/cmd",
			Ctor:    `{"Args":["init","a","100","b","200"]}`,
			Policy:  `OR ('Org1MSP.member','Org2MSP.member')`,
		}
		channelparticipation.JoinOrdererJoinPeersAppChannel(network, "testchannel", orderer, ordererRunner)
		nwo.DeployChaincodeLegacy(network, "testchannel", orderer, chaincode)
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

	It("enforces access control list policies", func() {
		invokeChaincode := commands.ChaincodeInvoke{
			ChannelID:    "testchannel",
			Orderer:      network.OrdererAddress(orderer, nwo.ListenPort),
			Name:         chaincode.Name,
			Ctor:         `{"Args":["invoke","a","b","10"]}`,
			WaitForEvent: true,
		}

		outputBlock := filepath.Join(testDir, "newest_block.pb")
		fetchNewest := commands.ChannelFetch{
			ChannelID:  "testchannel",
			Block:      "newest",
			OutputFile: outputBlock,
		}

		//
		// when the ACL policy for DeliverFiltered is satisfied
		//
		By("setting the filtered block event ACL policy to Org1/Admins")
		policyName := resources.Event_FilteredBlock
		policy := "/Channel/Application/Org1/Admins"
		SetACLPolicy(network, "testchannel", policyName, policy, "orderer")

		By("invoking chaincode as a permitted Org1 Admin identity")
		sess, err := network.PeerAdminSession(org1Peer0, invokeChaincode)
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess.Err, network.EventuallyTimeout).Should(gbytes.Say("Chaincode invoke successful. result: status:200"))

		//
		// when the ACL policy for DeliverFiltered is not satisfied
		//
		By("setting the filtered block event ACL policy to org2/Admins")
		policyName = resources.Event_FilteredBlock
		policy = "/Channel/Application/org2/Admins"
		SetACLPolicy(network, "testchannel", policyName, policy, "orderer")

		By("invoking chaincode as a forbidden Org1 Admin identity")
		sess, err = network.PeerAdminSession(org1Peer0, invokeChaincode)
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess.Err, network.EventuallyTimeout).Should(gbytes.Say(`\Qdeliver completed with status (FORBIDDEN)\E`))

		//
		// when the ACL policy for Deliver is satisfied
		//
		By("setting the block event ACL policy to Org1/Admins")
		policyName = resources.Event_Block
		policy = "/Channel/Application/Org1/Admins"
		SetACLPolicy(network, "testchannel", policyName, policy, "orderer")

		By("fetching the latest block from the peer as a permitted Org1 Admin identity")
		sess, err = network.PeerAdminSession(org1Peer0, fetchNewest)
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
		Expect(sess.Err).To(gbytes.Say("Received block: "))

		//
		// when the ACL policy for Deliver is not satisfied
		//
		By("fetching the latest block from the peer as a forbidden org2 Admin identity")
		sess, err = network.PeerAdminSession(org2Peer0, fetchNewest)
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit())
		Expect(sess.Err).To(gbytes.Say("can't read the block: &{FORBIDDEN}"))

		//
		// when the ACL policy for lscc/GetInstantiatedChaincodes is satisfied
		//
		By("setting the lscc/GetInstantiatedChaincodes ACL policy to Org1/Admins")
		policyName = resources.Lscc_GetInstantiatedChaincodes
		policy = "/Channel/Application/Org1/Admins"
		SetACLPolicy(network, "testchannel", policyName, policy, "orderer")

		By("listing the instantiated chaincodes as a permitted Org1 Admin identity")
		sess, err = network.PeerAdminSession(org1Peer0, commands.ChaincodeListInstantiatedLegacy{
			ChannelID: "testchannel",
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
		Expect(sess).To(gbytes.Say("Name: mycc, Version: 0.0, Path: .*, Escc: escc, Vscc: vscc"))

		//
		// when the ACL policy for lscc/GetInstantiatedChaincodes is not satisfied
		//
		By("listing the instantiated chaincodes as a forbidden org2 Admin identity")
		sess, err = network.PeerAdminSession(org2Peer0, commands.ChaincodeListInstantiatedLegacy{
			ChannelID: "testchannel",
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit())
		Expect(sess).NotTo(gbytes.Say("Name: mycc, Version: 0.0, Path: .*, Escc: escc, Vscc: vscc"))
		Expect(sess.Err).To(gbytes.Say(`access denied for \[getchaincodes\]\[testchannel\](.*)signature set did not satisfy policy`))

		//
		// when a system chaincode ACL policy is set and a query is performed
		//

		// getting a transaction id from a block in the ledger
		sess, err = network.PeerAdminSession(org1Peer0, fetchNewest)
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
		Expect(sess.Err).To(gbytes.Say("Received block: "))
		txID := GetTxIDFromBlockFile(outputBlock)

		ItEnforcesPolicy := func(scc, operation string, args ...string) {
			policyName := fmt.Sprintf("%s/%s", scc, operation)
			policy := "/Channel/Application/Org1/Admins"
			By("setting " + policyName + " to Org1 Admins")
			SetACLPolicy(network, "testchannel", policyName, policy, "orderer")

			args = append([]string{operation}, args...)
			chaincodeQuery := commands.ChaincodeQuery{
				ChannelID: "testchannel",
				Name:      scc,
				Ctor:      ToCLIChaincodeArgs(args...),
			}

			By("evaluating " + policyName + " for a permitted subject")
			sess, err := network.PeerAdminSession(org1Peer0, chaincodeQuery)
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))

			By("evaluating " + policyName + " for a forbidden subject")
			sess, err = network.PeerAdminSession(org2Peer0, chaincodeQuery)
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit())
			Expect(sess.Err).To(gbytes.Say(fmt.Sprintf(`access denied for \[%s\]\[%s\](.*)signature set did not satisfy policy`, operation, "testchannel")))
		}

		//
		// qscc
		//
		ItEnforcesPolicy("qscc", "GetChainInfo", "testchannel")
		ItEnforcesPolicy("qscc", "GetBlockByNumber", "testchannel", "0")
		ItEnforcesPolicy("qscc", "GetBlockByTxID", "testchannel", txID)
		ItEnforcesPolicy("qscc", "GetTransactionByID", "testchannel", txID)

		//
		// lscc
		//
		ItEnforcesPolicy("lscc", "GetChaincodeData", "testchannel", "mycc")
		ItEnforcesPolicy("lscc", "ChaincodeExists", "testchannel", "mycc")

		//
		// cscc
		//
		ItEnforcesPolicy("cscc", "GetConfigBlock", "testchannel")
		ItEnforcesPolicy("cscc", "GetChannelConfig", "testchannel")

		//
		// _lifecycle ACL policies
		//

		chaincode = nwo.Chaincode{
			Name:                "mycc",
			Version:             "0.0",
			Path:                components.Build("github.com/hyperledger/fabric/integration/chaincode/simple/cmd"),
			Lang:                "binary",
			PackageFile:         filepath.Join(testDir, "simplecc.tar.gz"),
			Ctor:                `{"Args":["init","a","100","b","200"]}`,
			ChannelConfigPolicy: "/Channel/Application/Endorsement",
			Sequence:            "1",
			InitRequired:        true,
			Label:               "my_prebuilt_chaincode",
		}

		nwo.PackageChaincodeBinary(chaincode)

		//
		// when the ACL policy for _lifecycle/InstallChaincode is not satisfied
		//
		By("installing the chaincode to an org1 peer as an org2 admin")
		sess, err = network.PeerAdminSession(org2Peer0, commands.ChaincodeInstall{
			PackageFile:   chaincode.PackageFile,
			PeerAddresses: []string{network.PeerAddress(org1Peer0, nwo.ListenPort)},
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit())
		Expect(sess.Err).To(gbytes.Say(`access denied: channel \[\] creator org unknown, creator is malformed`))

		By("installing the chaincode to an org1 peer as a non-admin org1 identity")
		sess, err = network.PeerUserSession(org1Peer0, "User1", commands.ChaincodeInstall{
			PackageFile: chaincode.PackageFile,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit())
		Expect(sess.Err).To(gbytes.Say(`Error: chaincode install failed with status: 500 - Failed to authorize invocation due to failed ACL check: Failed verifying that proposal's creator satisfies local MSP principal during channelless check policy with policy \Q[Admins]\E: \Q[The identity is not an admin under this MSP [Org1MSP]: The identity does not contain OU [ADMIN], MSP: [Org1MSP]]\E`))

		//
		// when the ACL policy for _lifecycle/InstallChaincode is satisfied
		//
		nwo.InstallChaincode(network, chaincode, org1Peer0, org2Peer0)

		//
		// when the V2_0 application capabilities flag has not yet been enabled
		//
		By("approving a chaincode definition on a channel without V2_0 capabilities enabled")
		sess, err = network.PeerAdminSession(org1Peer0, commands.ChaincodeApproveForMyOrg{
			ChannelID:           "testchannel",
			Orderer:             network.OrdererAddress(orderer, nwo.ListenPort),
			Name:                chaincode.Name,
			Version:             chaincode.Version,
			Sequence:            chaincode.Sequence,
			EndorsementPlugin:   chaincode.EndorsementPlugin,
			ValidationPlugin:    chaincode.ValidationPlugin,
			SignaturePolicy:     chaincode.SignaturePolicy,
			ChannelConfigPolicy: chaincode.ChannelConfigPolicy,
			InitRequired:        chaincode.InitRequired,
			CollectionsConfig:   chaincode.CollectionsConfig,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit())
		Expect(sess.Err).To(gbytes.Say("Error: proposal failed with status: 500 - cannot use new lifecycle for channel 'testchannel' as it does not have the required capabilities enabled"))

		By("committing a chaincode definition on a channel without V2_0 capabilities enabled")
		sess, err = network.PeerAdminSession(org1Peer0, commands.ChaincodeCommit{
			ChannelID:           "testchannel",
			Orderer:             network.OrdererAddress(orderer, nwo.ListenPort),
			Name:                chaincode.Name,
			Version:             chaincode.Version,
			Sequence:            chaincode.Sequence,
			EndorsementPlugin:   chaincode.EndorsementPlugin,
			ValidationPlugin:    chaincode.ValidationPlugin,
			SignaturePolicy:     chaincode.SignaturePolicy,
			ChannelConfigPolicy: chaincode.ChannelConfigPolicy,
			InitRequired:        chaincode.InitRequired,
			CollectionsConfig:   chaincode.CollectionsConfig,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit())
		Expect(sess.Err).To(gbytes.Say("Error: proposal failed with status: 500 - cannot use new lifecycle for channel 'testchannel' as it does not have the required capabilities enabled"))

		// enable V2_0 application capabilities on the channel
		By("enabling V2_0 application capabilities on the channel")
		nwo.EnableCapabilities(network, "testchannel", "Application", "V2_0", orderer, org1Peer0, org2Peer0)

		//
		// when the ACL policy for _lifecycle/ApproveChaincodeDefinitionForOrg is not satisfied
		//
		By("approving a chaincode definition for org1 as an org2 admin")
		sess, err = network.PeerAdminSession(org2Peer0, commands.ChaincodeApproveForMyOrg{
			ChannelID:           "testchannel",
			Orderer:             network.OrdererAddress(orderer, nwo.ListenPort),
			Name:                chaincode.Name,
			Version:             chaincode.Version,
			Sequence:            chaincode.Sequence,
			EndorsementPlugin:   chaincode.EndorsementPlugin,
			ValidationPlugin:    chaincode.ValidationPlugin,
			SignaturePolicy:     chaincode.SignaturePolicy,
			ChannelConfigPolicy: chaincode.ChannelConfigPolicy,
			InitRequired:        chaincode.InitRequired,
			CollectionsConfig:   chaincode.CollectionsConfig,
			PeerAddresses:       []string{network.PeerAddress(org1Peer0, nwo.ListenPort)},
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit())
		Expect(sess.Err).To(gbytes.Say(`Error: proposal failed with status: 500 - Failed to authorize invocation due to failed ACL check: Failed deserializing proposal creator during channelless check policy with policy \[Admins\]: \[expected MSP ID Org1MSP, received Org2MSP\]`))

		//
		// when the ACL policy for _lifecycle/ApproveChaincodeDefinitionForOrg is satisfied
		//
		By("approving a chaincode definition for org1 and org2")
		nwo.ApproveChaincodeForMyOrg(network, "testchannel", orderer, chaincode, org1Peer0, org2Peer0)

		//
		// when the ACL policy for _lifecycle/QueryApprovedChaincodeDefinition is not satisfied
		//
		By("querying the approved chaincode definition for org1 as an org2 admin")
		sess, err = network.PeerAdminSession(org2Peer0, commands.ChaincodeQueryApproved{
			ChannelID:     "testchannel",
			Name:          chaincode.Name,
			Sequence:      chaincode.Sequence,
			PeerAddresses: []string{network.PeerAddress(org1Peer0, nwo.ListenPort)},
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit())
		Expect(sess.Err).To(gbytes.Say(`\QError: query failed with status: 500 - Failed to authorize invocation due to failed ACL check: Failed deserializing proposal creator during channelless check policy with policy [Admins]: [expected MSP ID Org1MSP, received Org2MSP]\E`))

		//
		// when the ACL policy for _lifecycle/QueryApprovedChaincodeDefinition is satisfied
		//
		By("querying the approved chaincode definition for org1 as an org1 admin")
		nwo.EnsureChaincodeApproved(network, org1Peer0, "testchannel", chaincode.Name, chaincode.Sequence)

		//
		// when the ACL policy for CheckCommitReadiness is not satisfied
		//
		By("setting the simulate commit chaincode definition ACL policy to Org1/Admins")
		policyName = resources.Lifecycle_CheckCommitReadiness
		policy = "/Channel/Application/Org1/Admins"
		SetACLPolicy(network, "testchannel", policyName, policy, "orderer")

		By("simulating the commit of a chaincode dwefinition as a forbidden Org2 Admin identity")
		sess, err = network.PeerAdminSession(org2Peer0, commands.ChaincodeCheckCommitReadiness{
			ChannelID:           "testchannel",
			Name:                chaincode.Name,
			Version:             chaincode.Version,
			Sequence:            chaincode.Sequence,
			EndorsementPlugin:   chaincode.EndorsementPlugin,
			ValidationPlugin:    chaincode.ValidationPlugin,
			SignaturePolicy:     chaincode.SignaturePolicy,
			ChannelConfigPolicy: chaincode.ChannelConfigPolicy,
			InitRequired:        chaincode.InitRequired,
			CollectionsConfig:   chaincode.CollectionsConfig,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit())
		Expect(sess.Err).To(gbytes.Say(`\QError: query failed with status: 500 - Failed to authorize invocation due to failed ACL check: failed evaluating policy on signed data during check policy [/Channel/Application/Org1/Admins]: [signature set did not satisfy policy]\E`))

		//
		// when the ACL policy for CheckCommitReadiness is satisfied
		//
		nwo.CheckCommitReadinessUntilReady(network, "testchannel", chaincode, network.PeerOrgs(), org1Peer0)

		//
		// when the ACL policy for CommitChaincodeDefinition is not satisfied
		//
		By("setting the commit chaincode definition ACL policy to Org1/Admins")
		policyName = resources.Lifecycle_CommitChaincodeDefinition
		policy = "/Channel/Application/Org1/Admins"
		SetACLPolicy(network, "testchannel", policyName, policy, "orderer")

		By("committing the chaincode definition as a forbidden Org2 Admin identity")
		peerAddresses := []string{
			network.PeerAddress(org1Peer0, nwo.ListenPort),
			network.PeerAddress(org2Peer0, nwo.ListenPort),
		}
		sess, err = network.PeerAdminSession(org2Peer0, commands.ChaincodeCommit{
			ChannelID:           "testchannel",
			Orderer:             network.OrdererAddress(orderer, nwo.ListenPort),
			Name:                chaincode.Name,
			Version:             chaincode.Version,
			Sequence:            chaincode.Sequence,
			EndorsementPlugin:   chaincode.EndorsementPlugin,
			ValidationPlugin:    chaincode.ValidationPlugin,
			SignaturePolicy:     chaincode.SignaturePolicy,
			ChannelConfigPolicy: chaincode.ChannelConfigPolicy,
			InitRequired:        chaincode.InitRequired,
			CollectionsConfig:   chaincode.CollectionsConfig,
			PeerAddresses:       peerAddresses,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit())
		Expect(sess.Err).To(gbytes.Say(`\QError: proposal failed with status: 500 - Failed to authorize invocation due to failed ACL check: failed evaluating policy on signed data during check policy [/Channel/Application/Org1/Admins]: [signature set did not satisfy policy]\E`))

		//
		// when the ACL policy for CommitChaincodeDefinition is satisfied
		//
		nwo.CommitChaincode(network, "testchannel", orderer, chaincode, org1Peer0, org1Peer0, org2Peer0)

		//
		// when the ACL policy for QueryChaincodeDefinition is satisfied
		//
		By("setting the query chaincode definition ACL policy to Org1/Admins")
		policyName = resources.Lifecycle_QueryChaincodeDefinition
		policy = "/Channel/Application/Org1/Admins"
		SetACLPolicy(network, "testchannel", policyName, policy, "orderer")

		By("querying the chaincode definition as a permitted Org1 Admin identity")
		nwo.EnsureChaincodeCommitted(network, "testchannel", "mycc", "0.0", "1", []*nwo.Organization{network.Organization("Org1"), network.Organization("Org2")}, org1Peer0)

		By("querying the chaincode definition as a forbidden Org2 Admin identity")
		sess, err = network.PeerAdminSession(org2Peer0, commands.ChaincodeListCommitted{
			ChannelID: "testchannel",
			Name:      "mycc",
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit())
		Expect(sess.Err).To(gbytes.Say(`\QError: query failed with status: 500 - Failed to authorize invocation due to failed ACL check: failed evaluating policy on signed data during check policy [/Channel/Application/Org1/Admins]: [signature set did not satisfy policy]\E`))

		//
		// when the ACL policy for snapshot related commands is not satisfied (e.g., non-admin user or admin in another org)
		//
		By("calling snapshot submitrequest as a non-permitted identity")
		expectedMsgNonAdmin := `\Qfailed verifying that the signed data identity satisfies local MSP principal during channelless check policy with policy [Admins]: [The identity is not an admin under this MSP [Org1MSP]: The identity does not contain OU [ADMIN], MSP: [Org1MSP]]\E`
		expectedMsgAdminWrongOrg := `\Qfailed deserializing signed data identity during channelless check policy with policy [Admins]: [expected MSP ID Org1MSP, received Org2MSP]\E`
		submitrequestCmd := &commands.SnapshotSubmitRequest{
			ChannelID:   "testchannel",
			BlockNumber: "100",
			ClientAuth:  network.ClientAuthRequired,
			PeerAddress: network.PeerAddress(org1Peer0, nwo.ListenPort),
		}
		verifyCommandErr(network, org1Peer0, "User1", submitrequestCmd, expectedMsgNonAdmin)
		verifyCommandErr(network, org2Peer0, "Admin", submitrequestCmd, expectedMsgAdminWrongOrg)

		By("calling snapshot cancelrequest as a non-permitted identity")
		cancelrequestCmd := &commands.SnapshotCancelRequest{
			ChannelID:   "testchannel",
			BlockNumber: "100",
			ClientAuth:  network.ClientAuthRequired,
			PeerAddress: network.PeerAddress(org1Peer0, nwo.ListenPort),
		}
		verifyCommandErr(network, org1Peer0, "User1", cancelrequestCmd, expectedMsgNonAdmin)
		verifyCommandErr(network, org2Peer0, "Admin", cancelrequestCmd, expectedMsgAdminWrongOrg)

		By("calling snapshot listpending as a non-permitted identity")
		listpendingCmd := &commands.SnapshotListPending{
			ChannelID:   "testchannel",
			ClientAuth:  network.ClientAuthRequired,
			PeerAddress: network.PeerAddress(org1Peer0, nwo.ListenPort),
		}
		verifyCommandErr(network, org1Peer0, "User1", listpendingCmd, expectedMsgNonAdmin)
		verifyCommandErr(network, org2Peer0, "Admin", listpendingCmd, expectedMsgAdminWrongOrg)

		By("calling joinbysnapshot as a non-permitted identity")
		expectedMsgNonAdmin = `\QFailed verifying that proposal's creator satisfies local MSP principal during channelless check policy with policy [Admins]: [The identity is not an admin under this MSP [Org1MSP]: The identity does not contain OU [ADMIN], MSP: [Org1MSP]]\E`
		joinbysnapshotCmd := commands.ChannelJoinBySnapshot{
			SnapshotPath: "/tmp/dummy_dir",
			ClientAuth:   network.ClientAuthRequired,
		}
		verifyCommandErr(network, org1Peer0, "User1", joinbysnapshotCmd, expectedMsgNonAdmin)

		By("calling joinbysnapshotstatus as a non-permitted identity")
		joinbysnapshotstatusCmd := commands.ChannelJoinBySnapshotStatus{
			ClientAuth: network.ClientAuthRequired,
		}
		verifyCommandErr(network, org1Peer0, "User1", joinbysnapshotstatusCmd, expectedMsgNonAdmin)
	})
})

// SetACLPolicy sets the ACL policy for a running network. It resets all
// previously defined ACL policies, generates the config update, signs the
// configuration with Org2's signer, and then submits the config update using
// Org1.
func SetACLPolicy(network *nwo.Network, channel, policyName, policy string, ordererName string) {
	orderer := network.Orderer(ordererName)
	submitter := network.Peer("Org1", "peer0")
	signer := network.Peer("Org2", "peer0")

	config := nwo.GetConfig(network, submitter, orderer, channel)
	updatedConfig := proto.Clone(config).(*common.Config)

	// set the policy
	updatedConfig.ChannelGroup.Groups["Application"].Values["ACLs"] = &common.ConfigValue{
		ModPolicy: "Admins",
		Value: protoutil.MarshalOrPanic(&pb.ACLs{
			Acls: map[string]*pb.APIResource{
				policyName: {PolicyRef: policy},
			},
		}),
	}

	nwo.UpdateConfig(network, orderer, channel, config, updatedConfig, true, submitter, signer)
}

// GetTxIDFromBlock gets a transaction id from a block that has been
// marshaled and stored on the filesystem
func GetTxIDFromBlockFile(blockFile string) string {
	block := nwo.UnmarshalBlockFromFile(blockFile)

	txID, err := protoutil.GetOrComputeTxIDFromEnvelope(block.Data.Data[0])
	Expect(err).NotTo(HaveOccurred())

	return txID
}

// ToCLIChaincodeArgs converts string args to args for use with chaincode calls
// from the CLI.
func ToCLIChaincodeArgs(args ...string) string {
	type cliArgs struct {
		Args []string
	}
	cArgs := &cliArgs{Args: args}
	cArgsJSON, err := json.Marshal(cArgs)
	Expect(err).NotTo(HaveOccurred())
	return string(cArgsJSON)
}

func verifyCommandErr(network *nwo.Network, peer *nwo.Peer, user string, cmd nwo.Command, expectedMsg string) {
	sess, err := network.PeerUserSession(peer, user, cmd)
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit())
	Expect(sess.Err).To(gbytes.Say(expectedMsg))
}
