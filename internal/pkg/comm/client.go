/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"context"

	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

type GRPCClient struct {
	config ClientConfig
}

// NewGRPCClient creates a new implementation of GRPCClient given an address
// and client configuration
func NewGRPCClient(config ClientConfig) (*GRPCClient, error) {
	return &GRPCClient{config: config}, nil
}

// NewConnection returns a grpc.ClientConn for the target address and
// overrides the server name used to verify the hostname on the
// certificate returned by a server when using TLS
func (client *GRPCClient) NewConnection(address string) (*grpc.ClientConn, error) {
	dialOpts, err := client.config.DialOptions()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), client.config.DialTimeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, address, dialOpts...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create new connection")
	}
	return conn, nil
}
