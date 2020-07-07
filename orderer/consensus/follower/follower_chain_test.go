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
	"github.com/hyperledger/fabric/orderer/common/types"
	"github.com/hyperledger/fabric/orderer/consensus/follower"
	"github.com/hyperledger/fabric/orderer/consensus/follower/mocks"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//TODO skeleton

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
	mockSupport := &mocks.Support{}
	mockSupport.On("ChannelID").Return("my-channel")
	mockSupport.On("Height").Return(uint64(5))
	options := follower.Options{Logger: testLogger, Cert: []byte{1, 2, 3, 4}}

	t.Run("with join block, not in channel", func(t *testing.T) {
		chain, err := follower.NewChain(mockSupport, joinBlockAppRaft, options, nil, nil, nil, iAmNotInChannel)
		assert.NoError(t, err)
		err = chain.Order(nil, 0)
		assert.EqualError(t, err, "orderer is a follower of channel my-channel")
		err = chain.Configure(nil, 0)
		assert.EqualError(t, err, "orderer is a follower of channel my-channel")
		err = chain.WaitReady()
		assert.EqualError(t, err, "orderer is a follower of channel my-channel")
		_, open := <-chain.Errored()
		assert.False(t, open)

		cRel, status := chain.StatusReport()
		assert.Equal(t, types.ClusterRelationFollower, cRel)
		assert.Equal(t, types.StatusOnBoarding, status)
	})

	t.Run("with join block, in channel", func(t *testing.T) {
		chain, err := follower.NewChain(mockSupport, joinBlockAppRaft, options, nil, nil, nil, iAmInChannel)
		assert.NoError(t, err)

		cRel, status := chain.StatusReport()
		assert.Equal(t, types.ClusterRelationMember, cRel)
		assert.Equal(t, types.StatusOnBoarding, status)

		assert.NotPanics(t, chain.Start)
		assert.NotPanics(t, chain.Start)
		assert.NotPanics(t, chain.Halt)
		assert.NotPanics(t, chain.Halt)
		assert.NotPanics(t, chain.Start)
	})

	t.Run("bad join block", func(t *testing.T) {
		chain, err := follower.NewChain(mockSupport, &common.Block{}, options, nil, nil, nil, nil)
		assert.EqualError(t, err, "block header is nil")
		assert.Nil(t, chain)
		chain, err = follower.NewChain(mockSupport, &common.Block{Header: &common.BlockHeader{}}, options, nil, nil, nil, nil)
		assert.EqualError(t, err, "block data is nil")
		assert.Nil(t, chain)
	})

	t.Run("without join block", func(t *testing.T) {
		chain, err := follower.NewChain(mockSupport, nil, options, nil, nil, nil, nil)
		assert.NoError(t, err)

		cRel, status := chain.StatusReport()
		assert.Equal(t, types.ClusterRelationFollower, cRel)
		assert.True(t, status == types.StatusActive)
	})
}
