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

	config.RequestBatchMaxCount = options.GetRequestBatchMaxCount()
	config.RequestBatchMaxBytes = options.GetRequestBatchMaxBytes()
	if config.RequestBatchMaxInterval, err = time.ParseDuration(options.GetRequestBatchMaxInterval()); err != nil {
		return config, errors.Wrap(err, "bad config metadata option RequestBatchMaxInterval")
	}
	config.IncomingMessageBufferSize = options.GetIncomingMessageBufferSize()
	config.RequestPoolSize = options.GetRequestPoolSize()
	if config.RequestForwardTimeout, err = time.ParseDuration(options.GetRequestForwardTimeout()); err != nil {
		return config, errors.Wrap(err, "bad config metadata option RequestForwardTimeout")
	}
	if config.RequestComplainTimeout, err = time.ParseDuration(options.GetRequestComplainTimeout()); err != nil {
		return config, errors.Wrap(err, "bad config metadata option RequestComplainTimeout")
	}
	if config.RequestAutoRemoveTimeout, err = time.ParseDuration(options.GetRequestAutoRemoveTimeout()); err != nil {
		return config, errors.Wrap(err, "bad config metadata option RequestAutoRemoveTimeout")
	}
	if config.ViewChangeResendInterval, err = time.ParseDuration(options.GetViewChangeResendInterval()); err != nil {
		return config, errors.Wrap(err, "bad config metadata option ViewChangeResendInterval")
	}
	if config.ViewChangeTimeout, err = time.ParseDuration(options.GetViewChangeTimeout()); err != nil {
		return config, errors.Wrap(err, "bad config metadata option ViewChangeTimeout")
	}
	if config.LeaderHeartbeatTimeout, err = time.ParseDuration(options.GetLeaderHeartbeatTimeout()); err != nil {
		return config, errors.Wrap(err, "bad config metadata option LeaderHeartbeatTimeout")
	}
	config.LeaderHeartbeatCount = options.GetLeaderHeartbeatCount()
	if config.CollectTimeout, err = time.ParseDuration(options.GetCollectTimeout()); err != nil {
		return config, errors.Wrap(err, "bad config metadata option CollectTimeout")
	}
	config.SyncOnStart = options.GetSyncOnStart()
	config.SpeedUpViewChange = options.GetSpeedUpViewChange()

	if options.GetLeaderRotation() != smartbft.Options_ROTATION_ON {
		config.LeaderRotation = false
		config.DecisionsPerLeader = 0
	} else {
		config.LeaderRotation = true
		config.DecisionsPerLeader = options.GetDecisionsPerLeader()
	}

	if err = config.Validate(); err != nil {
		return config, errors.Wrap(err, "config validation failed")
	}

	if options.GetRequestMaxBytes() == 0 {
		config.RequestMaxBytes = config.RequestBatchMaxBytes
	} else {
		config.RequestMaxBytes = options.GetRequestMaxBytes()
	}

	return config, nil
}
