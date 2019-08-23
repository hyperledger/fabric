/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package blkstorage

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIndexConfig(t *testing.T) {
	ic := &IndexConfig{
		AttrsToIndex: []IndexableAttr{
			IndexableAttrBlockNum,
			IndexableAttrTxID,
		},
	}

	assert := assert.New(t)
	assert.True(ic.Contains(IndexableAttrBlockNum))
	assert.True(ic.Contains(IndexableAttrTxID))
	assert.False(ic.Contains(IndexableAttrBlockNumTranNum))
}
