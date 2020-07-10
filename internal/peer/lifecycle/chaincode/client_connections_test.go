/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"testing"

	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/stretchr/testify/require"
)

func TestNewClientConnections(t *testing.T) {
	require := require.New(t)
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.Nil(err)

	t.Run("bad connection profile", func(t *testing.T) {
		input := &ClientConnectionsInput{
			CommandName:           "install",
			EndorserRequired:      true,
			ConnectionProfilePath: "testdata/connectionprofile-bad.yaml",
		}

		c, err := NewClientConnections(input, cryptoProvider)
		require.Nil(c)
		require.Error(err)
		require.Contains(err.Error(), "failed to validate peer connection parameters: error unmarshaling YAML")
	})

	t.Run("uneven connection profile", func(t *testing.T) {
		input := &ClientConnectionsInput{
			CommandName:           "commit",
			ChannelID:             "mychannel",
			EndorserRequired:      true,
			ConnectionProfilePath: "testdata/connectionprofile-uneven.yaml",
		}

		c, err := NewClientConnections(input, cryptoProvider)
		require.Nil(c)
		require.Error(err)
		require.EqualError(err, "failed to validate peer connection parameters: peer 'peer0.org2.example.com' doesn't have associated peer config")
	})

	t.Run("good connection profile - two peers", func(t *testing.T) {
		input := &ClientConnectionsInput{
			CommandName:           "approveformyorg",
			ChannelID:             "mychannel",
			EndorserRequired:      true,
			ConnectionProfilePath: "testdata/connectionprofile.yaml",
		}

		c, err := NewClientConnections(input, cryptoProvider)
		require.Nil(c)
		require.Error(err)
		require.Contains(err.Error(), "failed to retrieve endorser client")
	})

	t.Run("more than one peer not allowed", func(t *testing.T) {
		input := &ClientConnectionsInput{
			CommandName:      "install",
			EndorserRequired: true,
			PeerAddresses:    []string{"testing123", "testing321"},
		}

		c, err := NewClientConnections(input, cryptoProvider)
		require.Nil(c)
		require.Error(err)
		require.EqualError(err, "failed to validate peer connection parameters: 'install' command supports one peer. 2 peers provided")
	})

	t.Run("more TLS root cert files than peer addresses and TLS enabled", func(t *testing.T) {
		input := &ClientConnectionsInput{
			CommandName:      "install",
			EndorserRequired: true,
			PeerAddresses:    []string{"testing123"},
			TLSRootCertFiles: []string{"123testing", "321testing"},
			TLSEnabled:       true,
		}

		c, err := NewClientConnections(input, cryptoProvider)
		require.Nil(c)
		require.Error(err)
		require.EqualError(err, "failed to validate peer connection parameters: number of peer addresses (1) does not match the number of TLS root cert files (2)")
	})

	t.Run("failure connecting to endorser - TLS enabled", func(t *testing.T) {
		input := &ClientConnectionsInput{
			CommandName:      "install",
			EndorserRequired: true,
			PeerAddresses:    []string{"testing123"},
			TLSRootCertFiles: []string{"123testing"},
			TLSEnabled:       true,
		}

		c, err := NewClientConnections(input, cryptoProvider)
		require.Nil(c)
		require.Error(err)
		require.Contains(err.Error(), "failed to retrieve endorser client")
	})

	t.Run("failure connecting to endorser - TLS disabled", func(t *testing.T) {
		input := &ClientConnectionsInput{
			CommandName:      "install",
			EndorserRequired: true,
			PeerAddresses:    []string{"testing123"},
			TLSRootCertFiles: []string{"123testing"},
			TLSEnabled:       false,
		}

		c, err := NewClientConnections(input, cryptoProvider)
		require.Nil(c)
		require.Error(err)
		require.Contains(err.Error(), "failed to retrieve endorser client")
	})

	t.Run("no endorser clients - programming bug", func(t *testing.T) {
		input := &ClientConnectionsInput{
			CommandName:      "install",
			EndorserRequired: true,
		}

		c, err := NewClientConnections(input, cryptoProvider)
		require.Nil(c)
		require.Error(err)
		require.Contains(err.Error(), "no endorser clients retrieved")
	})

	t.Run("install using connection profile", func(t *testing.T) {
		input := &ClientConnectionsInput{
			CommandName:           "install",
			EndorserRequired:      true,
			ConnectionProfilePath: "testdata/connectionprofile.yaml",
			TargetPeer:            "peer0.org2.example.com",
		}

		c, err := NewClientConnections(input, cryptoProvider)
		require.Nil(c)
		require.Error(err)
		require.Contains(err.Error(), "failed to retrieve endorser client")
	})

	t.Run("install using connection profile - no target peer specified", func(t *testing.T) {
		input := &ClientConnectionsInput{
			CommandName:           "install",
			EndorserRequired:      true,
			ConnectionProfilePath: "testdata/connectionprofile.yaml",
			TargetPeer:            "",
		}

		c, err := NewClientConnections(input, cryptoProvider)
		require.Nil(c)
		require.Error(err)
		require.Contains(err.Error(), "failed to validate peer connection parameters: --targetPeer must be specified for channel-less operation using connection profile")
	})

	t.Run("install using connection profile - target peer doesn't exist", func(t *testing.T) {
		input := &ClientConnectionsInput{
			CommandName:           "install",
			EndorserRequired:      true,
			ConnectionProfilePath: "testdata/connectionprofile.yaml",
			TargetPeer:            "not-a-peer",
		}

		c, err := NewClientConnections(input, cryptoProvider)
		require.Nil(c)
		require.Error(err)
		require.Contains(err.Error(), "failed to validate peer connection parameters: peer 'not-a-peer' doesn't have associated peer config")
	})

	t.Run("failure connecting to orderer", func(t *testing.T) {
		input := &ClientConnectionsInput{
			OrdererRequired:  true,
			OrderingEndpoint: "testing",
			PeerAddresses:    []string{"testing123"},
			TLSRootCertFiles: []string{"123testing"},
		}

		c, err := NewClientConnections(input, cryptoProvider)
		require.Nil(c)
		require.Error(err)
		require.Contains(err.Error(), "cannot obtain orderer endpoint, empty endorser list")
	})
}
