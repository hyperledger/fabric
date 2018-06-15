/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package runner_test

import (
	"io"
	"io/ioutil"
	"net"
	"os"
	"syscall"
	"time"

	"github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric/integration/runner"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/tedsuo/ifrit"
)

var _ = Describe("ZooKeeper Runner", func() {
	var (
		errBuffer *gbytes.Buffer
		outBuffer *gbytes.Buffer
		zookeeper *runner.ZooKeeper

		process ifrit.Process
	)

	BeforeEach(func() {
		client, err := docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())

		errBuffer = gbytes.NewBuffer()
		outBuffer = gbytes.NewBuffer()
		zookeeper = &runner.ZooKeeper{
			Name:         "zookeeper0",
			StartTimeout: time.Second,
			ErrorStream:  io.MultiWriter(errBuffer, GinkgoWriter),
			OutputStream: io.MultiWriter(outBuffer, GinkgoWriter),
			Client:       client,
		}

		process = nil
	})

	AfterEach(func() {
		if process != nil {
			process.Signal(syscall.SIGTERM)
		}
		tempDir, _ := ioutil.TempDir("", "zk-runner")
		os.RemoveAll(tempDir)
	})

	It("starts and stops a docker container with the specified image", func() {
		By("using a real docker daemon")
		zookeeper.Client = nil
		zookeeper.StartTimeout = 5 * time.Second

		By("starting ZooKeeper")
		process = ifrit.Invoke(zookeeper)
		Eventually(process.Ready(), runner.DefaultStartTimeout).Should(BeClosed())
		Consistently(process.Wait(), 5*time.Second).ShouldNot(Receive())

		By("inspecting the container by name")
		container, err := zookeeper.Client.InspectContainer("zookeeper0")
		Expect(err).NotTo(HaveOccurred())

		Expect(container.Name).To(Equal("/zookeeper0"))
		Expect(container.State.Status).To(Equal("running"))
		Expect(container.Config).NotTo(BeNil())
		Expect(container.Config.Image).To(Equal("hyperledger/fabric-zookeeper:latest"))
		Expect(container.ID).To(Equal(zookeeper.ContainerID()))

		Expect(zookeeper.ContainerAddress()).To(Equal(net.JoinHostPort(container.NetworkSettings.IPAddress, "2181")))

		By("getting the container logs")
		Eventually(errBuffer, 5*time.Second).Should(gbytes.Say(`Using config: /conf/zoo.cfg`))
		Eventually(outBuffer, 5*time.Second).Should(gbytes.Say(`binding to port 0.0.0.0/0.0.0.0:2181`))

		By("terminating the container")
		err = zookeeper.Stop()
		Expect(err).NotTo(HaveOccurred())

		Eventually(ContainerExists(zookeeper.Client, "zookeeper0")).Should(BeFalse())
	})

	It("starts and stops multiple zookeepers", func() {
		client, err := docker.NewClientFromEnv()
		zk1 := &runner.ZooKeeper{
			Name:         "zookeeper1",
			ZooMyID:      1,
			ZooServers:   "server.1=zookeeper1:2888:3888 server.2=zookeeper2:2888:3888 server.3=zookeeper3:2888:3888",
			StartTimeout: 5 * time.Second,
			Client:       client,
		}
		err = zk1.Start()
		Expect(err).NotTo(HaveOccurred())

		zk2 := &runner.ZooKeeper{
			Name:         "zookeeper2",
			ZooMyID:      2,
			ZooServers:   "server.1=zookeeper1:2888:3888 server.2=zookeeper2:2888:3888 server.3=zookeeper3:2888:3888",
			StartTimeout: 5 * time.Second,
			Client:       client,
		}
		err = zk2.Start()
		Expect(err).NotTo(HaveOccurred())

		zk3 := &runner.ZooKeeper{
			Name:         "zookeeper3",
			ZooMyID:      3,
			ZooServers:   "server.1=zookeeper1:2888:3888 server.2=zookeeper2:2888:3888 server.3=zookeeper3:2888:3888",
			StartTimeout: 5 * time.Second,
			Client:       client,
		}
		err = zk3.Start()
		Expect(err).NotTo(HaveOccurred())

		container, err := zk1.Client.InspectContainer("zookeeper1")
		Expect(container.Config.Env).To(ContainElement(ContainSubstring("ZOO_MY_ID=1")))
		Expect(container.Config.Env).To(ContainElement(ContainSubstring("ZOO_SERVERS=server.1=zookeeper1:2888:3888 server.2=zookeeper2:2888:3888 server.3=zookeeper3:2888:3888")))
		container, err = zk2.Client.InspectContainer("zookeeper2")
		Expect(container.Config.Env).To(ContainElement(ContainSubstring("ZOO_MY_ID=2")))
		Expect(container.Config.Env).To(ContainElement(ContainSubstring("ZOO_SERVERS=server.1=zookeeper1:2888:3888 server.2=zookeeper2:2888:3888 server.3=zookeeper3:2888:3888")))
		container, err = zk3.Client.InspectContainer("zookeeper3")
		Expect(container.Config.Env).To(ContainElement(ContainSubstring("ZOO_MY_ID=3")))
		Expect(container.Config.Env).To(ContainElement(ContainSubstring("ZOO_SERVERS=server.1=zookeeper1:2888:3888 server.2=zookeeper2:2888:3888 server.3=zookeeper3:2888:3888")))

		Expect(zk3.Stop()).To(Succeed())
		Expect(zk2.Stop()).To(Succeed())
		Expect(zk1.Stop()).To(Succeed())

		Eventually(ContainerExists(zk1.Client, "zookeeper1")).Should(BeFalse())
		Eventually(ContainerExists(zk2.Client, "zookeeper2")).Should(BeFalse())
		Eventually(ContainerExists(zk3.Client, "zookeeper3")).Should(BeFalse())
	})
})
