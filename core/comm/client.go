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
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
)

type GRPCClient interface {
	// Certificate returns the tls.Certificate used to make TLS connections
	// when client certificates are required by the server
	Certificate() tls.Certificate
	// TLSEnabled is a flag indicating whether to use TLS for client
	// connections
	TLSEnabled() bool
	// MutualTLSRequired is a flag indicating whether the client
	// must send a certificate when making TLS connections
	MutualTLSRequired() bool
	// SetMaxRecvMsgSize sets the maximum message size the client can receive
	SetMaxRecvMsgSize(size int)
	// SetMaxSendMsgSize sets the maximum message size the client can send
	SetMaxSendMsgSize(size int)
	// SetServerRootCAs sets the list of authorities used to verify server
	// certificates based on a list of PEM-encoded X509 certificate authorities
	SetServerRootCAs(clientRoots [][]byte) error
	// NewConnection returns a grpc.ClientConn for the target address and
	// overrides the server name used to verify the hostname on the
	// certificate returned by a server when using TLS
	NewConnection(address string, serverNameOverride string) (*grpc.ClientConn, error)
}

type grpcClient struct {
	// Set of PEM-encoded X509 certificate authorities used to populate
	// the tlsConfig.RootCAs indexed by subject
	serverRootCAs map[string]*x509.Certificate
	// TLS configuration used by the grpc.ClientConn
	tlsConfig *tls.Config
	// Flag indicating whether TLS is enabled
	tlsEnabled bool
	// Flag indicating whether a client certificate is required
	mutualTLSRequired bool
	// Options for setting up new connections
	dialOpts []grpc.DialOption
	// Duration for which to block while established a new connection
	timeout time.Duration
	// Maximum message size the client can receive
	maxRecvMsgSize int
	// Maximum message size the client can send
	maxSendMsgSize int
}

// NewGRPCClient creates a new implementation of GRPCClient given an address
// and client configuration
func NewGRPCClient(config ClientConfig) (GRPCClient, error) {
	client := &grpcClient{
		tlsEnabled:        false,
		mutualTLSRequired: false}

	// parse secure options
	err := client.parseSecureOptions(config.SecOpts)
	if err != nil {
		return client, err
	}

	// keepalive options
	var kap keepalive.ClientParameters
	if config.KaOpts != nil {
		kap = keepalive.ClientParameters{
			Time:    config.KaOpts.ClientInterval,
			Timeout: config.KaOpts.ClientTimeout}
	} else {
		// use defaults
		kap = keepalive.ClientParameters{
			Time:    keepaliveOptions.ClientInterval,
			Timeout: keepaliveOptions.ClientTimeout}
	}
	kap.PermitWithoutStream = true
	// set keepalive and blocking
	client.dialOpts = append(client.dialOpts, grpc.WithKeepaliveParams(kap),
		grpc.WithBlock())
	client.timeout = config.Timeout
	// set send/recv message size to package defaults
	client.maxRecvMsgSize = maxRecvMsgSize
	client.maxSendMsgSize = maxSendMsgSize

	return client, nil
}

func (client *grpcClient) parseSecureOptions(opts *SecureOptions) error {

	if opts == nil || !opts.UseTLS {
		return nil
	}
	client.tlsEnabled = true
	client.tlsConfig = &tls.Config{
		MinVersion: tls.VersionTLS12} // TLS 1.2 only
	if len(opts.ServerRootCAs) > 0 {
		client.tlsConfig.RootCAs = x509.NewCertPool()
		for _, certBytes := range opts.ServerRootCAs {
			err := AddPemToCertPool(certBytes, client.tlsConfig.RootCAs)
			if err != nil {
				commLogger.Debugf("error adding root certificate: %v", err)
				return errors.WithMessage(err,
					"error adding root certificate")
			}
		}
	}
	if opts.RequireClientCert {
		client.mutualTLSRequired = true
		// make sure we have both Key and Certificate
		if opts.Key != nil &&
			opts.Certificate != nil {
			cert, err := tls.X509KeyPair(opts.Certificate,
				opts.Key)
			if err != nil {
				return errors.WithMessage(err, "failed to "+
					"load client certificate")
			}
			client.tlsConfig.Certificates = append(
				client.tlsConfig.Certificates, cert)
		} else {
			return errors.New("both Key and Certificate " +
				"are required when using mutual TLS")
		}
	}
	return nil
}

// Certificate returns the tls.Certificate used to make TLS connections
// when client certificates are required by the server
func (client *grpcClient) Certificate() tls.Certificate {
	cert := tls.Certificate{}
	if client.tlsConfig != nil && len(client.tlsConfig.Certificates) > 0 {
		cert = client.tlsConfig.Certificates[0]
	}
	return cert
}

// TLSEnabled is a flag indicating whether to use TLS for client
// connections
func (client *grpcClient) TLSEnabled() bool {
	return client.tlsEnabled
}

// MutualTLSRequired is a flag indicating whether the client
// must send a certificate when making TLS connections
func (client *grpcClient) MutualTLSRequired() bool {
	return client.mutualTLSRequired
}

// SetMaxRecvMsgSize sets the maximum message size the client can receive
func (client *grpcClient) SetMaxRecvMsgSize(size int) {
	client.maxRecvMsgSize = size
}

// SetMaxSendMsgSize sets the maximum message size the client can send
func (client *grpcClient) SetMaxSendMsgSize(size int) {
	client.maxSendMsgSize = size
}

// SetServerRootCAs sets the list of authorities used to verify server
// certificates based on a list of PEM-encoded X509 certificate authorities
func (client *grpcClient) SetServerRootCAs(serverRoots [][]byte) error {

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

// NewConnection returns a grpc.ClientConn for the target address and
// overrides the server name used to verify the hostname on the
// certificate returned by a server when using TLS
func (client *grpcClient) NewConnection(address string, serverNameOverride string) (
	*grpc.ClientConn, error) {

	var dialOpts []grpc.DialOption
	dialOpts = append(dialOpts, client.dialOpts...)

	// set transport credentials and max send/recv message sizes
	// immediately before creating a connection in order to allow
	// SetServerRootCAs / SetMaxRecvMsgSize / SetMaxSendMsgSize
	//  to take effect on a per connection basis
	if client.tlsConfig != nil {
		client.tlsConfig.ServerName = serverNameOverride
		dialOpts = append(dialOpts,
			grpc.WithTransportCredentials(
				credentials.NewTLS(client.tlsConfig)))
	} else {
		dialOpts = append(dialOpts, grpc.WithInsecure())
	}
	dialOpts = append(dialOpts, grpc.WithDefaultCallOptions(
		grpc.MaxCallRecvMsgSize(client.maxRecvMsgSize),
		grpc.MaxCallSendMsgSize(client.maxSendMsgSize)))

	ctx, cancel := context.WithTimeout(context.Background(), client.timeout)
	defer cancel()
	conn, err := grpc.DialContext(ctx, address, dialOpts...)
	if err != nil {
		return nil, errors.WithMessage(errors.WithStack(err),
			"failed to create new connection")
	}
	return conn, nil
}
