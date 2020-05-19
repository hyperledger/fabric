/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package types

// ErrorResponse carries the error response an HTTP request.
// This is marshaled into the body of the HTTP response.
type ErrorResponse struct {
	Error string `json:"error"`
}

// ChannelList carries the response to an HTTP request to List all the channels.
// This is marshaled into the body of the HTTP response.
type ChannelList struct {
	SystemChannel *ChannelInfoShort  `json:"systemChannel"` // The system channel, nil if doesn't exist
	Channels      []ChannelInfoShort `json:"channels"`      // Application channels only
}

// ChannelInfoShort carries a short info of a single channel.
type ChannelInfoShort struct {
	Name string `json:"name"` // The channel name
	URL  string `json:"url"`  // The channel relative URL (no Host:Port)
}

// ChannelInfo carries the response to an HTTP request to List a single channel.
// This is marshaled into the body of the HTTP response.
type ChannelInfo struct {
	Name            string `json:"name"`            // The channel name
	URL             string `json:"url"`             // The channel relative URL (no Host:Port)
	ClusterRelation string `json:"clusterRelation"` // Whether it is a “member” or ”follower”, case insensitive
	Status          string `json:"status"`          // ”onboarding” or ”active”, case insensitive
	Height          uint64 `json:"height"`          // Current block height
}
