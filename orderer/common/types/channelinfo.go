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
	// The system channel info, nil if it doesn't exist.
	SystemChannel *ChannelInfoShort `json:"systemChannel"`
	// Application channels only, nil or empty if no channels defined.
	Channels []ChannelInfoShort `json:"channels"`
}

// ChannelInfoShort carries a short info of a single channel.
type ChannelInfoShort struct {
	// The channel name.
	Name string `json:"name"`
	// The channel relative URL (no Host:Port, only path), e.g.: "/participation/v1/channels/my-channel".
	URL string `json:"url"`
}

// ChannelInfo carries the response to an HTTP request to List a single channel.
// This is marshaled into the body of the HTTP response.
type ChannelInfo struct {
	// The channel name.
	Name string `json:"name"`
	// The channel relative URL (no Host:Port, only path), e.g.: "/participation/v1/channels/my-channel".
	URL string `json:"url"`
	// Whether the orderer is a “member” or ”follower” of the cluster, for this channel. Case insensitive.
	ClusterRelation string `json:"clusterRelation"`
	// Whether the orderer is ”onboarding” or ”active”, for this channel. Case insensitive.
	Status string `json:"status"`
	// Current block height.
	Height uint64 `json:"height"`
}
