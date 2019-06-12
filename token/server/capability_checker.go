/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package server

import (
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/pkg/errors"
)

//go:generate counterfeiter -o mock/capability_checker.go -fake-name CapabilityChecker . CapabilityChecker

// CapabilityChecker is used to check whether or not a channel supports token functions.
type CapabilityChecker interface {
	FabToken(channelId string) (bool, error)
}

//go:generate counterfeiter -o mock/channel_config_getter.go -fake-name ChannelConfigGetter . ChannelConfigGetter

// ChannelConfigGetter is used to get a channel configuration.
type ChannelConfigGetter interface {
	GetChannelConfig(cid string) channelconfig.Resources
}

// TokenCapabilityChecker implements CapabilityChecker interface.
type TokenCapabilityChecker struct {
	ChannelConfigGetter ChannelConfigGetter
}

func (c *TokenCapabilityChecker) FabToken(channelId string) (bool, error) {
	channelConfig := c.ChannelConfigGetter.GetChannelConfig(channelId)
	if channelConfig == nil {
		// no channelConfig is found, most likely the channel does not exist
		return false, errors.Errorf("no channel config found for channel %s", channelId)
	}

	ac, ok := channelConfig.ApplicationConfig()
	if !ok {
		return false, errors.Errorf("no application config found for channel %s", channelId)
	}
	return ac.Capabilities().FabToken(), nil
}
