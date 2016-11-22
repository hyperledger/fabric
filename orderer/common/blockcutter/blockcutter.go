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
	"github.com/hyperledger/fabric/orderer/common/broadcastfilter"
	"github.com/hyperledger/fabric/orderer/common/configtx"
	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/golang/protobuf/proto"
	"github.com/op/go-logging"
)

var logger = logging.MustGetLogger("orderer/common/blockcutter")

func init() {
	logging.SetLevel(logging.DEBUG, "")
}

// Receiver defines a sink for the ordered broadcast messages
type Receiver interface {
	// Ordered should be invoked sequentially as messages are ordered
	// If the message is a valid normal message and does not fill the batch, nil, true is returned
	// If the message is a valid normal message and fills a batch, the batch, true is returned
	// If the message is a valid special message (like a config message) it terminates the current batch
	// and returns the current batch (if it is not empty), plus a second batch containing the special transaction and true
	// If the ordered message is determined to be invalid, then nil, false is returned
	Ordered(msg *cb.Envelope) ([][]*cb.Envelope, bool)

	// Cut returns the current batch and starts a new one
	Cut() []*cb.Envelope
}

type receiver struct {
	batchSize     int
	filters       *broadcastfilter.RuleSet
	configManager configtx.Manager
	curBatch      []*cb.Envelope
}

func NewReceiverImpl(batchSize int, filters *broadcastfilter.RuleSet, configManager configtx.Manager) Receiver {
	return &receiver{
		batchSize:     batchSize,
		filters:       filters,
		configManager: configManager,
	}
}

// Ordered should be invoked sequentially as messages are ordered
// If the message is a valid normal message and does not fill the batch, nil, true is returned
// If the message is a valid normal message and fills a batch, the batch, true is returned
// If the message is a valid special message (like a config message) it terminates the current batch
// and returns the current batch (if it is not empty), plus a second batch containing the special transaction and true
// If the ordered message is determined to be invalid, then nil, false is returned
func (r *receiver) Ordered(msg *cb.Envelope) ([][]*cb.Envelope, bool) {
	// The messages must be filtered a second time in case configuration has changed since the message was received
	action, _ := r.filters.Apply(msg)
	switch action {
	case broadcastfilter.Accept:
		logger.Debugf("Enqueuing message into batch")
		r.curBatch = append(r.curBatch, msg)

		if len(r.curBatch) < r.batchSize {
			return nil, true
		}

		logger.Debugf("Batch size met, creating block")
		newBatch := r.curBatch
		r.curBatch = nil
		return [][]*cb.Envelope{newBatch}, true
	case broadcastfilter.Reconfigure:
		// TODO, this is unmarshaling for a second time, we need a cleaner interface, maybe Apply returns a second arg with thing to put in the batch
		payload := &cb.Payload{}
		if err := proto.Unmarshal(msg.Payload, payload); err != nil {
			logger.Errorf("A change was flagged as configuration, but could not be unmarshaled: %v", err)
			return nil, false
		}
		newConfig := &cb.ConfigurationEnvelope{}
		if err := proto.Unmarshal(payload.Data, newConfig); err != nil {
			logger.Errorf("A change was flagged as configuration, but could not be unmarshaled: %v", err)
			return nil, false
		}
		err := r.configManager.Validate(newConfig)
		if err != nil {
			logger.Warningf("A configuration change made it through the ingress filter but could not be included in a batch: %v", err)
			return nil, false
		}

		logger.Debugf("Configuration change applied successfully, committing previous block and configuration block")
		firstBatch := r.curBatch
		r.curBatch = nil
		secondBatch := []*cb.Envelope{msg}
		if firstBatch == nil {
			return [][]*cb.Envelope{secondBatch}, true
		} else {
			return [][]*cb.Envelope{firstBatch, secondBatch}, true
		}
	case broadcastfilter.Reject:
		logger.Debugf("Rejecting message")
		return nil, false
	case broadcastfilter.Forward:
		logger.Debugf("Ignoring message because it was not accepted by a filter")
		return nil, false
	default:
		logger.Fatalf("Received an unknown rule response: %v", action)
	}

	return nil, false // Unreachable

}

// Cut returns the current batch and starts a new one
func (r *receiver) Cut() []*cb.Envelope {
	batch := r.curBatch
	r.curBatch = nil
	return batch
}
