/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msp

import (
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

	orderer := network.Orderer("orderer")
	ordererOrg := network.Organization("OrdererOrg")
	publicSource := filepath.Join(outputDir, "public", "organizations", "OrdererOrg", "msp")
	Expect(installMSPKeepingTLS(publicSource, network.OrdererOrgMSPDir(ordererOrg))).To(Succeed())
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
