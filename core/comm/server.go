/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"

	"google.golang.org/grpc"
)

// GRPCServer defines an interface representing a GRPC-based server
type GRPCServer interface {
	// Address returns the listen address for the GRPCServer
	Address() string
	// Start starts the underlying grpc.Server
	Start() error
	// Stop stops the underlying grpc.Server
	Stop()
	// Server returns the grpc.Server instance for the GRPCServer
	Server() *grpc.Server
	// Listener returns the net.Listener instance for the GRPCServer
	Listener() net.Listener
	// ServerCertificate returns the tls.Certificate used by the grpc.Server
	ServerCertificate() tls.Certificate
	// TLSEnabled is a flag indicating whether or not TLS is enabled for this
	// GRPCServer instance
	TLSEnabled() bool
	// MutualTLSRequired is a flag indicating whether or not client certificates
	// are required for this GRPCServer instance
	MutualTLSRequired() bool
	// AppendClientRootCAs appends PEM-encoded X509 certificate authorities to
	// the list of authorities used to verify client certificates
	AppendClientRootCAs(clientRoots [][]byte) error
	// RemoveClientRootCAs removes PEM-encoded X509 certificate authorities from
	// the list of authorities used to verify client certificates
	RemoveClientRootCAs(clientRoots [][]byte) error
	// SetClientRootCAs sets the list of authorities used to verify client
	// certificates based on a list of PEM-encoded X509 certificate authorities
	SetClientRootCAs(clientRoots [][]byte) error
	// SetServerCertificate assigns the current TLS certificate to be the peer's server certificate
	SetServerCertificate(tls.Certificate)
}

type grpcServerImpl struct {
	// Listen address for the server specified as hostname:port
	address string
	// Listener for handling network requests
	listener net.Listener
	// GRPC server
	server *grpc.Server
	// Certificate presented by the server for TLS communication
	// stored as an atomic reference
	serverCertificate atomic.Value
	// Key used by the server for TLS communication
	serverKeyPEM []byte
	// List of certificate authorities to optionally pass to the client during
	// the TLS handshake
	serverRootCAs []tls.Certificate
	// lock to protect concurrent access to append / remove
	lock *sync.Mutex
	// Set of PEM-encoded X509 certificate authorities used to populate
	// the tlsConfig.ClientCAs indexed by subject
	clientRootCAs map[string]*x509.Certificate
	// TLS configuration used by the grpc server
	tlsConfig *tls.Config
	// Is TLS enabled?
	tlsEnabled bool
	// Are client certifictes required
	mutualTLSRequired bool
}

// NewGRPCServer creates a new implementation of a GRPCServer given a
// listen address
func NewGRPCServer(address string, serverConfig ServerConfig) (GRPCServer, error) {
	if address == "" {
		return nil, errors.New("Missing address parameter")
	}
	//create our listener
	lis, err := net.Listen("tcp", address)

	if err != nil {
		return nil, err
	}
	return NewGRPCServerFromListener(lis, serverConfig)
}

// NewGRPCServerFromListener creates a new implementation of a GRPCServer given
// an existing net.Listener instance using default keepalive
func NewGRPCServerFromListener(listener net.Listener, serverConfig ServerConfig) (GRPCServer, error) {
	grpcServer := &grpcServerImpl{
		address:  listener.Addr().String(),
		listener: listener,
		lock:     &sync.Mutex{},
	}

	//set up our server options
	var serverOpts []grpc.ServerOption
	//check SecOpts
	secureConfig := serverConfig.SecOpts
	if secureConfig != nil && secureConfig.UseTLS {
		//both key and cert are required
		if secureConfig.Key != nil && secureConfig.Certificate != nil {
			grpcServer.tlsEnabled = true
			//load server public and private keys
			cert, err := tls.X509KeyPair(secureConfig.Certificate, secureConfig.Key)
			if err != nil {
				return nil, err
			}
			grpcServer.serverCertificate.Store(cert)

			//set up our TLS config
			if len(secureConfig.CipherSuites) == 0 {
				secureConfig.CipherSuites = tlsCipherSuites
			}
			getCert := func(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
				cert := grpcServer.serverCertificate.Load().(tls.Certificate)
				return &cert, nil
			}
			//base server certificate
			grpcServer.tlsConfig = &tls.Config{
				GetCertificate:         getCert,
				SessionTicketsDisabled: true,
				CipherSuites:           secureConfig.CipherSuites,
			}
			grpcServer.tlsConfig.ClientAuth = tls.RequestClientCert
			//check if client authentication is required
			if secureConfig.RequireClientCert {
				grpcServer.mutualTLSRequired = true
				//require TLS client auth
				grpcServer.tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
				//if we have client root CAs, create a certPool
				if len(secureConfig.ClientRootCAs) > 0 {
					grpcServer.clientRootCAs = make(map[string]*x509.Certificate)
					grpcServer.tlsConfig.ClientCAs = x509.NewCertPool()
					for _, clientRootCA := range secureConfig.ClientRootCAs {
						err = grpcServer.appendClientRootCA(clientRootCA)
						if err != nil {
							return nil, err
						}
					}
				}
			}

			// create credentials and add to server options
			creds := NewServerTransportCredentials(grpcServer.tlsConfig)
			serverOpts = append(serverOpts, grpc.Creds(creds))
		} else {
			return nil, errors.New("serverConfig.SecOpts must contain both Key and " +
				"Certificate when UseTLS is true")
		}
	}
	// set max send and recv msg sizes
	serverOpts = append(serverOpts, grpc.MaxSendMsgSize(MaxSendMsgSize()))
	serverOpts = append(serverOpts, grpc.MaxRecvMsgSize(MaxRecvMsgSize()))
	// set the keepalive options
	serverOpts = append(serverOpts, ServerKeepaliveOptions(serverConfig.KaOpts)...)

	grpcServer.server = grpc.NewServer(serverOpts...)

	return grpcServer, nil
}

// SetServerCertificate assigns the current TLS certificate to be the peer's server certificate
func (gServer *grpcServerImpl) SetServerCertificate(cert tls.Certificate) {
	gServer.serverCertificate.Store(cert)
}

// Address returns the listen address for this GRPCServer instance
func (gServer *grpcServerImpl) Address() string {
	return gServer.address
}

// Listener returns the net.Listener for the GRPCServer instance
func (gServer *grpcServerImpl) Listener() net.Listener {
	return gServer.listener
}

// Server returns the grpc.Server for the GRPCServer instance
func (gServer *grpcServerImpl) Server() *grpc.Server {
	return gServer.server
}

// ServerCertificate returns the tls.Certificate used by the grpc.Server
func (gServer *grpcServerImpl) ServerCertificate() tls.Certificate {
	return gServer.serverCertificate.Load().(tls.Certificate)
}

// TLSEnabled is a flag indicating whether or not TLS is enabled for the
// GRPCServer instance
func (gServer *grpcServerImpl) TLSEnabled() bool {
	return gServer.tlsEnabled
}

// MutualTLSRequired is a flag indicating whether or not client certificates
// are required for this GRPCServer instance
func (gServer *grpcServerImpl) MutualTLSRequired() bool {
	return gServer.mutualTLSRequired
}

// Start starts the underlying grpc.Server
func (gServer *grpcServerImpl) Start() error {
	return gServer.server.Serve(gServer.listener)
}

// Stop stops the underlying grpc.Server
func (gServer *grpcServerImpl) Stop() {
	gServer.server.Stop()
}

// AppendClientRootCAs appends PEM-encoded X509 certificate authorities to
// the list of authorities used to verify client certificates
func (gServer *grpcServerImpl) AppendClientRootCAs(clientRoots [][]byte) error {
	gServer.lock.Lock()
	defer gServer.lock.Unlock()
	for _, clientRoot := range clientRoots {
		err := gServer.appendClientRootCA(clientRoot)
		if err != nil {
			return err
		}
	}
	return nil
}

// internal function to add a PEM-encoded clientRootCA
func (gServer *grpcServerImpl) appendClientRootCA(clientRoot []byte) error {

	errMsg := "Failed to append client root certificate(s): %s"
	//convert to x509
	certs, subjects, err := pemToX509Certs(clientRoot)
	if err != nil {
		return fmt.Errorf(errMsg, err.Error())
	}

	if len(certs) < 1 {
		return fmt.Errorf(errMsg, "No client root certificates found")
	}

	for i, cert := range certs {
		//first add to the ClientCAs
		gServer.tlsConfig.ClientCAs.AddCert(cert)
		//add it to our clientRootCAs map using subject as key
		gServer.clientRootCAs[subjects[i]] = cert
	}
	return nil
}

// RemoveClientRootCAs removes PEM-encoded X509 certificate authorities from
// the list of authorities used to verify client certificates
func (gServer *grpcServerImpl) RemoveClientRootCAs(clientRoots [][]byte) error {
	gServer.lock.Lock()
	defer gServer.lock.Unlock()
	//remove from internal map
	for _, clientRoot := range clientRoots {
		err := gServer.removeClientRootCA(clientRoot)
		if err != nil {
			return err
		}
	}

	//create a new CertPool and populate with current clientRootCAs
	certPool := x509.NewCertPool()
	for _, clientRoot := range gServer.clientRootCAs {
		certPool.AddCert(clientRoot)
	}

	//replace the current ClientCAs pool
	gServer.tlsConfig.ClientCAs = certPool
	return nil
}

// internal function to remove a PEM-encoded clientRootCA
func (gServer *grpcServerImpl) removeClientRootCA(clientRoot []byte) error {

	errMsg := "Failed to remove client root certificate(s): %s"
	//convert to x509
	certs, subjects, err := pemToX509Certs(clientRoot)
	if err != nil {
		return fmt.Errorf(errMsg, err.Error())
	}

	if len(certs) < 1 {
		return fmt.Errorf(errMsg, "No client root certificates found")
	}

	for i, subject := range subjects {
		//remove it from our clientRootCAs map using subject as key
		//check to see if we have match
		if certs[i].Equal(gServer.clientRootCAs[subject]) {
			delete(gServer.clientRootCAs, subject)
		}
	}
	return nil
}

// SetClientRootCAs sets the list of authorities used to verify client
// certificates based on a list of PEM-encoded X509 certificate authorities
func (gServer *grpcServerImpl) SetClientRootCAs(clientRoots [][]byte) error {
	gServer.lock.Lock()
	defer gServer.lock.Unlock()

	errMsg := "Failed to set client root certificate(s): %s"

	//create a new map and CertPool
	clientRootCAs := make(map[string]*x509.Certificate)
	for _, clientRoot := range clientRoots {
		certs, subjects, err := pemToX509Certs(clientRoot)
		if err != nil {
			return fmt.Errorf(errMsg, err.Error())
		}
		if len(certs) >= 1 {
			for i, cert := range certs {
				//add it to our clientRootCAs map using subject as key
				clientRootCAs[subjects[i]] = cert
			}
		}
	}

	//create a new CertPool and populate with the new clientRootCAs
	certPool := x509.NewCertPool()
	for _, clientRoot := range clientRootCAs {
		certPool.AddCert(clientRoot)
	}
	//replace the internal map
	gServer.clientRootCAs = clientRootCAs
	//replace the current ClientCAs pool
	gServer.tlsConfig.ClientCAs = certPool
	return nil
}
