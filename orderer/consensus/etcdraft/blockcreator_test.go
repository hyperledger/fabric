/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package etcdraft

import (
	"testing"

	"github.com/hyperledger/fabric-lib-go/common/flogging"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestCreateNextBlock(t *testing.T) {
	first := protoutil.NewBlock(0, []byte("firsthash"))
	bc := &blockCreator{
		hash:   protoutil.BlockHeaderHash(first.Header),
		number: first.Header.Number,
		logger: flogging.NewFabricLogger(zap.NewNop()),
	}

	second := bc.createNextBlock([]*cb.Envelope{{Payload: []byte("some other bytes")}})
	require.Equal(t, first.Header.Number+1, second.Header.Number)
	require.Equal(t, protoutil.ComputeBlockDataHash(second.Data), second.Header.DataHash)
	require.Equal(t, protoutil.BlockHeaderHash(first.Header), second.Header.PreviousHash)

	third := bc.createNextBlock([]*cb.Envelope{{Payload: []byte("some other bytes")}})
	require.Equal(t, second.Header.Number+1, third.Header.Number)
	require.Equal(t, protoutil.ComputeBlockDataHash(third.Data), third.Header.DataHash)
	require.Equal(t, protoutil.BlockHeaderHash(second.Header), third.Header.PreviousHash)
}
