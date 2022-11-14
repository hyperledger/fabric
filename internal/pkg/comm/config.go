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

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/metrics"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

// Configuration defaults

// Max send and receive bytes for grpc clients and servers
const (
	DefaultMaxRecvMsgSize = 100 * 1024 * 1024
	DefaultMaxSendMsgSize = 100 * 1024 * 1024
)

var (
	// Default peer keepalive options
	DefaultKeepaliveOptions = KeepaliveOptions{
		ClientInterval:    time.Duration(1) * time.Minute,  // 1 min
		ClientTimeout:     time.Duration(20) * time.Second, // 20 sec - gRPC default
		ServerInterval:    time.Duration(2) * time.Hour,    // 2 hours - gRPC default
		ServerTimeout:     time.Duration(20) * time.Second, // 20 sec - gRPC default
		ServerMinInterval: time.Duration(1) * time.Minute,  // match ClientInterval
	}
	// strong TLS cipher suites
	DefaultTLSCipherSuites = []uint16{
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
	}
	// default connection timeout
	DefaultConnectionTimeout = 5 * time.Second
)

// ServerConfig defines the parameters for configuring a GRPCServer instance
type ServerConfig struct {
	// ConnectionTimeout specifies the timeout for connection establishment
	// for all new connections
	ConnectionTimeout time.Duration
	// SecOpts defines the security parameters
	SecOpts SecureOptions
	// KaOpts defines the keepalive parameters
	KaOpts KeepaliveOptions
	// StreamInterceptors specifies a list of interceptors to apply to
	// streaming RPCs.  They are executed in order.
	StreamInterceptors []grpc.StreamServerInterceptor
	// UnaryInterceptors specifies a list of interceptors to apply to unary
	// RPCs.  They are executed in order.
	UnaryInterceptors []grpc.UnaryServerInterceptor
	// Logger specifies the logger the server will use
	Logger *flogging.FabricLogger
	// HealthCheckEnabled enables the gRPC Health Checking Protocol for the server
	HealthCheckEnabled bool
	// ServerStatsHandler should be set if metrics on connections are to be reported.
	ServerStatsHandler *ServerStatsHandler
	// Maximum message size the server can receive
	MaxRecvMsgSize int
	// Maximum message size the server can send
	MaxSendMsgSize int
}

// ClientConfig defines the parameters for configuring a GRPCClient instance
type ClientConfig struct {
	// SecOpts defines the security parameters
	SecOpts SecureOptions
	// KaOpts defines the keepalive parameters
	KaOpts KeepaliveOptions
	// DialTimeout controls how long the client can block when attempting to
	// establish a connection to a server
	DialTimeout time.Duration
	// AsyncConnect makes connection creation non blocking
	AsyncConnect bool
	// Maximum message size the client can receive
	MaxRecvMsgSize int
	// Maximum message size the client can send
	MaxSendMsgSize int
}

// Convert the ClientConfig to the approriate set of grpc.DialOptions.
func (cc ClientConfig) DialOptions() ([]grpc.DialOption, error) {
	var dialOpts []grpc.DialOption
	dialOpts = append(dialOpts, grpc.WithKeepaliveParams(keepalive.ClientParameters{
		Time:                cc.KaOpts.ClientInterval,
		Timeout:             cc.KaOpts.ClientTimeout,
		PermitWithoutStream: true,
	}))

	// Unless asynchronous connect is set, make connection establishment blocking.
	if !cc.AsyncConnect {
		dialOpts = append(dialOpts,
			grpc.WithBlock(),
			grpc.FailOnNonTempDialError(true),
		)
	}
	// set send/recv message size to package defaults
	maxRecvMsgSize := DefaultMaxRecvMsgSize
	if cc.MaxRecvMsgSize != 0 {
		maxRecvMsgSize = cc.MaxRecvMsgSize
	}
	maxSendMsgSize := DefaultMaxSendMsgSize
	if cc.MaxSendMsgSize != 0 {
		maxSendMsgSize = cc.MaxSendMsgSize
	}
	dialOpts = append(dialOpts, grpc.WithDefaultCallOptions(
		grpc.MaxCallRecvMsgSize(maxRecvMsgSize),
		grpc.MaxCallSendMsgSize(maxSendMsgSize),
	))

	tlsConfig, err := cc.SecOpts.TLSConfig()
	if err != nil {
		return nil, err
	}
	if tlsConfig != nil {
		transportCreds := &DynamicClientCredentials{TLSConfig: tlsConfig}
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(transportCreds))
	} else {
		dialOpts = append(dialOpts, grpc.WithInsecure())
	}

	return dialOpts, nil
}

func (cc ClientConfig) Dial(address string) (*grpc.ClientConn, error) {
	dialOpts, err := cc.DialOptions()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), cc.DialTimeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, address, dialOpts...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create new connection")
	}
	return conn, nil
}

// Clone clones this ClientConfig
func (cc ClientConfig) Clone() ClientConfig {
	shallowClone := cc
	return shallowClone
}

// SecureOptions defines the TLS security parameters for a GRPCServer or
// GRPCClient instance.
type SecureOptions struct {
	// VerifyCertificate, if not nil, is called after normal
	// certificate verification by either a TLS client or server.
	// If it returns a non-nil error, the handshake is aborted and that error results.
	VerifyCertificate func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error
	// PEM-encoded X509 public key to be used for TLS communication
	Certificate []byte
	// PEM-encoded private key to be used for TLS communication
	Key []byte
	// Set of PEM-encoded X509 certificate authorities used by clients to
	// verify server certificates
	ServerRootCAs [][]byte
	// Set of PEM-encoded X509 certificate authorities used by servers to
	// verify client certificates
	ClientRootCAs [][]byte
	// Whether or not to use TLS for communication
	UseTLS bool
	// Whether or not TLS client must present certificates for authentication
	RequireClientCert bool
	// CipherSuites is a list of supported cipher suites for TLS
	CipherSuites []uint16
	// TimeShift makes TLS handshakes time sampling shift to the past by a given duration
	TimeShift time.Duration
	// ServerNameOverride is used to verify the hostname on the returned certificates. It
	// is also included in the client's handshake to support virtual hosting
	// unless it is an IP address.
	ServerNameOverride string
}

func (so SecureOptions) TLSConfig() (*tls.Config, error) {
	// if TLS is not enabled, return
	if !so.UseTLS {
		return nil, nil
	}

	tlsConfig := &tls.Config{
		MinVersion:            tls.VersionTLS12,
		ServerName:            so.ServerNameOverride,
		VerifyPeerCertificate: so.VerifyCertificate,
	}
	if len(so.ServerRootCAs) > 0 {
		tlsConfig.RootCAs = x509.NewCertPool()
		for _, certBytes := range so.ServerRootCAs {
			if !tlsConfig.RootCAs.AppendCertsFromPEM(certBytes) {
				return nil, errors.New("error adding root certificate")
			}
		}
	}

	if so.RequireClientCert {
		cert, err := so.ClientCertificate()
		if err != nil {
			return nil, errors.WithMessage(err, "failed to load client certificate")
		}
		tlsConfig.Certificates = append(tlsConfig.Certificates, cert)
	}

	if so.TimeShift > 0 {
		tlsConfig.Time = func() time.Time {
			return time.Now().Add((-1) * so.TimeShift)
		}
	}

	return tlsConfig, nil
}

// ClientCertificate returns the client certificate that will be used
// for mutual TLS.
func (so SecureOptions) ClientCertificate() (tls.Certificate, error) {
	if so.Key == nil || so.Certificate == nil {
		return tls.Certificate{}, errors.New("both Key and Certificate are required when using mutual TLS")
	}
	cert, err := tls.X509KeyPair(so.Certificate, so.Key)
	if err != nil {
		return tls.Certificate{}, errors.WithMessage(err, "failed to create key pair")
	}
	return cert, nil
}

// KeepaliveOptions is used to set the gRPC keepalive settings for both
// clients and servers
type KeepaliveOptions struct {
	// ClientInterval is the duration after which if the client does not see
	// any activity from the server it pings the server to see if it is alive
	ClientInterval time.Duration
	// ClientTimeout is the duration the client waits for a response
	// from the server after sending a ping before closing the connection
	ClientTimeout time.Duration
	// ServerInterval is the duration after which if the server does not see
	// any activity from the client it pings the client to see if it is alive
	ServerInterval time.Duration
	// ServerTimeout is the duration the server waits for a response
	// from the client after sending a ping before closing the connection
	ServerTimeout time.Duration
	// ServerMinInterval is the minimum permitted time between client pings.
	// If clients send pings more frequently, the server will disconnect them
	ServerMinInterval time.Duration
}

// ServerKeepaliveOptions returns gRPC keepalive options for a server.
func (ka KeepaliveOptions) ServerKeepaliveOptions() []grpc.ServerOption {
	var serverOpts []grpc.ServerOption
	kap := keepalive.ServerParameters{
		Time:    ka.ServerInterval,
		Timeout: ka.ServerTimeout,
	}
	serverOpts = append(serverOpts, grpc.KeepaliveParams(kap))
	kep := keepalive.EnforcementPolicy{
		MinTime: ka.ServerMinInterval,
		// allow keepalive w/o rpc
		PermitWithoutStream: true,
	}
	serverOpts = append(serverOpts, grpc.KeepaliveEnforcementPolicy(kep))
	return serverOpts
}

// ClientKeepaliveOptions returns gRPC keepalive dial options for clients.
func (ka KeepaliveOptions) ClientKeepaliveOptions() []grpc.DialOption {
	var dialOpts []grpc.DialOption
	kap := keepalive.ClientParameters{
		Time:                ka.ClientInterval,
		Timeout:             ka.ClientTimeout,
		PermitWithoutStream: true,
	}
	dialOpts = append(dialOpts, grpc.WithKeepaliveParams(kap))
	return dialOpts
}

type Metrics struct {
	// OpenConnCounter keeps track of number of open connections
	OpenConnCounter metrics.Counter
	// ClosedConnCounter keeps track of number connections closed
	ClosedConnCounter metrics.Counter
}
