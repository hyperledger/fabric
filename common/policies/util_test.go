/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package policies

import (
	"testing"

	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/stretchr/testify/require"
)

func basicTest(t *testing.T, sv *StandardConfigPolicy) {
	require.NotNil(t, sv)
	require.NotEmpty(t, sv.Key())
	require.NotNil(t, sv.Value())
}

func TestUtilsBasic(t *testing.T) {
	basicTest(t, ImplicitMetaAnyPolicy("foo"))
	basicTest(t, ImplicitMetaAllPolicy("foo"))
	basicTest(t, ImplicitMetaMajorityPolicy("foo"))
	basicTest(t, SignaturePolicy("foo", &cb.SignaturePolicyEnvelope{}))
}
