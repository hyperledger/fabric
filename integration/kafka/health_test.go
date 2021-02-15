/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kafka

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
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/tedsuo/ifrit"
)

var _ = Describe("Kafka Health", func() {
	var (
		testDir string
		client  *docker.Client
		network *nwo.Network
		process ifrit.Process
	)

	BeforeEach(func() {
		var err error
		testDir, err = ioutil.TempDir("", "kafka-health")
		Expect(err).NotTo(HaveOccurred())

		client, err = docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())

		config := nwo.BasicKafka()
		config.Consensus.Brokers = 3

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

	Describe("Kafka health checks", func() {
		var (
			oProcess ifrit.Process
			zProcess ifrit.Process
			kProcess []ifrit.Process

			authClient *http.Client
			healthURL  string
		)

		BeforeEach(func() {
			// Start Zookeeper
			zookeepers := []string{}
			zk := network.ZooKeeperRunner(0)
			zookeepers = append(zookeepers, fmt.Sprintf("%s:2181", zk.Name))
			zProcess = ifrit.Invoke(zk)
			Eventually(zProcess.Ready(), network.EventuallyTimeout).Should(BeClosed())

			// Start Kafka Brokers
			for i := 0; i < network.Consensus.Brokers; i++ {
				kafkaRunner := network.BrokerRunner(i, zookeepers)
				kp := ifrit.Invoke(kafkaRunner)
				Eventually(kp.Ready(), network.EventuallyTimeout).Should(BeClosed())
				kProcess = append(kProcess, kp)
			}

			// Start Orderer
			ordererRunner := network.OrdererGroupRunner()
			oProcess = ifrit.Invoke(ordererRunner)
			Eventually(oProcess.Ready(), network.EventuallyTimeout).Should(BeClosed())

			orderer := network.Orderers[0]
			authClient, _ = nwo.OrdererOperationalClients(network, orderer)
			healthURL = fmt.Sprintf("https://127.0.0.1:%d/healthz", network.OrdererPort(orderer, nwo.OperationsPort))
		})

		AfterEach(func() {
			if zProcess != nil {
				zProcess.Signal(syscall.SIGTERM)
				Eventually(zProcess.Wait(), network.EventuallyTimeout).Should(Receive())
			}
			if oProcess != nil {
				oProcess.Signal(syscall.SIGTERM)
				Eventually(oProcess.Wait(), network.EventuallyTimeout).Should(Receive())
			}
			for _, k := range kProcess {
				if k != nil {
					k.Signal(syscall.SIGTERM)
					Eventually(k.Wait(), network.EventuallyTimeout).Should(Receive())
				}
			}
		})

		Context("when running health checks on orderer the kafka health check", func() {
			It("returns appropriate response code", func() {
				By("returning 200 when all brokers are online", func() {
					statusCode, status := doHealthCheck(authClient, healthURL)
					Expect(statusCode).To(Equal(http.StatusOK))
					Expect(status.Status).To(Equal("OK"))
				})

				By("returning a 200 when one of the three brokers goes offline", func() {
					k := kProcess[1]
					k.Signal(syscall.SIGTERM)
					Eventually(k.Wait(), network.EventuallyTimeout).Should(Receive())

					var statusCode int
					var status *healthz.HealthStatus
					Consistently(func() int {
						statusCode, status = doHealthCheck(authClient, healthURL)
						return statusCode
					}).Should(Equal(http.StatusOK))
					Expect(status.Status).To(Equal("OK"))
				})

				By("returning a 503 when two of the three brokers go offline", func() {
					k := kProcess[0]
					k.Signal(syscall.SIGTERM)
					Eventually(k.Wait(), network.EventuallyTimeout).Should(Receive())

					var statusCode int
					var status *healthz.HealthStatus
					Eventually(func() int {
						statusCode, status = doHealthCheck(authClient, healthURL)
						return statusCode
					}, network.EventuallyTimeout).Should(Equal(http.StatusServiceUnavailable))
					Expect(status.Status).To(Equal("Service Unavailable"))
					Expect(status.FailedChecks[0].Component).To(Equal("systemchannel/0"))
					Expect(status.FailedChecks[0].Reason).To(MatchRegexp(`\[replica ids: \[\d \d \d\]\]: kafka server: Messages are rejected since there are fewer in-sync replicas than required\.`))
				})
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
