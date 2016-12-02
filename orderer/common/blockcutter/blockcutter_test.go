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
	"fmt"
	"testing"

	"github.com/hyperledger/fabric/orderer/common/broadcastfilter"
	"github.com/hyperledger/fabric/orderer/common/configtx"
	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/golang/protobuf/proto"
)

type mockConfigManager struct {
	validated   bool
	validateErr error
}

func (mcm *mockConfigManager) Validate(configtx *cb.ConfigurationEnvelope) error {
	mcm.validated = true
	return mcm.validateErr
}

func (mcm *mockConfigManager) Apply(message *cb.ConfigurationEnvelope) error {
	panic("Unimplemented")
}

func (mcm *mockConfigManager) ChainID() string {
	panic("Unimplemented")
}

type mockConfigFilter struct {
	manager configtx.Manager
}

func (mcf *mockConfigFilter) Apply(msg *cb.Envelope) broadcastfilter.Action {
	if bytes.Equal(msg.Payload, configTx.Payload) {
		if mcf.manager == nil || mcf.manager.Validate(nil) != nil {
			return broadcastfilter.Reject
		}
		return broadcastfilter.Reconfigure
	}
	return broadcastfilter.Forward
}

type mockRejectFilter struct{}

func (mrf mockRejectFilter) Apply(message *cb.Envelope) broadcastfilter.Action {
	if bytes.Equal(message.Payload, badTx.Payload) {
		return broadcastfilter.Reject
	}
	return broadcastfilter.Forward
}

type mockAcceptFilter struct{}

func (mrf mockAcceptFilter) Apply(message *cb.Envelope) broadcastfilter.Action {
	if bytes.Equal(message.Payload, goodTx.Payload) {
		return broadcastfilter.Accept
	}
	return broadcastfilter.Forward
}

func getFiltersAndConfig() (*broadcastfilter.RuleSet, *mockConfigManager) {
	cm := &mockConfigManager{}
	filters := broadcastfilter.NewRuleSet([]broadcastfilter.Rule{
		&mockConfigFilter{cm},
		&mockRejectFilter{},
		&mockAcceptFilter{},
	})
	return filters, cm
}

var badTx = &cb.Envelope{Payload: []byte("BAD")}
var goodTx = &cb.Envelope{Payload: []byte("GOOD")}
var configTx = &cb.Envelope{Payload: []byte("CONFIG")}
var unmatchedTx = &cb.Envelope{Payload: []byte("UNMATCHED")}

func init() {
	configBytes, err := proto.Marshal(&cb.ConfigurationEnvelope{})
	if err != nil {
		panic("Error marshaling empty config tx")
	}
	configTx = &cb.Envelope{Payload: configBytes}

}

func TestNormalBatch(t *testing.T) {
	filters, cm := getFiltersAndConfig()
	batchSize := 2
	r := NewReceiverImpl(batchSize, filters, cm)

	batches, ok := r.Ordered(goodTx)

	if batches != nil {
		t.Fatalf("Should not have created batch")
	}

	if !ok {
		t.Fatalf("Should have enqueued message into batch")
	}

	batches, ok = r.Ordered(goodTx)

	if batches == nil {
		t.Fatalf("Should have created batch")
	}

	if !ok {
		t.Fatalf("Should have enqueued second message into batch")
	}

}

func TestBadMessageInBatch(t *testing.T) {
	filters, cm := getFiltersAndConfig()
	batchSize := 2
	r := NewReceiverImpl(batchSize, filters, cm)

	batches, ok := r.Ordered(badTx)

	if batches != nil {
		t.Fatalf("Should not have created batch")
	}

	if ok {
		t.Fatalf("Should not have enqueued bad message into batch")
	}

	batches, ok = r.Ordered(goodTx)

	if batches != nil {
		t.Fatalf("Should not have created batch")
	}

	if !ok {
		t.Fatalf("Should have enqueued good message into batch")
	}

	batches, ok = r.Ordered(badTx)

	if batches != nil {
		t.Fatalf("Should not have created batch")
	}

	if ok {
		t.Fatalf("Should not have enqueued second bad message into batch")
	}
}

func TestUnmatchedMessageInBatch(t *testing.T) {
	filters, cm := getFiltersAndConfig()
	batchSize := 2
	r := NewReceiverImpl(batchSize, filters, cm)

	batches, ok := r.Ordered(unmatchedTx)

	if batches != nil {
		t.Fatalf("Should not have created batch")
	}

	if ok {
		t.Fatalf("Should not have enqueued unmatched message into batch")
	}

	batches, ok = r.Ordered(goodTx)

	if batches != nil {
		t.Fatalf("Should not have created batch")
	}

	if !ok {
		t.Fatalf("Should have enqueued good message into batch")
	}

	batches, ok = r.Ordered(unmatchedTx)

	if batches != nil {
		t.Fatalf("Should not have created batch from unmatched message")
	}

	if ok {
		t.Fatalf("Should not have enqueued second bad message into batch")
	}
}

func TestReconfigureEmptyBatch(t *testing.T) {
	filters, cm := getFiltersAndConfig()
	batchSize := 2
	r := NewReceiverImpl(batchSize, filters, cm)

	batches, ok := r.Ordered(configTx)

	if !ok {
		t.Fatalf("Should have enqueued config message")
	}

	if !cm.validated {
		t.Errorf("ConfigTx should have been validated before processing")
	}

	if len(batches) != 1 {
		t.Fatalf("Should created new batch, got %d", len(batches))
	}

	if len(batches[0]) != 1 {
		t.Fatalf("Should have had one config tx in the second batch got %d", len(batches[1]))
	}

	if !bytes.Equal(batches[0][0].Payload, configTx.Payload) {
		t.Fatalf("Should have had the normal tx in the first batch")
	}
}

func TestReconfigurePartialBatch(t *testing.T) {
	filters, cm := getFiltersAndConfig()
	batchSize := 2
	r := NewReceiverImpl(batchSize, filters, cm)

	batches, ok := r.Ordered(goodTx)

	if batches != nil {
		t.Fatalf("Should not have created batch")
	}

	if !ok {
		t.Fatalf("Should have enqueued good message into batch")
	}

	batches, ok = r.Ordered(configTx)

	if !ok {
		t.Fatalf("Should have enqueued config message")
	}

	if !cm.validated {
		t.Errorf("ConfigTx should have been validated before processing")
	}

	if len(batches) != 2 {
		t.Fatalf("Should have created two batches, got %d", len(batches))
	}

	if len(batches[0]) != 1 {
		t.Fatalf("Should have had one normal tx in the first batch got %d", len(batches[0]))
	}

	if !bytes.Equal(batches[0][0].Payload, goodTx.Payload) {
		t.Fatalf("Should have had the normal tx in the first batch")
	}

	if len(batches[1]) != 1 {
		t.Fatalf("Should have had one config tx in the second batch got %d", len(batches[1]))
	}

	if !bytes.Equal(batches[1][0].Payload, configTx.Payload) {
		t.Fatalf("Should have had the normal tx in the first batch")
	}
}

func TestReconfigureFailToVerify(t *testing.T) {
	filters, cm := getFiltersAndConfig()
	cm.validateErr = fmt.Errorf("Fail to apply")
	batchSize := 2
	r := NewReceiverImpl(batchSize, filters, cm)

	batches, ok := r.Ordered(goodTx)

	if batches != nil {
		t.Fatalf("Should not have created batch")
	}

	if !ok {
		t.Fatalf("Should have enqueued good message into batch")
	}

	batches, ok = r.Ordered(configTx)

	if !cm.validated {
		t.Errorf("ConfigTx should have been validated before processing")
	}

	if batches != nil {
		t.Fatalf("Should not have created batch")
	}

	if ok {
		t.Fatalf("Should not have enqueued bad config message into batch")
	}

	batches, ok = r.Ordered(goodTx)

	if batches == nil {
		t.Fatalf("Should have created batch")
	}

	if len(batches) != 1 {
		t.Fatalf("Batches should only have had one batch")
	}

	if len(batches[0]) != 2 {
		t.Fatalf("Should have had full batch")
	}

	if !ok {
		t.Fatalf("Should have enqueued good message into batch")
	}
}
