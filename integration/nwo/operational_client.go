/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package nwo

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"net/http"
	"path/filepath"

	. "github.com/onsi/gomega"
)

func OrdererOperationalClients(n *Network, o *Orderer) (authClient, unauthClient *http.Client) {
	return operationalClients(n.OrdererLocalTLSDir(o))
}

func PeerOperationalClients(n *Network, p *Peer) (authClient, unauthClient *http.Client) {
	return operationalClients(n.PeerLocalTLSDir(p))
}

func operationalClients(tlsDir string) (authClient, unauthClient *http.Client) {
	clientCert, err := tls.LoadX509KeyPair(
		filepath.Join(tlsDir, "server.crt"),
		filepath.Join(tlsDir, "server.key"),
	)
	Expect(err).NotTo(HaveOccurred())

	clientCertPool := x509.NewCertPool()
	caCert, err := ioutil.ReadFile(filepath.Join(tlsDir, "ca.crt"))
	Expect(err).NotTo(HaveOccurred())
	clientCertPool.AppendCertsFromPEM(caCert)

	authenticatedClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				Certificates: []tls.Certificate{clientCert},
				RootCAs:      clientCertPool,
			},
		},
	}
	unauthenticatedClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: clientCertPool},
		},
	}

	return authenticatedClient, unauthenticatedClient
}
