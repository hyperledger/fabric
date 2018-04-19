/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common_test

import (
	"testing"

	"github.com/hyperledger/fabric/peer/common"
	"github.com/stretchr/testify/assert"
)

func TestGetConfig(t *testing.T) {
	assert := assert.New(t)

	// failure - empty file name
	networkConfig, err := common.GetConfig("")
	assert.Error(err)
	assert.Nil(networkConfig)

	// failure - file doesn't exist
	networkConfig, err = common.GetConfig("fakefile.yaml")
	assert.Error(err)
	assert.Nil(networkConfig)

	// failure - unexpected values for a few bools in the connection profile
	networkConfig, err = common.GetConfig("testdata/connectionprofile-bad.yaml")
	assert.Error(err, "error should have been nil")
	assert.Nil(networkConfig, "network config should be set")

	// success
	networkConfig, err = common.GetConfig("testdata/connectionprofile.yaml")
	assert.NoError(err, "error should have been nil")
	assert.NotNil(networkConfig, "network config should be set")
	assert.Equal(networkConfig.Name, "connection-profile")

	channelPeers := networkConfig.Channels["mychannel"].Peers
	assert.Equal(len(channelPeers), 2)
	for _, peer := range channelPeers {
		assert.True(peer.EndorsingPeer)
	}

	peers := networkConfig.Peers
	assert.Equal(len(peers), 2)
	for _, peer := range peers {
		assert.NotEmpty(peer.TLSCACerts.Path)
	}
}
