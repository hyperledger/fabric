/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tlsgen

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"math/rand"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func createTLSService(t *testing.T, ca CA, host string) *grpc.Server {
	keyPair, err := ca.NewServerCertKeyPair(host)
	cert, err := tls.X509KeyPair(keyPair.Cert, keyPair.Key)
	assert.NoError(t, err)
	tlsConf := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    x509.NewCertPool(),
	}
	tlsConf.ClientCAs.AppendCertsFromPEM(ca.CertBytes())
	return grpc.NewServer(grpc.Creds(credentials.NewTLS(tlsConf)))
}

func TestTLSCA(t *testing.T) {
	// This test checks that the CA can create certificates
	// and corresponding keys that are signed by itself

	rand.Seed(time.Now().UnixNano())
	randomPort := 1234 + rand.Intn(1234) // some random port

	ca, err := NewCA()
	assert.NoError(t, err)
	assert.NotNil(t, ca)

	endpoint := fmt.Sprintf("127.0.0.1:%d", randomPort)
	srv := createTLSService(t, ca, "127.0.0.1")
	l, err := net.Listen("tcp", endpoint)
	assert.NoError(t, err)
	go srv.Serve(l)
	defer srv.Stop()
	defer l.Close()

	probeTLS := func(kp *CertKeyPair) error {
		keyBytes, err := base64.StdEncoding.DecodeString(kp.PrivKeyString())
		assert.NoError(t, err)
		certBytes, err := base64.StdEncoding.DecodeString(kp.PubKeyString())
		assert.NoError(t, err)
		cert, err := tls.X509KeyPair(certBytes, keyBytes)
		tlsCfg := &tls.Config{
			RootCAs:      x509.NewCertPool(),
			Certificates: []tls.Certificate{cert},
		}
		tlsCfg.RootCAs.AppendCertsFromPEM(ca.CertBytes())
		tlsOpts := grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg))
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		conn, err := grpc.DialContext(ctx, fmt.Sprintf("127.0.0.1:%d", randomPort), tlsOpts, grpc.WithBlock())
		if err != nil {
			return err
		}
		conn.Close()
		return nil
	}

	// Good path - use a cert key pair generated from the CA
	// that the TLS server started with
	kp, err := ca.NewClientCertKeyPair()
	assert.NoError(t, err)
	err = probeTLS(kp)
	assert.NoError(t, err)

	// Bad path - use a cert key pair generated from a foreign CA
	foreignCA, _ := NewCA()
	kp, err = foreignCA.NewClientCertKeyPair()
	assert.NoError(t, err)
	err = probeTLS(kp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context deadline exceeded")
}
