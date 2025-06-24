/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabhttp

import (
	"crypto/tls"
	"crypto/x509"
	"os"
	"time"

	"github.com/hyperledger/fabric/internal/pkg/comm"
)

type TLS struct {
	Enabled            bool
	CertFile           string
	KeyFile            string
	ClientCertRequired bool
	ClientCACertFiles  []string
	TimeShift          time.Duration
}

func (t TLS) Config() (*tls.Config, error) {
	var tlsConfig *tls.Config

	if t.Enabled {
		cert, err := tls.LoadX509KeyPair(t.CertFile, t.KeyFile)
		if err != nil {
			return nil, err
		}
		caCertPool := x509.NewCertPool()
		for _, caPath := range t.ClientCACertFiles {
			caPem, err := os.ReadFile(caPath)
			if err != nil {
				return nil, err
			}
			caCertPool.AppendCertsFromPEM(caPem)
		}
		tlsConfig = &tls.Config{
			MinVersion:   tls.VersionTLS12,
			Certificates: []tls.Certificate{cert},
			CipherSuites: comm.DefaultTLSCipherSuites,
			ClientCAs:    caCertPool,
		}
		if t.TimeShift > 0 {
			tlsConfig.Time = func() time.Time {
				return time.Now().Add((-1) * t.TimeShift)
			}
		}

		if t.ClientCertRequired {
			tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		} else {
			tlsConfig.ClientAuth = tls.VerifyClientCertIfGiven
		}
	}

	return tlsConfig, nil
}
