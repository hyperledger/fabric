/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"io/ioutil"
	"os"
	"syscall"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	"github.com/hyperledger/fabric/pkg/config"
	"github.com/tedsuo/ifrit"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Config", func() {
	var (
		client  *docker.Client
		testDir string
		network *nwo.Network
		process ifrit.Process
	)

	BeforeEach(func() {
		var err error
		testDir, err = ioutil.TempDir("", "config")
		Expect(err).NotTo(HaveOccurred())

		client, err = docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())

		network = nwo.New(nwo.BasicSolo(), testDir, client, StartPort(), components)

		// Generate config
		network.GenerateConfigTree()

		// bootstrap the network
		network.Bootstrap()

		networkRunner := network.NetworkGroupRunner()
		process = ifrit.Invoke(networkRunner)
		Eventually(process.Ready(), network.EventuallyTimeout).Should(BeClosed())
	})

	AfterEach(func() {
		if process != nil {
			process.Signal(syscall.SIGTERM)
			Eventually(process.Wait(), network.EventuallyTimeout).Should(Receive())
		}
		if network != nil {
			network.Cleanup()
		}

		os.RemoveAll(testDir)
	})

	It("creates channels and updates them using pkg/config", func() {
		orderer := network.Orderer("orderer")
		testPeers := network.PeersWithChannel("testchannel")
		org1peer0 := network.Peer("Org1", "peer0")

		By("setting up the channel")
		channel := config.Channel{
			ChannelID:  "testchannel",
			Consortium: "SampleConsortium",
			Application: config.Application{
				Organizations: []config.Organization{
					{
						Name: "Org1",
					},
					{
						Name: "Org2",
					},
				},
				Capabilities: map[string]bool{"V1_3": true},
				ACLs:         map[string]string{"event/Block": "/Channel/Application/Readers"},
				Policies: map[string]*config.Policy{
					config.ReadersPolicyKey: {
						Type: config.ImplicitMetaPolicyType,
						Rule: "ANY Readers",
					},
					config.WritersPolicyKey: {
						Type: config.ImplicitMetaPolicyType,
						Rule: "ANY Writers",
					},
					config.AdminsPolicyKey: {
						Type: config.ImplicitMetaPolicyType,
						Rule: "MAJORITY Admins",
					},
					config.EndorsementPolicyKey: {
						Type: config.ImplicitMetaPolicyType,
						Rule: "MAJORITY Endorsement",
					},
					config.LifecycleEndorsementPolicyKey: {
						Type: config.ImplicitMetaPolicyType,
						Rule: "MAJORITY Endorsement",
					},
				},
			},
		}

		envelope, err := config.NewCreateChannelTx(channel)
		Expect(err).NotTo(HaveOccurred())
		envBytes, err := proto.Marshal(envelope)
		Expect(err).NotTo(HaveOccurred())
		channelCreateTxPath := network.CreateChannelTxPath("testchannel")
		err = ioutil.WriteFile(channelCreateTxPath, envBytes, 0644)
		Expect(err).NotTo(HaveOccurred())

		By("creating the channel")
		createChannel := func() int {
			sess, err := network.PeerAdminSession(org1peer0, commands.ChannelCreate{
				ChannelID:   "testchannel",
				Orderer:     network.OrdererAddress(orderer, nwo.ListenPort),
				File:        channelCreateTxPath,
				OutputBlock: "/dev/null",
				ClientAuth:  network.ClientAuthRequired,
			})
			Expect(err).NotTo(HaveOccurred())
			return sess.Wait(network.EventuallyTimeout).ExitCode()
		}
		Eventually(createChannel, network.EventuallyTimeout).Should(Equal(0))

		By("joining all peers to the channel")
		network.JoinChannel("testchannel", orderer, testPeers...)
	})
})
