/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package e2e

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"syscall"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric-lib-go/healthz"
	"github.com/hyperledger/fabric/integration/nwo"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/ginkgomon"
)

var _ = Describe("Health", func() {
	var (
		testDir string
		client  *docker.Client
		network *nwo.Network
		process ifrit.Process
	)

	BeforeEach(func() {
		var err error
		testDir, err = ioutil.TempDir("", "e2e")
		Expect(err).NotTo(HaveOccurred())

		client, err = docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())

		network = nwo.New(nwo.BasicSolo(), testDir, client, BasePort(), components)
		network.GenerateConfigTree()
		network.Bootstrap()
	})

	AfterEach(func() {
		if process != nil {
			process.Signal(syscall.SIGKILL)
			Eventually(process.Wait).Should(Receive())
		}
	})

	Context("when the docker config is bad", func() {
		It("fails the health check", func() {
			peer := network.Peer("Org1", "peer0")
			core := network.ReadPeerConfig(peer)
			core.VM.Endpoint = "127.0.0.1:0" // bad endpoint
			network.WritePeerConfig(peer, core)

			peerRunner := network.PeerRunner(peer)
			process = ginkgomon.Invoke(peerRunner)
			Eventually(process.Ready()).Should(BeClosed())

			authClient, _ := PeerOperationalClients(network, peer)
			healthURL := fmt.Sprintf("https://127.0.0.1:%d/healthz", network.PeerPort(peer, nwo.OperationsPort))
			statusCode, status := DoHealthCheck(authClient, healthURL)
			Expect(statusCode).To(Equal(http.StatusServiceUnavailable))
			Expect(status.Status).To(Equal("Service Unavailable"))
			Expect(status.FailedChecks).To(ConsistOf(
				healthz.FailedCheck{Component: "docker", Reason: "failed to connect to Docker daemon: invalid endpoint"},
			))
		})
	})
})

func DoHealthCheck(client *http.Client, url string) (int, healthz.HealthStatus) {
	resp, err := client.Get(url)
	Expect(err).NotTo(HaveOccurred())
	bodyBytes, err := ioutil.ReadAll(resp.Body)
	Expect(err).NotTo(HaveOccurred())
	resp.Body.Close()

	var healthStatus healthz.HealthStatus
	err = json.Unmarshal(bodyBytes, &healthStatus)
	Expect(err).NotTo(HaveOccurred())

	return resp.StatusCode, healthStatus
}
