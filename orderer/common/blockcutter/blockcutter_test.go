/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

                 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package blockcutter

import (
	"testing"

	mockconfig "github.com/hyperledger/fabric/common/mocks/config"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/stretchr/testify/assert"
)

func init() {
	flogging.SetModuleLevel(pkgLogID, "DEBUG")
}

var tx = &cb.Envelope{Payload: []byte("GOOD")}
var txLarge = &cb.Envelope{Payload: []byte("GOOD"), Signature: make([]byte, 1000)}

func TestNormalBatch(t *testing.T) {
	maxMessageCount := uint32(2)
	absoluteMaxBytes := uint32(1000)
	preferredMaxBytes := uint32(100)
	r := NewReceiverImpl(&mockconfig.Orderer{BatchSizeVal: &ab.BatchSize{MaxMessageCount: maxMessageCount, AbsoluteMaxBytes: absoluteMaxBytes, PreferredMaxBytes: preferredMaxBytes}})

	batches, pending := r.Ordered(tx)
	assert.Nil(t, batches, "Should not have created batch")
	assert.True(t, pending, "Should have message pending in the receiver")

	batches, pending = r.Ordered(tx)
	assert.NotNil(t, batches, "Should have created batch")
	assert.False(t, pending, "Should not have message pending in the receiver")
}

func TestBatchSizePreferredMaxBytesOverflow(t *testing.T) {
	txBytes := messageSizeBytes(tx)

	// set preferred max bytes such that 10 tx will not fit
	preferredMaxBytes := txBytes*10 - 1

	// set message count > 9
	maxMessageCount := uint32(20)

	r := NewReceiverImpl(&mockconfig.Orderer{BatchSizeVal: &ab.BatchSize{MaxMessageCount: maxMessageCount, AbsoluteMaxBytes: preferredMaxBytes * 2, PreferredMaxBytes: preferredMaxBytes}})

	// enqueue 9 messages
	for i := 0; i < 9; i++ {
		batches, pending := r.Ordered(tx)
		assert.Nil(t, batches, "Should not have created batch")
		assert.True(t, pending, "Should have enqueued message into batch")
	}

	// next message should create batch
	batches, pending := r.Ordered(tx)
	assert.NotNil(t, batches, "Should have created batch")
	assert.True(t, pending, "Should still have message pending")
	assert.Len(t, batches, 1, "Should have created one batch")
	assert.Len(t, batches[0], 9, "Should have had nine normal tx in the batch")

	// force a batch cut
	messageBatch := r.Cut()
	assert.NotNil(t, batches, "Should have created batch")
	assert.Len(t, messageBatch, 1, "Should have had one tx in the batch")
}

func TestBatchSizePreferredMaxBytesOverflowNoPending(t *testing.T) {
	goodTxLargeBytes := messageSizeBytes(txLarge)

	// set preferred max bytes such that 1 txLarge will not fit
	preferredMaxBytes := goodTxLargeBytes - 1

	// set message count > 1
	maxMessageCount := uint32(20)

	r := NewReceiverImpl(&mockconfig.Orderer{BatchSizeVal: &ab.BatchSize{MaxMessageCount: maxMessageCount, AbsoluteMaxBytes: preferredMaxBytes * 3, PreferredMaxBytes: preferredMaxBytes}})

	// submit normal message
	batches, pending := r.Ordered(tx)
	assert.Nil(t, batches, "Should not have created batch")
	assert.True(t, pending, "Should have enqueued message into batch")

	// submit large message
	batches, pending = r.Ordered(txLarge)
	assert.NotNil(t, batches, "Should have created batch")
	assert.False(t, pending, "Should not have pending messages in receiver")
	assert.Len(t, batches, 2, "Should have created two batches")
	for i, batch := range batches {
		assert.Len(t, batch, 1, "Should have had one normal tx in batch %d", i)
	}
}
