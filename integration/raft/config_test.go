/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package raft

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/msp"
	protosorderer "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric-protos-go/orderer/etcdraft"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/integration/channelparticipation"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	"github.com/hyperledger/fabric/integration/ordererclient"
	"github.com/hyperledger/fabric/protoutil"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
	ginkgomon "github.com/tedsuo/ifrit/ginkgomon_v2"
)

var _ = Describe("EndToEnd reconfiguration and onboarding", func() {
	var (
		testDir string
		client  *docker.Client
		network *nwo.Network
		peer    *nwo.Peer

		networkProcess   ifrit.Process
		ordererProcesses []ifrit.Process
		ordererRunners   []*ginkgomon.Runner
	)

	BeforeEach(func() {
		ordererRunners = nil
		ordererProcesses = nil

		var err error
		testDir, err = ioutil.TempDir("", "e2e-etcdraft_reconfig")
		Expect(err).NotTo(HaveOccurred())

		client, err = docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if networkProcess != nil {
			networkProcess.Signal(syscall.SIGTERM)
			Eventually(networkProcess.Wait(), network.EventuallyTimeout).Should(Receive())
		}
		if network != nil {
			network.Cleanup()
		}
		for _, ordererInstance := range ordererProcesses {
			ordererInstance.Signal(syscall.SIGTERM)
			Eventually(ordererInstance.Wait(), network.EventuallyTimeout).Should(Receive())
		}
		os.RemoveAll(testDir)
	})

	Describe("three node etcdraft network with 2 orgs", func() {
		BeforeEach(func() {
			network = nwo.New(nwo.MultiNodeEtcdRaftNoSysChan(), testDir, client, StartPort(), components)
			network.GenerateConfigTree()
			network.Bootstrap()

			for _, o := range network.Orderers {
				runner, process := network.StartOrderer(o.Name)
				ordererRunners = append(ordererRunners, runner)
				ordererProcesses = append(ordererProcesses, process)
			}

			networkRunner := network.PeerGroupRunner()
			networkProcess = ifrit.Invoke(networkRunner)
			Eventually(networkProcess.Ready(), network.EventuallyTimeout).Should(BeClosed())
		})

		// This tests:
		//
		// 1. channel creation with raft orderer,
		// 2. all the nodes on three-node raft cluster are in sync wrt blocks,
		// 3. raft orderer processes type A config updates and delivers the
		//    config blocks to the peers.
		It("executes an etcdraft network with 2 orgs and three orderer nodes", func() {
			orderer1 := network.Orderer("orderer1")
			orderer2 := network.Orderer("orderer2")
			orderer3 := network.Orderer("orderer3")
			peer := network.Peer("Org1", "peer0")
			blockFile1 := filepath.Join(testDir, "newest_orderer1_block.pb")
			blockFile2 := filepath.Join(testDir, "newest_orderer2_block.pb")
			blockFile3 := filepath.Join(testDir, "newest_orderer3_block.pb")

			fetchLatestBlock := func(targetOrderer *nwo.Orderer, blockFile string) {
				c := commands.ChannelFetch{
					ChannelID:  "testchannel",
					Block:      "newest",
					OutputFile: blockFile,
				}
				if targetOrderer != nil {
					c.Orderer = network.OrdererAddress(targetOrderer, nwo.ListenPort)
				}
				sess, err := network.PeerAdminSession(peer, c)
				Expect(err).NotTo(HaveOccurred())
				Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
			}

			By("Creating a new channel, joining all orderers")
			channelparticipation.JoinOrderersAppChannelCluster(network, "testchannel", network.Orderers...)
			FindLeader(ordererRunners)

			// the above can work even if the orderer nodes are not in the same Raft
			// cluster; we need to verify all the three orderer nodes are in sync wrt
			// blocks.
			By("Fetching the latest blocks from all the orderer nodes and testing them for equality")
			fetchLatestBlock(orderer1, blockFile1)
			fetchLatestBlock(orderer2, blockFile2)
			fetchLatestBlock(orderer3, blockFile3)
			b1 := nwo.UnmarshalBlockFromFile(blockFile1)
			b2 := nwo.UnmarshalBlockFromFile(blockFile2)
			b3 := nwo.UnmarshalBlockFromFile(blockFile3)
			Expect(protoutil.BlockHeaderBytes(b1.Header)).To(Equal(protoutil.BlockHeaderBytes(b2.Header)))
			Expect(protoutil.BlockHeaderBytes(b2.Header)).To(Equal(protoutil.BlockHeaderBytes(b3.Header)))
		})
	})

	Describe("Invalid Raft config metadata", func() {
		It("refuses to join orderer to channel", func() {
			By("Creating malformed genesis block")
			network = nwo.New(nwo.BasicEtcdRaftNoSysChan(), testDir, client, StartPort(), components)
			network.GenerateConfigTree()
			network.Bootstrap()

			// Change the genesis block
			genesisPath := network.OutputBlockPath("testchannel")
			genesisBlock := nwo.UnmarshalBlockFromFile(genesisPath)
			envelope, err := protoutil.GetEnvelopeFromBlock(genesisBlock.Data.Data[0])
			Expect(err).NotTo(HaveOccurred())
			payload, err := protoutil.UnmarshalPayload(envelope.Payload)
			Expect(err).NotTo(HaveOccurred())
			configEnv := &common.ConfigEnvelope{}
			err = proto.Unmarshal(payload.Data, configEnv)
			Expect(err).NotTo(HaveOccurred())
			genesisConfig := configEnv.GetConfig()

			consensusTypeConfigValue := genesisConfig.ChannelGroup.Groups["Orderer"].Values["ConsensusType"]
			consensusTypeValue := &protosorderer.ConsensusType{}
			Expect(proto.Unmarshal(consensusTypeConfigValue.Value, consensusTypeValue)).To(Succeed())

			metadata := &etcdraft.ConfigMetadata{}
			Expect(proto.Unmarshal(consensusTypeValue.Metadata, metadata)).To(Succeed())

			metadata.Options.HeartbeatTick = 10
			metadata.Options.ElectionTick = 10

			newMetadata, err := proto.Marshal(metadata)
			Expect(err).NotTo(HaveOccurred())
			consensusTypeValue.Metadata = newMetadata

			genesisConfig.ChannelGroup.Groups["Orderer"].Values["ConsensusType"] = &common.ConfigValue{
				ModPolicy: "Admins",
				Value:     protoutil.MarshalOrPanic(consensusTypeValue),
			}

			configEnv.Config = genesisConfig
			payload.Data, err = protoutil.Marshal(configEnv)
			Expect(err).NotTo(HaveOccurred())
			envelope.Payload, err = protoutil.Marshal(payload)
			Expect(err).NotTo(HaveOccurred())
			genesisBlock.Data.Data[0], err = protoutil.Marshal(envelope)
			Expect(err).NotTo(HaveOccurred())
			genesisBlockBytes, err := protoutil.Marshal(genesisBlock)
			Expect(err).NotTo(HaveOccurred())
			err = ioutil.WriteFile(network.OutputBlockPath("testchannel"), genesisBlockBytes, 0o644)
			Expect(err).NotTo(HaveOccurred())

			By("Starting orderer")
			_, ordererProcess, peerProcess := network.StartSingleOrdererNetwork("orderer")
			ordererProcesses = append(ordererProcesses, ordererProcess)
			networkProcess = peerProcess

			By("Joining the orderer to a channel with malformed genesis block")
			orderer := network.Orderer("orderer")
			channelparticipationJoinFailure(network, orderer, "testchannel", genesisBlock, http.StatusBadRequest,
				"cannot join: failed to determine cluster membership from join-block: failed to validate config metadata of ordering config: ElectionTick (10) must be greater than HeartbeatTick (10)")
		})

		It("rejects config update", func() {
			network = nwo.New(nwo.BasicEtcdRaftNoSysChan(), testDir, client, StartPort(), components)
			network.GenerateConfigTree()
			network.Bootstrap()

			By("Starting orderer")
			odererRunner, ordererProcess, peerProcess := network.StartSingleOrdererNetwork("orderer")
			ordererProcesses = append(ordererProcesses, ordererProcess)
			networkProcess = peerProcess

			By("Joining the orderer to a channel")
			orderer := network.Orderer("orderer")
			channelparticipation.JoinOrdererJoinPeersAppChannel(network, "testchannel", orderer, odererRunner)

			By("Submitting channel config update with illegal value")
			org1Peer0 := network.Peer("Org1", "peer0")
			config := nwo.GetConfig(network, org1Peer0, orderer, "testchannel")
			updatedConfig := proto.Clone(config).(*common.Config)

			consensusTypeConfigValue := updatedConfig.ChannelGroup.Groups["Orderer"].Values["ConsensusType"]
			consensusTypeValue := &protosorderer.ConsensusType{}
			Expect(proto.Unmarshal(consensusTypeConfigValue.Value, consensusTypeValue)).To(Succeed())

			metadata := &etcdraft.ConfigMetadata{}
			Expect(proto.Unmarshal(consensusTypeValue.Metadata, metadata)).To(Succeed())

			metadata.Options.HeartbeatTick = 10
			metadata.Options.ElectionTick = 10

			newMetadata, err := proto.Marshal(metadata)
			Expect(err).NotTo(HaveOccurred())
			consensusTypeValue.Metadata = newMetadata

			updatedConfig.ChannelGroup.Groups["Orderer"].Values["ConsensusType"] = &common.ConfigValue{
				ModPolicy: "Admins",
				Value:     protoutil.MarshalOrPanic(consensusTypeValue),
			}

			sess := nwo.UpdateOrdererConfigSession(network, orderer, "testchannel", config, updatedConfig, org1Peer0, orderer)
			Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(1))
			Expect(sess.Err).To(gbytes.Say(`invalid new config metadata: ElectionTick \(10\) must be greater than HeartbeatTick \(10\)`))
		})
	})

	When("a single node cluster is expanded", func() {
		It("is still possible to onboard the new cluster member and then another one with a different TLS root CA", func() {
			launch := func(o *nwo.Orderer) {
				runner := network.OrdererRunner(o)
				ordererRunners = append(ordererRunners, runner)

				process := ifrit.Invoke(runner)
				Eventually(process.Ready(), network.EventuallyTimeout).Should(BeClosed())
				ordererProcesses = append(ordererProcesses, process)
			}

			network = nwo.New(nwo.BasicEtcdRaftNoSysChan(), testDir, client, StartPort(), components)
			network.GenerateConfigTree()
			network.Bootstrap()

			orderer := network.Orderer("orderer")
			peer = network.Peer("Org1", "peer0")

			By("Launching the orderer")
			launch(orderer)

			By("Joining the orderer to a channel")
			channelparticipation.JoinOrderersAppChannelCluster(network, "testchannel", orderer)

			By("Checking that it elected itself as a leader")
			FindLeader(ordererRunners)

			By("Extending the network configuration to add a new orderer")
			orderer2 := &nwo.Orderer{
				Name:         "orderer2",
				Organization: "OrdererOrg",
			}
			ports := nwo.Ports{}
			for _, portName := range nwo.OrdererPortNames() {
				ports[portName] = network.ReservePort()
			}
			network.PortsByOrdererID[orderer2.ID()] = ports
			network.Orderers = append(network.Orderers, orderer2)
			network.GenerateOrdererConfig(orderer2)
			extendNetwork(network)

			secondOrdererCertificatePath := filepath.Join(network.OrdererLocalTLSDir(orderer2), "server.crt")
			secondOrdererCertificate, err := ioutil.ReadFile(secondOrdererCertificatePath)
			Expect(err).NotTo(HaveOccurred())

			By("Adding the second orderer")
			addConsenter(network, peer, orderer, "testchannel", etcdraft.Consenter{
				ServerTlsCert: secondOrdererCertificate,
				ClientTlsCert: secondOrdererCertificate,
				Host:          "127.0.0.1",
				Port:          uint32(network.OrdererPort(orderer2, nwo.ClusterPort)),
			})

			By("Obtaining the last config block from the orderer")
			configBlock := nwo.GetConfigBlock(network, peer, orderer, "testchannel")

			By("Waiting for the existing orderer to relinquish its leadership")
			Eventually(ordererRunners[0].Err(), network.EventuallyTimeout).Should(gbytes.Say("1 stepped down to follower since quorum is not active"))
			Eventually(ordererRunners[0].Err(), network.EventuallyTimeout).Should(gbytes.Say("No leader is present, cluster size is 2"))

			By("Launching the second orderer")
			launch(orderer2)

			By("Joining the second orderer to the channel, as consenter")
			expectedChannelInfo := channelparticipation.ChannelInfo{
				Name:              "testchannel",
				URL:               "/participation/v1/channels/testchannel",
				Status:            "onboarding",
				ConsensusRelation: "consenter",
				Height:            0,
			}
			channelparticipation.Join(network, orderer2, "testchannel", configBlock, expectedChannelInfo)

			By("Waiting for a leader to be re-elected")
			FindLeader(ordererRunners)

			// In the next part of the test we're going to bring up a third node
			// with a different TLS root CA. We're then going to remove the TLS
			// root CA and restart the orderer, to ensure that we can dynamically
			// update TLS root CAs in Raft while membership stays the same.

			By("Creating configuration for a third orderer with a different TLS root CA")
			orderer3 := &nwo.Orderer{
				Name:         "orderer3",
				Organization: "OrdererOrg",
			}
			ports = nwo.Ports{}
			for _, portName := range nwo.OrdererPortNames() {
				ports[portName] = network.ReservePort()
			}
			network.PortsByOrdererID[orderer3.ID()] = ports
			network.Orderers = append(network.Orderers, orderer3)
			network.GenerateOrdererConfig(orderer3)

			tmpDir, err := ioutil.TempDir("", "e2e-etcfraft_reconfig")
			Expect(err).NotTo(HaveOccurred())
			defer os.RemoveAll(tmpDir)

			sess, err := network.Cryptogen(commands.Generate{
				Config: network.CryptoConfigPath(),
				Output: tmpDir,
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))

			name := network.Orderers[0].Name
			domain := network.Organization(network.Orderers[0].Organization).Domain
			nameDomain := fmt.Sprintf("%s.%s", name, domain)
			ordererTLSPath := filepath.Join(tmpDir, "ordererOrganizations", domain, "orderers", nameDomain, "tls")

			caCertPath := filepath.Join(tmpDir, "ordererOrganizations", domain, "tlsca", fmt.Sprintf("tlsca.%s-cert.pem", domain))

			caCert, err := ioutil.ReadFile(caCertPath)
			Expect(err).NotTo(HaveOccurred())

			thirdOrdererCertificatePath := filepath.Join(ordererTLSPath, "server.crt")
			thirdOrdererCertificate, err := ioutil.ReadFile(thirdOrdererCertificatePath)
			Expect(err).NotTo(HaveOccurred())

			By("Updating it on the file system")
			err = ioutil.WriteFile(caCertPath, caCert, 0o644)
			Expect(err).NotTo(HaveOccurred())
			err = ioutil.WriteFile(thirdOrdererCertificatePath, thirdOrdererCertificate, 0o644)
			Expect(err).NotTo(HaveOccurred())

			By("Overwriting the TLS directory of the new orderer")
			for _, fileName := range []string{"server.crt", "server.key", "ca.crt"} {
				dst := filepath.Join(network.OrdererLocalTLSDir(orderer3), fileName)

				data, err := ioutil.ReadFile(filepath.Join(ordererTLSPath, fileName))
				Expect(err).NotTo(HaveOccurred())

				err = ioutil.WriteFile(dst, data, 0o644)
				Expect(err).NotTo(HaveOccurred())
			}

			By("Launching orderer3")
			launch(orderer3)

			By("Obtaining the last config block from the orderer")
			configBlock = nwo.GetConfigBlock(network, peer, orderer, "testchannel")

			By("Joining the third orderer to the channel, as follower")
			expectedChannelInfo = channelparticipation.ChannelInfo{
				Name:              "testchannel",
				URL:               "/participation/v1/channels/testchannel",
				Status:            "onboarding",
				ConsensusRelation: "follower",
				Height:            0,
			}
			channelparticipation.Join(network, orderer3, "testchannel", configBlock, expectedChannelInfo)

			By("Expanding the TLS root CA certificates and adding orderer3 to the channel")
			updateOrdererMSPAndConsensusMetadata(network, peer, orderer, "testchannel", "OrdererOrg",
				func(config msp.FabricMSPConfig) msp.FabricMSPConfig { // MSP mutator
					config.TlsRootCerts = append(config.TlsRootCerts, caCert)
					return config
				},
				func(metadata *etcdraft.ConfigMetadata) { // etcdraft mutator
					metadata.Consenters = append(metadata.Consenters, &etcdraft.Consenter{
						ServerTlsCert: thirdOrdererCertificate,
						ClientTlsCert: thirdOrdererCertificate,
						Host:          "127.0.0.1",
						Port:          uint32(network.OrdererPort(orderer3, nwo.ClusterPort)),
					})
				},
			)

			By("Waiting for orderer3 to see the leader")
			FindLeader([]*ginkgomon.Runner{ordererRunners[2]})

			Expect(channelparticipation.ListOne(network, orderer3, "testchannel")).To(Equal(
				channelparticipation.ChannelInfo{
					Name:              "testchannel",
					URL:               "/participation/v1/channels/testchannel",
					Status:            "active",
					ConsensusRelation: "consenter",
					Height:            3,
				}))

			By("Attemping to add a consenter with invalid certs")
			// create new certs that are not in the channel config
			ca, err := tlsgen.NewCA()
			Expect(err).NotTo(HaveOccurred())
			client, err := ca.NewClientCertKeyPair()
			Expect(err).NotTo(HaveOccurred())

			newConsenterCertPem, _ := pem.Decode(client.Cert)
			newConsenterCert, err := x509.ParseCertificate(newConsenterCertPem.Bytes)
			Expect(err).NotTo(HaveOccurred())

			newConsenterHost := "127.0.0.1"
			newConsenterPort := uint32(network.OrdererPort(orderer3, nwo.ListenPort))

			current, updated := consenterAdder(
				network,
				peer,
				orderer,
				"testchannel",
				etcdraft.Consenter{
					ServerTlsCert: client.Cert,
					ClientTlsCert: client.Cert,
					Host:          newConsenterHost,
					Port:          newConsenterPort,
				},
			)
			sess = nwo.UpdateOrdererConfigSession(network, orderer, "testchannel", current, updated, peer, orderer)
			Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(1))
			Expect(sess.Err).To(gbytes.Say(fmt.Sprintf("BAD_REQUEST -- error applying config update to existing channel 'testchannel': consensus metadata update for channel config update is invalid: invalid new config metadata: consenter %s:%d has invalid certificate: verifying tls client cert with serial number %d: x509: certificate signed by unknown authority", newConsenterHost, newConsenterPort, newConsenterCert.SerialNumber)))
		})
	})

	When("a single node cluster has the tick interval overridden", func() {
		It("reflects this in its startup logs", func() {
			network = nwo.New(nwo.BasicEtcdRaftNoSysChan(), testDir, client, StartPort(), components)
			network.GenerateConfigTree()
			network.Bootstrap()

			orderer := network.Orderer("orderer")
			ordererConfig := network.ReadOrdererConfig(orderer)
			ordererConfig.Consensus["TickIntervalOverride"] = "642ms"
			network.WriteOrdererConfig(orderer, ordererConfig)

			By("Launching the orderer")
			runner := network.OrdererRunner(orderer)
			ordererRunners = append(ordererRunners, runner)

			process := ifrit.Invoke(runner)
			Eventually(process.Ready(), network.EventuallyTimeout).Should(BeClosed())
			ordererProcesses = append(ordererProcesses, process)

			By("Joining the orderer to a channel")
			channelparticipation.JoinOrderersAppChannelCluster(network, "testchannel", orderer)

			Eventually(runner.Err()).Should(gbytes.Say("TickIntervalOverride is set, overriding channel configuration tick interval to 642ms"))
		})
	})

	When("the orderer certificates are all rotated", func() {
		It("is possible to rotate certificate by adding & removing cert in single config", func() {
			layout := nwo.MultiNodeEtcdRaftNoSysChan()
			network = nwo.New(layout, testDir, client, StartPort(), components)
			network.GenerateConfigTree()
			network.Bootstrap()

			o1, o2, o3 := network.Orderer("orderer1"), network.Orderer("orderer2"), network.Orderer("orderer3")
			orderers := []*nwo.Orderer{o1, o2, o3}
			peer = network.Peer("Org1", "peer0")

			By("Launching the orderers")
			for _, o := range orderers {
				runner := network.OrdererRunner(o)
				ordererRunners = append(ordererRunners, runner)
				process := ifrit.Invoke(runner)
				ordererProcesses = append(ordererProcesses, process)
			}

			for _, ordererProc := range ordererProcesses {
				Eventually(ordererProc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			}

			By("Creating a new channel, joining all orderers")
			channelparticipation.JoinOrderersAppChannelCluster(network, "testchannel", network.Orderers...)

			By("Finding leader")
			leader := FindLeader(ordererRunners)
			leaderIndex := leader - 1
			blockSeq := 0

			By("Checking that all orderers are online")
			assertBlockReception(map[string]int{
				"testchannel": blockSeq,
			}, orderers, peer, network)

			By("Preparing new certificates for the orderer nodes")
			extendNetwork(network)
			certificateRotations := refreshOrdererPEMs(network)

			swap := func(o *nwo.Orderer, certificate []byte, c etcdraft.Consenter) {
				updateEtcdRaftMetadata(network, peer, o, "testchannel", func(metadata *etcdraft.ConfigMetadata) {
					var newConsenters []*etcdraft.Consenter
					for _, consenter := range metadata.Consenters {
						if bytes.Equal(consenter.ClientTlsCert, certificate) || bytes.Equal(consenter.ServerTlsCert, certificate) {
							continue
						}
						newConsenters = append(newConsenters, consenter)
					}
					newConsenters = append(newConsenters, &c)

					metadata.Consenters = newConsenters
				})
				blockSeq++
			}

			rotate := func(target int) {
				// submit a config tx to rotate the cert of an orderer.
				// The orderer being rotated is going to be unavailable
				// eventually, therefore submitter of tx is different
				// from the target, so the configuration can be reliably
				// checked.
				submitter := (target + 1) % 3
				rotation := certificateRotations[target]
				targetOrderer := network.Orderers[target]
				remainder := func() []*nwo.Orderer {
					var ret []*nwo.Orderer
					for i, o := range network.Orderers {
						if i == target {
							continue
						}
						ret = append(ret, o)
					}
					return ret
				}()
				submitterOrderer := network.Orderers[submitter]
				port := network.OrdererPort(targetOrderer, nwo.ClusterPort)

				fmt.Fprintf(GinkgoWriter, "Rotating certificate of orderer node %d\n", target+1)
				swap(submitterOrderer, rotation.oldCert, etcdraft.Consenter{
					ServerTlsCert: rotation.newCert,
					ClientTlsCert: rotation.newCert,
					Host:          "127.0.0.1",
					Port:          uint32(port),
				})

				By("Waiting for all orderers to sync")
				assertBlockReception(map[string]int{
					"testchannel": blockSeq,
				}, remainder, peer, network)

				By("Waiting for rotated node to be unavailable")
				c := commands.ChannelFetch{
					ChannelID:  "testchannel",
					Block:      "newest",
					OutputFile: "/dev/null",
					Orderer:    network.OrdererAddress(targetOrderer, nwo.ClusterPort),
				}
				Eventually(func() string {
					sess, err := network.OrdererAdminSession(targetOrderer, peer, c)
					Expect(err).NotTo(HaveOccurred())
					Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit())
					if sess.ExitCode() != 0 {
						return fmt.Sprintf("exit code is %d: %s", sess.ExitCode(), string(sess.Err.Contents()))
					}
					sessErr := string(sess.Err.Contents())
					expected := fmt.Sprintf("Received block: %d", blockSeq)
					if strings.Contains(sessErr, expected) {
						return ""
					}
					return sessErr
				}, network.EventuallyTimeout, time.Second).ShouldNot(BeEmpty())

				By("Killing the orderer")
				ordererProcesses[target].Signal(syscall.SIGTERM)
				Eventually(ordererProcesses[target].Wait(), network.EventuallyTimeout).Should(Receive())

				By("Starting the orderer again")
				ordererRunner := network.OrdererRunner(targetOrderer)
				ordererRunners = append(ordererRunners, ordererRunner)
				ordererProcesses[target] = ifrit.Invoke(ordererRunner)
				Eventually(ordererProcesses[target].Ready(), network.EventuallyTimeout).Should(BeClosed())

				By("And waiting for it to stabilize")
				assertBlockReception(map[string]int{
					"testchannel": blockSeq,
				}, orderers, peer, network)
			}

			By(fmt.Sprintf("Rotating cert on leader %d", leader))
			rotate(leaderIndex)

			By("Rotating certificates of other orderer nodes")
			for i := range certificateRotations {
				if i != leaderIndex {
					rotate(i)
				}
			}
		})

		It("is still possible to onboard new orderers", func() {
			// In this test, we have 3 OSNs and we rotate their TLS certificates one by one,
			// by adding the future certificate to the channel, killing the OSN to make it
			// grab the new certificate, and then removing the old certificate from the channel.

			// After we completely rotate all the certificates, we put the last config block
			// of the system channel into the file system of orderer4, and then launch it,
			// and ensure it onboards and pulls channels testchannel only, and not testchannel2
			// which it is not part of.

			// Consenter i after its certificate is rotated is denoted as consenter i'
			// The blocks of channels contain the following updates:
			//    |  testchannel  height  | update description
			// ------------------------------------------------------------------------
			// 0  |          1            | adding consenter 1'
			// 1  |          2            | removing consenter 1
			// 2  |          3            | adding consenter 2'
			// 3  |          4            | removing consenter 2
			// 4  |          5            | adding consenter 3'
			// 5  |          6            | removing consenter 3
			// 6  |          7            | adding consenter 4

			layout := nwo.MultiNodeEtcdRaftNoSysChan()
			layout.Channels = append(layout.Channels, &nwo.Channel{
				Name:    "testchannel2",
				Profile: "TwoOrgsAppChannelEtcdRaft",
			}, &nwo.Channel{
				Name:    "testchannel3",
				Profile: "TwoOrgsAppChannelEtcdRaft",
			})

			network = nwo.New(layout, testDir, client, StartPort(), components)
			network.GenerateConfigTree()
			network.Bootstrap()

			o1, o2, o3 := network.Orderer("orderer1"), network.Orderer("orderer2"), network.Orderer("orderer3")
			orderers := []*nwo.Orderer{o1, o2, o3}
			peer = network.Peer("Org1", "peer0")

			By("Launching the orderers")
			for _, o := range orderers {
				runner := network.OrdererRunner(o)
				ordererRunners = append(ordererRunners, runner)
				process := ifrit.Invoke(runner)
				ordererProcesses = append(ordererProcesses, process)
			}

			for _, ordererProc := range ordererProcesses {
				Eventually(ordererProc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			}

			By("Creating a channel, joining all orderers")
			channelparticipation.JoinOrderersAppChannelCluster(network, "testchannel", network.Orderers...)

			By("Finding leader")
			_ = FindLeader(ordererRunners)

			By("Checking that all orderers are online")
			assertBlockReception(map[string]int{
				"testchannel": 0,
			}, orderers, peer, network)

			By("Preparing new certificates for the orderer nodes")
			extendNetwork(network)
			certificateRotations := refreshOrdererPEMs(network)

			expectedBlockNumPerChannel := []map[string]int{
				{"testchannel": 1},
				{"testchannel": 2},
				{"testchannel": 3},
				{"testchannel": 4},
				{"testchannel": 5},
				{"testchannel": 6},
			}

			for i, rotation := range certificateRotations {
				o := network.Orderers[i]
				port := network.OrdererPort(o, nwo.ClusterPort)

				By(fmt.Sprintf("Adding the future certificate of orderer node %d", i))
				for _, channelName := range []string{"testchannel"} {
					addConsenter(network, peer, o, channelName, etcdraft.Consenter{
						ServerTlsCert: rotation.newCert,
						ClientTlsCert: rotation.newCert,
						Host:          "127.0.0.1",
						Port:          uint32(port),
					})
				}

				By("Waiting for all orderers to sync")
				assertBlockReception(expectedBlockNumPerChannel[i*2], orderers, peer, network)

				By("Killing the orderer")
				ordererProcesses[i].Signal(syscall.SIGTERM)
				Eventually(ordererProcesses[i].Wait(), network.EventuallyTimeout).Should(Receive())

				By("Starting the orderer again")
				ordererRunner := network.OrdererRunner(orderers[i])
				ordererRunners = append(ordererRunners, ordererRunner)
				ordererProcesses[i] = ifrit.Invoke(ordererRunner)
				Eventually(ordererProcesses[i].Ready(), network.EventuallyTimeout).Should(BeClosed())

				By("And waiting for it to stabilize")
				assertBlockReception(expectedBlockNumPerChannel[i*2], orderers, peer, network)

				By("Removing the previous certificate of the old orderer")
				for _, channelName := range []string{"testchannel"} {
					removeConsenter(network, peer, network.Orderers[(i+1)%len(network.Orderers)], channelName, rotation.oldCert)
				}

				By("Waiting for all orderers to sync")
				assertBlockReception(expectedBlockNumPerChannel[i*2+1], orderers, peer, network)
			}

			By("Getting the last config block from testchannel and using it as a template for testchannel2 and testchannel3 genesis block")
			blockFile := filepath.Join(testDir, "testchannel_last_config_block.pb")
			c := commands.ChannelFetch{
				ChannelID:  "testchannel",
				Block:      "newest",
				OutputFile: blockFile,
			}
			c.Orderer = network.OrdererAddress(o1, nwo.ListenPort)
			sess, err := network.PeerAdminSession(peer, c)
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
			lastConfigBlock := nwo.UnmarshalBlockFromFile(blockFile)

			genesisBlock2 := configToGenesisBlock(lastConfigBlock, "testchannel2")
			genesisBytes2, err := protoutil.Marshal(genesisBlock2)
			Expect(err).NotTo(HaveOccurred())
			path2 := network.OutputBlockPath("testchannel2")
			err = ioutil.WriteFile(path2, genesisBytes2, 0o644)
			Expect(err).NotTo(HaveOccurred())

			genesisBlock3 := configToGenesisBlock(lastConfigBlock, "testchannel3")
			genesisBytes3, err := protoutil.Marshal(genesisBlock3)
			Expect(err).NotTo(HaveOccurred())
			path3 := network.OutputBlockPath("testchannel3")
			err = ioutil.WriteFile(path3, genesisBytes3, 0o644)
			Expect(err).NotTo(HaveOccurred())

			By("Creating a testchannel2, joining all orderers")
			channelparticipation.JoinOrderersAppChannelCluster(network, "testchannel2", network.Orderers...)

			By("Creating a testchannel3, joining all orderers")
			channelparticipation.JoinOrderersAppChannelCluster(network, "testchannel3", network.Orderers...)

			assertBlockReception(map[string]int{
				"testchannel2": 0,
				"testchannel3": 0,
			}, orderers, peer, network)

			o4 := &nwo.Orderer{
				Name:         "orderer4",
				Organization: "OrdererOrg",
			}

			By("Configuring orderer4 in the network")
			ports := nwo.Ports{}
			for _, portName := range nwo.OrdererPortNames() {
				ports[portName] = network.ReservePort()
			}
			network.PortsByOrdererID[o4.ID()] = ports

			network.Orderers = append(network.Orderers, o4)
			network.GenerateOrdererConfig(network.Orderer("orderer4"))

			By("Adding orderer4 to the channel")
			orderer4CertificatePath := filepath.Join(network.OrdererLocalTLSDir(o4), "server.crt")
			orderer4Certificate, err := ioutil.ReadFile(orderer4CertificatePath)
			Expect(err).NotTo(HaveOccurred())

			addConsenter(network, peer, o1, "testchannel", etcdraft.Consenter{
				ServerTlsCert: orderer4Certificate,
				ClientTlsCert: orderer4Certificate,
				Host:          "127.0.0.1",
				Port:          uint32(network.OrdererPort(o4, nwo.ClusterPort)),
			})

			By("Ensuring all orderers know about orderer4's addition")
			assertBlockReception(map[string]int{
				"testchannel": 7,
			}, orderers, peer, network)

			By("Broadcasting envelope to testchannel")
			env := CreateBroadcastEnvelope(network, peer, "testchannel", []byte("hello"))
			resp, err := ordererclient.Broadcast(network, o1, env)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Status).To(Equal(common.Status_SUCCESS))

			assertBlockReception(map[string]int{
				"testchannel": 8,
			}, orderers, peer, network)

			By("Corrupting the readers policy of testchannel3")
			revokeReaderAccess(network, "testchannel3", o3, peer)

			By("Launching orderer4")
			orderers = append(orderers, o4)
			orderer4Runner := network.OrdererRunner(o4)
			ordererRunners = append(ordererRunners, orderer4Runner)
			// Spawn orderer4's process
			o4process := ifrit.Invoke(orderer4Runner)
			Eventually(o4process.Ready(), network.EventuallyTimeout).Should(BeClosed())
			ordererProcesses = append(ordererProcesses, o4process)
			// Get the last config block of the testchannel
			configBlock := nwo.GetConfigBlock(network, peer, o1, "testchannel")

			By("Joining the fourth orderer to the channel, as consenter")
			expectedChannelInfo := channelparticipation.ChannelInfo{
				Name:              "testchannel",
				URL:               "/participation/v1/channels/testchannel",
				Status:            "onboarding",
				ConsensusRelation: "consenter",
				Height:            0,
			}
			channelparticipation.Join(network, o4, "testchannel", configBlock, expectedChannelInfo)

			By("And waiting for it to sync with the rest of the orderers")
			assertBlockReception(map[string]int{
				"testchannel": 8,
			}, orderers, peer, network)

			By("Ensuring orderer4 doesn't serve testchannel2 and testchannel3")
			env = CreateBroadcastEnvelope(network, peer, "testchannel2", []byte("hello"))
			resp, err = ordererclient.Broadcast(network, o4, env)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Status).To(Equal(common.Status_BAD_REQUEST))
			Expect(orderer4Runner.Err()).To(gbytes.Say("channel does not exist"))

			env = CreateBroadcastEnvelope(network, peer, "testchannel3", []byte("hello"))
			resp, err = ordererclient.Broadcast(network, o4, env)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Status).To(Equal(common.Status_BAD_REQUEST))
			Expect(orderer4Runner.Err()).To(gbytes.Say("channel does not exist"))

			By("Adding orderer4 to testchannel2")
			addConsenter(network, peer, o1, "testchannel2", etcdraft.Consenter{
				ServerTlsCert: orderer4Certificate,
				ClientTlsCert: orderer4Certificate,
				Host:          "127.0.0.1",
				Port:          uint32(network.OrdererPort(o4, nwo.ClusterPort)),
			})

			// Get the last config block of the testchannel
			configBlock = nwo.GetConfigBlock(network, peer, o1, "testchannel2")
			expectedChannelInfo = channelparticipation.ChannelInfo{
				Name:              "testchannel2",
				URL:               "/participation/v1/channels/testchannel2",
				Status:            "onboarding",
				ConsensusRelation: "consenter",
				Height:            0,
			}
			channelparticipation.Join(network, o4, "testchannel2", configBlock, expectedChannelInfo)

			By("Waiting for orderer4 and to replicate testchannel2")
			assertBlockReception(map[string]int{
				"testchannel2": 1,
			}, []*nwo.Orderer{o4}, peer, network)

			By("Ensuring orderer4 doesn't have any errors in the logs")
			Consistently(orderer4Runner.Err()).ShouldNot(gbytes.Say("ERRO"))

			By("Submitting a transaction through orderer4")
			env = CreateBroadcastEnvelope(network, peer, "testchannel2", []byte("hello"))
			resp, err = ordererclient.Broadcast(network, o4, env)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Status).To(Equal(common.Status_SUCCESS))

			By("And ensuring it is propagated amongst all orderers")
			assertBlockReception(map[string]int{
				"testchannel2": 2,
			}, orderers, peer, network)

			By("Adding orderer4 to testchannel3")
			addConsenter(network, peer, o1, "testchannel3", etcdraft.Consenter{
				ServerTlsCert: orderer4Certificate,
				ClientTlsCert: orderer4Certificate,
				Host:          "127.0.0.1",
				Port:          uint32(network.OrdererPort(o4, nwo.ClusterPort)),
			})

			// Get the last config block of the testchannel3
			configBlock = nwo.GetConfigBlock(network, peer, o1, "testchannel3")
			expectedChannelInfo = channelparticipation.ChannelInfo{
				Name:              "testchannel3",
				URL:               "/participation/v1/channels/testchannel3",
				Status:            "onboarding",
				ConsensusRelation: "consenter",
				Height:            0,
			}
			channelparticipation.Join(network, o4, "testchannel3", configBlock, expectedChannelInfo)

			By("checking the logs that indicate that channel3 cannot be pulled using the delivery service, as the read policy is corrupt")
			Eventually(orderer4Runner.Err(), network.EventuallyTimeout).Should(gbytes.Say("Received status:FORBIDDEN  from 127\\.0\\.0\\.1:[0-9]+: forbidden pulling the channel channel=testchannel3"))
		})
	})

	When("an orderer channel is created with a subset of nodes", func() {
		It("is still possible to onboard a new orderer to the channel", func() {
			network = nwo.New(nwo.MultiNodeEtcdRaftNoSysChan(), testDir, client, StartPort(), components)
			network.Profiles = append(network.Profiles, &nwo.Profile{
				Name:          "myprofile",
				Consortium:    "MySampleConsortium",
				Orderers:      []string{"orderer1"},
				Organizations: []string{"Org1"},
			})
			network.Channels = append(network.Channels, &nwo.Channel{
				Name:    "mychannel",
				Profile: "myprofile",
			})

			network.GenerateConfigTree()
			network.Bootstrap()

			o1, o2, o3 := network.Orderer("orderer1"), network.Orderer("orderer2"), network.Orderer("orderer3")
			orderers := []*nwo.Orderer{o1, o2, o3}
			peer = network.Peer("Org1", "peer0")

			By("Launching the orderers")
			for _, o := range orderers {
				runner := network.OrdererRunner(o)
				ordererRunners = append(ordererRunners, runner)
				process := ifrit.Invoke(runner)
				ordererProcesses = append(ordererProcesses, process)
			}

			for _, ordererProc := range ordererProcesses {
				Eventually(ordererProc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			}

			By("Creating testchannel, joining all orderers")
			channelparticipation.JoinOrderersAppChannelCluster(network, "testchannel", network.Orderers...)

			By("Finding leader testchannel")
			_ = FindLeader(ordererRunners)

			By("Creating mychannel with a subset of orderers")
			channelparticipation.JoinOrderersAppChannelCluster(network, "mychannel", o1)

			By("Finding leader mychannel")
			_ = FindLeader(ordererRunners[:1])

			By("Waiting for the channel to be available")
			assertBlockReception(map[string]int{
				"mychannel": 0,
			}, []*nwo.Orderer{o1}, peer, network)

			By("Ensuring only orderer1 services the channel")
			env := CreateBroadcastEnvelope(network, peer, "mychannel", []byte("hello"))
			resp, err := ordererclient.Broadcast(network, o2, env)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Status).To(Equal(common.Status_BAD_REQUEST))
			Expect(ordererRunners[1].Err()).To(gbytes.Say("channel does not exist"))
			resp, err = ordererclient.Broadcast(network, o3, env)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Status).To(Equal(common.Status_BAD_REQUEST))
			Expect(ordererRunners[2].Err()).To(gbytes.Say("channel does not exist"))
			// With channel participation API, an orderer returns NOT_FOUND for channels it does not serve.
			ensureNotFound(o2, peer, network, "mychannel")
			ensureNotFound(o3, peer, network, "mychannel")

			By("Adding orderer2 to mychannel")
			ordererCertificatePath := filepath.Join(network.OrdererLocalTLSDir(o2), "server.crt")
			ordererCertificate, err := ioutil.ReadFile(ordererCertificatePath)
			Expect(err).NotTo(HaveOccurred())

			addConsenter(network, peer, o1, "mychannel", etcdraft.Consenter{
				ServerTlsCert: ordererCertificate,
				ClientTlsCert: ordererCertificate,
				Host:          "127.0.0.1",
				Port:          uint32(network.OrdererPort(o2, nwo.ClusterPort)),
			})

			configBlock := nwo.GetConfigBlock(network, peer, o1, "mychannel")
			By("Joining orderer2 to mychannel, as consenter")
			expectedChannelInfo := channelparticipation.ChannelInfo{
				Name:              "mychannel",
				URL:               "/participation/v1/channels/mychannel",
				Status:            "onboarding",
				ConsensusRelation: "consenter",
				Height:            0,
			}
			channelparticipation.Join(network, o2, "mychannel", configBlock, expectedChannelInfo)

			By("Waiting for orderer2 to join the channel")
			assertBlockReception(map[string]int{
				"mychannel": 1,
			}, []*nwo.Orderer{o1, o2}, peer, network)

			By("Adding orderer3 to the channel")
			ordererCertificatePath = filepath.Join(network.OrdererLocalTLSDir(o3), "server.crt")
			ordererCertificate, err = ioutil.ReadFile(ordererCertificatePath)
			Expect(err).NotTo(HaveOccurred())
			addConsenter(network, peer, o1, "mychannel", etcdraft.Consenter{
				ServerTlsCert: ordererCertificate,
				ClientTlsCert: ordererCertificate,
				Host:          "127.0.0.1",
				Port:          uint32(network.OrdererPort(o3, nwo.ClusterPort)),
			})

			configBlock = nwo.GetConfigBlock(network, peer, o1, "mychannel")
			By("Joining orderer3 to mychannel, as consenter")
			expectedChannelInfo = channelparticipation.ChannelInfo{
				Name:              "mychannel",
				URL:               "/participation/v1/channels/mychannel",
				Status:            "onboarding",
				ConsensusRelation: "consenter",
				Height:            0,
			}
			channelparticipation.Join(network, o3, "mychannel", configBlock, expectedChannelInfo)

			By("Waiting for orderer3 to join the channel")
			assertBlockReception(map[string]int{
				"mychannel": 2,
			}, orderers, peer, network)
		})
	})

	When("orderer cluster is not healthy", func() {
		var o1, o2 *nwo.Orderer

		BeforeEach(func() {
			network = nwo.New(nwo.MultiNodeEtcdRaftNoSysChan(), testDir, client, StartPort(), components)
			network.GenerateConfigTree()
			network.Bootstrap()

			o1, o2 = network.Orderer("orderer1"), network.Orderer("orderer2")
			orderers := []*nwo.Orderer{o1, o2}
			By("Launching the orderers")
			for _, o := range orderers {
				runner := network.OrdererRunner(o)
				ordererRunners = append(ordererRunners, runner)
				process := ifrit.Invoke(runner)
				ordererProcesses = append(ordererProcesses, process)
			}

			for _, ordererProc := range ordererProcesses {
				Eventually(ordererProc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			}

			channelparticipation.JoinOrderersAppChannelCluster(network, "testchannel", o1, o2)
		})

		AfterEach(func() {
			for _, ordererInstance := range ordererProcesses {
				ordererInstance.Signal(syscall.SIGTERM)
				Eventually(ordererInstance.Wait(), network.EventuallyTimeout).Should(Receive())
			}
		})

		It("refuses to reconfig if it results in quorum loss", func() {
			By("Waiting for them to elect a leader")
			FindLeader(ordererRunners)

			extendNetwork(network)
			certificatesOfOrderers := refreshOrdererPEMs(network)

			By("Removing alive node from 2/3 cluster")
			peer := network.Peer("Org1", "peer0")
			current, updated := consenterRemover(network, peer, o2, "testchannel", certificatesOfOrderers[1].oldCert)
			Eventually(func() []byte {
				sess := nwo.UpdateOrdererConfigSession(network, o2, "testchannel", current, updated, peer, o2)
				Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(1))
				return sess.Err.Contents()
			}, network.EventuallyTimeout).Should(ContainSubstring("2 out of 3 nodes are alive, configuration will result in quorum loss"))

			By("Adding node to 2/3 cluster")
			current, updated = consenterAdder(
				network,
				peer,
				o2,
				"testchannel",
				etcdraft.Consenter{
					ServerTlsCert: certificatesOfOrderers[0].newCert,
					ClientTlsCert: certificatesOfOrderers[0].newCert,
					Host:          "127.0.0.1",
					Port:          uint32(network.OrdererPort(o1, nwo.ListenPort)),
				},
			)
			sess := nwo.UpdateOrdererConfigSession(network, o2, "testchannel", current, updated, peer, o2)
			Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(1))
			Expect(string(sess.Err.Contents())).To(ContainSubstring("2 out of 3 nodes are alive, configuration will result in quorum loss"))
		})
	})

	When("an orderer node is evicted", func() {
		BeforeEach(func() {
			ordererRunners = nil
			ordererProcesses = nil

			network = nwo.New(nwo.MultiNodeEtcdRaftNoSysChan(), testDir, client, StartPort(), components)
			network.GenerateConfigTree()
			network.Bootstrap()

			o1, o2, o3 := network.Orderer("orderer1"), network.Orderer("orderer2"), network.Orderer("orderer3")
			orderers := []*nwo.Orderer{o1, o2, o3}
			peer = network.Peer("Org1", "peer0")

			By("Launching the orderers")
			for _, o := range orderers {
				runner := network.OrdererRunner(o, "FABRIC_LOGGING_SPEC=orderer.consensus.etcdraft=debug:info")
				ordererRunners = append(ordererRunners, runner)
				process := ifrit.Invoke(runner)
				ordererProcesses = append(ordererProcesses, process)
			}

			for _, ordererProc := range ordererProcesses {
				Eventually(ordererProc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			}

			channelparticipation.JoinOrderersAppChannelCluster(network, "testchannel", o1, o2, o3)
		})

		AfterEach(func() {
			for _, ordererInstance := range ordererProcesses {
				ordererInstance.Signal(syscall.SIGTERM)
				Eventually(ordererInstance.Wait(), network.EventuallyTimeout).Should(Receive())
			}
		})

		It("doesn't complain and does it obediently", func() {
			o1, o2, o3 := network.Orderer("orderer1"), network.Orderer("orderer2"), network.Orderer("orderer3")
			orderers := []*nwo.Orderer{o1, o2, o3}

			By("Waiting for them to elect a leader")
			firstEvictedNode := FindLeader(ordererRunners) - 1

			By("Removing the leader from 3-node channel")
			server1CertBytes, err := ioutil.ReadFile(filepath.Join(network.OrdererLocalTLSDir(orderers[firstEvictedNode]), "server.crt"))
			Expect(err).To(Not(HaveOccurred()))

			ordererEvicted1st := network.Orderers[(firstEvictedNode+1)%3]
			removeConsenter(network, peer, ordererEvicted1st, "testchannel", server1CertBytes)

			var survivedOrdererRunners []*ginkgomon.Runner
			for i := range orderers {
				if i == firstEvictedNode {
					continue
				}

				survivedOrdererRunners = append(survivedOrdererRunners, ordererRunners[i])
			}

			secondEvictedNode := FindLeader(survivedOrdererRunners) - 1

			var survivor int
			for i := range orderers {
				if i != firstEvictedNode && i != secondEvictedNode {
					survivor = i
					break
				}
			}

			By("Ensuring the evicted orderer stops rafting on channel testchannel and starts a follower")
			const stopMsg = "Raft node stopped channel=testchannel"
			const followerMsg = "Created and started a follower.Chain for channel testchannel"
			Eventually(ordererRunners[firstEvictedNode].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say(stopMsg))
			Eventually(ordererRunners[firstEvictedNode].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say(followerMsg))

			By("Ensuring the evicted orderer now doesn't serve clients")
			ensureNotFound(orderers[firstEvictedNode], peer, network, "testchannel")

			assertFollower := func(expected channelparticipation.ChannelInfo, o *nwo.Orderer) bool {
				current := channelparticipation.ListOne(network, o, "testchannel")
				ok := current == expected
				if !ok {
					fmt.Fprintf(GinkgoWriter, ">>> Current ChannelInfo: %+v \n", current)
				}
				return ok
			}

			expectedInfo := channelparticipation.ChannelInfo{
				Name:              "testchannel",
				URL:               "/participation/v1/channels/testchannel",
				Status:            "active",
				ConsensusRelation: "follower",
				Height:            2,
			}
			Eventually(assertFollower(expectedInfo, orderers[firstEvictedNode]), network.EventuallyTimeout, 100*time.Millisecond).Should(BeTrue())

			By("Removing the leader from 2-node channel")
			server2CertBytes, err := ioutil.ReadFile(filepath.Join(network.OrdererLocalTLSDir(orderers[secondEvictedNode]), "server.crt"))
			Expect(err).To(Not(HaveOccurred()))

			removeConsenter(network, peer, orderers[survivor], "testchannel", server2CertBytes)
			FindLeader([]*ginkgomon.Runner{ordererRunners[survivor]})

			fmt.Fprintln(GinkgoWriter, "Ensuring the other orderer detect the eviction of the node on channel testchannel")
			Eventually(ordererRunners[secondEvictedNode].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say(stopMsg))

			By("Ensuring the evicted orderer now doesn't serve clients")
			ensureNotFound(orderers[secondEvictedNode], peer, network, "testchannel")

			expectedInfo = channelparticipation.ChannelInfo{
				Name:              "testchannel",
				URL:               "/participation/v1/channels/testchannel",
				Status:            "active",
				ConsensusRelation: "follower",
				Height:            3,
			}
			Eventually(assertFollower(expectedInfo, orderers[secondEvictedNode]), network.EventuallyTimeout, 100*time.Millisecond).Should(BeTrue())

			By("Re-adding first evicted orderer")
			addConsenter(network, peer, network.Orderers[survivor], "testchannel", etcdraft.Consenter{
				Host:          "127.0.0.1",
				Port:          uint32(network.OrdererPort(orderers[firstEvictedNode], nwo.ClusterPort)),
				ClientTlsCert: server1CertBytes,
				ServerTlsCert: server1CertBytes,
			})

			By("Ensuring re-added orderer starts serving testchannel")
			assertBlockReception(map[string]int{
				"testchannel": 3,
			}, []*nwo.Orderer{orderers[firstEvictedNode]}, peer, network)

			By("Submitting tx")
			env := CreateBroadcastEnvelope(network, orderers[survivor], "testchannel", []byte("foo"))
			resp, err := ordererclient.Broadcast(network, orderers[survivor], env)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Status).To(Equal(common.Status_SUCCESS))

			By("Ensuring re-added orderer starts serving testchannel")
			assertBlockReception(map[string]int{
				"testchannel": 4,
			}, []*nwo.Orderer{orderers[firstEvictedNode], orderers[survivor]}, peer, network)
		})

		When("an evicted node is added back while it's offline", func() {
			It("can start with correct new raft id", func() {
				o1 := network.Orderer("orderer1")
				o2 := network.Orderer("orderer2")
				o3 := network.Orderer("orderer3")

				By("Waiting for them to elect a leader")
				FindLeader(ordererRunners)

				assertBlockReception(map[string]int{
					"testchannel": 0,
				}, []*nwo.Orderer{o1, o2, o3}, peer, network)

				By("Killing the first orderer")
				ordererProcesses[0].Signal(syscall.SIGTERM)
				Eventually(ordererProcesses[0].Wait(), network.EventuallyTimeout).Should(Receive())

				// We need to wait for stabilization, as we might have killed the leader OSN.
				By("Waiting for the channel to stabilize after killing the orderer")
				assertBlockReception(map[string]int{
					"testchannel": 0,
				}, []*nwo.Orderer{o2, o3}, peer, network)

				By("observing active nodes to shrink")
				o2Runner := ordererRunners[1]
				Eventually(o2Runner.Err(), network.EventuallyTimeout).Should(gbytes.Say("Current active nodes in cluster are: \\[2 3\\]"))

				By("Removing the first orderer from the application channel")
				server1CertBytes, err := ioutil.ReadFile(filepath.Join(network.OrdererLocalTLSDir(o1), "server.crt"))
				Expect(err).To(Not(HaveOccurred()))
				removeConsenter(network, peer, o2, "testchannel", server1CertBytes)

				By("Adding the evicted orderer back to the application channel")
				addConsenter(network, peer, o2, "testchannel", etcdraft.Consenter{
					ServerTlsCert: server1CertBytes,
					ClientTlsCert: server1CertBytes,
					Host:          "127.0.0.1",
					Port:          uint32(network.OrdererPort(o1, nwo.ClusterPort)),
				})

				By("Removing the first orderer from the application channel again")
				removeConsenter(network, peer, o2, "testchannel", server1CertBytes)

				By("Adding the evicted orderer back to the application channel again")
				addConsenter(network, peer, o2, "testchannel", etcdraft.Consenter{
					ServerTlsCert: server1CertBytes,
					ClientTlsCert: server1CertBytes,
					Host:          "127.0.0.1",
					Port:          uint32(network.OrdererPort(o1, nwo.ClusterPort)),
				})

				By("Starting the first orderer again")
				o1Runner := network.OrdererRunner(o1)
				ordererProcesses[0] = ifrit.Invoke(o1Runner)
				Eventually(ordererProcesses[0].Ready(), network.EventuallyTimeout).Should(BeClosed())

				// TODO the consenter will detect it has a stale RaftID (has 1, everyone else already know it as 5)
				// and "cowardly halt", which will trigger the follower chain. The follower chain will detect using the
				// last block that it is still a consenter, and will transfer control to the consenter, which creates a
				// never ending loop. We need to fix this behavior.
				// Currnetly, with channel participation, the only way to stop this is by explicitly removing the
				// orderer from the channel, which causes the ledger to be deleted.
				// See: https://github.com/hyperledger/fabric/issues/3992
				for i := 0; i < 5; i++ {
					Eventually(o1Runner.Err(), network.EventuallyTimeout).Should(gbytes.Say("Received msg to 5, my ID is probably wrong due to out of date, cowardly halting channel=testchannel node=1"))
					Eventually(o1Runner.Err(), network.EventuallyTimeout).Should(gbytes.Say("Created and started a follower.Chain for channel testchannel"))
				}

				By("Removing channel from the first orderer")
				// TODO the channelparticipation.Remove does not clean up the etcdraft folder. This may prevent the
				// correct re-creation of the channel on this orderer.
				// See: https://github.com/hyperledger/fabric/issues/3992
				channelparticipation.Remove(network, o1, "testchannel")
				Eventually(func() int { // Removal is async
					channelList := channelparticipation.List(network, o1)
					return len(channelList.Channels)
				}()).Should(BeZero())

				// TODO It is recommended to remove the etcdraft folder for the WAL to be re-created correctly
				// See: https://github.com/hyperledger/fabric/issues/3992
				err = os.RemoveAll(path.Join(network.OrdererDir(o1), "etcdraft", "wal", "testchannel"))
				Expect(err).NotTo(HaveOccurred())
				err = os.RemoveAll(path.Join(network.OrdererDir(o1), "etcdraft", "snapshot", "testchannel"))
				Expect(err).NotTo(HaveOccurred())

				By("Re-adding the channel to the first orderer")
				configBlock := nwo.GetConfigBlock(network, peer, o2, "testchannel")
				expectedInfo := channelparticipation.ChannelInfo{
					Name:              "testchannel",
					URL:               "/participation/v1/channels/testchannel",
					Status:            "onboarding",
					ConsensusRelation: "consenter",
					Height:            0,
				}
				channelparticipation.Join(network, o1, "testchannel", configBlock, expectedInfo)

				By("Submitting tx")
				env := CreateBroadcastEnvelope(network, o2, "testchannel", []byte("foo"))
				resp, err := ordererclient.Broadcast(network, o2, env)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.Status).To(Equal(common.Status_SUCCESS))

				By("Waiting for the channel to stabilize")
				expectedInfo = channelparticipation.ChannelInfo{
					Name:              "testchannel",
					URL:               "/participation/v1/channels/testchannel",
					Status:            "active",
					ConsensusRelation: "consenter",
					Height:            6,
				}
				assertCatchup := func(expected channelparticipation.ChannelInfo) bool {
					current := channelparticipation.ListOne(network, o1, "testchannel")
					ok := current == expected
					if !ok {
						fmt.Fprintf(GinkgoWriter, "Current ChannelInfo: %+v", current)
					}
					return ok
				}

				Eventually(assertCatchup(expectedInfo), network.EventuallyTimeout, 100*time.Millisecond).Should(BeTrue())

				assertBlockReception(map[string]int{
					"testchannel": 5,
				}, []*nwo.Orderer{o1, o2, o3}, peer, network)
			})
		})

		It("notices it even if it is down at the time of its eviction", func() {
			o1 := network.Orderer("orderer1")
			o2 := network.Orderer("orderer2")
			o3 := network.Orderer("orderer3")

			orderers := []*nwo.Orderer{o1, o2, o3}

			By("Waiting for them to elect a leader")
			FindLeader(ordererRunners)

			assertBlockReception(map[string]int{
				"testchannel": 0,
			}, []*nwo.Orderer{o1, o2, o3}, peer, network)

			By("Killing the orderer")
			ordererProcesses[0].Signal(syscall.SIGTERM)
			Eventually(ordererProcesses[0].Wait(), network.EventuallyTimeout).Should(Receive())

			// We need to wait for stabilization, as we might have killed the leader OSN.
			By("Waiting for the channel to stabilize after killing the orderer")
			assertBlockReception(map[string]int{
				"testchannel": 0,
			}, []*nwo.Orderer{o2, o3}, peer, network)

			By("Removing the first orderer from an application channel")
			o1cert, err := ioutil.ReadFile(path.Join(network.OrdererLocalTLSDir(o1), "server.crt"))
			Expect(err).ToNot(HaveOccurred())
			removeConsenter(network, peer, o2, "testchannel", o1cert)

			By("Starting the orderer again")
			ordererRunner := network.OrdererRunner(orderers[0])
			ordererRunners[0] = ordererRunner
			ordererProcesses[0] = ifrit.Invoke(ordererRunner)
			Eventually(ordererProcesses[0].Ready(), network.EventuallyTimeout).Should(BeClosed())

			By("Ensuring the remaining OSNs reject authentication")
			Eventually(ordererRunners[1].Err(), time.Minute, time.Second).Should(gbytes.Say("certificate extracted from TLS connection isn't authorized"))
			Eventually(ordererRunners[2].Err(), time.Minute, time.Second).Should(gbytes.Say("certificate extracted from TLS connection isn't authorized"))

			By("Ensuring it detects its eviction")
			evictionDetection := gbytes.Say(`Detected our own eviction from the channel in block \[1\] channel=testchannel`)
			Eventually(ordererRunner.Err(), time.Minute, time.Second).Should(evictionDetection)

			By("Ensuring all blocks are pulled up to the block that evicts the OSN")
			Eventually(ordererRunner.Err(), time.Minute, time.Second).Should(gbytes.Say("Periodic check is stopping. channel=testchannel"))
			Eventually(ordererRunner.Err(), time.Minute, time.Second).Should(gbytes.Say("Pulled all blocks up to eviction block. channel=testchannel"))

			By("Killing the evicted orderer")
			ordererProcesses[0].Signal(syscall.SIGTERM)
			Eventually(ordererProcesses[0].Wait(), network.EventuallyTimeout).Should(Receive())

			By("Starting the evicted orderer again")
			ordererRunner = network.OrdererRunner(orderers[0])
			ordererRunners[0] = ordererRunner
			ordererProcesses[0] = ifrit.Invoke(ordererRunner)
			Eventually(ordererProcesses[0].Ready(), network.EventuallyTimeout).Should(BeClosed())

			By("Ensuring the evicted orderer starts up with the channel as a follower")
			expectedChannelInfo := channelparticipation.ChannelInfo{
				Name:              "testchannel",
				URL:               "/participation/v1/channels/testchannel",
				Status:            "active",
				ConsensusRelation: "follower",
				Height:            2,
			}
			Eventually(func() channelparticipation.ChannelInfo {
				return channelparticipation.ListOne(network, o1, "testchannel")
			}, network.EventuallyTimeout).Should(Equal(expectedChannelInfo))

			By("Adding the evicted orderer back to the application channel")
			addConsenter(network, peer, o2, "testchannel", etcdraft.Consenter{
				ServerTlsCert: o1cert,
				ClientTlsCert: o1cert,
				Host:          "127.0.0.1",
				Port:          uint32(network.OrdererPort(orderers[0], nwo.ClusterPort)),
			})

			By("Ensuring the re-added orderer joins the Raft cluster")
			FindLeader([]*ginkgomon.Runner{ordererRunner})
		})
	})

	When("an orderer node is joined", func() {
		It("isn't influenced by outdated orderers", func() {
			// This test checks that if a lagged is not aware of newly added nodes,
			// among which leader is present, it eventually pulls config block from
			// the orderer it knows, gets the certificates from it and participate
			// in consensus again.
			//
			// Steps:
			// Initial nodes in cluster: <1, 2, 3, 4>
			// - start <1, 2, 3>
			// - add <5, 6, 7>, start <5, 6, 7>
			// - kill <1>
			// - submit a tx, so that Raft index on <1> is behind <5, 6, 7> and <2, 3>
			// - kill <2, 3>
			// - start <1> and <4>. Since <1> is behind <5, 6, 7>, leader is certainly
			//   going to be elected from <5, 6, 7>
			// - assert that even <4> is not aware of leader, it can pull config block
			//   from <1>, and start participating in consensus.

			orderers := make([]*nwo.Orderer, 7)
			ordererRunners = make([]*ginkgomon.Runner, 7)
			ordererProcesses = make([]ifrit.Process, 7)

			for i := range orderers {
				orderers[i] = &nwo.Orderer{
					Name:         fmt.Sprintf("orderer%d", i+1),
					Organization: "OrdererOrg",
				}
			}

			layout := nwo.MultiNodeEtcdRaftNoSysChan()
			layout.Orderers = orderers[:4]
			layout.Profiles[0].Orderers = []string{"orderer1", "orderer2", "orderer3", "orderer4"}

			network = nwo.New(layout, testDir, client, StartPort(), components)
			network.GenerateConfigTree()
			network.Bootstrap()

			peer = network.Peer("Org1", "peer0")

			launch := func(i int) {
				runner := network.OrdererRunner(orderers[i])
				ordererRunners[i] = runner
				process := ifrit.Invoke(runner)
				Eventually(process.Ready(), network.EventuallyTimeout).Should(BeClosed())
				ordererProcesses[i] = process
			}

			By("Launching 3 out of 4 orderers")
			for i := range orderers[:3] {
				launch(i)
			}
			channelparticipation.JoinOrderersAppChannelCluster(network, "testchannel", orderers[:3]...)

			leader := FindLeader(ordererRunners[:3])

			By("Checking that all orderers are online")
			assertBlockReception(map[string]int{
				"testchannel": 0,
			}, orderers[:3], peer, network)

			By("Configuring orderer[5, 6, 7] in the network")
			extendNetwork(network)

			for _, o := range orderers[4:7] {
				ports := nwo.Ports{}
				for _, portName := range nwo.OrdererPortNames() {
					ports[portName] = network.ReservePort()
				}
				network.PortsByOrdererID[o.ID()] = ports

				network.Orderers = append(network.Orderers, o)
				network.GenerateOrdererConfig(o)
			}

			blockNum := 0 // there's only one block in channel - genesis
			for _, i := range []int{4, 5, 6} {
				By(fmt.Sprintf("Adding orderer%d", i+1))
				ordererCertificatePath := filepath.Join(network.OrdererLocalTLSDir(orderers[i]), "server.crt")
				ordererCertificate, err := ioutil.ReadFile(ordererCertificatePath)
				Expect(err).NotTo(HaveOccurred())

				addConsenter(network, peer, orderers[0], "testchannel", etcdraft.Consenter{
					ServerTlsCert: ordererCertificate,
					ClientTlsCert: ordererCertificate,
					Host:          "127.0.0.1",
					Port:          uint32(network.OrdererPort(orderers[i], nwo.ClusterPort)),
				})
				blockNum++

				// Get the last config block of the system channel
				configBlock := nwo.GetConfigBlock(network, peer, orderers[0], "testchannel")

				By(fmt.Sprintf("Launching orderer%d", i+1))
				launch(i)

				expectedInfo := channelparticipation.ChannelInfo{
					Name:              "testchannel",
					URL:               "/participation/v1/channels/testchannel",
					Status:            "onboarding",
					ConsensusRelation: "consenter",
					Height:            0,
				}
				channelparticipation.Join(network, orderers[i], "testchannel", configBlock, expectedInfo)

				By(fmt.Sprintf("Checking that orderer%d has onboarded the network", i+1))
				assertBlockReception(map[string]int{
					"testchannel": blockNum,
				}, []*nwo.Orderer{orderers[i]}, peer, network)
			}

			Expect(FindLeader(ordererRunners[4:])).To(Equal(leader))

			// Later on, when we start [1, 4, 5, 6, 7], we want to make sure that leader
			// is elected from [5, 6, 7], who are unknown to [4]. So we can assert that
			// [4] suspects its own eviction, pulls block from [1], and join the cluster.
			// Otherwise, if [1] is elected, and all other nodes already knew it, [4] may
			// simply replicate missing blocks with Raft, instead of pulling from others
			// triggered by eviction suspector.

			By("Killing orderer1")
			ordererProcesses[0].Signal(syscall.SIGTERM)
			Eventually(ordererProcesses[0].Wait(), network.EventuallyTimeout).Should(Receive())

			By("Submitting another tx to increment Raft index on alive orderers")
			if leader == 1 {
				// if orderer1 was leader, we should expect a new leader being elected before going forward
				FindLeader([]*ginkgomon.Runner{ordererRunners[4]})
			}

			env := CreateBroadcastEnvelope(network, orderers[4], "testchannel", []byte("hello"))
			resp, err := ordererclient.Broadcast(network, orderers[4], env)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Status).To(Equal(common.Status_SUCCESS))
			blockNum++

			assertBlockReception(map[string]int{
				"testchannel": blockNum,
			}, []*nwo.Orderer{orderers[1], orderers[2], orderers[4], orderers[5], orderers[6]}, peer, network) // alive orderers: 2, 3, 5, 6, 7

			By("Killing orderer[2,3]")
			for _, i := range []int{1, 2} {
				ordererProcesses[i].Signal(syscall.SIGTERM)
				Eventually(ordererProcesses[i].Wait(), network.EventuallyTimeout).Should(Receive())
			}

			By("Launching the orderer that was never started from the genesis block")
			launch(3)
			channelparticipation.JoinOrderersAppChannelCluster(network, "testchannel", orderers[3])

			By("Launching orderer1")
			launch(0)

			By("Waiting until it suspects its eviction from the channel")
			Eventually(ordererRunners[3].Err(), time.Minute, time.Second).Should(gbytes.Say("Suspecting our own eviction from the channel"))

			By("Making sure 4/7 orderers form quorum and serve request")
			assertBlockReception(map[string]int{
				"testchannel": blockNum,
			}, []*nwo.Orderer{orderers[0], orderers[3], orderers[4], orderers[5], orderers[6]}, peer, network) // alive orderers: 1, 4, 5, 6, 7
		})
	})

	It("can create a channel that contains a subset of orderers in another channel", func() {
		config := nwo.BasicEtcdRaftNoSysChan()
		config.Orderers = []*nwo.Orderer{
			{Name: "orderer1", Organization: "OrdererOrg"},
			{Name: "orderer2", Organization: "OrdererOrg"},
			{Name: "orderer3", Organization: "OrdererOrg"},
		}
		config.Profiles = []*nwo.Profile{
			{
				Name:          "ThreeOrdererChannel",
				Consortium:    "SampleConsortium",
				Organizations: []string{"Org1", "Org2"},
				Orderers:      []string{"orderer1", "orderer2", "orderer3"},
			}, {
				Name:          "SingleOrdererChannel",
				Consortium:    "SampleConsortium",
				Organizations: []string{"Org1", "Org2"},
				Orderers:      []string{"orderer1"},
			},
		}
		config.Channels = []*nwo.Channel{
			{Name: "single-orderer-channel", Profile: "SingleOrdererChannel", BaseProfile: "SampleDevModeEtcdRaft"},
			{Name: "three-orderer-channel", Profile: "ThreeOrdererChannel"},
		}

		network = nwo.New(config, testDir, client, StartPort(), components)
		o1, o2, o3 := network.Orderer("orderer1"), network.Orderer("orderer2"), network.Orderer("orderer3")
		orderers := []*nwo.Orderer{o1, o2, o3}
		peer = network.Peer("Org1", "peer0")

		network.GenerateConfigTree()
		network.Bootstrap()

		By("Launching the orderers")
		for _, o := range orderers {
			runner := network.OrdererRunner(o)
			ordererRunners = append(ordererRunners, runner)
			process := ifrit.Invoke(runner)
			ordererProcesses = append(ordererProcesses, process)
		}

		for _, ordererProc := range ordererProcesses {
			Eventually(ordererProc.Ready(), network.EventuallyTimeout).Should(BeClosed())
		}

		channelparticipation.JoinOrderersAppChannelCluster(network, "three-orderer-channel", o1, o2, o3)
		FindLeader(ordererRunners)
		channelparticipation.JoinOrderersAppChannelCluster(network, "single-orderer-channel", o1)
		FindLeader([]*ginkgomon.Runner{ordererRunners[0]})

		channelparticipation.ChannelListMatcher(channelparticipation.List(network, o1),
			[]string{"single-orderer-channel", "three-orderer-channel"})
		for _, o := range orderers[1:] {
			channelparticipation.ChannelListMatcher(channelparticipation.List(network, o),
				[]string{"three-orderer-channel"})
		}
	})

	It("can add a new orderer organization", func() {
		network = nwo.New(nwo.MultiNodeEtcdRaftNoSysChan(), testDir, client, StartPort(), components)
		o1, o2, o3 := network.Orderer("orderer1"), network.Orderer("orderer2"), network.Orderer("orderer3")
		orderers := []*nwo.Orderer{o1, o2, o3}

		network.GenerateConfigTree()
		network.Bootstrap()

		By("Launching the orderers")
		for _, o := range orderers {
			runner := network.OrdererRunner(o)
			ordererRunners = append(ordererRunners, runner)
			process := ifrit.Invoke(runner)
			ordererProcesses = append(ordererProcesses, process)
		}

		for _, ordererProc := range ordererProcesses {
			Eventually(ordererProc.Ready(), network.EventuallyTimeout).Should(BeClosed())
		}

		By("Creating the testchannel")
		channelparticipation.JoinOrderersAppChannelCluster(network, "testchannel", o1, o2, o3)

		By("Waiting for the testchannel to be ready")
		FindLeader(ordererRunners)

		peer := network.Peer("Org1", "peer0")
		channel := "testchannel"

		config := nwo.GetConfig(network, peer, o1, channel)
		updatedConfig := proto.Clone(config).(*common.Config)

		ordererOrg := updatedConfig.ChannelGroup.Groups["Orderer"].Groups["OrdererOrg"]
		mspConfig := &msp.MSPConfig{}
		err := proto.Unmarshal(ordererOrg.Values["MSP"].Value, mspConfig)
		Expect(err).NotTo(HaveOccurred())

		fabMSPConfig := &msp.FabricMSPConfig{}
		err = proto.Unmarshal(mspConfig.Config, fabMSPConfig)
		Expect(err).NotTo(HaveOccurred())

		fabMSPConfig.Name = "OrdererMSP2"

		mspConfig.Config, _ = proto.Marshal(fabMSPConfig)
		updatedConfig.ChannelGroup.Groups["Orderer"].Groups["OrdererMSP2"] = &common.ConfigGroup{
			Values: map[string]*common.ConfigValue{
				"MSP": {
					Value:     protoutil.MarshalOrPanic(mspConfig),
					ModPolicy: "Admins",
				},
			},
			ModPolicy: "Admins",
		}

		nwo.UpdateOrdererConfig(network, o1, channel, config, updatedConfig, peer, o1)
	})
})

// ensureNotFound verfies that the orderer responds with NOT_FOUND to a block fetch request.
// With channel participation API, an orderer returns NOT_FOUND for channels it does not serve.
// The old implementation with the system channel and inactive chain used to return SERVICE_UNAVAILABLE.
func ensureNotFound(evictedOrderer *nwo.Orderer, submitter *nwo.Peer, network *nwo.Network, channel string) {
	c := commands.ChannelFetch{
		ChannelID:  channel,
		Block:      "newest",
		OutputFile: "/dev/null",
		Orderer:    network.OrdererAddress(evictedOrderer, nwo.ListenPort),
	}

	sess, err := network.OrdererAdminSession(evictedOrderer, submitter, c)
	Expect(err).NotTo(HaveOccurred())

	Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit())
	Expect(sess.Err).To(gbytes.Say("NOT_FOUND"))
}

var extendedCryptoConfig = `---
OrdererOrgs:
- Name: OrdererOrg
  Domain: example.com
  EnableNodeOUs: false
  CA:
    Hostname: ca
  Specs:
  - Hostname: orderer1
    SANS:
    - localhost
    - 127.0.0.1
    - ::1
  - Hostname: orderer1new
    SANS:
    - localhost
    - 127.0.0.1
    - ::1
  - Hostname: orderer2
    SANS:
    - localhost
    - 127.0.0.1
    - ::1
  - Hostname: orderer2new
    SANS:
    - localhost
    - 127.0.0.1
    - ::1
  - Hostname: orderer3
    SANS:
    - localhost
    - 127.0.0.1
    - ::1
  - Hostname: orderer3new
    SANS:
    - localhost
    - 127.0.0.1
    - ::1
  - Hostname: orderer4
    SANS:
    - localhost
    - 127.0.0.1
    - ::1
  - Hostname: orderer5
    SANS:
    - localhost
    - 127.0.0.1
    - ::1
  - Hostname: orderer6
    SANS:
    - localhost
    - 127.0.0.1
    - ::1
  - Hostname: orderer7
    SANS:
    - localhost
    - 127.0.0.1
    - ::1
`

type certificateChange struct {
	srcFile string
	dstFile string
	oldCert []byte
	oldKey  []byte
	newCert []byte
}

// extendNetwork rotates adds an additional orderer
func extendNetwork(n *nwo.Network) {
	// Overwrite the current crypto-config with additional orderers
	cryptoConfigYAML, err := ioutil.TempFile("", "crypto-config.yaml")
	Expect(err).NotTo(HaveOccurred())
	defer os.Remove(cryptoConfigYAML.Name())

	err = ioutil.WriteFile(cryptoConfigYAML.Name(), []byte(extendedCryptoConfig), 0o644)
	Expect(err).NotTo(HaveOccurred())

	// Invoke cryptogen extend to add new orderers
	sess, err := n.Cryptogen(commands.Extend{
		Config: cryptoConfigYAML.Name(),
		Input:  n.CryptoPath(),
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
}

// refreshOrdererPEMs rotates all TLS certificates of all nodes,
// and returns the deltas
func refreshOrdererPEMs(n *nwo.Network) []*certificateChange {
	var fileChanges []*certificateChange
	// Populate source to destination files
	err := filepath.Walk(n.CryptoPath(), func(path string, info os.FileInfo, err error) error {
		if !strings.Contains(path, "/tls/") {
			return nil
		}
		if strings.Contains(path, "new") {
			fileChanges = append(fileChanges, &certificateChange{
				srcFile: path,
				dstFile: strings.Replace(path, "new", "", -1),
			})
		}
		return nil
	})
	Expect(err).NotTo(HaveOccurred())

	var serverCertChanges []*certificateChange

	// Overwrite the destination files with the contents of the source files.
	for _, certChange := range fileChanges {
		previousCertBytes, err := ioutil.ReadFile(certChange.dstFile)
		Expect(err).NotTo(HaveOccurred())

		newCertBytes, err := ioutil.ReadFile(certChange.srcFile)
		Expect(err).NotTo(HaveOccurred())

		err = ioutil.WriteFile(certChange.dstFile, newCertBytes, 0o644)
		Expect(err).NotTo(HaveOccurred())

		if !strings.Contains(certChange.dstFile, "server.crt") {
			continue
		}

		// Read the previous key file
		previousKeyBytes, err := ioutil.ReadFile(strings.Replace(certChange.dstFile, "server.crt", "server.key", -1))
		Expect(err).NotTo(HaveOccurred())

		serverCertChanges = append(serverCertChanges, certChange)
		certChange.newCert = newCertBytes
		certChange.oldCert = previousCertBytes
		certChange.oldKey = previousKeyBytes
	}
	return serverCertChanges
}

// assertBlockReception asserts that the given orderers have the expected
// newest block number for the specified channels
func assertBlockReception(expectedBlockNumPerChannel map[string]int, orderers []*nwo.Orderer, p *nwo.Peer, n *nwo.Network) {
	for channelName, blockNum := range expectedBlockNumPerChannel {
		for _, orderer := range orderers {
			waitForBlockReception(orderer, p, n, channelName, blockNum)
		}
	}
}

func waitForBlockReception(o *nwo.Orderer, submitter *nwo.Peer, network *nwo.Network, channelName string, blockNum int) {
	c := commands.ChannelFetch{
		ChannelID:  channelName,
		Block:      "newest",
		OutputFile: "/dev/null",
		Orderer:    network.OrdererAddress(o, nwo.ListenPort),
	}
	Eventually(func() string {
		sess, err := network.OrdererAdminSession(o, submitter, c)
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit())
		if sess.ExitCode() != 0 {
			return fmt.Sprintf("exit code is %d: %s", sess.ExitCode(), string(sess.Err.Contents()))
		}
		sessErr := string(sess.Err.Contents())
		expected := fmt.Sprintf("Received block: %d", blockNum)
		if strings.Contains(sessErr, expected) {
			return ""
		}
		return sessErr
	}, network.EventuallyTimeout, time.Second).Should(BeEmpty())
}

func revokeReaderAccess(network *nwo.Network, channel string, orderer *nwo.Orderer, peer *nwo.Peer) {
	config := nwo.GetConfig(network, peer, orderer, channel)
	updatedConfig := proto.Clone(config).(*common.Config)

	// set the policy
	adminPolicy := protoutil.MarshalOrPanic(&common.ImplicitMetaPolicy{
		SubPolicy: "Admins",
		Rule:      common.ImplicitMetaPolicy_MAJORITY,
	})
	updatedConfig.ChannelGroup.Groups["Orderer"].Policies["Readers"].Policy.Value = adminPolicy
	nwo.UpdateOrdererConfig(network, orderer, channel, config, updatedConfig, peer, orderer)
}

// consenterAdder constructs configs that can be used by `UpdateOrdererConfig`
// to add a consenter.
func consenterAdder(n *nwo.Network, peer *nwo.Peer, orderer *nwo.Orderer, channel string, consenter etcdraft.Consenter) (current, updated *common.Config) {
	config := nwo.GetConfig(n, peer, orderer, channel)
	updatedConfig := proto.Clone(config).(*common.Config)

	consensusTypeConfigValue := updatedConfig.ChannelGroup.Groups["Orderer"].Values["ConsensusType"]
	consensusTypeValue := &protosorderer.ConsensusType{}
	err := proto.Unmarshal(consensusTypeConfigValue.Value, consensusTypeValue)
	Expect(err).NotTo(HaveOccurred())

	metadata := &etcdraft.ConfigMetadata{}
	err = proto.Unmarshal(consensusTypeValue.Metadata, metadata)
	Expect(err).NotTo(HaveOccurred())

	metadata.Consenters = append(metadata.Consenters, &consenter)

	consensusTypeValue.Metadata, err = proto.Marshal(metadata)
	Expect(err).NotTo(HaveOccurred())

	updatedConfig.ChannelGroup.Groups["Orderer"].Values["ConsensusType"] = &common.ConfigValue{
		ModPolicy: "Admins",
		Value:     protoutil.MarshalOrPanic(consensusTypeValue),
	}

	return config, updatedConfig
}

// consenterRemover constructs configs that can be used by
// `UpdateOrdererConfig` to remove a consenter.
func consenterRemover(n *nwo.Network, peer *nwo.Peer, orderer *nwo.Orderer, channel string, certificate []byte) (current, updated *common.Config) {
	config := nwo.GetConfig(n, peer, orderer, channel)
	updatedConfig := proto.Clone(config).(*common.Config)

	consensusTypeConfigValue := updatedConfig.ChannelGroup.Groups["Orderer"].Values["ConsensusType"]
	consensusTypeValue := &protosorderer.ConsensusType{}
	err := proto.Unmarshal(consensusTypeConfigValue.Value, consensusTypeValue)
	Expect(err).NotTo(HaveOccurred())

	metadata := &etcdraft.ConfigMetadata{}
	err = proto.Unmarshal(consensusTypeValue.Metadata, metadata)
	Expect(err).NotTo(HaveOccurred())

	var newConsenters []*etcdraft.Consenter
	for _, consenter := range metadata.Consenters {
		if bytes.Equal(consenter.ClientTlsCert, certificate) || bytes.Equal(consenter.ServerTlsCert, certificate) {
			continue
		}
		newConsenters = append(newConsenters, consenter)
	}

	metadata.Consenters = newConsenters
	consensusTypeValue.Metadata, err = proto.Marshal(metadata)
	Expect(err).NotTo(HaveOccurred())

	updatedConfig.ChannelGroup.Groups["Orderer"].Values["ConsensusType"] = &common.ConfigValue{
		ModPolicy: "Admins",
		Value:     protoutil.MarshalOrPanic(consensusTypeValue),
	}

	return config, updatedConfig
}

// addConsenter adds a new consenter to the given channel.
func addConsenter(n *nwo.Network, peer *nwo.Peer, orderer *nwo.Orderer, channel string, consenter etcdraft.Consenter) {
	updateEtcdRaftMetadata(n, peer, orderer, channel, func(metadata *etcdraft.ConfigMetadata) {
		metadata.Consenters = append(metadata.Consenters, &consenter)
	})
}

// removeConsenter removes a consenter with the given certificate in PEM format
// from the given channel.
func removeConsenter(n *nwo.Network, peer *nwo.Peer, orderer *nwo.Orderer, channel string, certificate []byte) {
	updateEtcdRaftMetadata(n, peer, orderer, channel, func(metadata *etcdraft.ConfigMetadata) {
		var newConsenters []*etcdraft.Consenter
		for _, consenter := range metadata.Consenters {
			if bytes.Equal(consenter.ClientTlsCert, certificate) || bytes.Equal(consenter.ServerTlsCert, certificate) {
				continue
			}
			newConsenters = append(newConsenters, consenter)
		}

		metadata.Consenters = newConsenters
	})
}

// updateEtcdRaftMetadata executes a config update that updates the etcdraft
// metadata according to the given function f.
func updateEtcdRaftMetadata(network *nwo.Network, peer *nwo.Peer, orderer *nwo.Orderer, channel string, f func(md *etcdraft.ConfigMetadata)) {
	nwo.UpdateConsensusMetadata(network, peer, orderer, channel, func(originalMetadata []byte) []byte {
		metadata := &etcdraft.ConfigMetadata{}
		err := proto.Unmarshal(originalMetadata, metadata)
		Expect(err).NotTo(HaveOccurred())

		f(metadata)

		newMetadata, err := proto.Marshal(metadata)
		Expect(err).NotTo(HaveOccurred())
		return newMetadata
	})
}

func mutateConsensusMetadata(originalMetadata []byte, f func(md *etcdraft.ConfigMetadata)) []byte {
	metadata := &etcdraft.ConfigMetadata{}
	err := proto.Unmarshal(originalMetadata, metadata)
	Expect(err).NotTo(HaveOccurred())

	f(metadata)

	newMetadata, err := proto.Marshal(metadata)
	Expect(err).NotTo(HaveOccurred())
	return newMetadata
}

func updateOrdererMSPAndConsensusMetadata(network *nwo.Network, peer *nwo.Peer, orderer *nwo.Orderer, channel, orgID string, mutateMSP nwo.MSPMutator, f func(md *etcdraft.ConfigMetadata)) {
	config := nwo.GetConfig(network, peer, orderer, channel)
	updatedConfig := proto.Clone(config).(*common.Config)

	// Unpack the MSP config
	rawMSPConfig := updatedConfig.ChannelGroup.Groups["Orderer"].Groups[orgID].Values["MSP"]
	mspConfig := &msp.MSPConfig{}
	err := proto.Unmarshal(rawMSPConfig.Value, mspConfig)
	Expect(err).NotTo(HaveOccurred())

	fabricConfig := &msp.FabricMSPConfig{}
	err = proto.Unmarshal(mspConfig.Config, fabricConfig)
	Expect(err).NotTo(HaveOccurred())

	// Mutate it as we are asked
	*fabricConfig = mutateMSP(*fabricConfig)

	// Wrap it back into the config
	mspConfig.Config = protoutil.MarshalOrPanic(fabricConfig)
	rawMSPConfig.Value = protoutil.MarshalOrPanic(mspConfig)

	consensusTypeConfigValue := updatedConfig.ChannelGroup.Groups["Orderer"].Values["ConsensusType"]
	consensusTypeValue := &protosorderer.ConsensusType{}
	err = proto.Unmarshal(consensusTypeConfigValue.Value, consensusTypeValue)
	Expect(err).NotTo(HaveOccurred())

	consensusTypeValue.Metadata = mutateConsensusMetadata(consensusTypeValue.Metadata, f)

	updatedConfig.ChannelGroup.Groups["Orderer"].Values["ConsensusType"] = &common.ConfigValue{
		ModPolicy: "Admins",
		Value:     protoutil.MarshalOrPanic(consensusTypeValue),
	}

	nwo.UpdateOrdererConfig(network, orderer, channel, config, updatedConfig, peer, orderer)
}

func configToGenesisBlock(configBlock *common.Block, channelId string) *common.Block {
	genesisBlock := proto.Clone(configBlock).(*common.Block)

	genesisBlock.Header.Number = 0
	genesisBlock.Header.PreviousHash = nil

	var metadataContents [][]byte
	for i := 0; i < len(common.BlockMetadataIndex_name); i++ {
		metadataContents = append(metadataContents, []byte{})
	}
	genesisBlock.Metadata = &common.BlockMetadata{Metadata: metadataContents}
	genesisBlock.Metadata.Metadata[common.BlockMetadataIndex_SIGNATURES] = protoutil.MarshalOrPanic(&common.Metadata{
		Value: protoutil.MarshalOrPanic(&common.OrdererBlockMetadata{
			LastConfig: &common.LastConfig{Index: 0},
		}),
	})

	envelope, err := protoutil.GetEnvelopeFromBlock(genesisBlock.Data.Data[0])
	Expect(err).NotTo(HaveOccurred())
	payload, err := protoutil.UnmarshalPayload(envelope.Payload)
	Expect(err).NotTo(HaveOccurred())

	Expect(err).NotTo(HaveOccurred())
	payloadChannelHeader, err := protoutil.UnmarshalChannelHeader(payload.Header.ChannelHeader)
	Expect(err).NotTo(HaveOccurred())
	payloadChannelHeader.ChannelId = channelId
	payloadSignatureHeader := protoutil.MakeSignatureHeader(nil, protoutil.CreateNonceOrPanic())
	protoutil.SetTxID(payloadChannelHeader, payloadSignatureHeader)
	payload.Header = protoutil.MakePayloadHeader(payloadChannelHeader, payloadSignatureHeader)

	envelope.Payload, err = protoutil.Marshal(payload)
	Expect(err).NotTo(HaveOccurred())
	genesisBlock.Data.Data[0], err = protoutil.Marshal(envelope)
	Expect(err).NotTo(HaveOccurred())

	genesisBlock.Header.DataHash = protoutil.BlockDataHash(genesisBlock.Data)

	return genesisBlock
}
