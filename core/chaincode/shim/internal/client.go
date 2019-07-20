/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package internal

import (
	"context"
	"crypto/tls"
	"time"

	peerpb "github.com/hyperledger/fabric/protos/peer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
)

func NewClientConn(
	address string,
	tlsConf *tls.Config,
	kaOpts keepalive.ClientParameters,
	timeout time.Duration,
) (*grpc.ClientConn, error) {

	dialOpts := []grpc.DialOption{
		grpc.WithKeepaliveParams(kaOpts),
		grpc.WithBlock(),
		grpc.FailOnNonTempDialError(true),
	}

	if tlsConf != nil {
		dialOpts = append(
			dialOpts,
			grpc.WithTransportCredentials(
				credentials.NewTLS(tlsConf),
			),
		)
	} else {
		dialOpts = append(dialOpts, grpc.WithInsecure())
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return grpc.DialContext(ctx, address, dialOpts...)
}

func NewRegisterClient(conn *grpc.ClientConn) (peerpb.ChaincodeSupport_RegisterClient, error) {
	return peerpb.NewChaincodeSupportClient(conn).Register(context.Background())
}
