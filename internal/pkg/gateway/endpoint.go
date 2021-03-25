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
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"google.golang.org/grpc"
)

type endorser struct {
	client peer.EndorserClient
	*endpointConfig
}

type orderer struct {
	client ab.AtomicBroadcastClient
	*endpointConfig
}

type endpointConfig struct {
	address string
	mspid   string
}

type (
	endorserConnector func(*grpc.ClientConn) peer.EndorserClient
	ordererConnector  func(*grpc.ClientConn) ab.AtomicBroadcastClient
)

//go:generate counterfeiter -o mocks/dialer.go --fake-name Dialer . dialer
type dialer func(ctx context.Context, target string, opts ...grpc.DialOption) (*grpc.ClientConn, error)

type endpointFactory struct {
	timeout         time.Duration
	connectEndorser endorserConnector
	connectOrderer  ordererConnector
	dialer          dialer
}

func (ef *endpointFactory) newEndorser(address, mspid string, tlsRootCerts [][]byte) (*endorser, error) {
	conn, err := ef.newConnection(address, tlsRootCerts)
	if err != nil {
		return nil, err
	}
	connectEndorser := ef.connectEndorser
	if connectEndorser == nil {
		connectEndorser = peer.NewEndorserClient
	}
	return &endorser{
		client:         connectEndorser(conn),
		endpointConfig: &endpointConfig{address: address, mspid: mspid},
	}, nil
}

func (ef *endpointFactory) newOrderer(address, mspid string, tlsRootCerts [][]byte) (*orderer, error) {
	conn, err := ef.newConnection(address, tlsRootCerts)
	if err != nil {
		return nil, err
	}
	connectOrderer := ef.connectOrderer
	if connectOrderer == nil {
		connectOrderer = ab.NewAtomicBroadcastClient
	}
	return &orderer{
		client:         connectOrderer(conn),
		endpointConfig: &endpointConfig{address: address, mspid: mspid},
	}, nil
}

func (ef *endpointFactory) newConnection(address string, tlsRootCerts [][]byte) (*grpc.ClientConn, error) {
	config := comm.ClientConfig{
		SecOpts: comm.SecureOptions{
			UseTLS:            len(tlsRootCerts) > 0,
			ServerRootCAs:     tlsRootCerts,
			RequireClientCert: false,
		},
		DialTimeout: ef.timeout,
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
