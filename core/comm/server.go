/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package comm

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

//GRPCServer defines an interface representing a GRPC-based server
type GRPCServer interface {
	//Address returns the listen address for the GRPCServer
	Address() string
	//Start starts the underlying grpc.Server
	Start() error
	//Stop stops the underlying grpc.Server
	Stop()
	//Server returns the grpc.Server instance for the GRPCServer
	Server() *grpc.Server
	//Listener returns the net.Listener instance for the GRPCServer
	Listener() net.Listener
	//ServerCertificate returns the tls.Certificate used by the grpc.Server
	ServerCertificate() tls.Certificate
	//TLSEnabled is a flag indicating whether or not TLS is enabled for this GRPCServer instance
	TLSEnabled() bool
}

type grpcServerImpl struct {
	//Listen address for the server specified as hostname:port
	address string
	//Listener for handling network requests
	listener net.Listener
	//GRPC server
	server *grpc.Server
	//Certificate presented by the server for TLS communication
	serverCertificate tls.Certificate
	//Key used by the server for TLS communication
	serverKeyPEM []byte
	//List of certificate authorities to optionally pass to the client during the TLS handshake
	serverRootCAs []tls.Certificate
	//List of certificate authorities to be used to authenticate clients if client authentication is required
	clientRootCAs *x509.CertPool
	//Is TLS enabled?
	tlsEnabled bool
}

/*
NewGRPCServer creates a new implementation of a GRPCServer given a listen address.
In order to enable TLS, serverKey and ServerCertificate are required.

Parameters:
	address:  Listen address to use formatted as hostname:port
	serverKey:  PEM-encoded private key to be used by the server for TLS communication
	serverCertificate:  PEM-encoded X509 public key to be used by the server for TLS communication
	serverRootCAs:  (optional) Set of PEM-encoded X509 certificate authorities to optionally send as part of
					the server handshake
	clientRootCAs:  (optional) Set of PEM-encoded X509 certificate authorities to use when verifying client
					certificates
*/
func NewGRPCServer(address string, serverKey []byte, serverCertificate []byte, serverRootCAs [][]byte,
	clientRootCAs [][]byte) (GRPCServer, error) {

	if address == "" {
		return nil, errors.New("Missing address parameter")
	}
	//create our listener
	lis, err := net.Listen("tcp", address)

	if err != nil {
		return nil, err
	}

	return NewGRPCServerFromListener(lis, serverKey, serverCertificate, serverRootCAs, clientRootCAs)

}

/*
NewGRPCServerFromListener creates a new implementation of a GRPCServer given an existing Listener instance.
In order to enable TLS, serverKey and ServerCertificate are required.

Parameters:
	listener:  Listener to use
	serverKey:  PEM-encoded private key to be used by the server for TLS communication
	serverCertificate:  PEM-encoded X509 public key to be used by the server for TLS communication
	serverRootCAs:  (optional) Set of PEM-encoded X509 certificate authorities to optionally send as part of
					the server handshake
	clientRootCAs:  (optional) Set of PEM-encoded X509 certificate authorities to use when verifying client
					certificates
*/
func NewGRPCServerFromListener(listener net.Listener, serverKey []byte, serverCertificate []byte,
	serverRootCAs [][]byte, clientRootCAs [][]byte) (GRPCServer, error) {

	grpcServer := &grpcServerImpl{
		address:  listener.Addr().String(),
		listener: listener,
	}

	//set up our server options
	var serverOpts []grpc.ServerOption
	//check for TLS parameters
	if serverKey != nil || serverCertificate != nil {
		//both are required
		if serverKey != nil && serverCertificate != nil {
			grpcServer.tlsEnabled = true
			//load server public and private keys
			cert, err := tls.X509KeyPair(serverCertificate, serverKey)
			if err != nil {
				return nil, err
			}
			grpcServer.serverCertificate = cert

			//set up our TLS config

			//base server certificate
			certificates := []tls.Certificate{grpcServer.serverCertificate}

			/**
			//if we have server root CAs append them
			if len(serverRootCAs) > 0 {
				//certificates = append(certificates, serverRootCAs...)
			}

			//if we have client root CAs, create a certPool
			if len(clientRootCAs) > 0 {
				grpcServer.clientRootCAs = x509.NewCertPool()
			}
			*/
			tlsConfig := &tls.Config{
				Certificates: certificates,
			}

			//create credentials
			creds := credentials.NewTLS(tlsConfig)

			//add to server options
			serverOpts = append(serverOpts, grpc.Creds(creds))

		} else {
			return nil, errors.New("Both serverKey and serverCertificate are required in order to enable TLS")
		}
	}

	grpcServer.server = grpc.NewServer(serverOpts...)

	return grpcServer, nil
}

//Address returns the listen address for this GRPCServer instance
func (gServer *grpcServerImpl) Address() string {
	return gServer.address
}

//Listener returns the net.Listener for the GRPCServer instance
func (gServer *grpcServerImpl) Listener() net.Listener {
	return gServer.listener
}

//Server returns the grpc.Server for the GRPCServer instance
func (gServer *grpcServerImpl) Server() *grpc.Server {
	return gServer.server
}

//ServerCertificate returns the tls.Certificate used by the grpc.Server
func (gServer *grpcServerImpl) ServerCertificate() tls.Certificate {
	return gServer.serverCertificate
}

//TLSEnabled is a flag indicating whether or not TLS is enabled for the GRPCServer instance
func (gServer *grpcServerImpl) TLSEnabled() bool {
	return gServer.tlsEnabled
}

//Start starts the underlying grpc.Server
func (gServer *grpcServerImpl) Start() error {
	return gServer.server.Serve(gServer.listener)
}

//Stop stops the underlying grpc.Server
func (gServer *grpcServerImpl) Stop() {
	gServer.server.Stop()
}
