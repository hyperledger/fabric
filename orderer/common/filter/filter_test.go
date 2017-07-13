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
	"testing"

	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/stretchr/testify/assert"
)

var RejectRule = Rule(rejectRule{})

type rejectRule struct{}

func (r rejectRule) Apply(message *cb.Envelope) (Action, Committer) {
	return Reject, nil
}

var ForwardRule = Rule(forwardRule{})

type forwardRule struct{}

func (r forwardRule) Apply(message *cb.Envelope) (Action, Committer) {
	return Forward, nil
}

func TestNoopCommitter(t *testing.T) {
	var nc noopCommitter
	assert.False(t, nc.Isolated(), "Should return false")
}

func TestEmptyRejectRule(t *testing.T) {
	result, _ := EmptyRejectRule.Apply(&cb.Envelope{})
	if result != Reject {
		t.Fatalf("Should have rejected")
	}
	result, _ = EmptyRejectRule.Apply(&cb.Envelope{Payload: []byte("fakedata")})
	if result != Forward {
		t.Fatalf("Should have forwarded")
	}
}

func TestAcceptReject(t *testing.T) {
	rs := NewRuleSet([]Rule{AcceptRule, RejectRule})
	_, err := rs.Apply(&cb.Envelope{})
	if err != nil {
		t.Fatalf("Should have accepted: %s", err)
	}
}

func TestRejectAccept(t *testing.T) {
	rs := NewRuleSet([]Rule{RejectRule, AcceptRule})
	_, err := rs.Apply(&cb.Envelope{})
	if err == nil {
		t.Fatalf("Should have rejected")
	}
}

func TestForwardAccept(t *testing.T) {
	rs := NewRuleSet([]Rule{ForwardRule, AcceptRule})
	_, err := rs.Apply(&cb.Envelope{})
	if err != nil {
		t.Fatalf("Should have accepted: %s ", err)
	}
}

func TestForward(t *testing.T) {
	rs := NewRuleSet([]Rule{ForwardRule})
	_, err := rs.Apply(&cb.Envelope{})
	if err == nil {
		t.Fatalf("Should have rejected")
	}
}

func TestNoRule(t *testing.T) {
	rs := NewRuleSet([]Rule{})
	_, err := rs.Apply(&cb.Envelope{})
	if err == nil {
		t.Fatalf("Should have rejected")
	}
}
