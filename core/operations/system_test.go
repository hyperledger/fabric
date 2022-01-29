/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package operations_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/hyperledger/fabric-lib-go/healthz"
	"github.com/hyperledger/fabric/common/fabhttp"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/common/metrics/prometheus"
	"github.com/hyperledger/fabric/common/metrics/statsd"
	"github.com/hyperledger/fabric/core/operations"
	"github.com/hyperledger/fabric/core/operations/fakes"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/tedsuo/ifrit"
)

var _ = Describe("System", func() {
	const AdditionalTestApiPath = "/some-additional-test-api"

	var (
		fakeLogger *fakes.Logger
		tempDir    string

		client       *http.Client
		unauthClient *http.Client
		options      operations.Options
		system       *operations.System
	)

	BeforeEach(func() {
		var err error
		tempDir, err = ioutil.TempDir("", "opssys")
		Expect(err).NotTo(HaveOccurred())

		generateCertificates(tempDir)
		client = newHTTPClient(tempDir, true)
		unauthClient = newHTTPClient(tempDir, false)

		fakeLogger = &fakes.Logger{}
		options = operations.Options{
			Options: fabhttp.Options{
				Logger:        fakeLogger,
				ListenAddress: "127.0.0.1:0",
				TLS: fabhttp.TLS{
					Enabled:            true,
					CertFile:           filepath.Join(tempDir, "server-cert.pem"),
					KeyFile:            filepath.Join(tempDir, "server-key.pem"),
					ClientCertRequired: false,
					ClientCACertFiles:  []string{filepath.Join(tempDir, "client-ca.pem")},
				},
			},
			Metrics: operations.MetricsOptions{
				Provider: "disabled",
			},
			Version: "test-version",
		}

		system = operations.NewSystem(options)
	})

	AfterEach(func() {
		os.RemoveAll(tempDir)
		if system != nil {
			system.Stop()
		}
	})

	It("hosts an unsecured endpoint for the version information", func() {
		err := system.Start()
		Expect(err).NotTo(HaveOccurred())

		versionURL := fmt.Sprintf("https://%s/version", system.Addr())
		resp, err := client.Get(versionURL)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		resp.Body.Close()
	})

	It("hosts a secure endpoint for logging", func() {
		err := system.Start()
		Expect(err).NotTo(HaveOccurred())

		logspecURL := fmt.Sprintf("https://%s/logspec", system.Addr())
		resp, err := client.Get(logspecURL)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		resp.Body.Close()

		resp, err = unauthClient.Get(logspecURL)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
	})

	It("does not host a secure endpoint for additional APIs by default", func() {
		err := system.Start()
		Expect(err).NotTo(HaveOccurred())

		addApiURL := fmt.Sprintf("https://%s%s", system.Addr(), AdditionalTestApiPath)
		resp, err := client.Get(addApiURL)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusNotFound)) // service is not handled by default, i.e. in peer
		resp.Body.Close()

		resp, err = unauthClient.Get(addApiURL)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
	})

	It("hosts a secure endpoint for additional APIs when added", func() {
		system.RegisterHandler(AdditionalTestApiPath, &fakes.Handler{Code: http.StatusOK, Text: "secure"}, options.TLS.Enabled)
		err := system.Start()
		Expect(err).NotTo(HaveOccurred())

		addApiURL := fmt.Sprintf("https://%s%s", system.Addr(), AdditionalTestApiPath)
		resp, err := client.Get(addApiURL)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(resp.Header.Get("Content-Type")).To(Equal("text/plain; charset=utf-8"))
		buff, err := ioutil.ReadAll(resp.Body)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(buff)).To(Equal("secure"))
		resp.Body.Close()

		resp, err = unauthClient.Get(addApiURL)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
	})

	Context("when TLS is disabled", func() {
		BeforeEach(func() {
			options.TLS.Enabled = false
			system = operations.NewSystem(options)
		})

		It("hosts an insecure endpoint for logging", func() {
			err := system.Start()
			Expect(err).NotTo(HaveOccurred())

			resp, err := client.Get(fmt.Sprintf("http://%s/logspec", system.Addr()))
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			resp.Body.Close()
		})

		It("does not host an insecure endpoint for additional APIs by default", func() {
			err := system.Start()
			Expect(err).NotTo(HaveOccurred())

			addApiURL := fmt.Sprintf("http://%s%s", system.Addr(), AdditionalTestApiPath)
			resp, err := client.Get(addApiURL)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusNotFound)) // service is not handled by default, i.e. in peer
			resp.Body.Close()
		})

		It("hosts an insecure endpoint for additional APIs when added", func() {
			system.RegisterHandler(AdditionalTestApiPath, &fakes.Handler{Code: http.StatusOK, Text: "insecure"}, options.TLS.Enabled)
			err := system.Start()
			Expect(err).NotTo(HaveOccurred())

			addApiURL := fmt.Sprintf("http://%s%s", system.Addr(), AdditionalTestApiPath)
			resp, err := client.Get(addApiURL)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(resp.Header.Get("Content-Type")).To(Equal("text/plain; charset=utf-8"))
			buff, err := ioutil.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(buff)).To(Equal("insecure"))
			resp.Body.Close()
		})
	})

	Context("when ClientCertRequired is true", func() {
		BeforeEach(func() {
			options.TLS.ClientCertRequired = true
			system = operations.NewSystem(options)
		})

		It("requires a client cert to connect", func() {
			err := system.Start()
			Expect(err).NotTo(HaveOccurred())

			_, err = unauthClient.Get(fmt.Sprintf("https://%s/healthz", system.Addr()))
			Expect(err).To(MatchError(ContainSubstring("remote error: tls: bad certificate")))
		})
	})

	Context("when listen fails", func() {
		var listener net.Listener

		BeforeEach(func() {
			var err error
			listener, err = net.Listen("tcp", "127.0.0.1:0")
			Expect(err).NotTo(HaveOccurred())

			options.ListenAddress = listener.Addr().String()
			system = operations.NewSystem(options)
		})

		AfterEach(func() {
			listener.Close()
		})

		It("returns an error", func() {
			err := system.Start()
			Expect(err).To(MatchError(ContainSubstring("bind: address already in use")))
		})
	})

	Context("when a bad TLS configuration is provided", func() {
		BeforeEach(func() {
			options.TLS.CertFile = "cert-file-does-not-exist"
			system = operations.NewSystem(options)
		})

		It("returns an error", func() {
			err := system.Start()
			Expect(err).To(MatchError("open cert-file-does-not-exist: no such file or directory"))
		})
	})

	It("proxies Log to the provided logger", func() {
		err := system.Log("key", "value")
		Expect(err).NotTo(HaveOccurred())

		Expect(fakeLogger.WarnCallCount()).To(Equal(1))
		Expect(fakeLogger.WarnArgsForCall(0)).To(Equal([]interface{}{"key", "value"}))
	})

	Context("when a logger is not provided", func() {
		BeforeEach(func() {
			options.Logger = nil
			system = operations.NewSystem(options)
		})

		It("does not panic when logging", func() {
			Expect(func() { system.Log("key", "value") }).NotTo(Panic())
		})

		It("returns nil from Log", func() {
			err := system.Log("key", "value")
			Expect(err).NotTo(HaveOccurred())
		})
	})

	It("hosts a health check endpoint", func() {
		err := system.Start()
		Expect(err).NotTo(HaveOccurred())

		healthy := &fakes.HealthChecker{}
		unhealthy := &fakes.HealthChecker{}
		unhealthy.HealthCheckReturns(errors.New("Unfortunately, I am not feeling well."))

		system.RegisterChecker("healthy", healthy)
		system.RegisterChecker("unhealthy", unhealthy)

		resp, err := client.Get(fmt.Sprintf("https://%s/healthz", system.Addr()))
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusServiceUnavailable))
		body, err := ioutil.ReadAll(resp.Body)
		Expect(err).NotTo(HaveOccurred())
		resp.Body.Close()

		var healthStatus healthz.HealthStatus
		err = json.Unmarshal(body, &healthStatus)
		Expect(err).NotTo(HaveOccurred())
		Expect(healthStatus.Status).To(Equal(healthz.StatusUnavailable))
		Expect(healthStatus.FailedChecks).To(ConsistOf(healthz.FailedCheck{
			Component: "unhealthy",
			Reason:    "Unfortunately, I am not feeling well.",
		}))
	})

	Context("when the metrics provider is disabled", func() {
		BeforeEach(func() {
			options.Metrics = operations.MetricsOptions{
				Provider: "disabled",
			}
			system = operations.NewSystem(options)
			Expect(system).NotTo(BeNil())
		})

		It("sets up a disabled provider", func() {
			Expect(system.Provider).To(Equal(&disabled.Provider{}))
		})
	})

	Context("when the metrics provider is prometheus", func() {
		BeforeEach(func() {
			options.Metrics = operations.MetricsOptions{
				Provider: "prometheus",
			}
			system = operations.NewSystem(options)
			Expect(system).NotTo(BeNil())
		})

		It("sets up prometheus as a provider", func() {
			Expect(system.Provider).To(Equal(&prometheus.Provider{}))
		})

		It("hosts a secure endpoint for metrics", func() {
			err := system.Start()
			Expect(err).NotTo(HaveOccurred())

			metricsURL := fmt.Sprintf("https://%s/metrics", system.Addr())
			resp, err := client.Get(metricsURL)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			body, err := ioutil.ReadAll(resp.Body)
			resp.Body.Close()
			Expect(err).NotTo(HaveOccurred())
			Expect(body).To(ContainSubstring("# TYPE go_gc_duration_seconds summary"))

			resp, err = unauthClient.Get(metricsURL)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
		})

		It("records the fabric version", func() {
			err := system.Start()
			Expect(err).NotTo(HaveOccurred())

			metricsURL := fmt.Sprintf("https://%s/metrics", system.Addr())
			resp, err := client.Get(metricsURL)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			body, err := ioutil.ReadAll(resp.Body)
			resp.Body.Close()
			Expect(err).NotTo(HaveOccurred())
			Expect(string(body)).To(ContainSubstring("# TYPE fabric_version gauge"))
			Expect(string(body)).To(ContainSubstring(`fabric_version{version="test-version"}`))
		})
	})

	Context("when the metrics provider is statsd", func() {
		var listener net.Listener

		BeforeEach(func() {
			var err error
			listener, err = net.Listen("tcp", "127.0.0.1:0")
			Expect(err).NotTo(HaveOccurred())

			options.Metrics = operations.MetricsOptions{
				Provider: "statsd",
				Statsd: &operations.Statsd{
					Network:       "tcp",
					Address:       listener.Addr().String(),
					WriteInterval: 100 * time.Millisecond,
					Prefix:        "prefix",
				},
			}
			system = operations.NewSystem(options)
			Expect(system).NotTo(BeNil())
		})

		recordStats := func(w io.Writer) {
			defer GinkgoRecover()

			// handle the dial check
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close()

			// handle the payload
			conn, err = listener.Accept()
			Expect(err).NotTo(HaveOccurred())
			defer conn.Close()

			conn.SetReadDeadline(time.Now().Add(time.Minute))
			_, err = io.Copy(w, conn)
			if err != nil && err != io.EOF {
				Expect(err).NotTo(HaveOccurred())
			}
		}

		AfterEach(func() {
			listener.Close()
		})

		It("sets up statsd as a provider", func() {
			provider, ok := system.Provider.(*statsd.Provider)
			Expect(ok).To(BeTrue())
			Expect(provider.Statsd).NotTo(BeNil())
		})

		It("emits statsd metrics", func() {
			statsBuffer := gbytes.NewBuffer()
			go recordStats(statsBuffer)

			err := system.Start()
			Expect(err).NotTo(HaveOccurred())
			Eventually(statsBuffer).Should(gbytes.Say(`\Qprefix.go.mem.gc_last_epoch_nanotime:\E`))
		})

		It("emits the fabric version statsd metric", func() {
			statsBuffer := gbytes.NewBuffer()
			go recordStats(statsBuffer)

			err := system.Start()
			Expect(err).NotTo(HaveOccurred())
			Eventually(statsBuffer).Should(gbytes.Say(`\Qprefix.fabric_version.test-version:1.000000|g\E`))
		})

		Context("when checking the network and address fails", func() {
			BeforeEach(func() {
				options.Metrics.Statsd.Network = "bob-the-network"
				system = operations.NewSystem(options)
			})

			It("returns an error", func() {
				err := system.Start()
				Expect(err).To(MatchError(ContainSubstring("bob-the-network")))
			})
		})
	})

	Context("when the metrics provider is unknown", func() {
		BeforeEach(func() {
			options.Metrics.Provider = "something-unknown"
			system = operations.NewSystem(options)
		})

		It("sets up a disabled provider", func() {
			Expect(system.Provider).To(Equal(&disabled.Provider{}))
		})

		It("logs the issue", func() {
			Expect(fakeLogger.WarnfCallCount()).To(Equal(1))
			msg, args := fakeLogger.WarnfArgsForCall(0)
			Expect(msg).To(Equal("Unknown provider type: %s; metrics disabled"))
			Expect(args).To(Equal([]interface{}{"something-unknown"}))
		})
	})

	It("supports ifrit", func() {
		process := ifrit.Invoke(system)
		Eventually(process.Ready()).Should(BeClosed())

		process.Signal(syscall.SIGTERM)
		Eventually(process.Wait()).Should(Receive(BeNil()))
	})

	Context("when start fails and ifrit is used", func() {
		BeforeEach(func() {
			options.TLS.CertFile = "non-existent-file"
			system = operations.NewSystem(options)
		})

		It("does not close the ready chan", func() {
			process := ifrit.Invoke(system)
			Consistently(process.Ready()).ShouldNot(BeClosed())
			Eventually(process.Wait()).Should(Receive(MatchError("open non-existent-file: no such file or directory")))
		})
	})
})
