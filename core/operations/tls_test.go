/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package operations_test

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/hyperledger/fabric/core/operations"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("TLS", func() {
	var opsTLS operations.TLS
	var tempDir string

	BeforeEach(func() {
		var err error
		tempDir, err = ioutil.TempDir("", "opstls")
		Expect(err).NotTo(HaveOccurred())

		generateCertificates(tempDir)

		opsTLS = operations.TLS{
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

		tlsConfig, err := opsTLS.Config()
		Expect(err).NotTo(HaveOccurred())
		Expect(tlsConfig).To(Equal(&tls.Config{
			Certificates: []tls.Certificate{cert},
			CipherSuites: []uint16{
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
			},
			ClientCAs:  clientCAPool,
			ClientAuth: tls.RequireAndVerifyClientCert,
		}))
	})

	Context("when TLS is not enabled", func() {
		BeforeEach(func() {
			opsTLS.Enabled = false
		})

		It("returns a nil config", func() {
			tlsConfig, err := opsTLS.Config()
			Expect(err).NotTo(HaveOccurred())
			Expect(tlsConfig).To(BeNil())
		})
	})

	Context("when a client certificate is not required", func() {
		BeforeEach(func() {
			opsTLS.ClientCertRequired = false
		})

		It("requests a client cert with verification", func() {
			tlsConfig, err := opsTLS.Config()
			Expect(err).NotTo(HaveOccurred())
			Expect(tlsConfig.ClientAuth).To(Equal(tls.VerifyClientCertIfGiven))
		})
	})

	Context("when the server certificate cannot be constructed", func() {
		BeforeEach(func() {
			opsTLS.CertFile = "non-existent-file"
		})

		It("returns an error", func() {
			_, err := opsTLS.Config()
			Expect(err).To(MatchError("open non-existent-file: no such file or directory"))
		})
	})

	Context("the client CA slice is empty", func() {
		BeforeEach(func() {
			opsTLS.ClientCACertFiles = nil
		})

		It("builds a TLS configuration without an empty CA pool", func() {
			tlsConfig, err := opsTLS.Config()
			Expect(err).NotTo(HaveOccurred())
			Expect(tlsConfig.ClientCAs.Subjects()).To(BeEmpty())
		})
	})

	Context("when a client CA cert cannot be read", func() {
		BeforeEach(func() {
			opsTLS.ClientCACertFiles = []string{
				"non-existent-file",
			}
		})

		It("returns an error", func() {
			_, err := opsTLS.Config()
			Expect(err).To(MatchError("open non-existent-file: no such file or directory"))
		})
	})
})
