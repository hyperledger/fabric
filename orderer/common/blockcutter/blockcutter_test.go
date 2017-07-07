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

	logging "github.com/op/go-logging"
	"github.com/stretchr/testify/assert"
)

func init() {
	logging.SetLevel(logging.DEBUG, "")
}

var goodTx = &cb.Envelope{Payload: []byte("GOOD")}
var goodTxLarge = &cb.Envelope{Payload: []byte("GOOD"), Signature: make([]byte, 1000)}

func TestNormalBatch(t *testing.T) {
	maxMessageCount := uint32(2)
	absoluteMaxBytes := uint32(1000)
	preferredMaxBytes := uint32(100)
	r := NewReceiverImpl(&mockconfig.Orderer{BatchSizeVal: &ab.BatchSize{MaxMessageCount: maxMessageCount, AbsoluteMaxBytes: absoluteMaxBytes, PreferredMaxBytes: preferredMaxBytes}})

	batches, ok := r.Ordered(goodTx)
	assert.Nil(t, batches, "Should not have created batch")
	assert.True(t, ok, "Should have enqueued message into batch")

	batches, ok = r.Ordered(goodTx)
	assert.NotNil(t, batches, "Should have created batch")
	assert.True(t, ok, "Should have enqueued second message into batch")
}

func TestBatchSizePreferredMaxBytesOverflow(t *testing.T) {
	goodTxBytes := messageSizeBytes(goodTx)

	// set preferred max bytes such that 10 goodTx will not fit
	preferredMaxBytes := goodTxBytes*10 - 1

	// set message count > 9
	maxMessageCount := uint32(20)

	r := NewReceiverImpl(&mockconfig.Orderer{BatchSizeVal: &ab.BatchSize{MaxMessageCount: maxMessageCount, AbsoluteMaxBytes: preferredMaxBytes * 2, PreferredMaxBytes: preferredMaxBytes}})

	// enqueue 9 messages
	for i := 0; i < 9; i++ {
		batches, ok := r.Ordered(goodTx)
		assert.Nil(t, batches, "Should not have created batch")
		assert.True(t, ok, "Should have enqueued message into batch")
	}

	// next message should create batch
	batches, ok := r.Ordered(goodTx)
	assert.NotNil(t, batches, "Should have created batch")
	assert.True(t, ok, "Should have enqueued message into batch")
	assert.Len(t, batches, 1, "Should have created one batch")
	assert.Len(t, batches[0], 9, "Should have had nine normal tx in the batch")

	// force a batch cut
	messageBatch := r.Cut()
	assert.NotNil(t, batches, "Should have created batch")
	assert.Len(t, messageBatch, 1, "Should have had one tx in the batch")
}

func TestBatchSizePreferredMaxBytesOverflowNoPending(t *testing.T) {
	goodTxLargeBytes := messageSizeBytes(goodTxLarge)

	// set preferred max bytes such that 1 goodTxLarge will not fit
	preferredMaxBytes := goodTxLargeBytes - 1

	// set message count > 1
	maxMessageCount := uint32(20)

	r := NewReceiverImpl(&mockconfig.Orderer{BatchSizeVal: &ab.BatchSize{MaxMessageCount: maxMessageCount, AbsoluteMaxBytes: preferredMaxBytes * 3, PreferredMaxBytes: preferredMaxBytes}})

	// submit large message
	batches, ok := r.Ordered(goodTxLarge)
	assert.NotNil(t, batches, "Should have created batch")
	assert.True(t, ok, "Should have enqueued message into batch")
	assert.Len(t, batches, 1, "Should have created one batch")
	assert.Len(t, batches[0], 1, "Should have had one normal tx in the batch")
}
