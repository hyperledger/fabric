/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle

// Helpers to access unexported state.

func SetChaincodeMap(c *Cache, channelID string, channelCache *ChannelCache) {
	c.definedChaincodes[channelID] = channelCache
}

func GetChaincodeMap(c *Cache, channelID string) *ChannelCache {
	return c.definedChaincodes[channelID]
}

func SetLocalChaincodesMap(c *Cache, localChaincodes map[string]*LocalChaincode) {
	c.localChaincodes = localChaincodes
}
