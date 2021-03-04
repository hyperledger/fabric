/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm_test

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"testing"

	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

func TestExtractCertificateHashFromContext(t *testing.T) {
	t.Parallel()
	require.Nil(t, comm.ExtractCertificateHashFromContext(context.Background()))

	p := &peer.Peer{}
	ctx := peer.NewContext(context.Background(), p)
	require.Nil(t, comm.ExtractCertificateHashFromContext(ctx))

	p.AuthInfo = &nonTLSConnection{}
	ctx = peer.NewContext(context.Background(), p)
	require.Nil(t, comm.ExtractCertificateHashFromContext(ctx))

	p.AuthInfo = credentials.TLSInfo{}
	ctx = peer.NewContext(context.Background(), p)
	require.Nil(t, comm.ExtractCertificateHashFromContext(ctx))

	p.AuthInfo = credentials.TLSInfo{
		State: tls.ConnectionState{
			PeerCertificates: []*x509.Certificate{
				{Raw: []byte{1, 2, 3}},
			},
		},
	}
	ctx = peer.NewContext(context.Background(), p)
	h := sha256.New()
	h.Write([]byte{1, 2, 3})
	require.Equal(t, h.Sum(nil), comm.ExtractCertificateHashFromContext(ctx))
}

type nonTLSConnection struct{}

func (*nonTLSConnection) AuthType() string {
	return ""
}

func TestGetLocalIP(t *testing.T) {
	ip, err := comm.GetLocalIP()
	require.NoError(t, err)
	t.Log(ip)
}
