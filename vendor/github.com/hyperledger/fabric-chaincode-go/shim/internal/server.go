// Copyright the Hyperledger Fabric contributors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package internal

import (
	"crypto/tls"
	"errors"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
)

const (
	serverInterval    = time.Duration(2) * time.Hour    // 2 hours - gRPC default
	serverTimeout     = time.Duration(20) * time.Second // 20 sec - gRPC default
	serverMinInterval = time.Duration(1) * time.Minute
	connectionTimeout = 5 * time.Second
)

// Server abstracts grpc service properties
type Server struct {
	Listener net.Listener
	Server   *grpc.Server
}

// Start the server
func (s *Server) Start() error {
	if s.Listener == nil {
		return errors.New("nil listener")
	}

	if s.Server == nil {
		return errors.New("nil server")
	}

	return s.Server.Serve(s.Listener)
}

// Stop the server
func (s *Server) Stop() {
	if s.Server != nil {
		s.Server.Stop()
	}
}

// NewServer creates a new implementation of a GRPC Server given a
// listen address
func NewServer(
	address string,
	tlsConf *tls.Config,
	srvKaOpts *keepalive.ServerParameters,
) (*Server, error) {
	if address == "" {
		return nil, errors.New("server listen address not provided")
	}

	//create our listener
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}

	//set up server options for keepalive and TLS
	var serverOpts []grpc.ServerOption

	if srvKaOpts != nil {
		serverOpts = append(serverOpts, grpc.KeepaliveParams(*srvKaOpts))
	} else {
		serverKeepAliveParameters := keepalive.ServerParameters{
			Time:    1 * time.Minute,
			Timeout: 20 * time.Second,
		}
		serverOpts = append(serverOpts, grpc.KeepaliveParams(serverKeepAliveParameters))
	}

	if tlsConf != nil {
		serverOpts = append(serverOpts, grpc.Creds(credentials.NewTLS(tlsConf)))
	}

	// Default properties follow - let's start simple and stick with defaults for now.
	// These match Fabric peer side properties. We can expose these as user properties
	// if needed

	// set max send and recv msg sizes
	serverOpts = append(serverOpts, grpc.MaxSendMsgSize(maxSendMessageSize))
	serverOpts = append(serverOpts, grpc.MaxRecvMsgSize(maxRecvMessageSize))

	//set enforcement policy
	kep := keepalive.EnforcementPolicy{
		MinTime: serverMinInterval,
		// allow keepalive w/o rpc
		PermitWithoutStream: true,
	}
	serverOpts = append(serverOpts, grpc.KeepaliveEnforcementPolicy(kep))

	//set default connection timeout
	serverOpts = append(serverOpts, grpc.ConnectionTimeout(connectionTimeout))

	server := grpc.NewServer(serverOpts...)

	return &Server{Listener: listener, Server: server}, nil
}
