// Copyright IBM Corp. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0

package smartbft_test

import (
	"testing"

	"github.com/hyperledger/SmartBFT/pkg/types"
	"github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	smartbftprotos "github.com/hyperledger/fabric-protos-go-apiv2/orderer/smartbft"
	"github.com/hyperledger/fabric/orderer/consensus/smartbft"
	smartBFTMocks "github.com/hyperledger/fabric/orderer/consensus/smartbft/mocks"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
)

func validBFTOptions() *smartbftprotos.Options {
	return &smartbftprotos.Options{
		RequestBatchMaxCount:      types.DefaultConfig.RequestBatchMaxCount,
		RequestBatchMaxBytes:      types.DefaultConfig.RequestBatchMaxBytes,
		RequestBatchMaxInterval:   types.DefaultConfig.RequestBatchMaxInterval.String(),
		IncomingMessageBufferSize: types.DefaultConfig.IncomingMessageBufferSize,
		RequestPoolSize:           types.DefaultConfig.RequestPoolSize,
		RequestForwardTimeout:     types.DefaultConfig.RequestForwardTimeout.String(),
		RequestComplainTimeout:    types.DefaultConfig.RequestComplainTimeout.String(),
		RequestAutoRemoveTimeout:  types.DefaultConfig.RequestAutoRemoveTimeout.String(),
		ViewChangeResendInterval:  types.DefaultConfig.ViewChangeResendInterval.String(),
		ViewChangeTimeout:         types.DefaultConfig.ViewChangeTimeout.String(),
		LeaderHeartbeatTimeout:    types.DefaultConfig.LeaderHeartbeatTimeout.String(),
		LeaderHeartbeatCount:      types.DefaultConfig.LeaderHeartbeatCount,
		CollectTimeout:            types.DefaultConfig.CollectTimeout.String(),
		SyncOnStart:               types.DefaultConfig.SyncOnStart,
		SpeedUpViewChange:         types.DefaultConfig.SpeedUpViewChange,
		LeaderRotation:            smartbftprotos.Options_ROTATION_OFF,
	}
}

func bftOrdererConfig(consensusType string, options *smartbftprotos.Options, batchSize *orderer.BatchSize) *smartBFTMocks.OrdererConfig {
	oc := &smartBFTMocks.OrdererConfig{}
	oc.ConsensusTypeReturns(consensusType)
	oc.ConsensusMetadataReturns(protoutil.MarshalOrPanic(options))
	oc.BatchSizeReturns(batchSize)
	return oc
}

func validBatchSize() *orderer.BatchSize {
	return &orderer.BatchSize{
		MaxMessageCount:   100,
		AbsoluteMaxBytes:  10 * 1024 * 1024,
		PreferredMaxBytes: 2 * 1024 * 1024,
	}
}

func TestBFTChainValidateConsensusMetadata(t *testing.T) {
	chain := &smartbft.BFTChain{}
	oldConfig := bftOrdererConfig("BFT", validBFTOptions(), validBatchSize())

	t.Run("valid options", func(t *testing.T) {
		newConfig := bftOrdererConfig("BFT", validBFTOptions(), validBatchSize())
		require.NoError(t, chain.ValidateConsensusMetadata(oldConfig, newConfig, false))
	})

	t.Run("RequestBatchMaxCount is overridden by BatchSize", func(t *testing.T) {
		// The runtime replaces RequestBatchMaxCount and RequestBatchMaxBytes with the values from BatchSize,
		// so a zero value in the options alone is not an error.
		options := validBFTOptions()
		options.RequestBatchMaxCount = 0
		options.RequestBatchMaxBytes = 0
		newConfig := bftOrdererConfig("BFT", options, validBatchSize())
		require.NoError(t, chain.ValidateConsensusMetadata(oldConfig, newConfig, false))
	})

	for _, tc := range []struct {
		name        string
		mutate      func(*smartbftprotos.Options)
		expectedErr string
	}{
		{
			name:        "zero IncomingMessageBufferSize",
			mutate:      func(o *smartbftprotos.Options) { o.IncomingMessageBufferSize = 0 },
			expectedErr: "IncomingMessageBufferSize should be greater than zero",
		},
		{
			name:        "zero RequestPoolSize",
			mutate:      func(o *smartbftprotos.Options) { o.RequestPoolSize = 0 },
			expectedErr: "RequestPoolSize should be greater than zero",
		},
		{
			name:        "zero LeaderHeartbeatCount",
			mutate:      func(o *smartbftprotos.Options) { o.LeaderHeartbeatCount = 0 },
			expectedErr: "LeaderHeartbeatCount should be greater than zero",
		},
		{
			name:        "non-positive ViewChangeTimeout",
			mutate:      func(o *smartbftprotos.Options) { o.ViewChangeTimeout = "0s" },
			expectedErr: "ViewChangeTimeout should be greater than zero",
		},
		{
			name:        "unparsable duration",
			mutate:      func(o *smartbftprotos.Options) { o.CollectTimeout = "not-a-duration" },
			expectedErr: "bad config metadata option CollectTimeout",
		},
		{
			name: "RequestForwardTimeout bigger than RequestComplainTimeout",
			mutate: func(o *smartbftprotos.Options) {
				o.RequestForwardTimeout = "10m"
				o.RequestComplainTimeout = "1m"
			},
			expectedErr: "RequestForwardTimeout is bigger than RequestComplainTimeout",
		},
		{
			name: "leader rotation without DecisionsPerLeader",
			mutate: func(o *smartbftprotos.Options) {
				o.LeaderRotation = smartbftprotos.Options_ROTATION_ON
				o.DecisionsPerLeader = 0
			},
			expectedErr: "DecisionsPerLeader should be greater than zero when leader rotation is active",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			options := validBFTOptions()
			tc.mutate(options)
			newConfig := bftOrdererConfig("BFT", options, validBatchSize())
			err := chain.ValidateConsensusMetadata(oldConfig, newConfig, false)
			require.ErrorContains(t, err, tc.expectedErr)
		})
	}

	t.Run("invalid BatchSize", func(t *testing.T) {
		batchSize := validBatchSize()
		batchSize.MaxMessageCount = 0
		newConfig := bftOrdererConfig("BFT", validBFTOptions(), batchSize)
		err := chain.ValidateConsensusMetadata(oldConfig, newConfig, false)
		require.ErrorContains(t, err, "RequestBatchMaxCount should be greater than zero")
	})

	t.Run("malformed metadata", func(t *testing.T) {
		newConfig := &smartBFTMocks.OrdererConfig{}
		newConfig.ConsensusTypeReturns("BFT")
		newConfig.ConsensusMetadataReturns([]byte{0xff, 0xff, 0xff})
		newConfig.BatchSizeReturns(validBatchSize())
		err := chain.ValidateConsensusMetadata(oldConfig, newConfig, false)
		require.ErrorContains(t, err, "failed to unmarshal consensus metadata")
	})

	t.Run("consensus type is not BFT", func(t *testing.T) {
		newConfig := bftOrdererConfig("etcdraft", validBFTOptions(), validBatchSize())
		err := chain.ValidateConsensusMetadata(oldConfig, newConfig, false)
		require.ErrorContains(t, err, "unsupported consensus type: etcdraft")
	})

	t.Run("nil new config", func(t *testing.T) {
		err := chain.ValidateConsensusMetadata(oldConfig, nil, false)
		require.ErrorContains(t, err, "new orderer config is nil")
	})
}
