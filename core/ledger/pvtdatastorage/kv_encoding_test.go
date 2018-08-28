/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtdatastorage

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDataKeyEncoding(t *testing.T) {
	dataKey1 := &dataKey{nsCollBlk: nsCollBlk{ns: "ns1", coll: "coll1", blkNum: 2}, txNum: 5}
	datakey2 := decodeDatakey(encodeDataKey(dataKey1))
	assert.Equal(t, dataKey1, datakey2)
}
