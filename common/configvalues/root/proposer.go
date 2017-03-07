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

	api "github.com/hyperledger/fabric/common/configvalues"
	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/golang/protobuf/proto"
	logging "github.com/op/go-logging"
)

var logger = logging.MustGetLogger("common/config")

// Values defines a mechanism to supply messages to unamrshal from config
// and a mechanism to validate the results
type Values interface {
	// ProtoMsg behaves like a map lookup for key
	ProtoMsg(key string) (proto.Message, bool)

	// Validate should ensure that the values set into the proto messages are correct
	// and that the new group values are allowed
	Validate(map[string]api.ValueProposer) error

	// Commit should call back into the Value handler to update the config
	Commit()
}

// Handler
type Handler interface {
	Allocate() Values
	NewGroup(name string) (api.ValueProposer, error)
}

type config struct {
	allocated Values
	groups    map[string]api.ValueProposer
}

type Proposer struct {
	vh      Handler
	current *config
	pending *config
}

func NewProposer(vh Handler) *Proposer {
	return &Proposer{
		vh:      vh,
		current: &config{},
	}
}

// BeginValueProposals called when a config proposal is begun
func (p *Proposer) BeginValueProposals(groups []string) ([]api.ValueProposer, error) {
	if p.pending != nil {
		logger.Panicf("Duplicated BeginValueProposals without Rollback or Commit")
	}

	result := make([]api.ValueProposer, len(groups))

	p.pending = &config{
		allocated: p.vh.Allocate(),
		groups:    make(map[string]api.ValueProposer),
	}

	for i, groupName := range groups {
		var group api.ValueProposer
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
				p.pending = nil
				return nil, fmt.Errorf("Error creating group %s: %s", groupName, err)
			}
		}

		p.pending.groups[groupName] = group
		result[i] = group
	}

	return result, nil
}

// ProposeValue called when config is added to a proposal
func (p *Proposer) ProposeValue(key string, configValue *cb.ConfigValue) error {
	msg, ok := p.pending.allocated.ProtoMsg(key)
	if !ok {
		return fmt.Errorf("Unknown value key %s for %T", key, p.vh)
	}

	if err := proto.Unmarshal(configValue.Value, msg); err != nil {
		return fmt.Errorf("Error unmarshaling key to proto message: %s", err)
	}

	return nil
}

// Validate ensures that the new config values is a valid change
func (p *Proposer) PreCommit() error {
	return p.pending.allocated.Validate(p.pending.groups)
}

// RollbackProposals called when a config proposal is abandoned
func (p *Proposer) RollbackProposals() {
	p.pending = nil
}

// CommitProposals called when a config proposal is committed
func (p *Proposer) CommitProposals() {
	if p.pending == nil {
		logger.Panicf("Attempted to commit with no pending values (indicates no Begin invoked)")
	}
	p.current = p.pending
	p.current.allocated.Commit()
	p.pending = nil
}
