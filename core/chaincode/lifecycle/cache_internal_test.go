/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle

// Helpers to access unexported state.

func SetChaincodeMap(c *Cache, channelID string, chaincodes map[string]*CachedChaincodeDefinition) {
	c.definedChaincodes[channelID] = chaincodes
}

func GetChaincodeMap(c *Cache, channelID string) map[string]*CachedChaincodeDefinition {
	return c.definedChaincodes[channelID]
}
