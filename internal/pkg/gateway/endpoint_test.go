/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package gateway

import (
	"testing"
	"time"

	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/internal/pkg/comm"
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
