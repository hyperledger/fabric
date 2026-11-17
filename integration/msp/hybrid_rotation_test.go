/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msp

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	mspprotos "github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"github.com/hyperledger/fabric/common/capabilities"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	oqs "github.com/hyperledger/fabric/pq-crypto"
	"github.com/hyperledger/fabric/protoutil"
	dcli "github.com/moby/moby/client"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/ginkgomon_v2"
	"google.golang.org/protobuf/proto"
)

var _ = Describe("Pactway hybrid orderer root rotation", func() {
	It("persists the rotation before signer activation and restarts from the committed config", func() {
		testDir, err := os.MkdirTemp("", "hybrid-root-rotation")
		Expect(err).NotTo(HaveOccurred())

		oldProfile, oldProfileSet := os.LookupEnv("PACTWAY_HYBRID_PROFILE")
		oldDevelopment, oldDevelopmentSet := os.LookupEnv("PACTWAY_ISOLATED_DEVELOPMENT")
		Expect(os.Setenv("PACTWAY_HYBRID_PROFILE", oqs.TemporaryHybridProfileV1)).To(Succeed())
		Expect(os.Setenv("PACTWAY_ISOLATED_DEVELOPMENT", "true")).To(Succeed())

		var ordererProcess, peerProcess ifrit.Process
		var network *nwo.Network
		DeferCleanup(func() {
			if peerProcess != nil {
				peerProcess.Signal(syscall.SIGTERM)
				Eventually(peerProcess.Wait(), network.EventuallyTimeout).Should(Receive())
			}
			if ordererProcess != nil {
				ordererProcess.Signal(syscall.SIGTERM)
				Eventually(ordererProcess.Wait(), network.EventuallyTimeout).Should(Receive())
			}
			if network != nil {
				network.Cleanup()
			}
			Expect(os.RemoveAll(testDir)).To(Succeed())
			restoreEnvironment("PACTWAY_HYBRID_PROFILE", oldProfile, oldProfileSet)
			restoreEnvironment("PACTWAY_ISOLATED_DEVELOPMENT", oldDevelopment, oldDevelopmentSet)
		})

		client, err := dcli.New(dcli.FromEnv)
		Expect(err).NotTo(HaveOccurred())
		network = nwo.New(nwo.BasicEtcdRaft(), testDir, client, StartPort(), components)
		ordererOrg := network.Organization("OrdererOrg")
		ordererOrg.MSPID = "OrdererOrgMSP"
		ordererOrg.EnableNodeOUs = true
		network.Profiles[0].ChannelCapabilities = []string{
			"V3_0",
			capabilities.PactwayHybridCertificateV1,
		}

		network.GenerateConfigTree()
		network.CreateDockerNetwork()
		sess, err := network.Cryptogen(commands.Generate{
			Config: network.CryptoConfigPath(),
			Output: network.CryptoPath(),
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))

		configDir, err := filepath.Abs(filepath.Join("..", "..", "sampleconfig"))
		Expect(err).NotTo(HaveOccurred())
		oldOutput := filepath.Join(testDir, "old-profile")
		newOutput := filepath.Join(testDir, "new-profile")
		generateHybridNetwork(oldOutput, configDir)
		generateHybridNetwork(newOutput, configDir)
		installHybridNetworkMSPs(network, oldOutput)

		for _, channel := range network.Channels {
			sess, err := network.ConfigTxGen(commands.OutputBlock{
				ChannelID:   channel.Name,
				Profile:     channel.Profile,
				ConfigPath:  network.RootDir,
				OutputBlock: network.OutputBlockPath(channel.Name),
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
		}
		network.ConcatenateTLSCACertificates()

		orderer := network.Orderer("orderer")
		org1Peer := network.Peer("Org1", "peer0")
		org2Peer := network.Peer("Org2", "peer0")
		ordererRunner, startedOrderer, startedPeers := network.StartSingleOrdererNetwork(orderer.Name)
		ordererProcess, peerProcess = startedOrderer, startedPeers
		nwo.JoinOrdererJoinPeersAppChannel(network, "testchannel", orderer, ordererRunner)

		currentBlock := nwo.CurrentConfigBlockNumber(network, org1Peer, nil, "testchannel")
		currentConfig := nwo.GetConfig(network, org1Peer, orderer, "testchannel")
		updatedConfig := proto.Clone(currentConfig).(*common.Config)
		ordererGroup := updatedConfig.GetChannelGroup().GetGroups()["Orderer"]
		ordererMSPGroup := ordererGroup.GetGroups()[ordererOrg.Name]
		currentMSP := &mspprotos.MSPConfig{}
		Expect(proto.Unmarshal(ordererMSPGroup.GetValues()["MSP"].GetValue(), currentMSP)).To(Succeed())
		currentFabricMSP := &mspprotos.FabricMSPConfig{}
		Expect(proto.Unmarshal(currentMSP.GetConfig(), currentFabricMSP)).To(Succeed())

		replacementMSP := ordererMSPConfigFromGenesis(newOutput)
		replacementFabricMSP := &mspprotos.FabricMSPConfig{}
		Expect(proto.Unmarshal(replacementMSP.GetConfig(), replacementFabricMSP)).To(Succeed())
		Expect(replacementFabricMSP.GetRootCerts()).NotTo(Equal(currentFabricMSP.GetRootCerts()))
		replacementFabricMSP.TlsRootCerts = currentFabricMSP.GetTlsRootCerts()
		replacementFabricMSP.TlsIntermediateCerts = currentFabricMSP.GetTlsIntermediateCerts()
		replacementMSP.Config = protoutil.MarshalOrPanic(replacementFabricMSP)
		ordererMSPGroup.GetValues()["MSP"].Value = protoutil.MarshalOrPanic(replacementMSP)

		updateFile := filepath.Join(testDir, "orderer-msp-update.pb")
		nwo.ComputeUpdateOrdererConfig(updateFile, network, "testchannel", currentConfig, updatedConfig, org1Peer, orderer)
		updateBytes, err := os.ReadFile(updateFile)
		Expect(err).NotTo(HaveOccurred())
		updateEnvelope := &common.Envelope{}
		Expect(proto.Unmarshal(updateBytes, updateEnvelope)).To(Succeed())
		nwo.Update(network, orderer, "testchannel", updateEnvelope)

		var ordererExitErr error
		Eventually(ordererProcess.Wait(), network.EventuallyTimeout).Should(Receive(&ordererExitErr))
		Expect(ordererExitErr).To(HaveOccurred())
		Expect(ordererRunner.Err()).To(gbytes.Say("incompatible local signing identity"))
		ordererProcess = nil

		staleRunner := network.OrdererRunner(orderer)
		ordererProcess = ifrit.Invoke(staleRunner)
		Eventually(ordererProcess.Wait(), network.EventuallyTimeout).Should(Receive(&ordererExitErr))
		ordererProcess = nil
		Expect(ordererExitErr).To(HaveOccurred())
		Expect(staleRunner.Err()).To(gbytes.Say("validating local signing identity for PQ-required channel"))

		installOrdererHybridIdentity(network, newOutput)

		_, ordererProcess = network.StartOrderer(orderer.Name)
		Eventually(func() uint64 {
			return nwo.CurrentConfigBlockNumber(network, org1Peer, nil, "testchannel")
		}, network.EventuallyTimeout).Should(BeNumerically(">", currentBlock))
		peerProcess.Signal(syscall.SIGTERM)
		Eventually(peerProcess.Wait(), network.EventuallyTimeout).Should(Receive())
		peerRunner := network.PeerGroupRunner()
		peerProcess = ifrit.Invoke(peerRunner)
		Eventually(peerProcess.Ready(), network.EventuallyTimeout).Should(BeClosed())

		rotatedConfig := nwo.GetConfig(network, org1Peer, orderer, "testchannel")
		nwo.EnableCapabilities(network, "testchannel", "Application", "V2_0", orderer, org1Peer, org2Peer)
		Eventually(func() uint64 {
			return nwo.CurrentConfigBlockNumber(network, org1Peer, nil, "testchannel")
		}, network.EventuallyTimeout).Should(BeNumerically(">", currentBlock+1))

		peerProcess.Signal(syscall.SIGTERM)
		Eventually(peerProcess.Wait(), network.EventuallyTimeout).Should(Receive())
		peerProcess = ifrit.Invoke(network.PeerGroupRunner())
		Eventually(peerProcess.Ready(), network.EventuallyTimeout).Should(BeClosed())

		replayedConfig := nwo.GetConfig(network, org1Peer, orderer, "testchannel")
		Expect(proto.Equal(rotatedConfig.GetChannelGroup().GetGroups()["Orderer"].GetGroups()[ordererOrg.Name],
			replayedConfig.GetChannelGroup().GetGroups()["Orderer"].GetGroups()[ordererOrg.Name])).To(BeTrue())
	})

	It("rotates a three-orderer cluster with overlapping roots and rejects a stale signer", func() {
		testDir, err := os.MkdirTemp("", "hybrid-multi-orderer-rotation")
		Expect(err).NotTo(HaveOccurred())
		oldProfile, oldProfileSet := os.LookupEnv("PACTWAY_HYBRID_PROFILE")
		oldDevelopment, oldDevelopmentSet := os.LookupEnv("PACTWAY_ISOLATED_DEVELOPMENT")
		Expect(os.Setenv("PACTWAY_HYBRID_PROFILE", oqs.TemporaryHybridProfileV1)).To(Succeed())
		Expect(os.Setenv("PACTWAY_ISOLATED_DEVELOPMENT", "true")).To(Succeed())

		var network *nwo.Network
		var peerProcess ifrit.Process
		ordererProcesses := make([]ifrit.Process, 3)
		DeferCleanup(func() {
			if peerProcess != nil {
				peerProcess.Signal(syscall.SIGTERM)
				Eventually(peerProcess.Wait(), network.EventuallyTimeout).Should(Receive())
			}
			for _, process := range ordererProcesses {
				if process != nil {
					process.Signal(syscall.SIGTERM)
					Eventually(process.Wait(), network.EventuallyTimeout).Should(Receive())
				}
			}
			if network != nil {
				network.Cleanup()
			}
			Expect(os.RemoveAll(testDir)).To(Succeed())
			restoreEnvironment("PACTWAY_HYBRID_PROFILE", oldProfile, oldProfileSet)
			restoreEnvironment("PACTWAY_ISOLATED_DEVELOPMENT", oldDevelopment, oldDevelopmentSet)
		})

		client, err := dcli.New(dcli.FromEnv)
		Expect(err).NotTo(HaveOccurred())
		network = nwo.New(nwo.MultiNodeEtcdRaft(), testDir, client, StartPort(), components)
		ordererOrg := network.Organization("OrdererOrg")
		ordererOrg.MSPID = "OrdererOrgMSP"
		ordererOrg.EnableNodeOUs = true
		network.Profiles[0].ChannelCapabilities = []string{"V3_0", capabilities.PactwayHybridCertificateV1}
		network.GenerateConfigTree()
		network.CreateDockerNetwork()
		sess, err := network.Cryptogen(commands.Generate{Config: network.CryptoConfigPath(), Output: network.CryptoPath()})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))

		configDir, err := filepath.Abs(filepath.Join("..", "..", "sampleconfig"))
		Expect(err).NotTo(HaveOccurred())
		oldOutputs, newOutputs := make([]string, 3), make([]string, 3)
		identities := make(map[string]bool)
		for i := range oldOutputs {
			oldOutputs[i] = filepath.Join(testDir, fmt.Sprintf("old-profile-%d", i))
			newOutputs[i] = filepath.Join(testDir, fmt.Sprintf("new-profile-%d", i))
			generateHybridNetwork(oldOutputs[i], configDir)
			generateHybridNetwork(newOutputs[i], configDir)
			for _, output := range []string{oldOutputs[i], newOutputs[i]} {
				cert, err := os.ReadFile(filepath.Join(output, "public", "organizations", "OrdererOrg", "identities", "orderer0-cert.pem"))
				Expect(err).NotTo(HaveOccurred())
				Expect(identities).NotTo(HaveKey(string(cert)))
				identities[string(cert)] = true
			}
		}
		installHybridPeerMSPs(network, oldOutputs[0])
		ordererMSPDir := network.OrdererOrgMSPDir(ordererOrg)
		Expect(installMSPKeepingTLS(filepath.Join(oldOutputs[0], "public", "organizations", "OrdererOrg", "msp"), ordererMSPDir)).To(Succeed())
		configPath := filepath.Join(ordererMSPDir, "config.yaml")
		configBytes, err := os.ReadFile(configPath)
		Expect(err).NotTo(HaveOccurred())
		certPin := []byte("    Certificate: intermediatecerts/intermediate.pem\n")
		Expect(bytes.Count(configBytes, certPin)).To(Equal(4))
		configBytes = bytes.ReplaceAll(configBytes, certPin, nil)
		Expect(os.WriteFile(configPath, configBytes, 0o644)).To(Succeed())
		for i, orderer := range network.Orderers {
			installMultiOrdererIdentity(network, orderer, oldOutputs[i])
			if i > 0 {
				for _, cert := range []struct{ dir, name string }{{"cacerts", "root.pem"}, {"intermediatecerts", "intermediate.pem"}} {
					source := filepath.Join(oldOutputs[i], "public", "organizations", "OrdererOrg", "msp", cert.dir, cert.name)
					data, err := os.ReadFile(source)
					Expect(err).NotTo(HaveOccurred())
					Expect(os.WriteFile(filepath.Join(ordererMSPDir, cert.dir, fmt.Sprintf("additional-%d.pem", i)), data, 0o644)).To(Succeed())
				}
			}
		}
		for _, channel := range network.Channels {
			sess, err := network.ConfigTxGen(commands.OutputBlock{
				ChannelID: channel.Name, Profile: channel.Profile, ConfigPath: network.RootDir,
				OutputBlock: network.OutputBlockPath(channel.Name),
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
		}
		network.ConcatenateTLSCACertificates()

		runners := make([]*ginkgomon_v2.Runner, 3)
		for i, orderer := range network.Orderers {
			runners[i], ordererProcesses[i] = network.StartOrderer(orderer.Name)
		}
		peerProcess = ifrit.Invoke(network.PeerGroupRunner())
		Eventually(peerProcess.Ready(), network.EventuallyTimeout).Should(BeClosed())
		nwo.JoinOrderersAppChannelCluster(network, "testchannel", network.Orderers...)
		for _, runner := range runners {
			Eventually(runner.Err(), network.EventuallyTimeout, time.Second).Should(
				gbytes.Say("Raft leader changed: 0 -> [1-9][0-9]* channel=testchannel"),
			)
		}
		orderer := network.Orderers[0]
		peer := network.Peer("Org1", "peer0")
		network.JoinChannel("testchannel", orderer, network.PeersWithChannel("testchannel")...)

		updateRoots := func(removeOld bool) {
			current := nwo.GetConfig(network, peer, orderer, "testchannel")
			updated := proto.Clone(current).(*common.Config)
			value := updated.GetChannelGroup().GetGroups()["Orderer"].GetGroups()[ordererOrg.Name].GetValues()["MSP"]
			config := &mspprotos.MSPConfig{}
			Expect(proto.Unmarshal(value.GetValue(), config)).To(Succeed())
			fabricConfig := &mspprotos.FabricMSPConfig{}
			Expect(proto.Unmarshal(config.GetConfig(), fabricConfig)).To(Succeed())
			if removeOld {
				fabricConfig.RootCerts = nil
				fabricConfig.IntermediateCerts = nil
			}
			for _, output := range newOutputs {
				newConfig := ordererMSPConfigFromGenesis(output)
				newFabricConfig := &mspprotos.FabricMSPConfig{}
				Expect(proto.Unmarshal(newConfig.GetConfig(), newFabricConfig)).To(Succeed())
				fabricConfig.RootCerts = append(fabricConfig.RootCerts, newFabricConfig.GetRootCerts()...)
				fabricConfig.IntermediateCerts = append(fabricConfig.IntermediateCerts, newFabricConfig.GetIntermediateCerts()...)
			}
			if removeOld {
				newConfig := ordererMSPConfigFromGenesis(newOutputs[0])
				newFabricConfig := &mspprotos.FabricMSPConfig{}
				Expect(proto.Unmarshal(newConfig.GetConfig(), newFabricConfig)).To(Succeed())
				fabricConfig.Admins = newFabricConfig.GetAdmins()
			}
			config.Config = protoutil.MarshalOrPanic(fabricConfig)
			value.Value = protoutil.MarshalOrPanic(config)
			nwo.UpdateOrdererConfig(network, orderer, "testchannel", current, updated, peer, orderer)
		}

		updateRoots(false)
		for i, node := range network.Orderers {
			ordererProcesses[i].Signal(syscall.SIGTERM)
			Eventually(ordererProcesses[i].Wait(), network.EventuallyTimeout).Should(Receive())
			ordererProcesses[i] = nil
			installMultiOrdererIdentity(network, node, newOutputs[i])
			runners[i], ordererProcesses[i] = network.StartOrderer(node.Name)
			Eventually(runners[i].Err(), network.EventuallyTimeout, time.Second).Should(
				gbytes.Say("Raft leader changed: 0 -> [1-9][0-9]* channel=testchannel"),
			)
		}
		updateRoots(true)
		rotatedConfig := nwo.GetConfig(network, peer, orderer, "testchannel")

		last := len(network.Orderers) - 1
		ordererProcesses[last].Signal(syscall.SIGTERM)
		Eventually(ordererProcesses[last].Wait(), network.EventuallyTimeout).Should(Receive())
		ordererProcesses[last] = nil
		installMultiOrdererIdentity(network, network.Orderers[last], oldOutputs[last])
		staleRunner := network.OrdererRunner(network.Orderers[last])
		ordererProcesses[last] = ifrit.Invoke(staleRunner)
		var staleExit error
		Eventually(ordererProcesses[last].Wait(), network.EventuallyTimeout).Should(Receive(&staleExit))
		ordererProcesses[last] = nil
		Expect(staleExit).To(HaveOccurred())
		Expect(staleRunner.Err()).To(gbytes.Say("validating local signing identity for PQ-required channel"))
		installMultiOrdererIdentity(network, network.Orderers[last], newOutputs[last])
		runners[last], ordererProcesses[last] = network.StartOrderer(network.Orderers[last].Name)
		Eventually(runners[last].Err(), network.EventuallyTimeout, time.Second).Should(
			gbytes.Say("Raft leader changed: 0 -> [1-9][0-9]* channel=testchannel"),
		)
		replayedConfig := nwo.GetConfig(network, peer, network.Orderers[last], "testchannel")
		Expect(proto.Equal(rotatedConfig.GetChannelGroup().GetGroups()["Orderer"].GetGroups()[ordererOrg.Name],
			replayedConfig.GetChannelGroup().GetGroups()["Orderer"].GetGroups()[ordererOrg.Name])).To(BeTrue())
	})
})

func generateHybridNetwork(outputDir, configDir string) {
	command := exec.Command(
		pqGenerator,
		"-output="+outputDir,
		"-configtx-dir="+configDir,
		"-fabric-profile=SampleAppChannelEtcdRaft",
		"-hybrid-profile="+oqs.TemporaryHybridProfileV1,
	)
	sess, err := gexec.Start(command, GinkgoWriter, GinkgoWriter)
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, 5*time.Minute).Should(gexec.Exit(0))
}

func installHybridNetworkMSPs(network *nwo.Network, outputDir string) {
	installHybridPeerMSPs(network, outputDir)

	orderer := network.Orderer("orderer")
	ordererOrg := network.Organization("OrdererOrg")
	publicSource := filepath.Join(outputDir, "public", "organizations", "OrdererOrg", "msp")
	Expect(installMSPKeepingTLS(publicSource, network.OrdererOrgMSPDir(ordererOrg))).To(Succeed())
	installMultiOrdererIdentity(network, orderer, outputDir)
}

func installHybridPeerMSPs(network *nwo.Network, outputDir string) {
	for _, peer := range network.Peers {
		org := network.Organization(peer.Organization)
		publicSource := filepath.Join(outputDir, "public", "organizations", peer.Organization, "msp")
		Expect(installMSPKeepingTLS(publicSource, network.PeerOrgMSPDir(org))).To(Succeed())

		privateSource := filepath.Join(outputDir, "private", "organizations", peer.Organization)
		nodeSource := filepath.Join(privateSource, "nodes", "peer0", "msp")
		Expect(installMSPKeepingTLS(nodeSource, network.PeerLocalMSPDir(peer))).To(Succeed())
		for user, sourceUser := range map[string]string{
			"Admin": "Admin",
			"User1": "Client",
			"User2": "Client",
		} {
			source := filepath.Join(privateSource, "users", sourceUser, "msp")
			Expect(installMSPKeepingTLS(source, network.PeerUserMSPDir(peer, user))).To(Succeed())
		}
	}
}

func installMultiOrdererIdentity(network *nwo.Network, orderer *nwo.Orderer, outputDir string) {
	privateSource := filepath.Join(outputDir, "private", "organizations", "OrdererOrg")
	Expect(installMSPKeepingTLS(
		filepath.Join(privateSource, "nodes", "orderer0", "msp"),
		network.OrdererLocalMSPDir(orderer),
	)).To(Succeed())
	Expect(installMSPKeepingTLS(
		filepath.Join(privateSource, "users", "Admin", "msp"),
		network.OrdererUserMSPDir(orderer, "Admin"),
	)).To(Succeed())
}

func installOrdererHybridIdentity(network *nwo.Network, outputDir string) {
	orderer := network.Orderer("orderer")
	privateSource := filepath.Join(outputDir, "private", "organizations", "OrdererOrg")
	Expect(installMSPKeepingTLS(
		filepath.Join(privateSource, "nodes", "orderer0", "msp"),
		network.OrdererLocalMSPDir(orderer),
	)).To(Succeed())
	Expect(installMSPKeepingTLS(
		filepath.Join(privateSource, "users", "Admin", "msp"),
		network.OrdererUserMSPDir(orderer, "Admin"),
	)).To(Succeed())
}

func ordererMSPConfigFromGenesis(outputDir string) *mspprotos.MSPConfig {
	block := nwo.UnmarshalBlockFromFile(filepath.Join(outputDir, "public", "genesis.block"))
	envelope, err := protoutil.GetEnvelopeFromBlock(block.GetData().GetData()[0])
	Expect(err).NotTo(HaveOccurred())
	payload, err := protoutil.UnmarshalPayload(envelope.GetPayload())
	Expect(err).NotTo(HaveOccurred())
	configEnvelope := &common.ConfigEnvelope{}
	Expect(proto.Unmarshal(payload.GetData(), configEnvelope)).To(Succeed())
	ordererOrgGroup := configEnvelope.GetConfig().GetChannelGroup().GetGroups()["Orderer"].GetGroups()["OrdererOrg"]
	mspConfig := &mspprotos.MSPConfig{}
	Expect(proto.Unmarshal(ordererOrgGroup.GetValues()["MSP"].GetValue(), mspConfig)).To(Succeed())
	return mspConfig
}

func installMSPKeepingTLS(source, destination string) error {
	tlsDirs := []string{"tlscacerts", "tlsintermediatecerts"}
	backups := make(map[string]string)
	for _, name := range tlsDirs {
		path := filepath.Join(destination, name)
		if _, err := os.Stat(path); err == nil {
			backup := destination + ".preserved-" + name
			if err := os.RemoveAll(backup); err != nil {
				return err
			}
			if err := os.Rename(path, backup); err != nil {
				return err
			}
			backups[name] = backup
		} else if !os.IsNotExist(err) {
			return err
		}
	}

	if err := os.RemoveAll(destination); err != nil {
		return err
	}
	if err := copyDirectory(source, destination); err != nil {
		return err
	}
	for name, backup := range backups {
		if err := os.RemoveAll(filepath.Join(destination, name)); err != nil {
			return err
		}
		if err := os.Rename(backup, filepath.Join(destination, name)); err != nil {
			return err
		}
	}
	return nil
}

func copyDirectory(source, destination string) error {
	return filepath.WalkDir(source, func(sourcePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relativePath, err := filepath.Rel(source, sourcePath)
		if err != nil {
			return err
		}
		destinationPath := filepath.Join(destination, relativePath)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(destinationPath, info.Mode().Perm())
		}
		sourceFile, err := os.Open(sourcePath)
		if err != nil {
			return err
		}
		destinationFile, err := os.OpenFile(destinationPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			sourceFile.Close()
			return err
		}
		_, copyErr := io.Copy(destinationFile, sourceFile)
		sourceCloseErr := sourceFile.Close()
		closeErr := destinationFile.Close()
		if copyErr != nil {
			return copyErr
		}
		if sourceCloseErr != nil {
			return sourceCloseErr
		}
		return closeErr
	})
}

func restoreEnvironment(name, value string, existed bool) {
	if existed {
		Expect(os.Setenv(name, value)).To(Succeed())
		return
	}
	Expect(os.Unsetenv(name)).To(Succeed())
}
