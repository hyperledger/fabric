/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

func TestSpawnEtcdRaft(t *testing.T) {
	gt := NewGomegaWithT(t)

	cwd, err := filepath.Abs(".")
	gt.Expect(err).NotTo(HaveOccurred())

	// Create tempdir to be used to store the genesis block for the system channel
	tempDir, err := ioutil.TempDir("", "etcdraft-orderer-launch")
	gt.Expect(err).NotTo(HaveOccurred())
	defer os.RemoveAll(tempDir)

	// Set the fabric root folder for easy navigation to sampleconfig folder
	fabricRootDir, err := filepath.Abs(filepath.Join("..", "..", ".."))
	gt.Expect(err).NotTo(HaveOccurred())

	// Build the configtxgen binary
	configtxgen, err := gexec.Build("github.com/hyperledger/fabric/common/tools/configtxgen")
	gt.Expect(err).NotTo(HaveOccurred())

	// Build the orderer binary
	orderer, err := gexec.Build("github.com/hyperledger/fabric/orderer")
	gt.Expect(err).NotTo(HaveOccurred())

	defer gexec.CleanupBuildArtifacts()

	t.Run("Invalid bootstrap block", func(t *testing.T) {
		testEtcdRaftOSNFailureInvalidBootstrapBlock(gt, tempDir, orderer, fabricRootDir)
	})

	t.Run("No TLS", func(t *testing.T) {
		testEtcdRaftOSNNoTLS(gt, tempDir, orderer, fabricRootDir, configtxgen)
	})

	t.Run("EtcdRaft launch success", func(t *testing.T) {
		testEtcdRaftOSNSuccess(gt, tempDir, configtxgen, cwd, orderer, fabricRootDir)
	})
}

func createBootstrapBlock(gt *GomegaWithT, tempDir, configtxgen, cwd string) string {
	// Create the genesis block for the system channel
	genesisBlockPath := filepath.Join(tempDir, "genesis.block")
	cmd := exec.Command(configtxgen, "-channelID", "system", "-profile", "SampleDevModeEtcdRaft",
		"-outputBlock", genesisBlockPath)
	cmd.Env = append(cmd.Env, fmt.Sprintf("FABRIC_CFG_PATH=%s", filepath.Join(cwd, "testdata")))
	configtxgenProcess, err := gexec.Start(cmd, nil, nil)
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Eventually(configtxgenProcess, time.Minute).Should(gexec.Exit(0))
	gt.Expect(configtxgenProcess.Err).To(gbytes.Say("Writing genesis block"))

	return genesisBlockPath
}

func testEtcdRaftOSNSuccess(gt *GomegaWithT, tempDir, configtxgen, cwd, orderer, fabricRootDir string) {
	genesisBlockPath := createBootstrapBlock(gt, tempDir, configtxgen, cwd)

	// Launch the OSN
	ordererProcess := launchOrderer(gt, orderer, tempDir, genesisBlockPath, fabricRootDir)
	defer ordererProcess.Kill()
	// The following configuration parameters are not specified in the orderer.yaml, so let's ensure
	// they are really configured autonomously via the localconfig code.
	gt.Eventually(ordererProcess.Err, time.Minute).Should(gbytes.Say("General.Cluster.DialTimeout = 5s"))
	gt.Eventually(ordererProcess.Err, time.Minute).Should(gbytes.Say("General.Cluster.RPCTimeout = 7s"))
	gt.Eventually(ordererProcess.Err, time.Minute).Should(gbytes.Say("General.Cluster.ReplicationBufferSize = 20971520"))
	gt.Eventually(ordererProcess.Err, time.Minute).Should(gbytes.Say("General.Cluster.ReplicationPullTimeout = 5s"))
	gt.Eventually(ordererProcess.Err, time.Minute).Should(gbytes.Say("General.Cluster.ReplicationRetryTimeout = 5s"))
	gt.Eventually(ordererProcess.Err, time.Minute).Should(gbytes.Say("General.Cluster.ReplicationBackgroundRefreshInterval = 5m0s"))
	gt.Eventually(ordererProcess.Err, time.Minute).Should(gbytes.Say("General.Cluster.ReplicationMaxRetries = 12"))
	gt.Eventually(ordererProcess.Err, time.Minute).Should(gbytes.Say("General.Cluster.SendBufferSize = 10"))
	gt.Eventually(ordererProcess.Err, time.Minute).Should(gbytes.Say("General.Cluster.CertExpirationWarningThreshold = 168h0m0s"))

	// Consensus.EvictionSuspicion is not specified in orderer.yaml, so let's ensure
	// it is really configured autonomously via the etcdraft chain itself.
	gt.Eventually(ordererProcess.Err, time.Minute).Should(gbytes.Say("EvictionSuspicion not set, defaulting to 10m"))
	// Wait until the the node starts up and elects itself as a single leader in a single node cluster.
	gt.Eventually(ordererProcess.Err, time.Minute).Should(gbytes.Say("Starting cluster listener on 127.0.0.1:5612"))
	gt.Eventually(ordererProcess.Err, time.Minute).Should(gbytes.Say("Beginning to serve requests"))
	gt.Eventually(ordererProcess.Err, time.Minute).Should(gbytes.Say("becomeLeader"))
}

func testEtcdRaftOSNFailureInvalidBootstrapBlock(gt *GomegaWithT, tempDir, orderer, fabricRootDir string) {
	// Grab an application channel genesis block
	genesisBlockPath := filepath.Join(filepath.Join("testdata", "mychannel.block"))
	genesisBlockBytes, err := ioutil.ReadFile(genesisBlockPath)
	gt.Expect(err).NotTo(HaveOccurred())

	// Copy it to the designated location in the temporary folder
	genesisBlockPath = filepath.Join(tempDir, "genesis.block")
	err = ioutil.WriteFile(genesisBlockPath, genesisBlockBytes, 0644)
	gt.Expect(err).NotTo(HaveOccurred())

	// Launch the OSN
	ordererProcess := launchOrderer(gt, orderer, tempDir, genesisBlockPath, fabricRootDir)
	defer ordererProcess.Kill()

	expectedErr := "Failed validating bootstrap block: the block isn't a system channel block because it lacks ConsortiumsConfig"
	gt.Eventually(ordererProcess.Err, time.Minute).Should(gbytes.Say(expectedErr))
}

func testEtcdRaftOSNNoTLS(gt *GomegaWithT, tempDir, orderer, fabricRootDir string, configtxgen string) {
	cwd, err := filepath.Abs(".")
	gt.Expect(err).NotTo(HaveOccurred())

	genesisBlockPath := createBootstrapBlock(gt, tempDir, configtxgen, cwd)

	cmd := exec.Command(orderer)
	cmd.Env = []string{
		"ORDERER_GENERAL_LISTENPORT=5611",
		"ORDERER_GENERAL_GENESISMETHOD=file",
		"ORDERER_GENERAL_SYSTEMCHANNEL=system",
		fmt.Sprintf("ORDERER_FILELEDGER_LOCATION=%s", filepath.Join(tempDir, "ledger")),
		fmt.Sprintf("ORDERER_GENERAL_GENESISFILE=%s", genesisBlockPath),
		fmt.Sprintf("FABRIC_CFG_PATH=%s", filepath.Join(fabricRootDir, "sampleconfig")),
	}
	ordererProcess, err := gexec.Start(cmd, nil, nil)
	gt.Expect(err).NotTo(HaveOccurred())
	defer ordererProcess.Kill()

	expectedErr := "TLS is required for running ordering nodes of type etcdraft."
	gt.Eventually(ordererProcess.Err, time.Minute).Should(gbytes.Say(expectedErr))
}

func launchOrderer(gt *GomegaWithT, orderer, tempDir, genesisBlockPath, fabricRootDir string) *gexec.Session {
	cwd, err := filepath.Abs(".")
	gt.Expect(err).NotTo(HaveOccurred())

	// Launch the orderer process
	cmd := exec.Command(orderer)
	cmd.Env = []string{
		"ORDERER_GENERAL_LISTENPORT=5611",
		"ORDERER_GENERAL_GENESISMETHOD=file",
		"ORDERER_GENERAL_SYSTEMCHANNEL=system",
		"ORDERER_GENERAL_TLS_CLIENTAUTHREQUIRED=true",
		"ORDERER_GENERAL_TLS_ENABLED=true",
		"ORDERER_OPERATIONS_TLS_ENABLED=false",
		fmt.Sprintf("ORDERER_FILELEDGER_LOCATION=%s", filepath.Join(tempDir, "ledger")),
		fmt.Sprintf("ORDERER_GENERAL_GENESISFILE=%s", genesisBlockPath),
		"ORDERER_GENERAL_CLUSTER_LISTENPORT=5612",
		"ORDERER_GENERAL_CLUSTER_LISTENADDRESS=127.0.0.1",
		fmt.Sprintf("ORDERER_GENERAL_CLUSTER_SERVERCERTIFICATE=%s", filepath.Join(cwd, "testdata", "tls", "server.crt")),
		fmt.Sprintf("ORDERER_GENERAL_CLUSTER_SERVERPRIVATEKEY=%s", filepath.Join(cwd, "testdata", "tls", "server.key")),
		fmt.Sprintf("ORDERER_GENERAL_CLUSTER_CLIENTCERTIFICATE=%s", filepath.Join(cwd, "testdata", "tls", "server.crt")),
		fmt.Sprintf("ORDERER_GENERAL_CLUSTER_CLIENTPRIVATEKEY=%s", filepath.Join(cwd, "testdata", "tls", "server.key")),
		fmt.Sprintf("ORDERER_GENERAL_CLUSTER_ROOTCAS=[%s]", filepath.Join(cwd, "testdata", "tls", "ca.crt")),
		fmt.Sprintf("ORDERER_GENERAL_TLS_ROOTCAS=[%s]", filepath.Join(cwd, "testdata", "tls", "ca.crt")),
		fmt.Sprintf("ORDERER_GENERAL_TLS_CERTIFICATE=%s", filepath.Join(cwd, "testdata", "tls", "server.crt")),
		fmt.Sprintf("ORDERER_GENERAL_TLS_PRIVATEKEY=%s", filepath.Join(cwd, "testdata", "tls", "server.key")),
		fmt.Sprintf("ORDERER_CONSENSUS_WALDIR=%s", filepath.Join(tempDir, "wal")),
		fmt.Sprintf("ORDERER_CONSENSUS_SNAPDIR=%s", filepath.Join(tempDir, "snapshot")),
		fmt.Sprintf("FABRIC_CFG_PATH=%s", filepath.Join(fabricRootDir, "sampleconfig")),
	}
	sess, err := gexec.Start(cmd, nil, nil)
	gt.Expect(err).NotTo(HaveOccurred())
	return sess
}
