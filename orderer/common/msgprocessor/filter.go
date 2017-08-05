/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msgprocessor

import (
	"errors"

	ab "github.com/hyperledger/fabric/protos/common"
)

// ErrEmptyMessage is returned by the empty message filter on rejection.
var ErrEmptyMessage = errors.New("Message was empty")

// Rule defines a filter function which accepts, rejects, or forwards (to the next rule) an Envelope
type Rule interface {
	// Apply applies the rule to the given Envelope, either successfully or returns error
	Apply(message *ab.Envelope) error
}

// EmptyRejectRule rejects empty messages
var EmptyRejectRule = Rule(emptyRejectRule{})

type emptyRejectRule struct{}

func (a emptyRejectRule) Apply(message *ab.Envelope) error {
	if message.Payload == nil {
		return ErrEmptyMessage
	}
	return nil
}

// AcceptRule always returns Accept as a result for Apply
var AcceptRule = Rule(acceptRule{})

type acceptRule struct{}

func (a acceptRule) Apply(message *ab.Envelope) error {
	return nil
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

// Apply applies the rules given for this set in order, returning nil on valid or err on invalid
func (rs *RuleSet) Apply(message *ab.Envelope) error {
	for _, rule := range rs.rules {
		err := rule.Apply(message)
		if err != nil {
			return err
		}
	}
	return nil
}
