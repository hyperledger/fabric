/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"path/filepath"
	"testing"
	"time"

	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

func TestServerKeepaliveOptions(t *testing.T) {
	t.Parallel()

	kap := keepalive.ServerParameters{
		Time:    DefaultKeepaliveOptions.ServerInterval,
		Timeout: DefaultKeepaliveOptions.ServerTimeout,
	}
	kep := keepalive.EnforcementPolicy{
		MinTime:             DefaultKeepaliveOptions.ServerMinInterval,
		PermitWithoutStream: true,
	}
	expectedOpts := []grpc.ServerOption{
		grpc.KeepaliveParams(kap),
		grpc.KeepaliveEnforcementPolicy(kep),
	}
	opts := DefaultKeepaliveOptions.ServerKeepaliveOptions()

	// Unable to test equality of options since the option methods return
	// functions and each instance is a different func.
	// Unable to test the equality of applying the options to the server
	// implementation because the server embeds channels.
	// Fallback to a sanity check.
	require.Len(t, opts, len(expectedOpts))
	for i := range opts {
		require.IsType(t, expectedOpts[i], opts[i])
	}
}

func TestClientKeepaliveOptions(t *testing.T) {
	t.Parallel()

	kap := keepalive.ClientParameters{
		Time:                DefaultKeepaliveOptions.ClientInterval,
		Timeout:             DefaultKeepaliveOptions.ClientTimeout,
		PermitWithoutStream: true,
	}
	expectedOpts := []grpc.DialOption{grpc.WithKeepaliveParams(kap)}
	opts := DefaultKeepaliveOptions.ClientKeepaliveOptions()

	// Unable to test equality of options since the option methods return
	// functions and each instance is a different func.
	// Fallback to a sanity check.
	require.Len(t, opts, len(expectedOpts))
	for i := range opts {
		require.IsType(t, expectedOpts[i], opts[i])
	}
}

func TestClientConfigClone(t *testing.T) {
	origin := ClientConfig{
		KaOpts: KeepaliveOptions{
			ClientInterval: time.Second,
		},
		SecOpts: SecureOptions{
			Key: []byte{1, 2, 3},
		},
		DialTimeout:  time.Second,
		AsyncConnect: true,
	}

	clone := origin

	// Same content, different inner fields references.
	require.Equal(t, origin, clone)

	// We change the contents of the fields and ensure it doesn't
	// propagate across instances.
	origin.AsyncConnect = false
	origin.KaOpts.ServerInterval = time.Second
	origin.KaOpts.ClientInterval = time.Hour
	origin.SecOpts.Certificate = []byte{1, 2, 3}
	origin.SecOpts.Key = []byte{5, 4, 6}
	origin.DialTimeout = time.Second * 2

	clone.SecOpts.UseTLS = true
	clone.KaOpts.ServerMinInterval = time.Hour

	expectedOriginState := ClientConfig{
		KaOpts: KeepaliveOptions{
			ClientInterval: time.Hour,
			ServerInterval: time.Second,
		},
		SecOpts: SecureOptions{
			Key:         []byte{5, 4, 6},
			Certificate: []byte{1, 2, 3},
		},
		DialTimeout: time.Second * 2,
	}

	expectedCloneState := ClientConfig{
		KaOpts: KeepaliveOptions{
			ClientInterval:    time.Second,
			ServerMinInterval: time.Hour,
		},
		SecOpts: SecureOptions{
			Key:    []byte{1, 2, 3},
			UseTLS: true,
		},
		DialTimeout:  time.Second,
		AsyncConnect: true,
	}

	require.Equal(t, expectedOriginState, origin)
	require.Equal(t, expectedCloneState, clone)
}

func TestSecureOptionsTLSConfig(t *testing.T) {
	ca1, err := tlsgen.NewCA()
	require.NoError(t, err, "failed to create CA1")
	ca2, err := tlsgen.NewCA()
	require.NoError(t, err, "failed to create CA2")
	ckp, err := ca1.NewClientCertKeyPair()
	require.NoError(t, err, "failed to create client key pair")
	clientCert, err := tls.X509KeyPair(ckp.Cert, ckp.Key)
	require.NoError(t, err, "failed to create client certificate")

	newCertPool := func(cas ...tlsgen.CA) *x509.CertPool {
		cp := x509.NewCertPool()
		for _, ca := range cas {
			ok := cp.AppendCertsFromPEM(ca.CertBytes())
			require.True(t, ok, "failed to add cert to pool")
		}
		return cp
	}

	tests := []struct {
		desc        string
		so          SecureOptions
		tc          *tls.Config
		expectedErr string
	}{
		{desc: "TLSDisabled"},
		{desc: "TLSEnabled", so: SecureOptions{UseTLS: true}, tc: &tls.Config{MinVersion: tls.VersionTLS12}},
		{
			desc: "ServerNameOverride",
			so:   SecureOptions{UseTLS: true, ServerNameOverride: "bob"},
			tc:   &tls.Config{MinVersion: tls.VersionTLS12, ServerName: "bob"},
		},
		{
			desc: "WithServerRootCAs",
			so:   SecureOptions{UseTLS: true, ServerRootCAs: [][]byte{ca1.CertBytes(), ca2.CertBytes()}},
			tc:   &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: newCertPool(ca1, ca2)},
		},
		{
			desc: "BadServerRootCertificate",
			so: SecureOptions{
				UseTLS: true,
				ServerRootCAs: [][]byte{
					[]byte("-----BEGIN CERTIFICATE-----\nYm9ndXM=\n-----END CERTIFICATE-----"),
				},
			},
			expectedErr: "error adding root certificate",
		},
		{
			desc: "WithRequiredClientKeyPair",
			so:   SecureOptions{UseTLS: true, RequireClientCert: true, Key: ckp.Key, Certificate: ckp.Cert},
			tc:   &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{clientCert}},
		},
		{
			desc:        "MissingClientKey",
			so:          SecureOptions{UseTLS: true, RequireClientCert: true, Certificate: ckp.Cert},
			expectedErr: "both Key and Certificate are required when using mutual TLS",
		},
		{
			desc:        "MissingClientCert",
			so:          SecureOptions{UseTLS: true, RequireClientCert: true, Key: ckp.Key},
			expectedErr: "both Key and Certificate are required when using mutual TLS",
		},
		{
			desc: "WithTimeShift",
			so:   SecureOptions{UseTLS: true, TimeShift: 2 * time.Hour},
			tc:   &tls.Config{MinVersion: tls.VersionTLS12},
		},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			tc, err := tt.so.TLSConfig()
			if tt.expectedErr != "" {
				require.ErrorContainsf(t, err, tt.expectedErr, "got %v, want %s", err, tt.expectedErr)
				return
			}
			require.NoError(t, err)

			if len(tt.so.ServerRootCAs) != 0 {
				require.NotNil(t, tc.RootCAs)
				require.Len(t, tc.RootCAs.Subjects(), len(tt.so.ServerRootCAs))
				for _, subj := range tt.tc.RootCAs.Subjects() {
					require.Contains(t, tc.RootCAs.Subjects(), subj, "missing subject %x", subj)
				}
				tt.tc.RootCAs, tc.RootCAs = nil, nil
			}

			if tt.so.TimeShift != 0 {
				require.NotNil(t, tc.Time)
				require.WithinDuration(t, time.Now().Add(-1*tt.so.TimeShift), tc.Time(), 10*time.Second)
				tc.Time = nil
			}

			require.Equal(t, tt.tc, tc)
		})
	}
}

func TestClientConfigDialOptions_GoodConfig(t *testing.T) {
	testCerts := LoadTestCerts(t)

	config := ClientConfig{}
	opts, err := config.DialOptions()
	require.NoError(t, err)
	require.NotEmpty(t, opts)

	secOpts := SecureOptions{
		UseTLS:            true,
		ServerRootCAs:     [][]byte{testCerts.CAPEM},
		RequireClientCert: false,
	}
	config.SecOpts = secOpts
	opts, err = config.DialOptions()
	require.NoError(t, err)
	require.NotEmpty(t, opts)

	secOpts = SecureOptions{
		Certificate:       testCerts.CertPEM,
		Key:               testCerts.KeyPEM,
		UseTLS:            true,
		ServerRootCAs:     [][]byte{testCerts.CAPEM},
		RequireClientCert: true,
	}
	clientCert, err := secOpts.ClientCertificate()
	require.NoError(t, err)
	require.Equal(t, testCerts.ClientCert, clientCert)
	config.SecOpts = secOpts
	opts, err = config.DialOptions()
	require.NoError(t, err)
	require.NotEmpty(t, opts)
}

func TestClientConfigDialOptions_BadConfig(t *testing.T) {
	testCerts := LoadTestCerts(t)

	// bad root cert
	config := ClientConfig{
		SecOpts: SecureOptions{
			UseTLS:        true,
			ServerRootCAs: [][]byte{[]byte(badPEM)},
		},
	}
	_, err := config.DialOptions()
	require.ErrorContains(t, err, "error adding root certificate")

	// missing key
	config.SecOpts = SecureOptions{
		Certificate:       []byte("cert"),
		UseTLS:            true,
		RequireClientCert: true,
	}
	_, err = config.DialOptions()
	require.ErrorContains(t, err, "both Key and Certificate are required when using mutual TLS")

	// missing cert
	config.SecOpts = SecureOptions{
		Key:               []byte("key"),
		UseTLS:            true,
		RequireClientCert: true,
	}
	_, err = config.DialOptions()
	require.ErrorContains(t, err, "both Key and Certificate are required when using mutual TLS")

	// bad key
	config.SecOpts = SecureOptions{
		Certificate:       testCerts.CertPEM,
		Key:               []byte(badPEM),
		UseTLS:            true,
		RequireClientCert: true,
	}
	_, err = config.DialOptions()
	require.ErrorContains(t, err, "failed to load client certificate")

	// bad cert
	config.SecOpts = SecureOptions{
		Certificate:       []byte(badPEM),
		Key:               testCerts.KeyPEM,
		UseTLS:            true,
		RequireClientCert: true,
	}
	_, err = config.DialOptions()
	require.ErrorContains(t, err, "failed to load client certificate")
}

type TestCerts struct {
	CAPEM      []byte
	CertPEM    []byte
	KeyPEM     []byte
	ClientCert tls.Certificate
	ServerCert tls.Certificate
}

func LoadTestCerts(t *testing.T) TestCerts {
	t.Helper()

	var certs TestCerts
	var err error
	certs.CAPEM, err = ioutil.ReadFile(filepath.Join("testdata", "certs", "Org1-cert.pem"))
	if err != nil {
		t.Fatalf("unexpected error reading root cert for test: %v", err)
	}
	certs.CertPEM, err = ioutil.ReadFile(filepath.Join("testdata", "certs", "Org1-client1-cert.pem"))
	if err != nil {
		t.Fatalf("unexpected error reading cert for test: %v", err)
	}
	certs.KeyPEM, err = ioutil.ReadFile(filepath.Join("testdata", "certs", "Org1-client1-key.pem"))
	if err != nil {
		t.Fatalf("unexpected error reading key for test: %v", err)
	}
	certs.ClientCert, err = tls.X509KeyPair(certs.CertPEM, certs.KeyPEM)
	if err != nil {
		t.Fatalf("unexpected error loading certificate for test: %v", err)
	}
	certs.ServerCert, err = tls.LoadX509KeyPair(
		filepath.Join("testdata", "certs", "Org1-server1-cert.pem"),
		filepath.Join("testdata", "certs", "Org1-server1-key.pem"),
	)
	require.NoError(t, err)

	return certs
}
