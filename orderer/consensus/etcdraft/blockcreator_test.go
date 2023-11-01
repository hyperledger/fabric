/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package etcdraft

import (
	"testing"

	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/flogging"
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
	hash, _ := protoutil.BlockDataHash(second.Data)
	require.Equal(t, first.Header.Number+1, second.Header.Number)
	require.Equal(t, hash, second.Header.DataHash)
	require.Equal(t, protoutil.BlockHeaderHash(first.Header), second.Header.PreviousHash)

	third := bc.createNextBlock([]*cb.Envelope{{Payload: []byte("some other bytes")}})
	hash, _ = protoutil.BlockDataHash(third.Data)
	require.Equal(t, second.Header.Number+1, third.Header.Number)
	require.Equal(t, hash, third.Header.DataHash)
	require.Equal(t, protoutil.BlockHeaderHash(second.Header), third.Header.PreviousHash)
}
