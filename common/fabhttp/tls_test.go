/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabhttp_test

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/hyperledger/fabric/common/fabhttp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("TLS", func() {
	var httpTLS fabhttp.TLS
	var tempDir string

	BeforeEach(func() {
		var err error
		tempDir, err = ioutil.TempDir("", "opstls")
		Expect(err).NotTo(HaveOccurred())

		generateCertificates(tempDir)

		httpTLS = fabhttp.TLS{
			Enabled:            true,
			CertFile:           filepath.Join(tempDir, "server-cert.pem"),
			KeyFile:            filepath.Join(tempDir, "server-key.pem"),
			ClientCertRequired: true,
			ClientCACertFiles: []string{
				filepath.Join(tempDir, "client-ca.pem"),
			},
		}
	})

	AfterEach(func() {
		os.RemoveAll(tempDir)
	})

	It("creates a valid TLS configuration", func() {
		cert, err := tls.LoadX509KeyPair(
			filepath.Join(tempDir, "server-cert.pem"),
			filepath.Join(tempDir, "server-key.pem"),
		)
		Expect(err).NotTo(HaveOccurred())

		pemBytes, err := ioutil.ReadFile(filepath.Join(tempDir, "client-ca.pem"))
		Expect(err).NotTo(HaveOccurred())

		clientCAPool := x509.NewCertPool()
		clientCAPool.AppendCertsFromPEM(pemBytes)

		tlsConfig, err := httpTLS.Config()
		Expect(err).NotTo(HaveOccurred())

		// https://go-review.googlesource.com/c/go/+/229917
		Expect(tlsConfig.ClientCAs.Subjects()).To(Equal(clientCAPool.Subjects()))
		tlsConfig.ClientCAs = nil

		Expect(tlsConfig).To(Equal(&tls.Config{
			MinVersion:   tls.VersionTLS12,
			Certificates: []tls.Certificate{cert},
			CipherSuites: []uint16{
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
			},
			ClientAuth: tls.RequireAndVerifyClientCert,
		}))
	})

	Context("when TLS is not enabled", func() {
		BeforeEach(func() {
			httpTLS.Enabled = false
		})

		It("returns a nil config", func() {
			tlsConfig, err := httpTLS.Config()
			Expect(err).NotTo(HaveOccurred())
			Expect(tlsConfig).To(BeNil())
		})
	})

	Context("when a client certificate is not required", func() {
		BeforeEach(func() {
			httpTLS.ClientCertRequired = false
		})

		It("requests a client cert with verification", func() {
			tlsConfig, err := httpTLS.Config()
			Expect(err).NotTo(HaveOccurred())
			Expect(tlsConfig.ClientAuth).To(Equal(tls.VerifyClientCertIfGiven))
		})
	})

	Context("when the server certificate cannot be constructed", func() {
		BeforeEach(func() {
			httpTLS.CertFile = "non-existent-file"
		})

		It("returns an error", func() {
			_, err := httpTLS.Config()
			Expect(err).To(MatchError("open non-existent-file: no such file or directory"))
		})
	})

	Context("the client CA slice is empty", func() {
		BeforeEach(func() {
			httpTLS.ClientCACertFiles = nil
		})

		It("builds a TLS configuration without an empty CA pool", func() {
			tlsConfig, err := httpTLS.Config()
			Expect(err).NotTo(HaveOccurred())
			Expect(tlsConfig.ClientCAs.Subjects()).To(BeEmpty())
		})
	})

	Context("when a client CA cert cannot be read", func() {
		BeforeEach(func() {
			httpTLS.ClientCACertFiles = []string{
				"non-existent-file",
			}
		})

		It("returns an error", func() {
			_, err := httpTLS.Config()
			Expect(err).To(MatchError("open non-existent-file: no such file or directory"))
		})
	})
})
