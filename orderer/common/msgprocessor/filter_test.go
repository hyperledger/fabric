/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msgprocessor

import (
	"fmt"
	"testing"

	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/stretchr/testify/require"
)

var RejectRule = Rule(rejectRule{})

type rejectRule struct{}

func (r rejectRule) Apply(message *cb.Envelope) error {
	return fmt.Errorf("Rejected")
}

func TestEmptyRejectRule(t *testing.T) {
	t.Run("Reject", func(t *testing.T) {
		require.NotNil(t, EmptyRejectRule.Apply(&cb.Envelope{}))
	})
	t.Run("Accept", func(t *testing.T) {
		require.Nil(t, EmptyRejectRule.Apply(&cb.Envelope{Payload: []byte("fakedata")}))
	})
}

func TestAcceptRule(t *testing.T) {
	require.Nil(t, AcceptRule.Apply(&cb.Envelope{}))
}

func TestRuleSet(t *testing.T) {
	t.Run("RejectAccept", func(t *testing.T) {
		require.NotNil(t, NewRuleSet([]Rule{RejectRule, AcceptRule}).Apply(&cb.Envelope{}))
	})
	t.Run("AcceptReject", func(t *testing.T) {
		require.NotNil(t, NewRuleSet([]Rule{AcceptRule, RejectRule}).Apply(&cb.Envelope{}))
	})
	t.Run("AcceptAccept", func(t *testing.T) {
		require.Nil(t, NewRuleSet([]Rule{AcceptRule, AcceptRule}).Apply(&cb.Envelope{}))
	})
	t.Run("Empty", func(t *testing.T) {
		require.Nil(t, NewRuleSet(nil).Apply(&cb.Envelope{}))
	})
}
