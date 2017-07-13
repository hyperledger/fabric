/*
Copyright IBM Corp. 2017 All Rights Reserved.

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

package config

import (
	"fmt"
	"sync"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/flogging"
)

var logger = flogging.MustGetLogger("common/config")

// ValueDeserializer provides a mechanism to retrieve proto messages to deserialize config values into
type ValueDeserializer interface {
	// Deserialize takes a Value key as a string, and a marshaled Value value as bytes
	// and returns the deserialized version of that value.  Note, this function operates
	// with side effects intended.  Using a ValueDeserializer to deserialize a message will
	// generally set the value in the Values interface that the ValueDeserializer derived from
	// Therefore, the proto.Message may be safely discarded, but may be retained for
	// inspection and or debugging purposes.
	Deserialize(key string, value []byte) (proto.Message, error)
}

// Values defines a mechanism to supply messages to unamrshal from config
// and a mechanism to validate the results
type Values interface {
	ValueDeserializer

	// Validate should ensure that the values set into the proto messages are correct
	// and that the new group values are allowed.  It also includes a tx ID in case cross
	// Handler invocations (ie to the MSP Config Manager) must be made
	Validate(interface{}, map[string]ValueProposer) error

	// Commit should call back into the Value handler to update the config
	Commit()
}

// Handler
type Handler interface {
	Allocate() Values
	NewGroup(name string) (ValueProposer, error)
}

type config struct {
	allocated Values
	groups    map[string]ValueProposer
}

type Proposer struct {
	vh          Handler
	pending     map[interface{}]*config
	current     *config
	pendingLock sync.RWMutex
}

func NewProposer(vh Handler) *Proposer {
	return &Proposer{
		vh:      vh,
		current: &config{},
		pending: make(map[interface{}]*config),
	}
}

// BeginValueProposals called when a config proposal is begun
func (p *Proposer) BeginValueProposals(tx interface{}, groups []string) (ValueDeserializer, []ValueProposer, error) {
	p.pendingLock.Lock()
	defer p.pendingLock.Unlock()
	if _, ok := p.pending[tx]; ok {
		logger.Panicf("Duplicated BeginValueProposals without Rollback or Commit")
	}

	result := make([]ValueProposer, len(groups))

	pending := &config{
		allocated: p.vh.Allocate(),
		groups:    make(map[string]ValueProposer),
	}

	for i, groupName := range groups {
		var group ValueProposer
		var ok bool

		if p.current == nil {
			ok = false
		} else {
			group, ok = p.current.groups[groupName]
		}

		if !ok {
			var err error
			group, err = p.vh.NewGroup(groupName)
			if err != nil {
				pending = nil
				return nil, nil, fmt.Errorf("Error creating group %s: %s", groupName, err)
			}
		}

		pending.groups[groupName] = group
		result[i] = group
	}

	p.pending[tx] = pending

	return pending.allocated, result, nil
}

// Validate ensures that the new config values is a valid change
func (p *Proposer) PreCommit(tx interface{}) error {
	p.pendingLock.RLock()
	pending, ok := p.pending[tx]
	p.pendingLock.RUnlock()
	if !ok {
		logger.Panicf("Serious Programming Error: attempted to pre-commit tx which had not been begun")
	}
	return pending.allocated.Validate(tx, pending.groups)
}

// RollbackProposals called when a config proposal is abandoned
func (p *Proposer) RollbackProposals(tx interface{}) {
	p.pendingLock.Lock()
	defer p.pendingLock.Unlock()
	delete(p.pending, tx)
}

// CommitProposals called when a config proposal is committed
func (p *Proposer) CommitProposals(tx interface{}) {
	p.pendingLock.Lock()
	defer p.pendingLock.Unlock()
	pending, ok := p.pending[tx]
	if !ok {
		logger.Panicf("Serious Programming Error: attempted to commit tx which had not been begun")
	}
	p.current = pending
	p.current.allocated.Commit()
	delete(p.pending, tx)
}
