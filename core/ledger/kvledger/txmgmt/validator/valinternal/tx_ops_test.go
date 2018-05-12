/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package valinternal

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTxOps(t *testing.T) {
	assert := assert.New(t)

	txops := txOps{}
	key1 := compositeKey{"ns1", "", "key1"}
	key2 := compositeKey{"ns1", "coll2", "key2"}
	key3 := compositeKey{"ns1", "coll3", "key3"}
	key4 := compositeKey{"ns1", "coll4", "key4"}

	txops.upsert(key1, []byte("key1-value1"))
	assert.True(txops[key1].isOnlyUpsert())

	txops.upsert(key2, []byte("key2-value2"))
	assert.True(txops[key2].isOnlyUpsert())
	txops.metadataUpdate(key2, []byte("key2-metadata"))
	assert.False(txops[key2].isOnlyUpsert())
	assert.True(txops[key2].isUpsertAndMetadataUpdate())

	txops.upsert(key3, []byte("key3-value"))
	assert.True(txops[key3].isOnlyUpsert())
	txops.metadataDelete(key3)
	assert.False(txops[key3].isOnlyUpsert())
	assert.True(txops[key3].isUpsertAndMetadataUpdate())

	txops.delete(key4)
	assert.True(txops[key4].isDelete())
}
