/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msp

import (
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/hyperledger/fabric/common/capabilities"
	"github.com/hyperledger/fabric/integration/nwo"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
	ginkgomon "github.com/tedsuo/ifrit/ginkgomon_v2"
)

var _ = Describe("Upstream channel capability compatibility", func() {
	It("rejects a channel requiring the Pactway hybrid capability", func() {
		upstreamPeer := os.Getenv("PACTWAY_UPSTREAM_PEER")
		if upstreamPeer == "" {
			Skip("set PACTWAY_UPSTREAM_PEER to a verified upstream peer binary")
		}

		testDir, err := os.MkdirTemp("", "upstream-channel-capability")
		Expect(err).NotTo(HaveOccurred())
		var network *nwo.Network
		var peerProcess ifrit.Process
		DeferCleanup(func() {
			if peerProcess != nil {
				peerProcess.Signal(syscall.SIGTERM)
				Eventually(peerProcess.Wait(), network.EventuallyTimeout).Should(Receive())
			}
			if network != nil {
				network.Cleanup()
			}
			Expect(os.RemoveAll(testDir)).To(Succeed())
		})

		network = nwo.New(nwo.BasicEtcdRaft(), testDir, nil, StartPort(), components)
		network.Profiles[0].ChannelCapabilities = []string{
			"V3_0",
			capabilities.PactwayHybridCertificateV1,
		}
		network.GenerateConfigTree()
		network.Bootstrap()

		peer := network.Peer("Org1", "peer0")
		peerConfigPath := network.PeerDir(peer)
		peerRunner := ginkgomon.New(ginkgomon.Config{
			Name: peer.ID(),
			Command: &exec.Cmd{
				Path: upstreamPeer,
				Args: []string{upstreamPeer, "node", "start"},
				Env: append(os.Environ(),
					"FABRIC_CFG_PATH="+peerConfigPath,
					"CORE_LEDGER_STATE_COUCHDBCONFIG_USERNAME=admin",
					"CORE_LEDGER_STATE_COUCHDBCONFIG_PASSWORD=adminpw",
				),
			},
			StartCheck:        `Started peer with ID=.*`,
			StartCheckTimeout: 15 * time.Second,
		})
		peerProcess = ifrit.Invoke(peerRunner)
		Eventually(peerProcess.Ready(), network.EventuallyTimeout).Should(BeClosed())

		joinCommand := exec.Command(
			upstreamPeer,
			"channel", "join",
			"-b", network.OutputBlockPath(network.Channels[0].Name),
		)
		joinCommand.Env = append(os.Environ(),
			"FABRIC_CFG_PATH="+peerConfigPath,
			"CORE_PEER_MSPCONFIGPATH="+network.PeerUserMSPDir(peer, "Admin"),
		)
		session, err := gexec.Start(joinCommand, GinkgoWriter, GinkgoWriter)
		Expect(err).NotTo(HaveOccurred())
		Eventually(session, network.EventuallyTimeout).Should(gexec.Exit())
		Expect(session.ExitCode()).NotTo(Equal(0))
		output := strings.TrimSpace(string(session.Out.Contents()) + string(session.Err.Contents()))
		Expect(output).To(ContainSubstring(capabilities.PactwayHybridCertificateV1))
	})
})
