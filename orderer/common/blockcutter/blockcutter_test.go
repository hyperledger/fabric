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
	"bytes"
	"testing"

	mockconfig "github.com/hyperledger/fabric/common/mocks/config"
	"github.com/hyperledger/fabric/orderer/common/filter"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	logging "github.com/op/go-logging"
	"github.com/stretchr/testify/assert"
)

func init() {
	logging.SetLevel(logging.DEBUG, "")
}

type isolatedCommitter struct{}

func (ic isolatedCommitter) Isolated() bool { return true }

func (ic isolatedCommitter) Commit() {}

type mockIsolatedFilter struct{}

func (mif *mockIsolatedFilter) Apply(msg *cb.Envelope) (filter.Action, filter.Committer) {
	if bytes.Equal(msg.Payload, isolatedTx.Payload) {
		return filter.Accept, isolatedCommitter{}
	}
	return filter.Forward, nil
}

type mockRejectFilter struct{}

func (mrf mockRejectFilter) Apply(message *cb.Envelope) (filter.Action, filter.Committer) {
	if bytes.Equal(message.Payload, badTx.Payload) {
		return filter.Reject, nil
	}
	return filter.Forward, nil
}

type mockAcceptFilter struct{}

func (mrf mockAcceptFilter) Apply(message *cb.Envelope) (filter.Action, filter.Committer) {
	if bytes.Equal(message.Payload, goodTx.Payload) {
		return filter.Accept, filter.NoopCommitter
	}
	return filter.Forward, nil
}

func getFilters() *filter.RuleSet {
	return filter.NewRuleSet([]filter.Rule{
		&mockIsolatedFilter{},
		&mockRejectFilter{},
		&mockAcceptFilter{},
	})
}

var badTx = &cb.Envelope{Payload: []byte("BAD")}
var goodTx = &cb.Envelope{Payload: []byte("GOOD")}
var goodTxLarge = &cb.Envelope{Payload: []byte("GOOD"), Signature: make([]byte, 1000)}
var isolatedTx = &cb.Envelope{Payload: []byte("ISOLATED")}
var unmatchedTx = &cb.Envelope{Payload: []byte("UNMATCHED")}

func TestNormalBatch(t *testing.T) {
	filters := getFilters()
	maxMessageCount := uint32(2)
	absoluteMaxBytes := uint32(1000)
	preferredMaxBytes := uint32(100)
	r := NewReceiverImpl(&mockconfig.Orderer{BatchSizeVal: &ab.BatchSize{MaxMessageCount: maxMessageCount, AbsoluteMaxBytes: absoluteMaxBytes, PreferredMaxBytes: preferredMaxBytes}}, filters)

	batches, committers, ok, pending := r.Ordered(goodTx)

	assert.Nil(t, batches, "Should not have created batch")
	assert.Nil(t, committers, "Should not have created batch")
	assert.True(t, ok, "Should have enqueued message into batch")
	assert.True(t, pending, "Should have pending messages")

	batches, committers, ok, pending = r.Ordered(goodTx)

	assert.Len(t, batches, 1, "Should have created 1 message batch, got %d", len(batches))
	assert.Len(t, committers, 1, "Should have created 1 committer batch, got %d", len(committers))
	assert.True(t, ok, "Should have enqueued message into batch")
	assert.False(t, pending, "Should not have pending messages")
}

func TestBadMessageInBatch(t *testing.T) {
	filters := getFilters()
	maxMessageCount := uint32(2)
	absoluteMaxBytes := uint32(1000)
	preferredMaxBytes := uint32(100)
	r := NewReceiverImpl(&mockconfig.Orderer{BatchSizeVal: &ab.BatchSize{MaxMessageCount: maxMessageCount, AbsoluteMaxBytes: absoluteMaxBytes, PreferredMaxBytes: preferredMaxBytes}}, filters)

	batches, committers, ok, _ := r.Ordered(badTx)

	assert.Nil(t, batches, "Should not have created batch")
	assert.Nil(t, committers, "Should not have created batch")
	assert.False(t, ok, "Should not have enqueued bad message into batch")

	batches, committers, ok, pending := r.Ordered(goodTx)

	assert.Nil(t, batches, "Should not have created batch")
	assert.Nil(t, committers, "Should not have created batch")
	assert.True(t, ok, "Should have enqueued good message into batch")
	assert.True(t, pending, "Should have pending messages")

	batches, committers, ok, _ = r.Ordered(badTx)

	assert.Nil(t, batches, "Should not have created batch")
	assert.Nil(t, committers, "Should not have created batch")
	assert.False(t, ok, "Should not have enqueued second bad message into batch")
}

func TestUnmatchedMessageInBatch(t *testing.T) {
	filters := getFilters()
	maxMessageCount := uint32(2)
	absoluteMaxBytes := uint32(1000)
	preferredMaxBytes := uint32(100)
	r := NewReceiverImpl(&mockconfig.Orderer{BatchSizeVal: &ab.BatchSize{MaxMessageCount: maxMessageCount, AbsoluteMaxBytes: absoluteMaxBytes, PreferredMaxBytes: preferredMaxBytes}}, filters)

	batches, committers, ok, _ := r.Ordered(unmatchedTx)

	assert.Nil(t, batches, "Should not have created batch")
	assert.Nil(t, committers, "Should not have created batch")
	assert.False(t, ok, "Should not have enqueued unmatched message into batch")

	batches, committers, ok, pending := r.Ordered(goodTx)

	assert.Nil(t, batches, "Should not have created batch")
	assert.Nil(t, committers, "Should not have created batch")
	assert.True(t, ok, "Should have enqueued good message into batch")
	assert.True(t, pending, "Should have pending messages")

	batches, committers, ok, _ = r.Ordered(unmatchedTx)

	assert.Nil(t, batches, "Should not have created batch from unmatched message")
	assert.Nil(t, committers, "Should not have created batch")
	assert.False(t, ok, "Should not have enqueued second unmatched message into batch")
}

func TestIsolatedEmptyBatch(t *testing.T) {
	filters := getFilters()
	maxMessageCount := uint32(2)
	absoluteMaxBytes := uint32(1000)
	preferredMaxBytes := uint32(100)
	r := NewReceiverImpl(&mockconfig.Orderer{BatchSizeVal: &ab.BatchSize{MaxMessageCount: maxMessageCount, AbsoluteMaxBytes: absoluteMaxBytes, PreferredMaxBytes: preferredMaxBytes}}, filters)

	batches, committers, ok, pending := r.Ordered(isolatedTx)

	assert.Len(t, batches, 1, "Should created 1 new message batch, got %d", len(batches))
	assert.Len(t, batches[0], 1, "Should have had one isolatedTx in the message batch, got %d", len(batches[0]))
	assert.Len(t, committers, 1, "Should created 1 new committer batch, got %d", len(committers))
	assert.Len(t, committers[0], 1, "Should have had one isolatedTx in the committer batch, got %d", len(committers[0]))
	assert.True(t, ok, "Should have enqueued isolated message into batch")
	assert.False(t, pending, "Should not have pending messages")
	assert.Equal(t, isolatedTx.Payload, batches[0][0].Payload, "Should have had the isolated tx in the first batch")
}

func TestIsolatedPartialBatch(t *testing.T) {
	filters := getFilters()
	maxMessageCount := uint32(2)
	absoluteMaxBytes := uint32(1000)
	preferredMaxBytes := uint32(100)
	r := NewReceiverImpl(&mockconfig.Orderer{BatchSizeVal: &ab.BatchSize{MaxMessageCount: maxMessageCount, AbsoluteMaxBytes: absoluteMaxBytes, PreferredMaxBytes: preferredMaxBytes}}, filters)

	batches, committers, ok, pending := r.Ordered(goodTx)

	assert.Nil(t, batches, "Should not have created batch")
	assert.Nil(t, committers, "Should not have created batch")
	assert.True(t, ok, "Should have enqueued message into batch")
	assert.True(t, pending, "Should have pending messages")

	batches, committers, ok, pending = r.Ordered(isolatedTx)

	assert.Len(t, batches, 2, "Should created 2 new message batch, got %d", len(batches))
	assert.Len(t, batches[0], 1, "Should have had one goodTx in the first message batch, got %d", len(batches[0]))
	assert.Len(t, batches[1], 1, "Should have had one isolatedTx in the second message batch, got %d", len(batches[1]))
	assert.Len(t, committers, 2, "Should created 2 new committer batch, got %d", len(committers))
	assert.Len(t, committers[0], 1, "Should have had 1 committer in the first committer batch, got %d", len(committers[0]))
	assert.Len(t, committers[1], 1, "Should have had 1 committer in the second committer batch, got %d", len(committers[1]))
	assert.True(t, ok, "Should have enqueued isolated message into batch")
	assert.False(t, pending, "Should not have pending messages")
	assert.Equal(t, goodTx.Payload, batches[0][0].Payload, "Should have had the good tx in the first batch")
	assert.Equal(t, isolatedTx.Payload, batches[1][0].Payload, "Should have had the isolated tx in the second batch")
}

func TestBatchSizePreferredMaxBytesOverflow(t *testing.T) {
	filters := getFilters()

	goodTxBytes := messageSizeBytes(goodTx)

	// set preferred max bytes such that 10 goodTx will not fit
	preferredMaxBytes := goodTxBytes*10 - 1

	// set message count > 9
	maxMessageCount := uint32(20)

	r := NewReceiverImpl(&mockconfig.Orderer{BatchSizeVal: &ab.BatchSize{MaxMessageCount: maxMessageCount, AbsoluteMaxBytes: preferredMaxBytes * 2, PreferredMaxBytes: preferredMaxBytes}}, filters)

	// enqueue 9 messages
	for i := 0; i < 9; i++ {
		batches, committers, ok, pending := r.Ordered(goodTx)

		assert.Nil(t, batches, "Should not have created batch")
		assert.Nil(t, committers, "Should not have created batch")
		assert.True(t, ok, "Should have enqueued message into batch")
		assert.True(t, pending, "Should have pending messages")
	}

	// next message should create batch
	batches, committers, ok, pending := r.Ordered(goodTx)

	assert.Len(t, batches, 1, "Should have created 1 message batch, got %d", len(batches))
	assert.Len(t, batches[0], 9, "Should have had nine normal tx in the message batch, got %d", len(batches[0]))
	assert.Len(t, committers, 1, "Should have created 1 committer batch, got %d", len(committers))
	assert.Len(t, committers[0], 9, "Should have had nine committers in the committer batch, got %d", len(committers[0]))
	assert.True(t, ok, "Should have enqueued message into batch")
	assert.True(t, pending, "Should still have pending messages")

	// force a batch cut
	messageBatch, committerBatch := r.Cut()

	assert.NotNil(t, messageBatch, "Should have created message batch")
	assert.Len(t, messageBatch, 1, "Should have had 1 tx in the batch, got %d", len(messageBatch))
	assert.NotNil(t, committerBatch, "Should have created committer batch")
	assert.Len(t, committerBatch, 1, "Should have had 1 committer in the committer batch, got %d", len(committerBatch))
}

func TestBatchSizePreferredMaxBytesOverflowNoPending(t *testing.T) {
	filters := getFilters()

	goodTxLargeBytes := messageSizeBytes(goodTxLarge)

	// set preferred max bytes such that 1 goodTxLarge will not fit
	preferredMaxBytes := goodTxLargeBytes - 1

	// set message count > 1
	maxMessageCount := uint32(20)

	r := NewReceiverImpl(&mockconfig.Orderer{BatchSizeVal: &ab.BatchSize{MaxMessageCount: maxMessageCount, AbsoluteMaxBytes: preferredMaxBytes * 3, PreferredMaxBytes: preferredMaxBytes}}, filters)

	// submit large message
	batches, committers, ok, pending := r.Ordered(goodTxLarge)

	assert.Len(t, batches, 1, "Should have created 1 message batch, got %d", len(batches))
	assert.Len(t, batches[0], 1, "Should have had 1 normal tx in the message batch, got %d", len(batches[0]))
	assert.Len(t, committers, 1, "Should have created 1 committer batch, got %d", len(committers))
	assert.Len(t, committers[0], 1, "Should have had 1 committer in the committer batch, got %d", len(committers[0]))
	assert.True(t, ok, "Should have enqueued message into batch")
	assert.False(t, pending, "Should not have pending messages")
}
