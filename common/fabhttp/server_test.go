/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabhttp_test

import (
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"syscall"

	"github.com/hyperledger/fabric/common/fabhttp"
	"github.com/hyperledger/fabric/core/operations/fakes"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/tedsuo/ifrit"
)

var _ = Describe("Server", func() {
	const AdditionalTestApiPath = "/some-additional-test-api"

	var (
		fakeLogger *fakes.Logger
		tempDir    string

		client       *http.Client
		unauthClient *http.Client
		options      fabhttp.Options
		server       *fabhttp.Server
	)

	BeforeEach(func() {
		var err error
		tempDir, err = ioutil.TempDir("", "fabhttp-test")
		Expect(err).NotTo(HaveOccurred())

		generateCertificates(tempDir)
		client = newHTTPClient(tempDir, true)
		unauthClient = newHTTPClient(tempDir, false)

		fakeLogger = &fakes.Logger{}
		options = fabhttp.Options{
			Logger:        fakeLogger,
			ListenAddress: "127.0.0.1:0",
			TLS: fabhttp.TLS{
				Enabled:            true,
				CertFile:           filepath.Join(tempDir, "server-cert.pem"),
				KeyFile:            filepath.Join(tempDir, "server-key.pem"),
				ClientCertRequired: false,
				ClientCACertFiles:  []string{filepath.Join(tempDir, "client-ca.pem")},
			},
		}

		server = fabhttp.NewServer(options)
	})

	AfterEach(func() {
		os.RemoveAll(tempDir)
		if server != nil {
			server.Stop()
		}
	})

	When("trying to connect with an old TLS version", func() {
		BeforeEach(func() {
			tlsOpts := []func(config *tls.Config){func(config *tls.Config) {
				config.MaxVersion = tls.VersionTLS11
				config.ClientAuth = tls.RequireAndVerifyClientCert
			}}

			client = newHTTPClient(tempDir, true, tlsOpts...)
		})

		It("does not answer clients using an older TLS version than 1.2", func() {
			server.RegisterHandler(AdditionalTestApiPath, &fakes.Handler{Code: http.StatusOK, Text: "secure"}, options.TLS.Enabled)
			err := server.Start()
			Expect(err).NotTo(HaveOccurred())

			addApiURL := fmt.Sprintf("https://%s%s", server.Addr(), AdditionalTestApiPath)
			_, err = client.Get(addApiURL)
			Expect(err.Error()).To(ContainSubstring("tls: no supported versions satisfy MinVersion and MaxVersion"))
		})
	})

	It("does not host a secure endpoint for additional APIs by default", func() {
		err := server.Start()
		Expect(err).NotTo(HaveOccurred())

		addApiURL := fmt.Sprintf("https://%s%s", server.Addr(), AdditionalTestApiPath)
		resp, err := client.Get(addApiURL)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusNotFound)) // service is not handled by default, i.e. in peer
		resp.Body.Close()

		resp, err = unauthClient.Get(addApiURL)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
	})

	It("hosts a secure endpoint for additional APIs when added", func() {
		server.RegisterHandler(AdditionalTestApiPath, &fakes.Handler{Code: http.StatusOK, Text: "secure"}, options.TLS.Enabled)
		err := server.Start()
		Expect(err).NotTo(HaveOccurred())

		addApiURL := fmt.Sprintf("https://%s%s", server.Addr(), AdditionalTestApiPath)
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
			server = fabhttp.NewServer(options)
		})

		It("does not host an insecure endpoint for additional APIs by default", func() {
			err := server.Start()
			Expect(err).NotTo(HaveOccurred())

			addApiURL := fmt.Sprintf("http://%s%s", server.Addr(), AdditionalTestApiPath)
			resp, err := client.Get(addApiURL)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusNotFound)) // service is not handled by default, i.e. in peer
			resp.Body.Close()
		})

		It("hosts an insecure endpoint for additional APIs when added", func() {
			server.RegisterHandler(AdditionalTestApiPath, &fakes.Handler{Code: http.StatusOK, Text: "insecure"}, options.TLS.Enabled)
			err := server.Start()
			Expect(err).NotTo(HaveOccurred())

			addApiURL := fmt.Sprintf("http://%s%s", server.Addr(), AdditionalTestApiPath)
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
			server = fabhttp.NewServer(options)
		})

		It("requires a client cert to connect", func() {
			err := server.Start()
			Expect(err).NotTo(HaveOccurred())

			_, err = unauthClient.Get(fmt.Sprintf("https://%s/healthz", server.Addr()))
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
			server = fabhttp.NewServer(options)
		})

		AfterEach(func() {
			listener.Close()
		})

		It("returns an error", func() {
			err := server.Start()
			Expect(err).To(MatchError(ContainSubstring("bind: address already in use")))
		})
	})

	Context("when a bad TLS configuration is provided", func() {
		BeforeEach(func() {
			options.TLS.CertFile = "cert-file-does-not-exist"
			server = fabhttp.NewServer(options)
		})

		It("returns an error", func() {
			err := server.Start()
			Expect(err).To(MatchError("open cert-file-does-not-exist: no such file or directory"))
		})
	})

	It("proxies Log to the provided logger", func() {
		err := server.Log("key", "value")
		Expect(err).NotTo(HaveOccurred())

		Expect(fakeLogger.WarnCallCount()).To(Equal(1))
		Expect(fakeLogger.WarnArgsForCall(0)).To(Equal([]interface{}{"key", "value"}))
	})

	Context("when a logger is not provided", func() {
		BeforeEach(func() {
			options.Logger = nil
			server = fabhttp.NewServer(options)
		})

		It("does not panic when logging", func() {
			Expect(func() { server.Log("key", "value") }).NotTo(Panic())
		})

		It("returns nil from Log", func() {
			err := server.Log("key", "value")
			Expect(err).NotTo(HaveOccurred())
		})
	})

	It("supports ifrit", func() {
		process := ifrit.Invoke(server)
		Eventually(process.Ready()).Should(BeClosed())

		process.Signal(syscall.SIGTERM)
		Eventually(process.Wait()).Should(Receive(BeNil()))
	})

	Context("when start fails and ifrit is used", func() {
		BeforeEach(func() {
			options.TLS.CertFile = "non-existent-file"
			server = fabhttp.NewServer(options)
		})

		It("does not close the ready chan", func() {
			process := ifrit.Invoke(server)
			Consistently(process.Ready()).ShouldNot(BeClosed())
			Eventually(process.Wait()).Should(Receive(MatchError("open non-existent-file: no such file or directory")))
		})
	})
})
