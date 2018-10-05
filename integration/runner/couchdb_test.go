/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package runner_test

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"syscall"
	"time"

	"github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric/integration/runner"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/ghttp"
	"github.com/tedsuo/ifrit"
)

var _ = Describe("CouchDB Runner", func() {
	var (
		dockerServer *ghttp.Server
		couchServer  *ghttp.Server

		createStatus    int
		createResponse  *docker.Container
		startStatus     int
		startResponse   string
		inspectStatus   int
		inspectResponse *docker.Container
		logsStatus      int
		logsResponse    string
		stopStatus      int
		stopResponse    string
		waitStatus      int
		waitResponse    string

		waitChan chan struct{}
		client   *docker.Client

		errBuffer *gbytes.Buffer
		outBuffer *gbytes.Buffer
		couchDB   *runner.CouchDB

		process ifrit.Process
	)

	BeforeEach(func() {
		couchServer = ghttp.NewServer()
		couchServer.Writer = GinkgoWriter
		couchServer.AppendHandlers(
			ghttp.RespondWith(http.StatusServiceUnavailable, "service unavailable"),
			ghttp.RespondWith(http.StatusServiceUnavailable, "service unavailable"),
			ghttp.RespondWith(http.StatusOK, "ready"),
		)

		couchAddr := couchServer.Addr()
		couchHost, couchPort, err := net.SplitHostPort(couchAddr)
		Expect(err).NotTo(HaveOccurred())

		waitChan = make(chan struct{}, 1)
		dockerServer = ghttp.NewServer()
		dockerServer.Writer = GinkgoWriter

		dockerServer.RouteToHandler("GET", "/version", ghttp.CombineHandlers(
			ghttp.VerifyRequest("GET", "/version"),
			ghttp.RespondWithJSONEncoded(http.StatusOK, map[string]string{
				"ApiVersion": "1.29",
			}),
		))

		createStatus = http.StatusCreated
		createResponse = &docker.Container{
			ID: "container-id",
		}
		dockerServer.RouteToHandler("POST", "/containers/create", ghttp.CombineHandlers(
			ghttp.VerifyRequest("POST", "/containers/create", "name=container-name"),
			ghttp.RespondWithJSONEncodedPtr(&createStatus, &createResponse),
		))

		startStatus = http.StatusNoContent
		dockerServer.RouteToHandler("POST", "/containers/container-id/start", ghttp.CombineHandlers(
			ghttp.VerifyRequest("POST", "/containers/container-id/start", ""),
			ghttp.RespondWithPtr(&startStatus, &startResponse),
		))

		inspectStatus = http.StatusOK
		inspectResponse = &docker.Container{
			ID: "container-id",
			NetworkSettings: &docker.NetworkSettings{
				Ports: map[docker.Port][]docker.PortBinding{
					docker.Port("5984/tcp"): {{
						HostIP:   couchHost,
						HostPort: couchPort,
					}},
				},
			},
		}
		dockerServer.RouteToHandler("GET", "/containers/container-id/json", ghttp.CombineHandlers(
			ghttp.VerifyRequest("GET", "/containers/container-id/json", ""),
			ghttp.RespondWithJSONEncodedPtr(&inspectStatus, &inspectResponse),
		))

		logsStatus = http.StatusOK
		dockerServer.RouteToHandler("GET", "/containers/container-id/logs", ghttp.CombineHandlers(
			ghttp.VerifyRequest("GET", "/containers/container-id/logs", "follow=1&stderr=1&stdout=1&tail=all"),
			ghttp.RespondWithPtr(&logsStatus, &logsResponse),
		))

		stopStatus = http.StatusNoContent
		dockerServer.RouteToHandler("POST", "/containers/container-id/stop", ghttp.CombineHandlers(
			ghttp.VerifyRequest("POST", "/containers/container-id/stop", "t=0"),
			ghttp.RespondWithPtr(&stopStatus, &stopResponse),
			func(_ http.ResponseWriter, _ *http.Request) {
				defer GinkgoRecover()
				Eventually(waitChan).Should(BeSent(struct{}{}))
			},
		))

		waitStatus = http.StatusNoContent
		waitResponse = `{ StatusCode: 0 }`
		dockerServer.RouteToHandler("POST", "/containers/container-id/wait", ghttp.CombineHandlers(
			ghttp.RespondWithPtr(&waitStatus, &waitResponse),
			func(w http.ResponseWriter, r *http.Request) { <-waitChan },
		))

		client, err = docker.NewClient(dockerServer.URL())
		Expect(err).NotTo(HaveOccurred())

		errBuffer = gbytes.NewBuffer()
		outBuffer = gbytes.NewBuffer()
		couchDB = &runner.CouchDB{
			Name:         "container-name",
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
		close(waitChan)
		dockerServer.Close()
		couchServer.Close()
	})

	It("starts and stops a docker container with the specified image", func() {
		containerName := runner.DefaultNamer()

		By("using a real docker daemon")
		client, err := docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())
		couchDB.Client = nil
		couchDB.StartTimeout = 0
		couchDB.Name = containerName

		By("starting couch DB")
		process = ifrit.Invoke(couchDB)
		Eventually(process.Ready(), runner.DefaultStartTimeout).Should(BeClosed())
		Consistently(process.Wait()).ShouldNot(Receive())

		By("inspecting the container by name")
		container, err := client.InspectContainer(containerName)
		Expect(err).NotTo(HaveOccurred())
		Expect(container.Name).To(Equal("/" + containerName))
		Expect(container.State.Status).To(Equal("running"))
		Expect(container.Config).NotTo(BeNil())
		Expect(container.Config.Image).To(Equal("hyperledger/fabric-couchdb:latest"))
		Expect(container.ID).To(Equal(couchDB.ContainerID()))
		portBindings := container.NetworkSettings.Ports[docker.Port("5984/tcp")]
		Expect(portBindings).To(HaveLen(1))
		Expect(couchDB.HostAddress()).To(Equal(net.JoinHostPort(portBindings[0].HostIP, portBindings[0].HostPort)))
		Expect(couchDB.ContainerAddress()).To(Equal(net.JoinHostPort(container.NetworkSettings.IPAddress, "5984")))

		By("getting the container logs")
		Eventually(errBuffer).Should(gbytes.Say(`WARNING: CouchDB is running in Admin Party mode.`))

		By("accessing the couch DB server")
		address := couchDB.Address()
		Expect(address).NotTo(BeEmpty())
		resp, err := http.Get(fmt.Sprintf("http://%s/", address))
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		By("terminating the container")
		process.Signal(syscall.SIGTERM)
		Eventually(process.Wait(), time.Minute).Should(Receive())
		process = nil

		Eventually(ContainerExists(client, containerName)).Should(BeFalse())
	})

	It("can be started and stopped with ifrit", func() {
		process = ifrit.Invoke(couchDB)
		Eventually(process.Ready()).Should(BeClosed())
		Expect(dockerServer.ReceivedRequests()).To(HaveLen(6))

		process.Signal(syscall.SIGTERM)
		Eventually(process.Wait()).Should(Receive())
		Expect(dockerServer.ReceivedRequests()).To(HaveLen(7))
		process = nil
	})

	It("can be started and stopped without ifrit", func() {
		err := couchDB.Start()
		Expect(err).NotTo(HaveOccurred())
		Expect(dockerServer.ReceivedRequests()).To(HaveLen(6))

		err = couchDB.Stop()
		Expect(err).NotTo(HaveOccurred())
		Expect(dockerServer.ReceivedRequests()).To(HaveLen(7))
	})

	Context("when a host port is provided", func() {
		BeforeEach(func() {
			couchDB.HostPort = 33333
			var data = struct {
				*docker.Config
				HostConfig *docker.HostConfig
			}{
				Config: &docker.Config{
					Image: "hyperledger/fabric-couchdb:latest",
				},
				HostConfig: &docker.HostConfig{
					AutoRemove: true,
					PortBindings: map[docker.Port][]docker.PortBinding{
						docker.Port("5984/tcp"): {{
							HostIP:   "127.0.0.1",
							HostPort: "33333",
						}},
					},
				},
			}
			dockerServer.RouteToHandler("POST", "/containers/create", ghttp.CombineHandlers(
				ghttp.VerifyRequest("POST", "/containers/create", "name=container-name"),
				ghttp.VerifyJSONRepresenting(data),
				ghttp.RespondWithJSONEncodedPtr(&createStatus, &createResponse),
			))
		})

		It("exposes couch on the specified port", func() {
			err := couchDB.Start()
			Expect(err).NotTo(HaveOccurred())
			err = couchDB.Stop()
			Expect(err).NotTo(HaveOccurred())

			Expect(dockerServer.ReceivedRequests()).To(HaveLen(7))
		})
	})

	Context("when the container endpoint becomes available before the host", func() {
		BeforeEach(func() {
			couchHost, couchPort, err := net.SplitHostPort(couchServer.Addr())
			Expect(err).NotTo(HaveOccurred())

			couchDB.ContainerPort = docker.Port(couchPort + "/tcp")
			inspectResponse.NetworkSettings.IPAddress = couchHost
			inspectResponse.NetworkSettings.Ports[couchDB.ContainerPort] = []docker.PortBinding{{
				HostIP:   "127.0.0.1",
				HostPort: "0",
			}}
		})

		It("returns the container address", func() {
			err := couchDB.Start()
			Expect(err).NotTo(HaveOccurred())
			err = couchDB.Stop()
			Expect(err).NotTo(HaveOccurred())

			Expect(couchDB.Address()).To(Equal(couchServer.Addr()))
			Expect(couchDB.ContainerAddress()).To(Equal(couchServer.Addr()))
		})
	})

	Context("when the host endpoint becomes available before the container", func() {
		BeforeEach(func() {
			inspectResponse.NetworkSettings.IPAddress = "127.0.0.255"
		})

		It("returns the host address", func() {
			err := couchDB.Start()
			Expect(err).NotTo(HaveOccurred())
			err = couchDB.Stop()
			Expect(err).NotTo(HaveOccurred())

			Expect(couchDB.Address()).To(Equal(couchServer.Addr()))
			Expect(couchDB.HostAddress()).To(Equal(couchServer.Addr()))
		})
	})

	Context("when creating the container fails", func() {
		BeforeEach(func() {
			createStatus = http.StatusServiceUnavailable
			createResponse = nil
		})

		It("returns an error", func() {
			err := couchDB.Start()
			Expect(err).To(HaveOccurred())
		})
	})

	Context("when starting the container fails", func() {
		BeforeEach(func() {
			startStatus = http.StatusConflict
		})

		It("returns an error", func() {
			err := couchDB.Start()
			Expect(err).To(HaveOccurred())
		})
	})

	Context("when inspecting the container fails", func() {
		BeforeEach(func() {
			inspectStatus = http.StatusNotFound
		})

		It("returns an error", func() {
			err := couchDB.Start()
			Expect(err).To(HaveOccurred())
		})
	})

	Context("when streaming logs fails", func() {
		BeforeEach(func() {
			logsStatus = http.StatusConflict
		})

		It("it records the error", func() {
			err := couchDB.Start()
			Expect(err).NotTo(HaveOccurred())
			Eventually(errBuffer).Should(gbytes.Say(`log stream ended with error: API error`))
		})
	})

	Context("when the log streams are both nil", func() {
		BeforeEach(func() {
			couchDB.ErrorStream = nil
			couchDB.OutputStream = nil

			logsStatus = http.StatusConflict
		})

		It("doesn't request logs from docker", func() {
			err := couchDB.Start()
			Expect(err).NotTo(HaveOccurred())

			err = couchDB.Stop()
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("when the container has already been stopped", func() {
		It("returns an error", func() {
			err := couchDB.Start()
			Expect(err).NotTo(HaveOccurred())

			err = couchDB.Stop()
			Expect(err).NotTo(HaveOccurred())
			err = couchDB.Stop()
			Expect(err).To(MatchError("container container-id already stopped"))
		})
	})

	Context("when stopping the container fails", func() {
		BeforeEach(func() {
			stopStatus = http.StatusGone
		})

		It("returns an error", func() {
			err := couchDB.Start()
			Expect(err).NotTo(HaveOccurred())

			err = couchDB.Stop()
			Expect(err).To(Equal(&docker.Error{Status: http.StatusGone}))
		})
	})

	Context("when stop fails after the process is signaled", func() {
		BeforeEach(func() {
			stopStatus = http.StatusGone
		})

		It("returns an error", func() {
			process = ifrit.Invoke(couchDB)
			Eventually(process.Ready()).Should(BeClosed())

			process.Signal(syscall.SIGTERM)
			Eventually(process.Wait()).Should(Receive(Equal(&docker.Error{Status: http.StatusGone})))
		})
	})

	Context("when startup times out", func() {
		BeforeEach(func() {
			couchDB.StartTimeout = 50 * time.Millisecond
			couchServer.RouteToHandler("GET", "/", ghttp.RespondWith(http.StatusServiceUnavailable, "service unavailable"))
		})

		It("returns an error", func() {
			err := couchDB.Start()
			Expect(err).To(MatchError(ContainSubstring("database in container container-id did not start")))
		})
	})

	Context("when the container exits", func() {
		BeforeEach(func() {
			Eventually(waitChan).Should(BeSent(struct{}{}))
		})

		It("returns an error", func() {
			err := couchDB.Start()
			Expect(err).To(HaveOccurred())
		})
	})

	Context("when a name isn't provided", func() {
		BeforeEach(func() {
			dockerServer.RouteToHandler("POST", "/containers/create", ghttp.RespondWith(http.StatusServiceUnavailable, "go away"))
		})

		It("generates a unique name", func() {
			db1 := &runner.CouchDB{Client: client}
			err := db1.Start()
			Expect(err).To(HaveOccurred())
			Expect(db1.Name).ShouldNot(BeEmpty())
			Expect(db1.Name).To(HaveLen(26))

			db2 := &runner.CouchDB{Client: client}
			err = db2.Start()
			Expect(err).To(HaveOccurred())
			Expect(db2.Name).ShouldNot(BeEmpty())
			Expect(db2.Name).To(HaveLen(26))

			Expect(db1.Name).NotTo(Equal(db2.Name))
		})
	})
})
