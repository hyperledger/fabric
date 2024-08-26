/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/hyperledger/fabric/orderer/common/broadcast/mock"
	"github.com/hyperledger/fabric/orderer/common/server"
	mock2 "github.com/hyperledger/fabric/orderer/common/server/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

func TestThrottlingAtomicBroadcast(t *testing.T) {
	p := &peer.Peer{
		AuthInfo: credentials.TLSInfo{
			State: tls.ConnectionState{
				PeerCertificates: []*x509.Certificate{
					{
						SubjectKeyId:   []byte("alice"),
						AuthorityKeyId: []byte("alice org"),
					},
				},
			},
		},
	}

	ctxWithTLS := peer.NewContext(context.Background(), p)

	for _, tst := range []struct {
		name              string
		client, org       []byte
		ctx               context.Context
		clientRL, orgRL   *mock2.RateLimiter
		throttlingEnabled bool
		shouldThrottle    bool
	}{
		{
			name:     "disabled",
			ctx:      context.Background(),
			clientRL: &mock2.RateLimiter{},
			orgRL:    &mock2.RateLimiter{},
		},
		{
			name:              "no mutual TLS",
			ctx:               context.Background(),
			throttlingEnabled: true,
			clientRL:          &mock2.RateLimiter{},
			orgRL:             &mock2.RateLimiter{},
		},
		{
			name:              "mutual TLS enabled",
			ctx:               ctxWithTLS,
			throttlingEnabled: true,
			shouldThrottle:    true,
			clientRL:          &mock2.RateLimiter{},
			orgRL:             &mock2.RateLimiter{},
			org:               []byte("alice org"),
			client:            []byte("alice"),
		},
	} {
		t.Run(tst.name, func(t *testing.T) {
			abs := &mock2.BroadcastService{}
			abs.BroadcastStub = func(stream orderer.AtomicBroadcast_BroadcastServer) error {
				stream.Recv()
				return nil
			}
			throttlingSrv := &server.ThrottlingAtomicBroadcast{
				PerClientRateLimiter:  tst.clientRL,
				PerOrgRateLimiter:     tst.orgRL,
				ThrottlingEnabled:     tst.throttlingEnabled,
				AtomicBroadcastServer: abs,
			}
			stream := &mock.ABServer{}
			stream.ContextReturns(tst.ctx)
			require.NoError(t, throttlingSrv.Broadcast(stream))
			require.Equal(t, 1, stream.RecvCallCount())
			if tst.shouldThrottle {
				require.Equal(t, 1, tst.orgRL.LimitRateCallCount())
				require.Equal(t, 1, tst.clientRL.LimitRateCallCount())
				require.Equal(t, hex.EncodeToString(tst.client), tst.clientRL.LimitRateArgsForCall(0))
				require.Equal(t, hex.EncodeToString(tst.org), tst.orgRL.LimitRateArgsForCall(0))
			} else {
				require.Equal(t, 0, tst.orgRL.LimitRateCallCount())
				require.Equal(t, 0, tst.clientRL.LimitRateCallCount())
			}
		})
	}
}
