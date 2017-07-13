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

package filter

import (
	"fmt"

	ab "github.com/hyperledger/fabric/protos/common"
)

// Action is used to express the output of a rule
type Action int

const (
	// Accept indicates that the message should be processed
	Accept = iota
	// Reject indicates that the message should not be processed
	Reject
	// Forward indicates that the rule could not determine the correct course of action
	Forward
)

// Rule defines a filter function which accepts, rejects, or forwards (to the next rule) an Envelope
type Rule interface {
	// Apply applies the rule to the given Envelope, replying with the Action to take for the message
	// If the filter Accepts a message, it should provide a committer to use when writing the message to the chain
	Apply(message *ab.Envelope) (Action, Committer)
}

// Committer is returned by postfiltering and should be invoked once the message has been written to the blockchain
type Committer interface {
	// Commit performs whatever action should be performed upon committing of a message
	Commit()

	// Isolated returns whether this transaction should have a block to itself or may be mixed with other transactions
	Isolated() bool
}

type noopCommitter struct{}

func (nc noopCommitter) Commit()        {}
func (nc noopCommitter) Isolated() bool { return false }

// NoopCommitter does nothing on commit and is not isolated
var NoopCommitter = Committer(noopCommitter{})

// EmptyRejectRule rejects empty messages
var EmptyRejectRule = Rule(emptyRejectRule{})

type emptyRejectRule struct{}

func (a emptyRejectRule) Apply(message *ab.Envelope) (Action, Committer) {
	if message.Payload == nil {
		return Reject, nil
	}
	return Forward, nil
}

// AcceptRule always returns Accept as a result for Apply
var AcceptRule = Rule(acceptRule{})

type acceptRule struct{}

func (a acceptRule) Apply(message *ab.Envelope) (Action, Committer) {
	return Accept, NoopCommitter
}

// RuleSet is used to apply a collection of rules
type RuleSet struct {
	rules []Rule
}

// NewRuleSet creates a new RuleSet with the given ordered list of Rules
func NewRuleSet(rules []Rule) *RuleSet {
	return &RuleSet{
		rules: rules,
	}
}

// Apply applies the rules given for this set in order, returning the committer, nil on valid, or nil, err on invalid
func (rs *RuleSet) Apply(message *ab.Envelope) (Committer, error) {
	for _, rule := range rs.rules {
		action, committer := rule.Apply(message)
		switch action {
		case Accept:
			return committer, nil
		case Reject:
			return nil, fmt.Errorf("Rejected by rule: %T", rule)
		default:
		}
	}
	return nil, fmt.Errorf("No matching filter found")
}
