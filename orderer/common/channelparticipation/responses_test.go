/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelparticipation_test

import (
	"encoding/json"
	"testing"

	"github.com/hyperledger/fabric/orderer/common/channelparticipation"
	"github.com/stretchr/testify/assert"
)

func TestChannelInfoShort(t *testing.T) {
	info := channelparticipation.ChannelInfoShort{
		Name: "my-channel",
		URL:  "/api/v1/channels/my-channel",
	}

	buff, err := json.Marshal(info)
	assert.NoError(t, err)
	assert.Equal(t, `{"name":"my-channel","url":"/api/v1/channels/my-channel"}`, string(buff))

	buff2 := []byte(`{"name":"my-channel2","url":"/api/v1/channels/my-channel2"}`)
	var info2 channelparticipation.ChannelInfoShort
	err = json.Unmarshal(buff2, &info2)
	assert.NoError(t, err)
	assert.NotNil(t, info2)
	assert.Equal(t, "my-channel2", info2.Name)
	assert.Equal(t, "/api/v1/channels/my-channel2", info2.URL)

	buff3 := []byte(`{"name":"my-channel2","url":"/api/v1/channels/my-channel3","oops"}`)
	var info3 channelparticipation.ChannelInfoShort
	err = json.Unmarshal(buff3, &info3)
	assert.Error(t, err)
}

func TestListAllChannels(t *testing.T) {
	list := channelparticipation.ListAllChannels{
		Channels:      nil,
		Size:          0,
		SystemChannel: "",
	}

	buff, err := json.Marshal(list)
	assert.NoError(t, err)
	assert.Equal(t, `{"size":0,"systemChannel":"","channels":null}`, string(buff))

	list.Size = 2
	list.SystemChannel = "A"
	list.Channels = []channelparticipation.ChannelInfoShort{
		{Name: "A", URL: "/api/channels/A"},
		{Name: "B", URL: "/api/channels/B"},
	}

	buff, err = json.Marshal(list)
	assert.NoError(t, err)
	assert.Equal(t, `{"size":2,"systemChannel":"A","channels":[{"name":"A","url":"/api/channels/A"},{"name":"B","url":"/api/channels/B"}]}`, string(buff))
}

func TestChannelInfoFull(t *testing.T) {
	info := channelparticipation.ChannelInfoFull{
		Name:            "a",
		URL:             "/api/channels/a",
		ClusterRelation: "follower",
		Status:          "active",
		Height:          uint64(1) << 60,
		BlockHash:       []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
	}

	buff, err := json.Marshal(info)
	assert.NoError(t, err)
	assert.Equal(t, `{"name":"a","url":"/api/channels/a","clusterRelation":"follower","status":"active","height":1152921504606846976,"blockHash":"AQIDBAUGBwgJCgsMDQ4PEA=="}`, string(buff))

	var info2 channelparticipation.ChannelInfoFull
	err = json.Unmarshal(buff, &info2)
	assert.NoError(t, err)
	assert.Equal(t, info.Height, info2.Height)
	assert.Equal(t, info.BlockHash, info2.BlockHash)
}
