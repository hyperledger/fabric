/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package policies

import (
	"testing"

	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/stretchr/testify/assert"
)

func basicTest(t *testing.T, sv *StandardConfigPolicy) {
	assert.NotNil(t, sv)
	assert.NotEmpty(t, sv.Key())
	assert.NotNil(t, sv.Value())
}

func TestUtilsBasic(t *testing.T) {
	basicTest(t, ImplicitMetaAnyPolicy("foo"))
	basicTest(t, ImplicitMetaAllPolicy("foo"))
	basicTest(t, ImplicitMetaMajorityPolicy("foo"))
	basicTest(t, SignaturePolicy("foo", &cb.SignaturePolicyEnvelope{}))
}
