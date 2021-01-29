/*
Copyright 2021 IBM All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/internal/pkg/gateway"
	"github.com/hyperledger/fabric/internal/pkg/gateway/registry"
)

var logger = flogging.MustGetLogger("gateway")

// Server represents the GRPC server for the Gateway
type Server struct {
	registry gateway.Registry
}

// CreateGatewayServer creates an embedded instance of the Gateway
func CreateGatewayServer(localEndorser gateway.Endorser) (*Server, error) {
	registry, err := registry.New(localEndorser)
	if err != nil {
		return nil, err
	}

	gwServer := &Server{
		registry: registry,
	}

	return gwServer, nil
}
