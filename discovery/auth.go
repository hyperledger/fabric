/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package discovery

import "github.com/hyperledger/fabric/protos/common"


// authCache defines an interface that authenticates a request in a channel context,
// and does memoization of invocations
type authCache struct {
	passThrough bool
	support
}

func newAuthCache(passThrough bool, s support) *authCache {
	return &authCache{
		passThrough: passThrough,
		support: s,
	}
}

// Eligible returns whether the given peer is eligible for receiving
// service from the discovery service for a given channel
func (ac *authCache) EligibleForService(channel string, data common.SignedData) error {
	// In the meantime we just do a passthrough
	return ac.support.EligibleForService(channel, data)
}
