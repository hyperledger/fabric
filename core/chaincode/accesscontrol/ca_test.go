/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package accesscontrol

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"math/rand"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func createTLSService(t *testing.T, clientCAcert []byte) *grpc.Server {
	ca, err := NewCA()
	assert.NoError(t, err)
	keyPair, err := ca.newCertKeyPair()
	cert, err := tls.X509KeyPair(keyPair.certBytes, keyPair.keyBytes)
	assert.NoError(t, err)
	tlsConf := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    x509.NewCertPool(),
	}
	tlsConf.ClientCAs.AppendCertsFromPEM(clientCAcert)
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

	srv := createTLSService(t, ca.CertBytes())
	l, err := net.Listen("tcp", fmt.Sprintf("%s:%d", "", randomPort))
	assert.NoError(t, err)
	go srv.Serve(l)
	defer srv.Stop()
	defer l.Close()

	probeTLS := func(kp *certKeyPair) error {
		keyBytes, err := base64.StdEncoding.DecodeString(kp.privKeyString())
		assert.NoError(t, err)
		certBytes, err := base64.StdEncoding.DecodeString(kp.pubKeyString())
		assert.NoError(t, err)
		cert, err := tls.X509KeyPair(certBytes, keyBytes)
		tlsCfg := &tls.Config{
			InsecureSkipVerify: true,
			Certificates:       []tls.Certificate{cert},
		}
		tlsOpts := grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg))
		ctx := context.Background()
		ctx, _ = context.WithTimeout(ctx, time.Second)
		conn, err := grpc.DialContext(ctx, fmt.Sprintf("localhost:%d", randomPort), tlsOpts, grpc.WithBlock())
		if err != nil {
			return err
		}
		conn.Close()
		return nil
	}

	// Good path - use a cert key pair generated from the CA
	// that the TLS server started with
	kp, err := ca.newCertKeyPair()
	assert.NoError(t, err)
	err = probeTLS(kp)
	assert.NoError(t, err)

	// Bad path - use a cert key pair generated from a foreign CA
	foreignCA, _ := NewCA()
	kp, err = foreignCA.newCertKeyPair()
	assert.NoError(t, err)
	err = probeTLS(kp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tls: bad certificate")
}
