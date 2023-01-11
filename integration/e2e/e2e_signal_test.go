/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package e2e

import (
	"io/ioutil"
	"os"
	"syscall"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric/integration/nwo"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/tedsuo/ifrit"
	ginkgomon "github.com/tedsuo/ifrit/ginkgomon_v2"
)

var _ = Describe("SignalHandling", func() {
	var (
		testDir string
		client  *docker.Client
		network *nwo.Network

		peerRunner, ordererRunner   *ginkgomon.Runner
		peerProcess, ordererProcess ifrit.Process
	)

	BeforeEach(func() {
		var err error
		testDir, err = ioutil.TempDir("", "e2e-sigs")
		Expect(err).NotTo(HaveOccurred())

		client, err = docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())

		network = nwo.New(nwo.BasicEtcdRaftNoSysChan(), testDir, client, StartPort(), components)
		network.GenerateConfigTree()
		network.Bootstrap()

		ordererRunner = network.OrdererRunner(network.Orderers[0])
		ordererProcess = ifrit.Invoke(ordererRunner)
		Eventually(ordererProcess.Ready(), network.EventuallyTimeout).Should(BeClosed())

		peerRunner = network.PeerRunner(network.Peers[0])
		peerProcess = ifrit.Invoke(peerRunner)
		Eventually(peerProcess.Ready(), network.EventuallyTimeout).Should(BeClosed())
	})

	AfterEach(func() {
		if peerProcess != nil {
			peerProcess.Signal(syscall.SIGKILL)
		}
		if ordererProcess != nil {
			ordererProcess.Signal(syscall.SIGKILL)
		}
		if network != nil {
			network.Cleanup()
		}
		os.RemoveAll(testDir)
	})

	It("handles signals", func() {
		By("verifying SIGUSR1 to the peer dumps go routines")
		peerProcess.Signal(syscall.SIGUSR1)
		Eventually(peerRunner.Err(), network.EventuallyTimeout).Should(gbytes.Say("Received signal: "))
		Eventually(peerRunner.Err(), network.EventuallyTimeout).Should(gbytes.Say(`Go routines report`))

		By("verifying SIGUSR1 to the orderer dumps go routines")
		ordererProcess.Signal(syscall.SIGUSR1)
		Eventually(ordererRunner.Err(), network.EventuallyTimeout).Should(gbytes.Say("Received signal: "))
		Eventually(ordererRunner.Err(), network.EventuallyTimeout).Should(gbytes.Say(`Go routines report`))

		By("verifying SIGUSR1 does not terminate processes")
		Consistently(peerProcess.Wait()).ShouldNot(Receive())
		Consistently(ordererProcess.Wait()).ShouldNot(Receive())

		By("verifying SIGTERM to the peer stops the process")
		peerProcess.Signal(syscall.SIGTERM)
		Eventually(peerRunner.Err(), network.EventuallyTimeout).Should(gbytes.Say("Received signal: "))
		Eventually(peerProcess.Wait(), network.EventuallyTimeout).Should(Receive())
		peerProcess = nil

		By("verifying SIGTERM to the orderer stops the process")
		ordererProcess.Signal(syscall.SIGTERM)
		Eventually(ordererRunner.Err(), network.EventuallyTimeout).Should(gbytes.Say("Received signal: "))
		Eventually(ordererProcess.Wait(), network.EventuallyTimeout).Should(Receive())
		ordererProcess = nil
	})
})
