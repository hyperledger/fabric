/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLedger(t *testing.T) {
	var info *BlockchainInfo
	info = nil
	assert.Equal(t, uint64(0), info.GetHeight())
	assert.Nil(t, info.GetCurrentBlockHash())
	assert.Nil(t, info.GetPreviousBlockHash())
	info = &BlockchainInfo{
		Height:            uint64(1),
		CurrentBlockHash:  []byte("blockhash"),
		PreviousBlockHash: []byte("previoushash"),
	}
	assert.Equal(t, uint64(1), info.GetHeight())
	assert.NotNil(t, info.GetCurrentBlockHash())
	assert.NotNil(t, info.GetPreviousBlockHash())
	info.Reset()
	assert.Equal(t, uint64(0), info.GetHeight())
	_ = info.String()
	_, _ = info.Descriptor()
	info.ProtoMessage()
}
