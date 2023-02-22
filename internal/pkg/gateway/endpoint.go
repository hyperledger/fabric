/*
Copyright 2021 IBM All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gateway

import (
	"context"
	"fmt"
	"time"

	ab "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/hyperledger/fabric/internal/pkg/peer/orderers"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

type endorser struct {
	client          peer.EndorserClient
	closeConnection func() error
	*endpointConfig
}

type orderer struct {
	client          ab.AtomicBroadcastClient
	closeConnection func() error
	*endpointConfig
}

type endpointConfig struct {
	pkiid        common.PKIidType
	address      string
	logAddress   string
	mspid        string
	tlsRootCerts [][]byte
}

type (
	endorserConnector func(*grpc.ClientConn) peer.EndorserClient
	ordererConnector  func(*grpc.ClientConn) ab.AtomicBroadcastClient
)

//go:generate counterfeiter -o mocks/dialer.go --fake-name Dialer . dialer
type dialer func(ctx context.Context, target string, opts ...grpc.DialOption) (*grpc.ClientConn, error)

type endpointFactory struct {
	timeout                  time.Duration
	connectEndorser          endorserConnector
	connectOrderer           ordererConnector
	dialer                   dialer
	clientCert               []byte
	clientKey                []byte
	ordererEndpointOverrides map[string]*orderers.Endpoint
}

func (ef *endpointFactory) newEndorser(pkiid common.PKIidType, address, mspid string, tlsRootCerts [][]byte) (*endorser, error) {
	conn, err := ef.newConnection(address, tlsRootCerts)
	if err != nil {
		return nil, err
	}
	connectEndorser := ef.connectEndorser
	if connectEndorser == nil {
		connectEndorser = peer.NewEndorserClient
	}
	close := func() error {
		if conn != nil && conn.GetState() != connectivity.Shutdown {
			logger.Infow("Closing connection to remote endorser", "address", address, "mspid", mspid)
			return conn.Close()
		}
		return nil
	}
	return &endorser{
		client:          connectEndorser(conn),
		closeConnection: close,
		endpointConfig:  &endpointConfig{pkiid: pkiid, address: address, logAddress: address, mspid: mspid, tlsRootCerts: tlsRootCerts},
	}, nil
}

func (ef *endpointFactory) newOrderer(address, mspid string, tlsRootCerts [][]byte) (*orderer, error) {
	connAddress := address
	logAddess := address
	connCerts := tlsRootCerts
	if override, ok := ef.ordererEndpointOverrides[address]; ok {
		connAddress = override.Address
		connCerts = override.RootCerts
		logAddess = fmt.Sprintf("%s (mapped from %s)", connAddress, address)
		logger.Debugw("Overriding orderer endpoint address", "from", address, "to", connAddress)
	}
	conn, err := ef.newConnection(connAddress, connCerts)
	if err != nil {
		return nil, err
	}
	connectOrderer := ef.connectOrderer
	if connectOrderer == nil {
		connectOrderer = ab.NewAtomicBroadcastClient
	}
	return &orderer{
		client:          connectOrderer(conn),
		closeConnection: conn.Close,
		endpointConfig:  &endpointConfig{address: address, logAddress: logAddess, mspid: mspid, tlsRootCerts: tlsRootCerts},
	}, nil
}

func (ef *endpointFactory) newConnection(address string, tlsRootCerts [][]byte) (*grpc.ClientConn, error) {
	config := comm.ClientConfig{
		SecOpts: comm.SecureOptions{
			UseTLS:            len(tlsRootCerts) > 0,
			ServerRootCAs:     tlsRootCerts,
			RequireClientCert: true,
			Certificate:       ef.clientCert,
			Key:               ef.clientKey,
		},
		DialTimeout:  ef.timeout,
		AsyncConnect: true,
	}
	dialOpts, err := config.DialOptions()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), ef.timeout)
	defer cancel()

	dialer := ef.dialer
	if dialer == nil {
		dialer = grpc.DialContext
	}
	conn, err := dialer(ctx, address, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create new connection: %w", err)
	}
	return conn, nil
}
