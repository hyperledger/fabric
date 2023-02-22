/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package e2e

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"syscall"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric-lib-go/healthz"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/runner"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/tedsuo/ifrit"
	ginkgomon "github.com/tedsuo/ifrit/ginkgomon_v2"
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

		config := nwo.BasicEtcdRaftNoSysChan()
		network = nwo.New(config, testDir, client, StartPort(), components)
		network.GenerateConfigTree()
		network.Bootstrap()
	})

	AfterEach(func() {
		if process != nil {
			process.Signal(syscall.SIGTERM)
			Eventually(process.Wait(), network.EventuallyTimeout).Should(Receive())
		}
		if network != nil {
			network.Cleanup()
		}
		os.RemoveAll(testDir)
	})

	Describe("CouchDB health checks", func() {
		var (
			couchAddr    string
			authClient   *http.Client
			healthURL    string
			peer         *nwo.Peer
			couchDB      *runner.CouchDB
			couchProcess ifrit.Process
		)

		BeforeEach(func() {
			couchDB = &runner.CouchDB{}
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
			Eventually(process.Ready(), network.EventuallyTimeout).Should(BeClosed())

			authClient, _ = nwo.PeerOperationalClients(network, peer)
			healthURL = fmt.Sprintf("https://127.0.0.1:%d/healthz", network.PeerPort(peer, nwo.OperationsPort))
		})

		AfterEach(func() {
			couchProcess.Signal(syscall.SIGTERM)
			Eventually(couchProcess.Wait(), network.EventuallyTimeout).Should(Receive())
		})

		When("running health checks on Couch DB", func() {
			It("returns appropriate response codes", func() {
				By("returning 200 when able to reach Couch DB")
				statusCode, status := doHealthCheck(authClient, healthURL)
				Expect(statusCode).To(Equal(http.StatusOK))
				Expect(status.Status).To(Equal("OK"))

				By("terminating CouchDB")
				couchProcess.Signal(syscall.SIGTERM)
				Eventually(couchProcess.Wait(), network.EventuallyTimeout).Should(Receive())

				By("waiting for termination to complete")
				Eventually(func() bool {
					if c, err := net.Dial("tcp", couchDB.Address()); err == nil {
						c.Close()
						return false
					}
					return true
				}, network.EventuallyTimeout).Should(BeTrue())

				By("returning 503 when unable to reach Couch DB")
				Eventually(func() int {
					statusCode, _ := doHealthCheck(authClient, healthURL)
					return statusCode
				}, network.EventuallyTimeout).Should(Equal(http.StatusServiceUnavailable))
				statusCode, status = doHealthCheck(authClient, healthURL)
				Expect(statusCode).To(Equal(http.StatusServiceUnavailable))
				Expect(status.Status).To(Equal("Service Unavailable"))
				Expect(status.FailedChecks[0].Component).To(Equal("couchdb"))
				Expect(status.FailedChecks[0].Reason).To(MatchRegexp(fmt.Sprintf(`failed to connect to couch db \[http error calling couchdb: Head "?http://%s"?: dial tcp %s: .*\]`, couchAddr, couchAddr)))
			})
		})
	})
})

func doHealthCheck(client *http.Client, url string) (int, *healthz.HealthStatus) {
	resp, err := client.Get(url)
	Expect(err).NotTo(HaveOccurred())

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	Expect(err).NotTo(HaveOccurred())
	resp.Body.Close()

	// This occurs when a request to the health check server times out, no body is
	// returned when a timeout occurs
	if len(bodyBytes) == 0 {
		return resp.StatusCode, nil
	}

	healthStatus := &healthz.HealthStatus{}
	err = json.Unmarshal(bodyBytes, &healthStatus)
	Expect(err).NotTo(HaveOccurred())

	return resp.StatusCode, healthStatus
}
