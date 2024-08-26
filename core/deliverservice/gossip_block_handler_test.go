/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package deliverservice

import (
	"testing"

	"github.com/hyperledger/fabric-lib-go/common/flogging"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric/common/deliverclient/blocksprovider/fake"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestGossipBlockHandler_HandleBlock(t *testing.T) {
	h := GossipBlockHandler{
		blockGossipDisabled: false,
		logger:              flogging.MustGetLogger("test.GossipBlockHandler"),
	}

	t.Run("error: cannot marshal block", func(t *testing.T) {
		h.gossip = &fake.GossipServiceAdapter{}
		err := h.HandleBlock("testchannel", nil)
		require.EqualError(t, err, "block from orderer could not be re-marshaled: proto: Marshal called with nil")
	})

	t.Run("error: cannot add payload", func(t *testing.T) {
		fakeGossip := &fake.GossipServiceAdapter{}
		fakeGossip.AddPayloadReturns(errors.New("oops"))
		h.gossip = fakeGossip
		err := h.HandleBlock("testchannel", &common.Block{Header: &common.BlockHeader{Number: 8}})
		require.EqualError(t, err, "could not add block as payload: oops")
	})

	t.Run("valid: gossip", func(t *testing.T) {
		fakeGossip := &fake.GossipServiceAdapter{}
		h.gossip = fakeGossip
		err := h.HandleBlock("testchannel", &common.Block{Header: &common.BlockHeader{Number: 8}})
		require.NoError(t, err)
		require.Equal(t, 1, fakeGossip.GossipCallCount())
	})

	t.Run("valid: no gossip", func(t *testing.T) {
		fakeGossip := &fake.GossipServiceAdapter{}
		h.gossip = fakeGossip
		h.blockGossipDisabled = true
		err := h.HandleBlock("testchannel", &common.Block{Header: &common.BlockHeader{Number: 8}})
		require.NoError(t, err)
		require.Equal(t, 0, fakeGossip.GossipCallCount())
	})
}
