/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package runner_test

import (
	"fmt"
	"io"
	"net"
	"syscall"
	"time"

	"github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric/integration/helpers"
	"github.com/hyperledger/fabric/integration/runner"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/tedsuo/ifrit"
)

var _ = Describe("Kafka Runner", func() {
	var (
		client  *docker.Client
		network *docker.Network

		outBuffer *gbytes.Buffer
		kafka     *runner.Kafka
		zookeeper *runner.ZooKeeper

		process ifrit.Process
	)

	BeforeEach(func() {
		outBuffer = gbytes.NewBuffer()
		process = nil

		var err error
		client, err = docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())

		// Create a network
		networkName := helpers.UniqueName()
		network, err = client.CreateNetwork(
			docker.CreateNetworkOptions{
				Name:   networkName,
				Driver: "bridge",
			},
		)

		// Start a ZooKeeper
		zookeeper = &runner.ZooKeeper{
			Name:        helpers.UniqueName(),
			ZooMyID:     1,
			NetworkName: network.Name,
		}
		err = zookeeper.Start()
		Expect(err).NotTo(HaveOccurred())

		kafka = &runner.Kafka{
			ErrorStream:      GinkgoWriter,
			OutputStream:     io.MultiWriter(outBuffer, GinkgoWriter),
			ZooKeeperConnect: net.JoinHostPort(zookeeper.Name, "2181"),
			BrokerID:         1,
			NetworkName:      network.Name,
		}
	})

	AfterEach(func() {
		if process != nil {
			process.Signal(syscall.SIGTERM)
		}

		err := zookeeper.Stop()
		Expect(err).NotTo(HaveOccurred())

		if network != nil {
			client.RemoveNetwork(network.Name)
		}
	})

	It("starts and stops a docker container with the specified image", func() {
		By("naming the container")
		kafka.Name = "kafka0"

		By("starting kafka broker")
		process = ifrit.Invoke(kafka)
		Eventually(process.Ready(), runner.DefaultStartTimeout).Should(BeClosed())
		Consistently(process.Wait()).ShouldNot(Receive())

		By("inspecting the container by name")
		container, err := client.InspectContainer("kafka0")
		Expect(err).NotTo(HaveOccurred())
		Expect(container.Name).To(Equal("/kafka0"))
		Expect(container.State.Status).To(Equal("running"))
		Expect(container.Config).NotTo(BeNil())
		Expect(container.Config.Image).To(Equal("hyperledger/fabric-kafka:latest"))
		Expect(container.ID).To(Equal(kafka.ContainerID))
		portBindings := container.NetworkSettings.Ports[docker.Port("9092/tcp")]
		Expect(portBindings).To(HaveLen(1))
		Expect(kafka.HostAddress).To(Equal(net.JoinHostPort(portBindings[0].HostIP, portBindings[0].HostPort)))
		Expect(kafka.ContainerAddress).To(Equal(net.JoinHostPort(container.NetworkSettings.Networks[kafka.NetworkName].IPAddress, "9092")))

		By("getting the container logs")
		Eventually(outBuffer, 30*time.Second).Should(gbytes.Say(`\Q[KafkaServer id=1] started (kafka.server.KafkaServer)\E`))

		By("accessing the kafka broker")
		address := kafka.Address
		Expect(address).NotTo(BeEmpty())

		By("terminating the container")
		process.Signal(syscall.SIGTERM)
		Eventually(process.Wait(), 10*time.Second).Should(Receive())
		process = nil

		Eventually(ContainerExists(client, "kafka0")).Should(BeFalse())
	})

	It("can be started and stopped without ifrit", func() {
		err := kafka.Start()
		Expect(err).NotTo(HaveOccurred())

		err = kafka.Stop()
		Expect(err).NotTo(HaveOccurred())
	})

	It("multiples can be started and stopped", func() {
		k1 := &runner.Kafka{
			ZooKeeperConnect:         net.JoinHostPort(zookeeper.Name, "2181"),
			BrokerID:                 1,
			MinInsyncReplicas:        2,
			DefaultReplicationFactor: 3,
			NetworkName:              network.Name,
		}
		err := k1.Start()
		Expect(err).NotTo(HaveOccurred())

		k2 := &runner.Kafka{
			ZooKeeperConnect:         net.JoinHostPort(zookeeper.Name, "2181"),
			BrokerID:                 2,
			MinInsyncReplicas:        2,
			DefaultReplicationFactor: 3,
			NetworkName:              network.Name,
		}
		err = k2.Start()
		Expect(err).NotTo(HaveOccurred())

		k3 := &runner.Kafka{
			ZooKeeperConnect:         net.JoinHostPort(zookeeper.Name, "2181"),
			BrokerID:                 3,
			MinInsyncReplicas:        2,
			DefaultReplicationFactor: 3,
			NetworkName:              network.Name,
		}
		err = k3.Start()
		Expect(err).NotTo(HaveOccurred())

		k4 := &runner.Kafka{
			ZooKeeperConnect:         net.JoinHostPort(zookeeper.Name, "2181"),
			BrokerID:                 4,
			MinInsyncReplicas:        2,
			DefaultReplicationFactor: 3,
			NetworkName:              network.Name,
		}
		err = k4.Start()
		Expect(err).NotTo(HaveOccurred())

		Expect(k1.Stop()).To(Succeed())
		Expect(k2.Stop()).To(Succeed())
		Expect(k3.Stop()).To(Succeed())
		Expect(k4.Stop()).To(Succeed())

		Eventually(ContainerExists(k1.Client, k1.Name)).Should(BeFalse())
		Eventually(ContainerExists(k2.Client, k2.Name)).Should(BeFalse())
		Eventually(ContainerExists(k3.Client, k3.Name)).Should(BeFalse())
		Eventually(ContainerExists(k4.Client, k4.Name)).Should(BeFalse())
	})

	Context("when the container has already been stopped", func() {
		It("returns an error", func() {
			err := kafka.Start()
			Expect(err).NotTo(HaveOccurred())
			containerID := kafka.ContainerID

			err = kafka.Stop()
			Expect(err).NotTo(HaveOccurred())
			err = kafka.Stop()
			Expect(err).To(MatchError(fmt.Sprintf("container %s already stopped", containerID)))
		})
	})
})
