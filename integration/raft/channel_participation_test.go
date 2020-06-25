/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package raft

import (
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric-config/configtx"
	"github.com/hyperledger/fabric-config/configtx/orderer"
	"github.com/hyperledger/fabric-protos-go/common"
	conftx "github.com/hyperledger/fabric/integration/configtx"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/ginkgomon"
)

var _ = Describe("ChannelParticipation", func() {
	var (
		testDir        string
		client         *docker.Client
		network        *nwo.Network
		ordererProcess ifrit.Process
		ordererRunner  *ginkgomon.Runner
	)

	BeforeEach(func() {
		var err error
		testDir, err = ioutil.TempDir("", "channel-participation")
		Expect(err).NotTo(HaveOccurred())

		client, err = docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if ordererProcess != nil {
			ordererProcess.Signal(syscall.SIGTERM)
			Eventually(ordererProcess.Wait(), network.EventuallyTimeout).Should(Receive())
		}
		if network != nil {
			network.Cleanup()
		}
		os.RemoveAll(testDir)
	})

	Describe("basic etcdraft network without a system channel", func() {
		BeforeEach(func() {
			raftConfig := nwo.BasicEtcdRaft()
			network = nwo.New(raftConfig, testDir, client, StartPort(), components)
			network.ChannelParticipationEnabled = true
			network.GenerateConfigTree()

			orderer := network.Orderer("orderer")
			ordererConfig := network.ReadOrdererConfig(orderer)
			ordererConfig.General.BootstrapMethod = "none"
			network.WriteOrdererConfig(orderer, ordererConfig)
			network.Bootstrap()

			ordererRunner = network.OrdererRunner(orderer)
			ordererProcess = ifrit.Invoke(ordererRunner)
			Eventually(ordererProcess.Ready, network.EventuallyTimeout).Should(BeClosed())
			Eventually(ordererRunner.Err(), network.EventuallyTimeout).Should(gbytes.Say("Registrar initializing without a system channel, number of application channels: 0"))

			nwo.ChannelParticipationList(network, orderer, nil)
		})

		It("starts the orderer but rejects channel creation requests via the legacy channel creation", func() {
			By("attempting to create a channel without a system channel defined")
			sess, err := network.PeerAdminSession(network.Peer("Org1", "peer0"), commands.ChannelCreate{
				ChannelID:   "testchannel",
				Orderer:     network.OrdererAddress(network.Orderer("orderer"), nwo.ListenPort),
				File:        network.CreateChannelTxPath("testchannel"),
				OutputBlock: "/dev/null",
				ClientAuth:  network.ClientAuthRequired,
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(1))
			Eventually(sess.Err, network.EventuallyTimeout).Should(gbytes.Say("channel creation request not allowed because the orderer system channel is not defined"))
		})

		It("joins application channels using the channel participation API from genesis block", func() {
			orderer := network.Orderer("orderer")
			peer := network.Peer("Org1", "peer0")

			By("joining channel as a member")
			genesisBlock := applicationChannelGenesisBlock(network, orderer, peer, "participation-trophy")
			expectedChannelInfo := nwo.ChannelInfo{
				Name:            "participation-trophy",
				URL:             "/participation/v1/channels/participation-trophy",
				Status:          "active",
				ClusterRelation: "member",
				Height:          1,
			}
			nwo.ChannelParticipationJoin(network, orderer, "participation-trophy", genesisBlock, expectedChannelInfo)
			nwo.ChannelParticipationListOne(network, orderer, expectedChannelInfo)

			By("waiting for the leader to be ready")
			findLeader([]*ginkgomon.Runner{ordererRunner})

			By("ensuring the channel is usable by submitting a transaction")
			env := CreateBroadcastEnvelope(network, peer, "participation-trophy", []byte("hello"))
			resp, err := nwo.Broadcast(network, orderer, env)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Status).To(Equal(common.Status_SUCCESS))
			waitForBlockReception(orderer, peer, network, "participation-trophy", 1)

			By("checking the channel height")
			expectedChannelInfo.Height = 2
			nwo.ChannelParticipationListOne(network, orderer, expectedChannelInfo)

			By("joining another channel as a member")
			genesisBlock2 := applicationChannelGenesisBlock(network, orderer, peer, "another-participation-trophy")
			expectedChannelInfo = nwo.ChannelInfo{
				Name:            "another-participation-trophy",
				URL:             "/participation/v1/channels/another-participation-trophy",
				Status:          "active",
				ClusterRelation: "member",
				Height:          1,
			}
			nwo.ChannelParticipationJoin(network, orderer, "another-participation-trophy", genesisBlock2, expectedChannelInfo)
			nwo.ChannelParticipationListOne(network, orderer, expectedChannelInfo)

			By("listing all channels")
			nwo.ChannelParticipationList(network, orderer, []string{"participation-trophy", "another-participation-trophy"})
		})
	})
})

func applicationChannelGenesisBlock(n *nwo.Network, o *nwo.Orderer, p *nwo.Peer, channel string) *common.Block {
	ordererCACert := nwo.ParseCertificate(n.OrdererCACert(o))
	ordererAdminCert := nwo.ParseCertificate(n.OrdererUserCert(o, "Admin"))
	ordererTLSCert := nwo.ParseCertificate(filepath.Join(n.OrdererLocalTLSDir(o), "server.crt"))
	ordererOrg := n.Organization(o.Organization)
	host, port := conftx.OrdererHostPort(n, o)

	peerOrg := n.Organization(p.Organization)
	peerCACert := nwo.ParseCertificate(n.PeerCACert(p))
	peerAdminCert := nwo.ParseCertificate(n.PeerUserCert(p, "Admin"))

	channelConfig := configtx.Channel{
		Orderer: configtx.Orderer{
			OrdererType: "etcdraft",
			Organizations: []configtx.Organization{
				{
					Name: o.Organization,
					Policies: map[string]configtx.Policy{
						"Readers": {
							Type: "Signature",
							Rule: fmt.Sprintf("OR('%s.member')", ordererOrg.MSPID),
						},
						"Writers": {
							Type: "Signature",
							Rule: fmt.Sprintf("OR('%s.member')", ordererOrg.MSPID),
						},
						"Admins": {
							Type: "Signature",
							Rule: fmt.Sprintf("OR('%s.member')", ordererOrg.MSPID),
						},
					},
					OrdererEndpoints: []string{
						n.OrdererAddress(o, nwo.ListenPort),
					},
					MSP: configtx.MSP{
						Name:      ordererOrg.MSPID,
						RootCerts: []*x509.Certificate{ordererCACert},
						Admins:    []*x509.Certificate{ordererAdminCert},
					},
				},
			},
			EtcdRaft: orderer.EtcdRaft{
				Consenters: []orderer.Consenter{
					{
						Address: orderer.EtcdAddress{
							Host: host,
							Port: port,
						},
						ClientTLSCert: ordererTLSCert,
						ServerTLSCert: ordererTLSCert,
					},
				},
				Options: orderer.EtcdRaftOptions{
					TickInterval:         "500ms",
					ElectionTick:         10,
					HeartbeatTick:        1,
					MaxInflightBlocks:    5,
					SnapshotIntervalSize: 16 * 1024 * 1024, // 16 MB
				},
			},
			Policies: map[string]configtx.Policy{
				"Readers": {
					Type: "ImplicitMeta",
					Rule: "ANY Readers",
				},
				"Writers": {
					Type: "ImplicitMeta",
					Rule: "ANY Writers",
				},
				"Admins": {
					Type: "ImplicitMeta",
					Rule: "MAJORITY Admins",
				},
				"BlockValidation": {
					Type: "ImplicitMeta",
					Rule: "ANY Writers",
				},
			},
			Capabilities: []string{"V2_0"},
			BatchSize: orderer.BatchSize{
				MaxMessageCount:   100,
				AbsoluteMaxBytes:  1024 * 1024,
				PreferredMaxBytes: 512 * 1024,
			},
			BatchTimeout: 2 * time.Second,
			State:        "STATE_NORMAL",
		},
		Application: configtx.Application{
			Organizations: []configtx.Organization{
				{
					Name: p.Organization,
					Policies: map[string]configtx.Policy{
						"Readers": {
							Type: "Signature",
							Rule: fmt.Sprintf("OR('%s.member')", peerOrg.MSPID),
						},
						"Writers": {
							Type: "Signature",
							Rule: fmt.Sprintf("OR('%s.member')", peerOrg.MSPID),
						},
						"Admins": {
							Type: "Signature",
							Rule: fmt.Sprintf("OR('%s.member')", peerOrg.MSPID),
						},
					},
					OrdererEndpoints: []string{
						n.OrdererAddress(o, nwo.ListenPort),
					},
					MSP: configtx.MSP{
						Name:      peerOrg.MSPID,
						RootCerts: []*x509.Certificate{peerCACert},
						Admins:    []*x509.Certificate{peerAdminCert},
					},
				},
			},
			Capabilities: []string{"V2_0"},
			Policies: map[string]configtx.Policy{
				"Readers": {
					Type: "ImplicitMeta",
					Rule: "ANY Readers",
				},
				"Writers": {
					Type: "ImplicitMeta",
					Rule: "ANY Writers",
				},
				"Admins": {
					Type: "ImplicitMeta",
					Rule: "MAJORITY Admins",
				},
				"Endorsement": {
					Type: "ImplicitMeta",
					Rule: "MAJORITY Endorsement",
				},
				"LifecycleEndorsement": {
					Type: "ImplicitMeta",
					Rule: "MAJORITY Endorsement",
				},
			},
		},
		Capabilities: []string{"V2_0"},
		Policies: map[string]configtx.Policy{
			"Readers": {
				Type: "ImplicitMeta",
				Rule: "ANY Readers",
			},
			"Writers": {
				Type: "ImplicitMeta",
				Rule: "ANY Writers",
			},
			"Admins": {
				Type: "ImplicitMeta",
				Rule: "MAJORITY Admins",
			},
		},
	}

	genesisBlock, err := configtx.NewApplicationChannelGenesisBlock(channelConfig, channel)
	Expect(err).NotTo(HaveOccurred())

	return genesisBlock
}
