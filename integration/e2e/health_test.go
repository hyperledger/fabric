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
	"os"
	"syscall"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric-lib-go/healthz"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/runner"
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
			process.Signal(syscall.SIGTERM)
			Eventually(process.Wait, network.EventuallyTimeout).Should(Receive())
		}
		if network != nil {
			network.Cleanup()
		}
		os.RemoveAll(testDir)
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

	Describe("CouchDB health checks", func() {
		var (
			couchAddr    string
			authClient   *http.Client
			healthURL    string
			peer         *nwo.Peer
			couchProcess ifrit.Process
		)

		BeforeEach(func() {
			couchDB := &runner.CouchDB{}
			couchProcess = ifrit.Invoke(couchDB)
			Eventually(couchProcess.Ready(), runner.DefaultStartTimeout).Should(BeClosed())
			Consistently(couchProcess.Wait()).ShouldNot(Receive())
			couchAddr = couchDB.Address()

			peer = network.Peer("Org1", "peer0")
			core := network.ReadPeerConfig(peer)
			core.Ledger.State.StateDatabase = "CouchDB"
			core.Ledger.State.CouchDBConfig.CouchDBAddress = couchAddr
			network.WritePeerConfig(peer, core)

			peerRunner := network.PeerRunner(peer)
			process = ginkgomon.Invoke(peerRunner)
			Eventually(process.Ready()).Should(BeClosed())

			authClient, _ = PeerOperationalClients(network, peer)
			healthURL = fmt.Sprintf("https://127.0.0.1:%d/healthz", network.PeerPort(peer, nwo.OperationsPort))
		})

		AfterEach(func() {
			couchProcess.Signal(syscall.SIGTERM)
			Eventually(couchProcess.Wait(), network.EventuallyTimeout).Should(Receive())
		})

		Context("when CouchDB is configured and available", func() {
			It("passes the health check when CouchDB is listening", func() {
				statusCode, status := DoHealthCheck(authClient, healthURL)
				Expect(statusCode).To(Equal(http.StatusOK))
				Expect(status.Status).To(Equal("OK"))
			})
		})

		Context("when CouchDB is unavailable", func() {
			BeforeEach(func() {
				couchProcess.Signal(syscall.SIGTERM)
				Eventually(couchProcess.Wait(), network.EventuallyTimeout).Should(Receive())
			})

			It("fails the health check", func() {
				statusCode, status := DoHealthCheck(authClient, healthURL)
				Expect(statusCode).To(Equal(http.StatusServiceUnavailable))
				Expect(status.Status).To(Equal("Service Unavailable"))
				Expect(status.FailedChecks[0].Component).To(Equal("couchdb"))
				Expect(status.FailedChecks[0].Reason).Should((HavePrefix(fmt.Sprintf("failed to connect to couch db [Head http://%s: dial tcp %s: ", couchAddr, couchAddr))))
			})
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
