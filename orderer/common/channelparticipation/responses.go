/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelparticipation

type ErrorResponse struct {
	Error string `json:"error"`
}

// ListAllChannels carries the response to a List of all channels.
type ListAllChannels struct {
	Size          int                `json:"size"`          // The number of channels
	SystemChannel string             `json:"systemChannel"` // The system channel name, empty if doesn't exist
	Channels      []ChannelInfoShort `json:"channels"`      // All channels
}

// The short info of a single channel.
type ChannelInfoShort struct {
	Name string `json:"name"` // The channel name
	URL  string `json:"url"`  // The channel relative URL (no Host:Port)
}

// ChannelInfoFull carries the response to a List of a single channel.
type ChannelInfoFull struct {
	Name            string `json:"name"`            // The channel name
	URL             string `json:"url"`             // The channel relative URL (no Host:Port)
	ClusterRelation string `json:"clusterRelation"` // Whether it is a “member” or ”follower”, case insensitive
	Status          string `json:"status"`          // ”onboarding” or ”active”, case insensitive
	Height          uint64 `json:"height"`          // Current block height
	BlockHash       []byte `json:"blockHash"`       // Last block hash
}
