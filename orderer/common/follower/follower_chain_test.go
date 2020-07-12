/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package follower_test

import (
	"testing"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/orderer/common/follower"
	"github.com/hyperledger/fabric/orderer/common/follower/mocks"
	"github.com/hyperledger/fabric/orderer/common/types"
	"github.com/hyperledger/fabric/orderer/consensus"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

//TODO skeleton

//go:generate counterfeiter -o mocks/cluster_consenter.go -fake-name ClusterConsenter . clusterConsenter
type clusterConsenter interface {
	consensus.ClusterConsenter
}

var testLogger = flogging.MustGetLogger("follower.test")

var iAmNotInChannel = func(configBlock *common.Block) error {
	return errors.New("not in channel")
}

var iAmInChannel = func(configBlock *common.Block) error {
	return nil
}

func TestFollowerNewChain(t *testing.T) {
	tlsCA, _ := tlsgen.NewCA()
	channelID := "my-raft-channel"
	joinBlockAppRaft := generateJoinBlock(t, tlsCA, channelID, 10)
	require.NotNil(t, joinBlockAppRaft)

	options := follower.Options{Logger: testLogger, Cert: []byte{1, 2, 3, 4}}
	channelPuller := &mocks.ChannelPuller{}
	createBlockPuller := func() (follower.ChannelPuller, error) { return channelPuller, nil }

	t.Run("with join block, not in channel", func(t *testing.T) {
		mockResources := &mocks.LedgerResources{}
		mockResources.On("ChannelID").Return("my-channel")
		mockResources.On("Height").Return(uint64(5))
		clusterConsenter := &mocks.ClusterConsenter{}
		clusterConsenter.IsChannelMemberReturns(false, nil)
		chain, err := follower.NewChain(mockResources, clusterConsenter, joinBlockAppRaft, options, createBlockPuller, nil, nil)
		require.NoError(t, err)

		cRel, status := chain.StatusReport()
		require.Equal(t, types.ClusterRelationFollower, cRel)
		require.Equal(t, types.StatusOnBoarding, status)
	})

	t.Run("with join block, in channel", func(t *testing.T) {
		mockResources := &mocks.LedgerResources{}
		mockResources.On("ChannelID").Return("my-channel")
		mockResources.On("Height").Return(uint64(5))
		clusterConsenter := &mocks.ClusterConsenter{}
		clusterConsenter.IsChannelMemberReturns(true, nil)
		chain, err := follower.NewChain(mockResources, clusterConsenter, joinBlockAppRaft, options, createBlockPuller, nil, nil)
		require.NoError(t, err)

		cRel, status := chain.StatusReport()
		require.Equal(t, types.ClusterRelationMember, cRel)
		require.Equal(t, types.StatusOnBoarding, status)

		require.NotPanics(t, chain.Start)
		require.NotPanics(t, chain.Start)
		require.NotPanics(t, chain.Halt)
		require.NotPanics(t, chain.Halt)
		require.NotPanics(t, chain.Start)
	})

	t.Run("bad join block", func(t *testing.T) {
		mockResources := &mocks.LedgerResources{}
		mockResources.On("ChannelID").Return("my-channel")
		chain, err := follower.NewChain(mockResources, nil, &common.Block{}, options, createBlockPuller, nil, nil)
		require.EqualError(t, err, "block header is nil")
		require.Nil(t, chain)
		chain, err = follower.NewChain(mockResources, nil, &common.Block{Header: &common.BlockHeader{}}, options, createBlockPuller, nil, nil)
		require.EqualError(t, err, "block data is nil")
		require.Nil(t, chain)
	})

	t.Run("without join block", func(t *testing.T) {
		mockResources := &mocks.LedgerResources{}
		mockResources.On("ChannelID").Return("my-channel")
		mockResources.On("Height").Return(uint64(5))
		clusterConsenter := &mocks.ClusterConsenter{}
		clusterConsenter.IsChannelMemberReturns(false, nil)
		chain, err := follower.NewChain(mockResources, clusterConsenter, nil, options, createBlockPuller, nil, nil)
		require.NoError(t, err)

		cRel, status := chain.StatusReport()
		require.Equal(t, types.ClusterRelationFollower, cRel)
		require.True(t, status == types.StatusActive)
	})
}
