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

	"github.com/hyperledger/fabric/orderer/common/filter"
	mocksharedconfig "github.com/hyperledger/fabric/orderer/mocks/sharedconfig"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
)

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
var isolatedTx = &cb.Envelope{Payload: []byte("ISOLATED")}
var unmatchedTx = &cb.Envelope{Payload: []byte("UNMATCHED")}

func TestNormalBatch(t *testing.T) {
	filters := getFilters()
	maxMessageCount := uint32(2)
	r := NewReceiverImpl(&mocksharedconfig.Manager{BatchSizeVal: &ab.BatchSize{MaxMessageCount: maxMessageCount}}, filters)

	batches, committers, ok := r.Ordered(goodTx)

	if batches != nil || committers != nil {
		t.Fatalf("Should not have created batch")
	}

	if !ok {
		t.Fatalf("Should have enqueued message into batch")
	}

	batches, committers, ok = r.Ordered(goodTx)

	if batches == nil || committers == nil {
		t.Fatalf("Should have created batch")
	}

	if !ok {
		t.Fatalf("Should have enqueued second message into batch")
	}

}

func TestBadMessageInBatch(t *testing.T) {
	filters := getFilters()
	maxMessageCount := uint32(2)
	r := NewReceiverImpl(&mocksharedconfig.Manager{BatchSizeVal: &ab.BatchSize{MaxMessageCount: maxMessageCount}}, filters)

	batches, committers, ok := r.Ordered(badTx)

	if batches != nil || committers != nil {
		t.Fatalf("Should not have created batch")
	}

	if ok {
		t.Fatalf("Should not have enqueued bad message into batch")
	}

	batches, committers, ok = r.Ordered(goodTx)

	if batches != nil || committers != nil {
		t.Fatalf("Should not have created batch")
	}

	if !ok {
		t.Fatalf("Should have enqueued good message into batch")
	}

	batches, committers, ok = r.Ordered(badTx)

	if batches != nil || committers != nil {
		t.Fatalf("Should not have created batch")
	}

	if ok {
		t.Fatalf("Should not have enqueued second bad message into batch")
	}
}

func TestUnmatchedMessageInBatch(t *testing.T) {
	filters := getFilters()
	maxMessageCount := uint32(2)
	r := NewReceiverImpl(&mocksharedconfig.Manager{BatchSizeVal: &ab.BatchSize{MaxMessageCount: maxMessageCount}}, filters)

	batches, committers, ok := r.Ordered(unmatchedTx)

	if batches != nil || committers != nil {
		t.Fatalf("Should not have created batch")
	}

	if ok {
		t.Fatalf("Should not have enqueued unmatched message into batch")
	}

	batches, committers, ok = r.Ordered(goodTx)

	if batches != nil || committers != nil {
		t.Fatalf("Should not have created batch")
	}

	if !ok {
		t.Fatalf("Should have enqueued good message into batch")
	}

	batches, committers, ok = r.Ordered(unmatchedTx)

	if batches != nil || committers != nil {
		t.Fatalf("Should not have created batch from unmatched message")
	}

	if ok {
		t.Fatalf("Should not have enqueued second bad message into batch")
	}
}

func TestIsolatedEmptyBatch(t *testing.T) {
	filters := getFilters()
	maxMessageCount := uint32(2)
	r := NewReceiverImpl(&mocksharedconfig.Manager{BatchSizeVal: &ab.BatchSize{MaxMessageCount: maxMessageCount}}, filters)

	batches, committers, ok := r.Ordered(isolatedTx)

	if !ok {
		t.Fatalf("Should have enqueued isolated message")
	}

	if len(batches) != 1 || len(committers) != 1 {
		t.Fatalf("Should created new batch, got %d and %d", len(batches), len(committers))
	}

	if len(batches[0]) != 1 || len(committers[0]) != 1 {
		t.Fatalf("Should have had one isolatedTx in the second batch got %d and %d", len(batches[1]), len(committers[0]))
	}

	if !bytes.Equal(batches[0][0].Payload, isolatedTx.Payload) {
		t.Fatalf("Should have had the isolated tx in the first batch")
	}
}

func TestIsolatedPartialBatch(t *testing.T) {
	filters := getFilters()
	maxMessageCount := uint32(2)
	r := NewReceiverImpl(&mocksharedconfig.Manager{BatchSizeVal: &ab.BatchSize{MaxMessageCount: maxMessageCount}}, filters)

	batches, committers, ok := r.Ordered(goodTx)

	if batches != nil || committers != nil {
		t.Fatalf("Should not have created batch")
	}

	if !ok {
		t.Fatalf("Should have enqueued good message into batch")
	}

	batches, committers, ok = r.Ordered(isolatedTx)

	if !ok {
		t.Fatalf("Should have enqueued isolated message")
	}

	if len(batches) != 2 || len(committers) != 2 {
		t.Fatalf("Should have created two batches, got %d and %d", len(batches), len(committers))
	}

	if len(batches[0]) != 1 || len(committers[0]) != 1 {
		t.Fatalf("Should have had one normal tx in the first batch got %d and %d committers", len(batches[0]), len(committers[0]))
	}

	if !bytes.Equal(batches[0][0].Payload, goodTx.Payload) {
		t.Fatalf("Should have had the normal tx in the first batch")
	}

	if len(batches[1]) != 1 || len(committers[1]) != 1 {
		t.Fatalf("Should have had one isolated tx in the second batch got %d and %d committers", len(batches[1]), len(committers[1]))
	}

	if !bytes.Equal(batches[1][0].Payload, isolatedTx.Payload) {
		t.Fatalf("Should have had the isolated tx in the second batch")
	}
}
