/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package etcdraft

import (
	"testing"

	"github.com/hyperledger/fabric/common/flogging"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func getSeedBlock() *cb.Block {
	seedBlock := cb.NewBlock(0, []byte("firsthash"))
	seedBlock.Data.Data = [][]byte{[]byte("somebytes")}
	return seedBlock
}

func TestCreateNextBlock(t *testing.T) {
	first := cb.NewBlock(0, []byte("firsthash"))
	bc := &blockCreator{
		hash:   first.Header.Hash(),
		number: first.Header.Number,
		logger: flogging.NewFabricLogger(zap.NewNop()),
	}

	second := bc.createNextBlock([]*cb.Envelope{{Payload: []byte("some other bytes")}})
	assert.Equal(t, first.Header.Number+1, second.Header.Number)
	assert.Equal(t, second.Data.Hash(), second.Header.DataHash)
	assert.Equal(t, first.Header.Hash(), second.Header.PreviousHash)

	third := bc.createNextBlock([]*cb.Envelope{{Payload: []byte("some other bytes")}})
	assert.Equal(t, second.Header.Number+1, third.Header.Number)
	assert.Equal(t, third.Data.Hash(), third.Header.DataHash)
	assert.Equal(t, second.Header.Hash(), third.Header.PreviousHash)
}
