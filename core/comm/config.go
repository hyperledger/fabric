/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"crypto/tls"
	"crypto/x509"
	"time"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/metrics"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

// Configuration defaults
var (
	// Max send and receive bytes for grpc clients and servers
	MaxRecvMsgSize = 100 * 1024 * 1024
	MaxSendMsgSize = 100 * 1024 * 1024
	// Default peer keepalive options
	DefaultKeepaliveOptions = &KeepaliveOptions{
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
	SecOpts *SecureOptions
	// KaOpts defines the keepalive parameters
	KaOpts *KeepaliveOptions
	// StreamInterceptors specifies a list of interceptors to apply to
	// streaming RPCs.  They are executed in order.
	StreamInterceptors []grpc.StreamServerInterceptor
	// UnaryInterceptors specifies a list of interceptors to apply to unary
	// RPCs.  They are executed in order.
	UnaryInterceptors []grpc.UnaryServerInterceptor
	// Logger specifies the logger the server will use
	Logger *flogging.FabricLogger
	// ServerStatsHandler should be set if metrics on connections are to be reported.
	ServerStatsHandler *ServerStatsHandler
}

// ClientConfig defines the parameters for configuring a GRPCClient instance
type ClientConfig struct {
	// SecOpts defines the security parameters
	SecOpts *SecureOptions
	// KaOpts defines the keepalive parameters
	KaOpts *KeepaliveOptions
	// Timeout specifies how long the client will block when attempting to
	// establish a connection
	Timeout time.Duration
	// AsyncConnect makes connection creation non blocking
	AsyncConnect bool
}

// Clone clones this ClientConfig
func (cc ClientConfig) Clone() ClientConfig {
	shallowClone := cc
	if shallowClone.SecOpts != nil {
		secOptsClone := *cc.SecOpts
		shallowClone.SecOpts = &secOptsClone
	}
	if shallowClone.KaOpts != nil {
		kaOptsClone := *cc.KaOpts
		shallowClone.KaOpts = &kaOptsClone
	}
	return shallowClone
}

// SecureOptions defines the security parameters (e.g. TLS) for a
// GRPCServer or GRPCClient instance
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

type Metrics struct {
	// OpenConnCounter keeps track of number of open connections
	OpenConnCounter metrics.Counter
	// ClosedConnCounter keeps track of number connections closed
	ClosedConnCounter metrics.Counter
}

// ServerKeepaliveOptions returns gRPC keepalive options for server.  If
// opts is nil, the default keepalive options are returned
func ServerKeepaliveOptions(ka *KeepaliveOptions) []grpc.ServerOption {
	// use default keepalive options if nil
	if ka == nil {
		ka = DefaultKeepaliveOptions
	}
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

// ClientKeepaliveOptions returns gRPC keepalive options for clients.  If
// opts is nil, the default keepalive options are returned
func ClientKeepaliveOptions(ka *KeepaliveOptions) []grpc.DialOption {
	// use default keepalive options if nil
	if ka == nil {
		ka = DefaultKeepaliveOptions
	}

	var dialOpts []grpc.DialOption
	kap := keepalive.ClientParameters{
		Time:                ka.ClientInterval,
		Timeout:             ka.ClientTimeout,
		PermitWithoutStream: true,
	}
	dialOpts = append(dialOpts, grpc.WithKeepaliveParams(kap))
	return dialOpts
}
