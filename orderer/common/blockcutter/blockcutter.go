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
	"github.com/hyperledger/fabric/common/config"
	"github.com/hyperledger/fabric/orderer/common/filter"
	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/op/go-logging"
)

var logger = logging.MustGetLogger("orderer/common/blockcutter")

// Receiver defines a sink for the ordered broadcast messages
type Receiver interface {
	// Ordered should be invoked sequentially as messages are ordered
	// If the current message valid, and no batches need to be cut:
	//   - Ordered will return (nil, true) (indicating ok).
	// If the current message is valid, and batches need to be cut:
	//   - Ordered will return 1 or 2 batches of messages, and true (indicating ok).
	// If the current message is invalid:
	//   - Ordered will return (nil, false) (to indicate not ok).
	//
	// Given a valid message, if the current message needs to be isolated because it exceeds the preferred batch size
	//   - Ordered will return:
	//     * The pending batch (if not empty), and a second batch containing only the isolated message.
	//     * true (indicating ok).
	// Otherwise, given a valid message, the pending batch, if not empty, will be cut and returned if:
	//   - The current message will cause the pending batch size in bytes to exceed BatchSize.PreferredMaxBytes.
	//   - After adding the current message to the pending batch, the message count has reached BatchSize.MaxMessageCount.
	Ordered(msg *cb.Envelope) ([][]*cb.Envelope, bool)

	// Cut returns the current batch and starts a new one
	Cut() []*cb.Envelope
}

type receiver struct {
	sharedConfigManager   config.Orderer
	filters               *filter.RuleSet
	pendingBatch          []*cb.Envelope
	pendingBatchSizeBytes uint32
	pendingCommitters     []filter.Committer
}

// NewReceiverImpl creates a Receiver implementation based on the given configtxorderer manager and filters
func NewReceiverImpl(sharedConfigManager config.Orderer, filters *filter.RuleSet) Receiver {
	return &receiver{
		sharedConfigManager: sharedConfigManager,
		filters:             filters,
	}
}

// Ordered should be invoked sequentially as messages are ordered
// If the current message valid, and no batches need to be cut:
//   - Ordered will return nil, nil, and true (indicating ok).
// If the current message valid, and batches need to be cut:
//   - Ordered will return 1 or 2 batches of messages, 1 or 2 batches of committers, and true (indicating ok).
// If the current message is invalid:
//   - Ordered will return nil, nil, and false (to indicate not ok).
//
// Given a valid message, if the current message needs to be isolated (as determined during filtering).
//   - Ordered will return:
//     * The pending batch of (if not empty), and a second batch containing only the isolated message.
//     * The corresponding batches of committers.
//     * true (indicating ok).
// Otherwise, given a valid message, the pending batch, if not empty, will be cut and returned if:
//   - The current message needs to be isolated (as determined during filtering).
//   - The current message will cause the pending batch size in bytes to exceed BatchSize.PreferredMaxBytes.
//   - After adding the current message to the pending batch, the message count has reached BatchSize.MaxMessageCount.
func (r *receiver) Ordered(msg *cb.Envelope) ([][]*cb.Envelope, bool) {
	// The messages must be filtered a second time in case configuration has changed since the message was received
	committer, err := r.filters.Apply(msg)
	if err != nil {
		logger.Debugf("Rejecting message: %s", err)
		return nil, false
	}

	if committer.Isolated() {
		logger.Panicf("The use of isolated committers has been deprecated and should no longer appear in this path")
	}

	messageSizeBytes := messageSizeBytes(msg)

	if messageSizeBytes > r.sharedConfigManager.BatchSize().PreferredMaxBytes {
		logger.Debugf("The current message, with %v bytes, is larger than the preferred batch size of %v bytes and will be isolated.", messageSizeBytes, r.sharedConfigManager.BatchSize().PreferredMaxBytes)

		var messageBatches [][]*cb.Envelope

		// cut pending batch, if it has any messages
		if len(r.pendingBatch) > 0 {
			messageBatch := r.Cut()
			messageBatches = append(messageBatches, messageBatch)
		}

		// create new batch with single message
		messageBatches = append(messageBatches, []*cb.Envelope{msg})

		return messageBatches, true
	}

	var messageBatches [][]*cb.Envelope

	messageWillOverflowBatchSizeBytes := r.pendingBatchSizeBytes+messageSizeBytes > r.sharedConfigManager.BatchSize().PreferredMaxBytes

	if messageWillOverflowBatchSizeBytes {
		logger.Debugf("The current message, with %v bytes, will overflow the pending batch of %v bytes.", messageSizeBytes, r.pendingBatchSizeBytes)
		logger.Debugf("Pending batch would overflow if current message is added, cutting batch now.")
		messageBatch := r.Cut()
		messageBatches = append(messageBatches, messageBatch)
	}

	logger.Debugf("Enqueuing message into batch")
	r.pendingBatch = append(r.pendingBatch, msg)
	r.pendingBatchSizeBytes += messageSizeBytes

	if uint32(len(r.pendingBatch)) >= r.sharedConfigManager.BatchSize().MaxMessageCount {
		logger.Debugf("Batch size met, cutting batch")
		messageBatch := r.Cut()
		messageBatches = append(messageBatches, messageBatch)
	}

	return messageBatches, true
}

// Cut returns the current batch and starts a new one
func (r *receiver) Cut() []*cb.Envelope {
	batch := r.pendingBatch
	r.pendingBatch = nil
	r.pendingBatchSizeBytes = 0
	return batch
}

func messageSizeBytes(message *cb.Envelope) uint32 {
	return uint32(len(message.Payload) + len(message.Signature))
}
