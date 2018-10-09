/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package server

import (
	"github.com/hyperledger/fabric/core/peer"
	"github.com/pkg/errors"
)

//go:generate counterfeiter -o mock/capability_checker.go -fake-name CapabilityChecker . CapabilityChecker

// CapabilityChecker is used to check whether or not a channel supports token functions.
type CapabilityChecker interface {
	FabToken(channelId string) (bool, error)
}

// TokenCapabilityChecker implements CapabilityChecker interface
type TokenCapabilityChecker struct {
	PeerOps peer.Operations
}

func (c *TokenCapabilityChecker) FabToken(channelId string) (bool, error) {
	ac, ok := c.PeerOps.GetChannelConfig(channelId).ApplicationConfig()
	if !ok {
		return false, errors.Errorf("no application config found for channel %s", channelId)
	}
	return ac.Capabilities().FabToken(), nil
}
