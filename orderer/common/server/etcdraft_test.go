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

	// Create the genesis block for the system channel
	genesisBlockPath := filepath.Join(tempDir, "genesis.block")
	cmd := exec.Command(configtxgen, "-channelID", "system", "-profile", "SampleDevModeEtcdRaft",
		"-outputBlock", genesisBlockPath)
	cmd.Env = append(cmd.Env, fmt.Sprintf("FABRIC_CFG_PATH=%s", filepath.Join(cwd, "testdata")))
	configtxgenProcess, err := gexec.Start(cmd, nil, nil)
	gt.Expect(err).NotTo(HaveOccurred())

	gt.Eventually(configtxgenProcess, time.Minute).Should(gexec.Exit(0))
	gt.Expect(configtxgenProcess.Err).To(gbytes.Say("Writing genesis block"))

	// Launch the OSN
	ordererProcess := launchOrderer(gt, cmd, orderer, tempDir, genesisBlockPath, fabricRootDir)
	gt.Eventually(ordererProcess.Err, time.Minute).Should(gbytes.Say("Beginning to serve requests"))
	gt.Eventually(ordererProcess.Err, time.Minute).Should(gbytes.Say("becomeLeader"))
	ordererProcess.Kill()
}

func launchOrderer(gt *GomegaWithT, cmd *exec.Cmd, orderer, tempDir, genesisBlockPath, fabricRootDir string) *gexec.Session {
	cwd, err := filepath.Abs(".")
	gt.Expect(err).NotTo(HaveOccurred())

	// Launch the orderer process
	cmd = exec.Command(orderer)
	cmd.Env = []string{
		"ORDERER_GENERAL_LISTENPORT=5611",
		"ORDERER_GENERAL_GENESISMETHOD=file",
		"ORDERER_GENERAL_SYSTEMCHANNEL=system",
		"ORDERER_GENERAL_TLS_CLIENTAUTHREQUIRED=true",
		"ORDERER_GENERAL_TLS_ENABLED=true",
		fmt.Sprintf("ORDERER_FILELEDGER_LOCATION=%s", filepath.Join(tempDir, "ledger")),
		fmt.Sprintf("ORDERER_GENERAL_GENESISFILE=%s", genesisBlockPath),
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
