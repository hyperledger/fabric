/*
Copyright IBM Corp. 2016-2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package common

import (
	"context"
	"crypto/tls"

	ab "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/pkg/errors"
)

// OrdererClient represents a client for communicating with an ordering
// service
type OrdererClient struct {
	CommonClient
}

// NewOrdererClientFromEnv creates an instance of an OrdererClient from the
// global Viper instance
func NewOrdererClientFromEnv() (*OrdererClient, error) {
	address, override, clientConfig, err := configFromEnv("orderer")
	if err != nil {
		return nil, errors.WithMessage(err, "failed to load config for OrdererClient")
	}
	gClient, err := comm.NewGRPCClient(clientConfig)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to create OrdererClient from config")
	}
	oClient := &OrdererClient{
		CommonClient: CommonClient{
			GRPCClient: gClient,
			Address:    address,
			sn:         override,
		},
	}
	return oClient, nil
}

// Broadcast returns a broadcast client for the AtomicBroadcast service
func (oc *OrdererClient) Broadcast() (ab.AtomicBroadcast_BroadcastClient, error) {
	conn, err := oc.CommonClient.NewConnection(oc.Address, comm.ServerNameOverride(oc.sn))
	if err != nil {
		return nil, errors.WithMessagef(err, "orderer client failed to connect to %s", oc.Address)
	}
	// TODO: check to see if we should actually handle error before returning
	return ab.NewAtomicBroadcastClient(conn).Broadcast(context.TODO())
}

// Deliver returns a deliver client for the AtomicBroadcast service
func (oc *OrdererClient) Deliver() (ab.AtomicBroadcast_DeliverClient, error) {
	conn, err := oc.CommonClient.NewConnection(oc.Address, comm.ServerNameOverride(oc.sn))
	if err != nil {
		return nil, errors.WithMessagef(err, "orderer client failed to connect to %s", oc.Address)
	}
	// TODO: check to see if we should actually handle error before returning
	return ab.NewAtomicBroadcastClient(conn).Deliver(context.TODO())
}

// Certificate returns the TLS client certificate (if available)
func (oc *OrdererClient) Certificate() tls.Certificate {
	return oc.CommonClient.Certificate()
}
