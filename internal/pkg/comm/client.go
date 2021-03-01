/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"time"

	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

type GRPCClient struct {
	// TLS configuration used by the grpc.ClientConn
	tlsConfig *tls.Config
	// Options for setting up new connections
	dialOpts []grpc.DialOption
	// Duration for which to block while established a new connection
	timeout time.Duration
}

// NewGRPCClient creates a new implementation of GRPCClient given an address
// and client configuration
func NewGRPCClient(config ClientConfig) (*GRPCClient, error) {
	// parse secure options
	tlsConfig, err := config.SecOpts.TLSConfig()
	if err != nil {
		return nil, err
	}

	return &GRPCClient{
		tlsConfig: tlsConfig,
		dialOpts:  config.DialOptions(),
		timeout:   config.DialTimeout,
	}, nil
}

// Certificate returns the tls.Certificate used to make TLS connections
// when client certificates are required by the server
func (client *GRPCClient) Certificate() tls.Certificate {
	cert := tls.Certificate{}
	if client.tlsConfig != nil && len(client.tlsConfig.Certificates) > 0 {
		cert = client.tlsConfig.Certificates[0]
	}
	return cert
}

// TLSEnabled is a flag indicating whether to use TLS for client
// connections
func (client *GRPCClient) TLSEnabled() bool {
	return client.tlsConfig != nil
}

// SetServerRootCAs sets the list of authorities used to verify server
// certificates based on a list of PEM-encoded X509 certificate authorities
func (client *GRPCClient) SetServerRootCAs(serverRoots [][]byte) error {
	// NOTE: if no serverRoots are specified, the current cert pool will be
	// replaced with an empty one
	certPool := x509.NewCertPool()
	for _, root := range serverRoots {
		err := AddPemToCertPool(root, certPool)
		if err != nil {
			return errors.WithMessage(err, "error adding root certificate")
		}
	}
	client.tlsConfig.RootCAs = certPool
	return nil
}

type TLSOption func(tlsConfig *tls.Config)

func ServerNameOverride(name string) TLSOption {
	return func(tlsConfig *tls.Config) {
		tlsConfig.ServerName = name
	}
}

func CertPoolOverride(pool *x509.CertPool) TLSOption {
	return func(tlsConfig *tls.Config) {
		tlsConfig.RootCAs = pool
	}
}

// NewConnection returns a grpc.ClientConn for the target address and
// overrides the server name used to verify the hostname on the
// certificate returned by a server when using TLS
func (client *GRPCClient) NewConnection(address string, tlsOptions ...TLSOption) (*grpc.ClientConn, error) {
	var dialOpts []grpc.DialOption
	dialOpts = append(dialOpts, client.dialOpts...)

	// set transport credentials and max send/recv message sizes
	// immediately before creating a connection in order to allow
	// SetServerRootCAs / SetMaxRecvMsgSize / SetMaxSendMsgSize
	//  to take effect on a per connection basis
	if client.tlsConfig != nil {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(
			&DynamicClientCredentials{
				TLSConfig:  client.tlsConfig,
				TLSOptions: tlsOptions,
			},
		))
	} else {
		dialOpts = append(dialOpts, grpc.WithInsecure())
	}

	ctx, cancel := context.WithTimeout(context.Background(), client.timeout)
	defer cancel()
	conn, err := grpc.DialContext(ctx, address, dialOpts...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create new connection")
	}
	return conn, nil
}
