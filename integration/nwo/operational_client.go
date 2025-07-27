/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package nwo

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/gomega"
)

func OrdererOperationalClients(n *Network, o *Orderer) (authClient, unauthClient *http.Client) {
	return OrdererOperationalClientsTimeShift(n, o, 0)
}

func OrdererOperationalClientsTimeShift(n *Network, o *Orderer, timeShift time.Duration) (authClient, unauthClient *http.Client) {
	return operationalClients(n, n.OrdererLocalTLSDir(o), timeShift)
}

func PeerOperationalClients(n *Network, p *Peer) (authClient, unauthClient *http.Client) {
	return operationalClients(n, n.PeerLocalTLSDir(p), 0)
}

func operationalClients(n *Network, tlsDir string, timeShift time.Duration) (authClient, unauthClient *http.Client) {
	fingerprint := "http::" + tlsDir
	if d := n.throttleDuration(fingerprint); d > 0 {
		time.Sleep(d)
	}

	clientCert, err := tls.LoadX509KeyPair(
		filepath.Join(tlsDir, "server.crt"),
		filepath.Join(tlsDir, "server.key"),
	)
	Expect(err).NotTo(HaveOccurred())

	clientCertPool := x509.NewCertPool()
	caCert, err := os.ReadFile(filepath.Join(tlsDir, "ca.crt"))
	Expect(err).NotTo(HaveOccurred())
	clientCertPool.AppendCertsFromPEM(caCert)

	authenticatedTlsConfig := &tls.Config{
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      clientCertPool,
	}
	unauthenticatedTlsConfig := &tls.Config{RootCAs: clientCertPool}
	if timeShift > 0 {
		authenticatedTlsConfig.Time = func() time.Time {
			return time.Now().Add((-1) * timeShift)
		}
		unauthenticatedTlsConfig.Time = func() time.Time {
			return time.Now().Add((-1) * timeShift)
		}
	}

	authenticatedClient := &http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost: -1,
			TLSClientConfig:     authenticatedTlsConfig,
		},
	}
	unauthenticatedClient := &http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost: -1,
			TLSClientConfig:     unauthenticatedTlsConfig,
		},
	}

	return authenticatedClient, unauthenticatedClient
}
