/*
Copyright 2021 IBM All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gateway

import (
	"context"
	"time"

	ab "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"google.golang.org/grpc"
)

func newEndorser(address string, tlsRootCerts [][]byte) (peer.EndorserClient, error) {
	conn, err := newConnection(address, tlsRootCerts)
	if err != nil {
		return nil, err
	}
	return peer.NewEndorserClient(conn), nil
}

func newOrderer(address string, tlsRootCerts [][]byte) (ab.AtomicBroadcast_BroadcastClient, error) {
	conn, err := newConnection(address, tlsRootCerts)
	if err != nil {
		return nil, err
	}
	abc := ab.NewAtomicBroadcastClient(conn)
	return abc.Broadcast(context.TODO()) // TODO build a context using timeouts specified in the gateway config (future user story)
}

func newConnection(address string, tlsRootCerts [][]byte) (*grpc.ClientConn, error) {
	secOpts := comm.SecureOptions{
		UseTLS:            len(tlsRootCerts) > 0,
		ServerRootCAs:     tlsRootCerts,
		RequireClientCert: false,
	}
	config := comm.ClientConfig{
		SecOpts: secOpts,
		Timeout: 5 * time.Second, // TODO from config
	}
	client, err := comm.NewGRPCClient(config)
	if err != nil {
		return nil, err
	}

	return client.NewConnection(address)
}
