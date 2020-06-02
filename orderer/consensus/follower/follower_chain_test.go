/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package follower_test

import (
	"github.com/hyperledger/fabric/orderer/common/types"
	"testing"

	"github.com/hyperledger/fabric/orderer/consensus/follower"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

//TODO skeleton

func TestFollowerChain(t *testing.T) {
	err := errors.New("bar")
	chain := &follower.Chain{Err: err}

	assert.Equal(t, err, chain.Order(nil, 0))
	assert.Equal(t, err, chain.Configure(nil, 0))
	assert.Equal(t, err, chain.WaitReady())
	assert.NotPanics(t, chain.Start)
	assert.NotPanics(t, chain.Halt)
	_, open := <-chain.Errored()
	assert.False(t, open)

	cRel, status := chain.StatusReport()
	assert.Equal(t, types.ClusterRelationFollower, cRel)
	assert.True(t, status == types.StatusActive || status == types.StatusOnBoarding)
}
