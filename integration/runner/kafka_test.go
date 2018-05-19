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
		zookeeper *runner.Zookeeper

		process ifrit.Process
	)

	BeforeEach(func() {
		outBuffer = gbytes.NewBuffer()
		process = nil

		var err error
		client, err = docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())

		// Create a network
		networkName := runner.UniqueName()
		network, err = client.CreateNetwork(
			docker.CreateNetworkOptions{
				Name:   networkName,
				Driver: "bridge",
			},
		)

		// Start a zookeeper
		zookeeper = &runner.Zookeeper{
			Name:        "zookeeper0",
			ZooMyID:     1,
			NetworkID:   network.ID,
			NetworkName: network.Name,
		}
		err = zookeeper.Start()
		Expect(err).NotTo(HaveOccurred())

		kafka = &runner.Kafka{
			Name:             "kafka1",
			ErrorStream:      GinkgoWriter,
			OutputStream:     io.MultiWriter(outBuffer, GinkgoWriter),
			ZookeeperConnect: "zookeeper0:2181",
			BrokerID:         1,
			NetworkID:        network.ID,
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
		By("starting kafka broker")
		process = ifrit.Invoke(kafka)
		Eventually(process.Ready(), runner.DefaultStartTimeout).Should(BeClosed())
		Consistently(process.Wait()).ShouldNot(Receive())

		By("inspecting the container by name")
		container, err := client.InspectContainer("kafka1")
		Expect(err).NotTo(HaveOccurred())
		Expect(container.Name).To(Equal("/kafka1"))
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
		Eventually(process.Wait()).Should(Receive(BeNil()))
		process = nil

		_, err = client.InspectContainer("kafka1")
		Expect(err).To(MatchError("No such container: kafka1"))
	})

	It("can be started and stopped without ifrit", func() {
		err := kafka.Start()
		Expect(err).NotTo(HaveOccurred())

		err = kafka.Stop()
		Expect(err).NotTo(HaveOccurred())
	})

	It("multiples can be started and stopped", func() {
		k1 := &runner.Kafka{
			Name:                     "kafka1",
			ZookeeperConnect:         "zookeeper0:2181",
			BrokerID:                 1,
			MinInsyncReplicas:        2,
			DefaultReplicationFactor: 3,
			NetworkID:                network.ID,
			NetworkName:              network.Name,
		}
		err := k1.Start()
		Expect(err).NotTo(HaveOccurred())

		k2 := &runner.Kafka{
			Name:                     "kafka2",
			ZookeeperConnect:         "zookeeper0:2181",
			BrokerID:                 2,
			MinInsyncReplicas:        2,
			DefaultReplicationFactor: 3,
			NetworkID:                network.ID,
			NetworkName:              network.Name,
		}
		err = k2.Start()
		Expect(err).NotTo(HaveOccurred())

		k3 := &runner.Kafka{
			Name:                     "kafka3",
			ZookeeperConnect:         "zookeeper0:2181",
			BrokerID:                 3,
			MinInsyncReplicas:        2,
			DefaultReplicationFactor: 3,
			NetworkID:                network.ID,
			NetworkName:              network.Name,
		}
		err = k3.Start()
		Expect(err).NotTo(HaveOccurred())

		k4 := &runner.Kafka{
			Name:                     "kafka4",
			ZookeeperConnect:         "zookeeper0:2181",
			BrokerID:                 4,
			MinInsyncReplicas:        2,
			DefaultReplicationFactor: 3,
			NetworkID:                network.ID,
			NetworkName:              network.Name,
		}
		err = k4.Start()
		Expect(err).NotTo(HaveOccurred())

		err = k1.Stop()
		Expect(err).NotTo(HaveOccurred())
		err = k2.Stop()
		Expect(err).NotTo(HaveOccurred())
		err = k3.Stop()
		Expect(err).NotTo(HaveOccurred())
		err = k4.Stop()
		Expect(err).NotTo(HaveOccurred())
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

	Context("when a name isn't provided", func() {
		It("generates a unique name", func() {
			k1 := &runner.Kafka{}
			err := k1.Start()
			Expect(err).To(HaveOccurred())
			Expect(k1.Name).ShouldNot(BeEmpty())
			Expect(k1.Name).To(HaveLen(26))

			k2 := &runner.Kafka{}
			err = k2.Start()
			Expect(err).To(HaveOccurred())
			Expect(k2.Name).ShouldNot(BeEmpty())
			Expect(k2.Name).To(HaveLen(26))

			Expect(k1.Name).NotTo(Equal(k2.Name))

			err = k1.Stop()
			Expect(err).To(HaveOccurred())

			err = k2.Stop()
			Expect(err).To(HaveOccurred())
		})
	})
})
