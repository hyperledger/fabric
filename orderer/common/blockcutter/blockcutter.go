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
	"github.com/hyperledger/fabric/orderer/common/filter"
	"github.com/hyperledger/fabric/orderer/common/sharedconfig"
	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/op/go-logging"
)

var logger = logging.MustGetLogger("orderer/common/blockcutter")

func init() {
	logging.SetLevel(logging.DEBUG, "")
}

// Receiver defines a sink for the ordered broadcast messages
type Receiver interface {
	// Ordered should be invoked sequentially as messages are ordered
	// If the message is a valid normal message and does not fill the batch, nil, nil, true is returned
	// If the message is a valid normal message and fills a batch, the batch, committers, true is returned
	// If the message is a valid special message (like a config message) it terminates the current batch
	// and returns the current batch and committers (if it is not empty), plus a second batch containing the special transaction and commiter, and true
	// If the ordered message is determined to be invalid, then nil, nil, false is returned
	Ordered(msg *cb.Envelope) ([][]*cb.Envelope, [][]filter.Committer, bool)

	// Cut returns the current batch and starts a new one
	Cut() ([]*cb.Envelope, []filter.Committer)
}

type receiver struct {
	sharedConfigManager sharedconfig.Manager
	filters             *filter.RuleSet
	curBatch            []*cb.Envelope
	batchComs           []filter.Committer
}

// NewReceiverImpl creates a Receiver implementation based on the given sharedconfig manager and filters
func NewReceiverImpl(sharedConfigManager sharedconfig.Manager, filters *filter.RuleSet) Receiver {
	return &receiver{
		sharedConfigManager: sharedConfigManager,
		filters:             filters,
	}
}

// Ordered should be invoked sequentially as messages are ordered
// If the message is a valid normal message and does not fill the batch, nil, nil, true is returned
// If the message is a valid normal message and fills a batch, the batch, committers, true is returned
// If the message is a valid special message (like a config message) it terminates the current batch
// and returns the current batch and committers (if it is not empty), plus a second batch containing the special transaction and commiter, and true
// If the ordered message is determined to be invalid, then nil, nil, false is returned
func (r *receiver) Ordered(msg *cb.Envelope) ([][]*cb.Envelope, [][]filter.Committer, bool) {
	// The messages must be filtered a second time in case configuration has changed since the message was received
	committer, err := r.filters.Apply(msg)
	if err != nil {
		logger.Debugf("Rejecting message: %s", err)
		return nil, nil, false
	}

	if committer.Isolated() {
		logger.Debugf("Found message which requested to be isolated, cutting into its own block")
		firstBatch := r.curBatch
		r.curBatch = nil
		firstComs := r.batchComs
		r.batchComs = nil
		secondBatch := []*cb.Envelope{msg}
		if firstBatch == nil {
			return [][]*cb.Envelope{secondBatch}, [][]filter.Committer{[]filter.Committer{committer}}, true
		}
		return [][]*cb.Envelope{firstBatch, secondBatch}, [][]filter.Committer{firstComs, []filter.Committer{committer}}, true
	}

	logger.Debugf("Enqueuing message into batch")
	r.curBatch = append(r.curBatch, msg)
	r.batchComs = append(r.batchComs, committer)

	if uint32(len(r.curBatch)) < r.sharedConfigManager.BatchSize().MaxMessageCount {
		return nil, nil, true
	}

	logger.Debugf("Batch size met, creating block")
	newBatch := r.curBatch
	newComs := r.batchComs
	r.curBatch = nil
	return [][]*cb.Envelope{newBatch}, [][]filter.Committer{newComs}, true
}

// Cut returns the current batch and starts a new one
func (r *receiver) Cut() ([]*cb.Envelope, []filter.Committer) {
	batch := r.curBatch
	r.curBatch = nil
	committers := r.batchComs
	r.batchComs = nil
	return batch, committers
}
