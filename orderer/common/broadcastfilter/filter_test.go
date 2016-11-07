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
	"testing"

	cb "github.com/hyperledger/fabric/protos/common"
)

var RejectRule = Rule(rejectRule{})

type rejectRule struct{}

func (r rejectRule) Apply(message *cb.Envelope) Action {
	return Reject
}

var ForwardRule = Rule(forwardRule{})

type forwardRule struct{}

func (r forwardRule) Apply(message *cb.Envelope) Action {
	return Forward
}

func TestEmptyRejectRule(t *testing.T) {
	rs := NewRuleSet([]Rule{EmptyRejectRule})
	result, rule := rs.Apply(&cb.Envelope{})
	if result != Reject {
		t.Fatalf("Should have rejected")
	}
	if rule != EmptyRejectRule {
		t.Fatalf("Rejected but not for the right rule")
	}
	result, _ = rs.Apply(&cb.Envelope{Payload: []byte("fakedata")})
	if result != Forward {
		t.Fatalf("Should have forwarded")
	}
}

func TestAcceptReject(t *testing.T) {
	rs := NewRuleSet([]Rule{AcceptRule, RejectRule})
	result, rule := rs.Apply(&cb.Envelope{})
	if result != Accept {
		t.Fatalf("Should have accepted")
	}
	if rule != AcceptRule {
		t.Fatalf("Accepted but not for the right rule")
	}
}

func TestRejectAccept(t *testing.T) {
	rs := NewRuleSet([]Rule{RejectRule, AcceptRule})
	result, rule := rs.Apply(&cb.Envelope{})
	if result != Reject {
		t.Fatalf("Should have rejected")
	}
	if rule != RejectRule {
		t.Fatalf("Rejected but not for the right rule")
	}
}

func TestForwardAccept(t *testing.T) {
	rs := NewRuleSet([]Rule{ForwardRule, AcceptRule})
	result, rule := rs.Apply(&cb.Envelope{})
	if result != Accept {
		t.Fatalf("Should have accepted")
	}
	if rule != AcceptRule {
		t.Fatalf("Accepted but not for the right rule")
	}
}

func TestForward(t *testing.T) {
	rs := NewRuleSet([]Rule{ForwardRule})
	result, rule := rs.Apply(&cb.Envelope{})
	if result != Forward {
		t.Fatalf("Should have forwarded")
	}
	if rule != nil {
		t.Fatalf("Forwarded but rule is set")
	}
}

func TestNoRule(t *testing.T) {
	rs := NewRuleSet([]Rule{})
	result, rule := rs.Apply(&cb.Envelope{})
	if result != Forward {
		t.Fatalf("Should have forwarded")
	}
	if rule != nil {
		t.Fatalf("Forwarded but rule is set")
	}
}
