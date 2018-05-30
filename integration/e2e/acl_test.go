/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package e2e

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/gogo/protobuf/proto"
	"github.com/hyperledger/fabric/common/tools/configtxlator/update"
	"github.com/hyperledger/fabric/core/aclmgmt/resources"
	"github.com/hyperledger/fabric/integration/runner"
	"github.com/hyperledger/fabric/integration/world"
	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("EndToEndACL", func() {
	var (
		w          *world.World
		deployment world.Deployment
		org1Peer0  *runner.Peer
	)

	BeforeEach(func() {
		w = world.GenerateBasicConfig("solo", 2, 2, testDir, components)

		// sets up the world for all tests
		deployment = world.Deployment{
			Channel: "testchannel",
			Chaincode: world.Chaincode{
				Name:     "mycc",
				Version:  "0.0",
				Path:     "github.com/hyperledger/fabric/integration/chaincode/simple/cmd",
				ExecPath: os.Getenv("PATH"),
			},
			InitArgs: `{"Args":["init","a","100","b","200"]}`,
			Policy:   `OR ('Org1MSP.member','Org2MSP.member')`,
			Orderer:  "127.0.0.1:7050",
		}
		w.BootstrapNetwork(deployment.Channel)
		copyFile(filepath.Join("testdata", "orderer.yaml"), filepath.Join(testDir, "orderer.yaml"))
		copyPeerConfigs(w.PeerOrgs, w.Rootpath)
		w.BuildNetwork()
		err := w.SetupChannel(deployment, []string{"peer0.org1.example.com", "peer0.org2.example.com"})
		Expect(err).NotTo(HaveOccurred())

		org1Peer0 = components.Peer()
		org1Peer0.ConfigDir = filepath.Join(w.Rootpath, "peer0.org1.example.com")
		org1Peer0.MSPConfigPath = filepath.Join(w.Rootpath, "crypto", "peerOrganizations", "org1.example.com", "users", "Admin@org1.example.com", "msp")
	})

	AfterEach(func() {
		w.Close(deployment)
	})

	It("enforces access control list policies", func() {
		//
		// when the ACL policy for DeliverFiltered is satisified
		//
		By("setting the filtered block event ACL policy to Org1/Admins")
		policyName := resources.Event_FilteredBlock
		policy := "/Channel/Application/Org1/Admins"
		SetACLPolicy(w, deployment, policyName, policy)

		By("invoking chaincode as a permitted Org1 Admin identity")
		adminRunner := org1Peer0.InvokeChaincode(deployment.Chaincode.Name, deployment.Channel, `{"Args":["invoke","a","b","10"]}`, deployment.Orderer, "--waitForEvent")
		execute(adminRunner)
		Eventually(adminRunner.Err()).Should(gbytes.Say("Chaincode invoke successful. result: status:200"))

		//
		// when the ACL policy for DeliverFiltered is not satisifed
		//
		By("setting the filtered block event ACL policy to Org2/Admins")
		policyName = resources.Event_FilteredBlock
		policy = "/Channel/Application/Org2/Admins"
		SetACLPolicy(w, deployment, policyName, policy)

		By("invoking chaincode as a forbidden Org1 Admin identity")
		adminRunner = org1Peer0.InvokeChaincode(deployment.Chaincode.Name, deployment.Channel, `{"Args":["invoke","a","b","10"]}`, deployment.Orderer, "--waitForEvent")
		execute(adminRunner)
		Eventually(adminRunner.Err()).Should(gbytes.Say(`\Qdeliver completed with status (FORBIDDEN)\E`))

		//
		// when the ACL policy for Deliver is satisfied
		//
		By("setting the block event ACL policy to Org1/Admins")
		policyName = resources.Event_Block
		policy = "/Channel/Application/Org1/Admins"
		SetACLPolicy(w, deployment, policyName, policy)

		By("setting the log level for deliver to debug")
		lRunner := org1Peer0.SetLogLevel("common/deliver", "debug")
		execute(lRunner)
		Expect(lRunner.Err()).To(gbytes.Say("Log level set for peer modules matching regular expression 'common/deliver': DEBUG"))

		By("fetching the latest block from the peer as a permitted Org1 Admin identity")
		fRunner := org1Peer0.FetchChannel(deployment.Channel, filepath.Join(testDir, "newest_block.pb"), "newest", "")
		execute(fRunner)
		Expect(fRunner.Err()).To(gbytes.Say("Received block: "))
		// TODO - enable this once the peer's logs are available here
		// Expect(peerRunner.Err()).To(gbytes.Say(`\Q[channel: testchannel] Done delivering \E`))

		//
		// when the ACL policy for Deliver is not satisifed
		//
		By("setting the block event ACL policy to Org2/Admins")
		policyName = resources.Event_Block
		policy = "/Channel/Application/Org2/Admins"
		SetACLPolicy(w, deployment, policyName, policy)

		By("fetching the latest block from the peer as a forbidden Org1 Admin identity")
		fRunner = org1Peer0.FetchChannel(deployment.Channel, filepath.Join(testDir, "newest_block.pb"), "newest", "")
		execute(fRunner)
		Expect(fRunner.Err()).To(gbytes.Say("can't read the block: &{FORBIDDEN}"))
		// TODO - enable this once the peer's logs are available here
		// Expect(peerRunner.Err()).To(gbytes.Say(`\Q[channel: testchannel] Done delivering \Q`))

		By("setting the log level for deliver to back to info")
		lRunner = org1Peer0.SetLogLevel("common/deliver", "info")
		execute(lRunner)
		Expect(lRunner.Err()).To(gbytes.Say("Log level set for peer modules matching regular expression 'common/deliver': INFO"))
	})
})

// SetACLPolicy sets the ACL policy for the world on a running network. It resets all
// previously defined ACL policies. It performs the generation of the config update,
// signs the configuration with Org2's signer, and then submits the config update
// using Org1
func SetACLPolicy(w *world.World, deployment world.Deployment, policyName, policy string) {
	outputFile := filepath.Join(testDir, "updated_config.pb")
	GenerateACLConfigUpdate(w, deployment, policyName, policy, outputFile)

	signConfigDir := filepath.Join(testDir, "peer0.org2.example.com")
	signMSPConfigPath := filepath.Join(w.Rootpath, "crypto", "peerOrganizations", "org2.example.com", "users", "Admin@org2.example.com", "msp")
	SignConfigUpdate(w, outputFile, signConfigDir, signMSPConfigPath)

	sendConfigDir := filepath.Join(testDir, "peer0.org1.example.com")
	sendMSPConfigPath := filepath.Join(w.Rootpath, "crypto", "peerOrganizations", "org1.example.com", "users", "Admin@org1.example.com", "msp")
	SendConfigUpdate(deployment, outputFile, sendConfigDir, sendMSPConfigPath)
}

func GenerateACLConfigUpdate(w *world.World, deployment world.Deployment, policyName, policy, outputFile string) {
	// fetch the config block
	fetchRun := components.Peer()
	fetchRun.ConfigDir = filepath.Join(testDir, "peer0.org1.example.com")
	fetchRun.MSPConfigPath = filepath.Join(w.Rootpath, "crypto", "peerOrganizations", "org1.example.com", "users", "Admin@org1.example.com", "msp")
	output := filepath.Join(testDir, "config_block.pb")
	fRunner := fetchRun.FetchChannel(deployment.Channel, output, "config", deployment.Orderer)
	execute(fRunner)
	Expect(fRunner.Err()).To(gbytes.Say("Received block: "))
	// TODO - enable this once the orderer's logs are available here
	// Expect(ordererRunner.Err()).To(gbytes.Say(`\Q[channel: mychan] Done delivering \E`))

	// read the config block file
	fileBytes, err := ioutil.ReadFile(output)
	Expect(err).To(BeNil())
	Expect(fileBytes).NotTo(BeNil())

	// unmarshal the config block bytes
	configBlock := &common.Block{}
	err = proto.Unmarshal(fileBytes, configBlock)
	Expect(err).To(BeNil())

	// unmarshal the envelope bytes
	env := &common.Envelope{}
	err = proto.Unmarshal(configBlock.Data.Data[0], env)
	Expect(err).To(BeNil())

	// unmarshal the payload bytes
	payload := &common.Payload{}
	err = proto.Unmarshal(env.Payload, payload)
	Expect(err).To(BeNil())

	// unmarshal the config envelope bytes
	configEnv := &common.ConfigEnvelope{}
	err = proto.Unmarshal(payload.Data, configEnv)
	Expect(err).To(BeNil())

	// clone the config
	config := configEnv.Config
	updatedConfig := proto.Clone(config).(*common.Config)

	acls := &pb.ACLs{
		Acls: make(map[string]*pb.APIResource),
	}

	// set the policy
	apiResource := &pb.APIResource{}
	apiResource.PolicyRef = policy
	acls.Acls[policyName] = apiResource
	cv := &common.ConfigValue{
		Value:     utils.MarshalOrPanic(acls),
		ModPolicy: "Admins",
	}
	updatedConfig.ChannelGroup.Groups["Application"].Values["ACLs"] = cv

	// compute the config update and package it into an envelope
	configUpdate, err := update.Compute(config, updatedConfig)
	Expect(err).To(BeNil())
	configUpdate.ChannelId = deployment.Channel
	configUpdateEnvelope := &common.ConfigUpdateEnvelope{
		ConfigUpdate: utils.MarshalOrPanic(configUpdate),
	}
	signedEnv, err := utils.CreateSignedEnvelope(common.HeaderType_CONFIG_UPDATE, deployment.Channel, nil, configUpdateEnvelope, 0, 0)
	Expect(err).To(BeNil())
	Expect(signedEnv).NotTo(BeNil())

	// write the config update envelope to the file system
	signedEnvelopeBytes := utils.MarshalOrPanic(signedEnv)
	err = ioutil.WriteFile(outputFile, signedEnvelopeBytes, 0660)
	Expect(err).To(BeNil())
}

func SignConfigUpdate(w *world.World, outputFile, configDir, mspConfigPath string) {
	signRun := components.Peer()
	signRun.ConfigDir = configDir
	signRun.MSPConfigPath = mspConfigPath
	sRunner := signRun.SignConfigTx(outputFile)
	// signconfigtx doesn't output anything so check that error is nil
	err := execute(sRunner)
	Expect(err).To(BeNil())
}

func SendConfigUpdate(deployment world.Deployment, outputFile, configDir, mspConfigPath string) {
	updateRun := components.Peer()
	updateRun.ConfigDir = configDir
	updateRun.MSPConfigPath = mspConfigPath
	uRunner := updateRun.UpdateChannel(outputFile, deployment.Channel, deployment.Orderer)
	execute(uRunner)
	Expect(uRunner.Err()).To(gbytes.Say("Successfully submitted channel update"))
}
