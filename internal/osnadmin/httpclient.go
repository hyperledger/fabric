/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package osnadmin

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"time"
)

func httpClient(caCertPool *x509.CertPool, tlsClientCert tls.Certificate, timeShift time.Duration) *http.Client {
	tlsConfig := &tls.Config{
		RootCAs:      caCertPool,
		Certificates: []tls.Certificate{tlsClientCert},
	}

	if timeShift > 0 {
		tlsConfig.Time = func() time.Time {
			return time.Now().Add((-1) * timeShift)
		}
	}

	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}
}

func httpDo(req *http.Request, caCertPool *x509.CertPool, tlsClientCert tls.Certificate) (*http.Response, error) {
	return httpDoTimeShift(req, caCertPool, tlsClientCert, 0)
}

func httpDoTimeShift(req *http.Request, caCertPool *x509.CertPool, tlsClientCert tls.Certificate, timeShift time.Duration) (*http.Response, error) {
	client := httpClient(caCertPool, tlsClientCert, timeShift)
	return client.Do(req)
}

func httpGet(url string, caCertPool *x509.CertPool, tlsClientCert tls.Certificate) (*http.Response, error) {
	client := httpClient(caCertPool, tlsClientCert, 0)
	return client.Get(url)
}
