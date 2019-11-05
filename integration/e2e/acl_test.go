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
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/aclmgmt/resources"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
)

var _ = Describe("EndToEndACL", func() {
	var (
		testDir   string
		client    *docker.Client
		network   *nwo.Network
		chaincode nwo.Chaincode
		process   ifrit.Process

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
		soloConfig := nwo.BasicSolo()
		soloConfig.RemovePeer("Org1", "peer1")
		soloConfig.RemovePeer("Org2", "peer1")
		Expect(soloConfig.Peers).To(HaveLen(2))

		network = nwo.New(soloConfig, testDir, client, BasePort(), components)
		network.GenerateConfigTree()
		network.Bootstrap()

		networkRunner := network.NetworkGroupRunner()
		process = ifrit.Invoke(networkRunner)
		Eventually(process.Ready(), network.EventuallyTimeout).Should(BeClosed())

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
		network.CreateAndJoinChannel(orderer, "testchannel")
		nwo.DeployChaincode(network, "testchannel", orderer, chaincode)
	})

	AfterEach(func() {
		process.Signal(syscall.SIGTERM)
		Eventually(process.Wait(), network.EventuallyTimeout).Should(Receive())
		network.Cleanup()
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
		// when the ACL policy for DeliverFiltered is satisified
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
		// when the ACL policy for DeliverFiltered is not satisifed
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
		// when the ACL policy for Deliver is not satisifed
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
		sess, err = network.PeerAdminSession(org1Peer0, commands.ChaincodeListInstantiated{
			ChannelID: "testchannel",
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
		Expect(sess).To(gbytes.Say("Name: mycc, Version: 0.0, Path: .*, Escc: escc, Vscc: vscc"))

		//
		// when the ACL policy for lscc/GetInstantiatedChaincodes is not satisfied
		//
		By("listing the instantiated chaincodes as a forbidden org2 Admin identity")
		sess, err = network.PeerAdminSession(org2Peer0, commands.ChaincodeListInstantiated{
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
			Eventually(sess, 30*time.Second).Should(gexec.Exit(0))

			By("evaluating " + policyName + " for a forbidden subject")
			sess, err = network.PeerAdminSession(org2Peer0, chaincodeQuery)
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, 30*time.Second).Should(gexec.Exit())
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
		ItEnforcesPolicy("cscc", "GetConfigTree", "testchannel")
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
		Value: utils.MarshalOrPanic(&pb.ACLs{
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

	txID, err := utils.GetOrComputeTxIDFromEnvelope(block.Data.Data[0])
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
