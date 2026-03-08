// Copyright the Hyperledger Fabric contributors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package internal

import (
	"context"
	"crypto/tls"
	"time"

	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

const (
	dialTimeout        = 10 * time.Second
	maxRecvMessageSize = 100 * 1024 * 1024 // 100 MiB
	maxSendMessageSize = 100 * 1024 * 1024 // 100 MiB
)

// NewClientConn ...
func NewClientConn(
	address string,
	tlsConf *tls.Config,
	kaOpts keepalive.ClientParameters,
) (*grpc.ClientConn, error) {

	dialOpts := []grpc.DialOption{
		grpc.WithKeepaliveParams(kaOpts),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(maxRecvMessageSize),
			grpc.MaxCallSendMsgSize(maxSendMessageSize),
		),
	}

	if tlsConf != nil {
		creds := credentials.NewTLS(tlsConf)
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(creds))
	} else {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	return grpc.NewClient(address, dialOpts...)
}

// NewRegisterClient ...
func NewRegisterClient(conn *grpc.ClientConn) (peer.ChaincodeSupport_RegisterClient, error) {
	return peer.NewChaincodeSupportClient(conn).Register(context.Background())
}
