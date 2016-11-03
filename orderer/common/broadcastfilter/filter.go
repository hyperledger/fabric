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

package broadcastfilter

import (
	ab "github.com/hyperledger/fabric/protos/common"
)

// Action is used to express the output of a rule
type Action int

const (
	// Accept indicates that the message should be processed
	Accept = iota
	// Reconfigure indicates that this message modifies this rule, and should therefore be processed in a batch by itself
	Reconfigure
	// Reject indicates that the message should not be processed
	Reject
	// Forward indicates that the rule could not determine the correct course of action
	Forward
)

// Rule defines a filter function which accepts, rejects, or forwards (to the next rule) a Envelope
type Rule interface {
	// Apply applies the rule to the given Envelope, replying with the Action to take for the message
	Apply(message *ab.Envelope) Action
}

// EmptyRejectRule rejects empty messages
var EmptyRejectRule = Rule(emptyRejectRule{})

type emptyRejectRule struct{}

func (a emptyRejectRule) Apply(message *ab.Envelope) Action {
	if message.Payload == nil {
		return Reject
	}
	return Forward
}

// AcceptRule always returns Accept as a result for Apply
var AcceptRule = Rule(acceptRule{})

type acceptRule struct{}

func (a acceptRule) Apply(message *ab.Envelope) Action {
	return Accept
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

// Apply applies the rules given for this set in order, returning the first non-Forward result and the Rule which generated it
// or returning Forward, nil if no rules accept or reject it
func (rs *RuleSet) Apply(message *ab.Envelope) (Action, Rule) {
	for _, rule := range rs.rules {
		action := rule.Apply(message)
		switch action {
		case Forward:
			continue
		default:
			return action, rule
		}
	}
	return Forward, nil
}
