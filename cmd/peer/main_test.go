/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"fmt"
	"net"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

func TestPluginLoadingFailure(t *testing.T) {
	gt := NewGomegaWithT(t)
	peer, err := gexec.Build("github.com/hyperledger/fabric/cmd/peer")
	gt.Expect(err).NotTo(HaveOccurred())
	defer gexec.CleanupBuildArtifacts()

	parentDir, err := filepath.Abs("../..")
	gt.Expect(err).NotTo(HaveOccurred())

	tempDir := t.TempDir()

	peerListener, err := net.Listen("tcp", "localhost:0")
	gt.Expect(err).NotTo(HaveOccurred())
	peerListenAddress := peerListener.Addr()

	chaincodeListener, err := net.Listen("tcp", "localhost:0")
	gt.Expect(err).NotTo(HaveOccurred())
	chaincodeListenAddress := chaincodeListener.Addr()

	operationsListener, err := net.Listen("tcp", "localhost:0")
	gt.Expect(err).NotTo(HaveOccurred())
	operationsListenAddress := operationsListener.Addr()

	err = peerListener.Close()
	gt.Expect(err).NotTo(HaveOccurred())
	err = chaincodeListener.Close()
	gt.Expect(err).NotTo(HaveOccurred())
	err = operationsListener.Close()
	gt.Expect(err).NotTo(HaveOccurred())

	for _, plugin := range []string{
		"ENDORSERS_ESCC",
		"VALIDATORS_VSCC",
	} {
		t.Run(plugin, func(t *testing.T) {
			cmd := exec.Command(peer, "node", "start")
			cmd.Env = []string{
				"CORE_PEER_FILESYSTEMPATH=" + tempDir,
				"CORE_LEDGER_SNAPSHOTS_ROOTDIR=" + filepath.Join(tempDir, "snapshots"),
				fmt.Sprintf("CORE_PEER_HANDLERS_%s_LIBRARY=%s", plugin, filepath.Join(parentDir, "internal/peer/testdata/invalid_plugins/invalidplugin.so")),
				fmt.Sprintf("CORE_PEER_LISTENADDRESS=%s", peerListenAddress),
				fmt.Sprintf("CORE_PEER_CHAINCODELISTENADDRESS=%s", chaincodeListenAddress),
				"CORE_PEER_MSPCONFIGPATH=msp",
				fmt.Sprintf("CORE_OPERATIONS_LISTENADDRESS=%s", operationsListenAddress),
				"CORE_OPERATIONS_TLS_ENABLED=false",
				"FABRIC_CFG_PATH=" + filepath.Join(parentDir, "sampleconfig"),
			}
			sess, err := gexec.Start(cmd, nil, nil)
			gt.Expect(err).NotTo(HaveOccurred())
			gt.Eventually(sess, time.Minute).Should(gexec.Exit(2))

			gt.Expect(sess.Err).To(gbytes.Say("panic: Error opening plugin at path " + filepath.Join(parentDir, "internal/peer/testdata/invalid_plugins/invalidplugin.so")))
			gt.Expect(sess.Err).To(gbytes.Say("plugin.Open"))
		})
	}
}
