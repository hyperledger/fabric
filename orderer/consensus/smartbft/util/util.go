/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package util

import (
	"time"

	"github.com/hyperledger-labs/SmartBFT/pkg/types"
	"github.com/hyperledger/fabric-protos-go-apiv2/orderer/smartbft"
	"github.com/pkg/errors"
)

func ConfigFromMetadataOptions(selfID uint64, options *smartbft.Options) (types.Configuration, error) {
	var err error

	config := types.DefaultConfig
	config.SelfID = selfID

	if options == nil {
		return config, errors.New("config metadata options field is nil")
	}

	config.RequestBatchMaxCount = options.RequestBatchMaxCount
	config.RequestBatchMaxBytes = options.RequestBatchMaxBytes
	if config.RequestBatchMaxInterval, err = time.ParseDuration(options.RequestBatchMaxInterval); err != nil {
		return config, errors.Wrap(err, "bad config metadata option RequestBatchMaxInterval")
	}
	config.IncomingMessageBufferSize = options.IncomingMessageBufferSize
	config.RequestPoolSize = options.RequestPoolSize
	if config.RequestForwardTimeout, err = time.ParseDuration(options.RequestForwardTimeout); err != nil {
		return config, errors.Wrap(err, "bad config metadata option RequestForwardTimeout")
	}
	if config.RequestComplainTimeout, err = time.ParseDuration(options.RequestComplainTimeout); err != nil {
		return config, errors.Wrap(err, "bad config metadata option RequestComplainTimeout")
	}
	if config.RequestAutoRemoveTimeout, err = time.ParseDuration(options.RequestAutoRemoveTimeout); err != nil {
		return config, errors.Wrap(err, "bad config metadata option RequestAutoRemoveTimeout")
	}
	if config.ViewChangeResendInterval, err = time.ParseDuration(options.ViewChangeResendInterval); err != nil {
		return config, errors.Wrap(err, "bad config metadata option ViewChangeResendInterval")
	}
	if config.ViewChangeTimeout, err = time.ParseDuration(options.ViewChangeTimeout); err != nil {
		return config, errors.Wrap(err, "bad config metadata option ViewChangeTimeout")
	}
	if config.LeaderHeartbeatTimeout, err = time.ParseDuration(options.LeaderHeartbeatTimeout); err != nil {
		return config, errors.Wrap(err, "bad config metadata option LeaderHeartbeatTimeout")
	}
	config.LeaderHeartbeatCount = options.LeaderHeartbeatCount
	if config.CollectTimeout, err = time.ParseDuration(options.CollectTimeout); err != nil {
		return config, errors.Wrap(err, "bad config metadata option CollectTimeout")
	}
	config.SyncOnStart = options.SyncOnStart
	config.SpeedUpViewChange = options.SpeedUpViewChange

	if options.LeaderRotation != smartbft.Options_ROTATION_ON {
		config.LeaderRotation = false
		config.DecisionsPerLeader = 0
	} else {
		config.LeaderRotation = true
		config.DecisionsPerLeader = options.DecisionsPerLeader
	}

	if err = config.Validate(); err != nil {
		return config, errors.Wrap(err, "config validation failed")
	}

	if options.RequestMaxBytes == 0 {
		config.RequestMaxBytes = config.RequestBatchMaxBytes
	} else {
		config.RequestMaxBytes = options.RequestMaxBytes
	}

	return config, nil
}
