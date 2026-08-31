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
		hash:   protoutil.BlockHeaderHash(first.GetHeader()),
		number: first.GetHeader().GetNumber(),
		logger: flogging.NewFabricLogger(zap.NewNop()),
	}

	second := bc.createNextBlock([]*cb.Envelope{{Payload: []byte("some other bytes")}})
	require.Equal(t, first.GetHeader().GetNumber()+1, second.GetHeader().GetNumber())
	require.Equal(t, protoutil.ComputeBlockDataHash(second.GetData()), second.GetHeader().GetDataHash())
	require.Equal(t, protoutil.BlockHeaderHash(first.GetHeader()), second.GetHeader().GetPreviousHash())

	third := bc.createNextBlock([]*cb.Envelope{{Payload: []byte("some other bytes")}})
	require.Equal(t, second.GetHeader().GetNumber()+1, third.GetHeader().GetNumber())
	require.Equal(t, protoutil.ComputeBlockDataHash(third.GetData()), third.GetHeader().GetDataHash())
	require.Equal(t, protoutil.BlockHeaderHash(second.GetHeader()), third.GetHeader().GetPreviousHash())
}
