/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/common/metrics/prometheus"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/spf13/viper"
)

func TestInitializeMetricsDisabled(t *testing.T) {
	defer viper.Reset()

	t.Run("ExplicitlyDisabled", func(t *testing.T) {
		gt := NewGomegaWithT(t)
		viper.Reset()
		viper.Set("metrics.provider", "disabled")
		provider, _, err := initializeMetrics()
		gt.Expect(err).NotTo(HaveOccurred())
		gt.Expect(provider).To(Equal(&disabled.Provider{}))
	})

	t.Run("BadProvider", func(t *testing.T) {
		gt := NewGomegaWithT(t)
		viper.Reset()
		viper.Set("metrics.provider", "not-a-valid-provider")
		provider, _, err := initializeMetrics()
		gt.Expect(err).NotTo(HaveOccurred())
		gt.Expect(provider).To(Equal(&disabled.Provider{}))
	})
}

func receiveDatagrams(sock *net.UDPConn, w io.Writer, done <-chan struct{}, errCh chan<- error) {
	defer sock.Close()

	buf := make([]byte, 1024*1024)
	for {
		select {
		case <-done:
			errCh <- nil
			return

		default:
			n, _, err := sock.ReadFrom(buf)
			if err != nil {
				errCh <- err
				return
			}
			_, err = w.Write(buf[0:n])
			if err != nil {
				errCh <- err
				return
			}
		}
	}
}

func TestInitializeMetricsStatsd(t *testing.T) {
	tests := []struct {
		name        string
		overrides   map[string]interface{}
		expectedErr string
	}{
		{name: "ValidConfig", expectedErr: ""},
		{name: "BadNetwork", overrides: map[string]interface{}{"metrics.statsd.network": "bad"}, expectedErr: "dial bad: unknown network bad"},
		{name: "BadAddress", overrides: map[string]interface{}{"metrics.statsd.address": "0.0.0.0:badport"}, expectedErr: "dial udp:.*udp/badport"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gt := NewGomegaWithT(t)
			defer viper.Reset()

			udpAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
			gt.Expect(err).NotTo(HaveOccurred())
			sock, err := net.ListenUDP("udp", udpAddr)
			gt.Expect(err).NotTo(HaveOccurred())
			err = sock.SetReadBuffer(1024 * 1024)
			gt.Expect(err).NotTo(HaveOccurred())

			done := make(chan struct{})
			errCh := make(chan error, 1)
			datagramBuffer := gbytes.NewBuffer()
			go receiveDatagrams(sock, datagramBuffer, done, errCh)

			viper.Reset()
			viper.Set("metrics.provider", "statsd")
			viper.Set("metrics.statsd.network", "udp")
			viper.Set("metrics.statsd.address", sock.LocalAddr().String())
			viper.Set("metrics.statsd.writeInterval", 100*time.Millisecond)
			viper.Set("metrics.statsd.prefix", "stats_prefix")
			for k, v := range tc.overrides {
				viper.Set(k, v)
			}

			provider, shutdown, err := initializeMetrics()
			if tc.expectedErr == "" {
				gt.Expect(err).NotTo(HaveOccurred())
				gt.Expect(provider).NotTo(BeNil())
				defer shutdown()
				gt.Eventually(datagramBuffer).Should(gbytes.Say("stats_prefix.go.goroutine_count"))
			} else {
				gt.Expect(err).To(HaveOccurred())
				gt.Expect(provider).To(BeNil())
				gt.Expect(shutdown).To(BeNil())
				gt.Expect(err.Error()).To(MatchRegexp(tc.expectedErr))
			}
		})
	}
}

func generateCertificates(t *testing.T, tempDir string) {
	gt := NewGomegaWithT(t)

	serverCA, err := tlsgen.NewCA()
	gt.Expect(err).NotTo(HaveOccurred())
	err = ioutil.WriteFile(filepath.Join(tempDir, "server-ca.pem"), serverCA.CertBytes(), 0640)
	gt.Expect(err).NotTo(HaveOccurred())
	serverKeyPair, err := serverCA.NewServerCertKeyPair("127.0.0.1")
	gt.Expect(err).NotTo(HaveOccurred())
	err = ioutil.WriteFile(filepath.Join(tempDir, "server-cert.pem"), serverKeyPair.Cert, 0640)
	gt.Expect(err).NotTo(HaveOccurred())
	err = ioutil.WriteFile(filepath.Join(tempDir, "server-key.pem"), serverKeyPair.Key, 0640)
	gt.Expect(err).NotTo(HaveOccurred())

	clientCA, err := tlsgen.NewCA()
	gt.Expect(err).NotTo(HaveOccurred())
	err = ioutil.WriteFile(filepath.Join(tempDir, "client-ca.pem"), clientCA.CertBytes(), 0640)
	gt.Expect(err).NotTo(HaveOccurred())
	clientKeyPair, err := clientCA.NewClientCertKeyPair()
	gt.Expect(err).NotTo(HaveOccurred())
	err = ioutil.WriteFile(filepath.Join(tempDir, "client-cert.pem"), clientKeyPair.Cert, 0640)
	gt.Expect(err).NotTo(HaveOccurred())
	err = ioutil.WriteFile(filepath.Join(tempDir, "client-key.pem"), clientKeyPair.Key, 0640)
	gt.Expect(err).NotTo(HaveOccurred())
}

func TestInitializeMetricsPrometheus(t *testing.T) {
	defer viper.Reset()

	gt := NewGomegaWithT(t)
	tempDir, err := ioutil.TempDir("", "metricstls")
	gt.Expect(err).NotTo(HaveOccurred())
	defer os.RemoveAll(tempDir)

	generateCertificates(t, tempDir)
	serverCAPEM, err := ioutil.ReadFile(filepath.Join(tempDir, "server-ca.pem"))
	gt.Expect(err).NotTo(HaveOccurred())

	t.Run("Insecure", func(t *testing.T) {
		gt := NewGomegaWithT(t)
		viper.Reset()
		viper.Set("metrics.provider", "prometheus")
		viper.Set("metrics.prometheus.listenAddress", "127.0.0.1:33333")
		viper.Set("metrics.prometheus.handlerPath", "/metricz")
		provider, shutdown, err := initializeMetrics()
		gt.Expect(err).NotTo(HaveOccurred())
		defer shutdown()
		gt.Expect(provider).To(Equal(&prometheus.Provider{}))

		resp, err := http.Get("http://127.0.0.1:33333/metricz")
		gt.Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()

		gt.Expect(resp.StatusCode).To(Equal(http.StatusOK))
		body, err := ioutil.ReadAll(resp.Body)
		gt.Expect(err).NotTo(HaveOccurred())
		gt.Expect(body).To(ContainSubstring("# TYPE go_goroutines gauge"))
		gt.Expect(body).To(ContainSubstring("go_goroutines"))

	})

	t.Run("Shutdown", func(t *testing.T) {
		gt := NewGomegaWithT(t)
		viper.Reset()
		viper.Set("metrics.provider", "prometheus")
		viper.Set("metrics.prometheus.listenAddress", "127.0.0.1:33333")
		viper.Set("metrics.prometheus.handlerPath", "/metricz")

		_, shutdown, err := initializeMetrics()
		gt.Expect(err).NotTo(HaveOccurred())
		shutdown()

		gt.Eventually(func() error {
			conn, err := net.Dial("tcp", "127.0.0.1:33333")
			if err == nil {
				conn.Close()
			}
			return err
		}).Should(MatchError("dial tcp 127.0.0.1:33333: connect: connection refused"))
	})

	t.Run("BadListenAddress", func(t *testing.T) {
		gt := NewGomegaWithT(t)
		viper.Reset()
		viper.Set("metrics.provider", "prometheus")
		viper.Set("metrics.prometheus.listenAddress", "bad.bad:bad")
		viper.Set("metrics.prometheus.handlerPath", "/metricz")

		_, _, err := initializeMetrics()
		gt.Expect(err).To(HaveOccurred())
		opErr, ok := err.(*net.OpError)
		gt.Expect(ok).To(BeTrue())
		gt.Expect(opErr.Op).To(Equal("listen"))
	})

	t.Run("TLSEnabledWithClientCert", func(t *testing.T) {
		gt := NewGomegaWithT(t)

		viper.Reset()
		viper.Set("metrics.provider", "prometheus")
		viper.Set("metrics.prometheus.listenAddress", "127.0.0.1:33333")
		viper.Set("metrics.prometheus.handlerPath", "/metricz")
		viper.Set("metrics.prometheus.tls.enabled", true)
		viper.Set("metrics.prometheus.tls.cert.file", filepath.Join(tempDir, "server-cert.pem"))
		viper.Set("metrics.prometheus.tls.key.file", filepath.Join(tempDir, "server-key.pem"))
		viper.Set("metrics.prometheus.tls.clientAuthRequired", true)
		viper.Set("metrics.prometheus.tls.clientRootCAs.files", []string{filepath.Join(tempDir, "client-ca.pem")})

		clientCert, err := tls.LoadX509KeyPair(
			filepath.Join(tempDir, "client-cert.pem"),
			filepath.Join(tempDir, "client-key.pem"),
		)
		gt.Expect(err).NotTo(HaveOccurred())
		clientCertPool := x509.NewCertPool()
		clientCertPool.AppendCertsFromPEM(serverCAPEM)
		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					Certificates: []tls.Certificate{clientCert},
					RootCAs:      clientCertPool,
				},
			},
		}

		_, shutdown, err := initializeMetrics()
		gt.Expect(err).NotTo(HaveOccurred())
		defer shutdown()

		resp, err := client.Get("https://127.0.0.1:33333/metricz")
		gt.Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()
		gt.Expect(resp.StatusCode).To(Equal(http.StatusOK))

		// Without client cert
		client = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					RootCAs: clientCertPool,
				},
			},
		}
		_, err = client.Get("https://127.0.0.1:33333/metricz")
		gt.Expect(err).To(MatchError("Get https://127.0.0.1:33333/metricz: remote error: tls: bad certificate"))
	})

	t.Run("TLSEnabledWithoutClientCert", func(t *testing.T) {
		gt := NewGomegaWithT(t)

		viper.Reset()
		viper.Set("metrics.provider", "prometheus")
		viper.Set("metrics.prometheus.listenAddress", "127.0.0.1:33333")
		viper.Set("metrics.prometheus.handlerPath", "/metricz")
		viper.Set("metrics.prometheus.tls.enabled", true)
		viper.Set("metrics.prometheus.tls.cert.file", filepath.Join(tempDir, "server-cert.pem"))
		viper.Set("metrics.prometheus.tls.key.file", filepath.Join(tempDir, "server-key.pem"))
		viper.Set("metrics.prometheus.tls.clientAuthRequired", false)

		clientCertPool := x509.NewCertPool()
		clientCertPool.AppendCertsFromPEM(serverCAPEM)
		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					RootCAs: clientCertPool,
				},
			},
		}

		_, shutdown, err := initializeMetrics()
		gt.Expect(err).NotTo(HaveOccurred())
		defer shutdown()

		resp, err := client.Get("https://127.0.0.1:33333/metricz")
		gt.Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()
		gt.Expect(resp.StatusCode).To(Equal(http.StatusOK))
	})

	t.Run("TLSEnabledErrors", func(t *testing.T) {
		tests := []struct {
			name        string
			setup       func()
			expectedErr string
		}{
			{
				name:        "BadServerCert",
				setup:       func() { viper.Set("metrics.prometheus.tls.cert.file", filepath.Join(tempDir, "missing-cert.pem")) },
				expectedErr: "missing-cert.pem: no such file or directory",
			},
			{
				name:        "BadServerKey",
				setup:       func() { viper.Set("metrics.prometheus.tls.key.file", filepath.Join(tempDir, "missing-key.pem")) },
				expectedErr: "missing-key.pem: no such file or directory",
			},
			{
				name:        "BadClientCA",
				setup:       func() { viper.Set("metrics.prometheus.tls.clientRootCAs.files", filepath.Join(tempDir, "missing.pem")) },
				expectedErr: "missing.pem: no such file or directory",
			},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				gt := NewGomegaWithT(t)
				viper.Reset()
				viper.Set("metrics.provider", "prometheus")
				viper.Set("metrics.prometheus.listenAddress", "127.0.0.1:33333")
				viper.Set("metrics.prometheus.handlerPath", "/metricz")
				viper.Set("metrics.prometheus.tls.enabled", true)
				viper.Set("metrics.prometheus.tls.cert.file", filepath.Join(tempDir, "server-cert.pem"))
				viper.Set("metrics.prometheus.tls.key.file", filepath.Join(tempDir, "server-key.pem"))
				viper.Set("metrics.prometheus.tls.clientAuthRequired", true)
				viper.Set("metrics.prometheus.tls.clientRootCAs.files", []string{filepath.Join(tempDir, "client-ca.pem")})
				tc.setup()

				_, _, err := initializeMetrics()
				gt.Expect(err).To(HaveOccurred())
				gt.Expect(err.Error()).To(MatchRegexp(tc.expectedErr))
			})
		}
	})
}
