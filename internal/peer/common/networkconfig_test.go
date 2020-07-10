/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common_test

import (
	"testing"

	"github.com/hyperledger/fabric/internal/peer/common"
	"github.com/stretchr/testify/require"
)

func TestGetConfig(t *testing.T) {
	require := require.New(t)

	// failure - empty file name
	networkConfig, err := common.GetConfig("")
	require.Error(err)
	require.Nil(networkConfig)

	// failure - file doesn't exist
	networkConfig, err = common.GetConfig("fakefile.yaml")
	require.Error(err)
	require.Nil(networkConfig)

	// failure - unexpected values for a few bools in the connection profile
	networkConfig, err = common.GetConfig("testdata/connectionprofile-bad.yaml")
	require.Error(err, "error should have been nil")
	require.Nil(networkConfig, "network config should be set")

	// success
	networkConfig, err = common.GetConfig("testdata/connectionprofile.yaml")
	require.NoError(err, "error should have been nil")
	require.NotNil(networkConfig, "network config should be set")
	require.Equal(networkConfig.Name, "connection-profile")

	channelPeers := networkConfig.Channels["mychannel"].Peers
	require.Equal(len(channelPeers), 2)
	for _, peer := range channelPeers {
		require.True(peer.EndorsingPeer)
	}

	peers := networkConfig.Peers
	require.Equal(len(peers), 2)
	for _, peer := range peers {
		require.NotEmpty(peer.TLSCACerts.Path)
	}
}
