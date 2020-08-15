/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package raft

import (
	"crypto"
	"crypto/x509"
	"encoding/pem"
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
	"github.com/hyperledger/fabric/integration/channelparticipation"
	conftx "github.com/hyperledger/fabric/integration/configtx"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	"github.com/hyperledger/fabric/integration/ordererclient"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/ginkgomon"
)

var _ = Describe("ChannelParticipation", func() {
	var (
		testDir          string
		client           *docker.Client
		network          *nwo.Network
		ordererProcesses []ifrit.Process
		ordererRunners   []*ginkgomon.Runner
	)

	BeforeEach(func() {
		var err error
		testDir, err = ioutil.TempDir("", "channel-participation")
		Expect(err).NotTo(HaveOccurred())

		client, err = docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())

		ordererProcesses = []ifrit.Process{}
		ordererRunners = []*ginkgomon.Runner{}
	})

	AfterEach(func() {
		for _, ordererProcess := range ordererProcesses {
			ordererProcess.Signal(syscall.SIGTERM)
			Eventually(ordererProcess.Wait(), network.EventuallyTimeout).Should(Receive())
		}
		if network != nil {
			network.Cleanup()
		}
		os.RemoveAll(testDir)
	})

	Describe("three node etcdraft network without a system channel", func() {
		startOrderer := func(o *nwo.Orderer) {
			ordererRunner := network.OrdererRunner(o)
			ordererProcess := ifrit.Invoke(ordererRunner)
			Eventually(ordererProcess.Ready(), network.EventuallyTimeout).Should(BeClosed())
			Eventually(ordererRunner.Err(), network.EventuallyTimeout).Should(gbytes.Say("Registrar initializing without a system channel, number of application channels: 0"))
			ordererProcesses = append(ordererProcesses, ordererProcess)
			ordererRunners = append(ordererRunners, ordererRunner)
		}

		BeforeEach(func() {
			network = nwo.New(nwo.MultiNodeEtcdRaft(), testDir, client, StartPort(), components)
			network.Consensus.ChannelParticipationEnabled = true
			network.Consensus.BootstrapMethod = "none"
			network.GenerateConfigTree()
			network.Bootstrap()
		})

		It("starts an orderer but rejects channel creation requests via the legacy channel creation", func() {
			orderer1 := network.Orderer("orderer1")
			startOrderer(orderer1)

			channelparticipation.List(network, orderer1, nil)

			By("attempting to create a channel without a system channel defined")
			sess, err := network.PeerAdminSession(network.Peer("Org1", "peer0"), commands.ChannelCreate{
				ChannelID:   "testchannel",
				Orderer:     network.OrdererAddress(orderer1, nwo.ListenPort),
				File:        network.CreateChannelTxPath("testchannel"),
				OutputBlock: "/dev/null",
				ClientAuth:  network.ClientAuthRequired,
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(1))
			Eventually(sess.Err, network.EventuallyTimeout).Should(gbytes.Say("channel creation request not allowed because the orderer system channel is not defined"))
		})

		It("joins application channels from genesis block and removes a channel using the channel participation API", func() {
			orderer1 := network.Orderer("orderer1")
			orderer2 := network.Orderer("orderer2")
			orderer3 := network.Orderer("orderer3")
			orderers := []*nwo.Orderer{orderer1, orderer2, orderer3}
			members := []*nwo.Orderer{orderer1, orderer2}
			peer := network.Peer("Org1", "peer0")

			By("starting all three orderers")
			for _, o := range orderers {
				startOrderer(o)
				channelparticipation.List(network, o, nil)
			}

			genesisBlock := applicationChannelGenesisBlock(network, members, peer, "participation-trophy")
			expectedChannelInfoPT := channelparticipation.ChannelInfo{
				Name:            "participation-trophy",
				URL:             "/participation/v1/channels/participation-trophy",
				Status:          "active",
				ClusterRelation: "member",
				Height:          1,
			}

			for _, o := range members {
				By("joining " + o.Name + " to channel as a member")
				channelparticipation.Join(network, o, "participation-trophy", genesisBlock, expectedChannelInfoPT)
				channelInfo := channelparticipation.ListOne(network, o, "participation-trophy")
				Expect(channelInfo).To(Equal(expectedChannelInfoPT))
			}

			By("waiting for the leader to be ready")
			memberRunners := []*ginkgomon.Runner{ordererRunners[0], ordererRunners[1]}
			leader := findLeader(memberRunners)

			By("ensuring the channel is usable by submitting a transaction to each member")
			env := CreateBroadcastEnvelope(network, peer, "participation-trophy", []byte("hello"))
			for _, o := range members {
				By("submitting transaction to " + o.Name)
				expectedChannelInfoPT.Height++
				resp, err := ordererclient.Broadcast(network, o, env)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.Status).To(Equal(common.Status_SUCCESS))
				expectedBlockNumPerChannel := map[string]int{"participation-trophy": int(expectedChannelInfoPT.Height - 1)}
				assertBlockReception(expectedBlockNumPerChannel, members, peer, network)

				By("checking the channel height")
				channelInfo := channelparticipation.ListOne(network, o, "participation-trophy")
				Expect(channelInfo).To(Equal(expectedChannelInfoPT))
			}

			By("joining orderer3 to the channel as a follower")
			// make sure we can join using a config block from one of the other orderers
			configBlockPT := nwo.GetConfigBlock(network, peer, orderer2, "participation-trophy")
			expectedChannelInfoPTFollower := channelparticipation.ChannelInfo{
				Name:            "participation-trophy",
				URL:             "/participation/v1/channels/participation-trophy",
				Status:          "onboarding",
				ClusterRelation: "follower",
				Height:          0,
			}
			channelparticipation.Join(network, orderer3, "participation-trophy", configBlockPT, expectedChannelInfoPTFollower)

			By("ensuring orderer3 completes onboarding successfully")
			expectedChannelInfoPTFollower.Status = "active"
			expectedChannelInfoPTFollower.Height = expectedChannelInfoPT.Height
			Eventually(func() channelparticipation.ChannelInfo {
				return channelparticipation.ListOne(network, orderer3, "participation-trophy")
			}, network.EventuallyTimeout).Should(Equal(expectedChannelInfoPTFollower))

			By("adding orderer3 to the consenters set")
			channelConfig := nwo.GetConfig(network, peer, orderer1, "participation-trophy")
			c := configtx.New(channelConfig)
			err := c.Orderer().AddConsenter(consenterChannelConfig(network, orderer3))
			Expect(err).NotTo(HaveOccurred())
			computeSignSubmitConfigUpdate(network, orderer1, peer, c, "participation-trophy")

			By("ensuring orderer3 transitions from follower to member")
			// config update above added a block
			expectedChannelInfoPT.Height++
			Eventually(func() channelparticipation.ChannelInfo {
				return channelparticipation.ListOne(network, orderer3, "participation-trophy")
			}, network.EventuallyTimeout).Should(Equal(expectedChannelInfoPT))

			// make sure orderer3 finds the leader
			newLeader := findLeader([]*ginkgomon.Runner{ordererRunners[2]})
			Expect(newLeader).To(Equal(leader))

			By("submitting transaction to orderer3 to ensure it is active")
			env = CreateBroadcastEnvelope(network, peer, "participation-trophy", []byte("hello-again"))
			expectedChannelInfoPT.Height++
			resp, err := ordererclient.Broadcast(network, orderer3, env)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Status).To(Equal(common.Status_SUCCESS))
			expectedBlockNumPerChannel := map[string]int{"participation-trophy": int(expectedChannelInfoPT.Height - 1)}
			assertBlockReception(expectedBlockNumPerChannel, orderers, peer, network)

			By("joining orderer1 to another channel as a member")
			genesisBlockAPT := applicationChannelGenesisBlock(network, []*nwo.Orderer{orderer1}, peer, "another-participation-trophy")
			expectedChannelInfoAPT := channelparticipation.ChannelInfo{
				Name:            "another-participation-trophy",
				URL:             "/participation/v1/channels/another-participation-trophy",
				Status:          "active",
				ClusterRelation: "member",
				Height:          1,
			}
			channelparticipation.Join(network, orderer1, "another-participation-trophy", genesisBlockAPT, expectedChannelInfoAPT)
			channelInfo := channelparticipation.ListOne(network, orderer1, "another-participation-trophy")
			Expect(channelInfo).To(Equal(expectedChannelInfoAPT))

			By("listing all channels for orderer1")
			channelparticipation.List(network, orderer1, []string{"participation-trophy", "another-participation-trophy"})

			By("removing orderer1 from the consenter set")
			channelConfig = nwo.GetConfig(network, peer, orderer2, "participation-trophy")
			c = configtx.New(channelConfig)
			err = c.Orderer().RemoveConsenter(consenterChannelConfig(network, orderer1))
			Expect(err).NotTo(HaveOccurred())
			computeSignSubmitConfigUpdate(network, orderer2, peer, c, "participation-trophy")

			// remove orderer1 from the orderer runners
			ordererRunners = ordererRunners[1:]
			if leader == 1 {
				By("waiting for the new leader to be ready")
				newLeader := findLeader(ordererRunners)
				Expect(newLeader).NotTo(Equal(leader))
			}

			By("removing orderer1 from a channel")
			channelparticipation.Remove(network, orderer1, "participation-trophy")

			By("listing all channels for orderer1")
			channelparticipation.List(network, orderer1, []string{"another-participation-trophy"})

			By("ensuring the channel is still usable by submitting a transaction to each remaining consenter for the channel")
			env = CreateBroadcastEnvelope(network, peer, "participation-trophy", []byte("hello"))
			orderers = []*nwo.Orderer{orderer2, orderer3}
			for _, o := range orderers {
				By("submitting transaction to " + o.Name)
				expectedChannelInfoPT.Height++
				resp, err := ordererclient.Broadcast(network, o, env)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.Status).To(Equal(common.Status_SUCCESS))
				expectedBlockNumPerChannel := map[string]int{"participation-trophy": int(expectedChannelInfoPT.Height - 1)}
				assertBlockReception(expectedBlockNumPerChannel, orderers, peer, network)

				By("checking the channel height")
				channelInfo = channelparticipation.ListOne(network, o, "participation-trophy")
				Expect(channelInfo).To(Equal(expectedChannelInfoPT))
			}
		})
	})
})

func applicationChannelGenesisBlock(n *nwo.Network, orderers []*nwo.Orderer, p *nwo.Peer, channel string) *common.Block {
	ordererOrgs, consenters := ordererOrganizationsAndConsenters(n, orderers)
	peerOrgs := peerOrganizations(n, p)

	channelConfig := configtx.Channel{
		Orderer: configtx.Orderer{
			OrdererType:   "etcdraft",
			Organizations: ordererOrgs,
			EtcdRaft: orderer.EtcdRaft{
				Consenters: consenters,
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
			Organizations: peerOrgs,
			Capabilities:  []string{"V2_0"},
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

// parseCertificate loads the PEM-encoded x509 certificate at the specified
// path.
func parseCertificate(path string) *x509.Certificate {
	certBytes, err := ioutil.ReadFile(path)
	Expect(err).NotTo(HaveOccurred())
	pemBlock, _ := pem.Decode(certBytes)
	cert, err := x509.ParseCertificate(pemBlock.Bytes)
	Expect(err).NotTo(HaveOccurred())
	return cert
}

// parsePrivateKey loads the PEM-encoded private key at the specified path.
func parsePrivateKey(path string) crypto.PrivateKey {
	pkBytes, err := ioutil.ReadFile(path)
	Expect(err).NotTo(HaveOccurred())
	pemBlock, _ := pem.Decode(pkBytes)
	privateKey, err := x509.ParsePKCS8PrivateKey(pemBlock.Bytes)
	Expect(err).NotTo(HaveOccurred())
	return privateKey
}

func ordererOrganizationsAndConsenters(n *nwo.Network, orderers []*nwo.Orderer) ([]configtx.Organization, []orderer.Consenter) {
	ordererOrgsMap := map[string]*configtx.Organization{}
	consenters := make([]orderer.Consenter, len(orderers))

	for i, o := range orderers {
		rootCert := parseCertificate(n.OrdererCACert(o))
		adminCert := parseCertificate(n.OrdererUserCert(o, "Admin"))
		tlsRootCert := parseCertificate(filepath.Join(n.OrdererLocalTLSDir(o), "ca.crt"))

		orgConfig, ok := ordererOrgsMap[o.Organization]
		if !ok {
			orgConfig := configtxOrganization(n.Organization(o.Organization), rootCert, adminCert, tlsRootCert)
			orgConfig.OrdererEndpoints = []string{
				n.OrdererAddress(o, nwo.ListenPort),
			}
			ordererOrgsMap[o.Organization] = orgConfig
		} else {
			orgConfig.OrdererEndpoints = append(orgConfig.OrdererEndpoints, n.OrdererAddress(o, nwo.ListenPort))
			orgConfig.MSP.RootCerts = append(orgConfig.MSP.RootCerts, rootCert)
			orgConfig.MSP.Admins = append(orgConfig.MSP.Admins, adminCert)
			orgConfig.MSP.TLSRootCerts = append(orgConfig.MSP.TLSRootCerts, tlsRootCert)
		}

		consenters[i] = consenterChannelConfig(n, o)
	}

	ordererOrgs := []configtx.Organization{}
	for _, o := range ordererOrgsMap {
		ordererOrgs = append(ordererOrgs, *o)
	}

	return ordererOrgs, consenters
}

func peerOrganizations(n *nwo.Network, p *nwo.Peer) []configtx.Organization {
	rootCert := parseCertificate(n.PeerCACert(p))
	adminCert := parseCertificate(n.PeerUserCert(p, "Admin"))
	tlsRootCert := parseCertificate(filepath.Join(n.PeerLocalTLSDir(p), "ca.crt"))

	peerOrg := configtxOrganization(n.Organization(p.Organization), rootCert, adminCert, tlsRootCert)

	return []configtx.Organization{*peerOrg}
}

func configtxOrganization(org *nwo.Organization, rootCert, adminCert, tlsRootCert *x509.Certificate) *configtx.Organization {
	orgConfig := &configtx.Organization{
		Name: org.Name,
		Policies: map[string]configtx.Policy{
			"Readers": {
				Type: "Signature",
				Rule: fmt.Sprintf("OR('%s.member')", org.MSPID),
			},
			"Writers": {
				Type: "Signature",
				Rule: fmt.Sprintf("OR('%s.member')", org.MSPID),
			},
			"Admins": {
				Type: "Signature",
				Rule: fmt.Sprintf("OR('%s.member')", org.MSPID),
			},
		},
		MSP: configtx.MSP{
			Name:         org.MSPID,
			RootCerts:    []*x509.Certificate{rootCert},
			Admins:       []*x509.Certificate{adminCert},
			TLSRootCerts: []*x509.Certificate{tlsRootCert},
		},
	}

	return orgConfig
}

func computeSignSubmitConfigUpdate(n *nwo.Network, o *nwo.Orderer, p *nwo.Peer, c configtx.ConfigTx, channel string) {
	configUpdate, err := c.ComputeMarshaledUpdate(channel)
	Expect(err).NotTo(HaveOccurred())

	signingIdentity := configtx.SigningIdentity{
		Certificate: parseCertificate(n.OrdererUserCert(o, "Admin")),
		PrivateKey:  parsePrivateKey(n.OrdererUserKey(o, "Admin")),
		MSPID:       n.Organization(o.Organization).MSPID,
	}
	signature, err := signingIdentity.CreateConfigSignature(configUpdate)
	Expect(err).NotTo(HaveOccurred())

	configUpdateEnvelope, err := configtx.NewEnvelope(configUpdate, signature)
	Expect(err).NotTo(HaveOccurred())
	err = signingIdentity.SignEnvelope(configUpdateEnvelope)
	Expect(err).NotTo(HaveOccurred())

	currentBlockNumber := nwo.CurrentConfigBlockNumber(n, p, o, channel)

	resp, err := ordererclient.Broadcast(n, o, configUpdateEnvelope)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.Status).To(Equal(common.Status_SUCCESS))

	ccb := func() uint64 { return nwo.CurrentConfigBlockNumber(n, p, o, channel) }
	Eventually(ccb, n.EventuallyTimeout).Should(BeNumerically(">", currentBlockNumber))
}

func consenterChannelConfig(n *nwo.Network, o *nwo.Orderer) orderer.Consenter {
	host, port := conftx.OrdererClusterHostPort(n, o)
	tlsCert := parseCertificate(filepath.Join(n.OrdererLocalTLSDir(o), "server.crt"))
	return orderer.Consenter{
		Address: orderer.EtcdAddress{
			Host: host,
			Port: port,
		},
		ClientTLSCert: tlsCert,
		ServerTLSCert: tlsCert,
	}
}
