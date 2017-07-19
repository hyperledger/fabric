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

	batches, ok := r.Ordered(goodTx)
	assert.Nil(t, batches, "Should not have created batch")
	assert.True(t, ok, "Should have enqueued message into batch")

	batches, ok = r.Ordered(goodTx)
	assert.NotNil(t, batches, "Should have created batch")
	assert.True(t, ok, "Should have enqueued second message into batch")
}

func TestBadMessageInBatch(t *testing.T) {
	filters := getFilters()
	maxMessageCount := uint32(2)
	absoluteMaxBytes := uint32(1000)
	preferredMaxBytes := uint32(100)
	r := NewReceiverImpl(&mockconfig.Orderer{BatchSizeVal: &ab.BatchSize{MaxMessageCount: maxMessageCount, AbsoluteMaxBytes: absoluteMaxBytes, PreferredMaxBytes: preferredMaxBytes}}, filters)

	batches, ok := r.Ordered(badTx)
	assert.Nil(t, batches, "Should not have created batch")
	assert.False(t, ok, "Should not have enqueued bad message into batch")

	batches, ok = r.Ordered(goodTx)
	assert.Nil(t, batches, "Should not have created batch")
	assert.True(t, ok, "Should have enqueued good message into batch")

	batches, ok = r.Ordered(badTx)
	assert.Nil(t, batches, "Should not have created batch")
	assert.False(t, ok, "Should not have enqueued second bad message into batch")
}

func TestUnmatchedMessageInBatch(t *testing.T) {
	filters := getFilters()
	maxMessageCount := uint32(2)
	absoluteMaxBytes := uint32(1000)
	preferredMaxBytes := uint32(100)
	r := NewReceiverImpl(&mockconfig.Orderer{BatchSizeVal: &ab.BatchSize{MaxMessageCount: maxMessageCount, AbsoluteMaxBytes: absoluteMaxBytes, PreferredMaxBytes: preferredMaxBytes}}, filters)

	batches, ok := r.Ordered(unmatchedTx)
	assert.Nil(t, batches, "Should not have created batch")
	assert.False(t, ok, "Should not have enqueued unmatched message into batch")

	batches, ok = r.Ordered(goodTx)
	assert.Nil(t, batches, "Should not have created batch")
	assert.True(t, ok, "Should have enqueued good message into batch")

	batches, ok = r.Ordered(unmatchedTx)
	assert.Nil(t, batches, "Should not have created batch from unmatched message")
	assert.False(t, ok, "Should not have enqueued second bad message into batch")
}

func TestIsolatedEmptyBatch(t *testing.T) {
	filters := getFilters()
	maxMessageCount := uint32(2)
	absoluteMaxBytes := uint32(1000)
	preferredMaxBytes := uint32(100)
	r := NewReceiverImpl(&mockconfig.Orderer{BatchSizeVal: &ab.BatchSize{MaxMessageCount: maxMessageCount, AbsoluteMaxBytes: absoluteMaxBytes, PreferredMaxBytes: preferredMaxBytes}}, filters)

	assert.Panics(t, func() { r.Ordered(isolatedTx) }, "Should not have handled an isolated by committer message")
}

func TestIsolatedPartialBatch(t *testing.T) {
	filters := getFilters()
	maxMessageCount := uint32(2)
	absoluteMaxBytes := uint32(1000)
	preferredMaxBytes := uint32(100)
	r := NewReceiverImpl(&mockconfig.Orderer{BatchSizeVal: &ab.BatchSize{MaxMessageCount: maxMessageCount, AbsoluteMaxBytes: absoluteMaxBytes, PreferredMaxBytes: preferredMaxBytes}}, filters)

	batches, ok := r.Ordered(goodTx)
	assert.Nil(t, batches, "Should not have created batch")
	assert.True(t, ok, "Should have enqueued good message into batch")

	assert.Panics(t, func() { r.Ordered(isolatedTx) }, "Should not have handled an isolated by committer message")
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
	filters := getFilters()

	goodTxLargeBytes := messageSizeBytes(goodTxLarge)

	// set preferred max bytes such that 1 goodTxLarge will not fit
	preferredMaxBytes := goodTxLargeBytes - 1

	// set message count > 1
	maxMessageCount := uint32(20)

	r := NewReceiverImpl(&mockconfig.Orderer{BatchSizeVal: &ab.BatchSize{MaxMessageCount: maxMessageCount, AbsoluteMaxBytes: preferredMaxBytes * 3, PreferredMaxBytes: preferredMaxBytes}}, filters)

	// submit large message
	batches, ok := r.Ordered(goodTxLarge)
	assert.NotNil(t, batches, "Should have created batch")
	assert.True(t, ok, "Should have enqueued message into batch")
	assert.Len(t, batches, 1, "Should have created one batch")
	assert.Len(t, batches[0], 1, "Should have had one normal tx in the batch")
}
