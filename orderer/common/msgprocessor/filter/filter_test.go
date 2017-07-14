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
	"testing"

	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/stretchr/testify/assert"
)

var RejectRule = Rule(rejectRule{})

type rejectRule struct{}

func (r rejectRule) Apply(message *cb.Envelope) error {
	return fmt.Errorf("Rejected")
}

func TestEmptyRejectRule(t *testing.T) {
	t.Run("Reject", func(t *testing.T) {
		assert.NotNil(t, EmptyRejectRule.Apply(&cb.Envelope{}))
	})
	t.Run("Accept", func(t *testing.T) {
		assert.Nil(t, EmptyRejectRule.Apply(&cb.Envelope{Payload: []byte("fakedata")}))
	})
}

func TestAcceptRule(t *testing.T) {
	assert.Nil(t, AcceptRule.Apply(&cb.Envelope{}))
}

func TestRuleSet(t *testing.T) {
	t.Run("RejectAccept", func(t *testing.T) {
		assert.NotNil(t, NewRuleSet([]Rule{RejectRule, AcceptRule}).Apply(&cb.Envelope{}))
	})
	t.Run("AcceptReject", func(t *testing.T) {
		assert.NotNil(t, NewRuleSet([]Rule{AcceptRule, RejectRule}).Apply(&cb.Envelope{}))
	})
	t.Run("AcceptAccept", func(t *testing.T) {
		assert.Nil(t, NewRuleSet([]Rule{AcceptRule, AcceptRule}).Apply(&cb.Envelope{}))
	})
	t.Run("Empty", func(t *testing.T) {
		assert.Nil(t, NewRuleSet(nil).Apply(&cb.Envelope{}))
	})
}
