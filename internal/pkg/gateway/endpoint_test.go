/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package gateway

import (
	"context"
	"testing"
	"time"

	peerprotos "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/hyperledger/fabric/internal/pkg/gateway/mocks"
	"github.com/stretchr/testify/require"
)

func TestMutualTLS(t *testing.T) {
	ca, err := tlsgen.NewCA()
	require.NoError(t, err, "failed to create CA")

	serverPair, err := ca.NewServerCertKeyPair("127.0.0.1")
	require.NoError(t, err, "failed to create server key pair")

	clientPair, err := ca.NewClientCertKeyPair()
	require.NoError(t, err, "failed to create client key pair")

	rootTLSCert := ca.CertBytes()

	server, err := comm.NewGRPCServer("127.0.0.1:0", comm.ServerConfig{
		SecOpts: comm.SecureOptions{
			UseTLS:            true,
			RequireClientCert: true,
			Certificate:       serverPair.Cert,
			Key:               serverPair.Key,
			ClientRootCAs:     [][]byte{rootTLSCert},
		},
	})
	require.NoError(t, err)

	go server.Start()
	defer server.Stop()

	factory := &endpointFactory{
		timeout:    10 * time.Second,
		clientCert: clientPair.Cert,
		clientKey:  clientPair.Key,
	}

	endorser, err := factory.newEndorser(common.PKIidType{}, server.Address(), "msp1", [][]byte{rootTLSCert})
	require.NoError(t, err, "failed to make mTLS connection to server")

	err = endorser.closeConnection()
	require.NoError(t, err, "failed to close connection")
}

//go:generate counterfeiter -o mocks/endorserserver.go --fake-name EndorserServer . endorserServer
type endorserServer interface {
	peerprotos.EndorserServer
}

var payload = []byte("test response")

func TestAsyncConnect(t *testing.T) {
	server, err := comm.NewGRPCServer("127.0.0.1:0", comm.ServerConfig{})
	require.NoError(t, err)

	endorserServer := &mocks.EndorserServer{}
	endorserServer.ProcessProposalReturns(&peerprotos.ProposalResponse{Payload: payload}, nil)
	peerprotos.RegisterEndorserServer(server.Server(), endorserServer)

	factory := &endpointFactory{
		timeout: 10 * time.Second,
	}

	endorser, err := factory.newEndorser(common.PKIidType{}, server.Address(), "msp1", nil)
	require.NoError(t, err, "failed to make async connection to server")

	endorse := func() (*peerprotos.ProposalResponse, error) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		return endorser.client.ProcessProposal(ctx, &peerprotos.SignedProposal{})
	}

	_, err = endorse() // server not started, so should timeout
	require.ErrorContains(t, err, "DeadlineExceeded")

	go server.Start()
	defer server.Stop()

	pr, err := endorse()
	require.NoError(t, err)
	require.Equal(t, payload, pr.GetPayload())

	err = endorser.closeConnection()
	require.NoError(t, err, "failed to close connection")
}
